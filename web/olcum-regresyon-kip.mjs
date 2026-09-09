/**
 * REGRESYON — KOYU TEMA ve AZALTILMIŞ HAREKET, genel sayfalarda.
 *
 * NEDEN: `globals.css` bu turda iki yerden değişti ve ikisi de PAZARLAMA
 * yüzeyini etkiliyor:
 *  (a) durum metin jetonları + kontrol kenarlığı → koyu/açık iki tema,
 *  (b) `prefers-reduced-motion` joker `0.01ms` kill'i KALDIRILDI (§4.5).
 *
 * (b) EN RİSKLİ DEĞİŞİKLİK. Pazarlama sayfaları `[data-belir]` ile açılışta
 * `opacity: 0`'dan gelir (globals.css:344). Eski joker kural azaltılmış kipte
 * süreyi 0.01ms'e indirerek öğeleri ANINDA görünür yapıyordu — yani boş sayfa
 * korumasının bir parçasıydı. Joker kalkınca bu koruma kalktıysa, hareketi
 * azaltılmış bir kullanıcı BOŞ SAYFA görür. Bu bir "estetik gerileme" değil,
 * içerik kaybıdır; ölçülmesi gereken tam olarak budur:
 *   → azaltılmış kipte görünür metin uzunluğu, olağan kiple AYNI mı?
 *
 * Ayrıca sonsuz döngülü dekoratif hareket (`.nabiz` 2,2s) azaltılmış kipte
 * durmuş mu, ve `animate-spin` (durum taşır) çalışmaya devam ediyor mu?
 *
 * Kullanım: cd web && node olcum-regresyon-kip.mjs
 */
import { chromium } from 'playwright';
import { mkdirSync } from 'node:fs';

const KOK = 'http://localhost:3000';
const CIKTI = '/tmp/regresyon-kip';
mkdirSync(CIKTI, { recursive: true });

const YOLLAR = ['/', '/fiyatlar', '/servisler', '/sss', '/blog', '/hakkimizda',
                '/iletisim', '/kiralama', '/gizlilik', '/kullanim-sartlari', '/giris', '/kayit'];

const OLC = () => {
  const gorunurMetin = (document.body.innerText || '').replace(/\s+/g, ' ').trim();

  // `[data-belir]` öğelerinin GERÇEK opaklığı — sınıf değil, hesaplanmış değer.
  const belir = [...document.querySelectorAll('[data-belir]')];
  const gizliBelir = belir.filter((e) => parseFloat(getComputedStyle(e).opacity) < 0.99);

  // Sonsuz döngülü animasyonlar: dekoratif olan durmalı, durum taşıyan sürmeli.
  const sonsuz = [];
  for (const e of document.querySelectorAll('*')) {
    const s = getComputedStyle(e);
    if (s.animationName === 'none' || !s.animationName) continue;
    const r = e.getBoundingClientRect();
    if (r.width === 0 || r.height === 0) continue;
    sonsuz.push({
      ad: s.animationName,
      sure: s.animationDuration,
      sayi: s.animationIterationCount,
      sinif: String(e.className).slice(0, 40),
    });
  }

  return {
    metinUzunluk: gorunurMetin.length,
    belirSayi: belir.length,
    gizliBelirSayi: gizliBelir.length,
    sonsuz: sonsuz.slice(0, 10),
    scrollWidth: document.documentElement.scrollWidth,
    clientWidth: document.documentElement.clientWidth,
  };
};

const tarayici = await chromium.launch();
const sonuc = {};

for (const [ad, opts] of [
  ['olagan', { colorScheme: 'dark', reducedMotion: 'no-preference' }],
  ['azaltilmis', { colorScheme: 'dark', reducedMotion: 'reduce' }],
  ['acik-tema', { colorScheme: 'light', reducedMotion: 'no-preference' }],
]) {
  const baglam = await tarayici.newContext({
    viewport: { width: 390, height: 844 }, locale: 'tr-TR', ...opts,
  });
  const sayfa = await baglam.newPage();
  sonuc[ad] = {};
  for (const yol of YOLLAR) {
    const konsol = [];
    sayfa.on('console', (m) => m.type() === 'error' && konsol.push(m.text().slice(0, 120)));
    await sayfa.goto(KOK + yol, { waitUntil: 'networkidle', timeout: 30000 });
    // Belirme animasyonu 550ms; olağan kipte bitmesini bekle ki karşılaştırma dürüst olsun.
    await sayfa.waitForTimeout(1200);
    sonuc[ad][yol] = { ...(await sayfa.evaluate(OLC)), konsol };
    sayfa.removeAllListeners('console');
    if (yol === '/' || yol === '/fiyatlar') {
      await sayfa.screenshot({ path: `${CIKTI}/${ad}${yol.replace(/\//g, '_') || '_kok'}.png`, fullPage: true });
    }
  }
  await baglam.close();
}
await tarayici.close();

console.log('yol                  olağan-metin  azaltılmış-metin  fark   gizli[data-belir]  konsol');
let kayip = 0;
for (const yol of YOLLAR) {
  const o = sonuc.olagan[yol], a = sonuc.azaltilmis[yol];
  const fark = a.metinUzunluk - o.metinUzunluk;
  if (a.gizliBelirSayi > 0) kayip++;
  console.log(
    yol.padEnd(21) + String(o.metinUzunluk).padEnd(14) + String(a.metinUzunluk).padEnd(18) +
    String(fark).padEnd(7) + `${a.gizliBelirSayi}/${a.belirSayi}`.padEnd(19) +
    (o.konsol.length + a.konsol.length),
  );
}

console.log(`\nazaltılmış kipte GİZLİ KALAN [data-belir] içeren sayfa: ${kayip}/${YOLLAR.length}`);

console.log('\n── sonsuz döngülü hareket (olağan → azaltılmış) ──');
for (const yol of YOLLAR) {
  const o = sonuc.olagan[yol].sonsuz, a = sonuc.azaltilmis[yol].sonsuz;
  if (!o.length && !a.length) continue;
  const ozet = (l) => [...new Set(l.map((x) => `${x.ad}(${x.sure}×${x.sayi})`))].join(' ');
  console.log(`${yol.padEnd(21)} olağan: ${ozet(o) || '-'}\n${' '.repeat(21)} azaltılmış: ${ozet(a) || '-'}`);
}

console.log('\n── açık tema konsol/taşma ──');
for (const yol of YOLLAR) {
  const t = sonuc['acik-tema'][yol];
  if (t.konsol.length || t.scrollWidth > t.clientWidth + 1) {
    console.log(`${yol}: taşma=${t.scrollWidth}>${t.clientWidth} konsol=${JSON.stringify(t.konsol)}`);
  }
}
console.log('(yukarısı boşsa açık tema temiz)');
