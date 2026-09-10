/**
 * API istemcisi — docs/frontend-contract.md §3.1'in birebir uygulaması.
 *
 * Buradaki her madde bir TARAYICI HATASINI önler, stil tercihi değildir.
 * Sırayı veya maddeleri değiştirmeden önce sözleşmedeki gerekçe tablosunu okuyun.
 */

export type ApiErrorCode =
  | 'TIMEOUT' | 'NETWORK' | 'GATEWAY_ERROR' | 'INVALID_RESPONSE'
  | 'UNAUTHENTICATED' | 'RATE_LIMITED' | (string & {});

export interface FieldError { field: string; message: string }

export class ApiError extends Error {
  readonly code: ApiErrorCode;
  readonly status: number;
  readonly requestId?: string;
  readonly fields?: FieldError[];
  readonly retryAfterMs?: number;

  constructor(o: {
    code: ApiErrorCode; message: string; status?: number;
    requestId?: string; fields?: FieldError[]; retryAfterMs?: number;
  }) {
    super(o.message);
    this.name = 'ApiError';
    this.code = o.code;
    this.status = o.status ?? 0;
    this.requestId = o.requestId;
    this.fields = o.fields;
    this.retryAfterMs = o.retryAfterMs;
  }

  /** Sunucunun {error:{code,message,requestId}} + isteğe bağlı {fields:[]} zarfı. */
  static fromBody(body: unknown, status: number, retryAfterMs?: number): ApiError {
    const b = body as { error?: { code?: string; message?: string; requestId?: string }; fields?: FieldError[] };
    // Sunucu Türkçe, kullanıcıya gösterilebilir metin döner (design.md §11).
    const sunucuMesaji = b?.error?.message ?? 'Beklenmeyen bir hata oluştu.';
    return new ApiError({
      code: b?.error?.code ?? 'UNKNOWN',
      message: bekleyisiEkle(sunucuMesaji, status, retryAfterMs),
      status,
      requestId: b?.error?.requestId,
      fields: b?.fields,
      retryAfterMs,
    });
  }

  /** Alan bazlı hataları form alanı adına göre indeksler. */
  fieldMap(): Record<string, string> {
    const m: Record<string, string> = {};
    for (const f of this.fields ?? []) if (!(f.field in m)) m[f.field] = f.message;
    return m;
  }
}

const DEFAULT_TIMEOUT_MS = 15_000;

export interface ApiInit extends Omit<RequestInit, 'body'> {
  body?: unknown;
  timeoutMs?: number;
  idempotencyKey?: string;
}

const isMutating = (m?: string) =>
  !!m && ['POST', 'PUT', 'PATCH', 'DELETE'].includes(m.toUpperCase());

/**
 * Çift gönderimli CSRF token'ı.
 *
 * DURUM: Sunucu tarafı zorlama HENÜZ YOK (M6 sertleştirme kalemi). Bugünkü
 * koruma `SameSite=Lax` çerezidir; bu, çapraz siteden gelen POST'ları zaten
 * engeller. Başlığı şimdiden göndeririz ki sunucu zorlamayı açtığında istemci
 * tarafında değişiklik gerekmesin.
 *
 * Bu yorum bilerek "korunuyoruz" DEMEZ — vermediğimiz garantiyi yazmak,
 * okuyanı kontrolün var olduğuna inandırır (CLAUDE.md değişmez #29).
 */
function readCsrfCookie(): string {
  if (typeof document === 'undefined') return '';
  const m = document.cookie.match(/(?:^|;\s*)csrf=([^;]*)/);
  return m?.[1] ? decodeURIComponent(m[1]) : '';
}

/** AbortError tarayıcıdan tarayıcıya farklı sınıfla gelir; ada göre bakarız. */
function isAbortError(e: unknown): boolean {
  return e instanceof Error && (e.name === 'AbortError' || e.name === 'TimeoutError');
}

export async function apiFetch<T>(path: string, init: ApiInit = {}): Promise<T> {
  // (1) ZAMAN AŞIMI — fetch'in yerleşik zaman aşımı YOKTUR. Mobil ağda kopan
  //     bağlantıda arayüz sonsuza kadar "yükleniyor" kalırdı.
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), init.timeoutMs ?? DEFAULT_TIMEOUT_MS);

  // (2) Çağıranın kendi sinyalini ELLE birleştir: AbortSignal.any() Safari
  //     17.4'ten önce yok.
  const onOuterAbort = () => controller.abort();
  init.signal?.addEventListener('abort', onOuterAbort, { once: true });

  const method = init.method ?? 'GET';
  const hasBody = init.body !== undefined && init.body !== null;

  // (4b) DOSYA YÜKLEME. Dekont `multipart/form-data` ile gider ve bir FormData
  //      JSON'a çevrilemez. Content-Type'ı BİZ YAZMAYIZ: sınır dizesini
  //      (boundary) tarayıcı üretir, elle yazılan bir `multipart/form-data`
  //      başlığı sınırsız kalır ve sunucu gövdeyi ayrıştıramaz.
  const isForm = typeof FormData !== 'undefined' && init.body instanceof FormData;

  try {
    const res = await fetch(`/api/v1${path}`, {
      ...init,
      method,
      body: hasBody ? (isForm ? (init.body as FormData) : JSON.stringify(init.body)) : undefined,
      signal: controller.signal,
      // (3) Aynı alan adı olsa bile AÇIKÇA belirt — Safari ITP belirsizliğini kaldırır.
      credentials: 'same-origin',
      headers: {
        Accept: 'application/json',
        // (4) Content-Type YALNIZ gövde varsa; gövdesiz POST'ta bazı WAF'ları tetikler.
        ...(hasBody && !isForm ? { 'Content-Type': 'application/json' } : {}),
        ...(isMutating(method) ? { 'X-CSRF-Token': readCsrfCookie() } : {}),
        // (6) Para harcayan isteklerde çift harcamayı önler.
        ...(init.idempotencyKey ? { 'Idempotency-Key': init.idempotencyKey } : {}),
        ...init.headers,
      },
      // (7) Safari GET yanıtlarını agresif önbellekler → bakiye eski görünür.
      cache: init.cache ?? (isMutating(method) ? 'no-store' : 'default'),
    });

    // (7b) OTURUM BİTTİYSE DOĞRUDAN GİRİŞE. Ayrıntı ve muafiyetler
    //      `oturumBittiyseGirisEGonder` başında.
    if (res.status === 401) oturumBittiyseGirisEGonder(path);

    // (8) 204'te res.json() SyntaxError atar.
    if (res.status === 204) return undefined as T;

    const retryAfterMs = parseRetryAfter(res.headers.get('retry-after'));

    // (9) YANIT HER ZAMAN JSON DEĞİLDİR. 502/504'te vekil HTML döner ve
    //     res.json() patlar — üretimde en sık görülen arayüz hatası budur.
    const ct = res.headers.get('content-type') ?? '';
    if (!ct.includes('application/json')) {
      throw new ApiError({
        code: res.ok ? 'INVALID_RESPONSE' : 'GATEWAY_ERROR',
        message: 'Sunucuya ulaşılamadı. Lütfen tekrar deneyin.',
        status: res.status,
        retryAfterMs,
      });
    }

    const body: unknown = await res.json();
    if (!res.ok) throw ApiError.fromBody(body, res.status, retryAfterMs);
    return body as T;
  } catch (err) {
    // (10) HATA NORMALİZASYONU — çağıran TEK bir hata tipi görür.
    if (err instanceof ApiError) throw err;
    if (isAbortError(err)) {
      return Promise.reject(new ApiError({ code: 'TIMEOUT', message: 'İstek zaman aşımına uğradı.' }));
    }
    // TypeError: ağ/DNS/çevrimdışı. Chrome "Failed to fetch", Safari "Load failed"
    // der — MESAJA GÖRE AYRIM YAPILMAZ.
    return Promise.reject(new ApiError({
      code: 'NETWORK', message: 'Bağlantı kurulamadı. İnternetinizi kontrol edin.',
    }));
  } finally {
    clearTimeout(timer);
    init.signal?.removeEventListener('abort', onOuterAbort);
  }
}

/**
 * Oturum bittiyse kullanıcıyı GİRİŞ sayfasına gönderir.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * NEDEN
 * ══════════════════════════════════════════════════════════════════════════
 * Kabuklar (`panel-shell`, `admin-shell`) zaten yönlendiriyordu — ama YALNIZ
 * `/me` sorgusu 401 döndüğünde. Kullanıcı panelde dururken oturumu biterse ve
 * bir düğmeye basarsa, o isteğin 401'i ekranda bir HATA KUTUSU olarak
 * belirir: "Bu işlem için giriş yapmalısınız." Kullanıcı giriş yapmak ister
 * ama nereye tıklayacağını bilmez; sayfa hâlâ paneldir, veriler hâlâ ekrandadır.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * ÜÇ MUAFİYET — üçü de zorunlu
 * ══════════════════════════════════════════════════════════════════════════
 *  1. `/me` — 401 burada NORMAL cevaptır (giriş yapmamış ziyaretçi). Oturum
 *     kancası onu zaten "anonim" diye okuyor; yönlendirsek ana sayfaya giren
 *     HERKES giriş ekranına atılırdı.
 *  2. `/auth/*` — yanlış parolayla giriş denemesi de 401'dir. Yönlendirmek,
 *     kullanıcıyı hatasını görmeden aynı sayfaya geri atmak olurdu.
 *  3. Yalnız `/panel` ve `/yonetim` altında çalışır. Herkese açık bir sayfada
 *     401 alan bir istek varsa kullanıcının oturumu zaten yoktur ve onu giriş
 *     ekranına sürüklemek istemediği bir yere götürmektir.
 *
 * `replace` kullanılır, `assign` DEĞİL: geri tuşu oturumu bitmiş ölü sayfaya
 * dönmemeli. `devam` parametresi girişten sonra kullanıcıyı kaldığı yere
 * geri getirir — kabukların kullandığı biçimin aynısı.
 */
const OTURUM_MUAF = ['/me', '/auth/'] as const;
const KORUMALI_ALANLAR = ['/panel', '/yonetim'] as const;

function oturumBittiyseGirisEGonder(path: string): void {
  if (typeof window === 'undefined') return;
  if (OTURUM_MUAF.some((m) => path.startsWith(m))) return;

  const yol = window.location.pathname;
  if (!KORUMALI_ALANLAR.some((k) => yol.startsWith(k))) return;
  // Döngü koruması: giriş sayfasındayken tekrar giriş sayfasına gönderilmez.
  if (yol.startsWith('/giris')) return;

  const devam = encodeURIComponent(yol + window.location.search);
  window.location.replace(`/giris?sebep=oturum&devam=${devam}`);
}

/**
 * 429 mesajına GERÇEK bekleme süresini ekler.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * NEDEN
 * ══════════════════════════════════════════════════════════════════════════
 * Sunucunun mesajı "Çok fazla istek gönderdiniz. Lütfen biraz bekleyin."
 * — "biraz" ne kadar? Kullanıcı bilmediği için saniyede bir tekrar deniyor,
 * her deneme limiti yeniden dolduruyor ve bekleme UZUYOR. Süreyi söylemek
 * yalnız nezaket değil, sorunun kendisini çözer.
 *
 * Süre SUNUCUDAN gelir (`Retry-After` başlığı, `middleware/ratelimit.go`);
 * istemci tahmin etmez. Başlık yoksa mesaj olduğu gibi kalır.
 *
 * 🔴 MESAJ EKLENMEZ, DEĞİŞTİRİLİR — ve yalnız 429'da, yalnız süre bilindiğinde.
 * Sunucunun cümlesi zaten "Lütfen biraz bekleyin." diyor; sonuna "18 saniye
 * sonra tekrar deneyin." eklemek aynı şeyi iki kez söylerdi. Belirsiz yarı
 * atılır, yerine kesin olanı konur.
 *
 * Bu, istemcide ikinci bir hata sözlüğü kurmak DEĞİLDİR (CLAUDE.md #12):
 * tek bir hata kodunun (RATE_LIMITED) metnini, yalnız sunucunun kendi
 * gönderdiği veriyle (`Retry-After`) kesinleştiriyoruz. Başlık yoksa sunucu
 * metni olduğu gibi kalır — istemci hiçbir şey uydurmaz.
 *
 * Değer ANLIK bir görüntüdür: hata gösterildiği andaki kalan süredir, geri
 * sayan bir sayaç değildir. Sayaç kurmak her hata kutusuna zamanlayıcı
 * eklemek demekti; kullanıcının ihtiyacı "18 saniye mi, 10 dakika mı"
 * sorusunun cevabı, saniye saniye takip değil.
 */
function bekleyisiEkle(mesaj: string, status: number, retryAfterMs?: number): string {
  if (status !== 429 || retryAfterMs === undefined) return mesaj;
  const saniye = Math.ceil(retryAfterMs / 1000);
  if (saniye <= 0) return mesaj;
  const sure =
    saniye < 60
      ? `${saniye} saniye`
      : `${Math.ceil(saniye / 60)} dakika`;
  return `Çok fazla istek gönderdiniz. ${sure} sonra tekrar deneyin.`;
}

/** Retry-After hem saniye hem HTTP tarihi olabilir; ikisini de kabul ederiz. */
function parseRetryAfter(v: string | null): number | undefined {
  if (!v) return undefined;
  const secs = Number(v);
  if (Number.isFinite(secs)) return Math.max(0, secs * 1000);
  const at = Date.parse(v);
  return Number.isNaN(at) ? undefined : Math.max(0, at - Date.now());
}

/**
 * Yeniden denenebilir mi? (frontend-contract.md §3.2)
 * MUTASYONLAR BU FONKSİYONA HİÇ SORULMAZ — TanStack Query'de mutations.retry=false.
 */
export function isRetryable(err: unknown): boolean {
  if (!(err instanceof ApiError)) return false;
  if (err.code === 'NETWORK' || err.code === 'TIMEOUT' || err.code === 'GATEWAY_ERROR') return true;
  return err.status === 429 || err.status >= 500;
}

/* ─────────────── İkili (binary) yanıtlar ─────────────── */

export interface BlobResult {
  /**
   * 🔴 Object URL. Kullanan taraf işi bitince `URL.revokeObjectURL(url)`
   * çağırmak ZORUNDADIR — blob, sekme kapanana kadar bellekte kalır.
   */
  url: string;
  /** Sunucunun belirlediği tip (`image/jpeg`, `application/pdf`). */
  mime: string;
  size: number;
}

/**
 * Dosya indiren uçlar için (dekont: GET /admin/deposits/:id/receipt).
 *
 * `apiFetch` her yanıtı JSON bekler ve dosya uçlarında patlar. Ayrı bir
 * fonksiyon YAZILDI ama çağrı yine BU DOSYADAN geçer (değişmez #15):
 * zaman aşımı, çerez politikası ve hata normalizasyonu tek yerde kalsın —
 * bileşenin içinde çıplak bir `fetch` bunların üçünü de kaybederdi.
 *
 * Yalnız okuma uçları içindir: gövde göndermez, durum değiştirmez.
 */
export async function apiBlob(path: string, init: ApiInit = {}): Promise<BlobResult> {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), init.timeoutMs ?? DEFAULT_TIMEOUT_MS);
  const onOuterAbort = () => controller.abort();
  init.signal?.addEventListener('abort', onOuterAbort, { once: true });

  try {
    const res = await fetch(`/api/v1${path}`, {
      ...init,
      method: 'GET',
      body: undefined,
      signal: controller.signal,
      credentials: 'same-origin',
      headers: { ...init.headers },
      // Dekont `Cache-Control: no-store` ile gelir; istemcide de saklamayız.
      cache: 'no-store',
    });

    const retryAfterMs = parseRetryAfter(res.headers.get('retry-after'));
    const ct = res.headers.get('content-type') ?? '';

    if (!res.ok) {
      // Hata gövdesi JSON'dur; dosya değil. Aynı ApiError'a normalize edilir.
      if (ct.includes('application/json')) {
        throw ApiError.fromBody(await res.json(), res.status, retryAfterMs);
      }
      throw new ApiError({
        code: 'GATEWAY_ERROR',
        message: 'Dosya alınamadı. Lütfen tekrar deneyin.',
        status: res.status,
        retryAfterMs,
      });
    }

    const blob = await res.blob();
    // Tip kararı SUNUCUNUNDUR (nosniff). `blob.type` bazı tarayıcılarda boş
    // gelir; başlıktan okurken `; charset=` kuyruğu atılır.
    const mime = (blob.type || ct.split(';')[0] || '').trim();
    return { url: URL.createObjectURL(blob), mime, size: blob.size };
  } catch (err) {
    if (err instanceof ApiError) throw err;
    if (isAbortError(err)) {
      return Promise.reject(new ApiError({ code: 'TIMEOUT', message: 'İstek zaman aşımına uğradı.' }));
    }
    return Promise.reject(new ApiError({
      code: 'NETWORK', message: 'Bağlantı kurulamadı. İnternetinizi kontrol edin.',
    }));
  } finally {
    clearTimeout(timer);
    init.signal?.removeEventListener('abort', onOuterAbort);
  }
}
