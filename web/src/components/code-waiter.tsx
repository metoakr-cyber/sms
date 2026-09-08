'use client';

/**
 * Kod bekleme ekranı — docs/frontend-contract.md §2.6.
 *
 * Kullanıcının EN GERGİN olduğu an: parası gitti, numara elinde, kod bekliyor.
 * Buradaki her ayrıntı o gerginliği azaltmak için:
 *   · kod çok büyük ve tek dokunuşla kopyalanır
 *   · numara seçilebilir (mobilde uzun basıp kopyalama yaygın)
 *   · geri sayım sunucudan gelir, donmuş sekmede yalan söylemez
 *   · bağlantı koptuğunda kullanıcı bunu ANLAR ve panik yapmaz
 *   · ekran uyanık kalır — kod gelirken telefon kilitlenmemeli
 */

import * as React from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { ApiError, apiFetch } from '@/lib/api';
import { formatMoney, formatDuration } from '@/lib/format';
import { useOrderStream } from '@/hooks/useOrderStream';
import { Button, Badge, Alert, cx } from './ui';
import { ServiceIcon } from './service-icon';
import type { Order } from '@/lib/types';

export function CodeWaiter({
  order: initial, onClose, iconUrl,
}: { order: Order; onClose: () => void; iconUrl?: string }) {
  const qc = useQueryClient();
  const { order, messages, transport, secondsLeft, cancellableIn } =
    useOrderStream(initial.id, initial);

  const current = order ?? initial;
  const latest = messages.length ? messages[messages.length - 1] : undefined;
  const done = current.status === 'COMPLETED';
  const refunded = current.status === 'REFUNDED' || current.status === 'CANCELLED';

  useWakeLock(!done && !refunded);
  useArrivalFeedback(latest?.code);

  const cancel = useMutation({
    mutationFn: () => apiFetch<Order>(`/orders/${encodeURIComponent(current.id)}`, { method: 'DELETE' }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['me'] });
      qc.invalidateQueries({ queryKey: ['orders'] });
    },
  });

  return (
    <div className="flex flex-col gap-4">
      {/* ── Servis + numara ── */}
      <div className="raised rounded-xl border p-4">
        <div className="flex items-center gap-3">
          <ServiceIcon name={current.serviceName} iconUrl={iconUrl} size={36} />
          <div className="min-w-0">
            <p className="truncate text-sm font-semibold">{current.serviceName}</p>
            <p className="text-xs text-muted">{current.countryName}</p>
          </div>
        </div>

        <div className="mt-4">
          <p className="text-xs font-medium uppercase tracking-wide text-muted">
            Tahsis edilen numara
          </p>
          <div className="mt-1.5 flex items-center justify-between gap-3">
            {/* select-text ZORUNLU: mobilde uzun basıp kopyalamak en yaygın yol */}
            <p className="select-text break-anywhere text-xl font-bold tracking-tight sm:text-2xl">
              {current.phoneNumber}
            </p>
            <CopyButton value={current.phoneNumber} label="Numarayı kopyala" />
          </div>
        </div>
      </div>

      {/* ── Kod ── */}
      {latest ? (
        <div className="rounded-xl border border-[var(--color-ok)]/40 bg-[var(--color-ok)]/10 p-4">
          <p className="text-xs font-medium uppercase tracking-wide text-[var(--color-ok)]">
            Doğrulama kodu geldi
          </p>
          <div className="mt-2 flex items-center justify-between gap-3">
            <p className="select-text text-4xl font-black tracking-[0.18em] text-[var(--color-ok)] sm:text-5xl">
              {latest.code || '—'}
            </p>
            {latest.code && <CopyButton value={latest.code} label="Kodu kopyala" big />}
          </div>
          {latest.body && (
            <p className="mt-3 select-text break-anywhere border-t border-[var(--color-ok)]/25 pt-3
                          text-xs leading-relaxed text-muted">
              {latest.body}
            </p>
          )}
        </div>
      ) : refunded ? (
        <Alert tone="warn">
          Sipariş iptal edildi ve <strong>{formatMoney(current.price)}</strong> bakiyenize
          iade edildi.
        </Alert>
      ) : (
        <WaitingPanel secondsLeft={secondsLeft} transport={transport} />
      )}

      {/* ── Birden çok mesaj (FR-415) ── */}
      {messages.length > 1 && (
        <div className="raised rounded-xl border p-4">
          <p className="text-xs font-medium uppercase tracking-wide text-muted">
            Önceki mesajlar
          </p>
          <ul className="mt-2 flex flex-col gap-2">
            {messages.slice(0, -1).map((m, i) => (
              <li key={`${m.receivedAt}-${i}`} className="flex items-center justify-between gap-3">
                <span className="select-text font-mono text-sm">{m.code || '—'}</span>
                <span className="truncate text-xs text-muted">{m.body}</span>
              </li>
            ))}
          </ul>
        </div>
      )}

      {/* ── Eylemler ── */}
      {!done && !refunded && (
        <>
          {cancel.error instanceof ApiError && (
            <Alert>
              {cancel.error.code === 'CANCEL_TOO_EARLY'
                ? 'Numarayı henüz iptal edemezsiniz. Geri sayım bitince tekrar deneyin.'
                : cancel.error.message}
            </Alert>
          )}
          <Button
            variant="danger"
            fullWidth
            loading={cancel.isPending}
            disabled={cancellableIn > 0}
            onClick={() => cancel.mutate()}
          >
            {cancellableIn > 0
              ? `İptal ve iade — ${formatDuration(cancellableIn)} sonra`
              : `İşlemi iptal et ve ${formatMoney(current.price)} iade al`}
          </Button>
          <p className="text-center text-xs leading-relaxed text-muted">
            SMS kodu geldiğinde otomatik olarak ekrana yansıyacaktır.
            {cancellableIn > 0 && ' Sağlayıcı ilk iki dakikada iptali kabul etmiyor.'}
          </p>
        </>
      )}

      {(done || refunded) && (
        <Button variant="outline" fullWidth onClick={onClose}>Kapat</Button>
      )}
    </div>
  );
}

/* ─────────────────────────── Bekleme paneli ─────────────────────────── */

function WaitingPanel({ secondsLeft, transport }: { secondsLeft: number; transport: string }) {
  const expired = secondsLeft <= 0;
  return (
    <div className="raised flex flex-col items-center gap-3 rounded-xl border p-6 text-center">
      {!expired && (
        <span className="relative flex size-3">
          <span className="absolute inline-flex size-full animate-ping rounded-full bg-brand-400 opacity-70" />
          <span className="relative inline-flex size-3 rounded-full bg-brand-500" />
        </span>
      )}
      <p className="font-medium">
        {expired ? 'Süre doldu — iade işleniyor' : 'SMS bekleniyor…'}
      </p>
      {!expired && (
        <p className="text-2xl font-bold tabular-nums">{formatDuration(secondsLeft)}</p>
      )}

      {/*
        BAĞLANTI GÖSTERGESİ.
        Yoklamaya düşmüş bir bağlantıyı gizlemek, kullanıcıya her şey yolunda
        izlenimi verir; kod 5 saniye gecikince "bozuk" der. Durumu söylemek
        beklentiyi doğru kurar.
      */}
      {transport === 'polling' && (
        <p className="text-xs text-[var(--color-warn)]">
          Canlı bağlantı kurulamadı — durum 5 saniyede bir kontrol ediliyor.
        </p>
      )}
      {transport === 'connecting' && (
        <p className="text-xs text-muted">Canlı bağlantı kuruluyor…</p>
      )}
    </div>
  );
}

/* ─────────────────────────── Kopyalama ─────────────────────────── */

/**
 * Kopyala butonu.
 *
 * `navigator.clipboard` GÜVENLİ OLMAYAN BAĞLAMDA (http://) ve bazı iOS
 * sürümlerinde YOKTUR. Yedek olarak eski `execCommand('copy')` yolu kullanılır;
 * o da olmazsa kullanıcıya "elle seçin" denir — sessizce hiçbir şey yapmayan
 * bir buton en kötüsüdür.
 */
function CopyButton({ value, label, big }: { value: string; label: string; big?: boolean }) {
  const [state, setState] = React.useState<'idle' | 'ok' | 'fail'>('idle');

  async function copy() {
    try {
      if (navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(value);
        setState('ok');
      } else {
        setState(legacyCopy(value) ? 'ok' : 'fail');
      }
    } catch {
      setState(legacyCopy(value) ? 'ok' : 'fail');
    }
    window.setTimeout(() => setState('idle'), 2000);
  }

  return (
    <button
      type="button"
      onClick={copy}
      aria-label={label}
      className={cx(
        'grid shrink-0 place-items-center rounded-xl border transition-colors',
        'active:scale-95',
        big ? 'size-14' : 'size-11',
        state === 'ok'
          ? 'border-[var(--color-ok)] text-[var(--color-ok)]'
          : state === 'fail'
            ? 'border-[var(--color-bad)] text-[var(--color-bad)]'
            : 'border-[var(--border)] text-muted',
      )}
    >
      {state === 'ok' ? (
        <svg viewBox="0 0 24 24" className="size-5" fill="none" stroke="currentColor"
             strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
          <path d="m5 13 4 4L19 7" />
        </svg>
      ) : (
        <svg viewBox="0 0 24 24" className="size-5" fill="none" stroke="currentColor"
             strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
          <rect x="9" y="9" width="12" height="12" rx="2.4" />
          <path d="M5 15V5a2 2 0 0 1 2-2h10" />
        </svg>
      )}
      <span className="sr-only" aria-live="polite">
        {state === 'ok' ? 'Kopyalandı' : state === 'fail' ? 'Kopyalanamadı, elle seçin' : ''}
      </span>
    </button>
  );
}

function legacyCopy(value: string): boolean {
  try {
    const ta = document.createElement('textarea');
    ta.value = value;
    // Ekran dışına almak yerine görünmez yapmak iOS'ta seçimi bozar;
    // sabit konumlandırıp opaklığı sıfırlarız.
    ta.style.cssText = 'position:fixed;top:0;left:0;opacity:0;pointer-events:none';
    ta.setAttribute('readonly', '');
    document.body.appendChild(ta);
    ta.select();
    ta.setSelectionRange(0, value.length);
    const ok = document.execCommand('copy');
    document.body.removeChild(ta);
    return ok;
  } catch {
    return false;
  }
}

/* ─────────────────────────── Ekran ve bildirim ─────────────────────────── */

/**
 * Ekranı uyanık tut.
 *
 * Kullanıcı kod beklerken telefon kilitlenirse SMS geldiğinde ekranı açıp
 * uygulamaya dönmesi gerekir — ve bfcache'ten dönen sayfa çoğu tarayıcıda
 * ölü bir bağlantıyla gelir. Uyanık tutmak bu zinciri hiç başlatmaz.
 *
 * Desteklenmiyorsa SESSİZCE geçilir: Safari'de Wake Lock yalnız 16.4+ ve
 * kullanıcı etkileşimi sonrası çalışır.
 */
function useWakeLock(active: boolean) {
  React.useEffect(() => {
    if (!active) return;
    let lock: WakeLockSentinel | null = null;
    let cancelled = false;

    const request = async () => {
      try {
        const wl = (navigator as Navigator & { wakeLock?: WakeLock }).wakeLock;
        if (!wl) return;
        lock = await wl.request('screen');
      } catch {
        // İzin verilmedi veya desteklenmiyor — sorun değil.
      }
    };
    void request();

    // Sekme geri geldiğinde kilit düşmüş olur; yeniden isteriz.
    const onVisible = () => {
      if (document.visibilityState === 'visible' && !cancelled) void request();
    };
    document.addEventListener('visibilitychange', onVisible);

    return () => {
      cancelled = true;
      document.removeEventListener('visibilitychange', onVisible);
      void lock?.release().catch(() => {});
    };
  }, [active]);
}

/**
 * Kod geldiğinde titret.
 *
 * iOS'ta `navigator.vibrate` YOKTUR; kontrol edilmeden çağırmak hata atar.
 * Android'de kullanıcı ekrana bakmıyorsa fark eder.
 */
function useArrivalFeedback(code: string | undefined) {
  const seen = React.useRef<string | undefined>(undefined);
  React.useEffect(() => {
    if (!code || seen.current === code) return;
    seen.current = code;
    try {
      navigator.vibrate?.([120, 60, 120]);
    } catch {
      // Desteklenmiyor — sessizce geç.
    }
  }, [code]);
}
