'use client';

/**
 * Siparişlerim — `GET /orders`. Kapanan modaldan sonra siparişe dönmenin tek
 * yolu; sipariş buradan yeniden açılır ve canlı akış kaldığı yerden devam eder.
 *
 * SUNUM KATMANI `components/yonetim`: tablo/kart İKİZİ, yerel `PAGE` sabiti,
 * sayfalama şeridi, hata kutusu ve iskelet BURADAN ÇIKTI (§5.3 · §6.1).
 * Paket adı "yonetim" ama bileşenler geneldir; `/panel` de Operate modudur.
 *
 * 🔴 SSE'YE DOKUNULMADI: canlı kod, geri sayım ve iptal `CodeWaiter` +
 * `useOrderStream` içinde. Kalan süre sunucudaki `expiresAt`'ten gelir
 * (CLAUDE.md #18) — bu dosyada sayaç yok. Davranış aynı: aynı uç nokta, aynı
 * sorgu anahtarı, aynı modal. Tek bilinçli değişiklik sayfa boyutudur.
 */

import * as React from 'react';
import { keepPreviousData, useQuery, useQueryClient } from '@tanstack/react-query';
import { apiFetch } from '@/lib/api';
import { formatDateTime, formatMoney } from '@/lib/format';
import { Button, Card, Empty } from '@/components/ui';
import { Modal } from '@/components/modal';
import { CodeWaiter } from '@/components/code-waiter';
import { ServiceIcon } from '@/components/service-icon';
import {
  apiHatasi,
  DurumRozeti,
  KayitSayaci,
  Sayfalama,
  SAYFA_BOYUTU,
  SayfaBasligi,
  VeriTablosu,
} from '@/components/yonetim';
import type { DurumTonu, Sunum, Sutun } from '@/components/yonetim';
import type { Order, OrderList } from '@/lib/types';

/**
 * `db.OrderStatus` → etiket + ton. Bilinmeyen kod ham hâliyle basılmaz.
 *
 * 🔴 TON AÇIKÇA VERİLİR: katmandaki `durumTonu('CANCELLED')` → `bad`, oysa
 * burada iptal kullanıcının kendi isteğiyle yapılan ve parası İADE EDİLEN
 * normal bir sonuçtur — kırmızı "bir şeyler ters gitti" der. §5.4'ün `neutral`
 * tanımı ("nötr, kapalı, arşiv") budur; `DurumRozeti` ezmeyi bu yüzden kabul
 * ediyor. Yönetimde `bad` doğrudur: orada iptal bir anomalidir.
 */
const DURUM: Record<string, { etiket: string; ton: DurumTonu }> = {
  PENDING: { etiket: 'Kod bekleniyor', ton: 'warn' },
  COMPLETED: { etiket: 'Tamamlandı', ton: 'ok' },
  CANCELLED: { etiket: 'İptal edildi', ton: 'neutral' },
  REFUNDED: { etiket: 'İade edildi', ton: 'neutral' },
  FAILED: { etiket: 'Başarısız', ton: 'bad' },
};

const BILINMEYEN: { etiket: string; ton: DurumTonu } = {
  etiket: 'Bilinmeyen durum',
  ton: 'neutral',
};

/**
 * FAILED DIŞARIDA: `CodeWaiter` yalnız COMPLETED/CANCELLED/REFUNDED'ı "bitmiş"
 * sayar (`code-waiter.tsx:35`); FAILED için hâlâ geri sayım ve "iptal et ve
 * iade al" düğmesi çizer — parası iade edilmiş siparişe ikinci iade sözü.
 */
const ACILABILIR = new Set(['PENDING', 'COMPLETED', 'CANCELLED', 'REFUNDED']);

function eylemMetni(durum: string): string {
  if (durum === 'PENDING') return 'Kodu bekle';
  if (durum === 'COMPLETED') return 'Kodu gör';
  return 'Detay';
}

/**
 * Tablo ve kartta AYNI metin. `sunum` yalnız GENİŞLİK için okunur; metni
 * sunuma göre değiştirmek yasak — ölçülen etiket ayrışmalarının tamamı buydu.
 */
function SatirEylemi({
  siparis,
  sunum,
  onAc,
}: {
  siparis: Order;
  sunum: Sunum;
  onAc: (siparis: Order) => void;
}) {
  if (!ACILABILIR.has(siparis.status)) return null;

  return (
    <Button
      variant={siparis.status === 'PENDING' ? 'primary' : 'outline'}
      size="sm"
      fullWidth={sunum === 'kart'}
      onClick={() => onAc(siparis)}
    >
      {eylemMetni(siparis.status)}
    </Button>
  );
}

export default function OrdersPage() {
  const qc = useQueryClient();
  const [offset, setOffset] = React.useState(0);
  const [acik, setAcik] = React.useState<Order | null>(null);

  // Modal içinde durum değişmiş olabilir: kod geldi, sipariş iptal edildi,
  // süre doldu. Kapanışta tazelemezsek liste hâlâ "Kod bekleniyor" gösterir.
  const detayiKapat = React.useCallback(() => {
    setAcik(null);
    qc.invalidateQueries({ queryKey: ['orders'] });
  }, [qc]);

  // Kararlı olmalı: `sutunlar` memo'sunun bağımlılığı.
  const ac = React.useCallback((siparis: Order) => setAcik(siparis), []);

  // SAYFA BOYUTU: yerel `PAGE = 20` kaldırıldı → ortak `SAYFA_BOYUTU` (25).
  // §6.3 "tek bileşen, tek `limit`": aynı panelde iki sayfa boyutu, "kaç kayıt
  // kaldı" sorusunun ekrandan ekrana farklı cevaplanmasıdır.
  const q = useQuery({
    queryKey: ['orders', { limit: SAYFA_BOYUTU, offset }],
    queryFn: () => apiFetch<OrderList>(`/orders?limit=${SAYFA_BOYUTU}&offset=${offset}`),
    // Sayfa değişince liste boşalıp zıplamasın; eski veri yenisi gelene dek kalır.
    placeholderData: keepPreviousData,
  });

  const toplam = q.data?.total ?? 0;
  const hata = apiHatasi(q.error);
  const sayfali = toplam > SAYFA_BOYUTU;

  /*
   * SÜTUNLAR — tek veri tanımı. Özgün dosyada aynı altı alan İKİ KEZ yazılıydı
   * (mobil kart + tablo); tek `baslik`/`hucre` etiket ayrışmasını imkânsız
   * kılar (§6.1 kural 2). ÖNCELİK (kural 3): altı sütun 768px'e sığmaz, Tarih
   * `oncelik: 3` ile `lg:` üstüne alındı — mobil kartta yine tam metinle var.
   */
  const sutunlar = React.useMemo<ReadonlyArray<Sutun<Order>>>(
    () => [
      {
        anahtar: 'servis',
        baslik: 'Servis',
        mobilRol: 'baslik',
        hucre: (o) => (
          <span className="flex items-center gap-3">
            {/* 🔴 `iconUrl` GEÇİLİR: sunucu canlı katalog logosunu taşıyor
                (`ListUserOrders` + `services` LEFT JOIN). Geçilmediğinde
                `ServiceIcon` HER siparişi harf rozetiyle çiziyordu. */}
            <ServiceIcon name={o.serviceName} iconUrl={o.iconUrl} size={32} />
            <span className="min-w-0">
              <span className="block font-medium">{o.serviceName}</span>
              {/* `text-sm`, `text-xs` DEĞİL: veri taşıyan metin (§3.2). */}
              <span className="block text-sm text-muted">{o.countryName}</span>
            </span>
          </span>
        ),
      },
      {
        anahtar: 'durum',
        baslik: 'Durum',
        mobilRol: 'rozet',
        hucre: (o) => {
          const d = DURUM[o.status] ?? BILINMEYEN;
          return <DurumRozeti durum={o.status} etiket={d.etiket} ton={d.ton} />;
        },
      },
      {
        anahtar: 'numara',
        baslik: 'Numara',
        // Numara bir sayı dizisidir: `tabular-nums` olmadan alt alta gelen
        // numaralar kayar (§3.5). Hizası SOLDA kalır — bu bir kimlik, tutar değil.
        sayisal: true,
        // `select-text`: mobilde uzun basıp kopyalamak en yaygın yol.
        hucre: (o) => <span className="select-text font-medium">{o.phoneNumber}</span>,
      },
      {
        anahtar: 'tutar',
        baslik: 'Tutar',
        hizala: 'sag',
        sayisal: true,
        // Biçimleme yalnız `lib/format`; istemcide para aritmetiği yok (#1).
        hucre: (o) => <span className="font-medium">{formatMoney(o.price)}</span>,
      },
      {
        anahtar: 'tarih',
        baslik: 'Tarih',
        hizala: 'sag',
        sayisal: true,
        oncelik: 3,
        // RFC 3339 + açık `Europe/Istanbul`, `lib/format` içinde (#19).
        hucre: (o) => <span className="text-muted">{formatDateTime(o.createdAt)}</span>,
      },
      {
        anahtar: 'islem',
        baslik: 'İşlem',
        hizala: 'sag',
        // Görünmez ama ekran okuyucuda VAR (özgün `sr-only` başlığın karşılığı).
        basligiGizle: true,
        mobilRol: 'eylem',
        hucre: (o, sunum) => <SatirEylemi siparis={o} sunum={sunum} onAc={ac} />,
      },
    ],
    [ac],
  );

  return (
    <div className="mx-auto flex max-w-5xl flex-col gap-5">
      <SayfaBasligi baslik="Siparişlerim" aciklama="Aldığınız numaralar ve gelen kodlar." />

      <Card>
        <div className="flex flex-wrap items-center justify-between gap-4">
          <h2 className="text-lg font-semibold">Sipariş geçmişi</h2>
          <KayitSayaci toplam={toplam} />
        </div>

        <VeriTablosu
          className="mt-4"
          baslik="Sipariş geçmişi"
          sutunlar={sutunlar}
          satirlar={q.data?.items}
          // `id` zaten `public_id`'dir (#10).
          satirAnahtari={(o) => o.id}
          yukleniyor={q.isLoading}
          // Hata metni + `requestId` tek yerden (`HataDurumu`).
          hata={hata}
          // Sayfalama kendi aralığını duyuruyor; iki canlı bölge aynı anda
          // konuşmasın (`veri-tablosu.tsx` `duyuru` notu).
          duyuru={!sayfali}
          bos={
            // §6.3: burada süzgeç YOK, yani boş liste her zaman "gerçekten
            // boş"tur. İpucu ikinci bir düğme eklemez, üstteki gezintiyi adıyla
            // gösterir — aynı işi yapan iki kontrol tutarsızlık üretir (§9.2).
            <Empty
              title="Henüz siparişiniz yok"
              hint="Üstteki “Numara al” bölümünden bir numara aldığınızda siparişleriniz burada listelenir."
            />
          }
        />

        {/* TEKRAR DENE — `HataDurumu`'nun eylem slotu yok (katman eksiği);
            düğme hatanın altında, özgün davranış birebir korunuyor. */}
        {hata && (
          <div className="mt-4">
            <Button variant="outline" size="sm" onClick={() => q.refetch()} loading={q.isFetching}>
              Tekrar dene
            </Button>
          </div>
        )}

        <Sayfalama
          className="mt-6"
          offset={offset}
          limit={SAYFA_BOYUTU}
          toplam={toplam}
          onDegis={setOffset}
        />
      </Card>

      {/*
        Detay = numara alma ekranındaki KOD BEKLEME bileşeninin ta kendisi.
        Modal §6.4'ün izin verdiği yerde: korunmuş odak + canlı akış.
        `key` akışı sıfırlar. `iconUrl`: `CodeWaiter` logoyu AYRI prop'tan
        alıyor ve bugüne dek hiç verilmiyordu — modaldaki rozet listedekiyle
        uyuşmuyordu; tip vardı, değer eksikti.
      */}
      <Modal open={!!acik} onClose={detayiKapat} title={acik?.serviceName ?? ''}>
        {acik && (
          <CodeWaiter key={acik.id} order={acik} iconUrl={acik.iconUrl} onClose={detayiKapat} />
        )}
      </Modal>
    </div>
  );
}
