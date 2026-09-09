import { defineConfig, devices } from 'playwright/test';

/**
 * Playwright yapılandırması — docs/frontend-contract.md §7.1, docs/trd.md NFR-809/KK-809.
 *
 * Sunucuları BU DOSYA BAŞLATMAZ. `webServer` bilerek kullanılmıyor: uçtan uca
 * akış hem Go API'sini hem Next sunucusunu ister, ikisinin sırası ve hazır
 * olma koşulu birbirine bağlıdır ve bu iş `scripts/e2e.sh` içinde tek yerde
 * duruyor. İki yerde iki farklı başlatma mantığı olsaydı, biri düzeltilip
 * diğeri unutulurdu.
 *
 * Çalıştırma:  ./scripts/e2e.sh
 */

const BASE_URL = process.env.E2E_BASE_URL ?? 'http://localhost:3100';

export default defineConfig({
  testDir: './e2e',
  globalSetup: './e2e/global-setup.ts',

  /*
    Tüm kanıt TEK YERDE: `.e2e-logs/`.

    İz, ekran görüntüsü ve HTML rapor, sunucu log'larıyla AYNI dizinde durur —
    düşen bir tarayıcı testinde ikisine birden bakılır. Dağınık olsalardı
    (web/test-results + .e2e-logs) biri kopyalanır, diğeri unutulurdu; CI
    kanıt paketi de bu yüzden tek yolu topluyor. `scripts/check.sh`in
    `.check-logs` deseniyle aynı.
  */
  outputDir: '../.e2e-logs/test-results',

  /*
    TEK İŞÇİ, PARALELLİK KAPALI — bu bir performans tercihi değil, doğruluk şartı.

    1) Dört proje de AYNI veritabanına ve AYNI API sürecine konuşur; testler
       birbirinin kaydını değil ama aynı sayaçları paylaşır.
    2) /auth uçlarında IP başına 30 istek/dakika limiti var (router.go authLimit).
       Paralel işçiler kayıt+doğrulama+giriş isteklerini aynı dakikaya yığar ve
       testler 429 ile, ürün hatası yokken düşer.

    Süre uzuyor; ama rastgele kırmızıya dönen bir takım kimsenin bakmadığı bir
    takıma dönüşür.
  */
  fullyParallel: false,
  workers: 1,

  /*
    YENİDEN DENEME YOK.

    `retries: 1` kararsız bir testi yeşile boyar ve kararsızlık ancak aylar
    sonra, hata ayıklanamayacak bir anda ortaya çıkar. Bir test düşüyorsa ya
    üründe ya testte gerçek bir sorun vardır; ikisi de bakılmayı hak eder.
  */
  retries: 0,
  forbidOnly: !!process.env.CI,

  timeout: 90_000,
  expect: { timeout: 10_000 },

  reporter: process.env.CI
    ? [['github'], ['list'], ['html', { open: 'never', outputFolder: '../.e2e-logs/rapor' }]]
    : [['list'], ['html', { open: 'never', outputFolder: '../.e2e-logs/rapor' }]],

  use: {
    baseURL: BASE_URL,
    // Kanıt YALNIZ başarısızlıkta saklanır: her koşuda video/iz üretmek
    // diski doldurur ve gürültüde gerçek başarısızlık kaybolur.
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
    actionTimeout: 15_000,
    navigationTimeout: 30_000,
    locale: 'tr-TR',
    timezoneId: 'Europe/Istanbul',
  },

  /*
    DÖRT PROJE (frontend-contract.md §7.1).

    webkit-mobile EN KRİTİK OLANIDIR: iOS'ta tüm tarayıcılar WebKit motorunu
    kullanır, dolayısıyla "Chrome'da çalışıyor" iOS için hiçbir şey söylemez.
  */
  projects: [
    {
      name: 'chromium-desktop',
      use: { ...devices['Desktop Chrome'], viewport: { width: 1440, height: 900 } },
    },
    {
      name: 'webkit-desktop',
      use: { ...devices['Desktop Safari'], viewport: { width: 1440, height: 900 } },
    },
    {
      name: 'chromium-mobile',
      use: { ...devices['Pixel 7'] },
    },
    {
      name: 'webkit-mobile',
      use: { ...devices['iPhone 14'] },
    },
  ],
});
