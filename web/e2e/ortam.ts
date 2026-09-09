import path from 'node:path';
import { fileURLToPath } from 'node:url';

/** Bu dosyanın bulunduğu dizin (web/e2e). */
const HERE = path.dirname(fileURLToPath(import.meta.url));

/** Test edilen web sunucusunun kökü. `scripts/e2e.sh` bunu ayarlar. */
export const BASE_URL = process.env.E2E_BASE_URL ?? 'http://localhost:3100';

/**
 * API sürecinin log dosyası.
 *
 * E-posta doğrulama token'ı BURADAN okunur: geliştirmede posta gönderilmez,
 * `mailer.Console` yalnız bağlantıyı log'a yazar. Veritabanında token'ın
 * SHA-256 özeti tutulduğu için oradan okunamaz — kasıtlı bir tasarımdır.
 */
export const API_LOG_PATH =
  process.env.E2E_API_LOG ?? path.resolve(HERE, '../../.e2e-logs/api.log');

/**
 * Yönetici hesabı — GELİŞTİRME tohum verisi.
 *
 * Değer ortamdan gelir (`scripts/e2e.sh` verir); burada varsayılan YOKTUR ve
 * olmayacaktır. Bir parola varsayılanı yazmak, onu tüm kopyalarda geçerli bir
 * tahmin hâline getirirdi. Boşsa `global-setup.ts` koşuyu baştan durdurur.
 */
/**
 * E2E koşusuna AYRILMIŞ Redis veritabanı.
 *
 * Yalnız hız limiti sayaçlarını (`rl:*`) sıfırlamak için kullanılır; bkz.
 * `redis.ts`. Tanımsızsa sıfırlama atlanır ve `/auth` limiti (30/dk) uzun bir
 * koşuyu düşürebilir — global-setup bunu uyarı olarak bildirir.
 */
export const REDIS_URL = process.env.E2E_REDIS_URL ?? '';

export const ADMIN = {
  email: process.env.E2E_ADMIN_EMAIL ?? '',
  password: process.env.E2E_ADMIN_PASSWORD ?? '',
};
