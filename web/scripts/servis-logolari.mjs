/**
 * Servis logolarını ÜRETİR — çalışma anında dış servise gidilmez.
 *
 * NEDEN BÖYLE
 * ───────────
 * Referans sitede logolar kendi sunucularında duruyor. Biz de aynısını
 * yapıyoruz. Logoları ÇALIŞMA ANINDA bir CDN'den çekmek üç sorun üretirdi:
 *   1. Her ziyaretçinin IP'si üçüncü tarafa gider (gizlilik).
 *   2. O servis düşerse sitemizde yüzlerce kırık ikon olur.
 *   3. CSP'yi dış alan adlarına açmak gerekir.
 * Bu yüzden ikonlar BİR KEZ indirilip `web/public/servis-logolari/` altına yazılır.
 *
 * ÜÇ KATMAN — sırayla denenir
 * ───────────────────────────
 *   1. simple-icons  → vektör, markanın resmi rengiyle. EN İYİSİ.
 *      Ama büyük markaların çoğu (Amazon, Microsoft, LinkedIn, Adobe…) marka
 *      sahibi talebiyle bu setten ÇIKARILMIŞ durumda — tek başına %17 kapsıyor.
 *   2. favicon       → markanın kendi sitesindeki simge. Kapsamı geniş.
 *   3. harf rozeti   → hiçbiri bulunamazsa arayüz kendi çizer (dosya yazılmaz).
 *
 * KULLANIM
 *   node scripts/servis-logolari.mjs                 # üret + SQL yaz
 *   node scripts/servis-logolari.mjs --uygula        # üret + veritabanına yaz
 *   node scripts/servis-logolari.mjs --eksikleri-goster
 *   node scripts/servis-logolari.mjs --sadece-vektor # ağa hiç çıkma
 */

import * as si from 'simple-icons';
import { writeFileSync, mkdirSync, readdirSync, unlinkSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';

const HERE = dirname(fileURLToPath(import.meta.url));
const OUT_DIR = join(HERE, '..', 'public', 'servis-logolari');
const API = process.env.API_ORIGIN ?? 'http://127.0.0.1:8091';
const VEKTOR_ONLY = process.argv.includes('--sadece-vektor');

/* ═══════════════════════ Normalizasyon ═══════════════════════ */

const normalize = (s) =>
  s
    .toLocaleLowerCase('tr')
    .replace(/ı/g, 'i').replace(/İ/g, 'i').replace(/ş/g, 's')
    .replace(/ğ/g, 'g').replace(/ü/g, 'u').replace(/ö/g, 'o').replace(/ç/g, 'c')
    .replace(/[^a-z0-9]/g, '');

/* ═══════════════════════ Katman 1: simple-icons ═══════════════════════ */

const iconIndex = new Map();
for (const key of Object.keys(si)) {
  if (!key.startsWith('si')) continue;
  const icon = si[key];
  if (!icon?.path || !icon?.title) continue;
  for (const alias of [icon.title, icon.slug].filter(Boolean)) {
    const k = normalize(alias);
    if (k && !iconIndex.has(k)) iconIndex.set(k, icon);
  }
}

/**
 * Elle eşleme tablosu.
 *
 * Sağlayıcının servis adları marka adlarıyla birebir örtüşmüyor:
 * "Google,youtube,Gmail" tek bir servistir, "Instagram+Threads" da öyle.
 * Otomatik eşleme bunları bulamaz; bulmuş gibi yapmak YANLIŞ logo koymaktır.
 *
 * Değer biçimleri:
 *   'slug'          → simple-icons slug'ı
 *   {d:'alan.com'}  → doğrudan favicon alan adı
 *   null            → marka değil, logo konmaz
 */
const OVERRIDES = {
  ot: null, full: null, any: null,
  go: 'google', ig: 'instagram', wa: 'whatsapp', tg: 'telegram', fb: 'facebook',
  lf: 'tiktok', ds: 'discord', vi: 'viber', we: 'wechat',
  am: { d: 'amazon.com' }, mm: { d: 'microsoft.com' }, mb: { d: 'yahoo.com' },
  dr: { d: 'openai.com' }, tn: { d: 'linkedin.com' }, ya: { d: 'yandex.com' },
  me: { d: 'line.me' }, za: { d: 'jd.com' }, pf: { d: 'pof.com' },
  ki: { d: '99app.com' }, asj: { d: 't-online.de' }, im: { d: 'imo.im' },
  gp: { d: 'ticketmaster.com' }, byh: { d: 'adobe.com' },
  aez: { d: 'shein.com' }, mo: { d: 'bumble.com' }, yw: { d: 'grindr.com' },
  xd: { d: 'tokopedia.com' }, dl: { d: 'lazada.com' }, sg: { d: 'ozon.ru' },
  uu: { d: 'wildberries.ru' }, cq: { d: 'mercadolibre.com' },
  fr: { d: 'dana.id' }, wr: { d: 'walmart.com' }, qf: { d: 'xiaohongshu.com' },
  kf: { d: 'weibo.com' }, vz: { d: 'hinge.co' }, fd: { d: 'mamba.ru' },
  xh: { d: 'ovo.id' }, xk: { d: 'didiglobal.com' }, df: { d: 'happn.com' },
  ue: { d: 'onet.pl' }, bz: { d: 'blizzard.com' }, aba: { d: 'rappi.com' },
  ada: { d: 'truthsocial.com' }, jq: { d: 'paysafecard.com' },
  do: { d: 'leboncoin.fr' }, tm: { d: 'akulaku.com' }, blw: { d: 'bp.com' },
  lj: { d: 'santander.com' }, awv: { d: 'wallapop.com' }, pm: { d: 'aol.com' },
};

const parts = (name) =>
  name.split(/[+,/|&]| ve |\band\b/i).map((p) => normalize(p)).filter(Boolean);

function findVectorIcon(code, name) {
  const ov = OVERRIDES[code];
  if (ov === null) return null;
  if (typeof ov === 'string') return iconIndex.get(normalize(ov)) ?? null;
  if (ov && typeof ov === 'object') return null; // alan adı verilmiş → katman 2

  const whole = iconIndex.get(normalize(name));
  if (whole) return whole;
  for (const p of parts(name)) {
    const hit = iconIndex.get(p);
    if (hit) return hit;
  }
  return null;
}

/* ═══════════════════════ Renk ═══════════════════════ */

function luminance(hex) {
  const v = [0, 2, 4].map((i) => {
    const c = parseInt(hex.slice(i, i + 2), 16) / 255;
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * v[0] + 0.7152 * v[1] + 0.0722 * v[2];
}
const darken = (hex, f) =>
  [0, 2, 4]
    .map((i) => Math.max(0, Math.min(255, Math.round(parseInt(hex.slice(i, i + 2), 16) * f))))
    .map((c) => c.toString(16).padStart(2, '0'))
    .join('');

/**
 * Zemin rengi.
 *
 * Simple Icons markanın RESMİ rengini verir ama bazıları beyaza çok yakındır
 * ve üzerine beyaz sembol konamaz; bazıları saf siyahtır ve koyu temada
 * kaybolur. İkisini de düzeltiriz — markayı tanınmaz hâle getirmeden.
 */
function tileColor(hex) {
  const L = luminance(hex);
  if (L > 0.62) return darken(hex, 0.55);
  if (L < 0.02) return '2b2f36';
  return hex;
}

function makeSVG(icon) {
  const bg = tileColor(icon.hex);
  // Simple Icons yolu 24x24 alanda tanımlıdır. Sembolü %62 ölçekleyip
  // ortalarız: kenar boşluğu olmadan ikon tıkış görünür.
  const s = 0.62;
  const off = (24 - 24 * s) / 2;
  const label = icon.title.replace(/[<>"&]/g, '');
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" role="img" aria-label="${label}">
  <rect width="24" height="24" rx="5.4" fill="#${bg}"/>
  <g transform="translate(${off.toFixed(3)} ${off.toFixed(3)}) scale(${s})">
    <path d="${icon.path}" fill="#ffffff"/>
  </g>
</svg>
`;
}

/* ═══════════════════════ Katman 2: favicon ═══════════════════════ */

/** Servis adından alan adı tahmini. "LinkedIN" → linkedin.com */
function guessDomain(code, name) {
  const ov = OVERRIDES[code];
  if (ov && typeof ov === 'object' && ov.d) return ov.d;
  if (ov === null) return null;

  const base = normalize(parts(name)[0] ?? name);
  if (base.length < 2) return null;
  // Salt rakamdan oluşan adlar ("99", "360") alan adı tahmini için güvenilmez.
  if (/^\d+$/.test(base)) return null;
  return `${base}.com`;
}

const FAVICON = (domain) =>
  `https://www.google.com/s2/favicons?domain=${encodeURIComponent(domain)}&sz=128`;

const sha = (buf) => createHash('sha256').update(buf).digest('hex');

async function fetchIcon(domain) {
  try {
    const res = await fetch(FAVICON(domain), {
      redirect: 'follow',
      signal: AbortSignal.timeout(12_000),
      headers: { Accept: 'image/png,image/*' },
    });
    // Tanınmayan alan adı 404 + gövdede genel bir 'dünya' ikonu döner.
    // res.ok kontrolü onu eler; ayrıca parmak izi bakmaya gerek yok.
    if (!res.ok) return null;
    const buf = Buffer.from(await res.arrayBuffer());
    // PNG imzası. HTML hata sayfası veya boş yanıt yazılmamalı.
    if (buf.length < 100 || buf[0] !== 0x89 || buf[1] !== 0x50) return null;
    return buf;
  } catch {
    return null;
  }
}

/** Basit eşzamanlılık havuzu — 810 isteği sırayla atmak dakikalar sürer. */
async function pool(items, limit, fn) {
  const out = new Array(items.length);
  let next = 0;
  await Promise.all(
    Array.from({ length: Math.min(limit, items.length) }, async () => {
      while (true) {
        const i = next++;
        if (i >= items.length) return;
        out[i] = await fn(items[i], i);
      }
    }),
  );
  return out;
}

/* ═══════════════════════ Ana akış ═══════════════════════ */

const res = await fetch(`${API}/api/v1/catalog/services`, {
  headers: { Accept: 'application/json' },
  signal: AbortSignal.timeout(15_000),
});
if (!res.ok) {
  console.error(`✗ servis listesi alınamadı: HTTP ${res.status} — API çalışıyor mu?`);
  process.exit(1);
}
const { items } = await res.json();

mkdirSync(OUT_DIR, { recursive: true });
// Önceki üretimi temizle: elden çıkarılmış bir servisin logosu kalırsa dizin
// zamanla çöplüğe döner. BENIOKU.md korunur.
for (const f of readdirSync(OUT_DIR)) {
  if (f.endsWith('.svg') || f.endsWith('.png')) unlinkSync(join(OUT_DIR, f));
}

const matched = [];
const needFavicon = [];

// ── Katman 1 ──
for (const s of items) {
  const icon = findVectorIcon(s.code, s.name);
  if (icon) {
    writeFileSync(join(OUT_DIR, `${s.code}.svg`), makeSVG(icon), 'utf8');
    matched.push({ code: s.code, url: `/servis-logolari/${s.code}.svg`, src: 'vektör' });
  } else {
    needFavicon.push(s);
  }
}
console.log(`  · katman 1 (vektör): ${matched.length} servis`);

// ── Katman 2 ──
let faviconCount = 0;
const missed = [];
if (!VEKTOR_ONLY && needFavicon.length) {
  const results = await pool(needFavicon, 8, async (s) => {
    const domain = guessDomain(s.code, s.name);
    if (!domain) return null;
    const buf = await fetchIcon(domain);
    if (!buf) return null;
    return { s, buf };
  });

  // YER TUTUCU AYIKLAMA.
  //
  // Tahmin edilen alan adı park edilmişse (GoDaddy vb.) o kayıt sitesinin
  // kendi ikonu döner. Tek tek bakınca gerçek bir logo gibi görünür; ancak
  // AYNI görsel onlarca servise düştüğünde belli olur. Bu koşuda 22 servis
  // aynı GoDaddy ikonunu almıştı.
  //
  // Kural: aynı görsel 3 veya daha fazla servise düşüyorsa YER TUTUCUDUR.
  // İki servisin aynı ikonu alması meşrudur (aynı markanın iki kaydı).
  const byHash = new Map();
  for (const r of results) {
    if (!r) continue;
    const h = sha(r.buf);
    (byHash.get(h) ?? byHash.set(h, []).get(h)).push(r);
  }
  let placeholders = 0;
  for (const [, group] of byHash) {
    if (group.length >= 3) { placeholders += group.length; continue; }
    for (const r of group) {
      writeFileSync(join(OUT_DIR, `${r.s.code}.png`), r.buf);
      matched.push({ code: r.s.code, url: `/servis-logolari/${r.s.code}.png`, src: 'favicon' });
      faviconCount++;
    }
  }
  if (placeholders) {
    console.log(`  · ${placeholders} yer tutucu ikon elendi (park edilmiş alan adları)`);
  }
  for (const s of needFavicon) {
    if (!matched.some((m) => m.code === s.code)) missed.push(`${s.code} — ${s.name}`);
  }
  console.log(`  · katman 2 (favicon): ${faviconCount} servis`);
} else {
  missed.push(...needFavicon.map((s) => `${s.code} — ${s.name}`));
}

const pct = ((matched.length / items.length) * 100).toFixed(1);
console.log(`\n  ✓ ${matched.length}/${items.length} servis logolu (%${pct})`);
console.log(`  · ${missed.length} servis harf rozeti gösterecek`);

// SQL: eşleşmeyenlerin icon_url'ini BOŞALT.
//
// Boşaltılmazsa, bir logo dosyası silindiğinde veritabanı hâlâ ona işaret eder
// ve arayüz kırık ikon gösterir. Harf rozeti, kırık ikondan iyidir.
const sql =
  `-- ÜRETİLEN DOSYA — elle düzenlemeyin.\n` +
  `-- Kaynak: web/scripts/servis-logolari.mjs\n` +
  `UPDATE services SET icon_url = '' WHERE icon_url <> '';\n` +
  matched
    .map((m) => `UPDATE services SET icon_url = '${m.url}' WHERE code = '${m.code.replace(/'/g, "''")}';`)
    .join('\n') + '\n';

const sqlPath = join(HERE, 'servis-logolari.sql');
writeFileSync(sqlPath, sql, 'utf8');
console.log(`  · SQL: ${sqlPath}`);

if (process.argv.includes('--uygula')) {
  try {
    execFileSync(
      'docker',
      ['exec', '-i', 'smsplatform-dev-postgres-1', 'psql', '-U', 'smsplatform', '-d', 'smsplatform', '-q'],
      { input: sql, stdio: ['pipe', 'inherit', 'inherit'] },
    );
    console.log('  ✓ veritabanına uygulandı');
  } catch {
    console.error('  ✗ veritabanına yazılamadı — SQL dosyasını elle çalıştırın');
    process.exit(1);
  }
}

if (missed.length && process.argv.includes('--eksikleri-goster')) {
  console.log('\n  Eşleşmeyenler:');
  for (const m of missed.slice(0, 80)) console.log('   ', m);
  if (missed.length > 80) console.log(`    … ve ${missed.length - 80} tane daha`);
}
