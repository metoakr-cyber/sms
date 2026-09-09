/**
 * Hareket kipi ölçümü: normal vs prefers-reduced-motion.
 * Kullanım: node olcum-hareket.mjs [etiket]
 */
import { chromium } from 'playwright';
import fs from 'node:fs';

const ETIKET = process.argv[2] ?? 'olcum';
const CIKTI = '/private/tmp/claude-501/-Users-morepayroll-Desktop-opencart3x-extension-smsEntegrasyon/96f93d50-0fe8-49fa-bca9-aa862db0e2d1/scratchpad';
const KOK = 'http://localhost:3000';

const OKU = `(() => {
  const al = (sec) => {
    const el = document.querySelector(sec);
    if (!el) return { yok: true };
    const s = getComputedStyle(el);
    return {
      ozellik: s.transitionProperty,
      sure: s.transitionDuration,
      egri: s.transitionTimingFunction,
      anim: s.animationName + ' ' + s.animationDuration + ' ' + s.animationIterationCount,
    };
  };
  return {
    'Button (basma-geri-bildirimi)': al('button.basma-geri-bildirimi'),
    'Kart hover (.kart-hover)': al('.kart-hover'),
    'CTA (.cta-marka)': al('.cta-marka'),
    'Belirme ([data-belir])': al('[data-belir]'),
    'Site gezinti bağlantısı': al('nav a.transition-colors'),
    'Belirme opaklik': (() => {
      const el = document.querySelector('[data-belir]');
      if (!el) return { yok: true };
      const s = getComputedStyle(el);
      return { opacity: s.opacity, transform: s.transform };
    })(),
    'Gorunur bolum sayisi': document.querySelectorAll('[data-belir]').length,
    'Gizli kalan bolum': [...document.querySelectorAll('[data-belir]')]
      .filter((e) => parseFloat(getComputedStyle(e).opacity) < 0.99).length,
  };
})()`;

const OKU_SPIN = `(() => {
  const d = document.createElement('div');
  d.innerHTML = '<span class="animate-spin" id="__s"></span><span class="animate-pulse" id="__p"></span><span class="animate-ping" id="__g"></span>';
  document.body.appendChild(d);
  const g = (id) => { const s = getComputedStyle(document.getElementById(id)); return s.animationName + ' | ' + s.animationDuration + ' | ' + s.animationIterationCount; };
  const r = { 'animate-spin': g('__s'), 'animate-pulse': g('__p'), 'animate-ping': g('__g') };
  d.remove();
  return r;
})()`;

const tarayici = await chromium.launch();
const sonuc = {};
for (const kip of ['no-preference', 'reduce']) {
  const baglam = await tarayici.newContext({ viewport: { width: 1280, height: 900 }, reducedMotion: kip });
  const sayfa = await baglam.newPage();
  await sayfa.goto(`${KOK}/`, { waitUntil: 'networkidle' });
  await sayfa.waitForTimeout(1500);
  sonuc[kip] = await sayfa.evaluate(OKU);
  sonuc[kip].anahtarKareler = await sayfa.evaluate(OKU_SPIN);
  await sayfa.screenshot({ path: `${CIKTI}/hareket-${ETIKET}-${kip}.png`, fullPage: false });
  await baglam.close();
}
fs.writeFileSync(`${CIKTI}/hareket-${ETIKET}.json`, JSON.stringify(sonuc, null, 2));
console.log(JSON.stringify(sonuc, null, 2));
await tarayici.close();
