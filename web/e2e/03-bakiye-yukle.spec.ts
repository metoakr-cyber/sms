import { test, expect } from './fixtures';

/**
 * FR-500 … FR-503 · bakiye yükleme talebi.
 *
 * Ödeme yöntemi bu testin KENDİ kurduğu, benzersiz kodlu bir kayıttır ve test
 * bitince silinir. Var olan yöntemlere dokunulmaz: paylaşılan bir yöntemi
 * aktifleştirip kapatmak, aynı anda koşan başka bir testin altından zemini
 * çekerdi.
 */
test.describe('cüzdan ve bakiye yükleme', () => {
  test('kullanıcı yöntem seçip talep oluşturur ve talebi geçmişinde görür', async ({
    kullaniciSayfasi: page,
    odemeYontemi,
  }) => {
    const referans = `E2E-${odemeYontemi.code}`;

    // ── Cüzdan ──
    await page.goto('/panel/cuzdan');
    await expect(page.getByRole('heading', { name: 'Cüzdan', exact: true })).toBeVisible();
    await expect(page.getByText('Kullanılabilir bakiye')).toBeVisible();

    await page.getByRole('link', { name: 'Bakiye yükle' }).first().click();
    await expect(page).toHaveURL(/\/panel\/bakiye-yukle$/);

    // ── Yöntem seçimi ──
    const yontemKutusu = page.getByRole('radio').and(page.locator('[name="odeme-yontemi"]'));
    await expect(yontemKutusu.first()).toBeVisible();
    await page.getByText(odemeYontemi.name, { exact: true }).click();

    // Hesap bilgileri: kullanıcı parayı NEREYE göndereceğini görmeli.
    await expect(page.getByRole('heading', { name: 'Nereye göndereceksiniz' })).toBeVisible();
    await expect(page.getByText('TR000000000000000000000000')).toBeVisible();

    // ── Talep formu ──
    await page.getByLabel('Tutar (TL)').fill('250,00');
    await page.getByLabel('Havale açıklaması / dekont numarası').fill(referans);
    await page.getByRole('button', { name: 'Talebi oluştur' }).click();

    // ── Sonuç ──
    await expect(page.getByRole('heading', { name: 'Talebiniz alındı' })).toBeVisible();
    await expect(page.getByText('250,00', { exact: false }).first()).toBeVisible();

    // ── Geçmişte görünür ──
    const gecmis = page.getByRole('heading', { name: 'Yükleme talepleriniz' });
    await expect(gecmis).toBeVisible();
    await expect(page.getByText(odemeYontemi.name).first()).toBeVisible();
    await expect(page.getByText('Onay bekliyor').first()).toBeVisible();
  });

  test('en az tutarın altındaki talep reddedilir', async ({
    kullaniciSayfasi: page,
    odemeYontemi,
  }) => {
    await page.goto('/panel/bakiye-yukle');
    await page.getByText(odemeYontemi.name, { exact: true }).click();

    await page.getByLabel('Tutar (TL)').fill('1,00');
    await page.getByLabel('Havale açıklaması / dekont numarası').fill('E2E-cok-dusuk');
    await page.getByRole('button', { name: 'Talebi oluştur' }).click();

    // Talep OLUŞMAZ ve kullanıcı ne yapması gerektiğini görür.
    await expect(page.getByRole('alert').filter({ hasText: /En az/ }).first()).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Talebiniz alındı' })).toHaveCount(0);
  });
});
