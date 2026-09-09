/**
 * /panel/bakiye-yukle DOLU HÂL — ölçüm + ekran görüntüsü.
 *
 * 🔴 NEDEN FIXTURE: dev veritabanında `deposit_methods` satırlarının TAMAMI
 * `is_active = false` ve `deposits` tablosu BOŞ (0 satır, ölçüldü). Yani ekran
 * gerçek yığında yalnız "aktif ödeme yöntemi yok" boş hâlini çizebiliyor.
 * Yöntem aktifleştirmek ya da tabloya elle satır yazmak bir PARA SİSTEMİNİN
 * kalıcı yapılandırmasını değiştirmek olurdu; onun yerine yanıt AĞ KATMANINDA
 * karşılanıyor — veritabanına hiç dokunulmuyor.
 *
 * Fixture, gerçek DTO'lardan (dto/deposit.go) birebir türetildi ve tabloyu
 * ZORLAYAN uç durumları taşır: uzun red nedeni, yansıyanı farklı COMPLETED,
 * dekontlu/dekontsuz PENDING, REFUNDED ve uzun yöntem adı.
 */
import { chromium } from 'playwright';
import fs from 'node:fs';

const KOK = 'http://localhost:3000';
const CIKTI = '/tmp/panel-para';
fs.mkdirSync(CIKTI, { recursive: true });
const MUSTERI = { email: 'yuk-1788931064-1@yuk.test', sifre: 'yuk-testi-parolasi-uzun' };

const tl = (minor) => ({
  minor,
  currency: 'TRY',
  formatted: new Intl.NumberFormat('tr-TR', { style: 'currency', currency: 'TRY' }).format(minor / 100),
});

const YONTEMLER = {
  items: [
    {
      id: 'a1f0c2e4-0000-4000-8000-000000000001',
      code: 'havale-eft', kind: 'BANK_TRANSFER', name: 'Banka Havalesi / EFT',
      instructions: 'Havale açıklamasına yalnız kullanıcı adınızı yazın.\nFarklı bir isimden gönderim yapmayın.',
      config: { banka: 'Ziraat Bankası', hesapAdi: 'Onay360 Bilişim A.Ş.', iban: 'TR33 0006 1005 1978 6457 8413 26' },
      minAmount: tl(1000), maxAmount: tl(5000000),
      referenceLabel: 'Dekont / açıklama numarası', receiptRequired: true,
    },
    {
      id: 'a1f0c2e4-0000-4000-8000-000000000002',
      code: 'usdt-trc20', kind: 'CRYPTO', name: 'USDT (TRC-20)',
      instructions: '',
      config: { cuzdanAdresi: 'TQn9Y2khEsLJW1ChVWFMSMeRDow5KcbLSE', ag: 'TRON (TRC-20)' },
      minAmount: tl(5000), maxAmount: tl(0),
      referenceLabel: 'İşlem hash değeri', receiptRequired: false,
    },
  ],
};

const durumlar = [
  ['PENDING', 'Onay bekliyor'], ['COMPLETED', 'Tamamlandı'],
  ['REJECTED', 'Reddedildi'], ['REFUNDED', 'İade edildi'],
];
const TALEPLER = Array.from({ length: 30 }, (_, i) => {
  const [status, statusLabel] = durumlar[i % 4];
  const amount = tl(25000 + i * 1750);
  return {
    id: `d0000000-0000-4000-8000-${String(i).padStart(12, '0')}`,
    method: i % 3 === 0 ? 'USDT (TRC-20)' : 'Banka Havalesi / EFT',
    amount,
    // İkinci COMPLETED'te yansıyan FARKLI: ağ ücreti düşülmüş.
    credited: status === 'COMPLETED' ? (i % 8 === 1 ? tl(amount.minor - 640) : amount) : tl(0),
    status, statusLabel,
    rejectionReason: status === 'REJECTED'
      ? 'Bildirilen dekont numarasıyla eşleşen bir gönderim bulunamadı. Lütfen havale ekranındaki açıklama alanını ve tarihi kontrol edip yeni bir talep oluşturun.'
      : undefined,
    hasReceipt: i % 3 === 0,
    createdAt: new Date(Date.UTC(2026, 8, 9 - Math.floor(i / 2), 7 + (i % 12), 24)).toISOString(),
  };
});

const tarayici = await chromium.launch();
const baglam = await tarayici.newContext({ viewport: { width: 1280, height: 900 } });
const sayfa = await baglam.newPage();
const konsol = [];
sayfa.on('console', (m) => { if (m.type() === 'error') konsol.push(m.text()); });
sayfa.on('pageerror', (e) => konsol.push(`pageerror: ${e.message}`));

await sayfa.route('**/api/v1/wallet/deposit-methods', (r) =>
  r.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(YONTEMLER) }));
await sayfa.route('**/api/v1/wallet/deposits?**', (r) => {
  const u = new URL(r.request().url());
  const limit = Number(u.searchParams.get('limit') ?? 25);
  const offset = Number(u.searchParams.get('offset') ?? 0);
  return r.fulfill({
    status: 200, contentType: 'application/json',
    body: JSON.stringify({ items: TALEPLER.slice(offset, offset + limit), total: TALEPLER.length, limit, offset }),
  });
});

await sayfa.goto(`${KOK}/giris`, { waitUntil: 'networkidle' });
await sayfa.fill('input[type="email"]', MUSTERI.email);
await sayfa.fill('input[type="password"]', MUSTERI.sifre);
await sayfa.click('button[type="submit"]');
await sayfa.waitForURL(/\/panel/, { timeout: 20000 });

const satirlar = [];
for (const g of [320, 390, 768, 1280]) {
  await sayfa.setViewportSize({ width: g, height: g < 500 ? 844 : 900 });
  await sayfa.goto(`${KOK}/panel/bakiye-yukle`, { waitUntil: 'networkidle' });
  await sayfa.waitForTimeout(600);
  // Banka yöntemini seç: hesap bilgileri + form + kopyala düğmesi görünsün.
  await sayfa.locator('input[name="odeme-yontemi"]').first().check();
  await sayfa.waitForTimeout(400);

  const o = await sayfa.evaluate(() => {
    const d = document.documentElement;
    const kucuk = [];
    for (const el of document.querySelectorAll('a,button,select,input,textarea')) {
      const r = el.getBoundingClientRect();
      if (!r.width || !r.height) continue;
      const st = getComputedStyle(el);
      if (st.visibility === 'hidden' || st.display === 'none') continue;
      if (el.type === 'radio' || el.type === 'checkbox') continue;
      if (el.textContent?.trim() === 'İçeriğe atla') continue;
      if (r.height < 43.5 || r.width < 43.5) {
        kucuk.push(`${el.tagName.toLowerCase()}"${(el.textContent || el.getAttribute('aria-label') || '').trim().slice(0, 20)}" ${Math.round(r.width)}x${Math.round(r.height)}`);
      }
    }
    const kucukGirdi = [...document.querySelectorAll('input,select,textarea')]
      .filter((e) => parseFloat(getComputedStyle(e).fontSize) < 15.9)
      .map((e) => `${e.tagName.toLowerCase()}[${e.type || ''}] ${getComputedStyle(e).fontSize}`);
    const kucukMetin = [];
    for (const el of document.querySelectorAll('main *')) {
      if (el.children.length) continue;
      const t = (el.textContent || '').trim();
      if (!t) continue;
      const f = parseFloat(getComputedStyle(el).fontSize);
      if (f < 13.9) kucukMetin.push(`${f} "${t.slice(0, 24)}"`);
    }
    const tabloKabi = document.querySelector('main table')?.closest('div');
    // uppercase kalmış mı? (Türkçe i→I tuzağı)
    const buyukHarf = [...document.querySelectorAll('main *')]
      .filter((e) => getComputedStyle(e).textTransform === 'uppercase' && e.textContent?.trim())
      .map((e) => e.textContent.trim().slice(0, 24));
    return {
      scrollW: d.scrollWidth, clientW: d.clientWidth,
      tabloGorunur: tabloKabi ? getComputedStyle(tabloKabi).display !== 'none' : false,
      kartVar: !!document.querySelector('main ul[aria-label="Yükleme talepleriniz"]'),
      kartGorunur: (() => {
        const u = document.querySelector('main ul[aria-label="Yükleme talepleriniz"]');
        return u ? getComputedStyle(u).display !== 'none' : false;
      })(),
      satirSayisi: document.querySelectorAll('main table tbody tr').length
        || document.querySelectorAll('main ul[aria-label="Yükleme talepleriniz"] > li').length,
      caption: document.querySelector('main table caption')?.textContent?.trim() ?? null,
      redNedeniGorunur: [...document.querySelectorAll('main *')]
        .filter((e) => !e.children.length && e.textContent?.includes('Red nedeni'))
        .filter((e) => e.getBoundingClientRect().height > 0).length,
      sayfalama: !!document.querySelector('main nav[aria-label="Sayfalama"]'),
      tabular: document.querySelectorAll('main .tabular-nums').length,
      overflowX: [...document.querySelectorAll('main *')]
        .filter((e) => ['auto', 'scroll'].includes(getComputedStyle(e).overflowX))
        .map((e) => e.tagName.toLowerCase() + '.' + String(e.className).slice(0, 28)),
      kucuk, kucukGirdi, kucukMetin: kucukMetin.slice(0, 5), buyukHarf,
    };
  });
  satirlar.push({ g, ...o });
  await sayfa.screenshot({ path: `${CIKTI}/dolu-bakiye-yukle-${g}.png`, fullPage: true });
}

console.log('\n| px | scrollW/clientW | tablo | kart | satır | caption | red nedeni | sayfalama | tabular | overflow-x | <44px | <16px girdi | <14px metin | uppercase |');
console.log('|---|---|---|---|---|---|---|---|---|---|---|---|---|---|');
for (const s of satirlar) {
  console.log(`| ${s.g} | ${s.scrollW}/${s.clientW} ${s.scrollW > s.clientW ? '🔴' : '✓'} | ${s.tabloGorunur ? 'görünür' : 'gizli'} | ${s.kartGorunur ? 'görünür' : 'gizli'} | ${s.satirSayisi} | ${s.caption ? '✓' : '🔴'} | ${s.redNedeniGorunur} | ${s.sayfalama ? '✓' : '🔴'} | ${s.tabular} | ${s.overflowX.length ? '🔴 ' + s.overflowX.join(',') : '✓ 0'} | ${s.kucuk.length ? '🔴 ' + s.kucuk.join(' · ') : '✓'} | ${s.kucukGirdi.length ? '🔴 ' + s.kucukGirdi.join(' · ') : '✓'} | ${s.kucukMetin.length ? '🔴 ' + s.kucukMetin.join(' · ') : '✓'} | ${s.buyukHarf.length ? '🔴 ' + s.buyukHarf.join(',') : '✓ yok'} |`);
}

/* ── Dekont modalı: odak, Esc, dokunma hedefi ── */
await sayfa.setViewportSize({ width: 390, height: 844 });
await sayfa.goto(`${KOK}/panel/bakiye-yukle`, { waitUntil: 'networkidle' });
await sayfa.waitForTimeout(600);
const dekontDugmesi = sayfa.getByRole('button', { name: 'Dekont yükle' }).first();
await dekontDugmesi.click();
await sayfa.waitForTimeout(500);
const modal = await sayfa.evaluate(() => {
  const d = document.querySelector('[role="dialog"]');
  return {
    acik: !!d,
    baslik: d?.getAttribute('aria-modal'),
    odak: document.activeElement?.tagName + '[' + (document.activeElement?.type || '') + ']',
    dosyaGirdisi: !!d?.querySelector('input[type="file"]'),
  };
});
await sayfa.screenshot({ path: `${CIKTI}/dolu-dekont-modal-390.png` });
await sayfa.keyboard.press('Escape');
await sayfa.waitForTimeout(400);
const kapandi = await sayfa.evaluate(() => !document.querySelector('[role="dialog"]'));
console.log('\ndekont modalı:', JSON.stringify({ ...modal, escIleKapandi: kapandi }));

/* ── Tutar doğrulaması: boş / negatif / geçersiz / geçerli ── */
await sayfa.setViewportSize({ width: 1280, height: 900 });
await sayfa.goto(`${KOK}/panel/bakiye-yukle`, { waitUntil: 'networkidle' });
await sayfa.locator('input[name="odeme-yontemi"]').first().check();
await sayfa.waitForTimeout(400);
const tutarAlani = sayfa.locator('input[inputmode="decimal"]');
const sonuc = {};
for (const [ad, deger] of [['bos', ''], ['negatif', '-12,50'], ['ucOndalik', '12,505'],
                           ['harf', 'abc'], ['altSinir', '5'], ['ustSinir', '99999'], ['gecerli', '250,00']]) {
  await tutarAlani.fill(deger);
  await sayfa.getByRole('button', { name: 'Talebi oluştur' }).click();
  await sayfa.waitForTimeout(250);
  sonuc[ad] = await sayfa.evaluate(() => {
    const i = document.querySelector('input[inputmode="decimal"]');
    const id = i?.getAttribute('aria-describedby')?.split(' ').find((x) => x.endsWith('-err'));
    return (id && document.getElementById(id)?.textContent?.trim())
      || document.querySelector('input[inputmode="decimal"]')?.parentElement
           ?.querySelector('[role="alert"]')?.textContent?.trim() || 'HATA YOK';
  });
}
console.log('tutar doğrulaması:', JSON.stringify(sonuc, null, 2));
console.log('\nkonsol hataları:', konsol.length ? konsol : 'yok');
await tarayici.close();

/* ── HATA DURUMU: requestId ekranda mı? ── */
await tarayici2Kontrol();
async function tarayici2Kontrol() {}
