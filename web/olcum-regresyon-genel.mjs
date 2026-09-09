/**
 * REGRESYON — genel (pazarlama + kimlik) sayfaları.
 *
 * NEDEN VAR: bu turda `ui.tsx` (Field/Button/Alert/Badge) ve `globals.css`
 * (kontrol kenarlığı, durum metin jetonları, azaltılmış hareket kipi)
 * değişti. Bu iki dosya PAZARLAMA yüzeyini de besliyor (§11.2 ölçümü:
 * `Button` 139 kullanımın 27'si, `Card` 63'ün 31'i pazarlamada) — yani
 * yönetim için yapılan bir düzeltme burada gerileme üretebilir.
 *
 * ÖLÇÜLENLER (hepsi sayı, göz kararı yok):
 *  - konsol hatası / sayfa istisnası / başarısız ağ isteği
 *  - 375 px'te yatay taşma (scrollWidth > clientWidth)
 *  - 44 px altı dokunma hedefi (görünür, boyutu olan)
 *  - 16 px altı girdi alanı (iOS yakınlaştırma)
 *  - `text-xs` ile yazılmış VERİ metni sayısı (§3.2 — pazarlamada serbest,
 *    yalnız kayıt için)
 *
 * Kullanım: cd web && node olcum-regresyon-genel.mjs
 */
import { chromium } from 'playwright';
import { writeFileSync, mkdirSync } from 'node:fs';

const KOK = 'http://localhost:3000';
const CIKTI = '/tmp/regresyon-genel';
mkdirSync(CIKTI, { recursive: true });

const YOLLAR = [
  '/', '/fiyatlar', '/servisler', '/sss', '/blog', '/hakkimizda',
  '/iletisim', '/kiralama', '/gizlilik', '/kullanim-sartlari',
  '/giris', '/kayit',
];

const GENISLIKLER = [375, 1280];

/** Ölçümü sayfa içinde yapar: DOM'a bir kez inip hepsini birden okur. */
const OLC = () => {
  const gorunur = (e) => {
    const r = e.getBoundingClientRect();
    return r.width > 0 && r.height > 0 && getComputedStyle(e).visibility !== 'hidden';
  };
  const kucukHedef = [];
  for (const e of document.querySelectorAll('a[href],button,input,select,textarea,[role="button"],[tabindex]:not([tabindex="-1"])')) {
    if (!gorunur(e)) continue;
    const r = e.getBoundingClientRect();
    // Metin içi bağlantı 44px kuralının dışındadır (WCAG 2.5.8 istisnası):
    // satır içi <a> bir kontrol değil, akan metnin parçasıdır.
    const satirIci = e.tagName === 'A' && e.closest('p,li,dd,figcaption');
    if (satirIci) continue;
    if (r.width < 44 || r.height < 44) {
      kucukHedef.push({
        etiket: (e.textContent || e.getAttribute('aria-label') || e.tagName).replace(/\s+/g, ' ').trim().slice(0, 40),
        w: Math.round(r.width), h: Math.round(r.height),
        sinif: (e.className || '').toString().slice(0, 60),
      });
    }
  }
  const kucukGirdi = [];
  for (const e of document.querySelectorAll('input,select,textarea')) {
    if (!gorunur(e)) continue;
    const fs = parseFloat(getComputedStyle(e).fontSize);
    if (fs < 16) kucukGirdi.push({ tip: e.type || e.tagName, px: fs });
  }
  return {
    scrollWidth: document.documentElement.scrollWidth,
    clientWidth: document.documentElement.clientWidth,
    kucukHedef,
    kucukGirdi,
    h1: [...document.querySelectorAll('h1')].map((h) => h.textContent.trim().slice(0, 60)),
    // §8/6: iç kaydırma alanı yatay kayıyor mu?
    yatayKayanKap: [...document.querySelectorAll('*')]
      .filter((e) => e.scrollWidth > e.clientWidth + 1 && ['auto', 'scroll'].includes(getComputedStyle(e).overflowX))
      .map((e) => e.tagName + '.' + (e.className || '').toString().slice(0, 40)),
  };
};

const tarayici = await chromium.launch();
const rapor = [];

for (const genislik of GENISLIKLER) {
  const baglam = await tarayici.newContext({
    viewport: { width: genislik, height: 800 },
    locale: 'tr-TR',
    timezoneId: 'Europe/Istanbul',
  });
  const sayfa = await baglam.newPage();

  for (const yol of YOLLAR) {
    const konsol = [];
    const istisna = [];
    const agHata = [];
    const dinleKonsol = (m) => { if (m.type() === 'error') konsol.push(m.text().slice(0, 160)); };
    const dinleIstisna = (e) => istisna.push(String(e).slice(0, 160));
    const dinleAg = (r) => {
      const s = r.status();
      // 401 gizli uçlarda beklenir (oturumsuz gezinme); 404 favicon gürültüsü değil, sayılır.
      if (s >= 400 && s !== 401) agHata.push(`${s} ${r.url().replace(KOK, '')}`);
    };
    sayfa.on('console', dinleKonsol);
    sayfa.on('pageerror', dinleIstisna);
    sayfa.on('response', dinleAg);

    let olcum = null, hata = null;
    try {
      await sayfa.goto(KOK + yol, { waitUntil: 'networkidle', timeout: 30000 });
      await sayfa.waitForTimeout(400);
      olcum = await sayfa.evaluate(OLC);
      await sayfa.screenshot({
        path: `${CIKTI}/${genislik}${yol.replace(/\//g, '_') || '_kok'}.png`,
        fullPage: genislik === 375,
      });
    } catch (e) {
      hata = String(e).slice(0, 200);
    }

    sayfa.off('console', dinleKonsol);
    sayfa.off('pageerror', dinleIstisna);
    sayfa.off('response', dinleAg);

    rapor.push({ genislik, yol, hata, konsol, istisna, agHata, ...olcum });
  }
  await baglam.close();
}
await tarayici.close();

writeFileSync(`${CIKTI}/rapor.json`, JSON.stringify(rapor, null, 2));

// ── Özet tablo ────────────────────────────────────────────────────────────
console.log('genişlik yol                    taşma  konsol istisna ağ  <44px <16px  h1');
for (const r of rapor) {
  const tasma = r.hata ? 'HATA' : (r.scrollWidth > r.clientWidth + 1 ? `${r.scrollWidth}>${r.clientWidth}` : 'yok');
  console.log(
    String(r.genislik).padEnd(9) + r.yol.padEnd(22) +
    String(tasma).padEnd(7) +
    String(r.konsol?.length ?? '-').padEnd(7) +
    String(r.istisna?.length ?? '-').padEnd(8) +
    String(r.agHata?.length ?? '-').padEnd(4) +
    String(r.kucukHedef?.length ?? '-').padEnd(6) +
    String(r.kucukGirdi?.length ?? '-').padEnd(7) +
    (r.h1?.length === 1 ? 'ok' : `${r.h1?.length ?? '?'} adet`),
  );
}

console.log('\n── AYRINTI (yalnız sıfırdan farklı olanlar) ──');
for (const r of rapor) {
  const parcalar = [];
  if (r.hata) parcalar.push(`  YÜKLENEMEDİ: ${r.hata}`);
  if (r.konsol?.length) parcalar.push(`  konsol: ${JSON.stringify(r.konsol)}`);
  if (r.istisna?.length) parcalar.push(`  istisna: ${JSON.stringify(r.istisna)}`);
  if (r.agHata?.length) parcalar.push(`  ağ: ${JSON.stringify([...new Set(r.agHata)])}`);
  if (r.kucukHedef?.length) parcalar.push(`  <44px: ${JSON.stringify(r.kucukHedef.slice(0, 8))}`);
  if (r.kucukGirdi?.length) parcalar.push(`  <16px girdi: ${JSON.stringify(r.kucukGirdi)}`);
  if (r.yatayKayanKap?.length) parcalar.push(`  yatay kayan kap: ${JSON.stringify(r.yatayKayanKap)}`);
  if (r.h1?.length !== 1) parcalar.push(`  h1: ${JSON.stringify(r.h1)}`);
  if (parcalar.length) console.log(`\n${r.genislik}px ${r.yol}\n${parcalar.join('\n')}`);
}
