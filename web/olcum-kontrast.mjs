/**
 * Kontrast ölçüm betiği — GERÇEK render'dan okur, jetondan hesaplamaz.
 * Her ölçüm iki temada (koyu varsayılan + açık) tekrarlanır.
 *
 * Kullanım: node olcum-kontrast.mjs [etiket]
 */
import { chromium } from 'playwright';
import fs from 'node:fs';

const ETIKET = process.argv[2] ?? 'olcum';
const CIKTI = '/private/tmp/claude-501/-Users-morepayroll-Desktop-opencart3x-extension-smsEntegrasyon/96f93d50-0fe8-49fa-bca9-aa862db0e2d1/scratchpad';
const KOK = 'http://localhost:3000';
const YONETICI = { e: 'admin@onay360.test', p: 'dogru-at-pil-zimba' };

const OLCUM_KODU = `
(() => {
  /*
   * 🔴 NEDEN CANVAS: Tailwind v4 opaklık değiştiricisini
   * \`color-mix(in oklab, var(--color-bad) 12%, transparent)\` olarak üretiyor.
   * getComputedStyle bunu ham \`color-mix()\`/\`oklab()\` dizgesi olarak döndürür;
   * rgba() regex'i BOŞ döner ve rozet zeminleri ölçüme hiç girmez (ilk turda
   * tam bu oldu: rozetin arkası kart rengi sanıldı). Canvas, tarayıcının kendi
   * renk motorudur — hangi sözdizimi olursa olsun gerçek pikseli verir.
   */
  const cv = document.createElement('canvas');
  cv.width = cv.height = 1;
  const ctx = cv.getContext('2d', { willReadFrequently: true });

  const boya = (renkler) => {
    ctx.clearRect(0, 0, 1, 1);
    ctx.fillStyle = '#ffffff';
    ctx.fillRect(0, 0, 1, 1);
    for (const c of renkler) {
      if (!c) continue;
      ctx.fillStyle = '#000000';
      ctx.fillStyle = c;                       // geçersizse '#000000' kalır
      if (ctx.fillStyle === '#000000' && !/^(#000000|black|rgb\\(0, 0, 0\\))$/.test(String(c).trim())) continue;
      ctx.fillRect(0, 0, 1, 1);
    }
    const d = ctx.getImageData(0, 0, 1, 1).data;
    return { r: d[0], g: d[1], b: d[2], a: 1 };
  };
  const zeminYigini = (el, kendiDahil) => {
    const y = [];
    let n = kendiDahil ? el : el.parentElement;
    while (n && n.nodeType === 1) {
      y.push(getComputedStyle(n).backgroundColor);
      n = n.parentElement;
    }
    return boya(y.reverse());
  };
  const uzerinde = (renk, zemin) => boya([
    'rgb(' + Math.round(zemin.r) + ',' + Math.round(zemin.g) + ',' + Math.round(zemin.b) + ')',
    renk,
  ]);
  const parlaklik = (c) => {
    const f = (v) => { v /= 255; return v <= 0.03928 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4); };
    return 0.2126 * f(c.r) + 0.7152 * f(c.g) + 0.0722 * f(c.b);
  };
  const oran = (a, b) => {
    const L1 = parlaklik(a), L2 = parlaklik(b);
    const hi = Math.max(L1, L2), lo = Math.min(L1, L2);
    return +((hi + 0.05) / (lo + 0.05)).toFixed(2);
  };
  const hex = (c) => '#' + [c.r, c.g, c.b].map((v) => Math.round(v).toString(16).padStart(2, '0')).join('');
  const gorunur = (el) => { const r = el.getBoundingClientRect(); return r.width > 0 && r.height > 0; };

  /* Aynı (ön, arka) çiftini bir kez raporlar; örnek metni taşır. */
  window.__metinler = (sec, ad, esik) => {
    const cikti = new Map();
    for (const el of document.querySelectorAll(sec)) {
      if (!gorunur(el)) continue;
      const s = getComputedStyle(el);
      const zemin = zeminYigini(el, true);
      const renk = uzerinde(s.color, zemin);
      const anahtar = hex(renk) + '|' + hex(zemin);
      if (cikti.has(anahtar)) { cikti.get(anahtar).adet++; continue; }
      cikti.set(anahtar, {
        ad, tur: 'metin', ornek: (el.textContent || '').trim().slice(0, 26),
        on: hex(renk), arka: hex(zemin), boyut: s.fontSize, kalinlik: s.fontWeight,
        olculen: oran(renk, zemin), esik: esik ?? 4.5, adet: 1,
      });
    }
    return [...cikti.values()];
  };

  window.__kenarlar = (sec, ad) => {
    const cikti = new Map();
    for (const el of document.querySelectorAll(sec)) {
      if (!gorunur(el)) continue;
      const s = getComputedStyle(el);
      if (parseFloat(s.borderTopWidth) === 0) continue;
      const disZemin = zeminYigini(el, false);
      const dolgu = uzerinde(s.backgroundColor, disZemin);
      const kenar = uzerinde(s.borderTopColor, disZemin);
      const anahtar = hex(kenar) + '|' + hex(dolgu) + '|' + hex(disZemin);
      if (cikti.has(anahtar)) { cikti.get(anahtar).adet++; continue; }
      cikti.set(anahtar, {
        ad, tur: 'kenar', etiket: el.tagName.toLowerCase(),
        on: hex(kenar), dolgu: hex(dolgu), disari: hex(disZemin),
        kenarDolgu: oran(kenar, dolgu), kenarDisari: oran(kenar, disZemin),
        esik: 3, adet: 1,
      });
    }
    return [...cikti.values()];
  };

  /* .select-ok oku bir data-URI arka plan görselidir; rengi CSS metninden çekilir. */
  window.__oklar = (sec, ad) => {
    const cikti = new Map();
    for (const el of document.querySelectorAll(sec)) {
      if (!gorunur(el)) continue;
      const s = getComputedStyle(el);
      const bi = s.backgroundImage || '';
      const m = bi.match(/stroke='?(?:%23|#)([0-9a-fA-F]{6})'?/);
      if (!m) continue;
      const h = m[1];
      const renk = { r: parseInt(h.slice(0,2),16), g: parseInt(h.slice(2,4),16), b: parseInt(h.slice(4,6),16), a: 1 };
      const disZemin = zeminYigini(el, false);
      const dolgu = uzerinde(s.backgroundColor, disZemin);
      const anahtar = '#' + h + '|' + hex(dolgu);
      if (cikti.has(anahtar)) { cikti.get(anahtar).adet++; continue; }
      cikti.set(anahtar, { ad, tur: 'metin', ornek: 'ok', on: '#' + h, arka: hex(dolgu), olculen: oran(renk, dolgu), esik: 3, adet: 1 });
    }
    return [...cikti.values()];
  };

  /*
   * SONDA. Bir "Reddedildi"/"Askıda" rozeti ekranda o an duruyor olmayabilir
   * (talepler kuyruğu boş, tüm sağlayıcılar kurulu…). Sınıflar derlenmiş CSS'te
   * HER ZAMAN vardır (Tailwind kaynağı tarar), bu yüzden aynı sınıflarla bir
   * sonda düğümü basmak gerçek rozetle BİREBİR aynı pikseli üretir — ve ölçüm
   * veri durumuna bağlı olmaktan çıkar.
   */
  window.__sonda = (siniflar) => {
    document.getElementById('__sonda')?.remove();
    const kap = document.createElement('div');
    kap.id = '__sonda';
    kap.className = 'surface border';
    kap.style.cssText = 'padding:12px;margin:12px;border-width:1px';
    const yuvala = (ana, ekSinif) => {
      for (const [ad, sinif] of Object.entries(siniflar.rozet)) {
        const s = document.createElement('span');
        s.className = siniflar.rozetTaban + ' ' + sinif;
        s.dataset.sonda = 'Badge ' + ad + ekSinif;
        s.textContent = 'Reddedildi';
        ana.appendChild(s);
      }
      for (const [ad, sinif] of Object.entries(siniflar.uyari)) {
        const d = document.createElement('div');
        d.className = siniflar.uyariTaban + ' ' + sinif;
        d.dataset.sonda = 'Alert ' + ad + ekSinif;
        d.textContent = 'Bir hata oluştu. requestId: 7f3c';
        ana.appendChild(d);
      }
    };
    yuvala(kap, ' @surface');
    const ic = document.createElement('div');
    ic.className = 'raised border';
    ic.style.cssText = 'padding:12px;margin-top:12px;border-width:1px';
    yuvala(ic, ' @raised');
    const b = document.createElement('button');
    b.className = siniflar.tehlike;
    b.dataset.sonda = 'Button danger';
    b.textContent = 'Kalıcı olarak sil';
    ic.appendChild(b);
    kap.appendChild(ic);
    (document.querySelector('main') || document.body).appendChild(kap);
    return true;
  };

  window.__sondalar = () => {
    const cikti = [];
    for (const el of document.querySelectorAll('#__sonda [data-sonda]')) {
      const s = getComputedStyle(el);
      const zemin = zeminYigini(el, true);
      const renk = uzerinde(s.color, zemin);
      cikti.push({
        ad: el.dataset.sonda, tur: 'metin', ornek: (el.textContent || '').slice(0, 14),
        on: hex(renk), arka: hex(zemin), boyut: s.fontSize,
        olculen: oran(renk, zemin), esik: 4.5, adet: 1,
      });
    }
    return cikti;
  };

  window.__hareket = (sec) => {
    const el = document.querySelector(sec);
    if (!el) return { yok: true, sec };
    const s = getComputedStyle(el);
    return { sec, ozellik: s.transitionProperty, sure: s.transitionDuration, egri: s.transitionTimingFunction };
  };
  return true;
})();
`;

async function olc(sayfa, hedefler) {
  await sayfa.evaluate(OLCUM_KODU);
  const cikti = {};
  for (const tema of ['dark', 'light']) {
    await sayfa.evaluate((t) => document.documentElement.setAttribute('data-theme', t), tema);
    await sayfa.waitForTimeout(150);
    cikti[tema] = await sayfa.evaluate((hs) => {
      const hepsi = [];
      for (const h of hs) {
        if (h.tur === 'metin') hepsi.push(...window.__metinler(h.sec, h.ad, h.esik));
        else if (h.tur === 'kenar') hepsi.push(...window.__kenarlar(h.sec, h.ad));
        else if (h.tur === 'ok') hepsi.push(...window.__oklar(h.sec, h.ad));
      }
      return hepsi;
    }, hedefler);
  }
  await sayfa.evaluate(() => document.documentElement.removeAttribute('data-theme'));
  return cikti;
}

/*
 * 🔴 SONDA SINIFLARI `src/components/ui.tsx` İÇİNDEN OKUNUR — elle kopyalanmaz.
 * Elle kopyalansaydı "önce" ölçümü eski, "sonra" ölçümü yeni sınıflarla yapılır
 * ve tablo kendi kendini doğrulayamazdı.
 */
const UI = fs.readFileSync('src/components/ui.tsx', 'utf8');
const blok = (ad) => {
  const i = UI.indexOf(`export function ${ad}(`);
  if (i < 0) throw new Error(`${ad} bulunamadi`);
  const j = UI.indexOf('\nexport ', i + 1);
  return UI.slice(i, j < 0 ? UI.length : j);
};
const tonlariCek = (metin) => {
  const t = metin.slice(metin.indexOf('const tones'));
  const cikti = {};
  for (const m of t.matchAll(/^\s*(neutral|ok|warn|bad|info|brand):\s*'([^']*)'/gm)) cikti[m[1]] = m[2];
  return cikti;
};
const rozetBlok = blok('Badge');
const uyariBlok = blok('Alert');
const dugmeBlok = blok('Button');
const SONDA = {
  rozetTaban: (rozetBlok.match(/cx\(\s*\n\s*'([^']+)'/) || [])[1],
  rozet: tonlariCek(rozetBlok),
  uyariTaban: (uyariBlok.match(/cx\('([^']+)'/) || [])[1],
  uyari: tonlariCek(uyariBlok),
  tehlike:
    (dugmeBlok.match(/danger:\s*'([^']*)'/) || [])[1] + ' ' +
    (dugmeBlok.match(/md:\s*'([^']*)'/) || [])[1] +
    ' inline-flex items-center justify-center rounded-xl font-medium',
};
if (!SONDA.rozetTaban || !SONDA.uyariTaban || !SONDA.tehlike) {
  throw new Error('ui.tsx sinif cikarimi basarisiz: ' + JSON.stringify(SONDA));
}

const ROZET = 'span.rounded-lg.border';
const ALERT = 'div.rounded-xl.border.px-4.py-3.text-sm';
const GIRDI = 'input.raised, select.raised, textarea.raised';

const tarayici = await chromium.launch();
const baglam = await tarayici.newContext({ viewport: { width: 1440, height: 900 } });
const sayfa = await baglam.newPage();
const sonuc = {};

// ── 1) /giris (genel sayfa) ────────────────────────────────────────────────
await sayfa.goto(`${KOK}/giris`, { waitUntil: 'networkidle' });
sonuc['giris (genel)'] = await olc(sayfa, [
  { tur: 'kenar', ad: 'Field girdisi', sec: GIRDI },
  { tur: 'metin', ad: 'Button primary', sec: 'form button[type="submit"]' },
]);
sonuc['_hareket_dugme'] = await sayfa.evaluate(() => window.__hareket('form button[type="submit"]'));

// ── 2) Yönetici girişi ─────────────────────────────────────────────────────
await sayfa.fill('input[name="email"]', YONETICI.e);
await sayfa.fill('input[name="password"]', YONETICI.p);
await sayfa.click('form button[type="submit"]');
await sayfa.waitForURL(/\/(yonetim|panel)/, { timeout: 25000 }).catch(() => {});
await sayfa.waitForTimeout(1200);

// ── 3) /yonetim/talepler — rozetler (tablo, --surface) + Secim + Alert ─────
await sayfa.goto(`${KOK}/yonetim/talepler`, { waitUntil: 'networkidle' });
await sayfa.waitForTimeout(1500);
sonuc['talepler 1440 (tablo/--surface)'] = await olc(sayfa, [
  { tur: 'metin', ad: 'Badge', sec: ROZET },
  { tur: 'metin', ad: 'Alert', sec: ALERT },
  { tur: 'kenar', ad: 'Girdi/Secim kenarlığı', sec: GIRDI },
  { tur: 'ok', ad: '.select-ok oku', sec: 'select.select-ok' },
]);

// ── 4) 390px — rozetler mobil kartta (--raised) ───────────────────────────
await sayfa.setViewportSize({ width: 390, height: 844 });
await sayfa.waitForTimeout(600);
sonuc['talepler 390 (kart/--raised)'] = await olc(sayfa, [
  { tur: 'metin', ad: 'Badge', sec: ROZET },
  { tur: 'metin', ad: 'Alert', sec: ALERT },
]);

// ── 5) /yonetim/saglayicilar — "Kurulu değil" bad rozeti ──────────────────
await sayfa.setViewportSize({ width: 1440, height: 900 });
await sayfa.goto(`${KOK}/yonetim/saglayicilar`, { waitUntil: 'networkidle' });
await sayfa.waitForTimeout(1500);
sonuc['saglayicilar 1440'] = await olc(sayfa, [
  { tur: 'metin', ad: 'Badge', sec: ROZET },
  { tur: 'metin', ad: 'Alert', sec: ALERT },
]);
await sayfa.setViewportSize({ width: 390, height: 844 });
await sayfa.waitForTimeout(600);
sonuc['saglayicilar 390'] = await olc(sayfa, [{ tur: 'metin', ad: 'Badge', sec: ROZET }]);

// ── 6) SONDA: dört ton × iki zemin (--surface / --raised), veriden bağımsız ─
await sayfa.setViewportSize({ width: 1440, height: 900 });
await sayfa.goto(`${KOK}/yonetim/saglayicilar`, { waitUntil: 'networkidle' });
await sayfa.waitForTimeout(1200);
await sayfa.evaluate(OLCUM_KODU);
await sayfa.evaluate((s) => window.__sonda(s), SONDA);
{
  const cikti = {};
  for (const tema of ['dark', 'light']) {
    await sayfa.evaluate((t) => document.documentElement.setAttribute('data-theme', t), tema);
    await sayfa.waitForTimeout(150);
    cikti[tema] = await sayfa.evaluate(() => window.__sondalar());
  }
  await sayfa.evaluate(() => document.documentElement.removeAttribute('data-theme'));
  sonuc['SONDA (ui.tsx siniflari)'] = cikti;
}

// ── 7) /yonetim/bakiye — Field + kalıcı Alert ─────────────────────────────
await sayfa.setViewportSize({ width: 1440, height: 900 });
await sayfa.goto(`${KOK}/yonetim/bakiye`, { waitUntil: 'networkidle' });
await sayfa.waitForTimeout(1200);
sonuc['bakiye 1440'] = await olc(sayfa, [
  { tur: 'kenar', ad: 'Field girdisi', sec: GIRDI },
  { tur: 'metin', ad: 'Alert', sec: ALERT },
  { tur: 'metin', ad: 'Danger buton', sec: 'button.bg-\\[var\\(--color-bad\\)\\]' },
]);

fs.writeFileSync(`${CIKTI}/kontrast-${ETIKET}.json`, JSON.stringify(sonuc, null, 2));

const satir = (r) => {
  if (r.tur === 'kenar') {
    const g1 = r.kenarDolgu >= r.esik ? 'GECTI' : 'KALDI';
    const g2 = r.kenarDisari >= r.esik ? 'GECTI' : 'KALDI';
    return `  [${r.etiket}] ${r.ad} — kenar ${r.on} / dolgu ${r.dolgu} / dis ${r.disari}\n` +
           `        kenar|dolgu ${r.kenarDolgu} ${g1}   kenar|dis ${r.kenarDisari} ${g2}   (esik ${r.esik}, ${r.adet}x)`;
  }
  const g = r.olculen >= r.esik ? 'GECTI' : 'KALDI';
  return `  ${String(r.olculen).padStart(6)} ${g.padEnd(5)} esik ${r.esik}  ${r.ad}: "${r.ornek}"  ${r.on} / ${r.arka} (${r.adet}x)`;
};
let rapor = `# KONTRAST OLCUMU — ${ETIKET}\n`;
for (const [bolum, temalar] of Object.entries(sonuc)) {
  if (bolum.startsWith('_')) { rapor += `\n## ${bolum}\n  ${JSON.stringify(temalar)}\n`; continue; }
  rapor += `\n## ${bolum}\n`;
  for (const [tema, satirlar] of Object.entries(temalar)) {
    rapor += ` [${tema === 'dark' ? 'KOYU' : 'ACIK'}]\n`;
    if (!satirlar.length) rapor += '  (hedef bulunamadi)\n';
    for (const r of satirlar) rapor += satir(r) + '\n';
  }
}
fs.writeFileSync(`${CIKTI}/kontrast-${ETIKET}.txt`, rapor);
console.log(rapor);

await tarayici.close();
