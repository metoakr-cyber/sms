'use client';

/**
 * Sipariş canlı akışı — docs/frontend-contract.md §4.2'nin birebir uygulaması.
 *
 * Buradaki her madde GERÇEK bir arıza senaryosunu kapatır. Kullanıcı bu ekranda
 * parasını vermiş ve kod bekliyor; sessizce ölmüş bir bağlantı, "kod gelmedi"
 * şikâyeti olarak geri döner.
 *
 * ON MADDE (hepsi zorunlu):
 *   1. EventSource kur, code/status/cancelled olaylarını dinle
 *   2. Keepalive izleme: 25 sn sessizlik → bağlantı ölü, yeniden kur
 *   3. Üstel geri çekilme + jitter (1→2→4→8, tavan 15 sn)
 *   4. Yoklama yedeği: 10 sn'de bağlanamazsa 5 sn aralıkla REST
 *   5. visibilitychange → önce REST senkronu, sonra akış
 *   6. pageshow (bfcache) → yeniden kur
 *   7. online → yeniden kur
 *   8. Unmount'ta tam temizlik
 *   9. Terminal durumda KAPAT, yeniden kurma
 *  10. Taşıma göstergesi döndür
 */

import * as React from 'react';
import { apiFetch } from '@/lib/api';
import type { Order, OrderMessage } from '@/lib/types';

const KEEPALIVE_TIMEOUT_MS = 25_000; // sunucu 20 sn'de bir gönderir → 5 sn pay
const CONNECT_DEADLINE_MS = 10_000;  // bu süre içinde bağlanamazsa yoklamaya düş
const POLL_INTERVAL_MS = 5_000;
const BACKOFF_MS = [1_000, 2_000, 4_000, 8_000, 15_000];

export type Transport = 'connecting' | 'sse' | 'polling' | 'closed';

const TERMINAL = new Set(['COMPLETED', 'REFUNDED', 'CANCELLED', 'FAILED']);

export interface OrderStreamState {
  order: Order | null;
  messages: OrderMessage[];
  transport: Transport;
  /** expiresAt'ten TÜRETİLİR, yerel sayaçtan değil. */
  secondsLeft: number;
  cancellableIn: number;
  error: string | null;
}

export function useOrderStream(orderId: string | null, initial?: Order): OrderStreamState {
  const [order, setOrder] = React.useState<Order | null>(initial ?? null);
  const [messages, setMessages] = React.useState<OrderMessage[]>(initial?.messages ?? []);
  const [transport, setTransport] = React.useState<Transport>('connecting');
  const [error, setError] = React.useState<string | null>(null);

  // Zaman damgaları state DEĞİL ref: her tik yeniden render tetiklemesin.
  const expiresAtRef = React.useRef<number | null>(
    initial ? Date.parse(initial.expiresAt) : null);
  const cancellableAtRef = React.useRef<number | null>(
    initial ? Date.parse(initial.cancellableAt) : null);

  const [, forceTick] = React.useReducer((n: number) => n + 1, 0);

  const esRef = React.useRef<EventSource | null>(null);
  const timers = React.useRef<{ keepalive?: number; retry?: number; poll?: number; deadline?: number }>({});
  const attemptRef = React.useRef(0);
  const stoppedRef = React.useRef(false);

  /** Tüm zamanlayıcıları temizle — bellek sızıntısı ve hayalet istek yok. */
  const clearTimers = React.useCallback(() => {
    for (const k of ['keepalive', 'retry', 'poll', 'deadline'] as const) {
      const id = timers.current[k];
      if (id !== undefined) window.clearTimeout(id);
      if (k === 'poll' && id !== undefined) window.clearInterval(id);
      timers.current[k] = undefined;
    }
  }, []);

  const closeStream = React.useCallback(() => {
    esRef.current?.close();
    esRef.current = null;
  }, []);

  /** Siparişi REST ile tazele. Yoklama yedeği ve görünürlük senkronu bunu kullanır. */
  const syncOnce = React.useCallback(async (): Promise<Order | null> => {
    if (!orderId) return null;
    try {
      // no-store: Safari GET yanıtlarını agresif önbellekler; eski durum
      // "kod gelmedi" gibi görünür.
      const o = await apiFetch<Order>(`/orders/${encodeURIComponent(orderId)}`, { cache: 'no-store' });
      setOrder(o);
      setMessages(o.messages ?? []);
      expiresAtRef.current = Date.parse(o.expiresAt);
      cancellableAtRef.current = Date.parse(o.cancellableAt);
      setError(null);
      return o;
    } catch {
      // Tek bir başarısız yoklama hata göstermez: ağ bir saniyeliğine
      // kesilmiş olabilir ve kullanıcıyı korkutmanın anlamı yok.
      return null;
    }
  }, [orderId]);

  /** Yoklama moduna geç. */
  const startPolling = React.useCallback(() => {
    if (stoppedRef.current || timers.current.poll !== undefined) return;
    setTransport('polling');
    void syncOnce();
    timers.current.poll = window.setInterval(async () => {
      const o = await syncOnce();
      if (o && TERMINAL.has(o.status)) {
        stopAll();
      }
    }, POLL_INTERVAL_MS);
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [syncOnce]);

  const stopAll = React.useCallback(() => {
    stoppedRef.current = true;
    clearTimers();
    closeStream();
    setTransport('closed');
  }, [clearTimers, closeStream]);

  const connect = React.useCallback(() => {
    if (!orderId || stoppedRef.current) return;
    closeStream();

    // EventSource olmayan bir tarayıcıda (veya devre dışı bırakılmışsa)
    // doğrudan yoklamaya düşeriz — kullanıcı farkı hissetmez.
    if (typeof window === 'undefined' || typeof window.EventSource === 'undefined') {
      startPolling();
      return;
    }

    setTransport('connecting');
    const es = new EventSource(`/api/v1/orders/${encodeURIComponent(orderId)}/stream`, {
      withCredentials: true,
    });
    esRef.current = es;

    // BAĞLANTI SÜRESİ: 10 saniyede açılmazsa yoklamaya düşeriz. Bazı kurumsal
    // vekiller SSE'yi hiç açmaz ve EventSource sessizce bekler — kullanıcı
    // boş ekrana bakar.
    timers.current.deadline = window.setTimeout(() => {
      if (esRef.current?.readyState !== 1) {
        closeStream();
        startPolling();
      }
    }, CONNECT_DEADLINE_MS);

    // KEEPALIVE İZLEME: sunucu 20 sn'de bir yorum satırı gönderir. 25 saniye
    // hiçbir şey gelmediyse bağlantı ÖLMÜŞTÜR. EventSource bunu kendiliğinden
    // fark etmez: mobil ağlarda TCP bağlantısı "açık" görünmeye devam eder.
    const armKeepalive = () => {
      if (timers.current.keepalive !== undefined) window.clearTimeout(timers.current.keepalive);
      timers.current.keepalive = window.setTimeout(() => {
        closeStream();
        scheduleReconnect();
      }, KEEPALIVE_TIMEOUT_MS);
    };

    const onAnyTraffic = () => armKeepalive();

    es.onopen = () => {
      attemptRef.current = 0;
      setTransport('sse');
      setError(null);
      if (timers.current.deadline !== undefined) {
        window.clearTimeout(timers.current.deadline);
        timers.current.deadline = undefined;
      }
      // Yoklama yedeği açıksa kapat — iki kaynaktan veri çekmenin anlamı yok.
      if (timers.current.poll !== undefined) {
        window.clearInterval(timers.current.poll);
        timers.current.poll = undefined;
      }
      armKeepalive();
    };

    const handlePayload = (raw: string) => {
      onAnyTraffic();
      try {
        const data = JSON.parse(raw) as {
          status?: string;
          messages?: OrderMessage[];
          expiresAt?: string;
        };
        if (data.expiresAt) expiresAtRef.current = Date.parse(data.expiresAt);
        if (data.status) {
          setOrder((prev) => (prev ? { ...prev, status: data.status! } : prev));
        }
        if (data.messages?.length) {
          // Mesajlar BİRİKTİRİLİR, değiştirilmez: sipariş çok mesajlı olabilir
          // (FR-415) ve ikinci kod birincinin yerine geçmemeli.
          setMessages((prev) => {
            const seen = new Set(prev.map((m) => m.code + '|' + m.receivedAt));
            const add = data.messages!.filter((m) => !seen.has(m.code + '|' + m.receivedAt));
            return add.length ? [...prev, ...add] : prev;
          });
        }
        if (data.status && TERMINAL.has(data.status)) {
          // Terminal durum: akış KAPANIR ve yeniden kurulmaz.
          // Son bir REST senkronu yaparız ki iade tutarı gibi alanlar tam olsun.
          void syncOnce().then(stopAll);
        }
      } catch {
        // Bozuk gövde akışı düşürmez.
      }
    };

    es.addEventListener('status', (e) => handlePayload((e as MessageEvent).data));
    es.addEventListener('code', (e) => handlePayload((e as MessageEvent).data));
    es.addEventListener('cancelled', (e) => handlePayload((e as MessageEvent).data));
    es.addEventListener('close', () => { void syncOnce().then(stopAll); });
    es.onmessage = onAnyTraffic; // isimsiz olaylar da trafik sayılır

    es.onerror = () => {
      closeStream();
      scheduleReconnect();
    };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [orderId, closeStream, startPolling, syncOnce, stopAll]);

  /** Üstel geri çekilme + jitter. Jitter olmadan tüm istemciler aynı anda döner. */
  const scheduleReconnect = React.useCallback(() => {
    if (stoppedRef.current) return;
    const i = Math.min(attemptRef.current, BACKOFF_MS.length - 1);
    const wait = BACKOFF_MS[i]! + Math.random() * 400;
    attemptRef.current += 1;

    // İkinci başarısızlıktan sonra yoklamayı da açarız: kullanıcı 4 saniye
    // boyunca hiçbir güncelleme görmemeli.
    if (attemptRef.current >= 2) startPolling();

    timers.current.retry = window.setTimeout(connect, wait);
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [connect, startPolling]);

  /* ── Kurulum ── */

  React.useEffect(() => {
    if (!orderId) return;
    stoppedRef.current = false;
    attemptRef.current = 0;

    // Açılışta REST ile bir kez senkron: akış kurulana kadar geçen sürede
    // gelmiş bir kodu kaçırmayalım.
    void syncOnce().then((o) => {
      if (o && TERMINAL.has(o.status)) {
        stopAll();
        return;
      }
      connect();
    });

    return () => {
      stoppedRef.current = true;
      clearTimers();
      closeStream();
    };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [orderId]);

  /* ── Görünürlük, bfcache, ağ ── */

  React.useEffect(() => {
    if (!orderId) return;

    const resync = () => {
      if (stoppedRef.current) return;
      // ÖNCE REST: sekme arkada beklerken gelmiş bir kod olabilir ve akışın
      // yeniden kurulmasını beklemek kullanıcıyı boş ekranda tutar.
      void syncOnce().then((o) => {
        if (o && TERMINAL.has(o.status)) { stopAll(); return; }
        if (esRef.current?.readyState !== 1) connect();
      });
    };

    const onVisibility = () => { if (document.visibilityState === 'visible') resync(); };
    // bfcache'ten geri dönüş: Safari'de sayfa "dondurulmuş" hâlde geri gelir
    // ve EventSource ölüdür ama onerror TETİKLENMEZ.
    const onPageShow = (e: PageTransitionEvent) => { if (e.persisted) resync(); };
    const onOnline = () => resync();

    document.addEventListener('visibilitychange', onVisibility);
    window.addEventListener('pageshow', onPageShow);
    window.addEventListener('online', onOnline);
    return () => {
      document.removeEventListener('visibilitychange', onVisibility);
      window.removeEventListener('pageshow', onPageShow);
      window.removeEventListener('online', onOnline);
    };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [orderId]);

  /* ── Geri sayım ── */

  React.useEffect(() => {
    // SAYAÇ TUTULMAZ, her tikte expiresAt'ten YENİDEN HESAPLANIR.
    // Sekme dondurulduğunda setInterval kısılır; "kalan--" yapan bir sayaç
    // geride kalır ve kullanıcı süresi dolmuş numarayı canlı sanır.
    const id = window.setInterval(forceTick, 500);
    return () => window.clearInterval(id);
  }, []);

  const now = Date.now();
  const secondsLeft = expiresAtRef.current
    ? Math.max(0, Math.floor((expiresAtRef.current - now) / 1000)) : 0;
  const cancellableIn = cancellableAtRef.current
    ? Math.max(0, Math.ceil((cancellableAtRef.current - now) / 1000)) : 0;

  return { order, messages, transport, secondsLeft, cancellableIn, error };
}
