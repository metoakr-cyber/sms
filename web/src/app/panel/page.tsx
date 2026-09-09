'use client';

/**
 * /panel — Özet.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * BU EKRAN NE SORUYA CEVAP VERİR
 * ══════════════════════════════════════════════════════════════════════════
 * Müşteri sabah paneli açtığında üç şey sorar:
 *   1. "Param ne kadar?"            → BAŞLIKTA. Bu sayfada TEKRAR EDİLMEZ (aşağıda).
 *   2. "Bekleyen siparişim var mı?" → `Aktif siparişler` bölümü. Zamana duyarlı
 *                                      olan tek şey budur, bu yüzden ÜSTTE durur.
 *   3. "Ne olup bitti?"             → `Son hareketler` bölümü.
 *
 * ── 🔴 BAKİYE NEDEN TEKRAR EDİLMİYOR ──────────────────────────────────────
 * Eski özet, 3xl puntoyla bir "Bakiye" karosu çiziyordu. Yan sütun kalkıp
 * yerine üst başlık geldiğinde bakiye `panel-header.tsx` içindeki
 * `BakiyeRozeti`'ne taşındı: HER sayfada, HER genişlikte, `tabular-nums` ile,
 * cüzdana bağlı bir DEĞER olarak duruyor (ölçüldü: 320 px dâhil). Aynı sayıyı
 * 60 px aşağıda ikinci kez, üstelik daha büyük yazmak
 *   · aynı veriyi iki yerde biçimlemek demektir (§6 etiket ayrışmasının kaynağı),
 *   · ve bir karo ızgarasını sayfa iskeleti yapar — `craft-floor.md:25`,
 *     "cards are the lazy container", §9.2'de yasak.
 * Bakiye bu ekranda BAĞLAM İÇİNDE görünür: `Son hareketler` tablosunun
 * `Bakiye sonrası` sütunu, her hareketten sonraki bakiyeyi verir — yani sayıyı
 * değil, sayıyı DEĞİŞTİREN şeyi gösterir. "Bakiye yükle" eylemi başlıkta durur.
 *
 * ── VERİ: YALNIZ VAR OLAN UÇLAR ───────────────────────────────────────────
 * `router.go`'dan doğrulandı (satır 187-188, 296):
 *   GET /wallet/entries   → son hareketler
 *   GET /orders           → sipariş listesi (limit/offset; SÜZGEÇ ALMAZ)
 * Uydurma sayaç yok: ekranda hiçbir yerde "N aktif sipariş" gibi bir SAYI
 * gösterilmiyor, listenin kendisi gösteriliyor.
 *
 * 🔴 AKTİF SİPARİŞ SÜZGECİ İSTEMCİDEDİR — ve bu güvenlidir:
 * `ListUserOrders` (`api/queries/orders.sql:82`) `ORDER BY created_at DESC`
 * döner ve terminal olmayan bir sipariş tanımı gereği EN YENİLERDENDİR
 * (aktivasyon TTL'i dakikalarla ölçülür). İlk sayfa (20 kayıt) bu yüzden
 * pratikte tüm aktif siparişleri kapsar. Sunucuya durum süzgeci eklenirse
 * doğru düzeltme burada değil, uçta yapılır.
 *
 * ── DAVRANIŞ DEĞİŞİKLİĞİ (bilerek, tek) ───────────────────────────────────
 * `Aktif siparişler` bölümü YENİDİR; eski özette yoktu. Gerekçe: eski ekranın
 * ikinci karosu "Hesap durumu" idi ve taşıdığı iki bilgi de artık kabukta
 * duruyor — e-posta doğrulanmadı uyarısı `panel-shell.tsx`'te bir `Alert`,
 * yönetici işareti başlıktaki "Yönetim" maddesi ve profil menüsü. Yani o karo
 * bugün SAF TEKRARDI. Yerine, kabuğun gösteremediği tek zamana duyarlı bilgi
 * kondu: kodu bekleyen sipariş. Yeni bir uç, yeni bir mutasyon, yeni bir
 * hesaplama yok — var olan `GET /orders` okunuyor.
 *
 * ── SUNUM ─────────────────────────────────────────────────────────────────
 * İki liste de ORTAK `VeriTablosu` ile çizilir: `< md` kart, `≥ md` tablo,
 * yükleniyor/boş/hata üçü de zorunlu prop, `tabular-nums` sütun tanımından
 * gelir. Bu ekranın kendi tablo/kart ikizi YOKTUR.
 * Hareket: yok (§4.2 — özet günde birkaç kez açılır ama satırlar veri satırıdır;
 * `Button`'ın kendi `:active` geri bildirimi dışında bu dosya hiçbir geçiş
 * tanımlamaz).
 */

import * as React from 'react';
import Link from 'next/link';
import { useQuery } from '@tanstack/react-query';
import { apiFetch } from '@/lib/api';
import { formatDateTime, formatMoney } from '@/lib/format';
import { useSession } from '@/hooks/useSession';
import { Button, Card, cx } from '@/components/ui';
import { ServiceIcon } from '@/components/service-icon';
import { DurumRozeti, SayfaBasligi, VeriTablosu, apiHatasi } from '@/components/yonetim';
import type { Sutun } from '@/components/yonetim';
import type { LedgerEntry, Money, Order, OrderList, Statement } from '@/lib/types';

/** Özette gösterilen hareket sayısı. Tümü için cüzdana gidilir. */
const SON_HAREKET = 5;

/**
 * Aktif sipariş taraması.
 *
 * 20 SEÇİLDİ ÇÜNKÜ `/panel/siparisler` DE 20 KULLANIYOR (`siparisler/page.tsx:22`).
 * Aynı `queryKey` şekli → TanStack Query ÖNBELLEĞİ PAYLAŞILIR: özetten
 * siparişlere geçen kullanıcı ikinci bir istek beklemez. Değer orada değişirse
 * paylaşım kaybolur, ama bu bir hata değil yalnız kaçırılmış bir kazançtır.
 */
const SIPARIS_TARAMA = 20;

/**
 * Terminal OLMAYAN sipariş durumları — `api/internal/domain/order/order.go:74-87`.
 *
 * COMPLETED ve REFUNDED terminaldir. CANCELLED ve FAILED birer GEÇİŞTİR ama
 * kullanıcı açısından iş bitmiştir (ikisinin de oku REFUNDED'a çıkar), bu
 * yüzden "aktif" sayılmazlar. ACTIVE ise kiralık siparişin süren dönemidir ve
 * DAHİLDİR: /panel/numara-al kiralama satıyor, süren bir kiralamayı özette
 * gizlemek kullanıcıyı yanıltırdı.
 */
const AKTIF_DURUMLAR = new Set(['PENDING', 'ACTIVE']);

/**
 * Durum kodu → Türkçe etiket.
 *
 * `PENDING` etiketi `/panel/siparisler`'deki `STATUS` haritasıyla BİREBİR
 * aynıdır — aynı veri iki ekranda iki farklı kelimeyle anılmaz (§6).
 * `ACTIVE` orada YOK: o ekran süren bir kiralamayı bugün "Bilinmeyen durum"
 * diye gösteriyor. Burada doğrusu yazıldı; o dosya bu turda başka bir ajana
 * ait, eksik rapora yazıldı.
 */
const DURUM_ETIKET: Record<string, string> = {
  PENDING: 'Kod bekleniyor',
  ACTIVE: 'Kiralama sürüyor',
};

/* ═══════════════════════ Sütun tanımları ═══════════════════════ */

const AKTIF_SUTUNLAR: ReadonlyArray<Sutun<Order>> = [
  {
    anahtar: 'servis',
    baslik: 'Servis',
    mobilRol: 'baslik',
    hucre: (o) => (
      <span className="flex items-center gap-2">
        {/* `iconUrl` GEÇİLİR: sunucu sipariş listesinde canlı katalog logosunu
            taşır (`ListUserOrders` + services LEFT JOIN). Geçilmezse HER satır
            harf rozetine düşer — logo vardır, ekrana gelmez. */}
        <ServiceIcon name={o.serviceName} iconUrl={o.iconUrl} size={32} />
        <span className="min-w-0">
          <span className="block truncate font-medium">{o.serviceName}</span>
          <span className="block truncate text-sm text-muted">{o.countryName}</span>
        </span>
      </span>
    ),
  },
  {
    anahtar: 'numara',
    baslik: 'Numara',
    // Numara bir sayı dizisidir: `tabular-nums` olmadan alt alta gelen satırlar
    // kayar ve kullanıcı doğrulama kutusuna yanlış haneyi kopyalar (§3.5).
    sayisal: true,
    hucre: (o) => <span className="select-text font-medium">{o.phoneNumber}</span>,
  },
  {
    anahtar: 'bitis',
    baslik: 'Son geçerlilik',
    sayisal: true,
    /* 🔴 `oncelik: 3` TERCİH DEĞİL, ÖLÇÜM SONUCU (§6.1 kural 3).
       768 px'te tabloya 670 px kalıyor. Beş sütunun doğal (`max-content`)
       genişliği kısa adlarla 618 px — sığıyor gibi görünüyor. Ama gerçekçi
       uzun bir kayıtla ("Microsoft Teams Business" / "Birleşik Arap
       Emirlikleri") 749 px'e çıkıyor ve SAYFA YATAY KAYIYOR (ölçüldü:
       scrollWidth 779 > 768). `sayisal` sütunlar `whitespace-nowrap` taşır,
       yani sıkışamazlar. Tarih `lg:` üstünde geri gelir; mobil kartta zaten
       her zaman var. */
    oncelik: 3,
    /* 🔴 GERİ SAYIM DEĞİL, SUNUCUNUN `expiresAt`'İ (CLAUDE.md #18).
       Yerel bir sayaç sekme donduğunda durur, gerçek süre durmaz; burada
       mutlak bitiş anı yazılır ve geri sayım sipariş ekranındaki CodeWaiter'a
       bırakılır. */
    hucre: (o) => <span className="text-muted">{formatDateTime(o.expiresAt)}</span>,
  },
  {
    anahtar: 'durum',
    baslik: 'Durum',
    mobilRol: 'rozet',
    // Liste bir istemci süzgecinden geçiyor; durumu GÖSTERMEK süzgeci görünür
    // kılar. Sunucu yarın yeni bir terminal olmayan durum eklerse ekran
    // "hepsi kod bekliyor" diye yalan söylemez.
    hucre: (o) => (
      <DurumRozeti durum={o.status} etiket={DURUM_ETIKET[o.status] ?? 'Bilinmeyen durum'} />
    ),
  },
  {
    anahtar: 'islem',
    baslik: 'İşlem',
    basligiGizle: true,
    hizala: 'sag',
    mobilRol: 'eylem',
    /* `sunum` YALNIZ SUNUM FARKI İÇİN: metin iki yerde de aynı ("Siparişi aç"),
       değişen tek şey mobilde düğmenin tam genişlik olmasıdır. Farklı METİN
       yazmak yasaktır (veri-tablosu.tsx:115). */
    hucre: (_o, sunum) => (
      <Link href="/panel/siparisler" className={sunum === 'kart' ? 'block' : 'inline-block'}>
        <Button variant="outline" size="sm" fullWidth={sunum === 'kart'}>
          Siparişi aç
        </Button>
      </Link>
    ),
  },
];

const HAREKET_SUTUNLARI: ReadonlyArray<Sutun<LedgerEntry>> = [
  {
    anahtar: 'islem',
    baslik: 'İşlem',
    mobilRol: 'baslik',
    // `typeLabel` SUNUCUDAN gelir: tip kodunun Türkçesini istemcide üretmek
    // ikinci bir sözlük demektir.
    hucre: (e) => <span className="font-medium">{e.typeLabel}</span>,
  },
  {
    anahtar: 'tarih',
    baslik: 'Tarih',
    sayisal: true,
    hucre: (e) => <span className="text-muted">{formatDateTime(e.createdAt)}</span>,
  },
  {
    anahtar: 'tutar',
    baslik: 'Tutar',
    hizala: 'sag',
    sayisal: true,
    // Mobil kartta ÜST SATIRIN SAĞI: "ne oldu" ile "ne kadar" yan yana okunur.
    // `<dl>` gövdesine inseydi tutar, tarihin altında üçüncü satır olurdu ve
    // kartın taradığı asıl sayı en sona düşerdi. `/panel/cuzdan` de aynı
    // yerleşimi kullanıyor — iki ekran aynı kaydı aynı biçimde gösterir.
    mobilRol: 'rozet',
    hucre: (e) => <Tutar deger={e.amount} />,
  },
  {
    anahtar: 'bakiye',
    baslik: 'Bakiye sonrası',
    hizala: 'sag',
    sayisal: true,
    /* ÖNCELİK DÜŞÜRÜLMEDİ. §6.1 kural 3 beşten çok sütunu olan tabloları
       gizlemeye zorlar; bu tablo DÖRT sütun ve 768 px'te ölçüldü: `Tarih` ile
       `Tutar` arasında ~300 px boşluk kalıyor. Bakiyeyi orada gizlemek, boş
       yer varken bilgi saklamak olurdu — üstelik "bakiye başlıkta, geçmişi
       burada" dengesi bu sütuna dayanıyor. */
    hucre: (e) => formatMoney(e.balanceAfter),
  },
];

/* ═══════════════════════════ Ekran ═══════════════════════════ */

export default function OzetPage() {
  const { user } = useSession();

  const siparisler = useQuery({
    queryKey: ['orders', { limit: SIPARIS_TARAMA, offset: 0 }],
    queryFn: () => apiFetch<OrderList>(`/orders?limit=${SIPARIS_TARAMA}&offset=0`),
  });

  const hareketler = useQuery({
    queryKey: ['statement', { limit: SON_HAREKET }],
    queryFn: () => apiFetch<Statement>(`/wallet/entries?limit=${SON_HAREKET}&offset=0`),
  });

  // Yüklenirken `undefined` KALIR (boş dizi değil): `VeriTablosu` boş diziyi
  // "sonuç yok" sayar ve iskeletin yerine boş durumu çizerdi.
  const aktif = siparisler.data?.items.filter((o) => AKTIF_DURUMLAR.has(o.status));

  // §6.3: SÜZGEÇTEN DOLAYI boş ile GERÇEKTEN boş farklı metinlerdir.
  // `total` sunucudan gelir; ekranda sayı olarak GÖSTERİLMEZ, yalnız hangi
  // boş durumun doğru olduğunu seçer.
  const hicSiparisYok = (siparisler.data?.total ?? 0) === 0;

  return (
    // `max-w-5xl`: kardeş panel ekranlarıyla (`siparisler`, `cuzdan`) AYNI
    // genişlik. Kabuğun `main`i `max-w-6xl` verir; burada daraltmak sayfalar
    // arası zıplamayı önler (§10 "içerik genişliği panelin geri kalanıyla aynı").
    <div className="mx-auto flex max-w-5xl flex-col gap-6 md:gap-8">
      <SayfaBasligi
        baslik={user ? `Merhaba, ${user.username}` : 'Özet'}
        aciklama="Hesabınızın özeti."
      >
        <Link href="/panel/bakiye-yukle">
          <Button variant="outline" size="sm">Bakiye yükle</Button>
        </Link>
        <Link href="/panel/numara-al">
          <Button size="sm">Numara al</Button>
        </Link>
      </SayfaBasligi>

      <Bolum baslik="Aktif siparişler" href="/panel/siparisler" bagEtiket="Tüm siparişler">
        <VeriTablosu<Order>
          baslik="Kodu bekleyen siparişleriniz"
          sutunlar={AKTIF_SUTUNLAR}
          satirlar={aktif}
          satirAnahtari={(o) => o.id}
          yukleniyor={siparisler.isLoading}
          hata={apiHatasi(siparisler.error)}
          // Bu blok genelde 0-2 satırdır; 5 iskelet satırı veri gelince
          // sayfayı gereksiz yere kısaltır.
          iskeletSatir={2}
          bos={
            hicSiparisYok ? (
              <BosDurum
                baslik="Henüz siparişiniz yok"
                ipucu="Servis ve ülke seçip numara aldığınızda, gelen kodu bu ekrandan takip edersiniz."
                eylem="Numara al"
                href="/panel/numara-al"
              />
            ) : (
              <BosDurum
                baslik="Bekleyen siparişiniz yok"
                ipucu="Kod bekleyen bir siparişiniz olduğunda burada görünür. Geçmiş siparişleriniz Siparişler ekranında durur."
                eylem="Numara al"
                href="/panel/numara-al"
              />
            )
          }
        />
      </Bolum>

      <Bolum baslik="Son hareketler" href="/panel/cuzdan" bagEtiket="Tüm hareketler">
        <VeriTablosu<LedgerEntry>
          baslik="Son para hareketleriniz"
          sutunlar={HAREKET_SUTUNLARI}
          satirlar={hareketler.data?.items}
          satirAnahtari={(e) => e.id}
          yukleniyor={hareketler.isLoading}
          hata={apiHatasi(hareketler.error)}
          iskeletSatir={SON_HAREKET}
          /* 🔴 TEK CANLI DUYURU: iki liste aynı anda yüklenir ve ikisi de
             duyurursa ekran okuyucu bağlamsız iki cümle üst üste okur
             ("2 kayıt listelendi" / "5 kayıt listelendi") — hangisi hangisi
             belli olmaz. `VeriTablosu` bu durum için `duyuru={false}`yi zaten
             öneriyor (veri-tablosu.tsx:155). Duyuru, zamana duyarlı olan
             aktif sipariş listesinde bırakıldı. */
          duyuru={false}
          bos={
            <BosDurum
              baslik="Henüz hareket yok"
              ipucu="Bakiye yüklediğinizde ve numara aldığınızda hareketleriniz burada görünür."
              eylem="Bakiye yükle"
              href="/panel/bakiye-yukle"
            />
          }
        />
      </Bolum>
    </div>
  );
}

/* ═══════════════════════ Yerel parçalar ═══════════════════════ */

/**
 * Bölüm kartı — başlık + "tümü" bağlantısı + içerik.
 *
 * Bu dosyada İKİ kez kullanılıyor; üçüncü bir özet ekranı doğduğunda
 * `components/yonetim/` altına taşınmalıdır. Bugün taşınmadı çünkü bu turda
 * o paket başka ajanların ortak zemini.
 *
 * Bağlantı `min-h-11`: 44 px dokunma hedefi (§7.3). Düz bir `<a>`'nın satır
 * yüksekliği 21 px'tir ve telefonda ıskalanır.
 */
function Bolum({
  baslik,
  href,
  bagEtiket,
  children,
}: {
  baslik: string;
  href: string;
  /** "Tümü" değil, NEYİN tümü: ekran okuyucu bağlantıları listeler. */
  bagEtiket: string;
  children: React.ReactNode;
}) {
  return (
    <Card>
      <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2">
        {/* `text-lg font-semibold`: h2'nin TEK boyutu (§3.3). */}
        <h2 className="text-lg font-semibold">{baslik}</h2>
        <Link
          href={href}
          className="inline-flex min-h-11 items-center text-sm text-brand-400
                     underline-offset-4 hover:underline"
        >
          {bagEtiket}
        </Link>
      </div>
      <div className="mt-4">{children}</div>
    </Card>
  );
}

/**
 * Boş durum + EYLEM.
 *
 * `ui.tsx`'in `Empty`'si bir eylem slotu taşımıyor (§5.2'de "bilinen eksik"
 * olarak yazılı) ve `operate.md:35` boş durumun ÖĞRETMESİNİ istiyor: "burada
 * bir şey yok" demek değil, ne yapılacağını göstermek. Negatif kenar boşluğuyla
 * `Empty`'nin dolgusunu geri almak yerine kompozisyon burada kuruldu.
 * `Empty`'ye eylem slotu eklendiği gün bu parça silinir.
 */
function BosDurum({
  baslik,
  ipucu,
  eylem,
  href,
}: {
  baslik: string;
  ipucu: string;
  eylem: string;
  href: string;
}) {
  return (
    <div className="flex flex-col items-center gap-4 px-4 py-10 text-center">
      <div>
        <p className="font-medium">{baslik}</p>
        {/* `max-w-sm` ≈ 60ch: düz metin satır uzunluğu bandı (§3.4). */}
        <p className="mx-auto mt-2 max-w-sm text-sm text-muted">{ipucu}</p>
      </div>
      <Link href={href}>
        <Button size="sm">{eylem}</Button>
      </Link>
    </div>
  );
}

/**
 * Hareket tutarı.
 *
 * 🔴 ANLAMI İŞARET TAŞIR, RENK YALNIZ PEKİŞTİRİR (§7.1): kırmızı-yeşil ayırt
 * edemeyen kullanıcı `+` ile `-` arasındaki farkı görür. `formatMoney`
 * sunucunun `formatted` alanını döner (eksi işareti oradan gelir); artı işareti
 * yalnız ÖNEK olarak eklenir — istemcide para aritmetiği YOK (CLAUDE.md #1).
 */
function Tutar({ deger }: { deger: Money }) {
  return (
    <span
      className={cx(
        // `tabular-nums` BURADA DA yazılır: sütunun `sayisal` bayrağı tabloyu
        // ve kart `<dd>`'sini kapsar, ama bu hücre mobilde `rozet` slotunda
        // duruyor — orada sarmalayıcı yok, hizalanan tutarlar kayardı (§3.5).
        'font-semibold tabular-nums',
        deger.minor < 0 ? 'text-[var(--color-bad)]' : 'text-[var(--color-ok)]',
      )}
    >
      {deger.minor > 0 ? '+' : ''}
      {formatMoney(deger)}
    </span>
  );
}
