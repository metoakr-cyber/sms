import { ImageResponse } from 'next/og';

/**
 * Paylaşım kapak görseli (1200×630) — ÜRETİLİR, statik dosya değildir.
 *
 * NEDEN üretiliyor: `public/og-gorsel.png` bir kez çizilir ve marka değişince
 * kimse güncellemeyi hatırlamaz. Burada görsel `logo.tsx` ile aynı motiften
 * (360° dairesel ok + onay işareti, mavi #1a7fd4 / kırmızı #e30613) türetilir.
 *
 * 🔴 FONT: `fonts` seçeneği BİLEREK VERİLMEDİ. `next/og`'a Google Fonts'tan
 * indirilen bir font geçmek derlemeyi ağa bağımlı kılar — ağ yoksa ya da
 * fonts.gstatic.com yavaşsa `next build` düşer. Gömülü varsayılan font
 * (Noto Sans) Türkçe harfleri (ğ ş ı İ ç ö ü) kapsar; çıktı gerçekten
 * üretilip gözle doğrulandı.
 *
 * NEDEN AYRICA `twitter-image.tsx` YOK: Next bu dosyadan hem `og:image` hem
 * `twitter:image` etiketini üretiyor (aynı 1200×630 görsel, `summary_large_image`
 * kartı için doğru oran). İkinci bir dosya, her derlemede aynı görseli bir kez
 * daha rasterleştirmekten başka bir şey yapmazdı. Bu varsayım değil, ölçüm:
 * `web/scripts/seo-check.mjs` her sayfada `twitter:image` etiketinin VARLIĞINI
 * ve görselin 200 döndüğünü ayrıca doğrular.
 */
export const alt = 'Onay360 — Geçici numara ve onaylama hizmetleri';
export const size = { width: 1200, height: 630 };
export const contentType = 'image/png';

const MAVI = '#1a7fd4';
const KIRMIZI = '#e30613';
const ZEMIN = '#0b1120'; // globals.css --color-ink-950

export default function OpengraphImage() {
  return new ImageResponse(
    (
      <div
        style={{
          width: '100%',
          height: '100%',
          display: 'flex',
          flexDirection: 'column',
          justifyContent: 'center',
          gap: 64,
          backgroundColor: ZEMIN,
          backgroundImage: `linear-gradient(135deg, ${ZEMIN} 0%, #10203c 55%, #0b1120 100%)`,
          padding: '72px 80px',
        }}
      >
        {/* Üst şerit: marka işareti + kelime markası */}
        <div style={{ display: 'flex', alignItems: 'center', gap: 28 }}>
          <svg width="128" height="128" viewBox="0 0 40 40">
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
            <span style={{ fontSize: 104, fontWeight: 700, color: MAVI, letterSpacing: -2 }}>
              Onay
            </span>
            <span style={{ fontSize: 104, fontWeight: 700, color: KIRMIZI, letterSpacing: -2 }}>
              360
            </span>
            <span style={{ fontSize: 44, fontWeight: 700, color: KIRMIZI, marginTop: 8 }}>°</span>
          </div>
        </div>

        {/* Alt blok: tagline + ürün sözü */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 26 }}>
          <div style={{ display: 'flex', width: 132, height: 6, backgroundColor: KIRMIZI }} />
          <span
            style={{
              fontSize: 40,
              fontWeight: 700,
              color: '#e5e7eb',
              letterSpacing: 3,
            }}
          >
            GEÇİCİ NUMARA VE ONAYLAMA HİZMETLERİ
          </span>
          <span style={{ fontSize: 32, color: '#91a1b6' }}>
            Yüzlerce servis için anında SMS onay kodu — kod gelmezse ücret iade edilir.
          </span>
        </div>
      </div>
    ),
    size,
  );
}
