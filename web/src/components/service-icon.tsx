'use client';

import * as React from 'react';
import { cx } from './ui';

/**
 * Servis logosu.
 *
 * Logo YOKSA veya YÜKLENEMEZSE harf rozetine düşer. Kırık bir <img> ikonu
 * göstermek, hiç logo göstermemekten kötüdür: kullanıcı sistemin bozuk
 * olduğunu düşünür. Bu yüzden `onError` yakalanır ve sessizce yedeğe geçilir.
 *
 * Logolar `web/public/servis-logolari/` altında durur ve veritabanındaki
 * `services.icon_url` alanı onlara işaret eder (`/servis-logolari/wa.svg`).
 * Ayarlamak için:  go run ./cmd/cli catalog:icon --service=wa --url=/servis-logolari/wa.svg
 *
 * Yedek görünüm bir "geçici çözüm" DEĞİLDİR, sürekli görülen bir durumdur:
 * katalogdaki her servisin logosu yoktur ve `icon_url` dolu olsa bile dosya
 * eksik olabilir (ölçüldü: `/servisler` sayfasında SSR edilen logoların bir
 * kısmı 404 dönüyordu). Bu yüzden yedek, servis kodundan türetilen kararlı
 * bir renk + baş harflerle ayrı ayrı tasarlandı.
 *
 * HİDRASYON: bu bileşenin İLK render'ı sunucuda ve istemcide aynıdır —
 * `renkSec` saf bir karma, `broken` başlangıçta `false`, `onError` yalnız
 * bağlandıktan sonra çalışır. Ölçüldü (`/fiyatlar`, SSR HTML ↔ hidre DOM):
 * renk sınıfı, `font-size` ve baş harfler birebir aynı; React uyarı vermiyor.
 * Sunucuda basılmış bir `<img>` hidrasyondan ÖNCE hata verse bile yedeğe
 * düşüyor — `/servisler`'de 55 logodan 404 dönenlerin hepsi rozete döndü,
 * DOM'da kırık `<img>` kalmadı.
 */

/** Yedek rozet paleti — `globals.css` içindeki ölçülmüş vurgu aileleri. */
const PALET = ['mavi', 'mor', 'yesil', 'turuncu', 'pembe', 'deniz'] as const;
export type ServisRenk = (typeof PALET)[number];

const ZEMIN: Record<ServisRenk, string> = {
  mavi:    'bg-[var(--v-mavi-zemin)]    text-[var(--v-mavi-metin)]',
  mor:     'bg-[var(--v-mor-zemin)]     text-[var(--v-mor-metin)]',
  yesil:   'bg-[var(--v-yesil-zemin)]   text-[var(--v-yesil-metin)]',
  turuncu: 'bg-[var(--v-turuncu-zemin)] text-[var(--v-turuncu-metin)]',
  pembe:   'bg-[var(--v-pembe-zemin)]   text-[var(--v-pembe-metin)]',
  deniz:   'bg-[var(--v-deniz-zemin)]   text-[var(--v-deniz-metin)]',
};

/**
 * Ada göre KARARLI renk seçer.
 *
 * Rastgele seçilseydi renk her render'da (ve sunucu ile istemci arasında)
 * değişir, hidrasyon uyuşmazlığı üretirdi. Basit bir toplama karması yeterli:
 * amaç güvenlik değil, "WhatsApp her yerde aynı renkte görünsün".
 */
function renkSec(anahtar: string): ServisRenk {
  let t = 0;
  for (let i = 0; i < anahtar.length; i++) t = (t * 31 + anahtar.charCodeAt(i)) >>> 0;
  return PALET[t % PALET.length] ?? 'mavi';
}

/** Baş harfler: iki kelimeli adlarda her kelimenin ilki ("Google Voice" → GV). */
function basHarfler(ad: string): string {
  const kelimeler = ad.trim().split(/[\s._-]+/).filter(Boolean);
  const [ilk, ikinci] = kelimeler;
  if (ilk && ikinci) return `${ilk[0] ?? ''}${ikinci[0] ?? ''}`.toLocaleUpperCase('tr');
  return ad.trim().slice(0, 2).toLocaleUpperCase('tr');
}

export function ServiceIcon({
  name, iconUrl, className, size = 32, renk,
}: {
  name: string; iconUrl?: string; className?: string; size?: number;
  /** Izgarada sıraya göre renk dağıtmak için. Verilmezse addan türetilir. */
  renk?: ServisRenk;
}) {
  const [broken, setBroken] = React.useState(false);
  const show = iconUrl && !broken;

  // iconUrl değişince kırık bayrağını sıfırla; yoksa bir kez hata veren
  // servis, logo düzeltildikten sonra da harf rozetinde kalırdı.
  React.useEffect(() => setBroken(false), [iconUrl]);

  const secilen = renk ?? renkSec(name);

  return (
    <span
      className={cx('grid shrink-0 place-items-center overflow-hidden rounded-xl', className)}
      style={{ width: size, height: size }}
      aria-hidden
    >
      {show ? (
        /* next/image yerine <img>: logolar `public/` altında statik dosyalar ve
           küçükler; Next'in optimizasyon hattına sokmak SVG'lerde kayıp veriyor
           ve dış alan adı yapılandırması gerektiriyor. */
        // eslint-disable-next-line @next/next/no-img-element
        <img
          src={iconUrl}
          alt=""
          width={size}
          height={size}
          loading="lazy"
          decoding="async"
          onError={() => setBroken(true)}
          className="size-full object-contain"
        />
      ) : (
        <span
          className={cx(
            'grid size-full place-items-center font-bold uppercase tracking-tight',
            ZEMIN[secilen],
          )}
          style={{ fontSize: Math.max(11, Math.round(size * 0.36)) }}
        >
          {basHarfler(name)}
        </span>
      )}
    </span>
  );
}
