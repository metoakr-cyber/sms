/** Görsel kontrol: genel sayfalardaki formlar, iki tema, iki genişlik. */
import { chromium } from 'playwright';

const ETIKET = process.argv[2] ?? 'sonra';
const CIKTI = '/private/tmp/claude-501/-Users-morepayroll-Desktop-opencart3x-extension-smsEntegrasyon/96f93d50-0fe8-49fa-bca9-aa862db0e2d1/scratchpad';
const KOK = 'http://localhost:3000';

const tarayici = await chromium.launch();

for (const [ad, w, h] of [['masaustu', 1280, 900], ['mobil', 390, 844]]) {
  const baglam = await tarayici.newContext({ viewport: { width: w, height: h } });
  const sayfa = await baglam.newPage();
  for (const yol of ['/giris', '/kayit']) {
    await sayfa.goto(KOK + yol, { waitUntil: 'networkidle' });
    await sayfa.waitForTimeout(600);
    for (const tema of ['dark', 'light']) {
      await sayfa.evaluate((t) => document.documentElement.setAttribute('data-theme', t), tema);
      await sayfa.waitForTimeout(200);
      await sayfa.screenshot({ path: `${CIKTI}/gorsel-${ETIKET}-${yol.slice(1)}-${ad}-${tema}.png` });
    }
  }
  await baglam.close();
}

// Hata durumu: alan kenarlığı kırmızıya dönüyor mu, hata metni okunuyor mu?
for (const tema of ['dark', 'light']) {
  const baglam = await tarayici.newContext({ viewport: { width: 1280, height: 900 } });
  const sayfa = await baglam.newPage();
  await sayfa.goto(`${KOK}/giris`, { waitUntil: 'networkidle' });
  await sayfa.evaluate((t) => document.documentElement.setAttribute('data-theme', t), tema);
  await sayfa.fill('input[name="email"]', 'gecersiz');
  await sayfa.fill('input[name="password"]', 'x');
  await sayfa.click('form button[type="submit"]');
  await sayfa.waitForTimeout(2500);
  const durum = await sayfa.evaluate(() => {
    const el = document.querySelector('input[aria-invalid="true"]');
    const p = document.querySelector('p[role="alert"]');
    return {
      hataliAlanVar: !!el,
      kenarRengi: el ? getComputedStyle(el).borderTopColor : null,
      hataMetniRengi: p ? getComputedStyle(p).color : null,
      hataMetniBoyutu: p ? getComputedStyle(p).fontSize : null,
      hataMetni: p ? p.textContent.slice(0, 40) : null,
    };
  });
  console.log(tema, JSON.stringify(durum));
  await sayfa.screenshot({ path: `${CIKTI}/gorsel-${ETIKET}-hata-${tema}.png` });
  await baglam.close();
}

await tarayici.close();
