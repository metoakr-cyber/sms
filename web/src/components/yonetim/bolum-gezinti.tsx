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

interface GezintiOgesi {
  href: string;
  etiket: string;
}

interface GezintiBolumu {
  baslik: string;
  ogeler: readonly GezintiOgesi[];
}

/** Bölümsüz, en üstte duran giriş noktası. */
export const GENEL_BAKIS: GezintiOgesi = { href: '/yonetim', etiket: 'Genel bakış' };

export const BOLUMLER: readonly GezintiBolumu[] = [
  {
    baslik: 'Para',
    ogeler: [
      { href: '/yonetim/talepler', etiket: 'Talepler' },
      { href: '/yonetim/odeme-yontemleri', etiket: 'Ödeme yöntemleri' },
      { href: '/yonetim/bakiye', etiket: 'Bakiye' },
    ],
  },
  {
    baslik: 'Katalog',
    ogeler: [
      { href: '/yonetim/saglayicilar', etiket: 'Sağlayıcılar' },
      { href: '/yonetim/fiyatlar', etiket: 'Fiyatlar' },
    ],
  },
  {
    baslik: 'Destek',
    ogeler: [
      { href: '/yonetim/destek', etiket: 'Talepler' },
      { href: '/yonetim/yorumlar', etiket: 'Yorumlar' },
    ],
  },
  {
    baslik: 'Sistem',
    ogeler: [
      { href: '/yonetim/kullanicilar', etiket: 'Kullanıcılar' },
      { href: '/yonetim/denetim', etiket: 'Denetim' },
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
        // min-h-11 = 44px dokunma hedefi (§7.3). `gap-1` ile öğeler arası
        // 4px + dolgu, hedefler arası ≥8px kuralını karşılar.
        'flex min-h-11 items-center rounded-xl px-3 text-sm',
        // Yalnız RENK geçer — konum/ölçek yok (§4.2 gezinti satırı).
        '[transition-property:color,background-color]',
        '[transition-duration:var(--sure-hizli)]',
        etkin
          ? /*
             ETKİN DURUM ÜÇ KANAL taşır: aria-current (ekran okuyucu),
             font-semibold (biçim) ve marka tonu (renk). §7.1 — anlam yalnız
             renkle taşınmaz.
             🔴 `text-brand-300` KULLANILMADI: açık temada beyaz üstünde
             ~2.3:1 verir, 4.5:1 eşiğinin çok altında. Metin `--text`te kalır,
             zemin marka tonuyla boyanır; kontrast iki temada da korunur.
            */
            'bg-brand-500/12 font-semibold text-[var(--text)]'
          : 'text-muted hover:bg-[var(--raised)] hover:text-[var(--text)]',
      )}
    >
      {oge.etiket}
    </Link>
  );
}
