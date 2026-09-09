/** Oturumlu görsel kontrol: yönetim + müşteri paneli, iki tema. */
import { chromium } from 'playwright';

const CIKTI = '/private/tmp/claude-501/-Users-morepayroll-Desktop-opencart3x-extension-smsEntegrasyon/96f93d50-0fe8-49fa-bca9-aa862db0e2d1/scratchpad';
const KOK = 'http://localhost:3000';
const tarayici = await chromium.launch();

async function girisYap(sayfa, e, p) {
  await sayfa.goto(`${KOK}/giris`, { waitUntil: 'networkidle' });
  await sayfa.fill('input[name="email"]', e);
  await sayfa.fill('input[name="password"]', p);
  await sayfa.click('form button[type="submit"]');
  await sayfa.waitForURL(/\/(yonetim|panel)/, { timeout: 25000 }).catch(() => {});
  await sayfa.waitForTimeout(1500);
}

const isler = [
  ['yonetici', 'admin@onay360.test', 'dogru-at-pil-zimba',
   [['/yonetim/talepler', 1440], ['/yonetim/bakiye', 1440], ['/yonetim/saglayicilar', 390]]],
  ['musteri', 'yuk-1788931064-1@yuk.test', 'yuk-testi-parolasi-uzun',
   [['/panel/siparisler', 1440], ['/panel/siparisler', 390], ['/panel/destek', 390]]],
];

for (const [kim, e, p, yollar] of isler) {
  const baglam = await tarayici.newContext({ viewport: { width: 1440, height: 950 } });
  const sayfa = await baglam.newPage();
  await girisYap(sayfa, e, p);
  for (const [yol, w] of yollar) {
    await sayfa.setViewportSize({ width: w, height: w < 500 ? 900 : 950 });
    await sayfa.goto(KOK + yol, { waitUntil: 'networkidle' });
    await sayfa.waitForTimeout(1600);
    for (const tema of ['dark', 'light']) {
      await sayfa.evaluate((t) => document.documentElement.setAttribute('data-theme', t), tema);
      await sayfa.waitForTimeout(250);
      const ad = `${kim}-${yol.replace(/\//g, '_')}-${w}-${tema}`;
      await sayfa.screenshot({ path: `${CIKTI}/gorsel2-${ad}.png` });
      const tasma = await sayfa.evaluate(() =>
        document.documentElement.scrollWidth - document.documentElement.clientWidth);
      console.log(ad, 'yatay tasma:', tasma);
    }
  }
  await baglam.close();
}
await tarayici.close();
