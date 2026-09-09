import { test, expect } from './fixtures';

/**
 * FR-104 oturum yönetimi · FR-105 rol ve izin.
 *
 * Oturumsuz kullanıcı panele giremez. Yönlendirme `devam` parametresini taşır:
 * giriş yaptıktan sonra kullanıcı gitmek istediği yere döner — aksi halde
 * paylaşılan her bağlantı kullanıcıyı özet sayfasına atardı.
 */
test.describe('oturum kapısı', () => {
  for (const yol of ['/panel', '/panel/cuzdan', '/panel/numara-al', '/panel/siparisler']) {
    test(`oturumsuz kullanıcı ${yol} yolundan /giris'e yönlendirilir`, async ({ page }) => {
      await page.goto(yol);

      await expect(page).toHaveURL(
        `/giris?sebep=oturum&devam=${encodeURIComponent(yol)}`,
      );
      await expect(page.getByRole('heading', { name: 'Giriş yap' })).toBeVisible();
      await expect(page.getByText('Oturumunuzun süresi doldu.')).toBeVisible();
    });
  }

  test("oturumsuz kullanıcı /yonetim yolundan /giris'e yönlendirilir", async ({ page }) => {
    await page.goto('/yonetim');
    await expect(page).toHaveURL(/\/giris\?sebep=oturum/);
    await expect(page.getByRole('heading', { name: 'Giriş yap' })).toBeVisible();
  });

  test('yetkisiz ama oturumlu kullanıcı yönetim ekranını göremez', async ({
    kullaniciSayfasi: page,
  }) => {
    await page.goto('/yonetim');
    await expect(page.getByText('Yönetim paneli yalnız yetkili hesaplara açıktır.')).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Genel bakış' })).toHaveCount(0);
  });

  test('çıkış yapan kullanıcı panele geri dönemez', async ({ kullaniciSayfasi: page }) => {
    await page.goto('/panel');
    await expect(page.getByRole('heading', { name: /^Merhaba,/ })).toBeVisible();

    const res = await page.request.post('/api/v1/auth/logout');
    expect(res.status()).toBe(200);

    await page.goto('/panel');
    await expect(page).toHaveURL(/\/giris\?sebep=oturum/);
  });
});
