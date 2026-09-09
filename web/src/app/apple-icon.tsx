import { ImageResponse } from 'next/og';

/**
 * iOS ana ekran simgesi (180×180).
 *
 * iOS köşeleri KENDİ maskesiyle yuvarlar, bu yüzden zemin tam kenara kadar
 * doldurulur — kendi `borderRadius`'umuzu koyarsak maskeden sonra çift
 * yuvarlatma ve kenarda ince bir hâle çıkar.
 *
 * 180 px'te `logo.tsx`'in tam motifi (mavi 360° oku + kırmızı onay) okunur;
 * `icon.tsx`'teki sadeleştirmeye gerek yoktur. Marka adı da sığıyor.
 */
export const size = { width: 180, height: 180 };
export const contentType = 'image/png';

const MAVI = '#1a7fd4';
const KIRMIZI = '#e30613';
const ZEMIN = '#0b1120'; // globals.css --color-ink-950

export default function AppleIcon() {
  return new ImageResponse(
    (
      <div
        style={{
          width: '100%',
          height: '100%',
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          justifyContent: 'center',
          gap: 6,
          backgroundColor: ZEMIN,
          backgroundImage: `linear-gradient(150deg, #12233f 0%, ${ZEMIN} 100%)`,
        }}
      >
        <svg width="104" height="104" viewBox="0 0 40 40">
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
        <div style={{ display: 'flex', alignItems: 'flex-start' }}>
          <span style={{ fontSize: 26, fontWeight: 700, color: MAVI, letterSpacing: -0.5 }}>
            Onay
          </span>
          <span style={{ fontSize: 26, fontWeight: 700, color: KIRMIZI, letterSpacing: -0.5 }}>
            360
          </span>
        </div>
      </div>
    ),
    size,
  );
}
