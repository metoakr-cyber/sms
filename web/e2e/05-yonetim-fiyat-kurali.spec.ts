import { test, expect, type Locator, type Page } from './fixtures';

/**
 * FR-303 fiyat kuralları · FR-304 fiyat hesaplama.
 *
 * Kural `SERVICE_COUNTRY` kapsamında ve `am × US` üzerine yazılır — GLOBAL'e
 * DOKUNULMAZ. GLOBAL kuralı değiştirmek katalogdaki HER ürünün fiyatını
 * kaydırırdı; numara alma testi de fiyat üzerinden doğrulama yapıyor ve bu
 * test onu gizemli biçimde kırardı. `am × US` başka hiçbir akışta kullanılmıyor
 * (satın alma `tg × RU` üzerinden gider).
 */
const SERVIS = 'am';
const ULKE = 'US';

test.describe('yönetim · fiyat kuralı', () => {
  test('marj değişince önizleme değişir; kural yazılınca listede görünür', async ({
    yoneticiSayfasi: page,
  }) => {
    await page.goto('/yonetim/fiyatlar');
    await expect(page.getByRole('heading', { name: 'Fiyat kuralları' })).toBeVisible();

    await secim(page, 'Kapsam').selectOption('SERVICE_COUNTRY');
    await secim(page, 'Servis').selectOption(SERVIS);
    await secim(page, 'Ülke').selectOption(ULKE);

    const marj = page.getByLabel('Marj (%)');

    // ── Düşük marj ──
    const dusuk = await marjUygulaVeFiyatOku(page, marj, '10');

    // ── Yüksek marj ── benzersiz değer: iki koşunun kuralı karışmasın.
    const yuksekMarj = String(300 + Math.floor(Math.random() * 500));
    const yuksek = await marjUygulaVeFiyatOku(page, marj, yuksekMarj);

    expect(
      yuksek,
      `marj %10 → %${yuksekMarj} fiyatı artırmalıydı (${dusuk} → ${yuksek} kuruş)`,
    ).toBeGreaterThan(dusuk);

    // ── Kuralı yaz ──
    await page.getByLabel('Not').fill(`Uçtan uca test kuralı (%${yuksekMarj}).`);
    await page.getByRole('button', { name: 'Kuralı kaydet' }).click();

    // "Kaydet" bir GÜNCELLEME DEĞİL, yeni kayıttır — bu yüzden ayrı bir onay.
    const onay = page.getByRole('dialog');
    await expect(onay).toBeVisible();
    await onay.getByRole('button', { name: 'Evet, kuralı yaz' }).click();

    await expect(page.getByText('Kural yazıldı:')).toBeVisible();

    // ── Etkin kurallar listesinde görünür ──
    await expect(page.getByRole('heading', { name: 'Etkin kurallar' })).toBeVisible();
    await expect(
      page
        .locator('li:visible, tr:visible')
        .filter({ hasText: `${SERVIS} × ${ULKE}` })
        .filter({ hasText: yuksekMarj })
        .first(),
    ).toBeVisible();
  });
});

/**
 * Bu ekrandaki bir `<select>`i başlığından bulur.
 *
 * `getByLabel` KULLANILAMAZ: etiketler `<select>`i SARAR ve bir etiketin metni
 * tüm alt düğümlerini — `<option>` metinleri dahil — içerir. "Kapsam"
 * etiketinin metni "Servis" ve "Servis + Ülke" seçeneklerini de taşıdığı için
 * `getByLabel('Servis')` İKİ öğeye birden çözülüyordu (ölçüldü: strict mode
 * ihlali). Başlık span'i tam eşleşmeyle aranınca belirsizlik kalmaz.
 */
function secim(page: Page, baslik: string): Locator {
  return page
    .locator('label')
    .filter({ has: page.locator('span').filter({ hasText: new RegExp(`^${baslik}$`) }) })
    .locator('select');
}

/**
 * Marjı yazar, ÖNİZLEMENİN O MARJI UYGULADIĞINI doğrular ve fiyatı okur.
 *
 * Senkronizasyon noktası rozettir, süre değil: önizleme 500 ms gecikmeli
 * sorgu atar ve rozet (`Marj %x`) sunucunun aynı istekte döndürdüğü değerdir.
 * Sabit beklemeyle okunsaydı, yavaş bir koşuda ÖNCEKİ marjın fiyatı okunur ve
 * karşılaştırma sessizce anlamsızlaşırdı.
 */
async function marjUygulaVeFiyatOku(page: Page, marj: Locator, deger: string): Promise<number> {
  await marj.fill(deger);
  await expect(page.getByText('Formdaki kural (kaydedilmedi)')).toBeVisible();
  await expect(page.getByText(`Marj %${deger}`, { exact: true })).toBeVisible();

  const fiyat = page
    .getByText('Satış fiyatı', { exact: true })
    .locator('xpath=following-sibling::p[1]');
  await expect(fiyat).toBeVisible();
  return kurusaCevir(await fiyat.innerText());
}

/**
 * Biçimlenmiş TL metnini kuruşa çevirir ("1.234,56 ₺" → 123456).
 *
 * Karşılaştırma SAYI üzerinden yapılır: metin karşılaştırmasında "9,90" ile
 * "10,00" ters sırada çıkar ve test yanlış yerde yeşil olurdu.
 */
function kurusaCevir(metin: string): number {
  const m = /([\d.]*\d),(\d{2})/.exec(metin);
  expect(m, `satış fiyatı okunamadı: "${metin}"`).not.toBeNull();
  const tam = (m?.[1] ?? '0').replace(/\./g, '');
  const kurus = m?.[2] ?? '00';
  return Number(tam) * 100 + Number(kurus);
}
