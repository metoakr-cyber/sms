import { ImageResponse } from 'next/og';

/**
 * Site simgeleri — TEK dosyadan üç boyut üretilir: 32 (sekme), 192 ve 512
 * (manifest / ana ekrana ekle). Chrome kurulabilirlik için manifest'te en az
 * 192 ve 512 px PNG ister; 32'lik favicon tek başına YETMEZ.
 * test: web/scripts/seo-check.mjs  (üç boyutun da 200 + image/png döndüğü ve
 * manifest'te 192/512'nin bildirildiği ölçülür)
 *
 * Biçim boyuta göre DEĞİŞİR, bu bilinçlidir:
 *
 *  • 32 px'te `logo.tsx`'in ince 360° oku okunmaz — 40 birimlik viewBox'ta
 *    3,4 birimlik kontur 32 px'e inince ~2,7 px kalır ve dairesel ok ile onay
 *    işareti gri bir lekeye dönüşür. Bu yüzden 32'de dolu marka mavisi zemin +
 *    KALIN BEYAZ onay işareti kullanılır; taşıyıcı biçim odur.
 *  • 192/512'de tam motif (mavi 360° oku + kırmızı onay) rahat okunur.
 *
 * Çıktı gerçekten üretilip 8× büyütülerek (nearest-neighbor) hem açık hem
 * koyu sekme zemininde incelendi; "muhtemelen okunur" varsayımı değildir.
 *
 * 🔴 FONT: `fonts` seçeneği verilmez — bkz. `opengraph-image.tsx`. Bu dosyada
 * zaten metin yoktur, dolayısıyla font gerekmez.
 */
const MAVI = '#1a7fd4';
const KIRMIZI = '#e30613';
const ZEMIN = '#0b1120'; // globals.css --color-ink-950

// Üç simge de DERLEME ANINDA üretilir. Bunu yazmazsak Next `icon.tsx` +
// `generateImageMetadata` bileşimini istek başına render eder (derleme
// çıktısında ƒ) ve her favicon isteği bir PNG rasterleştirmesi olur.
export const dynamic = 'force-static';

export function generateImageMetadata() {
  return [
    { id: '32', size: { width: 32, height: 32 }, contentType: 'image/png' },
    { id: '192', size: { width: 192, height: 192 }, contentType: 'image/png' },
    { id: '512', size: { width: 512, height: 512 }, contentType: 'image/png' },
  ];
}

export default function Icon({ id }: { id: string }) {
  const kenar = id === '512' ? 512 : id === '192' ? 192 : 32;

  if (kenar === 32) {
    return new ImageResponse(
      (
        <div
          style={{
            width: '100%',
            height: '100%',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            backgroundColor: MAVI,
            borderRadius: 7,
          }}
        >
          <svg width="32" height="32" viewBox="0 0 32 32">
            {/* 360° yayı — kırmızı, sağ üstte açık uçlu */}
            <path
              d="M16 5.2a10.8 10.8 0 1 0 9.9 6.6"
              fill="none"
              stroke={KIRMIZI}
              strokeWidth="3"
              strokeLinecap="round"
            />
            {/* onay işareti — beyaz ve kalın: 32 px'te taşıyıcı biçim budur */}
            <path
              d="M10.4 16.4l4.2 4.4L23 10.6"
              fill="none"
              stroke="#ffffff"
              strokeWidth="4"
              strokeLinecap="round"
              strokeLinejoin="round"
            />
          </svg>
        </div>
      ),
      { width: 32, height: 32 },
    );
  }

  // Motif kenarın %62'sinde tutuluyor: Android maskesi (daire/squircle)
  // kenardan ~%10 kırpar; %62'lik motif her maskede tam kalır.
  const motif = Math.round(kenar * 0.62);
  return new ImageResponse(
    (
      <div
        style={{
          width: '100%',
          height: '100%',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          backgroundColor: ZEMIN,
          backgroundImage: `linear-gradient(150deg, #12233f 0%, ${ZEMIN} 100%)`,
        }}
      >
        <svg width={motif} height={motif} viewBox="0 0 40 40">
          <path
            d="M20 5.5a14.5 14.5 0 1 0 13.4 9"
            fill="none"
            stroke={MAVI}
            strokeWidth="3.4"
            strokeLinecap="round"
          />
          <path d="M28.8 4.2l5.6 3.4-4.2 4.6z" fill={MAVI} />
          <path
            d="M12.5 20.5l5.4 5.6L31 10.8"
            fill="none"
            stroke={KIRMIZI}
            strokeWidth="4.2"
            strokeLinecap="round"
            strokeLinejoin="round"
          />
        </svg>
      </div>
    ),
    { width: kenar, height: kenar },
  );
}
