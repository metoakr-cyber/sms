/**
 * Teknik SEO denetimi — docs/frontend-contract.md §10.2 (S1–S9) ve §10.4.
 *
 * "%100 SEO uyumlu" ölçülemez; buradaki maddeler ölçülebilir. Betik CANLI
 * SİTEYE karşı koşar ve HTML'i JavaScript ÇALIŞTIRMADAN okur — yani bir
 * denetim geçtiyse arama motorunun gördüğü şey de geçmiştir (S9). Tarayıcı
 * kullanılsaydı istemcide üretilen içerik de sayılırdı ve denetim yalan
 * söylerdi.
 *
 * Kullanım:
 *   node scripts/seo-check.mjs [taban-url]      # varsayılan http://localhost:3000
 *
 * ⚠️ HEM `make dev` (Go API) HEM `make web` ayakta olmalı. API kapalıyken
 * katalogdan beslenen sayfalar (/, /fiyatlar, /kiralama) 500 döner ve denetim
 * bunu SEO sorunu olarak raporlar — ki doğrusu da budur: 500 dönen bir sayfa
 * indekslenmez. Ama sebebi kod değil, eksik sunucudur; önce ikisini de açın.
 *
 * Çıkış kodu: sorun yoksa 0, varsa 1.
 *
 * NOT: `scripts/check.sh` içinde DEĞİLDİR — responsive denetimi gibi ayakta
 * bir sunucu ister; birleştirme kapısını dış duruma bağlamak kapıyı rastgele
 * düşürür.
 */

const BASE = (process.argv[2] ?? process.env.SEO_BASE_URL ?? 'http://localhost:3000')
  .replace(/\/+$/, '');

/**
 * İNDEKSLENMESİ BEKLENEN sayfalar. Site haritasından TÜRETİLMEZ: site
 * haritasının kendisi denetimin konusu, dolayısıyla ölçüt olamaz. Eksik bir
 * sayfa buraya elle eklenir; böylece "haritaya koymayı unuttuk" hatası
 * denetime yakalanır.
 */
const GENEL_SAYFALAR = [
  '/', '/fiyatlar', '/kiralama', '/sss', '/hakkimizda', '/iletisim',
  '/gizlilik', '/kullanim-sartlari',
];

/** Site haritasında OLMASI beklenen yollar (§10.2 S4). */
const HARITADA_BEKLENEN = ['/', '/fiyatlar', '/kiralama', '/sss', '/hakkimizda', '/iletisim'];

/** `noindex` olması gereken sayfalar — oturum arkası her şey (§10.1). */
const GIZLI_SAYFALAR = [
  '/panel', '/panel/numara-al', '/panel/siparisler', '/panel/cuzdan', '/panel/destek',
  '/yonetim', '/yonetim/kullanicilar',
];

/** Kimlik sayfaları: indekslenmemeli ama site haritasına da girmemeli. */
const KIMLIK_SAYFALARI = ['/giris', '/sifremi-unuttum'];

const TITLE_SINIRI = 60;   // S1
const DESC_SINIRI = 160;   // S1

/* ──────────────────────── küçük yardımcılar ──────────────────────── */

const sorunlar = [];
const not = (yol, mesaj, dosya) =>
  sorunlar.push({ yol, mesaj, dosya });

function coz(html) {
  return html
    .replace(/&amp;/g, '&').replace(/&lt;/g, '<').replace(/&gt;/g, '>')
    .replace(/&quot;/g, '"').replace(/&#(?:39|x27);/g, "'");
}

/** `<meta name=… content=…>` — öznitelik SIRASI değişebilir, ona göre eşleşir. */
function meta(html, tur, ad) {
  const re = new RegExp(
    `<meta[^>]*${tur}=["']${ad.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}["'][^>]*>`, 'i');
  const etiket = html.match(re)?.[0];
  if (!etiket) return null;
  return coz(etiket.match(/content=["']([^"']*)["']/i)?.[1] ?? '');
}

function link(html, rel) {
  const etiket = html.match(new RegExp(`<link[^>]*rel=["']${rel}["'][^>]*>`, 'i'))?.[0];
  if (!etiket) return null;
  return coz(etiket.match(/href=["']([^"']*)["']/i)?.[1] ?? '');
}

/** Site haritası/canonical başka bir alan adına işaret ediyorsa taban URL'e taşır. */
function ayniKokene(u) {
  try {
    const url = new URL(u);
    return BASE + url.pathname + url.search;
  } catch {
    return BASE + (u.startsWith('/') ? u : `/${u}`);
  }
}

/** PNG başlığından gerçek boyut — "1200×630 yazdım" iddiası değil, ölçüm. */
function pngBoyutu(buf) {
  if (buf.length < 24 || buf.readUInt32BE(0) !== 0x89504e47) return null;
  return { w: buf.readUInt32BE(16), h: buf.readUInt32BE(20) };
}

async function getir(yol) {
  const url = yol.startsWith('http') ? yol : BASE + yol;
  const y = await fetch(url, { redirect: 'manual', headers: { 'accept-language': 'tr' } });
  return { durum: y.status, basliklar: y.headers, govde: y };
}

/* ──────────────────────── 1) genel sayfalar ──────────────────────── */

console.log(`─── SEO denetimi · ${BASE} ───`);

const gorulenBaslik = new Map();
const gorulenAciklama = new Map();
let ogGorseli = null;
let twGorseli = null;

for (const yol of GENEL_SAYFALAR) {
  const { durum, govde } = await getir(yol);
  if (durum !== 200) { not(yol, `HTTP ${durum} döndü (200 bekleniyordu)`); continue; }
  const html = await govde.text();

  // S1 — benzersiz ve sınır içi başlık
  const baslik = coz(html.match(/<title>([\s\S]*?)<\/title>/i)?.[1] ?? '').trim();
  if (!baslik) not(yol, '<title> yok');
  else {
    if (baslik.length > TITLE_SINIRI)
      not(yol, `title ${baslik.length} karakter (sınır ${TITLE_SINIRI}) — arama sonucunda kesilir`);
    const ilk = gorulenBaslik.get(baslik);
    if (ilk) not(yol, `title, ${ilk} ile AYNI — benzersiz olmalı (S1)`);
    else gorulenBaslik.set(baslik, yol);
  }

  // S1 — benzersiz ve sınır içi açıklama
  const aciklama = meta(html, 'name', 'description');
  if (!aciklama) not(yol, 'meta description yok');
  else {
    if (aciklama.length > DESC_SINIRI)
      not(yol, `description ${aciklama.length} karakter (sınır ${DESC_SINIRI}) — kesilir`);
    const ilk = gorulenAciklama.get(aciklama);
    if (ilk)
      not(yol, `description, ${ilk} ile AYNI — kök layout'un varsayılanı miras alınmış, ` +
               'sayfaya özel description yazılmalı (S1)');
    else gorulenAciklama.set(aciklama, yol);
  }

  // S3 — canonical, sayfanın KENDİSİNİ göstermeli
  const canonical = link(html, 'canonical');
  if (!canonical) not(yol, 'canonical yok (S3)');
  else {
    const bekleniyor = yol === '/' ? '/' : yol;
    const gelen = new URL(canonical, BASE).pathname.replace(/(.)\/$/, '$1');
    if (gelen !== bekleniyor)
      not(yol, `canonical "${gelen}" gösteriyor, "${bekleniyor}" olmalı — ` +
               'yanlış canonical sayfayı indeksten düşürür (S3)');
  }

  // S2 — tek h1
  const h1 = (html.match(/<h1[\s>]/gi) ?? []).length;
  if (h1 !== 1) not(yol, `${h1} adet <h1> var, tam olarak 1 olmalı (S2)`);

  // Genel sayfa noindex OLMAMALI
  const robots = meta(html, 'name', 'robots') ?? '';
  if (/noindex/i.test(robots))
    not(yol, `genel sayfa ama meta robots "${robots}" — indekslenmez`);

  // S6 — Open Graph + Twitter Card
  const og = meta(html, 'property', 'og:image');
  const tw = meta(html, 'name', 'twitter:image');
  if (!og) not(yol, 'og:image yok (S6)');
  if (!tw) not(yol, 'twitter:image yok (S6)');
  if (!meta(html, 'name', 'twitter:card')) not(yol, 'twitter:card yok (S6)');
  if (!meta(html, 'property', 'og:title')) not(yol, 'og:title yok (S6)');
  ogGorseli ??= og;
  twGorseli ??= tw;

  // S7 — JSON-LD blokları GEÇERLİ JSON olmalı ve @context/@type taşımalı.
  // Bozuk bir blok Google tarafından SESSİZCE atılır; hata görünmez.
  const bloklar = [...html.matchAll(
    /<script[^>]*type=["']application\/ld\+json["'][^>]*>([\s\S]*?)<\/script>/gi)];
  for (const [, ham] of bloklar) {
    let veri;
    try { veri = JSON.parse(coz(ham)); }
    catch (e) { not(yol, `JSON-LD ayrıştırılamadı: ${e.message} (S7)`); continue; }
    for (const dugum of Array.isArray(veri) ? veri : [veri]) {
      if (dugum['@context'] !== 'https://schema.org')
        not(yol, `JSON-LD @context "${dugum['@context']}" — https://schema.org olmalı (S7)`);
      if (!dugum['@type']) not(yol, 'JSON-LD @type yok (S7)');
    }
  }

  // S9 — içerik SUNUCUDA üretilmiş olmalı. h1 metni gövdede yoksa sayfa
  // istemci tarafında doluyordur ve bot boş sayfa görür.
  if (!/<h1[^>]*>\s*\S/i.test(html)) not(yol, '<h1> boş geldi — içerik sunucuda üretilmiyor (S9)');
}

/* ──────────────────────── 2) og:image gerçekten var mı ──────────────────────── */

for (const [ad, adres] of [['og:image', ogGorseli], ['twitter:image', twGorseli]]) {
  if (!adres) continue;
  const { durum, basliklar, govde } = await getir(ayniKokene(adres));
  if (durum !== 200) { not(ad, `görsel HTTP ${durum} döndü — paylaşımda boş kart çıkar`); continue; }
  const tip = basliklar.get('content-type') ?? '';
  if (!tip.startsWith('image/')) { not(ad, `content-type "${tip}" — görsel değil`); continue; }
  const boyut = pngBoyutu(Buffer.from(await govde.arrayBuffer()));
  if (!boyut) { not(ad, 'PNG başlığı okunamadı'); continue; }
  if (boyut.w !== 1200 || boyut.h !== 630)
    not(ad, `${boyut.w}×${boyut.h} — 1200×630 olmalı (S6)`);
  else console.log(`  ✓ ${ad} ${boyut.w}×${boyut.h} · ${tip}`);
}

/* ──────────────────────── 3) site haritası ──────────────────────── */

{
  const { durum, govde } = await getir('/sitemap.xml');
  if (durum !== 200) not('/sitemap.xml', `HTTP ${durum} (S4)`);
  else {
    const xml = await govde.text();
    const loclar = [...xml.matchAll(/<loc>([^<]+)<\/loc>/g)].map((m) => coz(m[1].trim()));
    if (loclar.length === 0) not('/sitemap.xml', 'hiç <loc> yok (S4)');
    const yollar = loclar.map((u) => new URL(u).pathname);

    // S4 — panel/yönetim ASLA haritada olmaz
    for (const y of yollar)
      if (/^\/(panel|yonetim|api)\b/.test(y))
        not('/sitemap.xml', `gizli yol haritada: ${y} (S4)`);

    for (const bekleniyor of HARITADA_BEKLENEN)
      if (!yollar.includes(bekleniyor))
        not('/sitemap.xml', `${bekleniyor} haritada yok — eklenmeli (S4)`);

    // Her URL gerçekten 200 dönmeli VE noindex olmamalı. Haritaya konmuş
    // noindex bir URL, Search Console'da "noindex olarak işaretlenmiş URL
    // gönderildi" hatası üretir.
    for (const u of loclar) {
      const { durum: d, basliklar, govde: g } = await getir(ayniKokene(u));
      const y = new URL(u).pathname;
      if (d !== 200) { not('/sitemap.xml', `${y} → HTTP ${d} (S12)`); continue; }
      const xr = basliklar.get('x-robots-tag') ?? '';
      const html = await g.text();
      const mr = meta(html, 'name', 'robots') ?? '';
      if (/noindex/i.test(xr) || /noindex/i.test(mr))
        not('/sitemap.xml', `${y} noindex ama haritada — biri yanlış (S4)`);
    }
    console.log(`  ✓ site haritası: ${loclar.length} URL denetlendi`);
  }
}

/* ──────────────────────── 4) panel/yönetim indekslenmiyor ──────────────────────── */

for (const yol of GIZLI_SAYFALAR) {
  const { durum, basliklar, govde } = await getir(yol);
  // Oturum yokken 200 (istemcide yönlendirme) ya da 3xx — ikisi de olabilir;
  // önemli olan HER İKİ durumda da noindex başlığının gelmesi.
  const xr = basliklar.get('x-robots-tag') ?? '';
  let mr = '';
  if (durum === 200) mr = meta(await govde.text(), 'name', 'robots') ?? '';
  if (!/noindex/i.test(xr) && !/noindex/i.test(mr))
    not(yol, `noindex YOK (X-Robots-Tag: "${xr}", meta robots: "${mr}") — ` +
             'oturum arkası sayfa indekslenebilir 🔴 (§10.1)');
}

for (const yol of KIMLIK_SAYFALARI) {
  const { durum, basliklar, govde } = await getir(yol);
  if (durum !== 200) { not(yol, `HTTP ${durum}`); continue; }
  const html = await govde.text();
  const mr = meta(html, 'name', 'robots') ?? '';
  const xr = basliklar.get('x-robots-tag') ?? '';
  if (!/noindex/i.test(mr) && !/noindex/i.test(xr))
    not(yol, `kimlik sayfası ama noindex yok (meta robots: "${mr}") — §10.1`);
  const canonical = link(html, 'canonical');
  const gelen = canonical ? new URL(canonical, BASE).pathname.replace(/(.)\/$/, '$1') : null;
  if (gelen !== yol)
    not(yol, `canonical "${gelen}" — kendini göstermeli, aksi hâlde sayfa ` +
             'ana sayfayla birleştirilir (S3)');
}

/* ──────────────────────── 5) robots.txt ──────────────────────── */

{
  const { durum, govde } = await getir('/robots.txt');
  if (durum !== 200) not('/robots.txt', `HTTP ${durum} (S5)`);
  else {
    const metin = await govde.text();
    for (const engel of ['/panel', '/yonetim', '/api'])
      if (!new RegExp(`^Disallow:\\s*${engel}`, 'im').test(metin))
        not('/robots.txt', `"Disallow: ${engel}" yok (S5)`);
    if (!/^Sitemap:\s*http/im.test(metin)) not('/robots.txt', 'Sitemap satırı yok (S5)');
    else console.log('  ✓ robots.txt: engeller ve sitemap bildirimi yerinde');
  }
}

/* ──────────────────────── 6) manifest ve simgeler ──────────────────────── */

{
  const { durum, govde } = await getir('/manifest.webmanifest');
  if (durum !== 200) not('/manifest.webmanifest', `HTTP ${durum}`);
  else {
    let m;
    try { m = JSON.parse(await govde.text()); }
    catch (e) { not('/manifest.webmanifest', `geçersiz JSON: ${e.message}`); }
    if (m) {
      for (const alan of ['name', 'short_name', 'start_url', 'display', 'theme_color'])
        if (!m[alan]) not('/manifest.webmanifest', `"${alan}" alanı yok`);
      // Chrome "ana ekrana ekle" için 192 ve 512 arar; 32'lik favicon yetmez.
      //
      // 🔴 `purpose` DE SAYILIR. İlk sürümde yalnız `sizes` kümesine bakılıyordu
      // ve denetim, 512'nin SADECE `maskable` olarak bildirildiği bir manifesti
      // geçirdi (sabotaj D). Chrome kurulabilirlik için `purpose: "any"` bir
      // simge ister; maskeli sürüm onun yerine geçmez. Yani o hâliyle denetim
      // "kurulabilir" derken uygulama kurulamıyor olurdu.
      const anyBoyutlari = new Set(
        (m.icons ?? [])
          .filter((i) => (i.purpose ?? 'any').split(/\s+/).includes('any'))
          .map((i) => i.sizes),
      );
      for (const b of ['192x192', '512x512'])
        if (!anyBoyutlari.has(b))
          not('/manifest.webmanifest',
              `${b} boyutunda purpose="any" simge yok — Chrome uygulamayı kuramaz`);
      for (const simge of new Set((m.icons ?? []).map((i) => i.src))) {
        const { durum: d, basliklar } = await getir(ayniKokene(simge));
        if (d !== 200) not('/manifest.webmanifest', `simge ${simge} → HTTP ${d}`);
        else if (!(basliklar.get('content-type') ?? '').startsWith('image/'))
          not('/manifest.webmanifest', `simge ${simge} görsel değil`);
      }
      if (!sorunlar.some((s) => s.yol === '/manifest.webmanifest'))
        console.log(`  ✓ manifest: ${(m.icons ?? []).length} simge ulaşılabilir`);
    }
  }
}

/* ──────────────────────── özet ──────────────────────── */

console.log();
if (sorunlar.length === 0) {
  console.log(`  ✓ SEO denetimi temiz: ${GENEL_SAYFALAR.length} genel sayfa, ` +
              `${GIZLI_SAYFALAR.length} gizli sayfa`);
  process.exit(0);
}
const gruplu = new Map();
for (const s of sorunlar) {
  if (!gruplu.has(s.yol)) gruplu.set(s.yol, []);
  gruplu.get(s.yol).push(s.mesaj);
}
for (const [yol, mesajlar] of gruplu) {
  console.log(`  ✗ ${yol}`);
  for (const m of mesajlar) console.log(`      ${m}`);
}
console.log(`\n  ${sorunlar.length} sorun bulundu.`);
process.exit(1);
