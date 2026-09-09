/**
 * /panel (Özet) — AKTİF SİPARİŞ SATIRLARININ görsel doğrulaması.
 *
 * Neden ağ katmanında: yük hesabının 32 siparişinin tamamı terminal durumda
 * (hiçbiri PENDING/ACTIVE değil) ve gerçek bir sipariş açmak PARA HARCAR ve
 * SAĞLAYICIYA `Purchase` çağrısı yapar. Bu betik yalnız YANITI yeniden yazar:
 * sunucuda hiçbir şey değişmez, yalnız ekranın o satırları nasıl çizdiği
 * görülür (logo, rozet, numara, "Siparişi aç").
 */
import { chromium } from 'playwright';
import fs from 'node:fs';

const KOK = 'http://localhost:3000';
const CIKTI = '/tmp/panel-ozet';
fs.mkdirSync(CIKTI, { recursive: true });
const MUSTERI = { email: 'yuk-1788931064-1@yuk.test', sifre: 'yuk-testi-parolasi-uzun' };

const tarayici = await chromium.launch();
const baglam = await tarayici.newContext({ viewport: { width: 1280, height: 900 } });
const sayfa = await baglam.newPage();
const konsol = [];
sayfa.on('console', (m) => { if (m.type() === 'error') konsol.push(m.text()); });
sayfa.on('pageerror', (e) => konsol.push(`pageerror: ${e.message}`));

await sayfa.goto(`${KOK}/giris`, { waitUntil: 'networkidle' });
await sayfa.fill('input[type="email"]', MUSTERI.email);
await sayfa.fill('input[type="password"]', MUSTERI.sifre);
await sayfa.click('button[type="submit"]');
await sayfa.waitForURL(/\/panel/, { timeout: 20000 });

// İlk üç siparişi aktif durumlara çevir + logolu servis kodları ver.
await sayfa.route('**/api/v1/orders?*', async (route) => {
  const yanit = await route.fetch();
  const g = await yanit.json();
  const kalip = [
    { status: 'PENDING', iconUrl: '/servis-logolari/wa.svg', serviceName: 'WhatsApp' },
    { status: 'PENDING', iconUrl: '/servis-logolari/tg.svg', serviceName: 'Telegram' },
    { status: 'ACTIVE', iconUrl: '', serviceName: 'Google Voice' },
  ];
  g.items = g.items.map((o, i) =>
    i < kalip.length ? { ...o, ...kalip[i], expiresAt: new Date(Date.now() + 9e5).toISOString() } : o,
  );
  await route.fulfill({ response: yanit, json: g });
});

for (const g of [390, 768, 1024, 1280]) {
  await sayfa.setViewportSize({ width: g, height: g < 500 ? 844 : 1000 });
  await sayfa.goto(`${KOK}/panel`, { waitUntil: 'networkidle' });
  await sayfa.waitForTimeout(900);

  const o = await sayfa.evaluate(() => {
    const d = document.documentElement;
    const rozetler = [...document.querySelectorAll('main span')]
      .filter((s) => /Kod bekleniyor|Kiralama sürüyor|Bilinmeyen/.test(s.textContent) && s.children.length <= 2)
      .map((s) => s.textContent.trim());
    return {
      scrollWidth: d.scrollWidth, clientWidth: d.clientWidth,
      img: [...document.querySelectorAll('main img')].map((i) => i.getAttribute('src')),
      harfRozeti: [...document.querySelectorAll('main span[class*="place-items-center"] span')].length,
      rozetler: [...new Set(rozetler)],
      satir: document.querySelectorAll('main tbody tr').length,
      kart: document.querySelectorAll('main ul.md\\:hidden > li').length,
      eylem: [...document.querySelectorAll('main a[href="/panel/siparisler"] button')].map((b) => ({
        m: b.textContent.trim(), w: Math.round(b.getBoundingClientRect().width), h: Math.round(b.getBoundingClientRect().height),
      })),
      basliklar: [...document.querySelectorAll('main th')].map((t) => t.textContent.trim()),
    };
  });
  console.log(`${g}px`, JSON.stringify(o, null, 1));
  await sayfa.screenshot({ path: `${CIKTI}/aktif-${g}.png`, fullPage: true });
}

console.log('konsol:', konsol.length ? konsol.join(' | ') : 'yok');
await tarayici.close();
