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
 * DÖRT KATMAN — sırayla denenir
 * ─────────────────────────────
 *   1. simple-icons  → vektör, markanın resmi rengiyle. EN İYİSİ.
 *      Ama büyük markaların çoğu (Amazon, Microsoft, LinkedIn, Adobe…) marka
 *      sahibi talebiyle bu setten ÇIKARILMIŞ durumda — tek başına %17 kapsıyor.
 *   2. favicon       → markanın kendi sitesindeki simge. Kapsamı geniş.
 *      İki kaynak sırayla: DuckDuckGo (kotasız), sonra Google s2.
 *   3. unavatar      → son çare. VARSAYILAN KAPALI, `UNAVATAR_ETKIN=1` ile açılır.
 *      Neden en sonda ve neden kapalı: aşağıda "UNAVATAR" bloğunda.
 *   4. harf rozeti   → hiçbiri bulunamazsa arayüz kendi çizer (dosya yazılmaz).
 *
 * KULLANIM
 *   node scripts/servis-logolari.mjs                 # üret + SQL yaz
 *   node scripts/servis-logolari.mjs --uygula        # üret + veritabanına yaz
 *   node scripts/servis-logolari.mjs --eksikleri-goster
 *   node scripts/servis-logolari.mjs --onizleme      # göz denetimi için HTML tablo
 *   node scripts/servis-logolari.mjs --sadece-vektor # ağa hiç çıkma
 *   UNAVATAR_ETKIN=1 node scripts/servis-logolari.mjs   # 3. katmanı da dene
 */

import * as si from 'simple-icons';
import { writeFileSync, mkdirSync, readdirSync, unlinkSync, statSync, existsSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';

const HERE = dirname(fileURLToPath(import.meta.url));
const OUT_DIR = join(HERE, '..', 'public', 'servis-logolari');
const API = process.env.API_ORIGIN ?? 'http://127.0.0.1:8091';
const VEKTOR_ONLY = process.argv.includes('--sadece-vektor');
const UNAVATAR_ETKIN = process.env.UNAVATAR_ETKIN === '1';

/**
 * Tek bir ikon dosyası için üst sınır.
 *
 * Bazı siteler favicon diye 400 KB'lık bir görsel sunuyor (ölçüm:
 * tngdigital.com.my → 410 598 bayt). Bu dosyalar depoya giriyor ve 32 px'lik
 * bir rozet için 400 KB taşımak saçma. Sınırı aşan ikon alınmaz; servis harf
 * rozetinde kalır.
 */
const MAX_IKON_BAYT = 150_000;

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
  lf: 'tiktok', ds: 'discord', vi: 'viber',
  // 🔴 `we` WeChat DEĞİL. WeChat `wb` kodundadır; `we` = DrugVokrug (Rus
  // mesajlaşma uygulaması). Burada `we: 'wechat'` yazılıydı ve iki servis
  // BİRBİRİNİN AYNI SVG'sini alıyordu — sha256'ları eşitti, üstelik
  // aria-label da "WeChat" diyordu, yani ekran okuyucu yanlış markayı okuyordu.
  // Yanlış marka logosu, logosuzluktan kötüdür.
  we: { d: 'drugvokrug.ru' },
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

  /* ── Kürate vektörler ───────────────────────────────────────────────────
   * simple-icons'ta marka VAR ama servis adı slug'a birebir oturmuyor.
   * Otomatik "yakın eşleşme" denenmedi çünkü felaket üretiyor: ölçümde
   * "Lightning AI" → Lightning Network, "MiniPay" → Mini (otomobil),
   * "Scalapay" → Scala (dil), "myboost" → Boost (C++ kütüphanesi) çıktı.
   * Bu yüzden yalnız ELLE doğrulanmış slug'lar yazılır. */
  ccx: 'googlemessages', gs: 'samsung', pb: 'sky', qq: 'qq', vk: 'vk',
  bob: 'shell', vg: 'shell', // Shell GO / Shell Box — aynı marka, aynı logo meşru

  /* ── Kürate alan adları ─────────────────────────────────────────────────
   * `<ad>.com` varsayımı bu servislerin %80'inde YANLIŞ alan adı üretiyordu
   * (ölçüm: 236 servisin 188'i 404). Aşağıdaki eşlemelerin her biri tek tek
   * sondalandı; yalnız gerçekten ikon dönen ve markası doğrulanan alan adları
   * yazıldı. Otomatik çoklu-TLD taraması BİLEREK yapılmadı: "BP" 5 farklı
   * TLD'de ikon döndürüyor ve hiçbiri BP değil. Para sisteminde YANLIŞ marka
   * logosu, logosuzluktan kötüdür — kullanıcı ne satın aldığını yanlış anlar. */
  aaq: { d: '163.com' }, abd: { d: 'beboo.ru' }, abh: { d: 'uol.com.br' },
  abt: { d: 'arenaplus.ph' }, abu: { d: 'bpjsketenagakerjaan.go.id' },
  acb: { d: 'walmart.com' }, acc: { d: 'luckylandslots.com' }, acd: { d: 'cloud.ru' },
  adt: { d: 'willhaben.at' }, adu: { d: 'seznam.cz' }, adz: { d: 'shoko.ru' },
  aeq: { d: 'godrejenterprises.com' }, afe: { d: 'gov.br' }, afm: { d: 'myboost.com.my' },
  afr: { d: 'ultragaz.com.br' }, aga: { d: 'publi24.ro' }, agh: { d: 'getnet.com.br' },
  agi: { d: 'njuskalo.hr' }, agj: { d: 'marktplaats.nl' }, ahx: { d: 'bitrue.com' },
  ais: { d: 'didiglobal.com' }, aiq: { d: 'primeopinion.com' }, aly: { d: 'bebeclub.co.id' },
  ana: { d: 'sicredi.com.br' },
  anw: { d: 'premmia.com.br' }, aoq: { d: 'jbhifi.com.au' }, aok: { d: 'neteller.com' },
  aps: { d: 'skelbiu.lt' }, apj: { d: 'tinkoff.ru' }, aru: { d: 'gangnamunni.com' },
  atl: { d: 'watsons.com.my' }, atu: { d: 'sber.ru' }, aup: { d: 'botim.me' },
  aux: { d: 'lightning.ai' }, avb: { d: 'tealive.com.my' }, avk: { d: 'quoka.de' },
  awg: { d: 'natura.com.br' }, axj: { d: 'fifgroup.co.id' },
  axs: { d: 'ais.th' }, axt: { d: 'gnjoy.com' }, ayn: { d: 'li.me' },
  ban: { d: 'bonuslink.com.my' }, bas: { d: 'stoiximan.gr' }, bcg: { d: '2gis.ru' },
  bcq: { d: 'kopikenangan.com' }, bd: { d: 'x5.ru' }, bdh: { d: 'single.dk' },
  bfr: { d: 'ridedott.com' }, bfv: { d: 'chocofamily.kz' }, bgt: { d: 'alfamidiku.com' },
  bhr: { d: 'dilmil.co' },
  bib: { d: 'bca.co.id' }, bic: { d: 'markt.de' },
  bla: { d: 'fdj.fr' }, bli: { d: 'scalapay.com' },
  blx: { d: '2ememain.be' }, bly: { d: 'cosmote.gr' },
  bme: { d: 'im3.id' }, bmi: { d: 'sisal.it' }, bmj: { d: 'betflag.it' },
  bn: { d: 'alfagift.id' }, bnp: { d: 'airba.kz' }, bog: { d: 'france-mobilites.fr' },
  bol: { d: 'telus.ca' }, bon: { d: 'retailmenot.com' }, bos: { d: 'casinoportugal.pt' },
  bpe: { d: 'silpo.ua' }, bpp: { d: 'bliq.app' },
  bqx: { d: 'q8.it' }, bqy: { d: 'snai.it' }, brc: { d: 'casa.it' },
  bre: { d: 'lemanapro.ru' }, bro: { d: 'yougov.com' }, brv: { d: '999.md' },
  bsf: { d: 'rupiahcepat.co.id' }, bsl: { d: 'oskelly.ru' }, bst: { d: 'admiralbet.rs' },
  bsu: { d: 'dingtone.me' }, bsy: { d: 'airmiles.ca' },
  bvi: { d: 'salams.app' }, bvs: { d: 'vchasno.ua' }, bwe: { d: 'immutable.com' },
  bwl: { d: 'guthaben.de' }, bwo: { d: 'kabanchik.ua' }, bwv: { d: 'manus.im' },
  bwx: { d: 'chagee.com.sg' }, bxi: { d: 'grupomadero.com.br' }, bxj: { d: 'queroquero.com.br' },
  byp: { d: 'kaito.ai' }, byw: { d: 'annoncelight.dk' }, bzf: { d: 'dabble.com.au' },
  cak: { d: 'bazaraki.com' }, cb: { d: 'bazos.cz' },
  cba: { d: 'enilive.it' }, cbs: { d: 'lalafo.kg' }, ccl: { d: 'cruzeiro.com.br' },
  cj: { d: 'dotz.com.br' }, cm: { d: 'prom.ua' }, gx: { d: 'hepsiburada.com' },
  hu: { d: 'ukr.net' }, ib: { d: 'immowelt.de' }, ir: { d: 'chispaapp.com' },
  kl: { d: 'kolesa.kz' }, km: { d: 'rozetka.com.ua' }, lc: { d: 'subito.it' },
  blz: { d: 'minipay.to' },
  ms: { d: 'novaposhta.ua' },
  pr: { d: 'trendyol.com' }, rr: { d: 'wolt.com' }, sl: { d: 'robota.ua' },
  te: { d: 'e-food.gr' }, tp: { d: 'indiagold.co' }, tv: { d: 'goflink.com' },
  us: { d: 'irctc.com' }, vs: { d: 'winzogames.com' },
  wd: { d: 'stoloto.ru' }, xm: { d: 'letu.ru' }, xz: { d: 'paycell.com.tr' },
  yl: { d: 'yalla.group' }, ym: { d: 'youla.ru' }, zb: { d: 'free-now.com' },
  zn: { d: 'biedronka.pl' },
  bls: { d: 'wetv.vip' }, btn: { d: 'itau.com.br' }, ari: { d: 'ring4.com' },

  /* Markası gerçek, alan adı DOĞRU — ama ne DuckDuckGo ne Google bu alan adını
   * önbelleğinde tutuyor (ikisi de 404). Yine de yazılıyorlar: 3. katman
   * (unavatar) açıldığında denenecek adres bu. Alan adını burada tutmak,
   * `<ad>.com` tahminine geri düşmekten her hâlükârda doğrudur. */
  afd: { d: 'astraotoshop.com' }, aka: { d: 'linkaja.id' }, aoy: { d: 'pln.co.id' },
  apg: { d: 'damai.cn' }, axm: { d: 'naomeperturbe.com.br' }, agb: { d: 'smiles.com.br' },
  bgv: { d: 'clearpay.co.uk' }, blp: { d: 'meeff.com' }, bng: { d: 'jush.ua' },
  bra: { d: 'touchngo.com.my' }, bxr: { d: 'alfursan.com.sa' }, byf: { d: 'seabank.co.id' },
  cbh: { d: 'zuldigital.com.br' }, ju: { d: 'indomaret.co.id' }, mx: { d: 'soulapp.cn' },
  nh: { d: 'allobank.com' }, ve: { d: 'dream11.com' }, acr: { d: 'qwikcilver.com' },
  aua: { d: 'ly.com' },

  /* GÖZ DENETİMİNDE ELENDİ.
   *
   * Bu alan adları ikon DÖNDÜRÜYOR ve otomatik denetimleri geçiyor; ekranda
   * bakınca BAŞKA bir markanın logosu oldukları görülüyor. Otomatik hiçbir
   * kural bunu yakalayamaz — bu yüzden `--onizleme` çıktısı bir insan
   * tarafından bir kez gözden geçirilmelidir. Her satırda EKRANDA NE GÖRÜNDÜĞÜ
   * yazılı ki gelecekte biri aynı alan adını yeniden denemesin. */
  anb: null, // abasteceai.com.br → sarı zeminde "KMV": başka bir şirket
  cam: null, // ele.me → 淘宝闪购 (Taobao) markası; servis adı "Eleme 饿了么"
  ly: null,  // olamoney.com → yeşil "M" (Ola Money); servis ise Ola Cabs
  ang: null, // tomorocoffee.com → "ROKOK 88" sigara/kahve reklam görseli, logo değil
  ny: null,  // bitcoinbon.at → kırmızı "K"; .com ise park edilmiş
  bl: null,  // bigo.tv → BIGO Live markasıyla ilgisiz karalama figürü
  uv: null,  // binbin.app → jenerik 4 kareli uygulama ikonu, BinBin değil
  avu: null, // karos.fr → mercan zeminde taç; Karos ortak yolculuk markası değil
  bif: null, // opap.gr → turkuaz kutuda küçük "a"; OPAP markası değil
  bsv: null, // amartha.com → mor mandala deseni; Amartha kelime markası değil
  bhf: null, // inpost.pl → sarı güneş; InPost sarı-siyah kelime markası değil
  bhj: null, // jofogas.hu → jenerik 3B karton koli ikonu, Jófogás markası değil

  /* Marka değil ya da doğrulanabilir bir markaya bağlanamıyor → logo konmaz.
   * "Adverts", "Bunda", "D4", "TOP", "ESX", "Radiate", "Treasury" gibi adlar
   * hem jenerik hem de sahibi belirsiz; tahmin etmek yanlış logo riskidir. */
  agw: null, afc: null, btl: null, bzv: null, bcr: null, btv: null, bnt: null,
  cac: null, bgn: null, bcz: null, ahr: null, ajv: null, bkz: null, tc: null,
  aub: null, ajq: null, bns: null, bhp: null, bso: null, alp: null, azl: null,
  awu: null, oj: null, pu: null, avt: null, bqm: null, bzk: null, blt: null,
  bqn: null, bdo: null, ajy: null, bsm: null, bkw: null, bzb: null, bdd: null,
  ael: null, bxw: null, bcy: null, bxn: null, xx: null, bdp: null, ayo: null,
  yj: null, zy: null, bvu: null, bba: null, bko: null, blh: null, bou: null,
  azd: null, agx: null, bvh: null, aoi: null, aju: null, caj: null, agm: null,
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

  // Ad ZATEN bir alan adıysa onu boz ma. `normalize()` noktayı siliyor ve
  // "kolesa.kz" → "kolesakz.com" gibi var olmayan bir adres üretiyordu.
  // Ölçümde bu ~12 servisi tek başına kaybettiriyordu (vk.com, youla.ru,
  // robota.ua, Cloud.ru, 999 md, Casa it, Markt.de, Mobile DE…).
  const ham = name.trim();
  if (/^[a-z0-9-]+\.[a-z]{2,3}(\.[a-z]{2})?$/i.test(ham)) return ham.toLowerCase();

  const base = normalize(parts(name)[0] ?? name);
  if (base.length < 2) return null;
  // Salt rakamdan oluşan adlar ("99", "360") alan adı tahmini için güvenilmez.
  if (/^\d+$/.test(base)) return null;
  return `${base}.com`;
}

const sha = (buf) => createHash('sha256').update(buf).digest('hex');

/**
 * BİLİNEN YER TUTUCU PARMAK İZLERİ (sha256'nın ilk 16 hanesi = 64 bit).
 *
 * Bir ikon servisi alan adını tanımadığında çoğu zaman HATA DÖNMEZ; kendi
 * genel avatarını 200 ile geri verir. Tek tek bakınca gerçek logo sanılır.
 * AYIRT ETME KANITI (ölçüldü): birbiriyle alâkasız iki uydurma alan adı
 *   unavatar.io/zzqqxxnotarealbrand99123.com  → 200, image/svg+xml, 569 B
 *   unavatar.io/bqm-beebs-nonexistent-xyz.com → 200, image/svg+xml, 569 B
 * AYNI sha256'yı üretti. Farklı girdiye aynı görsel = marka logosu değil.
 *
 * ⚠ DOSYA BOYUTUNA GÖRE ELEME YAPILMAZ. Teşhiste "15086 B ve 1150 B park
 * ikonudur" denmişti; ölçüm bunu ÇÜRÜTTÜ: rozetka.com.ua, wildberries.ru,
 * subito.it, walmart.com, immowelt.de hepsi 15086 B ama sha256'ları FARKLI.
 * 15086/1150, çok çözünürlüklü .ico kabının olağan boyları. Boyutla elemek
 * bu gerçek logoları çöpe atardı.
 */
const YER_TUTUCU_SHA = new Map([
  ['057f351eec416b7b', 'unavatar genel avatarı (569 B SVG)'],
  ['e5db88ea2322863c', 'DuckDuckGo genel ikonu (1478 B PNG, HTTP 404 ile gelir)'],
  // Google s2 de HTTP 200 ile genel ikon döndürebiliyor. İkisi de bu koşuda
  // yakalandı; ikisinin de kanıtı "alâkasız iki alan adı, aynı sha256":
  ['ea24570dd7740951', 'Google s2 genel ikonu — karos.co ve yalla.group aynı 1965 B'],
  ['76b40fe49a915f42', 'Google s2 genel ikonu — fruitz.com ve caixa.com aynı 367 B'],
]);

/**
 * Gerçek dosya imzasından uzantı türet.
 *
 * ÖNCEKİ HATA: yalnız PNG imzası kabul ediliyordu (`buf[0]!==0x89`). Google s2
 * pek çok alan adı için JPEG döndürüyor (ölçüldü: trendyol.com, wolt.com,
 * retailmenot.com, scalapay.com → ff d8 ff e0). Bu servisler geçerli 128 px
 * logoya sahipken sırf imza yüzünden eleniyordu. `<img>` hepsini gösterir.
 */
function uzantiBul(buf) {
  if (buf.length < 16) return null;
  const b = buf;
  if (b[0] === 0x89 && b[1] === 0x50 && b[2] === 0x4e && b[3] === 0x47) return 'png';
  if (b[0] === 0xff && b[1] === 0xd8 && b[2] === 0xff) return 'jpg';
  if (b[0] === 0x47 && b[1] === 0x49 && b[2] === 0x46) return 'gif';
  if (b.toString('ascii', 0, 4) === 'RIFF' && b.toString('ascii', 8, 12) === 'WEBP') return 'webp';
  if (b[0] === 0x00 && b[1] === 0x00 && b[2] === 0x01 && b[3] === 0x00) return 'ico';
  // SVG: XML bildirimi ya da doğrudan <svg. BOM olabilir.
  const bas = b.toString('utf8', 0, 300).replace(/^﻿/, '').trimStart();
  if (bas.startsWith('<?xml') || bas.startsWith('<svg')) {
    return bas.includes('<svg') ? 'svg' : null;
  }
  return null;
}

/* ── UNAVATAR — neden EN SONDA ve neden VARSAYILAN KAPALI ────────────────────
 *
 * 1. KOTA (ölçüldü). Ücretsiz katman IP başına GÜNDE 25 istek:
 *      x-pricing-tier: free · x-rate-limit-limit: 25
 *    Bu oturumda 3 istekten sonra `x-rate-limit-remaining: 0` ve
 *    `{"code":"ERATE"}` alındı. 236 servis tek koşuda ÇEKİLEMEZ; API anahtarı
 *    alınsa bile tavan 50/gün. Bu yüzden koşu başına sert bir sayaç var.
 * 2. ÜST KÜME, DAHA KÖTÜ EKONOMİ. unavatar bir toplayıcıdır; arka uçlarından
 *    ikisi zaten 2. katmanın kaynakları (DuckDuckGo, Google). Onu öne almak
 *    kotayı, 2. katmanın BEDAVA çözdüğü alan adlarına harcamak olurdu.
 * 3. YANLIŞ MARKA RİSKİ. unavatar sosyal medya PROFİL avatarına da düşebilir;
 *    bu bir marka logosu değildir. Para sisteminde yanlış logo, logosuzluktan
 *    kötüdür — bu yüzden en riskli kaynak en sona konur.
 *
 * `fallback=false` ZORUNLUDUR: onsuz bilinmeyen alan adı için 200 + genel
 * avatar döner. Onunla 404 döner. (`size=128` tek başına yetmez; ölçümde
 * whatsapp.com?size=128 yine 23×23 verdi, fallback=false&size=128 ise 194×194.)
 */
const KAYNAKLAR = [
  { ad: 'ddg', url: (d) => `https://icons.duckduckgo.com/ip3/${d}.ico` },
  { ad: 'google', url: (d) => `https://www.google.com/s2/favicons?domain=${encodeURIComponent(d)}&sz=128` },
];
const UNAVATAR = {
  ad: 'unavatar',
  url: (d) => `https://unavatar.io/${encodeURIComponent(d)}?fallback=false&size=128`,
};
const UNAVATAR_BUTCE = 20;
let unavatarKalan = UNAVATAR_ETKIN ? UNAVATAR_BUTCE : 0;
let unavatarKapandi = false;

async function tekKaynak(url) {
  try {
    const res = await fetch(url, {
      redirect: 'follow',
      signal: AbortSignal.timeout(15_000),
      headers: { Accept: 'image/*' },
    });
    if (!res.ok) return { hata: res.status };
    const buf = Buffer.from(await res.arrayBuffer());
    if (buf.length < 100) return { hata: 'kucuk' };
    if (buf.length > MAX_IKON_BAYT) return { hata: 'buyuk' };
    const ext = uzantiBul(buf);
    if (!ext) return { hata: 'imza' };
    const h = sha(buf);
    const yerTutucu = YER_TUTUCU_SHA.get(h.slice(0, 16));
    if (yerTutucu) return { hata: `yer-tutucu:${yerTutucu}` };
    return { buf, ext, hash: h };
  } catch {
    return { hata: 'ag' };
  }
}

/**
 * Bir ikonun "ağır" sayıldığı sınır.
 *
 * DuckDuckGo çoğu alan adı için .ico döndürüyor ve çok çözünürlüklü .ico kabı
 * 16/32/48/256 px kopyaların HEPSİNİ taşıyor — biz 32 px çiziyoruz. Ölçüm: 168
 * .ico dosyası tek başına 2,8 MB. Bu sınırın üstünde kalan bir ikon için diğer
 * kaynaklar da denenir ve EN KÜÇÜĞÜ alınır; kapsam aynı kalır, depo hafifler.
 */
const AGIR_IKON_BAYT = 20_000;

/** Kaynakları sırayla dener; ilk geçerli yanıt ağırsa daha hafifini arar. */
async function fetchIcon(domain, istatistik) {
  let enIyi = null;
  for (const k of KAYNAKLAR) {
    const r = await tekKaynak(k.url(domain));
    if (r.buf) {
      if (!enIyi || r.buf.length < enIyi.buf.length) enIyi = { ...r, src: k.ad };
      // Yeterince hafifse aramayı sürdürmenin anlamı yok.
      if (enIyi.buf.length <= AGIR_IKON_BAYT) return enIyi;
      continue;
    }
    if (typeof r.hata === 'string' && r.hata.startsWith('yer-tutucu')) istatistik.yerTutucu++;
  }
  if (enIyi) return enIyi;
  // 3. katman: yalnız açıksa, bütçe varsa ve kota dolmadıysa.
  if (unavatarKalan > 0 && !unavatarKapandi) {
    unavatarKalan--;
    const r = await tekKaynak(UNAVATAR.url(domain));
    if (r.buf) return { ...r, src: UNAVATAR.ad };
    if (typeof r.hata === 'string' && r.hata.startsWith('yer-tutucu')) istatistik.yerTutucu++;
    // 429 = günlük kota bitti. Katmanı sessizce kapat; kalan servisler için
    // boşuna istek atıp sıraya girmenin anlamı yok.
    // `!unavatarKapandi`: havuzdaki 8 işçiden birkaçı kontrolü aynı anda
    // geçmiş olabilir; bayrağı ilk gören yazdırsın, diğerleri sessizce dursun.
    if (r.hata === 429 && !unavatarKapandi) {
      unavatarKapandi = true;
      console.log('  · unavatar günlük kotası doldu (429) — 3. katman kapatıldı');
    }
  }
  return null;
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
const URETILEN_UZANTI = ['.svg', '.png', '.jpg', '.gif', '.webp', '.ico'];
for (const f of readdirSync(OUT_DIR)) {
  if (URETILEN_UZANTI.some((u) => f.endsWith(u))) unlinkSync(join(OUT_DIR, f));
}
// Önizleme yalnız istendiğinde üretilir; eski bir kopya `public/` altında
// unutulmasın diye her koşuda silinir.
const ONIZLEME_DOSYA = '_onizleme.html';
if (existsSync(join(OUT_DIR, ONIZLEME_DOSYA))) unlinkSync(join(OUT_DIR, ONIZLEME_DOSYA));

const matched = [];
const needFavicon = [];

// ── Katman 1 ──
for (const s of items) {
  const icon = findVectorIcon(s.code, s.name);
  if (icon) {
    writeFileSync(join(OUT_DIR, `${s.code}.svg`), makeSVG(icon), 'utf8');
    matched.push({
      code: s.code, name: s.name, url: `/servis-logolari/${s.code}.svg`,
      src: 'vektör', domain: icon.title,
    });
  } else {
    needFavicon.push(s);
  }
}
console.log(`  · katman 1 (vektör): ${matched.length} servis`);

// ── Katman 2 ──
let faviconCount = 0;
const missed = [];
if (!VEKTOR_ONLY && needFavicon.length) {
  const istatistik = { yerTutucu: 0 };
  const results = await pool(needFavicon, 8, async (s) => {
    const domain = guessDomain(s.code, s.name);
    if (!domain) return null;
    const r = await fetchIcon(domain, istatistik);
    if (!r) return null;
    return { s, domain, ...r };
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
    (byHash.get(r.hash) ?? byHash.set(r.hash, []).get(r.hash)).push(r);
  }
  let placeholders = 0;
  for (const [, group] of byHash) {
    if (group.length >= 3) {
      placeholders += group.length;
      console.log(`    ⚠ yer tutucu: ${group.map((r) => r.s.code).join(',')} aynı görseli aldı`);
      continue;
    }
    for (const r of group) {
      writeFileSync(join(OUT_DIR, `${r.s.code}.${r.ext}`), r.buf);
      matched.push({
        code: r.s.code, name: r.s.name, url: `/servis-logolari/${r.s.code}.${r.ext}`,
        src: r.src, domain: r.domain,
      });
      faviconCount++;
    }
  }
  if (placeholders) {
    console.log(`  · ${placeholders} yer tutucu ikon elendi (aynı görsel 3+ serviste)`);
  }
  if (istatistik.yerTutucu) {
    console.log(`  · ${istatistik.yerTutucu} genel avatar reddedildi (parmak izi kara listesi)`);
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

// Depoya giren toplam ağırlık — logolar sürüm denetimine dâhil.
let toplamBayt = 0;
for (const f of readdirSync(OUT_DIR)) {
  if (URETILEN_UZANTI.some((u) => f.endsWith(u))) toplamBayt += statSync(join(OUT_DIR, f)).size;
}
console.log(`  · toplam dosya boyutu: ${(toplamBayt / 1024 / 1024).toFixed(2)} MB`);

/* GÖZ DENETİMİ SAYFASI.
 *
 * Bir kaynağın 200 dönmesi, alan adının DOĞRU MARKA olduğunu kanıtlamaz.
 * Kürate haritayı bir insanın bir kez gözden geçirmesi şart; bu sayfa onu
 * tek ekranda mümkün kılar. Üretim çıktısı değildir, depoya girmez. */
if (process.argv.includes('--onizleme')) {
  const satir = (m) => `<figure><img src="${m.url}" alt=""><figcaption>
    <b>${m.code}</b><br>${(m.name ?? '').replace(/[<>&]/g, '')}<br>
    <small>${m.domain ?? ''} · ${m.src}</small></figcaption></figure>`;
  const html = `<!doctype html><meta charset="utf-8"><title>Servis logoları — göz denetimi</title>
<style>body{font:14px system-ui;background:#111;color:#eee;margin:0;padding:16px}
main{display:grid;grid-template-columns:repeat(auto-fill,minmax(150px,1fr));gap:12px}
figure{margin:0;padding:8px;background:#1c1c1c;border-radius:8px;text-align:center}
img{width:48px;height:48px;object-fit:contain;background:#fff;border-radius:8px}
small{color:#999;word-break:break-all}</style>
<h1>${matched.length} logo — yanlış marka var mı?</h1><main>
${matched.map(satir).join('\n')}</main>`;
  // `public/` altına yazılır ki geliştirme sunucusunda doğrudan açılabilsin:
  // http://localhost:3000/servis-logolari/_onizleme.html
  writeFileSync(join(OUT_DIR, ONIZLEME_DOSYA), html, 'utf8');
  console.log(`  · önizleme: /servis-logolari/${ONIZLEME_DOSYA}`);
}

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
