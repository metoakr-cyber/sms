import type { NextConfig } from 'next';

// Üretimde ters vekil `/api`'yi Go'ya taşır (tek alan adı — ADR: design.md §3).
// Geliştirmede aynı topolojiyi Next'in rewrite'ı ile TAKLİT EDERİZ; böylece
// çerezler, SameSite davranışı ve göreli yollar üretimle birebir aynı çalışır.
// Ayrı port + CORS ile geliştirmek, üretimde olmayan bir dünyada geliştirmektir.
const API_ORIGIN = process.env.API_ORIGIN ?? 'http://127.0.0.1:8091';

const config: NextConfig = {
  // Derleme çıktısı dizini AYRILABİLİR olmalı.
  //
  // `npm run build` ile `npm run dev` aynı `.next` dizinini paylaşır. Kontroller
  // (check.sh) geliştirme sunucusu ayaktayken koşarsa üretim derlemesi dev
  // sunucusunun parçalarını ezer ve tarayıcıda
  // "__webpack_modules__[moduleId] is not a function" hatası çıkar — kodda
  // hiçbir sorun yokken. check.sh bu yüzden NEXT_DIST_DIR ile ayrı bir dizine
  // derler.
  distDir: process.env.NEXT_DIST_DIR ?? '.next',
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
