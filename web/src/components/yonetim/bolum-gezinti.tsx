'use client';

/**
 * BolumGezinti — yönetim panelinin bölümlere ayrılmış gezintisi.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * NEDEN VAR
 * ══════════════════════════════════════════════════════════════════════════
 * `admin-shell.tsx:82-83` KENDİ YORUMUNDA "2-3 sekme için kabul edilebilir"
 * yazıyor ama `NAV` dizisinde 10 SEKME var; hepsi mobilde yatay kaydırılan
 * tek bir şeride diziliyor. Kaydırma çubuğunun sağında kalan sekmeler pratikte
 * görülmez — yani panelin yarısı keşfedilemez durumda. Aynı depoda
 * `panel-shell.tsx` DAHA AZ bölüm için yan menü kullanıyor: yönetim paneli
 * daha fazla bölümle daha zayıf desene sahip.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * BÖLÜMLER — kullanıcı kararı (9 Eylül 2026, tartışmasız)
 * ══════════════════════════════════════════════════════════════════════════
 * Yan sütun KALIYOR ve dört bölüme ayrılıyor:
 *   Para · Katalog · Destek · Sistem
 * Düz bir 10 maddelik liste, bir maddeyi bulmak için 10 maddeyi okumayı
 * gerektirir. Dört başlık altında 2–3 madde, önce başlığı taramayı sağlar —
 * Operate modunun tek ölçütü budur (taranabilirlik, §1.1).
 *
 * ══════════════════════════════════════════════════════════════════════════
 * 🔴 "TALEPLER" İKİ KEZ GEÇİYOR — kasıtlı
 * ══════════════════════════════════════════════════════════════════════════
 * Para bölümünde "Talepler" (bakiye talepleri), Destek bölümünde "Talepler"
 * (destek talepleri). Görünen metinler kullanıcı kararıdır ve değiştirilmedi.
 * Belirsizliği BÖLÜM BAŞLIĞI çözer ve bu ekran okuyucuya da taşınır:
 * her `<ul>`, bölüm başlığına `aria-labelledby` ile bağlıdır, yani liste
 * "Para" / "Destek" adıyla duyurulur.
 * 🔴 Bağlantılara `aria-label` ile "Bakiye talepleri" YAZILMADI: WCAG 2.5.3
 * (Label in Name) erişilebilir adın görünen metni İÇERMESİNİ ister; sesle
 * kontrol eden kullanıcı gördüğü kelimeyi söyler.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * HAREKET YOK (§4.2)
 * ══════════════════════════════════════════════════════════════════════════
 * Gezinti geçişi günde 100+ kez yaşanır: "Animasyon yok. Asla." Bölümler
 * AÇILIR-KAPANIR DEĞİLDİR — 2-3 maddelik bir bölümü katlamak, her gezinti
 * için fazladan bir tıklama demektir ve Operate modu tıklama sayısıyla
 * savunulur, gösterişle değil. Tek geçiş hover RENGİdir (`--sure-hizli`).
 *
 * ══════════════════════════════════════════════════════════════════════════
 * BU DALGADA BAĞLANMIYOR
 * ══════════════════════════════════════════════════════════════════════════
 * `admin-shell.tsx` DEĞİŞTİRİLMEDİ. Bileşen sunum kabından bağımsızdır:
 * masaüstünde bir `<aside>` içinde, mobilde bir çekmecenin içinde aynı şekilde
 * doğru görünür. Kabı seçmek sonraki dalganın işidir.
 */

import * as React from 'react';
import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { cx } from '@/components/ui';
/*
 * GÖRSEL DİL MÜŞTERİ PANELİYLE ORTAK (kullanıcı kararı, 10 Eylül 2026).
 * İkon karosu, renk ailesi, etkin zemin ve sol vurgu çubuğu
 * `components/gezinti-stil.tsx` içinde tanımlı ve `panel-header.tsx` ile
 * PAYLAŞILIYOR. Kopyalansaydı bir renk ailesi değiştiğinde iki dosyadan biri
 * unutulur ve iki yüzey sessizce ayrışırdı.
 *
 * Öneksiz haritalar kullanılır (`lg:` olanlar değil): bu gezinti HER
 * genişlikte dikey bir listedir — masaüstünde yan sütunda, `< lg`'de
 * çekmecenin içinde. Panelin `< lg` alt çubuğu gibi ikinci bir sunumu yok.
 */
import {
  ETKIN_ZEMIN,
  IKON_RENK,
  Ikon,
  KARO_ZEMIN,
  VURGU_CUBUK,
  type Renk,
} from '@/components/gezinti-stil';

interface GezintiOgesi {
  href: string;
  etiket: string;
  ikon: React.ReactNode;
  renk: Renk;
}

interface GezintiBolumu {
  baslik: string;
  ogeler: readonly GezintiOgesi[];
}

/** Bölümsüz, en üstte duran giriş noktası. */
export const GENEL_BAKIS: GezintiOgesi = {
  href: '/yonetim',
  etiket: 'Genel bakış',
  renk: 'mavi',
  ikon: Ikon('M4 4h7v7H4zM13 4h7v4h-7zM13 12h7v8h-7zM4 15h7v5H4z'),
};

export const BOLUMLER: readonly GezintiBolumu[] = [
  {
    baslik: 'Para',
    ogeler: [
      {
        href: '/yonetim/talepler',
        etiket: 'Talepler',
        renk: 'yesil',
        ikon: Ikon('M5 6h14a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2zM3 10h18M7 15h4'),
      },
      {
        href: '/yonetim/odeme-yontemleri',
        etiket: 'Ödeme yöntemleri',
        renk: 'deniz',
        ikon: Ikon('M21 12V7H5a2 2 0 0 1 0-4h14v4M3 5v14a2 2 0 0 0 2 2h16v-5M18 12a2 2 0 0 0 0 4h4v-4z'),
      },
      {
        href: '/yonetim/bakiye',
        etiket: 'Bakiye',
        renk: 'turuncu',
        ikon: Ikon('M12 1v22M17 5H9.5a3.5 3.5 0 0 0 0 7h5a3.5 3.5 0 0 1 0 7H6'),
      },
    ],
  },
  {
    baslik: 'Katalog',
    ogeler: [
      {
        href: '/yonetim/saglayicilar',
        etiket: 'Sağlayıcılar',
        renk: 'mor',
        ikon: Ikon('M5 12V7a2 2 0 0 1 2-2h10a2 2 0 0 1 2 2v5M3 12h18v5a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2zM7 16h.01M11 16h.01'),
      },
      {
        href: '/yonetim/fiyatlar',
        etiket: 'Fiyatlar',
        renk: 'pembe',
        ikon: Ikon('M20.6 13.4 13.4 20.6a2 2 0 0 1-2.8 0l-7.2-7.2A2 2 0 0 1 3 12V4a1 1 0 0 1 1-1h8a2 2 0 0 1 1.4.6l7.2 7.2a2 2 0 0 1 0 2.8zM7.5 7.5h.01'),
      },
    ],
  },
  {
    baslik: 'Destek',
    ogeler: [
      {
        href: '/yonetim/destek',
        etiket: 'Talepler',
        renk: 'mavi',
        ikon: Ikon('M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z'),
      },
      {
        href: '/yonetim/yorumlar',
        etiket: 'Yorumlar',
        renk: 'pembe',
        ikon: Ikon('M11.5 3.2l2.6 5.3 5.8.8-4.2 4.1 1 5.8-5.2-2.7-5.2 2.7 1-5.8L3.1 9.3l5.8-.8z'),
      },
    ],
  },
  {
    baslik: 'Sistem',
    ogeler: [
      {
        href: '/yonetim/kullanicilar',
        etiket: 'Kullanıcılar',
        renk: 'deniz',
        ikon: Ikon('M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2M9 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8M23 21v-2a4 4 0 0 0-3-3.9'),
      },
      {
        href: '/yonetim/denetim',
        etiket: 'Denetim',
        renk: 'mor',
        ikon: Ikon('M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8zM14 2v6h6M9 15l2 2 4-4'),
      },
    ],
  },
];

/**
 * `/yonetim` yalnız TAM eşleşmede etkindir; diğerleri önek eşleşmesiyle
 * (alt sayfalar da bölümü etkin göstermeli). `startsWith` kullanılırsa
 * `/yonetim` HER SAYFADA etkin görünürdü.
 */
function etkinMi(href: string, yol: string): boolean {
  return href === '/yonetim' ? yol === '/yonetim' : yol === href || yol.startsWith(`${href}/`);
}

export function BolumGezinti({ className }: { className?: string }) {
  const yol = usePathname();
  const idOneki = React.useId();

  return (
    <nav aria-label="Yönetim bölümleri" className={cx('flex flex-col gap-6', className)}>
      <ul className="flex flex-col gap-1">
        <li>
          <GezintiBaglantisi oge={GENEL_BAKIS} etkin={etkinMi(GENEL_BAKIS.href, yol)} />
        </li>
      </ul>

      {BOLUMLER.map((bolum, i) => {
        const basId = `${idOneki}-bolum-${i}`;
        return (
          <div key={bolum.baslik} className="flex flex-col gap-2">
            {/*
              Bölüm başlığı `<h2>` DEĞİL, `<p>` + `aria-labelledby`:
              §3.3 `h2`'yi `text-lg font-semibold` olarak sabitliyor ve 18px
              kalın bir başlık bir gezinti etiketinin taşıması gereken ağırlık
              değil. `<p>` + `aria-labelledby` listeye erişilebilir bir AD
              verir — başlık hiyerarşisini şişirmeden aynı bilgiyi taşır.
              `text-sm`, `text-xs` değil (§3.2).
            */}
            <p id={basId} className="px-3 text-sm font-medium text-muted">
              {bolum.baslik}
            </p>
            <ul aria-labelledby={basId} className="flex flex-col gap-1">
              {bolum.ogeler.map((oge) => (
                <li key={oge.href}>
                  <GezintiBaglantisi oge={oge} etkin={etkinMi(oge.href, yol)} />
                </li>
              ))}
            </ul>
          </div>
        );
      })}
    </nav>
  );
}

function GezintiBaglantisi({ oge, etkin }: { oge: GezintiOgesi; etkin: boolean }) {
  return (
    <Link
      href={oge.href}
      aria-current={etkin ? 'page' : undefined}
      className={cx(
        // Ölçüler `panel-header.tsx`'in yan sütun satırıyla BİREBİR:
        // min-h-11 (44px dokunma hedefi, §7.3) · gap-3 · px-3 · text-sm.
        // `relative`: etkin maddenin sol vurgu çubuğu `before:absolute` ile
        // çizilir ve bir konumlanma bağlamı ister.
        'relative flex min-h-11 w-full items-center gap-3 rounded-xl px-3 text-sm',
        // Yalnız RENK geçer — konum/ölçek yok (§4.2 gezinti satırı).
        '[transition-property:color,background-color]',
        '[transition-duration:var(--sure-hizli)]',
        etkin
          ? /*
             ETKİN DURUM ÜÇ KANAL taşır (§7.1, anlam yalnız renkle taşınmaz):
             `aria-current="page"` · `font-semibold` · ölçülmüş zemin tonu.
             Dördüncüsü SOL VURGU ÇUBUĞU — referans düzenin işareti; dikey
             listede gözün etkin satırı yakalamasını hızlandırır.

             🔴 Eski hâl `bg-brand-500/12` idi ve maddenin renk ailesinden
             bağımsızdı; panelde ise etkin zemin ikonun rengiyle ÖLÇÜLMÜŞ
             çiftidir. Artık iki yüzey aynı çifti kullanıyor.
            */
            cx(
              'font-semibold text-[var(--text)]',
              ETKIN_ZEMIN[oge.renk],
              'before:absolute before:left-0 before:top-1.5 before:bottom-1.5',
              'before:w-1 before:rounded-r-full',
              VURGU_CUBUK[oge.renk],
            )
          : 'text-muted hover:bg-[var(--raised)] hover:text-[var(--text)]',
      )}
    >
      {/*
        İKON KAROSU: yumuşak renkli zemin + üstünde renkli ikon.
        Zemin/metin çifti `--v-*-zemin` / `--v-*-metin`; ikisi BİRLİKTE
        ölçülmüş (globals.css). Karo, ikonu kendi ölçüldüğü zeminin üstüne
        koyar — yani kontrast varsayım değil.
      */}
      <span
        className={cx(
          'flex size-9 shrink-0 items-center justify-center rounded-lg',
          IKON_RENK[oge.renk],
          KARO_ZEMIN[oge.renk],
        )}
      >
        {oge.ikon}
      </span>

      {/* `truncate`: "Ödeme yöntemleri" 240px'lik sütunda karo + boşluklardan
          sonra sınırda kalıyor; kırpma sarmadan daha iyi — iki satıra saran
          bir gezinti maddesi komşularıyla hizasız durur. */}
      <span className="min-w-0 truncate">{oge.etiket}</span>
    </Link>
  );
}
