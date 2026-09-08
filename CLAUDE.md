# CLAUDE.md

Bu dosya, bu depoda çalışan Claude Code oturumları içindir. Kısa, bağlayıcı ve uygulanabilir olmalı.
Tartışma ve gerekçe `docs/` altındadır; burada **kurallar** vardır.

---

## Proje

Sanal numara (SMS doğrulama) satış platformu. Kullanıcı ön ödemeli **TL** bakiyesiyle, birden fazla
üst sağlayıcıyı soyutlayan bir panel üzerinden servis (WhatsApp, Telegram…) ve ülke seçip numara alır,
gelen SMS kodunu görür. Kod gelmezse otomatik iade edilir.

**Bu bir para sistemidir.** Buradaki bir hata gerçek para kaybettirir. Şüphede kaldığında dur ve sor.

### Dokümanlar — değişiklik yapmadan önce oku
| Dosya | Ne zaman |
|---|---|
| [docs/intent.md](docs/intent.md) | "Bunu yapmalı mıyız?" sorusunda |
| [docs/design.md](docs/design.md) | Mimari bir karar verirken |
| [docs/trd.md](docs/trd.md) | Bir özellik yazarken (FR/NFR + kabul kriterleri) |
| [docs/frontend-contract.md](docs/frontend-contract.md) | **Ön yüz kodu yazmadan önce — zorunlu.** Responsive, tarayıcı uyumluluğu, API istemcisi, SSE |
| [docs/provider-herosms.md](docs/provider-herosms.md) | **Sağlayıcı adaptörüne dokunmadan önce — zorunlu.** Doğrulanmış uç noktalar, hata kodları, webhook, iptal kuralları |
| [docs/roadmap.md](docs/roadmap.md) | Sıradaki iş ne? |
| [docs/memory.md](docs/memory.md) | **Her oturum başında** — tuzaklar ve kararlar burada |

---

## Yığın

**Backend:** Go 1.23+ · Gin · **sqlc + pgx** · PostgreSQL 16 · Redis 7 · asynq · goose
**Frontend:** Next.js 15 (App Router) · React · TypeScript · Tailwind CSS 4 · shadcn/ui · TanStack Query/Table · next-intl
**Yapı:** Monorepo — `api/` (Go) · `web/` (Next.js) · `docs/` · `deploy/`
**Topoloji:** Tek alan adı, Caddy ters vekil: `/` → Next.js, `/api` → Go

---

## Komutlar

```bash
make dev            # postgres + redis + api + web (yerel geliştirme)
make migrate-up     # goose up
make migrate-new    # yeni migration dosyası
make gen            # OpenAPI'den Go iskeleti + TS tipleri üret
make test           # go test ./... + web testleri
make lint           # golangci-lint + eslint + tsc
make check          # lint + test  (birleştirmeden önce bunu çalıştır)
```

---

## Değişmezler — asla ihlal edilmez

Bunlar tercih değil, **kural**. Bir görev bunlardan birini ihlal etmeyi gerektiriyorsa görev yanlıştır;
devam etme, sor.

1. **Para asla ondalıklı sayı (float) değildir.** `int64` kuruş. `float64` ile para hesabı yapılmaz,
   JSON'da çıplak ondalık para gönderilmez. API'de her zaman `{minor, currency, formatted}`.

2. **Bakiye yalnız `WalletRepo.ApplyEntry` üzerinden değişir.** Başka hiçbir yerde
   `UPDATE users SET balance_minor` yazılmaz. Her çağrı bir `idempotency_key` taşır.

3. **Bakiye okuma+yazma `SELECT ... FOR UPDATE` kilidi altında, tek transaction içinde yapılır.**

4. **`ledger_entries` üzerinde `UPDATE`/`DELETE` yoktur.** Düzeltme ters `ADJUSTMENT` kaydıdır.

5. **Dış çağrı (HTTP) asla veritabanı transaction'ı içinde yapılmaz.** Provizyon/onay deseni kullanılır
   (`docs/design.md` §5.4).

6. **`Purchase` çağrısı yeniden denenmez.** İdempotent değildir; iki numara alınmasına yol açar.

7. **Sahiplik kontrolü sorgunun parçasıdır**, ayrı bir `if` değil.
   `WHERE public_id = $1 AND user_id = $2`. Depo katmanında kullanıcı kapsamı olmayan metot yazılmaz.

8. **Durum değiştiren her işlem `POST`/`PATCH`/`DELETE`'tir.** Asla `GET`.

9. **İstemciden gelen fiyat, sağlayıcı veya maliyet bilgisine güvenilmez.**
   `POST /orders` gövdesi **yalnız** `{ quoteId }` alır.

10. **Dışarıya sayısal `id` verilmez.** Her zaman `public_id` (UUID).

11. **Sır kodda tutulmaz.** Ortam değişkeni; açılışta doğrulanır, eksikse süreç başlamaz.

12. **Kullanıcıya ham hata mesajı gösterilmez.** Tipli hata → Türkçe genel mesaj + `requestId`.

13. **`status` alanına durum makinesi fonksiyonu dışında yazılmaz.**

14. **Kullanıcıya görünen metin sabit yazılmaz.** `web/messages/tr.json` sözlüğünden gelir.

15. **Bileşenler `fetch`'i doğrudan çağırmaz.** Tüm HTTP çağrıları `web/src/lib/api/client.ts`
    sarmalayıcısından geçer — zaman aşımı, çerez, CSRF, JSON olmayan yanıt ve hata normalizasyonu
    orada tek yerde ele alınır.

16. **Mutasyonlar otomatik yeniden denenmez.** `POST /orders` asla tekrarlanmaz — iki numara alınır.

17. **Mobile-first yazılır.** Taban stiller mobil, büyük ekran `md:`/`lg:` ile eklenir.
    `max-*` ile geri alma zinciri yazılmaz. Tablolar `< md` altında karta dönüşür.

18. **Kalan süre her zaman sunucudaki `expiresAt`'ten hesaplanır**, yerel sayaçtan değil
    (sekme donduğunda yerel sayaç durur, gerçek süre durmaz).

19. **Tarih ayrıştırma yalnız RFC 3339.** Safari boşluklu formatı ayrıştıramaz.
    Biçimlemede `timeZone` her zaman açıkça `Europe/Istanbul`.

20. **Stok = `counts.physical` / `physicalCount`.** `count` / `total` alanı **asla** stok olarak
    kullanılmaz — sahte değer döndürür (WhatsApp×TR: `count=56964`, `physical=0`).

21. **Her satın almada `maxPrice` gönderilir.** Teklifteki maliyet üstünde ücretlendirme
    sağlayıcı sınırında engellenir.

22. **Sipariş TTL'i sağlayıcı yanıtındaki `expiredAt`'ten alınır.** Koda gömülmez.

23. **Webhook gövdesine güvenilmez.** Gelen bildirim bir tetikleyicidir; kod
    **`GET /activations/{id}/otp/last`** ile teyit edilerek alınır. Teyit boş dönerse durum
    **değiştirilmez**. Handler **3 saniye içinde `200`** döner — iş kuyrukta yapılır.
    🔴 **`GET /activations/{id}` diye bir uç nokta YOKTUR** (o yolda yalnız `delete`).

24. **`Cancel()` ve `Finish()` aynı şey değildir.** `DELETE` iade talep eder, `finish` etmez.
    Kod geldiyse `Finish()`, gelmediyse `Cancel()`. Her terminal sipariş sağlayıcıda **kapatılır**.

25. **Sağlayıcı yanıtı dizidir.** `POST /activations` `amount: 1` olsa bile dizi döner —
    `data[0]`; `len(data) != 1` → hata.

26. **OTP alan adları normalize edilir.** Üç kaynak, üç isim seti:
    `smsCode|code` → `Code`, `smsText|text` → `Body`, `receivedAt|date` → `ReceivedAt`.

27. **`maxPrice` gönderilir, `fixedPrice` GÖNDERİLMEZ** (`fixedPrice` = sabit fiyat, tavan değil).

29. **Bir yorum garanti veriyorsa, aynı blokta o garantiyi doğrulayan bir test
    referansı olmalıdır.** `// test: dosya_test.go#TestAdi`

    Gerekçe: kod incelemesinde **beş** yerde yorumun kodun sağlamadığı bir söz
    verdiği bulundu — "aynı istek tekrarlanırsa bakiye iki kez değişmez" (her
    tekrar yeni UUID üretiyordu), "Ama log'larız" (log yoktu), "config üretimde
    reddeder" (main config'i okumuyordu), "JavaScript okuyamaz" (aynı değer
    JSON'da dönüyordu). Bu, tekil hatalardan **daha tehlikelidir**: okuyan
    kişi yorumu okuyup kontrolün yapıldığını varsayar ve inceleme orada durur.
    Beş vakanın hepsinde yorum **doğru tasarımı** tarif ediyordu; sadece koda
    bağlanmamıştı.

    `scripts/check-guarantees.py` bunu zorlar ve `make check` ile CI'da koşar.

28. **Legacy sağlayıcı yolunda `StatusCode == 200` başarı demek DEĞİLDİR** — `BAD_KEY`,
    `NO_NUMBERS` vb. 200 gövdesinde düz metin gelir. Legacy çağrı URL'leri **log'a yazılmaz**.

---

## Katman kuralları (Go)

```
transport → service → domain
                ↓
              port  ←  adapter
```

| Katman | Yapar | **Yapmaz** |
|---|---|---|
| `transport/http` | DTO bağla, doğrula, servis çağır, yanıt yaz | İş kuralı, DB erişimi, dış API çağrısı |
| `service` | Kullanım senaryosunu yürüt, **transaction sınırını çiz**, olay yayınla | `http.Request`/`gin.Context` tanımak |
| `domain` | Saf iş kuralı (para, fiyat, durum makinesi) | G/Ç, DB, ağ, zaman (`ClockPort` kullan) |
| `port` | Arayüz tanımla | Uygulama içermek |
| `adapter` | Portları uygula (postgres, redis, sağlayıcı, mail) | İş kuralı içermek |

**Ek kurallar:**
- `service` paketleri birbirinin deposunu (`repo`) çağıramaz; yalnız diğer `service`in dışa açık metodunu.
- `transport` katmanı `adapter` paketlerini import edemez.
- Domain modeli **asla** HTTP yanıtına doğrudan serileştirilmez; `dto` yapıları kullanılır.

---

## sqlc kuralları

> sqlc bir ORM değil, kod üretecidir: SQL'i biz yazarız, o tip güvenli Go üretir.
> Gerekçe: `docs/design.md` §2.1 · ADR-004

- **Şema yalnız `goose` migration'ları ile değişir.** `api/migrations/*.sql`
- **Sorgular `api/queries/*.sql` içinde yazılır.** Adlandırma: `-- name: XxxYyy :one|:many|:exec`
- **`api/internal/db/` ÜRETİLEN koddur — elle düzenlenmez.** Değişiklik `queries/` içinde yapılır,
  sonra `make gen`
- **CI `sqlc diff` çalıştırır** — üretilen kod güncel değilse derleme başarısız olur
- **Transaction:** `qtx := q.WithTx(tx)`; transaction sınırı `service` katmanında çizilir
- **Dinamik filtreler:** `sqlc.narg()` + `COALESCE` deseni. **Go'da string birleştirerek SQL kurulmaz**
- **İndeks eklemeden önce `EXPLAIN ANALYZE`**

## Go stili

- `gofumpt` + `golangci-lint` — CI zorlar
- Hata sarmalama: `fmt.Errorf("sipariş oluşturulamadı: %w", err)`; kontrol `errors.Is/As`
- **Hata yutulmaz.** `_ = doSomething()` yalnız gerekçeli yorumla
- Dışa açık her fonksiyonun ilk parametresi `ctx context.Context`
- Alıcı (receiver) adları kısa ve tutarlı: `func (r *WalletRepo)`
- `panic` yalnız açılışta (yapılandırma eksikliği); istek yolunda asla
- Yorumlar **neden**i açıklar, **ne**yi değil. Türkçe yorum serbest, kod İngilizce

## TypeScript / React stili

- `strict: true`, `any` yasak (gerekirse `unknown` + daraltma)
- Sunucu bileşeni varsayılan; `"use client"` yalnız gerektiğinde
- Veri çekme: sunucuda `fetch`, istemcide TanStack Query
- Form: React Hook Form + Zod; şemalar OpenAPI tiplerinden türer
- Para/tarih biçimleme yalnız `web/src/lib/format` içinde

---

## Reçeteler

### Yeni sağlayıcı ekleme (hedef: 1 gün)
1. `providers.protocol` enum'una yeni değer ekle (migration)
2. `internal/adapter/provider/<isim>/` paketi oluştur, `port.ProviderPort` arayüzünü uygula
3. Gerçek API yanıtlarını `api/testdata/fixtures/<isim>/` altına kaydet
4. Sözleşme testi yaz — her metot için en az bir başarı ve bir hata senaryosu
5. `registry.go`'ya kaydet
6. Hata haritalaması: sağlayıcının ham hatalarını `ErrOutOfStock`, `ErrProviderAuth`,
   `ErrProviderTimeout`, `ErrProviderUnavailable` tiplerine çevir
7. Admin panelinden sağlayıcıyı ekle, boyut eşleştirmelerini senkronla
8. `docs/memory.md` §4'e API notlarını yaz

### Yeni uç nokta ekleme
1. `api/openapi/openapi.yaml`'a yaz — **sözleşme önce**
2. `make gen` → Go iskeleti + TS tipleri üretilir
3. `transport/http/dto/` içinde istek/yanıt yapıları (`validator` etiketleriyle)
4. `service/` içinde kullanım senaryosu
5. `handler` yaz — ince olmalı: bağla → çağır → yanıt yaz
6. Router'a **doğru middleware ile** ekle: `RequirePermission(...)` ve/veya sahiplik
7. **Üç yetki testi zorunlu:** oturumsuz(401) · yetkisiz(403) · başkasının kaynağı(404)
8. `docs/trd.md`'ye FR numarası ve kabul kriteri ekle

### Veritabanı değişikliği
1. `make migrate-new isim` → `goose` dosyası oluştur
2. `Up` **ve** `Down` ikisini de yaz
3. Para alanı ekliyorsan `BIGINT` + gerekiyorsa `CHECK` kısıtı
4. İndeks düşün: sorgu deseni ne? (`EXPLAIN ANALYZE` ile doğrula)
5. Gerekiyorsa `api/queries/*.sql` içindeki sorguları güncelle, sonra `make gen`
6. Entegrasyon testini çalıştır (testcontainers gerçek Postgres kullanır)

### Yeni ekran / bileşen ekleme
1. Mobil taban stilleri yaz, sonra `md:`/`lg:` ekle — **asla tersi**
2. Tablo varsa `< md` için kart sunumu da yaz
3. Metinleri `web/messages/tr.json`'a ekle, bileşende sabit yazma
4. Yükleniyor / boş / hata durumlarının **üçünü de** tasarla; hatada `requestId` göster
5. Veri: sunucu bileşeninde `fetch`, istemcide TanStack Query — ikisi de `apiFetch` üzerinden
6. `frontend-contract.md` §9 kontrol listesini uygula
7. Playwright testini `webkit-mobile` projesinde çalıştır

### Para hareketi ekleme
1. `LedgerType` enum'una tip ekle
2. `idempotency_key` şemasını belirle — deterministik olmalı (örn. `order:{uuid}:refund`)
3. `WalletRepo.ApplyEntry` çağır — **asla doğrudan `UPDATE`**
4. **Eşzamanlılık testi yaz:** aynı anahtarla N paralel çağrı → tek etki
5. Mutabakat testini çalıştır: `Σ ledger == balance`

---

## Test kuralları

| Alan | Hedef | Nasıl |
|---|---|---|
| `internal/domain/**` | ≥ %95 | Saf birim testi, bağımlılık yok |
| `internal/service/**` | ≥ %80 | Sahte (fake) portlarla |
| Sağlayıcı adaptörleri | %100 metot | Kaydedilmiş fixture ile sözleşme testi |
| Para yolları | **Eşzamanlılık testi zorunlu** | `t.Parallel()` + `errgroup` |
| Yetkilendirme | Her uç nokta | 401/403/404 matrisi |

- Gerçek Postgres kullan (`testcontainers-go`), SQLite ile taklit etme — `FOR UPDATE` davranışı farklı
- Zaman `ClockPort` üzerinden — `time.Now()` doğrudan çağrılmaz
- Dış çağrılar `FakeProvider` ile — testte gerçek ağ yok
- **Test yazılmadan özellik bitmiş sayılmaz.**

**Ön yüz testi:** Uçtan uca akışlar dört Playwright projesinde çalışır —
`chromium-desktop`, `webkit-desktop`, `chromium-mobile`, `webkit-mobile`.
**`webkit-mobile` (iPhone 14) en kritik olanıdır** — iOS'ta tüm tarayıcılar WebKit'tir.
SSE davranışı ayrıca **gerçek cihazda** doğrulanır; simülatör arka plan davranışını taklit etmez.

---

## Asla yapma

- ❌ Bakiyeyi `WalletRepo` dışında güncelleme
- ❌ Para için `float64` kullanma
- ❌ Transaction içinde HTTP çağrısı yapma
- ❌ `GET` ile durum değiştirme
- ❌ İstemciden gelen fiyat/sağlayıcı bilgisine güvenme
- ❌ `internal/db/` altındaki üretilen kodu elle düzenleme
- ❌ Go'da string birleştirerek SQL kurma
- ❌ Sahiplik kontrolünü atlama veya sorgudan ayırma
- ❌ Ham hata mesajını kullanıcıya gösterme
- ❌ Sır, API anahtarı veya şifreyi koda yazma
- ❌ Log'a şifre, oturum kimliği, API anahtarı, SMS kodu veya tam telefon numarası yazma
- ❌ `Purchase` çağrısını yeniden deneme
- ❌ Kullanıcıya görünen metni bileşene sabit yazma
- ❌ `docs/` güncellemeden mimari karar değiştirme
- ❌ Bileşende doğrudan `fetch` çağırma (→ `apiFetch`)
- ❌ Mutasyonu otomatik yeniden deneme
- ❌ `max-md:` ile masaüstünden mobile geri alma zinciri yazma
- ❌ Mobilde yatay kaydırılan tablo bırakma
- ❌ 16px'ten küçük fontlu girdi alanı (iOS yakınlaşır)
- ❌ `100vh` kullanma (→ `100dvh` + yedek)
- ❌ Yerel geri sayım sayacı kullanma (→ sunucudaki `expiresAt`)
- ❌ `new Date()` ile boşluklu tarih ayrıştırma
- ❌ İstemcide para aritmetiği yapma
- ❌ Sağlayıcının `count`/`total` alanını stok olarak kullanma
- ❌ `maxPrice` göndermeden satın alma çağrısı yapma
- ❌ Webhook gövdesindeki `code` alanını doğrudan siparişe yazma
- ❌ Sipariş süresini koda gömme (→ `expiredAt`)
- ❌ `GET /activations/{id}` çağırma (yok) → `/otp/last`
- ❌ Webhook handler'ında senkron DB/HTTP işi yapma (3 sn bütçe)
- ❌ Sağlayıcı `status` kodunu doğrudan `orders.status`'e yansıtma
- ❌ `fixedPrice: true` gönderme
- ❌ Sağlayıcı yanıtını tekil obje olarak ayrıştırma
- ❌ API anahtarını koda, log'a, sohbete veya doküman dosyasına yazma

---

## Dil

- **Kod İngilizce:** değişken, fonksiyon, tablo, sütun, commit mesajı, kod yorumu (Türkçe yorum da olur)
- **Kullanıcıya görünen her şey Türkçe:** hata mesajları, arayüz metinleri, e-postalar — hepsi
  `web/messages/tr.json` içinde
- **Dokümanlar Türkçe**

---

## Commit ve dal

- Dal: `feat/...`, `fix/...`, `chore/...`
- Commit: `<tip>(<kapsam>): <ne yapıldı>` — örn. `feat(wallet): idempotent ledger girişi ekle`
- **`"fix"` gibi anlamsız commit mesajı yazılmaz.** *(Eski repoda 127 commit'in tamamı böyleydi.)*
- `main` her zaman dağıtılabilir olmalı
- Birleştirmeden önce `make check` yeşil olmalı

---

## Oturum başında

1. `docs/memory.md` §5 (tuzaklar) ve §6 (açık sorular) — hızlı göz at
2. `docs/roadmap.md` — hangi kilometre taşındayız?
3. Değişiklik para veya yetkiye dokunuyorsa yukarıdaki **Değişmezler** listesini tekrar oku
4. Ön yüz kodu yazacaksan `docs/frontend-contract.md` — özellikle SSE'ye dokunuyorsan §4
5. Sağlayıcı adaptörüne dokunacaksan `docs/provider-herosms.md` — özellikle iade akışı için §6
6. Bir karar verildiyse `docs/memory.md` §1 karar günlüğüne yaz
