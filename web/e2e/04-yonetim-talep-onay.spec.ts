import { test, expect, bakiyeOku } from './fixtures';

/**
 * FR-502 · yönetici bakiye talebini onaylar, kullanıcının bakiyesi artar.
 *
 * Onay GERÇEK PARA yazar. Bu yüzden akış üç adımlıdır (talepler/page.tsx):
 * ayrıntı → tutar formu → ayrı bir onay ekranı. Test üçünü de geçer, çünkü
 * kısayol eklenirse bu test onu FARK ETMEZ ise kapı işe yaramaz.
 */
test.describe('yönetim · bakiye talebi onayı', () => {
  test('onaylanan talep kullanıcının bakiyesine yazılır', async ({
    yoneticiSayfasi: page,
    kullanici,
    kullaniciApi,
    odemeYontemi,
  }) => {
    const tutarMinor = 250_00;
    const referans = `E2E-onay-${odemeYontemi.code}`;

    // ── Kurulum: kullanıcı kendi talebini açar ──
    const talep = await kullaniciApi.post('/api/v1/wallet/deposits', {
      data: {
        methodId: odemeYontemi.id,
        amountMinor: tutarMinor,
        reference: referans,
        note: 'uçtan uca test',
      },
      headers: { 'Idempotency-Key': `e2e-${referans}` },
    });
    expect(talep.status(), `talep oluşturulamadı: ${await talep.text()}`).toBe(200);

    const oncekiBakiye = await bakiyeOku(kullaniciApi);

    // ── Yönetici talebi bulur ──
    await page.goto('/yonetim/talepler');
    await expect(page.getByRole('heading', { name: 'Bakiye talepleri' })).toBeVisible();

    // Liste en yeniden eskiye sıralıdır (ListDepositsForAdmin) ve varsayılan
    // süzgeç PENDING'dir; bu testin talebi en üsttedir. Yine de kullanıcı
    // adıyla eşleştiririz — sıraya güvenmek başka bir testin verisini
    // onaylamak demek olurdu.
    // `:visible` ZORUNLU: aynı veri iki kez çizilir — mobilde kart listesi,
    // masaüstünde tablo (§2.5). Biri her zaman gizlidir; görünmeyeni
    // tıklamaya çalışmak dört projenin ikisinde zaman aşımı demekti.
    const satir = page
      .locator('li:visible, tr:visible')
      .filter({ hasText: kullanici.username })
      .first();
    await expect(satir).toBeVisible();
    await satir.getByRole('button', { name: /İncele/ }).click();

    const modal = page.getByRole('dialog');
    await expect(modal.getByText(kullanici.email)).toBeVisible();

    // ── Adım 1: onaya gir ──
    await modal.getByRole('button', { name: 'Onayla', exact: true }).click();

    // ── Adım 2: yazılacak tutar (boş = bildirilen tutar) ──
    await expect(modal.getByLabel('Yazılacak tutar (TL)')).toBeVisible();
    await modal.getByRole('button', { name: 'Devam et' }).click();

    // ── Adım 3: geri alınamaz işlem için ayrı onay ──
    await expect(modal.getByText('Bu işlem geri alınamaz.')).toBeVisible();
    await modal.getByRole('button', { name: 'Evet, bakiyeye yaz' }).click();

    // ── Sonuç ──
    await expect(modal.getByText('Bakiyeye yazılan')).toBeVisible();
    await expect(modal.getByText('Kullanıcının yeni bakiyesi')).toBeVisible();

    // ── Kullanıcının bakiyesi gerçekten arttı mı ──
    expect(await bakiyeOku(kullaniciApi)).toBe(oncekiBakiye + tutarMinor);
  });
});
