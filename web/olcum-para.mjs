/**
 * Para ekranları (/panel/cuzdan · /panel/bakiye-yukle) — ölçüm + ekran görüntüsü.
 * Kullanım: node olcum-para.mjs [etiket]
 */
import { chromium } from 'playwright';
import fs from 'node:fs';

const ETIKET = process.argv[2] ?? 'sonra';
const KOK = 'http://localhost:3000';
const CIKTI = '/tmp/panel-para';
fs.mkdirSync(CIKTI, { recursive: true });

const MUSTERI = { email: 'yuk-1788931064-1@yuk.test', sifre: 'yuk-testi-parolasi-uzun' };
const YOLLAR = ['/panel/cuzdan', '/panel/bakiye-yukle'];
const GENISLIKLER = [320, 390, 768, 1280];

const tarayici = await chromium.launch();
const baglam = await tarayici.newContext({ viewport: { width: 1280, height: 900 } });
const sayfa = await baglam.newPage();

const konsol = [];
sayfa.on('console', (m) => { if (m.type() === 'error') konsol.push(m.text()); });
sayfa.on('pageerror', (e) => konsol.push(`pageerror: ${e.message}`));

const istekler = [];
sayfa.on('request', (r) => {
  const u = r.url();
  if (u.includes('/api/v1/wallet/')) istekler.push(u.replace(KOK, ''));
});

await sayfa.goto(`${KOK}/giris`, { waitUntil: 'networkidle' });
await sayfa.fill('input[type="email"]', MUSTERI.email);
await sayfa.fill('input[type="password"]', MUSTERI.sifre);
await sayfa.click('button[type="submit"]');
await sayfa.waitForURL(/\/panel/, { timeout: 20000 });
console.log('giriş: tamam');

const satirlar = [];

for (const yol of YOLLAR) {
  for (const g of GENISLIKLER) {
    await sayfa.setViewportSize({ width: g, height: g < 500 ? 844 : 900 });
    await sayfa.goto(`${KOK}${yol}`, { waitUntil: 'networkidle' });
    await sayfa.waitForTimeout(800);

    const o = await sayfa.evaluate(() => {
      const d = document.documentElement;
      const kucuk = [];
      for (const el of document.querySelectorAll('a,button,select,input,[role="menuitem"]')) {
        const r = el.getBoundingClientRect();
        if (r.width === 0 || r.height === 0) continue;
        const st = getComputedStyle(el);
        if (st.visibility === 'hidden' || st.display === 'none') continue;
        if (el.type === 'radio' || el.type === 'checkbox') continue; // etiketi hedeftir
        if (r.height < 43.5 || r.width < 43.5) {
          kucuk.push(`${el.tagName.toLowerCase()}"${(el.textContent || '').trim().slice(0, 20)}" ${Math.round(r.width)}x${Math.round(r.height)}`);
        }
      }
      const kucukGirdi = [];
      for (const el of document.querySelectorAll('input,select,textarea')) {
        const f = parseFloat(getComputedStyle(el).fontSize);
        if (f < 15.9) kucukGirdi.push(`${el.tagName.toLowerCase()}[${el.type || ''}] ${f}px`);
      }
      // 14px altı METİN (rozet hariç) — §3.2
      const kucukMetin = [];
      for (const el of document.querySelectorAll('main *')) {
        if (el.children.length > 0) continue;
        const t = (el.textContent || '').trim();
        if (!t) continue;
        const f = parseFloat(getComputedStyle(el).fontSize);
        if (f < 13.9) {
          const rozet = el.closest('span.inline-flex.rounded-lg, .rounded-lg.border');
          kucukMetin.push(`${Math.round(f)}px "${t.slice(0, 26)}"${rozet ? ' (rozet?)' : ''}`);
        }
      }
      const tablo = document.querySelector('main table');
      const tabloGorunur = tablo ? getComputedStyle(tablo.closest('div')).display !== 'none' : false;
      return {
        scrollW: d.scrollWidth,
        clientW: d.clientWidth,
        ariaCurrent: document.querySelectorAll('[aria-current="page"]').length,
        kucuk, kucukGirdi,
        kucukMetin: kucukMetin.slice(0, 6),
        tabloVar: !!tablo,
        tabloGorunur,
        kartVar: !!document.querySelector('main ul[aria-label]'),
        caption: tablo?.querySelector('caption')?.textContent?.trim() ?? null,
        canliBolge: document.querySelectorAll('main [aria-live]').length,
        tabularNums: document.querySelectorAll('main .tabular-nums').length,
        overflowX: [...document.querySelectorAll('main *')]
          .filter((e) => ['auto', 'scroll'].includes(getComputedStyle(e).overflowX))
          .map((e) => e.tagName.toLowerCase() + '.' + (e.className || '').toString().slice(0, 30)),
        h1: document.querySelector('main h1')?.textContent?.trim() ?? null,
      };
    });

    satirlar.push({ yol, g, ...o });
    const ad = `${CIKTI}/${ETIKET}-${yol.split('/').pop()}-${g}.png`;
    await sayfa.screenshot({ path: ad, fullPage: true });
  }
}

console.log('\n| yol | px | scrollW/clientW | aria-current | tablo | kart | caption | aria-live | tabular | overflow-x | <44px hedef | <16px girdi | <14px metin |');
console.log('|---|---|---|---|---|---|---|---|---|---|---|---|---|');
for (const s of satirlar) {
  console.log(`| ${s.yol} | ${s.g} | ${s.scrollW}/${s.clientW} ${s.scrollW > s.clientW ? '🔴' : '✓'} | ${s.ariaCurrent} | ${s.tabloGorunur ? 'görünür' : s.tabloVar ? 'gizli' : 'yok'} | ${s.kartVar ? 'var' : 'yok'} | ${s.caption ? '✓' : '🔴'} | ${s.canliBolge} | ${s.tabularNums} | ${s.overflowX.length ? '🔴 ' + s.overflowX.join(',') : '✓ yok'} | ${s.kucuk.length ? '🔴 ' + s.kucuk.join(' · ') : '✓'} | ${s.kucukGirdi.length ? '🔴 ' + s.kucukGirdi.join(' · ') : '✓'} | ${s.kucukMetin.length ? '🔴 ' + s.kucukMetin.join(' · ') : '✓'} |`);
}

console.log('\nwallet istekleri:');
console.log([...new Set(istekler)].join('\n'));
console.log('\nkonsol hataları:', konsol.length ? konsol : 'yok');
console.log('görüntüler:', CIKTI);

await tarayici.close();
