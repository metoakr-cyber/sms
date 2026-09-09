import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { randomUUID } from 'node:crypto';
import {
  test as base,
  expect,
  request as playwrightRequest,
  type APIRequestContext,
  type Locator,
  type Page,
} from 'playwright/test';
import { ADMIN, API_LOG_PATH, BASE_URL, REDIS_URL } from './ortam';
import { hizLimitiSifirla } from './redis';

/* ═══════════════════════════ Türler ═══════════════════════════ */

export interface TestUser {
  /** users.public_id (UUID) — sayısal id dışarı verilmez (CLAUDE.md #10). */
  id: string;
  email: string;
  username: string;
  password: string;
}

export interface TestDepositMethod {
  id: string;
  code: string;
  name: string;
}

/* ═══════════════════════ Benzersiz kimlikler ═══════════════════════ */

/**
 * Her test KENDİ verisini kurar; hiçbir test başka bir testin kaydını
 * görmemeli. Zaman damgası + rastgele parça, aynı saniyede başlayan iki
 * koşuyu da ayırır.
 */
function benzersiz(): string {
  return `${Date.now().toString(36)}${randomUUID().replace(/-/g, '').slice(0, 6)}`;
}

export function yeniKullanici(): Omit<TestUser, 'id'> {
  const s = benzersiz();
  return {
    // Alan adı da benzersiz: konsol posta sağlayıcısı adresi maskeler
    // (`ab***@alan`) ve yalnız alan adı ayırt edici kalır.
    email: `e2e@u${s}.test`,
    username: `e2e_${s}`.slice(0, 32),
    // Sunucu sözlük tabanlı zayıf parola kontrolü yapar; bu dizi geçer.
    password: `e2e-dogru-at-pil-zimba-${s}`.slice(0, 60),
  };
}

/* ═══════════════════ E-posta doğrulama (log üzerinden) ═══════════════════ */

const KILIT = path.join(os.tmpdir(), 'sms-platform-e2e-kayit.lock');
const KILIT_BAYAT_MS = 120_000;

/**
 * Kayıt + token okuma bölümünü serileştirir.
 *
 * Token API log'undan okunur ve log'da hangi satırın hangi kayda ait olduğunu
 * KESİN olarak söyleyen bir alan yoktur (adres maskelenir). İki kayıt aynı anda
 * yapılırsa yanlış token okunur ve test BAŞKA BİR kullanıcının hesabını
 * doğrular — sessiz ve teşhis edilemez bir hata. Kilit bu ihtimali kaldırır.
 */
export async function kayitKilidiyle<T>(fn: () => Promise<T>): Promise<T> {
  const bitis = Date.now() + 60_000;
  let fd: number | null = null;

  while (fd === null) {
    try {
      fd = fs.openSync(KILIT, 'wx');
    } catch {
      // Çöken bir koşu kilidi bırakmış olabilir; bayatlamışsa temizle.
      try {
        const yas = Date.now() - fs.statSync(KILIT).mtimeMs;
        if (yas > KILIT_BAYAT_MS) fs.rmSync(KILIT, { force: true });
      } catch {
        /* kilit tam o anda kaldırıldı — bir sonraki denemede alınır */
      }
      if (Date.now() > bitis) throw new Error('kayıt kilidi 60 sn içinde alınamadı');
      await new Promise((r) => setTimeout(r, 50));
    }
  }

  try {
    return await fn();
  } finally {
    fs.closeSync(fd);
    fs.rmSync(KILIT, { force: true });
  }
}

export function logBoyutu(): number {
  try {
    return fs.statSync(API_LOG_PATH).size;
  } catch {
    return 0;
  }
}

function logKuyrugu(ofset: number): string {
  const fd = fs.openSync(API_LOG_PATH, 'r');
  try {
    const boyut = fs.fstatSync(fd).size;
    if (boyut <= ofset) return '';
    const buf = Buffer.alloc(boyut - ofset);
    fs.readSync(fd, buf, 0, buf.length, ofset);
    return buf.toString('utf8');
  } finally {
    fs.closeSync(fd);
  }
}

/**
 * `ofset`ten sonra log'a düşen ilk doğrulama token'ını döndürür.
 *
 * SABİT BEKLEME YOK: koşul sağlanana kadar yoklanır, sağlanınca hemen döner.
 */
export async function dogrulamaTokeni(ofset: number): Promise<string> {
  let token = '';
  await expect
    .poll(
      () => {
        const m = /dogrula\?token=([A-Za-z0-9_-]+)/.exec(logKuyrugu(ofset));
        token = m?.[1] ?? '';
        return token;
      },
      {
        timeout: 15_000,
        intervals: [50, 100, 200, 500],
        message: `API log'unda doğrulama bağlantısı bulunamadı (${API_LOG_PATH})`,
      },
    )
    .not.toBe('');
  return token;
}

/* ═══════════════════════ API yardımcıları ═══════════════════════ */

async function apiBaglami(): Promise<APIRequestContext> {
  return playwrightRequest.newContext({ baseURL: BASE_URL });
}

/** Kayıt olur, e-postayı doğrular ve `/me` üzerinden public id'yi alır. */
export async function kullaniciOlustur(): Promise<TestUser> {
  const taslak = yeniKullanici();
  const ctx = await apiBaglami();
  try {
    await kayitKilidiyle(async () => {
      const ofset = logBoyutu();
      const kayit = await ctx.post('/api/v1/auth/register', {
        data: {
          email: taslak.email,
          username: taslak.username,
          password: taslak.password,
          acceptTerms: true,
          captchaToken: '',
        },
      });
      expect(kayit.status(), `kayıt başarısız: ${await kayit.text()}`).toBe(200);

      const token = await dogrulamaTokeni(ofset);
      const dogrula = await ctx.post('/api/v1/auth/verify-email', { data: { token } });
      expect(dogrula.status(), `doğrulama başarısız: ${await dogrula.text()}`).toBe(200);
    });

    const giris = await ctx.post('/api/v1/auth/login', {
      data: { email: taslak.email, password: taslak.password, captchaToken: '' },
    });
    expect(giris.status(), `giriş başarısız: ${await giris.text()}`).toBe(200);

    const me = await ctx.get('/api/v1/me');
    expect(me.status()).toBe(200);
    const govde = (await me.json()) as { id: string; emailVerified: boolean };
    expect(govde.emailVerified, 'kullanıcı doğrulanmış olmalıydı').toBe(true);

    return { ...taslak, id: govde.id };
  } finally {
    await ctx.dispose();
  }
}

/**
 * Sayfanın tarayıcı bağlamına oturum çerezi yerleştirir.
 *
 * `page.request` bağlamın çerez kavanozunu paylaşır — arayüzden giriş yapmak
 * yerine API'yi çağırmak, giriş formunun kendi testi dışındaki testleri o
 * formun ayrıntılarına bağımlı olmaktan kurtarır.
 */
export async function girisYap(page: Page, email: string, password: string): Promise<void> {
  const res = await page.request.post('/api/v1/auth/login', {
    data: { email, password, captchaToken: '' },
  });
  expect(res.status(), `giriş başarısız (${email}): ${await res.text()}`).toBe(200);
}

/** Yönetici oturumlu, yalnız API konuşan bir bağlam. */
export async function yoneticiApi(): Promise<APIRequestContext> {
  const ctx = await apiBaglami();
  const res = await ctx.post('/api/v1/auth/login', {
    data: { email: ADMIN.email, password: ADMIN.password, captchaToken: '' },
  });
  expect(res.status(), `yönetici girişi başarısız: ${await res.text()}`).toBe(200);
  return ctx;
}

/** Kullanıcının bakiyesini yönetici düzeltmesiyle artırır (kurulum adımı). */
export async function bakiyeEkle(
  admin: APIRequestContext,
  userId: string,
  minor: number,
  not: string,
): Promise<void> {
  const res = await admin.post(`/api/v1/admin/users/${userId}/balance`, {
    data: {
      amountMinor: minor,
      note: not,
      // Anahtar deterministik: aynı kurulum iki kez koşarsa bakiye iki kez artmaz.
      idempotencyKey: `e2e-${userId}-${minor}`.slice(0, 64),
    },
  });
  expect(res.status(), `bakiye düzeltmesi başarısız: ${await res.text()}`).toBe(200);
}

/** Kullanıcının güncel bakiyesini kuruş olarak okur. */
export async function bakiyeOku(ctx: APIRequestContext): Promise<number> {
  const res = await ctx.get('/api/v1/wallet/balance');
  expect(res.status()).toBe(200);
  const body = (await res.json()) as { balance: { minor: number } };
  return body.balance.minor;
}

/* ═══════════════════════════ Fixture'lar ═══════════════════════════ */

interface Fixtures {
  /**
   * Her testin başında `/auth` hız limiti sayaçlarını sıfırlar.
   *
   * Sınır (IP başına 30 istek/dk) ÜRÜN İÇİN DOĞRUDUR ve kaldırılmaz. Ama tek
   * makineden koşan bir takım onlarca kayıt/giriş yapar ve sınır, üründe hiçbir
   * hata yokken testleri düşürür. Sıfırlama yalnız e2e'ye ayrılmış Redis
   * veritabanındaki `rl:` anahtarlarına dokunur (bkz. redis.ts).
   */
  temizHizLimiti: void;
  /** Doğrulanmış, o teste ait taze kullanıcı. */
  kullanici: TestUser;
  /** `kullanici` ile giriş yapılmış sayfa. */
  kullaniciSayfasi: Page;
  /** `kullanici` oturumlu API bağlamı (kurulum ve doğrulama için). */
  kullaniciApi: APIRequestContext;
  /** Yönetici oturumlu API bağlamı. */
  yonetici: APIRequestContext;
  /** Yönetici ile giriş yapılmış sayfa. */
  yoneticiSayfasi: Page;
  /** Bu teste ait, aktif bir ödeme yöntemi; test bitince silinir. */
  odemeYontemi: TestDepositMethod;
}

export const test = base.extend<Fixtures>({
  // `auto: true` yetmez — Playwright otomatik fixture'ları diğerlerinden ÖNCE
  // kurar ama bunu belgelenmiş bir sıra olarak varsaymak kırılgan olurdu.
  // Oturum açan her fixture ayrıca bunu AÇIKÇA bağımlılık olarak alır.
  temizHizLimiti: [
    async ({}, use) => {
      if (REDIS_URL) await hizLimitiSifirla(REDIS_URL);
      await use();
    },
    { auto: true },
  ],

  kullanici: async ({ temizHizLimiti }, use) => {
    void temizHizLimiti;
    await use(await kullaniciOlustur());
  },

  kullaniciSayfasi: async ({ page, kullanici }, use) => {
    await girisYap(page, kullanici.email, kullanici.password);
    await use(page);
  },

  kullaniciApi: async ({ kullanici }, use) => {
    const ctx = await apiBaglami();
    const res = await ctx.post('/api/v1/auth/login', {
      data: { email: kullanici.email, password: kullanici.password, captchaToken: '' },
    });
    expect(res.status(), `kullanıcı girişi başarısız: ${await res.text()}`).toBe(200);
    await use(ctx);
    await ctx.dispose();
  },

  yonetici: async ({ temizHizLimiti }, use) => {
    void temizHizLimiti;
    const ctx = await yoneticiApi();
    await use(ctx);
    await ctx.dispose();
  },

  yoneticiSayfasi: async ({ page, temizHizLimiti }, use) => {
    void temizHizLimiti;
    await girisYap(page, ADMIN.email, ADMIN.password);
    await use(page);
  },

  odemeYontemi: async ({ yonetici }, use) => {
    const s = benzersiz();
    const kod = `e2e-${s}`.slice(0, 40);
    const ad = `E2E Test Bankası ${s}`;

    const olustur = await yonetici.post('/api/v1/admin/deposit-methods', {
      data: {
        code: kod,
        kind: 'BANK_TRANSFER',
        name: ad,
        instructions: 'Uçtan uca test yöntemi. Gerçek para gönderilmez.',
        config: { iban: 'TR000000000000000000000000', hesapAdi: 'E2E Test' },
        minAmountMinor: 1000,
        maxAmountMinor: 5_000_000,
        sortOrder: 900,
      },
    });
    /*
      200 BEKLENİYOR, 201 DEĞİL.

      `handler/admin.go:CreateDepositMethod` önce `c.Status(201)` yazıyor, hemen
      ardından `Responder.OK` → `c.JSON(200, …)` çağırıyor ve 201'i EZİYOR.
      Yani uç nokta 201 dönmek İSTİYOR ama 200 dönüyor. Test bugünkü gerçek
      davranışı sabitliyor; düzeltme o dosyanın sahibine bildirildi.
    */
    expect(olustur.status(), `ödeme yöntemi kurulamadı: ${await olustur.text()}`).toBe(200);
    const yontem = (await olustur.json()) as { id: string };

    // Yeni yöntem PASİF doğar (sunucu kararı); kullanıcının görmesi için açılır.
    const aktif = await yonetici.patch(`/api/v1/admin/deposit-methods/${yontem.id}/active`, {
      data: { isActive: true },
    });
    expect(aktif.status(), `yöntem aktifleştirilemedi: ${await aktif.text()}`).toBe(200);

    await use({ id: yontem.id, code: kod, name: ad });

    // TEMİZLİK: yöntem silinir, geçmiş yükleme kayıtları kalır
    // (deposits.method_id ON DELETE SET NULL + method_name anlık görüntüsü).
    // Silinmezse geliştirme ortamındaki "Bakiye yükle" ekranı her koşuda bir
    // sahte banka daha gösterirdi.
    await yonetici.delete(`/api/v1/admin/deposit-methods/${yontem.id}`);
  },
});

export { expect };
export type { APIRequestContext, Locator, Page };
