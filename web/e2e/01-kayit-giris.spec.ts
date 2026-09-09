import {
  test,
  expect,
  yeniKullanici,
  kayitKilidiyle,
  logBoyutu,
  dogrulamaTokeni,
} from './fixtures';

/**
 * FR-100 kayıt · FR-101 e-posta doğrulama · FR-102 giriş.
 *
 * Akış tamamen ARAYÜZDEN yürütülür: form doldurulur, gönderilir, doğrulama
 * bağlantısına gidilir, giriş yapılır ve panel açılır. API kısayolu yoktur —
 * bu testin varlık sebebi tam olarak formların çalıştığını göstermektir.
 */
test.describe('kayıt → doğrulama → giriş', () => {
  test('yeni kullanıcı kaydolur, e-postasını doğrular ve panele girer', async ({ page }) => {
    const u = yeniKullanici();

    await page.goto('/kayit');
    await expect(page.getByRole('heading', { name: 'Hesap aç' })).toBeVisible();

    await page.getByLabel('E-posta').fill(u.email);
    await page.getByLabel('Kullanıcı adı').fill(u.username);
    await page.getByLabel('Şifre', { exact: true }).fill(u.password);
    await page.getByRole('checkbox').check();

    const gonder = page.getByRole('button', { name: 'Hesap aç' });

    /*
      reCAPTCHA yapılandırılmışsa arayüzden kayıt OTOMATİKLEŞTİRİLEMEZ.

      Google'ın onay kutusunu bir bot olarak geçmek mümkün değildir ve
      geçmeye çalışmak testi kararsız yapardı. Bu durumda test ATLANIR ve
      sebebi raporda AÇIKÇA görünür — sessizce yeşil dönmez.
      Kutu çözülmeden gönder düğmesi `disabled` kalır (kayit/form.tsx).
    */
    if (await gonder.isDisabled()) {
      test.skip(
        true,
        'reCAPTCHA yapılandırılmış (NEXT_PUBLIC_RECAPTCHA_SITE_KEY dolu); ' +
          'arayüzden kayıt otomatikleştirilemez.',
      );
    }

    // Kayıt ile token okuma AYNI kilit altında: log satırları kaydın kime ait
    // olduğunu söylemez, araya başka bir kayıt girerse yanlış token okunur.
    const token = await kayitKilidiyle(async () => {
      const ofset = logBoyutu();
      await gonder.click();
      await expect(page).toHaveURL(/\/giris\?kayit=tamam$/);
      return dogrulamaTokeni(ofset);
    });

    await expect(page.getByText('Kaydınız alındı.')).toBeVisible();

    // ── Doğrulama ──
    await page.goto(`/dogrula?token=${encodeURIComponent(token)}`);
    await expect(page.getByText('E-posta adresiniz doğrulandı.')).toBeVisible();

    // ── Giriş ──
    await page.goto('/giris');
    await page.getByLabel('E-posta').fill(u.email);
    await page.getByLabel('Şifre', { exact: true }).fill(u.password);
    await page.getByRole('button', { name: 'Giriş yap' }).click();

    await expect(page).toHaveURL(/\/panel$/);
    await expect(page.getByRole('heading', { name: `Merhaba, ${u.username}` })).toBeVisible();
    // Doğrulanmış hesap uyarı bandını GÖRMEZ.
    await expect(page.getByText('E-posta adresiniz doğrulanmamış.')).toHaveCount(0);
  });

  test('yanlış parola girişi reddedilir ve panele geçilemez', async ({ page, kullanici }) => {
    await page.goto('/giris');
    await page.getByLabel('E-posta').fill(kullanici.email);
    await page.getByLabel('Şifre', { exact: true }).fill(`${kullanici.password}-yanlis`);
    await page.getByRole('button', { name: 'Giriş yap' }).click();

    // Hata bandı görünür; adres çubuğu /giris'te kalır.
    await expect(page.getByRole('alert').first()).toBeVisible();
    await expect(page).toHaveURL(/\/giris$/);
  });
});
