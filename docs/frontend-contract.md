# frontend-contract.md — Responsive ve Tarayıcı Uyumluluk Sözleşmesi

> **Doküman amacı:** "Mobilde kusursuz çalışsın, tarayıcı farklılıklarına takılmasın" gereksiniminin
> uygulanabilir kurallara çevrilmiş hali. Bu doküman **bağlayıcıdır** — buradaki her kural bir
> gerçek hata sınıfını önlemek için vardır.
>
> **Durum:** v1 · **Son güncelleme:** 2026-09-08 · **Önkoşul:** [design.md](design.md) §14

---

## 0. Neden ayrı bir doküman

Bu uygulamanın en kırılgan noktası **mobil tarayıcıda uzun süre açık kalan bir SSE bağlantısı**dır.
Kullanıcı numarayı alır, SMS'i bekler, bu sırada telefonunda başka bir uygulamaya geçer, geri döner.
Bu senaryoda iOS Safari bağlantıyı sessizce öldürür, Android Chrome sekmeyi dondurur, operatör vekil
sunucusu tamponlar. Hiçbiri hata üretmez — kullanıcı sadece kodun gelmediğini görür ve parasının
yandığını sanır.

Buradaki kurallar bu ve benzeri sessiz başarısızlıkları önlemek içindir.

---

## 1. Desteklenen tarayıcılar

| Tarayıcı | Minimum | Neden bu sınır |
|---|---|---|
| Safari / iOS Safari | **16.4+** | `dvh` birimleri, `Array.at`, modern `:has()` |
| Chrome / Edge (masaüstü + Android) | Son 2 sürüm | Sürekli güncellenir |
| Firefox | Son 2 sürüm | |
| Samsung Internet | 23+ | Türkiye'de Android'de ciddi pay |

**iOS'ta tüm tarayıcılar WebKit'tir.** iOS Chrome = iOS Safari motoru. "Chrome'da çalışıyor" demek
iOS'ta çalışıyor demek **değildir**. Test matrisinde iOS ayrı satırdır.

`browserslist` (package.json):
```json
["> 0.5% in TR", "last 2 versions", "iOS >= 16.4", "not dead"]
```

> **Kural:** Bu listenin dışındaki bir tarayıcı için özel kod yazılmaz; bunun yerine
> **zarif bozulma** (graceful degradation) sağlanır — SSE yoksa yoklama, `dvh` yoksa `vh`.

---

## 2. Mobil responsive kuralları

### 2.1 Mobile-first, istisnasız

Tailwind sınıfları **önce mobil** yazılır, büyük ekran `md:`/`lg:` ile eklenir.

```tsx
// DOĞRU — taban mobil, büyük ekran ekleme
<div className="flex flex-col gap-3 p-4 md:flex-row md:gap-6 md:p-8">

// YANLIŞ — masaüstü taban, mobilde geri alma
<div className="flex flex-row gap-6 p-8 max-md:flex-col max-md:p-4">
```

**Gerekçe:** `max-*` ile geri alma zincirleri birikir ve mobilde okunamaz CSS üretir. Mevcut Duralux
teması tam olarak bu hataya sahip (masaüstü tema + mobil düzeltmeler).

### 2.2 Kırılma noktaları

Tailwind varsayılanları kullanılır, özelleştirilmez:

| Ad | Genişlik | Hedef |
|---|---|---|
| *(taban)* | 0–639 | Telefon (dikey) — **birincil hedef** |
| `sm:` | 640+ | Telefon (yatay), küçük tablet |
| `md:` | 768+ | Tablet |
| `lg:` | 1024+ | Dizüstü |
| `xl:` | 1280+ | Masaüstü |

**Test genişlikleri (zorunlu):** `320` (iPhone SE) · `390` (iPhone 14/15) · `430` (iPhone Pro Max) ·
`768` (iPad) · `1440` (masaüstü).

> **320 px zorunludur.** Yatay kaydırma çubuğu çıkarsa hata sayılır.

### 2.3 Dokunma hedefleri

- Tıklanabilir her öğe **en az 44×44 CSS px** (Apple HIG) — görsel olarak küçükse `::before` ile
  dokunma alanı genişletilir
- Dokunma hedefleri arasında en az **8 px** boşluk
- **İstisna — paragraf içi bağlantılar:** akan metnin içindeki bir bağlantı (`<p>` içinde
  cümlenin parçası) 44 px kuralından muaftır. Satır yüksekliğini 44 px'e çıkarmak paragrafı
  okunamaz hale getirir ve bağlantı zaten tek başına duran bir denetim değildir. Kural,
  **kendi başına duran** denetimler için geçerlidir: butonlar, gezinme bağlantıları,
  liste öğeleri, ikon butonları.
- `hover:` durumlarına **asla** güvenilmez; dokunmatikte hover yoktur. Bilgi yalnız hover'da
  gösterilmez (örn. sipariş satırındaki "kopyala" butonu her zaman görünür olmalı)
- `active:` durumu görsel geri bildirim vermeli (dokunma anında)

```tsx
// Küçük ikon butonu — dokunma alanı genişletilmiş
<button className="relative p-2 before:absolute before:-inset-2 before:content-['']">
  <CopyIcon className="size-5" />
</button>
```

### 2.4 iOS Safari tuzakları

| Tuzak | Belirti | Çözüm |
|---|---|---|
| **`100vh` adres çubuğunu saymaz** | Sayfa alt kısmı adres çubuğunun altında kalır, içerik kırpılır | `100dvh` kullan; `@supports not (height: 100dvh)` içinde `100vh` yedeği |
| **Girdi odaklanınca sayfa yakınlaşır (zoom)** | Kullanıcı bir alana dokununca ekran zıplar | Tüm `input`/`select`/`textarea` için **`font-size: 16px` (`text-base`) minimum**. `maximum-scale=1` ile çözme — erişilebilirliği bozar |
| **Çentik / ev çubuğu alanı** | Sabit alt bar ev çubuğunun altında kalır | `padding-bottom: env(safe-area-inset-bottom)`; `viewport-fit=cover` meta |
| **`position: fixed` + klavye** | Klavye açılınca sabit öğeler kayar | Sabit alt bar yerine normal akış; gerekiyorsa `VisualViewport` API ile konumla |
| **Momentum kaydırma kilitleniyor** | Modal içinde kaydırma sayfayı kaydırıyor | Modal açıkken `body` üzerinde kaydırma kilidi (`overflow: hidden` + `position: fixed` + kaydırma konumu geri yükleme) |
| **`:hover` yapışıyor** | Dokunma sonrası buton hover'da kalıyor | `@media (hover: hover)` içinde hover stilleri |
| **Tarih ayrıştırma katı** | `new Date("2026-09-08 10:00")` → `Invalid Date` | **Yalnız RFC 3339:** `2026-09-08T10:00:00Z` (bkz. §5.2) |

**Viewport meta (Next.js `layout.tsx`):**
```ts
export const viewport: Viewport = {
  width: 'device-width',
  initialScale: 1,
  viewportFit: 'cover',       // çentik alanı için
  // maximumScale VE userScalable AYARLANMAZ — erişilebilirlik gereği yakınlaştırma serbest kalmalı
};
```

**Güvenli alan yardımcıları (globals.css):**
```css
@supports (padding: env(safe-area-inset-bottom)) {
  .pb-safe { padding-bottom: max(1rem, env(safe-area-inset-bottom)); }
  .pt-safe { padding-top:  max(1rem, env(safe-area-inset-top)); }
}
/* dvh yedeği */
.h-screen-safe { height: 100vh; }
@supports (height: 100dvh) { .h-screen-safe { height: 100dvh; } }
```

### 2.5 Yerleşim desenleri

| Bileşen | Mobil (< 768) | Masaüstü |
|---|---|---|
| **Kenar çubuğu** | Gizli; hamburger → tam ekran çekmece (Radix Dialog, odak tuzağı ile) | Sabit sol sütun |
| **Tablolar** | **Kart listesi** — her satır bir kart, etiket+değer çiftleri | Gerçek tablo |
| **Sipariş geçmişi** | Kart + "detay" açılır | Tablo + sıralama |
| **Servis ızgarası** | 2 sütun | 4–6 sütun |
| **Ülke seçimi** | Tam ekran arama sayfası | Açılır liste + arama |
| **Modal** | Alttan yükselen sayfa (bottom sheet), tam genişlik | Ortada diyalog |
| **Form** | Tek sütun, tam genişlik alanlar | İki sütun olabilir |
| **Ana eylem butonu** | Tam genişlik, `pb-safe` ile alta sabit | Normal genişlik |

> **Tablo kuralı:** Yatay kaydırılan tablo mobilde **kabul edilmez**. `overflow-x: auto` bir çözüm
> değil, bir erteleme. Tablolar `< md` altında karta dönüşür.

```tsx
// Desen: aynı veri, iki sunum
<>
  <div className="md:hidden">      {orders.map(o => <OrderCard key={o.id} order={o} />)} </div>
  <div className="hidden md:block"><OrderTable orders={orders} /></div>
</>
```

### 2.6 Kod bekleme ekranı — mobilde kritik

Bu ekran kullanıcının en gergin olduğu andır. Mobilde özel kurallar:

- Kod **çok büyük** gösterilir (`text-4xl`+), harf aralığı geniş, tek dokunuşla kopyalanır
- Kopyalama: `navigator.clipboard.writeText` → başarısızsa `document.execCommand('copy')` yedeği
  (iOS'ta güvenli olmayan bağlamda Clipboard API yok)
- Geri sayım `requestAnimationFrame` ile değil, **sunucudan gelen `expiresAt` ile** hesaplanır
  (sekme dondurulunca sayaç durur, `expiresAt` durmaz)
- Sayfa geri geldiğinde (`visibilitychange`) durum **hemen** yeniden sorgulanır
- Ekran uyanık tutulur: `navigator.wakeLock.request('screen')` — desteklenmiyorsa sessizce geçilir
- Titreşim ile bildirim: `navigator.vibrate?.(200)` — iOS'ta yok, kontrol edilerek çağrılır
- Numara ve kod **seçilebilir** olmalı (`select-text`); mobilde uzun basıp kopyalama yaygın

### 2.7 Görsel ve font

- `next/image` zorunlu; `sizes` özniteliği doğru verilir (mobilde tam boyut indirmek veri israfı)
- Fontlar `next/font` ile self-host; `display: swap`; Türkçe karakter alt kümesi (`latin-ext`)
- Kritik CSS satır içi (Next.js otomatik)
- Koyu tema varsayılan (Duralux paleti); `prefers-color-scheme` saygı görür ama tema seçimi kalıcı

---

## 3. API iletişim sözleşmesi

> **Tek kural:** Uygulamada **tek bir** HTTP istemci sarmalayıcısı vardır (`web/src/lib/api/client.ts`).
> Bileşenler `fetch`'i doğrudan çağırmaz. Bu, tarayıcı farklılıklarının tek bir yerde ele alınmasını sağlar.

### 3.1 İstemci sarmalayıcısı — zorunlu davranışlar

```ts
// web/src/lib/api/client.ts  (sözleşme — tam uygulama koda yazılacak)

const DEFAULT_TIMEOUT_MS = 15_000;

export async function apiFetch<T>(path: string, init: ApiInit = {}): Promise<T> {
  // 1) ZAMAN AŞIMI — fetch'in yerleşik zaman aşımı YOKTUR.
  //    AbortSignal.timeout() Safari 16'da var ama güvenli olsun diye elle kuruyoruz.
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), init.timeoutMs ?? DEFAULT_TIMEOUT_MS);

  // 2) Çağıranın kendi sinyali varsa birleştir (AbortSignal.any her yerde yok)
  init.signal?.addEventListener('abort', () => controller.abort(), { once: true });

  try {
    const res = await fetch(`/api/v1${path}`, {
      ...init,
      signal: controller.signal,
      // 3) ÇEREZ: aynı alan adı olsa bile AÇIKÇA belirt.
      //    Varsayılan 'same-origin'dir ama açık olmak Safari ITP davranışında belirsizliği kaldırır.
      credentials: 'same-origin',
      headers: {
        'Accept': 'application/json',
        // 4) Content-Type YALNIZ gövde varsa. Boş POST'ta göndermek bazı vekilleri şaşırtır.
        ...(init.body ? { 'Content-Type': 'application/json' } : {}),
        // 5) CSRF: durum değiştiren isteklerde çift gönderim token'ı
        ...(isMutating(init.method) ? { 'X-CSRF-Token': readCsrfCookie() } : {}),
        // 6) İdempotency: para harcayan isteklerde
        ...(init.idempotencyKey ? { 'Idempotency-Key': init.idempotencyKey } : {}),
        ...init.headers,
      },
      // 7) ÖNBELLEK: Safari GET yanıtlarını agresif önbellekler.
      //    Mutasyonlarda ve taze veri gerektiren GET'lerde 'no-store'.
      cache: init.cache ?? (isMutating(init.method) ? 'no-store' : 'default'),
    });

    // 8) 204 / boş gövde — res.json() burada patlar
    if (res.status === 204) return undefined as T;

    // 9) YANIT HER ZAMAN JSON DEĞİLDİR.
    //    Vekil hatası, 502 HTML sayfası, Cloudflare ara sayfası → res.json() SyntaxError atar.
    const contentType = res.headers.get('content-type') ?? '';
    if (!contentType.includes('application/json')) {
      throw new ApiError({
        code: res.ok ? 'INVALID_RESPONSE' : 'GATEWAY_ERROR',
        message: 'Sunucuya ulaşılamadı. Lütfen tekrar deneyin.',
        status: res.status,
      });
    }

    const body = await res.json();
    if (!res.ok) throw ApiError.fromBody(body, res.status);   // { error: { code, message, requestId } }
    return body as T;

  } catch (err) {
    // 10) HATA NORMALİZASYONU — çağıran tek bir hata tipi görür
    if (err instanceof ApiError) throw err;
    if (isAbortError(err)) throw new ApiError({ code: 'TIMEOUT', message: 'İstek zaman aşımına uğradı.' });
    // TypeError: ağ hatası, DNS, CORS, çevrimdışı — tarayıcılar farklı mesaj verir, ayrım yapma
    throw new ApiError({ code: 'NETWORK', message: 'Bağlantı kurulamadı. İnternetinizi kontrol edin.' });
  } finally {
    clearTimeout(timer);
  }
}
```

**Neden her madde var:**

| # | Önlenen hata |
|---|---|
| 1 | `fetch` sonsuza kadar bekler; mobil ağda kopan bağlantıda arayüz sonsuza kadar yükleniyor kalır |
| 2 | `AbortSignal.any()` Safari 17.4'ten önce yok — elle birleştirme gerekir |
| 3 | Çerez gönderimini açık yapmak, Safari'nin ITP davranışındaki belirsizliği kaldırır |
| 4 | Gövdesiz `POST`'ta `Content-Type: application/json` bazı vekilleri ve WAF'ları tetikler |
| 5 | CSRF koruması (`SameSite=Lax` tek başına yeterli değil) |
| 6 | Ağ tekrar denemesinde çift harcamayı önler |
| 7 | Safari `GET` yanıtlarını beklenenden agresif önbellekler; bakiye eski görünür |
| 8 | `res.json()` boş gövdede `SyntaxError` atar |
| 9 | **En sık görülen üretim hatası:** 502/504'te vekil HTML döner, `res.json()` patlar, kullanıcı anlamsız hata görür |
| 10 | `TypeError: Failed to fetch` (Chrome) vs `TypeError: Load failed` (Safari) — mesaja göre ayrım yapılmaz |

### 3.2 Yeniden deneme politikası

| İstek tipi | Yeniden deneme |
|---|---|
| `GET` (okuma) | ✅ 3 kez, üstel geri çekilme (300ms → 900ms → 2.7s) + jitter |
| `POST /orders` | ❌ **ASLA** — idempotent değil, iki numara alınır |
| `POST /wallet/deposits` | ⚠️ Yalnız `Idempotency-Key` ile |
| `POST /auth/login` | ❌ Hesap kilidi tetiklenir |
| `429` yanıtı | ✅ `Retry-After` başlığına uy |
| `5xx` | ✅ Yalnız `GET` için |
| Ağ hatası (`TypeError`) | ✅ Yalnız `GET` için |

TanStack Query yapılandırması:
```ts
new QueryClient({
  defaultOptions: {
    queries: {
      retry: (count, err) => count < 3 && isRetryable(err),
      retryDelay: (i) => Math.min(300 * 3 ** i, 5000) + Math.random() * 200,
      staleTime: 30_000,
      refetchOnWindowFocus: true,   // mobilde uygulamaya dönünce taze veri
      refetchOnReconnect: true,
    },
    mutations: { retry: false },     // MUTASYONLAR ASLA OTOMATİK TEKRARLANMAZ
  },
});
```

### 3.3 Çevrimdışı ve ağ durumu

- `navigator.onLine` **güvenilmezdir** (bağlı ama internetsiz Wi-Fi'de `true` döner).
  Yalnız *ipucu* olarak kullanılır; gerçek karar başarısız isteğe göre verilir.
- `online`/`offline` olaylarında TanStack Query otomatik yeniden çeker
- Çevrimdışıyken kullanıcıya kalıcı bir bant gösterilir; **satın alma butonu devre dışı bırakılmaz**
  (yanlış pozitif ihtimali var) ama uyarı gösterilir

---

## 4. SSE sertleştirme — en kritik bölüm

`EventSource` basit görünür ama mobilde en çok sorun çıkaran parçadır. Aşağıdaki kuralların
**tamamı** uygulanmadan kod bekleme ekranı üretime alınmaz.

### 4.1 Bilinen sorunlar ve karşılıkları

| Sorun | Etki | Karşılık |
|---|---|---|
| **HTTP/1.1'de origin başına 6 bağlantı sınırı** | Açık SSE bağlantısı bir slot tutar. 6 sekme açıksa uygulama **tamamen kilitlenir** | **HTTP/2 zorunlu** (Caddy TLS ile varsayılan). Yerel geliştirmede HTTP/1.1 olduğu için bu tuzak fark edilmez → **`deploy/` altında yerel HTTPS ile test edilir** |
| **Ters vekil yanıtı tamponlar** | Kod gelir ama ekranda görünmez | Caddy: `flush_interval -1`. Go: `X-Accel-Buffering: no` başlığı + her yazımdan sonra `Flusher.Flush()` |
| **iOS Safari arka planda bağlantıyı öldürür** | Kullanıcı uygulamaya dönünce akış ölü, hata **yok** | `visibilitychange` → görünür olunca **durumu REST ile sorgula** + akışı yeniden kur |
| **bfcache'ten geri dönüş** | Sayfa geri tuşuyla gelir, `EventSource` ölüdür | `pageshow` olayında `event.persisted` ise yeniden bağlan |
| **Ağ değişimi (Wi-Fi → mobil veri)** | Bağlantı sessizce kopar | Keepalive alınmıyorsa (25 sn) bağlantıyı **kendin** kapat ve yeniden kur |
| **`EventSource` özel başlık desteklemez** | Token gönderilemez | **Çerez tabanlı kimlik** — mimarimiz zaten böyle (ADR-003). Tek alan adı olduğu için sorunsuz |
| **Otomatik yeniden bağlanma sonsuz döngüye girer** | Sunucu 500 dönerse tarayıcı durmadan dener | `onerror`'da **kendi** geri çekilme mantığını uygula, `es.close()` çağır |
| **Bazı kurumsal/operatör vekilleri SSE'yi engeller** | Hiç olay gelmez | **Yoklama yedeği zorunlu** — 10 sn içinde ilk olay gelmezse yoklamaya düş |

### 4.2 `useOrderStream` sözleşmesi

```ts
// web/src/lib/hooks/useOrderStream.ts  (sözleşme)
//
// Sorumluluklar — HEPSİ ZORUNLU:
//  1. EventSource kur; 'code' | 'status' | 'cancelled' olaylarını dinle
//  2. KEEPALIVE İZLEME: 25 sn'dir olay/yorum gelmediyse bağlantı ölü sayılır -> kapat + yeniden kur
//  3. GERİ ÇEKİLME: yeniden bağlanma 1s -> 2s -> 4s -> 8s (üst sınır 15s) + jitter
//  4. YOKLAMA YEDEĞİ: 10 sn içinde bağlantı kurulamazsa veya EventSource yoksa
//     -> 5 sn aralıklı GET /orders/:id ile devam et. Kullanıcı farkı HİSSETMEZ.
//  5. GÖRÜNÜRLÜK: document.visibilityState === 'visible' olduğunda
//     -> önce REST ile durumu senkronla, sonra akışı yeniden kur
//  6. BFCACHE: window 'pageshow' (event.persisted) -> yeniden kur
//  7. AĞ: 'online' olayında yeniden kur
//  8. TEMİZLİK: unmount'ta es.close() + tüm zamanlayıcıları temizle (bellek sızıntısı yok)
//  9. TERMİNAL DURUM: COMPLETED | REFUNDED | CANCELLED gelince bağlantıyı KAPAT, yeniden kurma
// 10. Son bilinen durumu döndür; 'connecting' | 'live' | 'polling' | 'closed' göstergesi ver

export function useOrderStream(orderPublicId: string): {
  status: OrderStatus;
  code: string | null;
  transport: 'sse' | 'polling';    // arayüzde küçük bir gösterge için
  secondsLeft: number;             // expiresAt'ten türetilir, yerel sayaçtan DEĞİL
}
```

> **Kritik kural:** Kullanıcıya gösterilen kalan süre **her zaman** sunucudan gelen `expiresAt` ile
> hesaplanır (`expiresAt - Date.now()`). Yerel bir sayaç kullanılmaz — sekme donduğunda yerel sayaç
> durur, gerçek süre durmaz. Kullanıcı "2 dakikam vardı" derken süre çoktan dolmuş olur.

### 4.3 Sunucu tarafı SSE gereksinimleri (Go)

```go
// Zorunlu başlıklar
w.Header().Set("Content-Type", "text/event-stream")
w.Header().Set("Cache-Control", "no-cache, no-transform")   // no-transform: vekil sıkıştırmasını engelle
w.Header().Set("Connection", "keep-alive")
w.Header().Set("X-Accel-Buffering", "no")                   // nginx/Caddy tamponlamasını kapat

flusher, ok := w.(http.Flusher)
if !ok { /* SSE desteklenmiyor -> 501, istemci yoklamaya düşer */ }

// Her yazımdan sonra ZORUNLU
fmt.Fprintf(w, "event: code\ndata: %s\n\n", payload)
flusher.Flush()

// 20 sn'de bir keepalive (istemci 25 sn'de ölü sayıyor -> güvenlik payı var)
// : ile başlayan satır SSE yorumudur, istemci yok sayar ama bağlantı canlı kalır
fmt.Fprint(w, ": keepalive\n\n")
flusher.Flush()

// context iptalinde (istemci gitti) temizlik yap — goroutine sızdırma
<-r.Context().Done()
```

**Caddy yapılandırması:**
```
handle /api/v1/*/stream {
    reverse_proxy api:8080 {
        flush_interval -1        # ← SSE için ZORUNLU: tamponlama yok, anında ilet
    }
}
```

### 4.4 SSE kabul testi (üretim öncesi zorunlu)

- [ ] iOS Safari'de numara al → uygulamayı arka plana at → **2 dakika bekle** → geri dön → kod görünüyor
- [ ] Android Chrome'da aynı senaryo
- [ ] Wi-Fi'den mobil veriye geçiş sırasında akış kendini toparlıyor
- [ ] 7 sekme açıkken uygulama kilitlenmiyor (HTTP/2 doğrulaması)
- [ ] SSE ağ katmanında engellendiğinde (DevTools ile blokla) yoklamaya düşüyor ve kod yine geliyor
- [ ] Geri tuşuyla sayfaya dönüldüğünde (bfcache) akış yeniden kuruluyor
- [ ] Sipariş tamamlandığında bağlantı kapanıyor (Network sekmesinde asılı bağlantı yok)

---

## 5. Veri biçimlendirme — tarayıcı tuzakları

### 5.1 Para

```ts
// web/src/lib/format/money.ts — TEK YER

// API her zaman { minor: 1250, currency: 'TRY', formatted: '12,50 ₺' } döner.
// 'formatted' sunucudan gelir ve TERCİH EDİLİR — böylece sunucu ile istemci asla ayrışmaz.
// İstemci tarafı biçimleme yalnız yerel hesaplamalar için:

export function formatMoney(m: Money): string {
  try {
    return new Intl.NumberFormat('tr-TR', {
      style: 'currency', currency: m.currency,
      minimumFractionDigits: 2, maximumFractionDigits: 2,
    }).format(m.minor / 100);
  } catch {
    // Intl her yerde var ama TRY biçimi tarayıcıya göre '₺12,50' / '12,50 ₺' değişebilir.
    // Yedek: elle biçimle, tutarlılık > yerellik
    return `${(m.minor / 100).toFixed(2).replace('.', ',')} ₺`;
  }
}
```

> **Kural:** Para aritmetiği **asla** istemcide yapılmaz. Toplam, indirim, iade — hepsi sunucuda.
> İstemci yalnız gösterir. `m.minor / 100` sadece görüntüleme içindir.

### 5.2 Tarih

```ts
// API her zaman RFC 3339 UTC döner: "2026-09-08T10:15:30Z"
// Safari "2026-09-08 10:15:30" (boşluklu) formatını AYRIŞTIRAMAZ -> Invalid Date

export function parseApiDate(s: string): Date {
  const d = new Date(s);                       // RFC 3339 her tarayıcıda güvenli
  if (Number.isNaN(d.getTime())) throw new Error(`Geçersiz tarih: ${s}`);
  return d;
}

export function formatDateTime(d: Date): string {
  return new Intl.DateTimeFormat('tr-TR', {
    dateStyle: 'medium', timeStyle: 'short', timeZone: 'Europe/Istanbul',
  }).format(d);
}
```

> **Kural:** `timeZone` **her zaman açıkça** verilir. Kullanıcının cihaz saat dilimine güvenilmez —
> yurt dışındaki bir kullanıcı sipariş saatini yanlış görür ve destek talebi açar.

### 5.3 Sayı hassasiyeti

Para `int64` kuruş olarak gelir. JavaScript `Number.MAX_SAFE_INTEGER` = 9.007.199.254.740.991 →
**90 trilyon TL**'ye kadar güvenli. `BigInt` gerekmez. Ancak:

> **Kural:** Sunucu tarafında `int64` sınırına yaklaşan bir değer üretilemez; bakiye üst sınırı
> (`50.000,00 ₺` yükleme limiti, `trd.md` §10) bunu zaten garantiler.

---

## 6. Erişilebilirlik (mobilde de geçerli)

- Klavye ile tam gezinme; odak göstergesi görünür (`focus-visible:ring-2`)
- Modal'da odak tuzağı + `Esc` ile kapatma (Radix hazır sağlar)
- SSE ile gelen kod `aria-live="assertive"` bölgede duyurulur — ekran okuyucu kullanıcısı kodun
  geldiğini **duymalı**
- Geri sayım `aria-live="off"` (her saniye duyurulmamalı), yalnız son 30 sn'de `polite`
- Renk kontrastı ≥ 4.5:1 (koyu temada özellikle gri metinlere dikkat)
- `prefers-reduced-motion` saygı görür: animasyonlar kapatılır
- Form hataları alanla ilişkilendirilir (`aria-describedby`), yalnız renkle gösterilmez

---

## 7. Test matrisi

### 7.1 Otomatik (CI'da zorunlu)

```
Playwright projeleri:
  ├─ chromium-desktop   1440×900
  ├─ webkit-desktop     1440×900        ← Safari motoru
  ├─ chromium-mobile    Pixel 7  (393×852,  touch, mobil UA)
  └─ webkit-mobile      iPhone 14 (390×844, touch, mobil UA)   ← EN KRİTİK
```

Her uçtan uca akış **dört projede de** çalışır:
`kayıt → giriş → bakiye görüntüle → fiyat sorgula → satın al → kod bekle → iptal et`

Ek otomatik kontroller:
- **320 px'te yatay taşma yok** — her sayfa için `document.documentElement.scrollWidth <= clientWidth`
- **Dokunma hedefi boyutu** — tüm etkileşimli öğeler ≥ 44×44
- **Erişilebilirlik** — `@axe-core/playwright`, kritik ihlal yok
- **Görsel gerileme** — anahtar ekranların ekran görüntüsü karşılaştırması

### 7.2 Manuel (her sürüm öncesi)

| Cihaz | Neden |
|---|---|
| **Gerçek iPhone (iOS 16.4+)** | Simülatör klavye, güvenli alan ve arka plan davranışını **doğru taklit etmez** |
| **Gerçek Android (Chrome + Samsung Internet)** | Samsung Internet'in kendi tuhaflıkları var |
| Yavaş 3G kısıtlaması | Zaman aşımı ve iskelet ekranlar |
| Uçak modu aç/kapa | Çevrimdışı davranışı ve yeniden bağlanma |

> **Simülatör yeterli değildir.** SSE'nin arka plan davranışı yalnız gerçek cihazda doğrulanabilir.

---

## 8. Performans bütçesi (mobil, 4G)

| Ölçüt | Hedef |
|---|---|
| LCP (Largest Contentful Paint) | < 2,5 sn |
| INP (Interaction to Next Paint) | < 200 ms |
| CLS (Cumulative Layout Shift) | < 0,1 |
| İlk JS paketi (gzip) | < 150 KB |
| Panel sayfası TTFB | < 400 ms |

- `next/dynamic` ile ağır bileşenler (tablolar, grafikler) tembel yüklenir
- Görseller `next/image` + doğru `sizes`
- Font alt kümesi: `latin` + `latin-ext` (Türkçe karakterler)
- CI'da Lighthouse bütçe kontrolü; bütçe aşılırsa derleme uyarı verir

---

## 9. Kontrol listesi — her ekran için

Bir ekran "bitti" sayılmadan önce:

- [ ] 320 px'te yatay kaydırma yok
- [ ] Tüm dokunma hedefleri ≥ 44×44 px
- [ ] Girdi alanları ≥ 16px font (iOS yakınlaşma yok)
- [ ] Tablolar `< md` altında karta dönüşüyor
- [ ] Alt sabit öğelerde `pb-safe` var
- [ ] Yükleniyor / boş / hata durumlarının üçü de tasarlanmış
- [ ] Hata durumunda `requestId` gösteriliyor (destek için)
- [ ] Klavye ile gezinilebiliyor, odak görünür
- [ ] Metinler `messages/tr.json`'dan geliyor
- [ ] `webkit-mobile` Playwright projesinde geçiyor
- [ ] Gerçek bir telefonda **elle** açılıp denendi

---

## 10. SEO

> **"%100 SEO uyumlu" ölçülebilir bir hedef değildir** — arama motoru sıralaması
> rakiplere, alan adı otoritesine ve içeriğe bağlıdır ve hiçbiri kod tarafından
> garanti edilemez. Bu bölüm **teknik SEO**'yu ölçülebilir kriterlere çevirir:
> aşağıdaki maddeler bizim kontrolümüzdedir ve testle doğrulanır.

### 10.1 En kritik kural: panel indekslenmez

| Alan | Dizin | Neden |
|---|---|---|
| `(public)` — ana sayfa, fiyatlar, servis/ülke sayfaları, blog, SSS, yasal metinler | ✅ **İndekslenir** | Trafik buradan gelir |
| `(auth)` — giriş, kayıt, şifre sıfırlama | ⚠️ `noindex` | Arama sonucunda görünmesi değersiz; kayıt sayfası ana sayfadan yönlendirilir |
| `(panel)` ve `(admin)` — oturum arkası her şey | 🔴 **`noindex, nofollow` + `robots.txt` engeli** | Kullanıcı verisi ve sipariş ekranları asla indekslenmemeli |

```ts
// web/src/app/(panel)/layout.tsx
export const metadata: Metadata = { robots: { index: false, follow: false } }
```

> Bu ayrım kod incelemesinde kontrol edilir: `(panel)` veya `(admin)` altında
> `index: true` olan bir sayfa **birleştirilmez**.

### 10.2 Zorunlu teknik gereksinimler

| # | Gereksinim | Doğrulama |
|---|---|---|
| S1 | Her genel sayfada benzersiz `<title>` (≤60 karakter) ve `description` (≤160) | Otomatik test: iki sayfa aynı başlığı taşıyamaz |
| S2 | Tek `<h1>`, hiyerarşik başlık düzeni (h1→h2→h3, atlama yok) | `@axe-core` + özel test |
| S3 | `canonical` URL her sayfada | Otomatik test |
| S4 | `sitemap.xml` dinamik üretilir (Next.js `sitemap.ts`), yalnız genel sayfaları içerir | Test: panel yolları sitemap'te YOK |
| S5 | `robots.txt` (`robots.ts`): `/panel`, `/admin`, `/api` engelli; sitemap bildirilir | Test |
| S6 | Open Graph + Twitter Card etiketleri, `og:image` 1200×630 | Test |
| S7 | JSON-LD yapısal veri: `Organization`, `WebSite`, `BreadcrumbList`, fiyat sayfalarında `Product`+`Offer`, SSS'de `FAQPage` | Google Rich Results Test |
| S8 | `lang="tr"` kök öznitelik; i18n açıldığında `hreflang` + `x-default` | Test |
| S9 | Genel sayfalar **SSG veya ISR** ile render edilir — istemci tarafı render edilen içerik indekslenmez | Test: JS kapalıyken içerik görünür |
| S10 | Tüm görsellerde anlamlı `alt`; dekoratif olanlarda `alt=""` | `@axe-core` |
| S11 | Anlamsal HTML: `<nav>`, `<main>`, `<article>`, `<footer>`; `<div>` yığını değil | Kod incelemesi |
| S12 | Kırık iç bağlantı yok; yönlendirmeler 301 | Derleme sonrası tarama |
| S13 | HTTPS zorunlu, `www` ↔ kök tek yöne 301 | Caddy yapılandırması |
| S14 | Core Web Vitals: LCP < 2,5 sn · INP < 200 ms · CLS < 0,1 (mobil, 4G) | Lighthouse CI (§8) |
| S15 | **Lighthouse SEO puanı = 100** (mobil ve masaüstü) | CI'da eşik |

### 10.3 Programatik sayfalar — fırsat ve tuzak

Katalogumuzda 195 ülke × 811 servis var. Bunlardan `/sanal-numara/whatsapp/turkiye`
gibi sayfalar üretmek bu işte **en yüksek getirili SEO hamlesidir**: arama niyeti
tam olarak böyle ifade ediliyor.

> ⚠️ **Ama tuzağı da var.** 158.000 sayfayı şablonla üretip yalnız servis ve ülke
> adını değiştirmek Google'ın **"doorway pages"** ve **"thin content"** politikalarına
> girer; sonuç sıralama değil cezadır.

**Kural:** Bir programatik sayfa ancak şu üçünü sağlarsa üretilir:
1. **Gerçek veri** — o kombinasyonun canlı fiyatı, stok durumu, teslim süresi
2. **Özgün içerik** — o servise özgü en az 150 kelime (nasıl kullanılır, sık sorunlar)
3. **Stok var** — stoksuz kombinasyon için sayfa üretilmez, üretilmişse `noindex`

v1'de yalnız **en çok aranan ~50 kombinasyon** elle içerikle üretilir.
Tam programatik üretim v1.1 konusudur ve bu kurallara bağlıdır.

### 10.4 Kontrol listesi — her genel sayfa için

- [ ] Benzersiz `title` + `description`
- [ ] Tek `h1`, hiyerarşik başlıklar
- [ ] `canonical` var
- [ ] Open Graph + Twitter Card
- [ ] İlgiliyse JSON-LD
- [ ] Sunucu tarafı render (SSG/ISR)
- [ ] Görsellerde `alt`
- [ ] `sitemap.xml`'e eklendi
- [ ] Lighthouse SEO = 100

---

**İlgili:** [design.md](design.md) §14 · [trd.md](trd.md) §8 · [memory.md](memory.md) §5 · [../CLAUDE.md](../CLAUDE.md)
