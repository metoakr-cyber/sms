import { test, expect, bakiyeEkle, bakiyeOku } from './fixtures';

/**
 * FR-305 fiyat teklifi · FR-400 satın alma · FR-403/FR-411 kod teslimi.
 *
 * Sağlayıcı geliştirme ortamında FAKE'tir: gerçek para harcanmaz, gerçek
 * numara alınmaz. `tg × RU` seçilir çünkü sahte katalogda bol stoklu ve ucuz;
 * `wa × TR` bilerek STOKSUZ tutulur (gerçek gözlemin taklidi).
 */

const SERVIS = 'Telegram';
const ULKE = 'RU';

test.describe('numara alma', () => {
  test('servis ve ülke seçilince fiyat görünür', async ({ kullaniciSayfasi: page }) => {
    await page.goto('/panel/numara-al');
    await expect(page.getByRole('heading', { name: 'Numara Al' })).toBeVisible();

    await page.getByRole('button', { name: SERVIS }).click();

    const modal = page.getByRole('dialog');
    await expect(modal.getByRole('heading', { name: SERVIS })).toBeVisible();

    await modal.getByLabel('Ülke Seçin').selectOption(ULKE);

    await expect(modal.getByText('Stok Durumu')).toBeVisible();
    await expect(modal.getByText('Birim Fiyat')).toBeVisible();

    // Fiyat SUNUCUDAN gelir ve TL olarak biçimlenir; istemcide para
    // aritmetiği yapılmaz (CLAUDE.md #1).
    const satinAl = modal.getByRole('button', { name: /^Satın Al/ });
    await expect(satinAl).toBeVisible();
    await expect(satinAl).toContainText('₺');
  });

  test('bakiyesi olan kullanıcı numara alır ve kod ekrana düşer', async ({
    kullaniciSayfasi: page,
    kullanici,
    kullaniciApi,
    yonetici,
  }) => {
    // Kod sunucu tarafı yoklamasıyla gelir (worker `order-poller`, 30 sn).
    test.setTimeout(180_000);

    const baslangic = await bakiyeOku(kullaniciApi);
    await bakiyeEkle(yonetici, kullanici.id, 100_00, 'uçtan uca test kurulumu');
    const yuklendi = baslangic + 100_00;
    expect(await bakiyeOku(kullaniciApi)).toBe(yuklendi);

    await page.goto('/panel/numara-al');
    await page.getByRole('button', { name: SERVIS }).click();

    const modal = page.getByRole('dialog');
    await modal.getByLabel('Ülke Seçin').selectOption(ULKE);

    const satinAl = modal.getByRole('button', { name: /^Satın Al/ });
    await expect(satinAl).toBeEnabled();
    await satinAl.click();

    // ── Kod bekleme ekranı ──
    await expect(modal.getByText('Tahsis edilen numara')).toBeVisible();
    await expect(modal.getByRole('button', { name: 'Numarayı kopyala' })).toBeVisible();

    // Satın alma bir PARA HAREKETİDİR: bakiye gerçekten düşmüş olmalı.
    await expect
      .poll(() => bakiyeOku(kullaniciApi), {
        timeout: 15_000,
        message: 'satın alma sonrası bakiye düşmedi',
      })
      .toBeLessThan(yuklendi);

    /*
      Sahte sağlayıcı kodu anında hazırlar; sunucu tarafı yoklaması 30 sn'de
      bir koşar (worker/jobs.go `order-poller`). Bu yüzden pencere geniştir.
      SABİT BEKLEME DEĞİL: koşul sağlanır sağlanmaz devam eder.
    */
    await expect(modal.getByText('Doğrulama kodu geldi')).toBeVisible({ timeout: 120_000 });
    await expect(modal.getByRole('button', { name: 'Kodu kopyala' })).toBeVisible();

    // Sipariş geçmişte de görünür (FR-409).
    await page.goto('/panel/siparisler');
    await expect(page.getByRole('heading', { name: 'Siparişlerim' })).toBeVisible();
    // `:visible` ZORUNLU: aynı veri iki kez çizilir — mobilde kart listesi,
    // masaüstünde tablo (§2.5). Gizli olanı beklemek zaman aşımı demekti.
    await expect(
      page.locator('li:visible, tr:visible').filter({ hasText: SERVIS }).first(),
    ).toBeVisible();
  });

  test('stoksuz ülke seçilemez olarak gösterilir', async ({ kullaniciSayfasi: page }) => {
    await page.goto('/panel/numara-al');

    await page.getByRole('button', { name: 'Whatsapp' }).click();

    const modal = page.getByRole('dialog');
    // Kullanıcının kendi ülkesi listeden ELENMEZ, devre dışı gösterilir:
    // elemek "bu site Türk numarası satmıyor" izlenimi verirdi.
    const turkiye = modal.locator('option[value="TR"]');
    // `toBeDisabled` <option> için ÇALIŞMAZ (yalnız form denetimlerine bakar);
    // öznitelik doğrudan okunur.
    await expect(turkiye).toHaveAttribute('disabled', '');
    await expect(turkiye).toContainText('stok yok');
  });
});
