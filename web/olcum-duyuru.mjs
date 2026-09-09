/**
 * /yonetim — DUYURU DENETİMİ (aria-live + role="alert").
 *
 * İKİ GEÇİŞ:
 *  A. AÇILIŞ. Sayfa yüklenir yüklenmez DOM'da duran `role="alert"` sayısı.
 *     §7.4: `role="alert"` yalnız YENİ BELİREN hata/başarı içindir; açılışta
 *     duran her kutu ekran okuyucuya kesintili uyarı olarak okunur. Hedef 0.
 *     Ayrıca kaç `aria-live` bölgesinin AYNI ANDA konuştuğu — sayfalı listede
 *     konuşan tek bölge `Sayfalama` olmalıdır (`duyuru={!sayfali}`).
 *  B. HATA. Liste uç noktası 500 döndürüldüğünde tek olay için kaç duyuru
 *     üretildiği. `HataDurumu` zaten `role="alert"`tir; polite bölgenin ayrıca
 *     "Liste yüklenemedi." demesi İKİNCİ ve daha az bilgili duyurudur.
 *
 * Kullanım: cd web && node olcum-duyuru.mjs   (dev sunucusu :3000 açık olmalı)
 */
import { chromium } from 'playwright';

const KOK = 'http://localhost:3000';

const YOLLAR = [
  '/yonetim',
  '/yonetim/bakiye',
  '/yonetim/talepler',
  '/yonetim/kullanicilar',
  '/yonetim/destek',
  '/yonetim/yorumlar',
  '/yonetim/denetim',
  '/yonetim/saglayicilar',
  '/yonetim/fiyatlar',
  '/yonetim/odeme-yontemleri',
];

/** Hata geçişi: [ekran, engellenecek uç nokta]. */
const HATA_SENARYOLARI = [
  ['/yonetim/saglayicilar', '**/api/v1/admin/providers*'], // VeriTablosu yolu
  ['/yonetim/yorumlar', '**/api/v1/admin/reviews*'], // elle canlı bölge yolu
];

const kirp = (s) => (s || '').replace(/\s+/g, ' ').trim().slice(0, 80);

async function oku(sayfa) {
  return sayfa.evaluate(() => {
    const k = (s) => (s || '').replace(/\s+/g, ' ').trim().slice(0, 80);
    return {
      alerts: [...document.querySelectorAll('[role="alert"]')].map((e) => k(e.textContent)),
      live: [...document.querySelectorAll('[aria-live]')].map((e) => ({
        seviye: e.getAttribute('aria-live'),
        metin: k(e.textContent),
      })),
    };
  });
}

const tarayici = await chromium.launch();
const baglam = await tarayici.newContext({ viewport: { width: 1280, height: 900 } });
const sayfa = await baglam.newPage();

await sayfa.goto(`${KOK}/giris`, { waitUntil: 'networkidle' });
await sayfa.fill('input[type="email"]', 'admin@onay360.test');
await sayfa.fill('input[type="password"]', 'dogru-at-pil-zimba');
await sayfa.click('button[type="submit"]');
await sayfa.waitForURL(/\/panel/, { timeout: 20000 });

console.log('\n═══ A. AÇILIŞ — role="alert" hedefi 0 ═══');
let toplamAlert = 0;
for (const yol of YOLLAR) {
  await sayfa.goto(`${KOK}${yol}`, { waitUntil: 'networkidle' });
  await sayfa.waitForTimeout(900);
  const o = await oku(sayfa);
  toplamAlert += o.alerts.length;

  console.log(`\n── ${yol}`);
  console.log(`   role="alert": ${o.alerts.length}  ${o.alerts.length ? '🔴' : '✓'}`);
  o.alerts.forEach((t) => console.log(`      · ${t}`));
  o.live.forEach((l) => console.log(`   [${l.seviye}] "${l.metin}"`));
  const konusan = o.live.filter((l) => l.metin.length > 0);
  if (konusan.length > 1) console.log(`   🔴 ${konusan.length} bölge AYNI ANDA konuşuyor`);
}
console.log(`\n═══ Açılışta toplam role="alert": ${toplamAlert} (hedef 0) ═══`);

console.log('\n═══ B. HATA — tek olay, tek duyuru ═══');
for (const [yol, desen] of HATA_SENARYOLARI) {
  await sayfa.route(desen, (r) =>
    r.fulfill({
      status: 500,
      contentType: 'application/json',
      body: JSON.stringify({ code: 'INTERNAL', message: 'Sunucu hatası.', requestId: 'req-test-1' }),
    }),
  );
  await sayfa.goto(`${KOK}${yol}`, { waitUntil: 'networkidle' });
  // TanStack Query varsayılan olarak 3 kez yeniden dener; hata durumu ~10sn
  // sonra ekrana gelir. Kısa beklemede iskelet ölçülür, hata değil.
  await sayfa.waitForTimeout(12000);
  const o = await oku(sayfa);
  const duyuruSayisi = o.alerts.length + o.live.filter((l) => l.metin.length > 0).length;

  console.log(`\n── ${yol}  (uç nokta 500)`);
  console.log(`   role="alert": ${JSON.stringify(o.alerts)}`);
  console.log(`   aria-live   : ${JSON.stringify(o.live.map((l) => l.metin))}`);
  console.log(`   tek olay için duyuru: ${duyuruSayisi}  ${duyuruSayisi > 1 ? '🔴 ÇİFT' : '✓'}`);
  await sayfa.unroute(desen);
}

console.log();
await tarayici.close();
