import type { MetadataRoute } from 'next';

/**
 * Web uygulaması manifesti — "ana ekrana ekle".
 *
 * `start_url` PANELDİR, ana sayfa değil: simgeye dokunan kişi zaten
 * müşteridir, pazarlama sayfasını değil numara alma ekranını ister. Oturum
 * yoksa `/panel` zaten `/giris`e yönlendirir, dolayısıyla açılış her iki
 * durumda da doğru yere gider.
 *
 * Simge yolları `icon.tsx`'in ürettiği rotalardır (`/icon/<boyut>`). Chrome
 * kurulabilirlik için 192 ve 512 px PNG arar; 32'lik favicon tek başına
 * "ana ekrana ekle" önerisini TETİKLEMEZ.
 *
 * test: web/scripts/seo-check.mjs  (manifest bölümü: 192x192 ve 512x512
 * simgelerin BİLDİRİLDİĞİNİ ve her simge URL'inin 200 + image/* döndüğünü
 * doğrular; biri düşerse denetim kırmızı olur)
 */
export default function manifest(): MetadataRoute.Manifest {
  return {
    id: '/',
    name: 'Onay360 — Geçici Numara ve SMS Onayı',
    short_name: 'Onay360',
    description:
      'Yüzlerce servis için geçici numara ile anında SMS onay kodu. ' +
      'Kod gelmezse ücret iade edilir.',
    lang: 'tr',
    dir: 'ltr',
    start_url: '/panel',
    scope: '/',
    display: 'standalone',
    // Koyu VARSAYILANDIR (globals.css): açılış ekranı ile uygulamanın ilk
    // boyaması aynı renk olmalı, yoksa her açılışta beyaz patlama görülür.
    background_color: '#0b1120',
    theme_color: '#0b1120',
    orientation: 'portrait-primary',
    categories: ['utilities', 'productivity'],
    icons: [
      { src: '/icon/192', sizes: '192x192', type: 'image/png', purpose: 'any' },
      { src: '/icon/512', sizes: '512x512', type: 'image/png', purpose: 'any' },
      // Aynı PNG'ler maskeli olarak da bildiriliyor. Maskeli simgede Android
      // kenardan ~%10 kırpar; `icon.tsx` motifi kenarın %62'sinde tutuyor,
      // yani her yönde ~%19 boşluk bırakıyor. 192 px çıktısı daire ve
      // squircle maskesi altında GÖZLE de kontrol edildi.
      { src: '/icon/192', sizes: '192x192', type: 'image/png', purpose: 'maskable' },
      { src: '/icon/512', sizes: '512x512', type: 'image/png', purpose: 'maskable' },
    ],
    shortcuts: [
      { name: 'Numara al', url: '/panel/numara-al' },
      { name: 'Siparişlerim', url: '/panel/siparisler' },
      { name: 'Bakiye yükle', url: '/panel/bakiye-yukle' },
    ],
  };
}
