/**
 * /panel (Özet) — ölçüm ve ekran görüntüsü.
 * Kullanım: node olcum-ozet.mjs
 */
import { chromium } from 'playwright';
import fs from 'node:fs';

const KOK = 'http://localhost:3000';
const CIKTI = '/tmp/panel-ozet';
fs.mkdirSync(CIKTI, { recursive: true });

const HESAPLAR = [
  { ad: 'musteri', email: 'yuk-1788931064-1@yuk.test', sifre: 'yuk-testi-parolasi-uzun' },
  { ad: 'yonetici', email: 'admin@onay360.test', sifre: 'dogru-at-pil-zimba' },
];
const GENISLIKLER = [320, 390, 768, 1024, 1280, 1440];

const tarayici = await chromium.launch();

for (const hesap of HESAPLAR) {
  const baglam = await tarayici.newContext({ viewport: { width: 1280, height: 900 } });
  const sayfa = await baglam.newPage();
  const konsol = [];
  sayfa.on('console', (m) => { if (m.type() === 'error') konsol.push(m.text()); });
  sayfa.on('pageerror', (e) => konsol.push(`pageerror: ${e.message}`));

  await sayfa.goto(`${KOK}/giris`, { waitUntil: 'networkidle' });
  await sayfa.fill('input[type="email"]', hesap.email);
  await sayfa.fill('input[type="password"]', hesap.sifre);
  await sayfa.click('button[type="submit"]');
  await sayfa.waitForURL(/\/panel/, { timeout: 20000 });
  console.log(`\n=== ${hesap.ad} — giriş tamam ===`);

  for (const g of GENISLIKLER) {
    await sayfa.setViewportSize({ width: g, height: g < 500 ? 844 : 900 });
    await sayfa.goto(`${KOK}/panel`, { waitUntil: 'networkidle' });
    await sayfa.waitForTimeout(800);

    const o = await sayfa.evaluate(() => {
      const d = document.documentElement;
      const kucuk = [];
      for (const el of document.querySelectorAll('a,button,select,input,[role="menuitem"]')) {
        const r = el.getBoundingClientRect();
        if (r.width === 0 || r.height === 0) continue;
        const st = getComputedStyle(el);
        if (st.visibility === 'hidden' || st.display === 'none') continue;
        if (r.height < 43.5 || r.width < 43.5) {
          kucuk.push(`${el.tagName.toLowerCase()}"${(el.textContent || '').trim().slice(0, 20)}" ${Math.round(r.width)}x${Math.round(r.height)}`);
        }
      }
      const kucukMetin = [];
      for (const el of document.querySelectorAll('main *')) {
        if (!el.childNodes.length) continue;
        const metin = [...el.childNodes].some((n) => n.nodeType === 3 && n.textContent.trim());
        if (!metin) continue;
        const fs = parseFloat(getComputedStyle(el).fontSize);
        if (fs < 13.9) kucukMetin.push(`${el.tagName.toLowerCase()} ${fs}px "${el.textContent.trim().slice(0, 18)}"`);
      }
      const tablo = document.querySelector('main table');
      const kart = document.querySelector('main ul.md\\:hidden li');
      const bolumler = [...document.querySelectorAll('main h2')].map((h) => h.textContent.trim());
      // tabular-nums taşıyan hücre sayısı
      const tabular = [...document.querySelectorAll('main td, main dd')]
        .filter((el) => getComputedStyle(el).fontVariantNumeric.includes('tabular-nums')).length;
      return {
        scrollWidth: d.scrollWidth,
        clientWidth: d.clientWidth,
        ariaCurrent: document.querySelectorAll('[aria-current="page"]').length,
        bolumler,
        tabloGorunur: !!tablo && tablo.getBoundingClientRect().height > 0,
        kartGorunur: !!kart && kart.getBoundingClientRect().height > 0,
        satirSayisi: document.querySelectorAll('main tbody tr').length,
        kartSayisi: document.querySelectorAll('main ul.md\\:hidden > li').length,
        tabular,
        kucukHedef: kucuk,
        kucukMetin: kucukMetin.slice(0, 5),
        img: document.querySelectorAll('main img').length,
        ariaLive: document.querySelectorAll('main [aria-live]').length,
        bosMetin: [...document.querySelectorAll('main p')].map((p) => p.textContent.trim())
          .filter((t) => t.includes('yok')).slice(0, 3),
      };
    });

    console.log(
      `${String(g).padStart(4)} | sw ${o.scrollWidth}/${o.clientWidth} ${o.scrollWidth > o.clientWidth ? '🔴TAŞMA' : '✓'}` +
      ` | current ${o.ariaCurrent} | ${o.tabloGorunur ? 'TABLO' : ''}${o.kartGorunur ? 'KART' : ''}` +
      ` | satır ${o.satirSayisi} kart ${o.kartSayisi} | tabular ${o.tabular} | img ${o.img}` +
      ` | live ${o.ariaLive} | küçükHedef ${o.kucukHedef.length} | küçükMetin ${o.kucukMetin.length}`,
    );
    if (o.kucukHedef.length) console.log('     hedef<44:', o.kucukHedef.join(' · '));
    if (o.kucukMetin.length) console.log('     metin<14:', o.kucukMetin.join(' · '));
    if (g === 320) console.log('     bölümler:', o.bolumler.join(' | '), '| boş:', o.bosMetin.join(' / '));

    await sayfa.screenshot({ path: `${CIKTI}/${hesap.ad}-${g}.png`, fullPage: true });
  }

  console.log(`konsol hataları: ${konsol.length ? konsol.join(' | ') : 'yok'}`);
  await baglam.close();
}

await tarayici.close();
