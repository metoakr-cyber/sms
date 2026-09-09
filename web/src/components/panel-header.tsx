'use client';

/**
 * PanelHeader — `/panel` üst başlığı, gezintisi ve profil menüsü.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * NE DEĞİŞTİ: YAN SÜTUN → ÜST HEADER  (kullanıcı kararı, 9 Eylül 2026)
 * ══════════════════════════════════════════════════════════════════════════
 * Müşteri panelinden yan sütun KALDIRILDI. Yerine her genişlikte duran bir üst
 * başlık geldi: solda logo, sağda BAKİYE ve PROFİL. Gezinti maddeleri ikonlu
 * ve renkli.
 *
 * 🔴 Bu karar YALNIZ `/panel` içindir. `/yonetim` yan sütunu KALIYOR
 * (ayrı bir kullanıcı kararı — `admin-shell.tsx` + `bolum-gezinti.tsx`).
 * İki yüzey bilerek farklıdır: yönetici günde yüzlerce kez 11 bölüm arasında
 * dolaşır (bölümlenmiş yan sütun), müşteri günde birkaç kez 6 destinasyon
 * arasında dolaşır (üst şerit).
 *
 * ══════════════════════════════════════════════════════════════════════════
 * 🔴 GEZİNTİ DOM'DA TEK KEZ VARDIR — iki kopya değil
 * ══════════════════════════════════════════════════════════════════════════
 * Eski kabukta gezinti İKİ KEZ yazılıydı: masaüstü yan sütunu + mobil alt
 * çubuk. Ölçüldü: her `/panel` sayfasında `[aria-current="page"]` **2 adet**
 * çıkıyordu (biri `display:none` içinde). Aynı liste iki yerde yazıldığında
 * etiketler zamanla ayrışır — `tasarim-sistemi.md §6` bunun ölçülmüş
 * örneklerini sayıyor.
 *
 * Burada TEK bir `<nav>` var ve KABI CSS ile değişiyor:
 *   `< md`  → `fixed inset-x-0 bottom-0`  (başparmak menzilinde alt çubuk)
 *   `≥ md`  → akışta, başlığın ikinci satırı
 *
 * 🔴 Bunun ön koşulu: `fixed` çocuğun hiçbir atasında `transform` / `filter` /
 * `backdrop-filter` OLMAMASI — bu özellikler `position:fixed` için kapsayıcı
 * blok oluşturur ve çubuğu ekranın altına değil BAŞLIĞIN altına yapıştırır.
 * Bu yüzden yapışkan kapsayıcı OPAK ve `backdrop-blur` YOK. Cam efekti burada
 * zaten süstür (`tasarim-sistemi.md §9.2`). `position: sticky` böyle bir blok
 * oluşturmaz, yani kapsayıcının yapışkan olması sorun değildir.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * 🔴 MOBİLDE 5 SEKME — ÖLÇÜLDÜ, TAHMİN DEĞİL
 * ══════════════════════════════════════════════════════════════════════════
 * Eski alt çubuk `repeat(items.length, 1fr)` ızgarasıyla çiziliyordu: madde
 * sayısı arttıkça sekme daralıyordu. 320 px'te ölçüm:
 *   yönetici (7 madde) → sekme 46 px · müşteri (6 madde) → sekme 53 px
 * Dokunma hedefi 44 px'i geçiyordu AMA 11 px'lik etiketler birbirine giriyordu:
 * ekran görüntüsünde "Siparişler" komşusunun üstüne binmiş, "Yönetim"
 * kırpılmıştı. Yani kural sayıyla değil GÖZLE ihlal ediliyordu — ve ızgara
 * dinamik olduğu için her yeni madde durumu bir kademe kötüleştiriyordu.
 *
 * Çözüm: mobil şerit SABİT 5 slot (`grid-cols-5`), kullanıcı yönetici olsa da
 * olmasa da. Madde sayısı artınca daralma İHTİMALİ ortadan kalkar.
 *
 * Şeride girmeyen iki madde KAYBOLMAZ:
 *   · "Hesap"   → profil menüsünde (zaten hesabın menüsü)
 *   · "Yönetim" → profil menüsünde (yalnız yetkili hesapta)
 * Masaüstünde ikisi de başlık şeridindedir.
 */

import * as React from 'react';
import Link from 'next/link';
import { usePathname, useRouter } from 'next/navigation';
import { apiFetch } from '@/lib/api';
import { formatMoney } from '@/lib/format';
import { useClearSession } from '@/hooks/useSession';
import { useTheme } from './theme';
import { Logo } from './logo';
import { Spinner, cx } from './ui';
import type { User } from '@/lib/types';

/* ────────────────────────── İkon yardımcısı ────────────────────────── */

/**
 * Tek yollu SVG ikon. Emoji/unicode DEĞİL (`tasarim-sistemi.md §9.2`):
 * tek kütüphane, tek çizgi kalınlığı (1.8).
 */
const I = (d: string) => (
  <svg viewBox="0 0 24 24" className="size-5 shrink-0" fill="none" stroke="currentColor"
       strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
    <path d={d} />
  </svg>
);

/* ────────────────────────── Renk ────────────────────────── */

/**
 * Gezinti maddelerinin vurgu renkleri.
 *
 * 🔴 YENİ RENK UYDURULMADI. `globals.css`'teki ölçülmüş vurgu ailelerinden
 * (`--v-*-zemin` / `--v-*-metin`) alındı; her çift iki temada da ≥4.5:1
 * (globals.css:125 yorumu). Bu değerler kendi ZEMİNİ üstünde ölçüldü; sayfa
 * zemininde (`--bg`) kontrast daha da yükselir — yani ölçüm güvenli tarafta.
 *
 * 🔴 RENK İKONU TAŞIR, METNİ DEĞİL. Etiket her zaman `--muted` ya da `--text`,
 * yani renk seçimi metin kontrastını hiçbir durumda değiştirmez. Ve renk TEK
 * BAŞINA anlam taşımaz (§7.1): etkin madde ayrıca `aria-current="page"`,
 * `font-semibold` ve ölçülmüş zemin tonunu taşır — üç kanal.
 *
 * Sınıf dizgileri STATİK yazılır. Tailwind kaynağı metin olarak tarar;
 * `` `md:${degisken}` `` gibi çalışma anında kurulan bir sınıf üretilen CSS'te
 * HİÇ OLUŞMAZ ve sessizce renksiz kalır.
 */
type Renk = 'mavi' | 'mor' | 'yesil' | 'turuncu' | 'pembe' | 'deniz';

const IKON_RENK: Record<Renk, string> = {
  mavi:    'text-[var(--v-mavi-metin)]',
  mor:     'text-[var(--v-mor-metin)]',
  yesil:   'text-[var(--v-yesil-metin)]',
  turuncu: 'text-[var(--v-turuncu-metin)]',
  pembe:   'text-[var(--v-pembe-metin)]',
  deniz:   'text-[var(--v-deniz-metin)]',
};

/**
 * İkon karosunun zemini — YALNIZ `≥ lg` (yan sütun).
 * Sınıflar STATİK yazılır; `lg:${degisken}` gibi çalışma anında kurulan bir
 * sınıf Tailwind çıktısında HİÇ oluşmaz ve karo sessizce renksiz kalır.
 */
const KARO_ZEMIN: Record<Renk, string> = {
  mavi:    'lg:bg-[var(--v-mavi-zemin)]',
  mor:     'lg:bg-[var(--v-mor-zemin)]',
  yesil:   'lg:bg-[var(--v-yesil-zemin)]',
  turuncu: 'lg:bg-[var(--v-turuncu-zemin)]',
  pembe:   'lg:bg-[var(--v-pembe-zemin)]',
  deniz:   'lg:bg-[var(--v-deniz-zemin)]',
};

/** Etkin maddenin SOL VURGU ÇUBUĞU — ikon rengiyle aynı aile. */
const VURGU_CUBUK: Record<Renk, string> = {
  mavi:    'lg:before:bg-[var(--v-mavi-metin)]',
  mor:     'lg:before:bg-[var(--v-mor-metin)]',
  yesil:   'lg:before:bg-[var(--v-yesil-metin)]',
  turuncu: 'lg:before:bg-[var(--v-turuncu-metin)]',
  pembe:   'lg:before:bg-[var(--v-pembe-metin)]',
  deniz:   'lg:before:bg-[var(--v-deniz-metin)]',
};

/** Etkin maddenin zemini — ikon rengiyle ÖLÇÜLMÜŞ çift. */
const ETKIN_ZEMIN: Record<Renk, string> = {
  mavi:    'bg-[var(--v-mavi-zemin)]',
  mor:     'bg-[var(--v-mor-zemin)]',
  yesil:   'bg-[var(--v-yesil-zemin)]',
  turuncu: 'bg-[var(--v-turuncu-zemin)]',
  pembe:   'bg-[var(--v-pembe-zemin)]',
  deniz:   'bg-[var(--v-deniz-zemin)]',
};

/* ────────────────────────── Gezinti verisi ────────────────────────── */

interface Madde {
  href: string;
  etiket: string;
  ikon: React.ReactNode;
  renk: Renk;
  /** Mobil alt şeritte görünür mü? Sabit 5 slotun gerekçesi dosya başında. */
  mobilde: boolean;
  /** Bu maddeyi etkin sayan EK yollar (alt akışlar). */
  ekYollar?: readonly string[];
  /**
   * Şeritte yalnız `lg` üstünde görünür.
   * 1024 px'te 7 madde + logo + bakiye + profil sığmıyor ve gezinti alt
   * satıra sarıyor (ölçüldü). Bu maddeler profil menüsünde HER genişlikte
   * durduğu için şeritten çıkmaları erişimi kaybettirmez.
   */
  genisSerit?: boolean;
}

const HESAP: Madde = {
  href: '/panel/hesap', etiket: 'Hesap', renk: 'pembe', mobilde: false,
  ikon: I('M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2M12 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8'),
};

/**
 * Yönetim maddesi — YALNIZ yetkili hesapta.
 * `mavi`yi "Özet" ile paylaşır: altı vurgu ailesi var, yedinci madde için yeni
 * bir renk UYDURULMAZ. İkisi şeridin iki ucunda durur ve renk burada zaten tek
 * başına anlam taşımıyor (üç kanal kuralı, yukarıda).
 */
const YONETIM: Madde = {
  href: '/yonetim', etiket: 'Yönetim', renk: 'mavi', mobilde: false, genisSerit: true,
  ikon: I('M12 2l8 4v6c0 5-3.4 9.3-8 10-4.6-.7-8-5-8-10V6z'),
};

export const GEZINTI: readonly Madde[] = [
  { href: '/panel', etiket: 'Özet', renk: 'mavi', mobilde: true,
    ikon: I('M4 13h6V4H4zM14 20h6v-9h-6zM4 20h6v-4H4zM14 8h6V4h-6z') },
  { href: '/panel/numara-al', etiket: 'Numara al', renk: 'yesil', mobilde: true,
    ikon: I('M12 5v14M5 12h14') },
  { href: '/panel/siparisler', etiket: 'Siparişler', renk: 'mor', mobilde: true,
    ikon: I('M9 5H7a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V7a2 2 0 0 0-2-2h-2M9 5a2 2 0 0 0 2 2h2a2 2 0 0 0 2-2M9 5a2 2 0 0 1 2-2h2a2 2 0 0 1 2 2') },
  /* `bakiye-yukle` ayrı bir gezinti maddesi DEĞİL, cüzdanın alt akışıdır.
     Eşleştirilmezse o sayfada hiçbir sekme etkin görünmez ve kullanıcı
     "neredeyim?" sorusunu gezintiden cevaplayamaz. */
  { href: '/panel/cuzdan', etiket: 'Cüzdan', renk: 'turuncu', mobilde: true,
    ekYollar: ['/panel/bakiye-yukle'],
    ikon: I('M3 8h18v10a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2zM3 8V6a2 2 0 0 1 2-2h11M17 13h.01') },
  { href: '/panel/destek', etiket: 'Destek', renk: 'deniz', mobilde: true,
    ikon: I('M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z') },
  HESAP,
];

/**
 * İKİNCİL bağlantılar — gezinti şeridine girmez.
 *
 * 🔴 KAYBOLMALARI KABUL EDİLEMEZ: ikisi hukuki metindir. Bu yüzden İKİ yerden
 * ulaşılır:
 *   1) profil menüsü (her genişlikte)
 *   2) sayfa altındaki yardımcı şerit (her genişlikte, düz `<a>`)
 * Profil menüsü bir düğmeye bağlıdır; menü açılmazsa alttaki şerit hâlâ orada.
 */
export const IKINCIL = [
  { href: '/panel/yorumlarim', etiket: 'Yorumlarım',
    ikon: I('M12 3l2.7 5.5 6 .9-4.3 4.2 1 6-5.4-2.8-5.4 2.8 1-6L5.3 9.4l6-.9z') },
  { href: '/kullanim-sartlari', etiket: 'Kullanım Şartları',
    ikon: I('M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8zM14 2v6h6M8 13h8M8 17h5') },
  { href: '/gizlilik', etiket: 'Gizlilik',
    ikon: I('M5 11h14v10H5zM8 11V7a4 4 0 1 1 8 0v4') },
] as const;

/**
 * Etkinlik: `/panel` YALNIZ tam eşleşmede etkindir — `startsWith` kullanılsaydı
 * "Özet" her sayfada etkin görünürdü. Alt yollar `href + '/'` ile eşleşir,
 * yani `/panel/cuzdan-x` gibi bir yol "Cüzdan"ı yanlışlıkla etkinleştirmez.
 */
function etkinMi(m: Madde, yol: string): boolean {
  if (m.href === '/panel') return yol === '/panel';
  if (yol === m.href || yol.startsWith(`${m.href}/`)) return true;
  return (m.ekYollar ?? []).some((p) => yol === p || yol.startsWith(`${p}/`));
}

/* ────────────────────────── Başlık ────────────────────────── */

/**
 * PanelGezinti — SOL YAN SÜTUN (`≥ lg`) ve MOBİL ALT ÇUBUK (`< lg`).
 *
 * 🔴 TEK `<nav>`, TEK `maddeler.map`. Gezinti DOM'da bir kez vardır.
 * Eski kabukta iki kopya yazılıydı ve her sayfada `[aria-current]` İKİ ADET
 * çıkıyordu (biri `display:none` içinde); iki yerde yazılan liste zamanla
 * ayrışır. Kap CSS ile değişir, içerik değişmez:
 *   `< lg` → `fixed inset-x-0 bottom-0`  (başparmak menzilinde alt çubuk)
 *   `≥ lg` → sol yan sütunda dikey liste
 *
 * 🔴 `fixed`in çalışması için hiçbir atada `transform`/`filter`/
 * `backdrop-filter` OLMAMALI — bunlar `position:fixed` için kapsayıcı blok
 * üretir ve çubuğu ekranın altına değil o atanın altına yapıştırır.
 */
type Satir =
  | { tur: 'baslik'; baslik: string }
  | { tur: 'madde'; href: string };

/**
 * Yan sütunun DÜZ satır listesi: başlıklar ve maddeler tek dizide.
 *
 * Tek dizi olması bilinçli — gruplar iç içe yapı olsaydı mobilde onları
 * düzleştirmek için `display:contents` gerekirdi ve o, `ul`/`li` üzerinde
 * liste semantiğini düşüren bilinen bir tuzak.
 *
 * Gruplama maddeleri ANLAMA göre ayırır: kullanıcı "numara alacağım" diye
 * düşünür, "hangi sayfa?" diye değil. Yönetim ayrı durur çünkü ayrı bir
 * yetki dünyasıdır ve yalnız yetkili hesapta görünür.
 */
const SATIRLAR: readonly Satir[] = [
  { tur: 'baslik', baslik: 'İşlemler' },
  { tur: 'madde', href: '/panel' },
  { tur: 'madde', href: '/panel/numara-al' },
  { tur: 'madde', href: '/panel/siparisler' },
  { tur: 'baslik', baslik: 'Hesabım' },
  { tur: 'madde', href: '/panel/cuzdan' },
  { tur: 'madde', href: '/panel/destek' },
  { tur: 'madde', href: '/panel/hesap' },
  { tur: 'baslik', baslik: 'Yönetim' },
  { tur: 'madde', href: '/yonetim' },
];

/**
 * PanelGezinti — SOL YAN SÜTUN (`≥ lg`) ve MOBİL ALT ÇUBUK (`< lg`).
 *
 * 🔴 TEK `<nav>`, TEK madde listesi. Gezinti DOM'da bir kez vardır: eski
 * kabukta iki kopya yazılıydı ve her sayfada `[aria-current]` İKİ ADET
 * çıkıyordu (biri `display:none` içinde). İki yerde yazılan liste zamanla
 * ayrışır. Kap CSS ile değişir, içerik değişmez:
 *   `< lg` → `fixed inset-x-0 bottom-0`  (başparmak menzilinde alt çubuk)
 *   `≥ lg` → sol yan sütunda, bölümlere ayrılmış dikey liste
 *
 * 🔴 `fixed`in çalışması için hiçbir atada `transform`/`filter`/
 * `backdrop-filter` OLMAMALI — bunlar `position:fixed` için kapsayıcı blok
 * üretir ve çubuğu ekranın altına değil o atanın altına yapıştırır.
 */
export function PanelGezinti({ user }: { user: User }) {
  const yol = usePathname();
  const yonetici = user.permissions.length > 0;
  const maddeler = yonetici ? [...GEZINTI, YONETIM] : GEZINTI;
  const bul = (h: string) => maddeler.find((m) => m.href === h);

  return (
    <nav
      aria-label="Panel gezinmesi"
      className="fixed inset-x-0 bottom-0 z-30 border-t border-[var(--border)]
                 bg-[var(--bg)] lg:static lg:z-auto lg:border-t-0 lg:bg-transparent"
    >
      {/*
        🔴 TEK <ul>, TEK map. Bölüm başlıkları listenin İÇİNDE, sunum amaçlı
        <li>'ler olarak durur ve `< lg`'de gizlenir.

        Neden ayrı bir mobil listesi YAZILMADI: iki liste = iki `aria-current`
        (biri `display:none` içinde) ve zamanla ayrışan etiketler. Bu kabukta
        ölçülmüş hata tam olarak budur ve bu turda İKİ KEZ tekrar edildi.

        Neden `display:contents` ile gruplar sarmalanmadı: `contents`, `ul`/`li`
        üzerinde liste semantiğini düşüren bilinen bir erişilebilirlik tuzağı.
        Sunum amaçlı `<li>`, aynı düzeni semantiği bozmadan verir.

        MOBİL: nav `grid grid-cols-5`, başlıklar gizli, yalnız `mobilde`
        maddeleri görünür → tam 5 slot.
        MASAÜSTÜ: `flex flex-col`, başlıklar görünür, tüm maddeler dikey.
      */}
      <ul className="grid grid-cols-5 gap-1 px-2 pt-1
                     lg:flex lg:flex-col lg:gap-0.5 lg:px-0 lg:pt-0">
        {SATIRLAR.map((satir) => {
          if (satir.tur === 'baslik') {
            return (
              <li key={`b-${satir.baslik}`} role="presentation"
                  className="hidden lg:block lg:px-3 lg:pb-1.5 lg:pt-4 lg:first:pt-0">
                <span className="text-[11px] font-semibold uppercase tracking-wider text-muted">
                  {satir.baslik}
                </span>
              </li>
            );
          }
          const m = bul(satir.href);
          if (!m) return null;
          return (
            <li key={m.href} className={cx(!m.mobilde && 'hidden lg:block')}>
              <GezintiBaglantisi madde={m} etkin={etkinMi(m, yol)} />
            </li>
          );
        })}
      </ul>

      {/*
        Güvenli alan dolgusu AYRI öğede: `pb-safe` ile `lg:pb-0`'ı aynı öğeye
        yazmak riskli — ikisi de `@layer utilities` içinde ve çakışmayı kaynak
        sırası çözer, sınıf sırası değil.
      */}
      <div className="pb-safe lg:hidden" aria-hidden />
    </nav>
  );
}

/**
 * PanelHeader — üst şerit. YALNIZ bakiye ve profil (kullanıcı kararı).
 *
 * Gezinti burada DEĞİL: sol yan sütunda (`PanelGezinti`). Şerit `< lg`'de
 * logoyu da taşır çünkü orada yan sütun yoktur; `≥ lg`'de logo yan sütunun
 * tepesindedir ve burada tekrar edilmez — aynı logoyu iki kez çizmek, ekran
 * okuyucuda iki "Ana sayfa" bağlantısı demektir.
 */
export function PanelHeader({ user }: { user: User }) {
  const yonetici = user.permissions.length > 0;

  return (
    <header className="sticky top-0 z-20 border-b border-[var(--border)] bg-[var(--bg)]">
      <div className="flex items-center gap-2 px-4 py-2 md:px-6">
        <Link href="/" aria-label="Ana sayfa"
              className="inline-flex min-h-11 shrink-0 items-center lg:hidden">
          <Logo className="h-7" />
        </Link>

        <div className="ml-auto flex shrink-0 items-center gap-2">
          <BakiyeRozeti user={user} />
          <ProfilMenusu user={user} yonetici={yonetici} />
        </div>
      </div>
    </header>
  );
}

/**
 * Gezinti bağlantısı — TEK bileşen, iki sunum.
 * Taban stiller MOBİL (dikey, 56px yükseklik); `md:` yatay şeride çevirir.
 * `max-*` ile geri alma zinciri YOK.
 */
function GezintiBaglantisi({ madde, etkin }: { madde: Madde; etkin: boolean }) {
  return (
    <Link
      href={madde.href}
      aria-current={etkin ? 'page' : undefined}
      className={cx(
        // Mobilde YATAY DOLGU YOK: 320 px'te slot 58 px ve `px-1` (8 px) çıkınca
        // "Numara al" etiketi 50 px'e sığmayıp "Numar…" diye kırpılıyordu
        // (ekran görüntüsüyle doğrulandı). Dolgusuz slotta tam sığar. Dokunma
        // hedefi bundan etkilenmez: genişliği ızgara slotu verir (58 ≥ 44).
        'flex min-h-14 flex-col items-center justify-center gap-1 rounded-xl text-[11px] leading-tight',
        // `≥ lg`: SOL YAN SÜTUN satırı — tam genişlik, sola yaslı, yatay.
        // Dikey listede madde genişliği sütundan gelir; etiket kırpılmaz ve
        // tıklama alanı satırın tamamıdır (dar bir metin kutusu değil).
        'lg:min-h-11 lg:w-full lg:flex-row lg:justify-start lg:gap-3 lg:px-3 lg:text-sm',
        // Gezinti geçişi günde 100+ kez yaşanır → HAREKET YOK (§4.2).
        // Tek geçiş RENKtir ve fark edilmeyecek kadar hızlıdır.
        '[transition-property:color,background-color]',
        '[transition-duration:var(--sure-hizli)]',
        // Etkin madde ÜÇ kanal taşır (renk tek başına anlam taşımaz, §7.1):
        // `aria-current="page"` · kalın yazı · açık zemin hap.
        // `≥ lg`'de ayrıca SOL VURGU ÇUBUĞU — referans düzenin işareti; dikey
        // listede gözün satırı yakalamasını hızlandırır.
        etkin
          ? cx('font-semibold text-[var(--text)]', ETKIN_ZEMIN[madde.renk],
               'lg:relative lg:before:absolute lg:before:left-0 lg:before:top-1.5',
               'lg:before:bottom-1.5 lg:before:w-1 lg:before:rounded-r-full',
               VURGU_CUBUK[madde.renk])
          : 'text-muted hover:bg-[var(--raised)] hover:text-[var(--text)]',
      )}
    >
      {/*
        İKON KAROSU (`≥ lg`): yumuşak renkli zemin + üstünde renkli ikon.
        Mobil alt çubukta karo YOK — 72 px'lik bir slotta 32 px'lik bir karo
        etiketi ezerdi; orada ikon çıplak durur.

        🔴 Zemin/metin çifti `--v-*-zemin` / `--v-*-metin`; ikisi birlikte
        ölçülmüş (globals.css). Karo, ikonu kendi ölçüldüğü zeminin üstüne
        koyar — yani kontrast varsayım değil.
      */}
      <span
        className={cx(
          'flex shrink-0 items-center justify-center',
          'lg:size-9 lg:rounded-lg',
          IKON_RENK[madde.renk],
          KARO_ZEMIN[madde.renk],
        )}
      >
        {madde.ikon}
      </span>
      <span className="max-w-full truncate">{madde.etiket}</span>
    </Link>
  );
}

/* ────────────────────────── Bakiye ────────────────────────── */

/**
 * Bakiye rozeti — kullanıcının en çok baktığı değer; bu yüzden her genişlikte
 * başlıkta ve bir DEĞER olarak gösterilir.
 *
 * `tabular-nums`: bakiye sayfadan sayfaya değişir; orantılı rakamlarda genişlik
 * oynar ve rozet titrer (§3.5 — bir para sisteminde en görünür craft hatası).
 * Biçimleme `lib/format` üzerinden; istemcide para aritmetiği YOK.
 *
 * Erişilebilir ad "Bakiye: 1.234,56 ₺" olur ve GÖRÜNEN metni içerir
 * (WCAG 2.5.3 Label in Name) — bu yüzden `aria-label` değil `sr-only` metin.
 */
function BakiyeRozeti({ user }: { user: User }) {
  return (
    <Link
      href="/panel/cuzdan"
      className="raised flex min-h-11 shrink-0 items-center gap-2 rounded-xl border px-2
                 [transition-property:color,background-color,border-color]
                 [transition-duration:var(--sure-hizli)]
                 hover:border-[var(--v-turuncu-metin)] md:px-3"
    >
      {/*
        🔴 CÜZDAN İKONU 320 px'te YOK. Ölçüldü: başlık satırı 320 px'te
        32 (kenar) + 116 (logo) + 8 (boşluk) + 116 (ikonlu rozet) + 70 (profil)
        = 342 çıkıyordu, yani 22 px YATAY TAŞMA. İkon + boşluğu (28 px) ve
        profil okunu (24 px) mobilde kaldırınca toplam 280 px'e düşer ve
        bakiye altı haneli olana kadar pay kalır.
        İkon `md:` üstünde geri gelir; orada yer bol.
      */}
      <span className="hidden text-[var(--v-turuncu-metin)] md:inline-flex">
        {I('M3 8h18v10a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2zM3 8V6a2 2 0 0 1 2-2h11M17 13h.01')}
      </span>
      <span className="sr-only">Bakiye: </span>
      <span className="text-sm font-semibold tabular-nums">{formatMoney(user.balance)}</span>
    </Link>
  );
}

/* ────────────────────────── Profil menüsü ────────────────────────── */

/** Kullanıcı adından baş harf(ler) — avatarın tek içeriği. */
function basHarfler(ad: string): string {
  const k = ad.trim().split(/[\s._-]+/).filter(Boolean);
  const [a, b] = k;
  if (a && b) return `${a[0] ?? ''}${b[0] ?? ''}`.toLocaleUpperCase('tr');
  return ad.trim().slice(0, 2).toLocaleUpperCase('tr');
}

/** Menü satırı — 44px dokunma hedefi, 14px metin (§3.2: veri 14px altına inmez). */
const SATIR =
  'flex min-h-11 w-full items-center gap-3 rounded-xl px-3 text-left text-sm ' +
  '[transition-property:color,background-color] [transition-duration:var(--sure-hizli)] ' +
  'hover:bg-[var(--raised)]';

/**
 * Profil menüsü.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * 🔴 ÇIKIŞ NEDEN MENÜNÜN İÇİNDE
 * ══════════════════════════════════════════════════════════════════════════
 * Kullanıcı "çıkış düğmesi header'da yanlışlıkla tıklanabilir" uyarısını duydu
 * ve devam kararı verdi. Risk yine de AZALTILDI: çıkış başlıkta tek tıkla
 * erişilebilir DEĞİL — menünün içinde, ayırıcının altında, en sonda. Yanlışlıkla
 * çıkmak iki kasıtlı hareket gerektirir.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * KLAVYE
 * ══════════════════════════════════════════════════════════════════════════
 * Açılışta odak ilk maddeye gider. `Escape` kapatır ve odağı DÜĞMEYE geri
 * verir — odak asla `document.body`'ye düşmez, yoksa klavye kullanıcısı
 * sayfanın başına atılır. `Tab`/`Shift+Tab` menü içinde döner (odak tuzağı),
 * oklar maddeler arasında gezer, `Home`/`End` uçlara gider.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * HAREKET
 * ══════════════════════════════════════════════════════════════════════════
 * Menü "ara sıra" kademesindedir (Emil'in sıklık tablosu) → standart animasyon
 * MEŞRU. `--sure-menu` (200ms) · `--ease-out` · YALNIZ `opacity` + `transform`.
 * `transform-origin` tetikleyiciye bağlı (sağ üst), merkez değil (§9.3).
 * Kapanış ANİDİR: çıkış animasyonu odağı geri verme anını geciktirir ve hiçbir
 * şey anlatmaz.
 */
function ProfilMenusu({ user, yonetici }: { user: User; yonetici: boolean }) {
  const [acik, setAcik] = React.useState(false);
  const [belirdi, setBelirdi] = React.useState(false);
  const [cikisBusy, setCikisBusy] = React.useState(false);
  const yol = usePathname();
  const router = useRouter();
  const temizle = useClearSession();
  const { theme, setTheme } = useTheme();

  const dugmeRef = React.useRef<HTMLButtonElement>(null);
  const katmanRef = React.useRef<HTMLDivElement>(null);
  const menuId = React.useId();

  // Rota değişince kapanır: bir maddeye dokunan kullanıcı, açtığı sayfanın
  // üstünde duran menüyü elle kapatmak zorunda kalmasın.
  React.useEffect(() => { setAcik(false); }, [yol]);

  // Açılış animasyonu: önce DOM'a gir (opacity-0), SONRAKİ karede sınıfı
  // değiştir. `@keyframes` değil `transition` (§9.3) — kesintiye uğrayabilsin.
  React.useEffect(() => {
    if (!acik) { setBelirdi(false); return; }
    const r = requestAnimationFrame(() => setBelirdi(true));
    return () => cancelAnimationFrame(r);
  }, [acik]);

  // Açılınca ilk maddeye odak.
  React.useEffect(() => {
    if (!acik) return;
    katmanRef.current?.querySelector<HTMLElement>('[role="menuitem"]')?.focus();
  }, [acik]);

  // Dışarı tıklama. `pointerdown` — `click` beklenirse menü, tıklanan öğenin
  // üstündeyken bir kare daha durur ve yanlış hedefe basılabilir.
  React.useEffect(() => {
    if (!acik) return;
    const h = (e: PointerEvent) => {
      const t = e.target as Node;
      if (katmanRef.current?.contains(t) || dugmeRef.current?.contains(t)) return;
      setAcik(false);
    };
    document.addEventListener('pointerdown', h);
    return () => document.removeEventListener('pointerdown', h);
  }, [acik]);

  const kapatVeOdakla = React.useCallback(() => {
    setAcik(false);
    dugmeRef.current?.focus();
  }, []);

  function katmanTus(e: React.KeyboardEvent<HTMLDivElement>) {
    if (e.key === 'Escape') { e.preventDefault(); kapatVeOdakla(); return; }

    const ogeler = Array.from(
      katmanRef.current?.querySelectorAll<HTMLElement>('[role="menuitem"]') ?? [],
    ).filter((el) => !el.hasAttribute('disabled'));
    if (ogeler.length === 0) return;
    const i = ogeler.indexOf(document.activeElement as HTMLElement);

    if (e.key === 'ArrowDown') { e.preventDefault(); ogeler[(i + 1) % ogeler.length]?.focus(); return; }
    if (e.key === 'ArrowUp') { e.preventDefault(); ogeler[(i - 1 + ogeler.length) % ogeler.length]?.focus(); return; }
    if (e.key === 'Home') { e.preventDefault(); ogeler[0]?.focus(); return; }
    if (e.key === 'End') { e.preventDefault(); ogeler[ogeler.length - 1]?.focus(); return; }
    if (e.key === 'Tab') {
      // Odak tuzağı: Tab menüden ÇIKMAZ, başa/sona sarar.
      e.preventDefault();
      const sonraki = e.shiftKey
        ? ogeler[(i - 1 + ogeler.length) % ogeler.length]
        : ogeler[(i + 1) % ogeler.length];
      sonraki?.focus();
    }
  }

  async function cikis() {
    setCikisBusy(true);
    try { await apiFetch('/auth/logout', { method: 'POST' }); }
    catch { /* çerez zaten geçersizse de arayüzü çıkışa götürürüz */ }
    finally { temizle(); router.replace('/giris'); }
  }

  const koyuyaGec = theme !== 'dark';

  return (
    <div className="relative shrink-0">
      <button
        ref={dugmeRef}
        type="button"
        onClick={() => setAcik((v) => !v)}
        onKeyDown={(e) => { if (e.key === 'Escape' && acik) { e.preventDefault(); kapatVeOdakla(); } }}
        aria-haspopup="menu"
        aria-expanded={acik}
        aria-controls={acik ? menuId : undefined}
        className={cx(
          'flex min-h-11 items-center gap-2 rounded-xl border px-1.5',
          '[transition-property:color,background-color,border-color]',
          '[transition-duration:var(--sure-hizli)]',
          acik ? 'raised' : 'border-transparent hover:bg-[var(--raised)]',
        )}
      >
        <span className="sr-only">Hesap menüsü — </span>
        <span aria-hidden
              className="grid size-8 shrink-0 place-items-center rounded-full
                         bg-brand-500/15 text-xs font-bold text-[var(--text)]">
          {basHarfler(user.username)}
        </span>
        {/* Kullanıcı adı `lg` altında GİZLİ: adın kendisi 128 px yer kaplıyor ve
            o genişlikte gezintiyi alt satıra itiyordu. Kimlik avatardaki baş
            harflerle ve menünün içindeki tam adla korunur — kaybolmuyor. */}
        <span className="hidden max-w-32 truncate text-sm font-medium xl:block">
          {user.username}
        </span>
        {/* Ok `md:` üstünde. Mobilde düğme yalnız avatardır: 32 + 12 dolgu =
            44 px, yani dokunma hedefi tam sınırda karşılanır ve satırdan
            24 px kazanılır (gerekçe: BakiyeRozeti içindeki ölçüm). */}
        <svg viewBox="0 0 24 24" className="hidden size-4 shrink-0 text-muted md:block" fill="none"
             stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"
             aria-hidden>
          <path d="m6 9 6 6 6-6" />
        </svg>
      </button>

      {acik && (
        <div
          ref={katmanRef}
          onKeyDown={katmanTus}
          className={cx(
            'surface absolute right-0 top-[calc(100%+0.5rem)] z-20 w-64 origin-top-right',
            'thin-scroll max-h-[26rem] overflow-y-auto rounded-2xl border p-1.5 golge-2',
            '[transition-property:opacity,transform]',
            '[transition-duration:var(--sure-menu)]',
            '[transition-timing-function:var(--ease-out)]',
            belirdi ? 'scale-100 opacity-100' : 'scale-[0.97] opacity-0',
          )}
        >
          {/* Kimlik bloğu `role="menu"` AĞACININ DIŞINDA: bir menünün doğrudan
              çocukları yalnız menuitem/group/separator olabilir. Burada durunca
              ekran okuyucu "3 öğeden 1'i" gibi yanlış bir sayım duyurmaz. */}
          <div className="px-3 py-2">
            <p className="truncate text-sm font-semibold">{user.username}</p>
            <p className="mt-0.5 truncate text-xs text-muted">{user.email}</p>
          </div>
          <hr className="my-1 border-[var(--border)]" />

          <div id={menuId} role="menu" aria-label="Hesap menüsü">
            {/* 🔴 `aria-current` BURADA YOK — etkin sayfayı gezinti şeridi
                işaretler. İkisi birden işaretlerse ekran okuyucu iki "geçerli
                sayfa" duyar; DOM'da tam bir tane olmalı. */}
            <MenuBaglantisi href={HESAP.href} ikon={HESAP.ikon}>Hesap ayarları</MenuBaglantisi>
            {yonetici && (
              <MenuBaglantisi href={YONETIM.href} ikon={YONETIM.ikon}>Yönetim paneli</MenuBaglantisi>
            )}

            <hr className="my-1 border-[var(--border)]" />

            {IKINCIL.map((s) => (
              <MenuBaglantisi key={s.href} href={s.href} ikon={s.ikon}>{s.etiket}</MenuBaglantisi>
            ))}

            <hr className="my-1 border-[var(--border)]" />

            <button type="button" role="menuitem" className={SATIR}
                    onClick={() => setTheme(koyuyaGec ? 'dark' : 'light')}>
              <span aria-hidden className="text-muted">
                {koyuyaGec
                  ? I('M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8')
                  : I('M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4')}
              </span>
              {koyuyaGec ? 'Koyu temaya geç' : 'Açık temaya geç'}
            </button>

            <hr className="my-1 border-[var(--border)]" />

            {/* Çıkış EN SONDA ve ayırıcının altında — gerekçe bileşen başında. */}
            <button type="button" role="menuitem" onClick={cikis} disabled={cikisBusy}
                    className={cx(SATIR, 'text-[var(--color-bad)] disabled:opacity-50')}>
              <span aria-hidden>
                {cikisBusy
                  ? <Spinner />
                  : I('M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4M16 17l5-5-5-5M21 12H9')}
              </span>
              Çıkış yap
            </button>
          </div>
        </div>
      )}
    </div>
  );
}

function MenuBaglantisi({
  href, ikon, children,
}: { href: string; ikon: React.ReactNode; children: React.ReactNode }) {
  return (
    <Link href={href} role="menuitem" className={SATIR}>
      <span aria-hidden className="text-muted">{ikon}</span>
      {children}
    </Link>
  );
}

/* ────────────────────────── Yardımcı bağlantılar ────────────────────────── */

/**
 * Sayfa altındaki ikincil şerit — HER GENİŞLİKTE.
 *
 * Eskiden yalnız `md:` altında çiziliyordu (masaüstünde yan sütun taşıyordu).
 * Yan sütun gidince bu bağlantıların masaüstü karşılığı da giderdi; şerit artık
 * her genişlikte duruyor. Hukuki metne ulaşım JavaScript'e bağlı olamaz —
 * profil menüsü hiç açılmasa bile buradan gidilir.
 *
 * `aria-current` BURADA taşınır (bu maddeler şeritte yok), yani
 * `/panel/yorumlarim` sayfasında da tam bir etkin işaret bulunur.
 */
export function PanelYardimciBaglantilar() {
  const yol = usePathname();
  return (
    <nav className="mt-10 border-t border-[var(--border)] pt-3" aria-label="Yardımcı bağlantılar">
      <ul className="flex flex-wrap items-center gap-x-2 gap-y-1">
        {IKINCIL.map((s) => {
          const etkin = yol === s.href || yol.startsWith(`${s.href}/`);
          return (
            <li key={s.href}>
              <Link
                href={s.href}
                aria-current={etkin ? 'page' : undefined}
                className={cx(
                  // `min-w-11`: "Gizlilik" 320px'te 43 px GENİŞLİKTE ölçülmüştü —
                  // 44 px kuralı İKİ EKSENDE de geçerlidir; otomatik denetim
                  // bunu yakalamıştı. Yatay dolgu + alt sınır birlikte düzeltir.
                  'inline-flex min-h-11 min-w-11 items-center justify-center rounded-xl px-2 text-sm',
                  '[transition-property:color,background-color] [transition-duration:var(--sure-hizli)]',
                  etkin ? 'font-semibold text-[var(--text)]' : 'text-muted hover:text-[var(--text)]',
                )}
              >
                {s.etiket}
              </Link>
            </li>
          );
        })}
      </ul>
    </nav>
  );
}
