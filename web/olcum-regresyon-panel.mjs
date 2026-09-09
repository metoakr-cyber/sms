/**
 * REGRESYON — `/panel` kabuğu ve taşma bandı.
 *
 * İKİ SORU:
 *  1) Kullanıcının BAĞLAYICI kararı "yan sütun kaldırılsın, yerine üst header"
 *     idi. Bugün kodda `panel-shell.tsx:87` bir `lg:w-60` `aside` var.
 *     Tarayıcıda gerçekten çiziliyor mu, hangi genişlikten itibaren?
 *  2) `/yonetim`'de ölçülen 768/1024 taşma bandının `/panel` karşılığı var mı?
 *     `/panel` yan sütunu `lg:`de açıyor (yönetim `md:`de) — `VeriTablosu`
 *     kart→tablo geçişi ise `md:`. İki kabuk iki farklı kırılım kullanıyor,
 *     yani taşma bandı da farklı yerde olmalı.
 *
 * Kullanım: cd web && node olcum-regresyon-panel.mjs
 */
import { chromium } from 'playwright';
import { mkdirSync } from 'node:fs';

const KOK = 'http://localhost:3000';
const CIKTI = '/tmp/regresyon-panel';
mkdirSync(CIKTI, { recursive: true });

const EKRANLAR = ['/panel', '/panel/siparisler', '/panel/cuzdan', '/panel/bakiye-yukle',
                  '/panel/destek', '/panel/hesap', '/panel/yorumlarim', '/panel/numara-al'];
const GENISLIKLER = [320, 390, 430, 768, 860, 1024, 1100, 1280, 1440];

const OLC = () => {
  const kok = document.documentElement;
  const vw = kok.clientWidth;
  const yan = document.querySelector('aside');
  const ana = document.querySelector('main');
  const navlar = [...document.querySelectorAll('nav')].map((n) => {
    const r = n.getBoundingClientRect();
    return {
      etiket: n.getAttribute('aria-label') || '(etiketsiz)',
      konum: getComputedStyle(n).position,
      w: Math.round(r.width), h: Math.round(r.height),
      ust: Math.round(r.top), sol: Math.round(r.left),
      gorunur: r.width > 0 && r.height > 0,
    };
  });
  const tasan = [];
  for (const e of document.querySelectorAll('body *')) {
    const r = e.getBoundingClientRect();
    if (r.width === 0 || r.height === 0 || r.right <= vw + 1) continue;
    if ([...e.children].some((c) => c.getBoundingClientRect().right > vw + 1)) continue;
    tasan.push(`${e.tagName} gen=${Math.round(r.width)} "${(e.textContent || '').replace(/\s+/g, ' ').trim().slice(0, 35)}"`);
  }
  const tablo = document.querySelector('table');
  return {
    scrollWidth: kok.scrollWidth, clientWidth: vw,
    yanGen: yan ? Math.round(yan.getBoundingClientRect().width) : null,
    yanGorunur: yan ? getComputedStyle(yan).display !== 'contents' && yan.getBoundingClientRect().width > 0 : false,
    anaGen: ana ? Math.round(ana.getBoundingClientRect().width) : null,
    anaSol: ana ? Math.round(ana.getBoundingClientRect().left) : null,
    tabloGen: tablo && tablo.getBoundingClientRect().width > 0 ? Math.round(tablo.getBoundingClientRect().width) : null,
    ariaCurrent: document.querySelectorAll('[aria-current="page"]').length,
    navlar: navlar.filter((n) => n.gorunur),
    tasan: tasan.slice(0, 5),
  };
};

const tarayici = await chromium.launch();
const baglam = await tarayici.newContext({ viewport: { width: 1280, height: 900 }, locale: 'tr-TR' });
const sayfa = await baglam.newPage();
const konsol = [];
sayfa.on('console', (m) => m.type() === 'error' && konsol.push(m.text().slice(0, 120)));
sayfa.on('pageerror', (e) => konsol.push('İSTİSNA ' + String(e).slice(0, 120)));

await sayfa.goto(`${KOK}/giris`, { waitUntil: 'networkidle' });
await sayfa.fill('input[type="email"]', 'yuk-1788931064-1@yuk.test');
await sayfa.fill('input[type="password"]', 'yuk-testi-parolasi-uzun');
await sayfa.click('button[type="submit"]');
await sayfa.waitForURL('**/panel**', { timeout: 20000 });

// ── 1. Kabuk: yan sütun hangi genişlikte beliriyor? ──
console.log('══ KABUK — /panel ══');
console.log('gen   taşma        yanSütun  main  sol   aria-current  görünür nav(lar)');
for (const g of GENISLIKLER) {
  await sayfa.setViewportSize({ width: g, height: 900 });
  await sayfa.goto(`${KOK}/panel`, { waitUntil: 'networkidle' });
  await sayfa.waitForTimeout(400);
  const d = await sayfa.evaluate(OLC);
  console.log(
    String(g).padEnd(6) +
    (d.scrollWidth > d.clientWidth + 1 ? `TAŞMA ${d.scrollWidth}` : 'temiz').padEnd(13) +
    String(d.yanGorunur ? d.yanGen + 'px' : 'yok').padEnd(10) +
    String(d.anaGen).padEnd(6) + String(d.anaSol).padEnd(6) +
    String(d.ariaCurrent).padEnd(14) +
    d.navlar.map((n) => `${n.etiket}[${n.konum} ${n.w}x${n.h}]`).join(' '),
  );
  await sayfa.screenshot({ path: `${CIKTI}/panel-${g}.png` });
}

// ── 2. Taşma bandı ──
console.log('\n══ TAŞMA BANDI — tüm /panel ekranları ══');
console.log('ekran                 ' + GENISLIKLER.map((g) => String(g).padStart(6)).join(''));
for (const yol of EKRANLAR) {
  const satir = [];
  for (const g of GENISLIKLER) {
    await sayfa.setViewportSize({ width: g, height: 900 });
    await sayfa.goto(KOK + yol, { waitUntil: 'networkidle' });
    await sayfa.waitForTimeout(400);
    const d = await sayfa.evaluate(OLC);
    if (d.scrollWidth > d.clientWidth + 1) {
      satir.push(String(d.scrollWidth).padStart(6));
      console.error(`  ! ${yol} @${g}: ${d.scrollWidth}>${d.clientWidth} — ${d.tasan.join(' | ')}`);
      await sayfa.screenshot({ path: `${CIKTI}/tasma${yol.replace(/\//g, '_')}-${g}.png` });
    } else satir.push('     ·');
  }
  console.log(yol.padEnd(22) + satir.join(''));
}

console.log(`\nkonsol hatası: ${konsol.length}`);
for (const k of [...new Set(konsol)]) console.log('  ' + k);
await tarayici.close();
