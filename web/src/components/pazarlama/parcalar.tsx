/**
 * Pazarlama yüzeyinin ortak parçaları.
 *
 * Hepsi SUNUCU bileşenidir — hiçbiri durum tutmaz. Hareket ayrı bir sarmalayıcı
 * (`components/animasyon.tsx`) ile EKLENİR; böylece bu parçaların çıktısı
 * JavaScript'ten bağımsız olarak HTML'de durur (frontend-contract.md §10.2 S9).
 *
 * `ui.tsx` ORTAK bileşen dosyasıdır ve panel/yönetim de onu kullanır; oradaki
 * `Card`/`Badge` sade kalsın diye pazarlamaya özgü renkli varyantlar buraya
 * yazıldı, oraya değil.
 */
import * as React from 'react';
import { cx } from '../ui';

/** Vurgu ailesi. Renkler `globals.css` içinde iki tema için de ölçülerek tanımlı. */
export type Renk = 'mavi' | 'mor' | 'yesil' | 'turuncu' | 'pembe' | 'deniz';

const YUMUSAK: Record<Renk, string> = {
  mavi:    'bg-[var(--v-mavi-zemin)]    text-[var(--v-mavi-metin)]',
  mor:     'bg-[var(--v-mor-zemin)]     text-[var(--v-mor-metin)]',
  yesil:   'bg-[var(--v-yesil-zemin)]   text-[var(--v-yesil-metin)]',
  turuncu: 'bg-[var(--v-turuncu-zemin)] text-[var(--v-turuncu-metin)]',
  pembe:   'bg-[var(--v-pembe-zemin)]   text-[var(--v-pembe-metin)]',
  deniz:   'bg-[var(--v-deniz-zemin)]   text-[var(--v-deniz-metin)]',
};

const DOLU: Record<Renk, string> = {
  mavi: 'gradyan-mavi', mor: 'gradyan-mor', yesil: 'gradyan-yesil',
  turuncu: 'gradyan-turuncu', pembe: 'gradyan-pembe', deniz: 'gradyan-deniz',
};

const BOY = {
  sm: 'size-10 rounded-xl [&>svg]:size-5',
  md: 'size-12 rounded-2xl [&>svg]:size-6',
  lg: 'size-14 rounded-2xl [&>svg]:size-7',
};

/**
 * İkon karosu.
 *
 * `dolu` → doygun gradyan + BEYAZ simge (referans: hizmet kartları).
 * varsayılan → yumuşak renkli zemin + koyu simge (referans: istatistik ve
 * özellik kartları).
 *
 * Beyaz simge taşıyan gradyanların en açık ucu bile beyazla ≥ 4.5:1 verir;
 * yumuşak zeminlerde simge rengi zeminle ≥ 5.4:1. Ölçüm raporda.
 */
export function IkonKaro({
  renk = 'mavi', dolu = false, boy = 'md', className, children,
}: {
  renk?: Renk; dolu?: boolean; boy?: keyof typeof BOY;
  className?: string; children: React.ReactNode;
}) {
  return (
    <span
      className={cx(
        'grid shrink-0 place-items-center', BOY[boy],
        dolu ? cx(DOLU[renk], 'text-white golge-2') : YUMUSAK[renk],
        className,
      )}
      aria-hidden
    >
      {children}
    </span>
  );
}

/** Hap şeklinde bölüm rozeti — referans tasarımdaki "Profesyonel Çözümler". */
export function Hap({
  renk = 'mavi', children, className,
}: { renk?: Renk; children: React.ReactNode; className?: string }) {
  return (
    <span className={cx(
      'inline-flex items-center gap-2 rounded-full px-3.5 py-1.5',
      'text-xs font-semibold tracking-wide uppercase',
      YUMUSAK[renk], className,
    )}>
      {children}
    </span>
  );
}

/**
 * Bölüm başlığı.
 *
 * `vurgu` verilen kelime marka rengiyle boyanır (referans: "Hizmetlerimizi
 * **Keşfedin**"). Vurgu, başlığın İÇİNDEKİ bir kelimedir — ayrı bir düğüm
 * değil; bu yüzden `baslik` ikiye bölünmüş olarak alınır.
 */
export function BolumBasligi({
  hap, hapRenk = 'mavi', baslik, vurgu, sonEk, aciklama, ortala = true, id, seviye = 2,
}: {
  hap?: string; hapRenk?: Renk;
  baslik: string; vurgu?: string; sonEk?: string;
  aciklama?: React.ReactNode;
  ortala?: boolean; id?: string; seviye?: 1 | 2;
}) {
  const H = seviye === 1 ? 'h1' : 'h2';
  return (
    <div className={cx('flex flex-col gap-3', ortala && 'items-center text-center')}>
      {hap && <Hap renk={hapRenk}>{hap}</Hap>}
      <H
        id={id}
        className="text-balance text-2xl font-bold leading-tight tracking-tight
                   sm:text-3xl md:text-4xl"
      >
        {baslik}
        {vurgu && <> <span className="vurgu-metin">{vurgu}</span></>}
        {sonEk ? ` ${sonEk}` : null}
      </H>
      {aciklama && (
        <p className={cx(
          'text-pretty text-sm leading-relaxed text-muted md:text-base',
          ortala && 'max-w-2xl',
        )}>
          {aciklama}
        </p>
      )}
    </div>
  );
}

/** Renkli kart. `ui.tsx`'teki `Card`'ın pazarlama sürümü: gölge + hover yükselme. */
export function RenkliKart({
  className, children, ...rest
}: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      {...rest}
      className={cx(
        'surface kart-hover h-full rounded-2xl border p-5 golge-2 md:p-6',
        className,
      )}
    >
      {children}
    </div>
  );
}

/** Bölüm sarmalayıcısı — tek yerde tutulan yatay dolgu ve genişlik sınırı. */
export function Bolum({
  className, icClassName, children, ...rest
}: React.HTMLAttributes<HTMLElement> & { icClassName?: string }) {
  return (
    <section {...rest} className={cx('px-4 py-14 md:px-6 md:py-20', className)}>
      <div className={cx('mx-auto max-w-6xl', icClassName)}>{children}</div>
    </section>
  );
}
