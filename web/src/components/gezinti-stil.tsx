/**
 * Gezinti satırlarının ORTAK görsel dili — `/panel` ve `/yonetim`.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * NEDEN AYRI DOSYA
 * ══════════════════════════════════════════════════════════════════════════
 * Bu jetonlar `panel-header.tsx` içinde yazılmıştı ve `/yonetim` gezintisi
 * (`yonetim/bolum-gezinti.tsx`) onlardan habersizdi: müşteri panelinde ikon
 * karolu, vurgu çubuklu, renkli satırlar; yönetimde düz metin bağlantılar.
 * Aynı kişi iki yüzey arasında gidip geliyor ve gezinti her geçişte başka bir
 * dil konuşuyordu (kullanıcı kararı, 10 Eylül 2026: "sidebar tarafında
 * butonlar müşteri tarafıyla aynı olsun").
 *
 * Jetonları kopyalamak yerine buraya taşındı — kopyalansaydı bir renk ailesi
 * değiştiğinde iki dosyadan biri unutulur ve yüzeyler sessizce ayrışırdı.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * 🔴 SINIF DİZGİLERİ STATİK YAZILIR
 * ══════════════════════════════════════════════════════════════════════════
 * Tailwind kaynağı METİN olarak tarar. `` `lg:${degisken}` `` gibi çalışma
 * anında kurulan bir sınıf üretilen CSS'te HİÇ OLUŞMAZ ve öğe sessizce
 * renksiz kalır — ne tsc ne derleme bunu yakalar. Bu yüzden `lg:` önekli
 * varyantlar ayrı haritalar olarak, tam hâlleriyle yazılıdır.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * RENK TEK BAŞINA ANLAM TAŞIMAZ (§7.1)
 * ══════════════════════════════════════════════════════════════════════════
 * Renk İKONU taşır, metni değil: etiket her zaman `--muted` ya da `--text`,
 * yani renk seçimi metin kontrastını hiçbir durumda değiştirmez. Etkin madde
 * ayrıca `aria-current="page"`, `font-semibold` ve ölçülmüş zemin tonunu
 * taşır — üç kanal.
 *
 * Değerler `globals.css`'teki ölçülmüş vurgu ailelerinden gelir
 * (`--v-*-zemin` / `--v-*-metin`); her çift iki temada da ≥4.5:1.
 */

import * as React from 'react';

export type Renk = 'mavi' | 'mor' | 'yesil' | 'turuncu' | 'pembe' | 'deniz';

/**
 * Tek yollu SVG ikon. Emoji/unicode DEĞİL (`tasarim-sistemi.md §9.2`):
 * tek kütüphane, tek çizgi kalınlığı (1.8).
 */
export const Ikon = (d: string) => (
  <svg
    viewBox="0 0 24 24"
    className="size-5 shrink-0"
    fill="none"
    stroke="currentColor"
    strokeWidth="1.8"
    strokeLinecap="round"
    strokeLinejoin="round"
    aria-hidden
  >
    <path d={d} />
  </svg>
);

/** İkonun kendi rengi — her genişlikte aynı. */
export const IKON_RENK: Record<Renk, string> = {
  mavi: 'text-[var(--v-mavi-metin)]',
  mor: 'text-[var(--v-mor-metin)]',
  yesil: 'text-[var(--v-yesil-metin)]',
  turuncu: 'text-[var(--v-turuncu-metin)]',
  pembe: 'text-[var(--v-pembe-metin)]',
  deniz: 'text-[var(--v-deniz-metin)]',
};

/** Etkin maddenin zemini — ikon rengiyle ÖLÇÜLMÜŞ çift. */
export const ETKIN_ZEMIN: Record<Renk, string> = {
  mavi: 'bg-[var(--v-mavi-zemin)]',
  mor: 'bg-[var(--v-mor-zemin)]',
  yesil: 'bg-[var(--v-yesil-zemin)]',
  turuncu: 'bg-[var(--v-turuncu-zemin)]',
  pembe: 'bg-[var(--v-pembe-zemin)]',
  deniz: 'bg-[var(--v-deniz-zemin)]',
};

/* ─── Her genişlikte uygulanan varyantlar (dikey liste: `/yonetim`) ─── */

/** İkon karosunun zemini. */
export const KARO_ZEMIN: Record<Renk, string> = {
  mavi: 'bg-[var(--v-mavi-zemin)]',
  mor: 'bg-[var(--v-mor-zemin)]',
  yesil: 'bg-[var(--v-yesil-zemin)]',
  turuncu: 'bg-[var(--v-turuncu-zemin)]',
  pembe: 'bg-[var(--v-pembe-zemin)]',
  deniz: 'bg-[var(--v-deniz-zemin)]',
};

/** Etkin maddenin SOL VURGU ÇUBUĞU — ikon rengiyle aynı aile. */
export const VURGU_CUBUK: Record<Renk, string> = {
  mavi: 'before:bg-[var(--v-mavi-metin)]',
  mor: 'before:bg-[var(--v-mor-metin)]',
  yesil: 'before:bg-[var(--v-yesil-metin)]',
  turuncu: 'before:bg-[var(--v-turuncu-metin)]',
  pembe: 'before:bg-[var(--v-pembe-metin)]',
  deniz: 'before:bg-[var(--v-deniz-metin)]',
};

/* ─── `≥ lg` varyantları (`/panel`: `< lg`'de alt çubuk, orada karo yok) ─── */

export const KARO_ZEMIN_LG: Record<Renk, string> = {
  mavi: 'lg:bg-[var(--v-mavi-zemin)]',
  mor: 'lg:bg-[var(--v-mor-zemin)]',
  yesil: 'lg:bg-[var(--v-yesil-zemin)]',
  turuncu: 'lg:bg-[var(--v-turuncu-zemin)]',
  pembe: 'lg:bg-[var(--v-pembe-zemin)]',
  deniz: 'lg:bg-[var(--v-deniz-zemin)]',
};

export const VURGU_CUBUK_LG: Record<Renk, string> = {
  mavi: 'lg:before:bg-[var(--v-mavi-metin)]',
  mor: 'lg:before:bg-[var(--v-mor-metin)]',
  yesil: 'lg:before:bg-[var(--v-yesil-metin)]',
  turuncu: 'lg:before:bg-[var(--v-turuncu-metin)]',
  pembe: 'lg:before:bg-[var(--v-pembe-metin)]',
  deniz: 'lg:before:bg-[var(--v-deniz-metin)]',
};
