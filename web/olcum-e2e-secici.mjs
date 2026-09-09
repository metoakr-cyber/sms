/* Önerilen düzeltmenin TÜM akışta çalıştığını doğrular (yalnız ilk alanda değil). */
import { chromium } from 'playwright';
const t = await chromium.launch();
const p = await (await t.newContext({ viewport: { width: 1440, height: 900 }, locale: 'tr-TR' })).newPage();
await p.goto('http://localhost:3000/giris', { waitUntil: 'networkidle' });
await p.fill('input[type="email"]', 'admin@onay360.test');
await p.fill('input[type="password"]', 'dogru-at-pil-zimba');
await p.click('button[type="submit"]');
await p.waitForURL('**/panel**');
await p.goto('http://localhost:3000/yonetim/fiyatlar', { waitUntil: 'networkidle' });
await p.waitForTimeout(1000);

const secim = (ad) => p.getByLabel(ad, { exact: true });   // ÖNERİLEN yardımcı
await secim('Kapsam').selectOption('SERVICE_COUNTRY');
await p.waitForTimeout(600);
console.log('Kapsam seçildi ✓');
console.log("getByLabel('Servis') →", await secim('Servis').count());
console.log("getByLabel('Ülke')  →", await secim('Ülke').count());
await secim('Servis').selectOption('wa');
await secim('Ülke').selectOption('TR');
await p.waitForTimeout(800);
console.log('Servis+Ülke seçildi ✓');
console.log("getByLabel('Marj (%)') →", await p.getByLabel('Marj (%)').count());
console.log("getByLabel('Not') →", await secim('Not').count());
console.log("'Satış fiyatı' + kardeş p →",
  await p.getByText('Satış fiyatı', { exact: true }).locator('xpath=following-sibling::p[1]').first().textContent()
    .catch((e) => 'HATA ' + String(e).slice(0, 60)));
await t.close();
