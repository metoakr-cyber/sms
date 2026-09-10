import { test, expect, type Page } from './fixtures';

/**
 * NFR-809 / KK-809 · 320 px'te hiçbir sayfada yatay kaydırma YOK.
 *
 * 320 px iPhone SE genişliğidir ve sözleşmedeki en dar zorunlu ölçüdür
 * (docs/frontend-contract.md §2.2). Yatay kaydırma çubuğu çıkarsa HATA sayılır:
 * mobilde sağa kayan bir sayfada düğmelerin yarısı ekranın dışında kalır.
 *
 * Ölçüm göz kararı değildir: `scrollWidth <= clientWidth`.
 */

const GENISLIK = 320;

const GENEL_SAYFALAR = [
  '/', '/fiyatlar', '/sss', '/kiralama', '/hakkimizda', '/iletisim',
  '/giris', '/kayit', '/sifremi-unuttum',
  '/gizlilik', '/kullanim-sartlari',
];

const PANEL_SAYFALARI = [
  '/panel', '/panel/numara-al', '/panel/siparisler',
  '/panel/cuzdan', '/panel/bakiye-yukle', '/panel/destek', '/panel/hesap',
  // Hukuki metinlerin panel kopyaları. Genel sürümleri zaten yukarıda ölçülüyor
  // ama kap farklı: panelde içerik tam genişliktir ve sol yan sütun/alt gezinti
  // eklenir — yani 320 px'teki taşma riski aynı metin için ayrı bir sorudur.
  '/panel/kullanim-sartlari', '/panel/gizlilik',
];

const YONETIM_SAYFALARI = [
  '/yonetim', '/yonetim/talepler', '/yonetim/kullanicilar', '/yonetim/bakiye',
  '/yonetim/fiyatlar', '/yonetim/odeme-yontemleri', '/yonetim/saglayicilar',
  '/yonetim/destek', '/yonetim/denetim',
];

/**
 * Sayfayı açar ve yatay taşma ölçer.
 *
 * `+1` toleransı: tarayıcılar `scrollWidth`i kesirli düzenlerde bir piksel
 * yukarı yuvarlar (WebKit'te sık). Bir piksel kullanıcıya kaydırma çubuğu
 * göstermez; tolerans olmadan test motora göre farklı sonuç verirdi.
 */
async function tasmaOlc(page: Page, yol: string): Promise<{ scrollWidth: number; clientWidth: number }> {
  await page.goto(yol);

  // İskelet ekranı ölçmek, ASIL içeriğin taştığını görmeden yeşil dönmek
  // olurdu. Panel ve yönetim kabukları oturum yüklenene kadar tam ekran bir
  // yükleniyor göstergesi çizer; o kalkmadan ölçüm anlamsızdır.
  await expect(page.getByText('Yükleniyor', { exact: true })).toHaveCount(0);
  await expect(page.locator('body')).toBeVisible();

  return page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    clientWidth: document.documentElement.clientWidth,
  }));
}

test.describe('mobil · 320 px yatay taşma', () => {
  test('genel sayfalarda yatay kaydırma yok', async ({ page }) => {
    await page.setViewportSize({ width: GENISLIK, height: 720 });

    for (const yol of GENEL_SAYFALAR) {
      const { scrollWidth, clientWidth } = await tasmaOlc(page, yol);
      expect(scrollWidth, `${yol} 320 px'te yatay kayıyor`).toBeLessThanOrEqual(clientWidth + 1);
    }
  });

  test('panel sayfalarında yatay kaydırma yok', async ({ kullaniciSayfasi: page }) => {
    await page.setViewportSize({ width: GENISLIK, height: 720 });

    for (const yol of PANEL_SAYFALARI) {
      const { scrollWidth, clientWidth } = await tasmaOlc(page, yol);
      expect(scrollWidth, `${yol} 320 px'te yatay kayıyor`).toBeLessThanOrEqual(clientWidth + 1);
    }
  });

  test('yönetim sayfalarında yatay kaydırma yok', async ({ yoneticiSayfasi: page }) => {
    await page.setViewportSize({ width: GENISLIK, height: 720 });

    for (const yol of YONETIM_SAYFALARI) {
      const { scrollWidth, clientWidth } = await tasmaOlc(page, yol);
      expect(scrollWidth, `${yol} 320 px'te yatay kayıyor`).toBeLessThanOrEqual(clientWidth + 1);
    }
  });
});
