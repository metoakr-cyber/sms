/**
 * 768 px YATAY TAŞMA TEŞHİSİ — `/yonetim` üç ekranı.
 *
 * `scripts/responsive-check.mjs` 768 px'te üç ekranda yatay kaydırma ölçtü.
 * Bu betik SEBEBİ bulur: taşan öğeyi, sütun genişliklerini ve tabloya kalan
 * yeri sayıyla verir — "tablo geniş" demek teşhis değildir.
 *
 * Ayrıca kırılımı iki yandan tarar (760/768/776/1024): kart→tablo geçişinin
 * hangi genişlikte olduğunu ve taşmanın nerede başlayıp bittiğini gösterir.
 *
 * Kullanım: cd web && node olcum-tasma-768.mjs
 */
import { chromium } from 'playwright';
import { mkdirSync } from 'node:fs';

const KOK = 'http://localhost:3000';
const CIKTI = '/tmp/regresyon-768';
mkdirSync(CIKTI, { recursive: true });

const EKRANLAR = ['/yonetim/kullanicilar', '/yonetim/odeme-yontemleri', '/yonetim/denetim',
                  '/yonetim/saglayicilar', '/yonetim/fiyatlar', '/yonetim/talepler'];
const GENISLIKLER = [760, 768, 800, 860, 900, 960, 1024, 1100, 1180, 1280, 1440];

const TESHIS = () => {
  const kok = document.documentElement;
  const vw = kok.clientWidth;

  // Taşan öğeler: sağ kenarı görünür alanı geçen, en DERİN olanlar.
  // Ata zincirini de yazdıran bir liste teşhis vermez; yaprakları isteriz.
  const tasan = [];
  for (const e of document.querySelectorAll('body *')) {
    const r = e.getBoundingClientRect();
    if (r.width === 0 || r.height === 0) continue;
    if (r.right <= vw + 1) continue;
    // Çocuklarından biri de taşıyorsa bu ata; yaprağı raporla.
    const cocukTasiyor = [...e.children].some((c) => c.getBoundingClientRect().right > vw + 1);
    if (cocukTasiyor) continue;
    tasan.push({
      etiket: e.tagName + (e.className ? '.' + String(e.className).split(/\s+/).slice(0, 3).join('.') : ''),
      metin: (e.textContent || '').replace(/\s+/g, ' ').trim().slice(0, 45),
      sag: Math.round(r.right), gen: Math.round(r.width),
    });
  }

  const tablo = document.querySelector('table');
  const kabuk = document.querySelector('aside, [data-yan], nav[aria-label]');
  const ana = document.querySelector('main');

  return {
    scrollWidth: kok.scrollWidth,
    clientWidth: vw,
    tabloVar: !!tablo,
    kartVar: !!document.querySelector('ul li dl, [data-kart]'),
    tabloGen: tablo ? Math.round(tablo.getBoundingClientRect().width) : null,
    // Sütun başlıkları ve doğal genişlikleri — hangi sütun yeri yiyor?
    sutunlar: tablo
      ? [...tablo.querySelectorAll('thead th')].map((th) => ({
          b: (th.textContent || '').replace(/\s+/g, ' ').trim().slice(0, 18),
          w: Math.round(th.getBoundingClientRect().width),
        }))
      : null,
    anaGen: ana ? Math.round(ana.getBoundingClientRect().width) : null,
    anaSol: ana ? Math.round(ana.getBoundingClientRect().left) : null,
    yanSutunGen: kabuk ? Math.round(kabuk.getBoundingClientRect().width) : null,
    yanSutunEtiket: kabuk ? (kabuk.getAttribute('aria-label') || kabuk.tagName) : null,
    tasan: tasan.slice(0, 6),
  };
};

const tarayici = await chromium.launch();
const baglam = await tarayici.newContext({ viewport: { width: 768, height: 900 }, locale: 'tr-TR' });
const sayfa = await baglam.newPage();

await sayfa.goto(`${KOK}/giris`, { waitUntil: 'networkidle' });
await sayfa.fill('input[type="email"]', 'admin@onay360.test');
await sayfa.fill('input[type="password"]', 'dogru-at-pil-zimba');
await sayfa.click('button[type="submit"]');
await sayfa.waitForURL('**/panel**', { timeout: 20000 });

for (const yol of EKRANLAR) {
  console.log(`\n════ ${yol} ════`);
  for (const g of GENISLIKLER) {
    await sayfa.setViewportSize({ width: g, height: 900 });
    await sayfa.goto(KOK + yol, { waitUntil: 'networkidle' });
    await sayfa.waitForTimeout(500);
    const d = await sayfa.evaluate(TESHIS);
    const tasma = d.scrollWidth > d.clientWidth + 1 ? `TAŞMA ${d.scrollWidth}>${d.clientWidth}` : 'temiz';
    console.log(
      `  ${String(g).padEnd(5)} ${tasma.padEnd(20)} sunum=${d.tabloVar ? 'TABLO' : 'kart '} ` +
      `main=${String(d.anaGen).padEnd(5)} sol=${String(d.anaSol).padEnd(4)} ` +
      `yan=${d.yanSutunGen ?? '-'}(${d.yanSutunEtiket ?? '-'}) tablo=${d.tabloGen ?? '-'}`,
    );
    if (d.scrollWidth > d.clientWidth + 1) {
      if (d.sutunlar) console.log(`        sütunlar: ${d.sutunlar.map((s) => `${s.b}=${s.w}`).join(' · ')}`);
      for (const t of d.tasan) console.log(`        taşan: ${t.etiket} sağ=${t.sag} gen=${t.gen} "${t.metin}"`);
      await sayfa.screenshot({ path: `${CIKTI}/${yol.replace(/\//g, '_')}-${g}.png` });
    }
  }
}

await tarayici.close();
