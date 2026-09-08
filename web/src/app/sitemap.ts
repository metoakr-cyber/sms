import type { MetadataRoute } from 'next';

const SITE = process.env.NEXT_PUBLIC_SITE_URL ?? 'http://localhost:3000';

/** Yalnız GENEL sayfalar. Panel ve yönetim ASLA site haritasına girmez. */
export default function sitemap(): MetadataRoute.Sitemap {
  const now = new Date();
  return [
    { url: `${SITE}/`,        lastModified: now, changeFrequency: 'daily',  priority: 1 },
    { url: `${SITE}/fiyatlar`,lastModified: now, changeFrequency: 'daily',  priority: 0.9 },
    // Kiralama ayrı bir ÜRÜN ve ayrı bir arama niyeti ("numara kiralama",
    // "aylık sanal numara"). Ana sayfaya gömülü bir bölüm olsaydı bu
    // aramalarda hiç görünmezdi.
    { url: `${SITE}/kiralama`,lastModified: now, changeFrequency: 'daily',  priority: 0.9 },
    { url: `${SITE}/sss`,     lastModified: now, changeFrequency: 'weekly', priority: 0.7 },
    { url: `${SITE}/giris`,   lastModified: now, changeFrequency: 'yearly', priority: 0.4 },
    { url: `${SITE}/kayit`,   lastModified: now, changeFrequency: 'yearly', priority: 0.5 },
  ];
}
