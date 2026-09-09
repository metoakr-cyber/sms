import type { MetadataRoute } from 'next';
import { yazilariGetir } from './(genel)/blog/icerik';

const SITE = process.env.NEXT_PUBLIC_SITE_URL ?? 'http://localhost:3000';

/**
 * Yalnız GENEL sayfalar. Panel ve yönetim ASLA site haritasına girmez.
 * test: web/scripts/seo-check.mjs  ("gizli yol haritada" denetimi — haritadaki
 * her <loc> yolu /panel|/yonetim|/api desenine karşı sınanır)
 */
export default function sitemap(): MetadataRoute.Sitemap {
  const now = new Date();

  // Blog yazıları DOSYADAN okunur ve haritaya ELLE eklenmez: yeni yazı
  // eklendiğinde birinin haritayı güncellemeyi hatırlaması gerekseydi,
  // yazılar er geç haritasız kalırdı.
  const blog = yazilariGetir().map((y) => ({
    url: `${SITE}/blog/${y.slug}`,
    lastModified: new Date(y.tarih),
    changeFrequency: 'monthly' as const,
    priority: 0.5,
  }));

  return [
    { url: `${SITE}/`,        lastModified: now, changeFrequency: 'daily',  priority: 1 },
    { url: `${SITE}/fiyatlar`,lastModified: now, changeFrequency: 'daily',  priority: 0.9 },
    // Kiralama ayrı bir ÜRÜN ve ayrı bir arama niyeti ("numara kiralama",
    // "aylık sanal numara"). Ana sayfaya gömülü bir bölüm olsaydı bu
    // aramalarda hiç görünmezdi.
    { url: `${SITE}/kiralama`,lastModified: now, changeFrequency: 'daily',  priority: 0.9 },
    // Tam servis listesi. Katalog senkronla günde birkaç kez değişir ve
    // "X için sanal numara" aramalarının indiği sayfa burasıdır.
    { url: `${SITE}/servisler`, lastModified: now, changeFrequency: 'daily', priority: 0.8 },
    { url: `${SITE}/sss`,     lastModified: now, changeFrequency: 'weekly', priority: 0.7 },
    // Hakkımızda ve İletişim fiyat/stoktan bağımsızdır, yılda birkaç kez
    // değişir — bu yüzden 'monthly'. Öncelikleri SSS'in altında ama giriş
    // sayfalarının üstünde: satın alma niyeti taşımazlar, ama bir hizmete
    // para yatırmadan önce bakılan sayfalar bunlardır.
    { url: `${SITE}/hakkimizda`, lastModified: now, changeFrequency: 'monthly', priority: 0.6 },
    { url: `${SITE}/iletisim`,   lastModified: now, changeFrequency: 'monthly', priority: 0.6 },
    { url: `${SITE}/blog`,    lastModified: now, changeFrequency: 'weekly', priority: 0.6 },
    ...blog,
    { url: `${SITE}/kayit`,   lastModified: now, changeFrequency: 'yearly', priority: 0.5 },
    // Yasal metinler. ARTIK YÜRÜRLÜKTELER — "taslaktır" uyarısı kaldırıldı ve
    // ikisi de gerçek, bağlayıcı içerik taşıyor; haritadan uzak tutmanın
    // gerekçesi (yürürlükte olmayan bir sözleşmeyi yürürlükteymiş gibi
    // sunmamak) ortadan kalktı.
    //
    // priority 0.3 — haritadaki EN DÜŞÜK değer. Bu sayfalar satın alma niyeti
    // taşıyan aramaların indiği yerler değil; ama bir hizmete para yatırmadan
    // önce okunan ve mevzuatın yayımlanmasını beklediği belgelerdir, yani
    // "haritada hiç olmasın" da değil.
    //
    // changeFrequency 'yearly' — 'monthly' demek, ayda bir değişmediğini gören
    // tarayıcıya yalan söylemektir ve zamanla haritanın tamamındaki sinyali
    // değersizleştirir. Bir sözleşme metni yılda bir bile değişmeyebilir;
    // değiştiğinde `lastModified` zaten `now` olarak yeni tarihi taşır.
    { url: `${SITE}/gizlilik`,         lastModified: now, changeFrequency: 'yearly', priority: 0.3 },
    { url: `${SITE}/kullanim-sartlari`,lastModified: now, changeFrequency: 'yearly', priority: 0.3 },
  ];
  // NEDEN /giris ÇIKARILDI: sayfanın kendi metadata'sı `robots: noindex`
  // veriyor (frontend-contract.md §10.1 — kimlik sayfaları indekslenmez).
  // Aynı URL'i bir yandan "indeksleme" deyip bir yandan site haritasıyla
  // Google'a SUNMAK çelişkidir; Search Console bunu "noindex olarak
  // işaretlenmiş URL gönderildi" hatasıyla raporlar ve haritanın tamamına
  // olan güveni düşürür. `scripts/seo-check.mjs` bu çelişkiyi artık ölçüyor:
  // haritadaki her URL çekilip noindex olmadığı doğrulanıyor.
  //
  // /gizlilik ve /kullanim-sartlari için AYNI DENETİM GEÇERLİ: ikisinin de
  // kendi `metadata`sında `robots` alanı YOKTUR, yani indekslenebilirler.
  // Birine noindex eklenirse bu satırların da kaldırılması gerekir — seo-check
  // aksi hâlde "noindex ama haritada" diye düşer.
}
