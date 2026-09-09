// k6 yük senaryoları — tek dosya, SENARYO ortam değişkeni ile seçilir.
//
//   k6 run -e SENARYO=katalog load/senaryo.js
//
// Senaryolar AYRI koşumlar hâlinde çalıştırılır (scripts/yuk-testi.sh böyle
// yapar). Tek koşumda karıştırmak, NFR-800'ün uç nokta bazlı p95 hedefleriyle
// karşılaştırmayı imkânsız kılardı: yavaş bir uç, hızlı olanın örneklerini
// seyrelterek toplam p95'i yanıltır.

import http from 'k6/http';
import { check, sleep, fail } from 'k6';
import { Counter, Trend } from 'k6/metrics';
import {
  TABAN, KADEMELI, girisYap, oturumBasligi, vuKullanicisi,
} from './ortak.js';

const SENARYO = __ENV.SENARYO || 'katalog';

/* ─────────────────────────── Tohum verisi ─────────────────────────── */

function guvenliOku(yol) {
  try {
    return JSON.parse(open(yol));
  } catch (e) {
    return null;
  }
}

const KULLANICILAR = guvenliOku(__ENV.TOHUM_DOSYA || './sonuclar/kullanicilar.json');
const SIPARISLER = guvenliOku(__ENV.SIPARIS_DOSYA || './sonuclar/siparisler.json');

/* ─────────────────────────── Ölçütler ─────────────────────────── */

const hizLimiti429 = new Counter('hiz_limiti_429');
const stokYok = new Counter('stok_yok');
const bakiyeYetersiz = new Counter('bakiye_yetersiz');
const siparisBasarili = new Counter('siparis_basarili');
const sseTutulan = new Trend('sse_tutulan_saniye');
const sseHata = new Counter('sse_hata');

/* ─────────────────────────── Yük profilleri ─────────────────────────── */

// SSE koşumu kademeli DEĞİLDİR: ölçülen şey gecikme değil, aynı anda AÇIK
// TUTULAN bağlantı sayısının sunucuya maliyetidir (bağlantı başına goroutine
// + Redis aboneliği). Kademeli çıkış, tepe noktasında geçirilen süreyi
// kısaltarak tam da ölçmek istediğimiz şeyi küçültürdü.
const SSE_VU = Number(__ENV.SSE_VU || 50);
const SSE_SURE = Number(__ENV.SSE_SURE || 45);

// KISA=1: betiğin kendisini denemek için kısa profil. ÖLÇÜM DEĞİLDİR —
// bu profille alınan sayı rapora girmez, sistem ısınmaya bile fırsat bulamaz.
const KADEME = __ENV.KISA
  ? [{ duration: '5s', target: 3 }, { duration: '5s', target: 3 }, { duration: '3s', target: 0 }]
  : KADEMELI;

const profiller = {
  katalog: { executor: 'ramping-vus', startVUs: 0, stages: KADEME, exec: 'katalog' },
  panel: { executor: 'ramping-vus', startVUs: 0, stages: KADEME, exec: 'panel' },
  teklif: { executor: 'ramping-vus', startVUs: 0, stages: KADEME, exec: 'teklif' },
  satinalma: { executor: 'ramping-vus', startVUs: 0, stages: KADEME, exec: 'satinalma' },
  sse: {
    executor: 'constant-vus',
    vus: SSE_VU,
    duration: `${SSE_SURE + 10}s`,
    exec: 'sse',
  },
};

// NFR-800 hedefleri (docs/trd.md §NFR-800). Sayılar BURADA UYDURULMAZ,
// dokümandan gelir:
//   fiyat teklifi   p95 < 1500 ms
//   satın alma      p95 < 3000 ms
//   diğer okumalar  p95 <  200 ms
const esikler = {
  katalog: {
    'http_req_duration{uc:services}': ['p(95)<200'],
    'http_req_duration{uc:countries}': ['p(95)<200'],
    http_req_failed: ['rate<0.01'],
  },
  panel: {
    'http_req_duration{uc:me}': ['p(95)<200'],
    'http_req_duration{uc:balance}': ['p(95)<200'],
    'http_req_duration{uc:orders}': ['p(95)<200'],
    http_req_failed: ['rate<0.01'],
  },
  teklif: {
    'http_req_duration{uc:quote}': ['p(95)<1500'],
    http_req_failed: ['rate<0.01'],
  },
  satinalma: {
    'http_req_duration{uc:quote}': ['p(95)<1500'],
    'http_req_duration{uc:order}': ['p(95)<3000'],
    http_req_failed: ['rate<0.01'],
  },
  // SSE'de http_req_failed eşiği YOKTUR: uzun ömürlü akış istemci tarafında
  // zaman aşımıyla kapatılır ve k6 bunu "başarısız istek" sayar. Buradaki
  // başarı ölçütü sse_hata sayacıdır (bağlantı hiç kurulamadı mı).
  sse: {
    sse_hata: ['count<1'],
  },
};

export const options = {
  scenarios: { [SENARYO]: profiller[SENARYO] },
  thresholds: esikler[SENARYO],
  // Özet çıktısında p99 da olsun — NFR yalnız p95 diyor ama kuyruk
  // davranışını p99 olmadan okumak mümkün değil.
  summaryTrendStats: ['min', 'med', 'avg', 'p(90)', 'p(95)', 'p(99)', 'max'],
  discardResponseBodies: false,
  noConnectionReuse: false,
};

/* ─────────────────────────── setup ─────────────────────────── */

export function setup() {
  if (SENARYO === 'katalog') return { oturumlar: [] };

  if (!KULLANICILAR || KULLANICILAR.length === 0) {
    fail('tohum kullanıcı dosyası boş — önce ./scripts/yuk-testi.sh tohum çalıştırın');
  }
  // OTURUMLAR TOHUMDAN GELİR, BURADA AÇILMAZ.
  //
  // İlk sürüm setup() içinde 50 giriş yapıyordu ve 31. istekte 429 aldı:
  // /auth/* IP başına 30 istek/dk (NFR-802). k6 setup'ta atılan istisna tüm
  // koşumu düşürdüğü için senaryo hiç başlamadı. Ölçüm bunu bir "hata oranı"
  // olarak değil, sistemin gerçek bir tavanı olarak raporlar
  // (docs/yuk-testi-sonuc.md §6.1).
  const oturumlar = KULLANICILAR.map((k) => ({
    email: k.email,
    publicId: k.publicId,
    sid: k.sid || girisYap(k),
  }));
  return { oturumlar, siparisler: SIPARISLER || [] };
}

/* ─────────────────────────── 1. Katalog gezinme ─────────────────────────── */

// Oturumsuz ziyaretçi: servisleri görür, ülkeleri görür, düşünür.
export function katalog() {
  const r1 = http.get(`${TABAN}/catalog/services-in-stock`, { tags: { uc: 'services' } });
  check(r1, { 'servisler 200': (r) => r.status === 200 });

  sleep(0.5 + Math.random());

  const r2 = http.get(`${TABAN}/catalog/countries`, { tags: { uc: 'countries' } });
  check(r2, { 'ülkeler 200': (r) => r.status === 200 });

  sleep(1 + Math.random());
}

/* ─────────────────────────── 2. Oturumlu panel ─────────────────────────── */

export function panel(veri) {
  const o = vuKullanicisi(veri.oturumlar, __VU);
  const h = oturumBasligi(o.sid);

  const r1 = http.get(`${TABAN}/me`, { headers: h, tags: { uc: 'me' } });
  check(r1, { '/me 200': (r) => r.status === 200 });

  const r2 = http.get(`${TABAN}/wallet/balance`, { headers: h, tags: { uc: 'balance' } });
  check(r2, { 'bakiye 200': (r) => r.status === 200 });

  const r3 = http.get(`${TABAN}/orders?limit=20`, { headers: h, tags: { uc: 'orders' } });
  check(r3, { 'siparişler 200': (r) => r.status === 200 });

  sleep(1 + Math.random());
}

/* ─────────────────────────── 3. Fiyat teklifi ─────────────────────────── */

// Teklif kullanıcı başına 60/dk ile sınırlıdır (NFR-802). Bekleme süresi
// bunun ALTINDA tutulur; amaç uç noktanın gecikmesini ölçmek, hız limitini
// ölçmek değil. Limite takılırsak 429 sayacı bunu raporda gösterir.
const TEKLIF_BEKLEME = Number(__ENV.TEKLIF_BEKLEME || 1.2);

const kombinasyonlar = [
  { s: 'tg', c: 'RU' },
  { s: 'tg', c: 'UA' },
  { s: 'ig', c: 'RU' },
  { s: 'go', c: 'RU' },
  { s: 'fb', c: 'TR' },
];

function rastgeleKombinasyon() {
  return kombinasyonlar[Math.floor(Math.random() * kombinasyonlar.length)];
}

function teklifAl(h) {
  const k = rastgeleKombinasyon();
  const r = http.get(
    `${TABAN}/catalog/quote?serviceCode=${k.s}&countryIso=${k.c}`,
    { headers: h, tags: { uc: 'quote' } },
  );
  if (r.status === 429) hizLimiti429.add(1);
  if (r.status === 409) stokYok.add(1);
  return r;
}

export function teklif(veri) {
  const o = vuKullanicisi(veri.oturumlar, __VU);
  const r = teklifAl(oturumBasligi(o.sid));
  check(r, { 'teklif 200': (x) => x.status === 200 });
  sleep(TEKLIF_BEKLEME);
}

/* ─────────────────────────── 4. Satın alma ─────────────────────────── */

// Sipariş kullanıcı başına 20/dk ile sınırlıdır (NFR-802) → VU başına en
// fazla ~0,33 istek/sn. Varsayılan bekleme bunun iki katı güvenli tarafta:
// FAKE sağlayıcının süreç içi bakiyesi (100 USD) toplam ~1050 satın alma
// sonrası tükenir ve o noktadan sonra ölçtüğümüz şey hata yolu olur.
const SATINALMA_BEKLEME = Number(__ENV.SATINALMA_BEKLEME || 6);

export function satinalma(veri) {
  const o = vuKullanicisi(veri.oturumlar, __VU);
  const h = oturumBasligi(o.sid);

  const q = teklifAl(h);
  if (q.status !== 200) {
    sleep(SATINALMA_BEKLEME);
    return;
  }
  const quoteId = q.json('quoteId');

  const r = http.post(
    `${TABAN}/orders`,
    JSON.stringify({ quoteId }),
    { headers: h, tags: { uc: 'order' } },
  );
  if (r.status === 429) hizLimiti429.add(1);
  if (r.status === 200) siparisBasarili.add(1);
  if (r.status === 402) bakiyeYetersiz.add(1);
  if (r.status === 409) stokYok.add(1);

  check(r, { 'sipariş 200': (x) => x.status === 200 });
  sleep(SATINALMA_BEKLEME);
}

/* ─────────────────────────── 5. SSE ─────────────────────────── */

// Her VU bir siparişin olay akışını AÇIK TUTAR. k6'nın SSE modülü yok;
// uzun zaman aşımlı bir GET aynı sunucu maliyetini üretir (goroutine +
// Redis aboneliği + açık soket) ve ölçmek istediğimiz şey tam olarak budur.
// Gecikme değil, KAPASİTE ölçülür: eş zamanlı bağlantı başına bellek,
// goroutine ve Redis istemcisi. Örnekleyici (load/olcum.sh) bunları koşum
// boyunca 2 saniyede bir kaydeder.
export function sse(veri) {
  const o = vuKullanicisi(veri.oturumlar, __VU);
  const siparisler = veri.siparisler || [];
  if (siparisler.length === 0) {
    sseHata.add(1);
    fail('SSE tohumu yok — önce ./scripts/yuk-testi.sh tohum çalıştırın');
  }
  const siparis = siparisler[(__VU - 1) % siparisler.length];

  const bas = Date.now();
  const r = http.get(`${TABAN}/orders/${siparis.orderId}/stream`, {
    headers: { Cookie: `sid=${o.sid}`, Accept: 'text/event-stream' },
    timeout: `${SSE_SURE}s`,
    tags: { uc: 'sse' },
  });
  const gecen = (Date.now() - bas) / 1000;

  // 1050 = k6'nın istek zaman aşımı kodu. Akışın SÜRE BOYUNCA AÇIK kaldığı
  // anlamına gelir — beklenen sonuç budur. 404/401 ise akış hiç kurulamadı.
  if (r.error_code === 1050) {
    sseTutulan.add(gecen);
  } else {
    sseHata.add(1);
    console.error(`sse beklenmedik sonuç: status=${r.status} err=${r.error_code}`);
  }
}

// SENARYO'ya karşılık gelen exec fonksiyonu options içinde seçilir; k6 yine
// de bir varsayılan dışa aktarım bekler.
export default function () {
  katalog();
}
