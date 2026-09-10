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
import { formatDateTime, formatMoney, formatPhone } from '@/lib/format';
import { Button, Card, Empty, Field } from '@/components/ui';
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
  Secim,
  SuzgecCubugu,
  VeriTablosu,
  sayfalamaGorunur,
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
 * Durum süzgeci seçenekleri.
 *
 * 🔴 ETİKETLER `DURUM`DAN OKUNUR, ELLE YAZILMAZ. İki liste ayrı yazılsaydı
 * süzgeçte "İptal" seçip listede "İptal edildi" rozeti görürdünüz — bu panelin
 * ölçülmüş ve kapatılmış hatası tam olarak budur (bkz. `veri-tablosu.tsx`).
 * Sıra `DURUM`un tanım sırasıdır: kullanıcının en sık aradığı "Kod bekleniyor"
 * başta durur.
 */
const DURUM_SUZGECLERI: ReadonlyArray<{ value: string; label: string }> = [
  { value: '', label: 'Tüm durumlar' },
  ...Object.entries(DURUM).map(([kod, d]) => ({ value: kod, label: d.etiket })),
];

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

  // İKİ AYRI DURUM: `aramaGirdisi` kutuya yazılan, `arama` sunucuya giden.
  // Tek durum kullanılsaydı her tuşa basış bir istek olurdu.
  const [aramaGirdisi, setAramaGirdisi] = React.useState('');
  const [arama, setArama] = React.useState('');
  const [durum, setDurum] = React.useState('');

  // 350 ms: `/yonetim/kullanicilar` ile AYNI gecikme. Yazmayı bırakınca arama
  // kendiliğinden yapılır; ayrı bir "Ara" düğmesi yoktur.
  // `setOffset(0)`: süzgeç değişince 3. sayfada kalmak, sonucu olan bir aramada
  // BOŞ ekran gösterir — kullanıcı bunu "sonuç yok" sanar.
  React.useEffect(() => {
    const t = setTimeout(() => {
      setArama(aramaGirdisi.trim());
      setOffset(0);
    }, 350);
    return () => clearTimeout(t);
  }, [aramaGirdisi]);

  const suzgecTemizle = React.useCallback(() => {
    setAramaGirdisi('');
    setArama('');
    setDurum('');
    setOffset(0);
  }, []);

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
    queryKey: ['orders', { limit: SAYFA_BOYUTU, offset, arama, durum }],
    queryFn: () => {
      // 🔴 SÜZGEÇ SUNUCUDA UYGULANIR (`GET /orders?q=&status=`), istemcide
      // DEĞİL. İstemcide süzmek yalnız o sayfadaki 25 satırı arardı: kullanıcı
      // "numaram yok" sonucunu alır, oysa numara 2. sayfadadır. Sayfalı bir
      // listede istemci araması yanlış cevap veren bir aramadır.
      const p = new URLSearchParams({
        limit: String(SAYFA_BOYUTU),
        offset: String(offset),
      });
      if (arama) p.set('q', arama);
      if (durum) p.set('status', durum);
      return apiFetch<OrderList>(`/orders?${p.toString()}`);
    },
    // Sayfa değişince liste boşalıp zıplamasın; eski veri yenisi gelene dek kalır.
    placeholderData: keepPreviousData,
  });

  const toplam = q.data?.total ?? 0;
  const hata = apiHatasi(q.error);
  const sayfali = sayfalamaGorunur(toplam);
  const etkinSuzgec = (arama ? 1 : 0) + (durum ? 1 : 0);

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
        // Gruplu gösterim (`+90 534 794 92 67`): kesintisiz 12 hane taranamaz.
        hucre: (o) => (
          <span className="select-text font-medium">
            {formatPhone(o.phoneNumber, o.phoneCode)}
          </span>
        ),
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
    <div className="flex flex-col gap-5">
      <SayfaBasligi baslik="Siparişlerim" aciklama="Aldığınız numaralar ve gelen kodlar." />

      {/*
        SÜZGEÇ ÇUBUĞU AYRI KARTTA — `/yonetim/kullanicilar` ile aynı yerleşim.
        Liste kartının içine konsaydı, uzun bir listede süzgeç yukarı kayıp
        ekrandan çıkardı ve "arama nerede?" sorusu doğardı.
      */}
      <Card>
        <SuzgecCubugu etkinSayisi={etkinSuzgec} onTemizle={suzgecTemizle}>
          {/*
            `sm:self-start`: `SuzgecCubugu` tabana hizalar (`items-end`); `Field`
            ipucu metnini kutunun ALTINA koyduğu için arama kutusu seçim
            kutusundan ~18px yukarı kayar. Tepeden hizalamak farkı 2px'e indirir.
          */}
          <div className="w-full sm:w-80 sm:self-start">
            <Field
              label="Ara"
              type="search"
              value={aramaGirdisi}
              onChange={(e) => setAramaGirdisi(e.target.value)}
              placeholder="Numara, servis veya ülke"
              // Numara girilen bir kutuda iOS'un ilk harfi büyütmesi ve
              // otomatik düzeltmesi yalnız zarar verir.
              autoCapitalize="none"
              autoCorrect="off"
              autoComplete="off"
              spellCheck={false}
              hint="Yazmayı bıraktığınızda arama kendiliğinden yapılır."
            />
          </div>

          <div className="w-full sm:w-56 sm:self-start">
            <Secim
              etiket="Durum"
              value={durum}
              onChange={(e) => {
                setDurum(e.target.value);
                setOffset(0);
              }}
            >
              {DURUM_SUZGECLERI.map((d) => (
                <option key={d.value || 'tumu'} value={d.value}>
                  {d.label}
                </option>
              ))}
            </Secim>
          </div>
        </SuzgecCubugu>
      </Card>

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
            /*
              §6.3: "SÜZGEÇTEN DOLAYI BOŞ" İLE "GERÇEKTEN BOŞ" AYNI EKRAN
              DEĞİLDİR. Siparişi olan ama aramasıyla eşleşme bulamayan bir
              kullanıcıya "Henüz siparişiniz yok" demek, doğru olmadığı gibi
              yanlış eylemi de önerir: yapması gereken numara almak değil,
              süzgeci temizlemektir. Doğru eylem `SuzgecCubugu`'nun kendi
              "Süzgeçleri temizle" düğmesidir — burada ikinci bir kontrol
              açılmaz, aynı işi yapan iki düğme tutarsızlık üretir (§9.2).
            */
            etkinSuzgec > 0 ? (
              <Empty
                title="Bu aramaya uyan sipariş yok"
                hint="Arama metnini kısaltmayı ya da durum süzgecini “Tüm durumlar” yapmayı deneyin."
              />
            ) : (
              // İpucu ikinci bir düğme eklemez, üstteki gezintiyi adıyla gösterir.
              <Empty
                title="Henüz siparişiniz yok"
                hint="Üstteki “Numara al” bölümünden bir numara aldığınızda siparişleriniz burada listelenir."
              />
            )
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
