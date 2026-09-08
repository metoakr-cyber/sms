import 'server-only';

/**
 * Sunucu bileşenlerinden API çağrısı.
 *
 * `next.config.ts` içindeki rewrite YALNIZ tarayıcı isteklerine uygulanır;
 * sunucu tarafında göreli `/api/...` yolunun bir kökeni yoktur. Bu yüzden
 * burada API kökenine DOĞRUDAN gideriz.
 *
 * Yalnız GENEL (oturumsuz) uçlar içindir. Kullanıcıya özel veri sunucu
 * bileşeninde çekilmez: sayfa önbelleğe alınırsa bir kullanıcının verisi
 * başkasına servis edilir.
 */
const API_ORIGIN = process.env.API_ORIGIN ?? 'http://127.0.0.1:8091';

export async function fetchPublic<T>(
  path: string,
  opts: { revalidate?: number } = {},
): Promise<T | null> {
  try {
    const res = await fetch(`${API_ORIGIN}/api/v1${path}`, {
      headers: { Accept: 'application/json' },
      next: { revalidate: opts.revalidate ?? 300 },
      signal: AbortSignal.timeout(5_000),
    });
    if (!res.ok) return null;
    const ct = res.headers.get('content-type') ?? '';
    if (!ct.includes('application/json')) return null;
    return (await res.json()) as T;
  } catch {
    // API kapalıysa GENEL SAYFA YİNE AÇILIR, sadece dinamik liste boş kalır.
    // Pazarlama sayfasını bir arka uç arızasında 500'e düşürmek, SEO'da
    // sayfanın dizinden düşmesi demektir.
    return null;
  }
}
