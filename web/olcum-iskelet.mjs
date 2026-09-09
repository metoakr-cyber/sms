/* Skeleton açık temada görünüyor mu — GERÇEK render'dan piksel okuyarak. */
import { chromium } from 'playwright';
const t = await chromium.launch();
const p = await (await t.newContext({ viewport: { width: 1280, height: 900 }, locale: 'tr-TR', colorScheme: 'light' })).newPage();
await p.goto('http://localhost:3000/giris', { waitUntil: 'networkidle' });
await p.fill('input[type="email"]', 'admin@onay360.test');
await p.fill('input[type="password"]', 'dogru-at-pil-zimba');
await p.click('button[type="submit"]');
await p.waitForURL('**/panel**');
// Listeyi yavaşlat ki iskelet ekranda kalsın.
await p.route('**/api/v1/admin/users*', async (r) => { await new Promise((s) => setTimeout(s, 8000)); r.continue(); });
await p.goto('http://localhost:3000/yonetim/kullanicilar', { waitUntil: 'domcontentloaded' });
await p.waitForTimeout(1500);
const d = await p.evaluate(() => {
  const sk = document.querySelector('.animate-pulse');
  if (!sk) return { yok: true, iskeletSayi: 0 };
  const kap = sk.parentElement;
  const oku = (e) => { let n = e; while (n) { const b = getComputedStyle(n).backgroundColor;
    if (b && b !== 'rgba(0, 0, 0, 0)' && b !== 'transparent') return b; n = n.parentElement; } return 'yok'; };
  const rgb = (s) => (s.match(/\d+/g) || []).slice(0, 3).map(Number);
  const L = (c) => { const [r, g, b] = rgb(c).map((v) => { v /= 255; return v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4; });
    return 0.2126 * r + 0.7152 * g + 0.0722 * b; };
  const a = getComputedStyle(sk).backgroundColor, b = oku(kap);
  const o = (Math.max(L(a), L(b)) + 0.05) / (Math.min(L(a), L(b)) + 0.05);
  return { iskeletSayi: document.querySelectorAll('.animate-pulse').length, iskelet: a, zemin: b, oran: Math.round(o * 100) / 100 };
});
console.log('AÇIK TEMA:', JSON.stringify(d));
await p.screenshot({ path: '/tmp/regresyon-768/iskelet-acik-1280.png' });
await t.close();
