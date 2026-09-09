/**
 * Arayüz ikonları — SATIR İÇİ SVG.
 *
 * NEDEN KÜTÜPHANE YOK
 * ───────────────────
 * `lucide-react` / `heroicons` gibi bir paket eklemek 20 ikon için yüzlerce
 * ikonluk bir bağımlılık, ek bir paket boyutu ve bir tedarik zinciri yüzeyi
 * demek. Buradaki set 24×24 ızgarada, tek `stroke-width` ile çizilmiş ve
 * `currentColor` kullanır — rengi kapsayıcıdan alır, tema değişince kendisi
 * uyar. Servis LOGOLARI ayrı bir konudur: onlar veritabanından gelir
 * (`service-icon.tsx`).
 *
 * Kural: her ikon `aria-hidden`'dır. Anlam TAŞIMAZLAR; yanlarındaki metin
 * taşır. Tek başına duran bir ikon butonuna `aria-label` YAZILIR.
 */
import * as React from 'react';

export type IkonProps = React.SVGProps<SVGSVGElement> & { className?: string };

function Cerceve({ children, className = 'size-6', ...rest }: IkonProps) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.8}
      strokeLinecap="round"
      strokeLinejoin="round"
      className={className}
      aria-hidden
      {...rest}
    >
      {children}
    </svg>
  );
}

/* ─────────────── Ürün ─────────────── */

/** SMS balonu — gelen doğrulama mesajı. */
export function SmsBalon(p: IkonProps) {
  return (
    <Cerceve {...p}>
      <path d="M21 11.5a8.4 8.4 0 0 1-9 8.4 9.6 9.6 0 0 1-2.9-.4L4 21l1.3-3.9A8.2 8.2 0 0 1 3.6 11 8.4 8.4 0 0 1 12 3a8.4 8.4 0 0 1 9 8.5Z" />
      <path d="M8.5 11.5h.01M12 11.5h.01M15.5 11.5h.01" strokeWidth={2.4} />
    </Cerceve>
  );
}

/** Küre — ülke seçimi. */
export function Kure(p: IkonProps) {
  return (
    <Cerceve {...p}>
      <circle cx="12" cy="12" r="9" />
      <path d="M3.2 9h17.6M3.2 15h17.6" />
      <path d="M12 3c2.4 2.6 3.6 5.6 3.6 9s-1.2 6.4-3.6 9c-2.4-2.6-3.6-5.6-3.6-9S9.6 5.6 12 3Z" />
    </Cerceve>
  );
}

/** Telefon — sanal numara. */
export function Telefon(p: IkonProps) {
  return (
    <Cerceve {...p}>
      <rect x="6" y="2.5" width="12" height="19" rx="2.6" />
      <path d="M10.4 5.6h3.2" />
      <path d="M10.8 18.4h2.4" />
    </Cerceve>
  );
}

/** Kalkan — güvenlik, gizlilik. */
export function Kalkan(p: IkonProps) {
  return (
    <Cerceve {...p}>
      <path d="M12 2.8 4.8 5.8v5.4c0 4.4 3 8.3 7.2 9.9 4.2-1.6 7.2-5.5 7.2-9.9V5.8L12 2.8Z" />
      <path d="m9 12 2.1 2.2L15.2 10" />
    </Cerceve>
  );
}

/** Kullanıcılar — hesap, çoklu servis. */
export function Kullanicilar(p: IkonProps) {
  return (
    <Cerceve {...p}>
      <circle cx="9" cy="8" r="3.2" />
      <path d="M2.8 20a6.2 6.2 0 0 1 12.4 0" />
      <path d="M16.4 5.2a3.2 3.2 0 0 1 0 5.9" />
      <path d="M17.6 14.2A6.2 6.2 0 0 1 21.2 20" />
    </Cerceve>
  );
}

/** Onay — tamamlandı. */
export function Onay(p: IkonProps) {
  return (
    <Cerceve {...p}>
      <circle cx="12" cy="12" r="9" />
      <path d="m8.2 12.2 2.6 2.6 5-5.4" />
    </Cerceve>
  );
}

/** Saat — süre, geri sayım. */
export function Saat(p: IkonProps) {
  return (
    <Cerceve {...p}>
      <circle cx="12" cy="12" r="9" />
      <path d="M12 7.2V12l3.2 1.9" />
    </Cerceve>
  );
}

/** Yıldırım — anında. */
export function Yildirim(p: IkonProps) {
  return (
    <Cerceve {...p}>
      <path d="M13.4 2.5 4.6 13.4h6.2l-.8 8.1 8.8-10.9h-6.2l.8-8.1Z" />
    </Cerceve>
  );
}

/** İade — para geri döner. */
export function Iade(p: IkonProps) {
  return (
    <Cerceve {...p}>
      <path d="M3.4 9.6A9 9 0 0 1 20 8.4" />
      <path d="M3.4 4.6v5h5" />
      <path d="M20.6 14.4A9 9 0 0 1 4 15.6" />
      <path d="M20.6 19.4v-5h-5" />
    </Cerceve>
  );
}

/** Cüzdan — bakiye. */
export function Cuzdan(p: IkonProps) {
  return (
    <Cerceve {...p}>
      <path d="M3 7.4A2.4 2.4 0 0 1 5.4 5H17a2 2 0 0 1 2 2v1.4" />
      <path d="M3 7.4v9.2A2.4 2.4 0 0 0 5.4 19h13.2a2.4 2.4 0 0 0 2.4-2.4v-5.2a2.4 2.4 0 0 0-2.4-2.4H5.4A2.4 2.4 0 0 1 3 6.6" />
      <path d="M17 13.6h.01" strokeWidth={2.6} />
    </Cerceve>
  );
}

/** Kilit — gizlilik. */
export function Kilit(p: IkonProps) {
  return (
    <Cerceve {...p}>
      <rect x="4.4" y="10.2" width="15.2" height="10.6" rx="2.4" />
      <path d="M8.2 10.2V7.6a3.8 3.8 0 0 1 7.6 0v2.6" />
      <path d="M12 14.4v2.4" />
    </Cerceve>
  );
}

/** Takvim — kiralama süresi. */
export function Takvim(p: IkonProps) {
  return (
    <Cerceve {...p}>
      <rect x="3.4" y="5" width="17.2" height="16" rx="2.4" />
      <path d="M3.4 9.8h17.2M8.4 3v4M15.6 3v4" />
      <path d="M8 14h2M8 17.4h2M14 14h2" strokeWidth={2} />
    </Cerceve>
  );
}

/** Izgara — servis kataloğu. */
export function Izgara(p: IkonProps) {
  return (
    <Cerceve {...p}>
      <rect x="3.4" y="3.4" width="7.2" height="7.2" rx="2" />
      <rect x="13.4" y="3.4" width="7.2" height="7.2" rx="2" />
      <rect x="3.4" y="13.4" width="7.2" height="7.2" rx="2" />
      <rect x="13.4" y="13.4" width="7.2" height="7.2" rx="2" />
    </Cerceve>
  );
}

/** Grafik — istatistik. */
export function Grafik(p: IkonProps) {
  return (
    <Cerceve {...p}>
      <path d="M3.4 20.6h17.2" />
      <path d="M6.6 20.6v-6.2M11 20.6V7.4M15.4 20.6v-9M19.8 20.6V4.2" />
    </Cerceve>
  );
}

/* ─────────────── Gezinme ─────────────── */

export function SagOk(p: IkonProps) {
  return (
    <Cerceve {...p}>
      <path d="M4.6 12h14M13.4 6.6 18.8 12l-5.4 5.4" />
    </Cerceve>
  );
}

export function Arti(p: IkonProps) {
  return (
    <Cerceve {...p}>
      <path d="M12 5.4v13.2M5.4 12h13.2" strokeWidth={2.2} />
    </Cerceve>
  );
}

export function Eksi(p: IkonProps) {
  return (
    <Cerceve {...p}>
      <path d="M5.4 12h13.2" strokeWidth={2.2} />
    </Cerceve>
  );
}

export function Yildiz(p: IkonProps) {
  return (
    <Cerceve {...p} fill="currentColor" strokeWidth={0}>
      <path d="m12 2.6 2.9 5.9 6.5.9-4.7 4.6 1.1 6.4-5.8-3-5.8 3 1.1-6.4L2.6 9.4l6.5-.9L12 2.6Z" />
    </Cerceve>
  );
}
