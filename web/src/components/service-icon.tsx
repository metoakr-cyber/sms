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
 */
export function ServiceIcon({
  name, iconUrl, className, size = 32,
}: { name: string; iconUrl?: string; className?: string; size?: number }) {
  const [broken, setBroken] = React.useState(false);
  const show = iconUrl && !broken;

  // iconUrl değişince kırık bayrağını sıfırla; yoksa bir kez hata veren
  // servis, logo düzeltildikten sonra da harf rozetinde kalırdı.
  React.useEffect(() => setBroken(false), [iconUrl]);

  return (
    <span
      className={cx('grid shrink-0 place-items-center overflow-hidden rounded-lg', className)}
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
          className="grid size-full place-items-center bg-brand-500/15 text-[11px]
                     font-bold uppercase text-brand-300"
          style={{ fontSize: Math.max(10, size * 0.36) }}
        >
          {name.trim().slice(0, 2)}
        </span>
      )}
    </span>
  );
}
