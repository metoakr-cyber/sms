import fs from 'node:fs';
import { API_LOG_PATH, BASE_URL, REDIS_URL } from './ortam';
import { hizLimitiSifirla } from './redis';

/**
 * Koşu öncesi ön kontrol.
 *
 * Amacı: sunucular ayakta değilken testlerin "beklenen öğe bulunamadı" gibi
 * ALAKASIZ bir hatayla düşmesini engellemek. Yanlış hata mesajı, hatanın
 * kendisinden çok zaman kaybettirir (scripts/smoke-auth.sh'teki port kontrolü
 * de aynı sebeple var).
 */
export default async function globalSetup(): Promise<void> {
  const problems: string[] = [];

  try {
    const res = await fetch(`${BASE_URL}/giris`, { redirect: 'manual' });
    if (res.status >= 500) problems.push(`web sunucusu ${BASE_URL} adresinde ${res.status} döndü`);
  } catch {
    problems.push(`web sunucusuna ulaşılamadı: ${BASE_URL}`);
  }

  try {
    const res = await fetch(`${BASE_URL}/api/v1/catalog/services`);
    if (!res.ok) problems.push(`API vekili ${BASE_URL}/api/v1 üzerinde ${res.status} döndü`);
  } catch {
    problems.push(`API'ye ulaşılamadı: ${BASE_URL}/api/v1`);
  }

  // Kayıt akışı doğrulama token'ını API log'undan okur (konsol posta
  // sağlayıcısı). Log yoksa o testler anlaşılmaz biçimde düşerdi.
  if (!fs.existsSync(API_LOG_PATH)) {
    problems.push(`API log dosyası yok: ${API_LOG_PATH} (E2E_API_LOG)`);
  }

  if (!process.env.E2E_ADMIN_EMAIL || !process.env.E2E_ADMIN_PASSWORD) {
    problems.push('E2E_ADMIN_EMAIL / E2E_ADMIN_PASSWORD tanımsız');
  }

  // Hız limiti sıfırlaması olmadan uzun bir koşu 429 ile düşer (bkz. redis.ts).
  // Bunu koşunun ORTASINDA öğrenmek en pahalı yoldur.
  if (!REDIS_URL) {
    problems.push('E2E_REDIS_URL tanımsız — /auth hız limiti koşuyu düşürür');
  } else {
    try {
      await hizLimitiSifirla(REDIS_URL);
    } catch (e) {
      problems.push(`Redis'e ulaşılamadı (${REDIS_URL}): ${(e as Error).message}`);
    }
  }

  if (problems.length > 0) {
    throw new Error(
      'Uçtan uca testler için ortam hazır değil:\n  - ' +
        problems.join('\n  - ') +
        '\n\nBu testler sunucuları kendisi başlatmaz. Şunu çalıştırın:\n' +
        '  ./scripts/e2e.sh\n',
    );
  }
}
