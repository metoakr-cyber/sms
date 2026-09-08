import type { MetadataRoute } from 'next';

const SITE = process.env.NEXT_PUBLIC_SITE_URL ?? 'http://localhost:3000';

/**
 * frontend-contract.md §10.1 — EN KRİTİK SEO KURALI: panel indekslenmez.
 *
 * robots.txt TEK BAŞINA YETMEZ: taramayı engeller ama başka bir yerden
 * bağlantı verilmiş bir URL yine indekslenebilir. Asıl engel `X-Robots-Tag`
 * başlığıdır (next.config.ts) — burası ikinci katman.
 */
export default function robots(): MetadataRoute.Robots {
  return {
    rules: [{ userAgent: '*', allow: '/', disallow: ['/panel/', '/yonetim/', '/api/'] }],
    sitemap: `${SITE}/sitemap.xml`,
    host: SITE,
  };
}
