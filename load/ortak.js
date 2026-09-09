// Yük testi ortak yardımcıları.
//
// Buradaki her şey k6 tarafından hem init hem VU bağlamında kullanılabilir
// olmalıdır; dosya okuma (open) yalnız init bağlamında çalışır.

import http from 'k6/http';

export const TABAN = __ENV.API_TABAN || 'http://localhost:8191/api/v1';

// Tohum dosyası: scripts/yuk-testi.sh tarafından üretilir.
// Biçim: [{ email, sifre, publicId }, ...]
const TOHUM_YOLU = __ENV.TOHUM_DOSYA || './sonuclar/kullanicilar.json';

export function tohumOku() {
  return JSON.parse(open(TOHUM_YOLU));
}

// Sipariş tohumu: SSE senaryosu için önceden açılmış siparişler.
// Biçim: ["<order public_id>", ...]  (kullanıcı sırasıyla aynı dizilimde)
const SIPARIS_YOLU = __ENV.SIPARIS_DOSYA || './sonuclar/siparisler.json';

export function siparisTohumuOku() {
  return JSON.parse(open(SIPARIS_YOLU));
}

// girisYap oturum çerezinin ham değerini döner (Set-Cookie: sid=...).
//
// Çerez kavanozu yerine düz başlık kullanılır: k6'nın VU başına kavanozu
// senaryolar arasında taşınmaz ve setup() içinde kurulan oturumların VU'ya
// aktarılması gerekir.
export function girisYap(kullanici) {
  const r = http.post(
    `${TABAN}/auth/login`,
    JSON.stringify({ email: kullanici.email, password: kullanici.sifre }),
    { headers: { 'Content-Type': 'application/json' }, tags: { uc: 'login' } },
  );
  if (r.status !== 200) {
    throw new Error(`giriş başarısız (${kullanici.email}): ${r.status} ${r.body}`);
  }
  const ham = r.headers['Set-Cookie'] || '';
  const m = /sid=([^;]+)/.exec(ham);
  if (!m) throw new Error(`oturum çerezi yok (${kullanici.email})`);
  return m[1];
}

export function oturumBasligi(sid) {
  return { Cookie: `sid=${sid}`, 'Content-Type': 'application/json' };
}

// vuKullanicisi VU numarasını tohum listesine eşler.
//
// Aynı kullanıcının iki VU tarafından paylaşılması hız limitini (kullanıcı
// başına 60 teklif/dk, 20 sipariş/dk) yapay olarak tetikler ve ölçümü
// limitin kendisine dönüştürür. Bu yüzden tohum listesi en az VU sayısı
// kadar kullanıcı içermelidir; yetmezse döngüsel eşleme yapılır ve rapor
// bunu 429 sayısıyla ele verir.
export function vuKullanicisi(liste, vuID) {
  return liste[(vuID - 1) % liste.length];
}

// Ortak yük profili: 0 → 10 → 50 kullanıcı, kademeli.
//
// Ani yük yerine kademeli çıkış: bağlantı havuzu ve JIT ısınması ilk
// saniyelerde p99'u yanıltır; kademe, sistemin her seviyede durulmasına
// zaman tanır.
export const KADEMELI = [
  { duration: '20s', target: 10 },
  { duration: '40s', target: 10 },
  { duration: '20s', target: 50 },
  { duration: '60s', target: 50 },
  { duration: '10s', target: 0 },
];
