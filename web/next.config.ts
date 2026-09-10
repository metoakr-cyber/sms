import type { NextConfig } from 'next';

// Üretimde ters vekil `/api`'yi Go'ya taşır (tek alan adı — ADR: design.md §3).
// Geliştirmede aynı topolojiyi Next'in rewrite'ı ile TAKLİT EDERİZ; böylece
// çerezler, SameSite davranışı ve göreli yollar üretimle birebir aynı çalışır.
// Ayrı port + CORS ile geliştirmek, üretimde olmayan bir dünyada geliştirmektir.
const API_ORIGIN = process.env.API_ORIGIN ?? 'http://127.0.0.1:8091';

// ─────────────────────── İçerik Güvenlik Politikası (CSP) ───────────────────────
//
// CSP'nin TEK kaynağı burasıdır. Caddy aynı başlığı yalnız "yoksa" ekler
// (deploy/Caddyfile içindeki `?Content-Security-Policy`), böylece Next'in
// gönderdiği politika kazanır ve iki yerde iki farklı politika tutulmaz.
// İki kaynaklı bir başlık, birini güncelleyip diğerini unuttuğunuz gün
// sessizce çelişir.
//
// NEDEN script-src'te 'unsafe-inline' — kaldırılamadı, bilinçli gevşetildi:
//   • src/app/layout.tsx: tema betiği ilk boyamadan ÖNCE satır içi çalışmak
//     zorunda (aşağı taşınırsa beyaz patlama geri gelir).
//   • src/app/(genel)/page.tsx · kiralama/page.tsx · sss/page.tsx: JSON-LD
//     yapısal veri blokları satır içi <script type="application/ld+json">.
//   • Next.js App Router kendi hidrasyon verisini (self.__next_f.push)
//     satır içi betikle gönderir.
// Bunlar nonce ile kapatılabilir, ama nonce her istekte üretilip middleware
// üzerinden akmak zorundadır ve statik sayfa önbelleklemesini bozar. Yanlış
// kurulmuş bir CSP kayıt formunu SESSİZCE çalışmaz hâle getirir — bu projede
// bir kez yaşanmış hata sınıfı (docs/memory.md §3.10). Olmayan bir CSP'den
// kötü olan tek şey, sayfayı kıran bir CSP'dir.
// Karşılığında yüzey daraltıldı: object-src 'none', base-uri 'self',
// form-action 'self', frame-ancestors 'none'.
//
// NEDEN style-src'te 'unsafe-inline': next/font satır içi <style> yazar;
// Tailwind'in kritik CSS'i ve React'in style özniteliği de aynı yoldan geçer.
//
// NEDEN www.google.com / www.gstatic.com: reCAPTCHA v2 —
// src/components/recaptcha.tsx betiği google.com'dan yükler, onay kutusunu
// bir iframe içinde çizer, görsellerini gstatic'ten alır.
//
// NEDEN 'unsafe-eval' YALNIZ geliştirmede: `next dev` sıcak yeniden yükleme
// için eval kullanır. Üretim derlemesinde bu parça YOKTUR.
const gelistirme = process.env.NODE_ENV !== 'production';
const csp = [
  "default-src 'self'",
  `script-src 'self' 'unsafe-inline'${gelistirme ? " 'unsafe-eval'" : ''} https://www.google.com https://www.gstatic.com`,
  "style-src 'self' 'unsafe-inline'",
  // blob: ZORUNLU — dekont görüntüleyici. Dekont ucu ikili dosya döner ve
  // apiBlob() `URL.createObjectURL` ile blob: şeması üretir; blob: olmadan
  // yönetici dekontu GÖREMEDEN parayı onaylamak zorunda kalır.
  // blob: yalnız SAYFANIN KENDİ ürettiği veriye işaret eder, dışarıdan
  // yüklenemez — data: kadar bile geniş değildir.
  "img-src 'self' data: blob: https://www.google.com https://www.gstatic.com",
  "font-src 'self' data:",
  // SSE dâhil tüm API çağrıları AYNI kökene gider (ters vekil topolojisi),
  // bu yüzden connect-src genişletilmez.
  "connect-src 'self'",
  "frame-src https://www.google.com",
  "worker-src 'self' blob:",
  // object-src: PDF dekontlar <object> ile gösteriliyor. blob: SAYFANIN KENDİ
  // ürettiği veriye işaret eder (apiBlob → URL.createObjectURL), dışarıdan
  // yüklenemez. 'none' bırakılırsa yönetici PDF dekontu GÖREMEDEN parayı
  // onaylamak zorunda kalır — para sisteminde kabul edilemez bir körlük.
  "object-src blob:",
  "base-uri 'self'",
  "form-action 'self'",
  "frame-ancestors 'none'",
].join('; ');

const config: NextConfig = {
  // Üretim imajı `.next/standalone/server.js`'i çalıştırır: Next yalnız
  // gerçekten kullanılan modülleri kopyalar, imaja bütün node_modules
  // taşınmaz. `next dev`, `next start` ve check.sh'in NEXT_DIST_DIR akışı
  // ETKİLENMEZ — standalone EK bir çıktıdır, mevcut olanın yerine geçmez.
  output: 'standalone',
  // Derleme çıktısı dizini AYRILABİLİR olmalı.
  //
  // `npm run build` ile `npm run dev` aynı `.next` dizinini paylaşır. Kontroller
  // (check.sh) geliştirme sunucusu ayaktayken koşarsa üretim derlemesi dev
  // sunucusunun parçalarını ezer ve tarayıcıda
  // "__webpack_modules__[moduleId] is not a function" hatası çıkar — kodda
  // hiçbir sorun yokken. check.sh bu yüzden NEXT_DIST_DIR ile ayrı bir dizine
  // derler.
  distDir: process.env.NEXT_DIST_DIR ?? '.next',
  // Next'in geliştirme rozetini (sol altta duran "N" göstergesi) kapatır.
  //
  // Sürüm notu: `buildActivity` ve `appIsrStatus` alanları 15.2'de kullanım
  // dışı bırakıldı; bu sürümde tek kapatma yolu `false` vermektir
  // (next/dist/server/config-shared.d.ts:891 — "To disable, set
  // `devIndicators` to `false`"). Eski alanları yazmak sessizce etkisiz kalır.
  //
  // Yalnız geliştirmeyi etkiler; üretim derlemesinde bu gösterge zaten yoktur.
  devIndicators: false,
  reactStrictMode: true,
  poweredByHeader: false,
  async rewrites() {
    return [{ source: '/api/:path*', destination: `${API_ORIGIN}/api/:path*` }];
  },
  async headers() {
    return [
      {
        source: '/:path*',
        headers: [
          { key: 'X-Content-Type-Options', value: 'nosniff' },
          { key: 'Referrer-Policy', value: 'strict-origin-when-cross-origin' },
          { key: 'X-Frame-Options', value: 'DENY' },
          { key: 'Permissions-Policy', value: 'camera=(), microphone=(), geolocation=()' },
          { key: 'Content-Security-Policy', value: csp },
        ],
      },
      // Panel ve yönetim ASLA indekslenmez — frontend-contract.md §10.1.
      // Bu başlık `robots.txt`'e EK bir savunmadır: robots.txt yalnız taramayı
      // engeller, zaten bilinen bir URL'in indekslenmesini engellemez.
      { source: '/panel/:path*', headers: [{ key: 'X-Robots-Tag', value: 'noindex, nofollow' }] },
      { source: '/yonetim/:path*', headers: [{ key: 'X-Robots-Tag', value: 'noindex, nofollow' }] },
    ];
  },
};

export default config;
