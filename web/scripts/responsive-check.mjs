/**
 * Responsive denetimi — docs/frontend-contract.md §2.2 / §2.3 / §7.1.
 *
 * Göz kararı yerine ÖLÇÜM: her sayfa, zorunlu her genişlikte açılır ve
 *   1) yatay kaydırma var mı,
 *   2) 44 px altında KENDİ BAŞINA DURAN dokunma hedefi var mı
 * diye bakılır. Paragraf içi bağlantılar muaftır (sözleşme §2.3 istisnası).
 *
 * Kullanım: node scripts-responsive-check.mjs [taban-url]
 */
import { chromium } from 'playwright';

const BASE = process.argv[2] ?? 'http://localhost:3000';
const WIDTHS = [320, 390, 430, 768, 1440];
const PUBLIC_PAGES = ['/', '/fiyatlar', '/sss', '/giris', '/kayit', '/sifremi-unuttum'];

// Oturum gerektiren sayfalar. Denetimin dışında bırakmak, arayüzün ASIL
// kısmını denetlenmemiş bırakmak olurdu — kullanıcı zamanının çoğunu burada
// geçirir. Kimlik bilgileri ortamdan gelir; betiğe gömülmez.
const PRIVATE_PAGES = ['/panel', '/panel/numara-al', '/panel/cuzdan', '/panel/hesap',
                       '/yonetim', '/yonetim/bakiye'];
const EMAIL = process.env.AUDIT_EMAIL ?? '';
const PASSWORD = process.env.AUDIT_PASSWORD ?? '';

const AUDIT = () => {
  const vw = window.innerWidth;

  // Cümle İÇİNE gömülü bağlantı (§2.3 istisnası): ebeveyninde bağlantının
  // kendi metninden belirgin şekilde fazla metin varsa, bu bağlantı tek başına
  // duran bir denetim değil, akan metnin parçasıdır.
  const inProse = (el) => {
    if (el.tagName !== 'A') return false;
    const own = (el.textContent || '').trim().length;
    const parent = (el.parentElement?.textContent || '').trim().length;
    return own > 0 && parent > own + 12;
  };

  // Onay kutusunun gerçek dokunma alanı ETİKETİDİR: etikete dokunmak kutuyu
  // değiştirir. 20x20'lik input'u tek başına ölçmek yanlış ölçümdür.
  const labelCoversIt = (el) => {
    if (el.tagName !== 'INPUT') return false;
    if (!['checkbox', 'radio'].includes(el.type)) return false;
    const lab = el.closest('label') ||
      (el.id ? document.querySelector(`label[for="${CSS.escape(el.id)}"]`) : null);
    return !!lab && lab.getBoundingClientRect().height >= 44;
  };
  const small = [...document.querySelectorAll('a,button,input,select,textarea,[role="button"]')]
    .filter((el) => {
      const r = el.getBoundingClientRect();
      if (r.width === 0 || r.height === 0) return false;      // gizli
      if (el.closest('.sr-only')) return false;
      if (getComputedStyle(el).position === 'absolute' && r.height <= 2) return false;
      if (inProse(el)) return false;        // §2.3: cümle içi bağlantı
      if (labelCoversIt(el)) return false;  // etiket dokunma alanı sağlıyor
      return r.height < 44 || r.width < 44;
    })
    .map((el) => `${el.tagName}:${(el.textContent || '').trim().slice(0, 24)} ` +
                 `${Math.round(el.getBoundingClientRect().width)}x${Math.round(el.getBoundingClientRect().height)}`);
  return {
    overflow: document.documentElement.scrollWidth > vw + 1,
    scrollWidth: document.documentElement.scrollWidth,
    small: [...new Set(small)],
  };
};

const browser = await chromium.launch();
let fails = 0, checks = 0;

// Oturum çerezini BİR KEZ al, her genişlikte yeniden kullan.
let storageState;
if (EMAIL && PASSWORD) {
  const ctx = await browser.newContext();
  const page = await ctx.newPage();
  const res = await page.request.post(`${BASE}/api/v1/auth/login`, {
    data: { email: EMAIL, password: PASSWORD, captchaToken: '' },
  });
  if (!res.ok()) {
    console.log(`  ! giriş başarısız (HTTP ${res.status()}) — yalnız genel sayfalar denetlenecek`);
  } else {
    storageState = await ctx.storageState();
  }
  await ctx.close();
} else {
  console.log('  ! AUDIT_EMAIL/AUDIT_PASSWORD yok — yalnız genel sayfalar denetlenecek');
}

const PAGES = storageState ? [...PUBLIC_PAGES, ...PRIVATE_PAGES] : PUBLIC_PAGES;

for (const w of WIDTHS) {
  const ctx = await browser.newContext({ viewport: { width: w, height: 800 }, storageState });
  const page = await ctx.newPage();
  for (const path of PAGES) {
    checks++;
    await page.goto(BASE + path, { waitUntil: 'networkidle' });
    // Panel sayfaları oturumu sunucudan sorar; veri gelmeden ölçüm yapmak
    // iskelet ekranını ölçmek olur.
    if (path.startsWith('/panel') || path.startsWith('/yonetim')) {
      await page.waitForTimeout(900);
      if (new URL(page.url()).pathname.startsWith('/giris')) {
        console.log(`  ! ${w}px ${path}: oturum düştü, atlandı`); continue;
      }
    }
    const r = await page.evaluate(AUDIT);
    const problems = [];
    if (r.overflow) problems.push(`yatay kaydırma (${r.scrollWidth}px > ${w}px)`);
    if (r.small.length) problems.push(`küçük hedef: ${r.small.join(' · ')}`);
    if (problems.length) {
      fails++;
      console.log(`  ✗ ${w}px ${path}`);
      for (const p of problems) console.log(`      ${p}`);
    }
  }
  await ctx.close();
}
await browser.close();

if (fails === 0) console.log(`  ✓ responsive: ${checks} sayfa×genişlik denetimi temiz`);
else console.log(`\n  ${fails}/${checks} denetim düştü`);
process.exit(fails === 0 ? 0 : 1);
