# design.md — Sistem Tasarımı

> **Doküman amacı:** "Nasıl inşa ediyoruz ve neden böyle." Kararların gerekçeleri ve reddedilen
> alternatifler burada durur; altı ay sonra "bunu neden böyle yapmışız" sorusu tekrar tartışılmaz.
>
> **Durum:** v1 · **Son güncelleme:** 2026-09-08 · **Önkoşul:** [intent.md](intent.md)

---

## 0. Karar özeti

| Konu | Karar |
|---|---|
| Backend | **Go 1.23+**, Gin (HTTP), **sqlc + pgx** (veri erişimi), goose (migration) |
| Frontend | **Next.js 15 (App Router) + React + TypeScript + Tailwind CSS 4** |
| Görsel | Mevcut Duralux koyu temaya sadık, Tailwind ile yeniden inşa |
| Veritabanı | **PostgreSQL 16** |
| Önbellek / oturum / kuyruk | **Redis 7** |
| Topoloji | Tek alan adı, ters vekil: `/` → Next.js, `/api` → Go |
| Repo | **Monorepo:** `/api` (Go) + `/web` (Next.js) + `/docs` + `/deploy` |
| Kimlik | Sunucu tarafı oturum, Redis, `httpOnly` + `SameSite=Lax` çerez |
| Cüzdan | **Yalnız TRY**, tam sayı kuruş, ledger tabanlı |
| Kod bildirimi | Sağlayıcı **webhook** → Redis Pub/Sub → **SSE**; yoklama güvenlik ağı |
| Dağıtım | VPS + Docker Compose + Caddy |
| Sağlayıcı | **HeroSMS** (`HEROSMS_V1`, modern REST + webhook) — [provider-herosms.md](provider-herosms.md) |

---

## 1. Mimari genel bakış

İki dağıtılabilir birim, tek veri kaynağı. Mikroservis **değil**.

```
                    ┌──────────────────────────┐
   Tarayıcı ───────▶│  Caddy (ters vekil)      │
                    │  TLS · sıkıştırma · log  │
                    └────┬────────────────┬────┘
                         │ /              │ /api/*
              ┌──────────▼───────┐   ┌────▼─────────────────────────────┐
              │  Next.js (web)   │   │  Go API (api)                    │
              │  React · Tailwind│   │                                  │
              │  SSR + istemci   │   │  ┌────────────────────────────┐  │
              │  next-intl       │   │  │ transport/http (Gin)       │  │
              └──────────────────┘   │  │  handler · dto · middleware│  │
                     │               │  └─────────────┬──────────────┘  │
                     │  fetch (çerez)│  ┌─────────────▼──────────────┐  │
                     └──────────────▶│  │ service (kullanım senaryosu)│ │
                                     │  │  transaction sınırı burada  │ │
                                     │  └───┬────────────────────┬────┘ │
                                     │  ┌───▼──────────┐  ┌──────▼────┐ │
                                     │  │ domain       │  │ port      │ │
                                     │  │ money·pricing│  │ arayüzler │ │
                                     │  │ durum makine │  └──────┬────┘ │
                                     │  └──────────────┘         │      │
                                     │  ┌────────────────────────▼────┐ │
                                     │  │ adapter (altyapı)           │ │
                                     │  │ postgres·redis·herosms·mail │ │
                                     │  └─────────────────────────────┘ │
                                     └────────┬──────────────┬──────────┘
                                              ▼              ▼
                                        PostgreSQL        Redis
```

**Bağımlılık yönü tek yönlüdür:** `transport → service → domain`. `adapter` katmanı, `port` paketindeki
arayüzleri *uygular*; `service` somut adaptörü tanımaz. Pratik faydası: `HeroSMSAdapter` yerine
`FakeProvider` koyup tüm satın alma akışını ağ olmadan test edebilmek.

### 1.1 Reddedilen alternatifler

| Alternatif | Neden reddedildi |
|---|---|
| Mikroservisler | Dağıtık transaction gerekir; para sistemi için gereksiz maliyet. Tek geliştirici için ölümcül operasyonel yük. |
| Next.js tek başına (API Routes ile) | Go tercihi bilinçli: paralel sağlayıcı sorgusu, SSE, arka plan işçileri ve tip güvenliği için doğru araç. |
| Serverless | Uzun SSE bağlantıları ve kalıcı arka plan işçileri için kötü uyum. |
| Next.js BFF (tarayıcı Go'yu görmez) | Her uç nokta için iki kez kod yazmak demek; tek geliştiricide çift iş. |

---

## 2. Teknoloji seçimleri ve gerekçeleri

| Katman | Seçim | Gerekçe |
|---|---|---|
| Backend dili | **Go 1.23+** | Paralel sağlayıcı sorgusu goroutine ile doğal; tek ikili dosya dağıtımı; güçlü tip sistemi; düşük bellek. Öğrenme eğrisi kabul edildi (bkz. `roadmap.md` M0). |
| HTTP | **Gin** | En büyük ekosistem ve en çok öğrenme materyali — Go'ya yeni başlayan için belirleyici. *Alternatif: Echo (denk), chi (daha saf ama daha az rehber).* |
| Veri erişimi | **sqlc + pgx/v5** | ORM değil, kod üreteci: SQL'i biz yazarız, sqlc ondan tip güvenli Go üretir. Gizli sorgu, örtük davranış, N+1 sürprizi yok. Para sistemi için doğru araç — §2.1 |
| Migration | **goose** | Sürüm kontrollü, düz SQL. Şemanın tek kaynağı; sqlc bu dosyaları okur (§2.1). |
| DB sürücüsü | **pgx/v5** | sqlc'nin ürettiği kodun altında; `FOR UPDATE`, `COPY`, doğru tip eşlemesi, bağlantı havuzu. |
| Doğrulama | **go-playground/validator** + el yazımı DTO | Her istek gövdesi ayrı bir `Request` yapısına bağlanır; model yapısına **asla** doğrudan bağlanmaz. |
| Arka plan işleri | **asynq** (Redis) | Go'nun BullMQ'su; zamanlanmış + kuyruk işleri, yeniden deneme, izleme paneli. |
| Log | **log/slog** (stdlib, JSON) | Standart kütüphane, bağımlılık yok, `request_id` ile korelasyon. |
| Hata takibi | **Sentry** | Üretimde görünürlük. |
| Test | **stdlib testing + testify + testcontainers-go** | Gerçek Postgres'e karşı entegrasyon testi. |
| API sözleşmesi | **OpenAPI 3.1** (el yazımı, kaynak-doğruluk) | `oapi-codegen` ile Go sunucu iskeleti, `openapi-typescript` ile Next.js tipleri → **tek sözleşme, iki taraf senkron**. |
| Frontend | **Next.js 15 App Router** | SSR, dosya tabanlı yönlendirme, olgun ekosistem. |
| Stil | **Tailwind CSS 4** | Tek sürüm (mevcut projedeki v3/v4 çakışması giderilir). |
| Bileşenler | **shadcn/ui** (Radix tabanlı) | Kopyala-sahiplen modeli: bağımlılık kilidi yok, Duralux görünümüne serbestçe uyarlanır. |
| Sunucu durumu | **TanStack Query** | Önbellek, yeniden deneme, geçersizleştirme. |
| Form | **React Hook Form + Zod** | Zod şemaları OpenAPI tiplerinden türetilir. |
| Tablo | **TanStack Table** | Duralux'taki DataTables'ın yerine, jQuery'siz. |
| i18n | **next-intl** | Türkçe varsayılan; metinler sözlük dosyalarında. |
| Dağıtım | **Docker Compose + Caddy** | VPS'te tek komutla; Caddy otomatik TLS. |
| CI | **GitHub Actions** | `sqlc diff` → `go vet` → `golangci-lint` → `go test` → `tsc` → `eslint` → `next build`. |

### 2.1 Veri erişimi: sqlc

> **Karar değişikliği (2026-09-08):** Başlangıçta GORM seçilmişti; gerekçesi "geliştiricinin
> öğrenme hızı"ydı. Projeyi Claude geliştireceği için o gerekçe geçersiz kaldı ve karar
> yeniden değerlendirildi. → ADR-004

**sqlc bir ORM değildir, bir kod üretecidir.** Şema ve sorguları SQL olarak yazarız; sqlc
bunlardan tip güvenli Go fonksiyonları üretir. Üretilen kod okunabilir, adımlanabilir ve
çalıştırdığı SQL tam olarak yazdığımızdır.

```
api/
├── migrations/          # goose — şemanın tek kaynağı
│   └── 001_init.sql
├── queries/             # sqlc girdisi — elle yazılan SQL
│   ├── users.sql
│   ├── wallet.sql       # ledger + FOR UPDATE kilitleri
│   ├── orders.sql
│   └── catalog.sql
└── internal/db/         # sqlc ÇIKTISI — elle düzenlenmez
    ├── models.go
    ├── querier.go
    └── *.sql.go
```

Örnek — cüzdan kilidi, olduğu gibi:

```sql
-- queries/wallet.sql
-- name: LockUserBalance :one
SELECT id, balance_minor FROM users WHERE id = $1 FOR UPDATE;

-- name: GetLedgerEntryByKey :one
SELECT balance_after_minor FROM ledger_entries WHERE idempotency_key = $1;

-- name: InsertLedgerEntry :one
INSERT INTO ledger_entries (user_id, amount_minor, currency, entry_type,
                            reference_type, reference_id, balance_after_minor,
                            idempotency_key, created_by_user_id, note)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
RETURNING *;
```

sqlc bunlardan `LockUserBalance(ctx, id) (LockUserBalanceRow, error)` gibi imzalar üretir.
`FOR UPDATE` bir eklenti veya kaçış deliği değil — sadece SQL'in kendisi.

**Kurallar:**

1. **Şema yalnız `goose` migration'ları ile değişir.** sqlc migration'ları okur, oluşturmaz.
2. **`internal/db/` elle düzenlenmez.** Üretilen dosyalar; değişiklik `queries/*.sql` içinde yapılır.
   CI, `sqlc generate` sonrası çalışma ağacının temiz kaldığını doğrular (üretim güncel mi).
3. **Her sorgu `ctx` alır.** sqlc bunu zaten böyle üretir.
4. **Transaction:** `qtx := q.WithTx(tx)` — transaction sınırı `service` katmanında çizilir,
   `db` katmanında değil.
5. **Dinamik sorgu gerekiyorsa** (admin listelerindeki isteğe bağlı filtreler) sqlc'nin
   `sqlc.narg()` / `COALESCE` deseni kullanılır; Go'da string birleştirerek SQL kurulmaz.
6. **`EXPLAIN ANALYZE` ile doğrulanmamış indeks eklenmez.**

**sqlc'nin GORM'a karşı somut kazancı:** Bu bir para sistemi. GORM ile "para yollarında ham SQL
kullan, `Save` kullanma, `AutoMigrate` çağırma" gibi bir korkuluk listesi tutmak ve her kod
incelemesinde bunlara uyulduğunu denetlemek gerekiyordu. sqlc ile bu korkuluklar **gereksiz**:
zaten her sorgu açık SQL, örtük bir yazma yolu yok.

---

## 3. Dizin yapısı (monorepo)

```
.
├── api/                              # Go servisi
│   ├── cmd/
│   │   ├── server/main.go            # HTTP sunucusu
│   │   ├── worker/main.go            # asynq işçileri + zamanlayıcı
│   │   └── cli/main.go               # yönetim komutları (admin oluştur, mutabakat)
│   ├── internal/                     # dışarı açılmayan kod (Go derleyicisi zorlar)
│   │   ├── config/                   # ortam değişkeni yükleme + DOĞRULAMA (eksikse panic)
│   │   ├── domain/                   # saf iş mantığı, bağımlılıksız, %100 test edilebilir
│   │   │   ├── money/                # Money değer tipi (int64 kuruş)
│   │   │   ├── pricing/              # maliyet -> satış fiyatı
│   │   │   ├── order/                # durum makinesi
│   │   │   └── errors/               # DomainError hiyerarşisi
│   │   ├── port/                     # ARAYÜZLER (sözleşmeler)
│   │   │   ├── provider.go           # ProviderPort
│   │   │   ├── repository.go         # depo arayüzleri
│   │   │   ├── fx.go · mailer.go · clock.go · cache.go
│   │   ├── service/                  # kullanım senaryoları — transaction sınırı burada
│   │   │   ├── auth/ · wallet/ · catalog/ · order/ · deposit/ · ticket/ · admin/
│   │   ├── adapter/                  # port uygulamaları
│   │   │   ├── postgres/             # sqlc üretimi üzerine ince depo sarmalayıcıları
│   │   │   ├── redis/                # oturum, önbellek, kilit
│   │   │   ├── provider/
│   │   │   │   ├── registry.go       # protocol -> adaptör
│   │   │   │   ├── smsactivate/      # HeroSMS ve aynı protokoldeki türevler
│   │   │   │   ├── fivesim/          # (v1.1)
│   │   │   │   └── fake/             # testler ve yerel geliştirme
│   │   │   ├── fx/ · mailer/ · sentry/
│   │   ├── transport/http/
│   │   │   ├── router.go
│   │   │   ├── middleware/           # requestid · session · rbac · ratelimit · csrf · recover · cors
│   │   │   ├── handler/              # ince: DTO bağla -> service çağır -> yanıt yaz
│   │   │   ├── dto/                  # istek/yanıt yapıları — DOMAIN MODELİ ASLA DIŞARI SIZMAZ
│   │   │   └── sse/                  # olay akışı
│   │   └── worker/                   # zamanlanmış ve kuyruk işleri
│   ├── migrations/                   # goose .sql dosyaları
│   ├── openapi/openapi.yaml          # API SÖZLEŞMESİ — tek doğruluk kaynağı
│   └── testdata/fixtures/            # kaydedilmiş gerçek sağlayıcı yanıtları
│
├── web/                              # Next.js uygulaması
│   ├── src/app/
│   │   ├── (public)/                 # tanıtım sayfaları
│   │   ├── (auth)/                   # giriş, kayıt, şifre sıfırlama
│   │   ├── (panel)/                  # kullanıcı paneli — oturum zorunlu
│   │   │   ├── dashboard/ · orders/ · wallet/ · tickets/ · profile/
│   │   └── (admin)/                  # admin — izin zorunlu
│   │       ├── users/ · providers/ · pricing/ · deposits/ · tickets/ · audit/
│   │   └── api/                      # yalnız oturum köprüsü — iş mantığı YOK
│   ├── src/components/
│   │   ├── ui/                       # shadcn tabanlı temel bileşenler
│   │   └── domain/                   # ürün bileşenleri (ServiceCard, CodeWaiter, ...)
│   ├── src/lib/
│   │   ├── api/                      # üretilen tipler + fetch sarmalayıcı
│   │   ├── hooks/                    # useOrderStream (SSE), useQuote, ...
│   │   └── format/                   # para/tarih biçimleme — TEK YER
│   ├── messages/tr.json              # i18n sözlüğü
│   └── ...
│
├── deploy/                           # docker-compose.yml, Caddyfile, .env.example
├── docs/                             # bu dosyalar
└── CLAUDE.md
```

**Modül sınırı kuralı:** `service` paketleri birbirinin `repository`sini çağıramaz; yalnız diğer
`service`in dışa açık metodunu çağırır. `transport` katmanı `adapter` paketlerini import edemez.

---

## 4. Genişletilebilir katalog modeli

> Bu bölüm, mevcut veritabanından duyulan asıl memnuniyetsizliğe cevap verir:
> **"Yeni servis/özellik eklerken tablo değiştirmek zorunda kalmayayım."**

### 4.1 Problem

Mevcut şema `Country` × `Service` ikilisine ve iki ayrı eşleştirme tablosuna
(`ProviderCountryMapping`, `ProviderPlatformMapping`) sabitlenmiş. Kiralık numara eklemek yeni tablolar,
yeni eşleştirme tablosu ve mevcut sorguların değiştirilmesini gerektirir. Proxy satmak istesek baştan başlanır.

### 4.2 Çözüm: üç katmanlı ayrım

```
  BOYUTLAR (dimensions)          ÜRÜN (ne satıyoruz)         SAĞLAYICI TEKLİFİ (kim tedarik ediyor)
  ┌──────────────┐               ┌──────────────────┐        ┌────────────────────────┐
  │ services     │──┐            │ products         │───────▶│ provider_offers        │
  │ countries    │──┼───────────▶│  kind            │        │  cost_minor            │
  │ operators    │──┘            │  service_id      │        │  stock                 │
  │ (gelecekte:  │               │  country_id      │        │  is_available          │
  │  durations,  │               │  duration_min    │        │  synced_at             │
  │  protocols)  │               │  attributes JSONB│        └────────────────────────┘
  └──────┬───────┘               └──────────────────┘                    ▲
         │                                                               │
         │        ┌──────────────────────────────┐                       │
         └───────▶│ provider_dimension_maps      │───────────────────────┘
                  │  provider_id                 │
                  │  dimension  ('country'|      │   ⬅ YENİ BOYUT EKLEMEK:
                  │              'service'|...)  │      yeni SATIR, yeni TABLO değil
                  │  local_id                    │
                  │  remote_code                 │
                  └──────────────────────────────┘
```

**Üç kural:**

1. **`provider_dimension_maps`** yerel bir boyut değerini sağlayıcının uzak koduna çevirir.
   Bugün `country` ve `service`; yarın `operator`, `duration`, `proxy_protocol` — hepsi aynı tabloda.
   *Mevcut sistemdeki iki ayrı eşleştirme tablosu burada birleşir.*
2. **`products`** satılabilir birimdir (SKU). `kind` alanı ürün tipini ayırır:
   `SMS_ACTIVATION` (v1) · `SMS_RENTAL` (şema hazır, v1.1) · gelecekte diğerleri.
   Bilinen alanlar **tiplidir** (sütun); tipe özgü ekstralar `attributes JSONB` içinde durur ve
   her `kind` için bir JSON Schema ile doğrulanır. *EAV bataklığına girilmez: sık sorgulanan hiçbir
   alan JSONB'de tutulmaz.*
3. **`provider_offers`** fiyat/stok **anlık görüntüsüdür** (önbellek). Kaynak doğruluk sağlayıcının
   API'sidir; bu tablo listeleme ve filtreleme için vardır, satın alma anında **her zaman canlı fiyat sorulur.**

**⚠️ Bu modelin doğrulanmış sınırları (HeroSMS analizinden):**

| Sınır | Sonuç |
|---|---|
| **Operatör boyutu fiyatlanamıyor** — `getOperators` yalnız isim listesi döner; hiçbir uç noktada operatör kırılımlı fiyat/stok yok | `offers-sync` yalnız (servis, ülke) doldurur; `operator = 'any'` sabitlenir (ADR-029) |
| **`verificationType` (sms\|call) demetimizde olmayan bir boyut** ve `offers`'ta path segmenti; aynı servis+ülke için farklı fiyat/stok | `products.verification_type` sütunu eklendi |
| **Kiralıkta `duration` boyutu modern `offers`'ta YOK** | Kiralık fiyat/stok yalnız legacy'den → hibrit akış |
| **Kiralık süreleri spec'te üç yerde çelişiyor** (`RentDuration` enum vs `BAD_DURATION.info` vs `serviceCountRent` örneği) | Süreler **canlıdan** öğrenilir, sabit kodlanmaz |
| **E-posta ürününde `site` kataloğunu listeleyen uç nokta YOK** | `catalog-sync` bu boyutu **otomatik keşfedemez** — elle tanımlanır |

**Yeni ürün tipi eklemenin maliyeti:**

| Adım | v1'de | Yeni bir tip eklerken |
|---|---|---|
| Boyut eşleştirmesi | `provider_dimension_maps` | ✅ satır ekle |
| Ürün tanımı | `products.kind` | ✅ enum değeri + JSON Schema |
| Sipariş kaydı | `orders` ortak tablo | ✅ değişiklik yok |
| Tipe özgü alanlar | `attributes` / detay tablosu | ⬜ gerekiyorsa küçük detay tablosu |
| Sağlayıcı yeteneği | `ProviderPort.Capabilities()` | ✅ adaptörde ilan et |

---

## 5. Para modeli — sistemin kalbi

### 5.1 Kural: Para asla ondalıklı sayı (float) değildir

```go
// YANLIŞ — mevcut sistem böyle yapıyor
user.Balance = parseFloat(user.Balance) - salePrice   // 0.1 + 0.2 = 0.30000000000000004

// DOĞRU
type Minor int64          // TRY için kuruş, USD için cent
type Money struct {
    Minor    Minor
    Currency Currency      // TRY | USD
}
func (m Money) Add(o Money) (Money, error)  // farklı para birimi -> hata
func (m Money) IsNegative() bool
```

Tüm para değerleri **tam sayı, en küçük birim** olarak saklanır ve taşınır. Veritabanında `BIGINT`.

> ⚠️ **Ölçek uyarısı (ADR-026):** Kullanıcıya dönük TRY tutarları **kuruş** (2 hane) yeterlidir.
> Ama **sağlayıcı maliyeti kuruş/sent'e sığmaz:** HeroSMS fiyatları `float`, örnekler **4 ondalıklı**
> (`0.4321`), `MaxPrice.minimum = 0.0067`. Sent (2 hane) saklamak birim başına **0,0021 USD'ye
> kadar kırpar**; `amount` 10'a kadar çıkabildiği için batch'te 10 katı.
> → `orders.cost_micro_usd` **mikro-birim (6 hane)** olarak saklanır.
>
> Sağlayıcı JSON'u `float64`'e **değil**, `json.Number`/string olarak okunur ve oradan
> tam sayıya çevrilir.
Biçimleme yalnız sunum katmanında yapılır. `Money` tipi para birimi taşır; `TRY + USD` toplaması
derleme zamanında değil ama **çalışma zamanında ilk testte** yakalanır ve panic yerine hata döner.

> **JSON'da para:** API yanıtlarında para **her zaman** `{"minor": 12345, "currency": "TRY", "formatted": "123,45 ₺"}`
> şeklinde döner. Asla çıplak ondalıklı sayı gönderilmez — JavaScript tarafında float'a dönüşür.

### 5.2 Kural: Bakiye bir sayı değil, defterin sonucudur

Mevcut sistem `users.balance` alanını doğrudan güncelliyor: geçmiş yok, denetlenemez, eşzamanlılıkta bozulur.

**Yeni model: değişmez hareket defteri.**

```sql
CREATE TABLE ledger_entries (
  id                   BIGSERIAL PRIMARY KEY,
  user_id              BIGINT NOT NULL REFERENCES users(id),
  amount_minor         BIGINT NOT NULL,          -- + alacak, - borç
  currency             currency_t NOT NULL DEFAULT 'TRY',
  entry_type           ledger_type_t NOT NULL,   -- DEPOSIT|PURCHASE|REFUND|ADJUSTMENT|COMMISSION|CHARGEBACK
  reference_type       TEXT,                     -- 'deposit' | 'order' | 'manual' | 'referral'
  reference_id         BIGINT,
  balance_after_minor  BIGINT NOT NULL,          -- mutabakat için
  idempotency_key      TEXT NOT NULL UNIQUE,     -- ÇİFT İŞLEM KORUMASI
  created_by_user_id   BIGINT,                   -- admin işlemiyse
  note                 TEXT,
  created_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ON ledger_entries (user_id, created_at DESC);
```

- `users.balance_minor` **türetilmiş önbellektir**; aynı transaction içinde ledger ile birlikte güncellenir.
- Her gece mutabakat işi: `Σ amount_minor GROUP BY user_id == users.balance_minor`.
  Sapmada **alarm** üretilir, otomatik düzeltme **yapılmaz** — sapma bir hatanın belirtisidir, düzeltmek belirtiyi gizler.
- Ledger kayıtları **asla güncellenmez veya silinmez.** Düzeltme ters kayıtla (`ADJUSTMENT`) yapılır.
  Bu bir `BEFORE UPDATE/DELETE` tetikleyicisiyle veritabanı düzeyinde de zorlanır.

### 5.3 Kural: Eşzamanlılık kilit ile çözülür

```go
// internal/adapter/postgres/wallet_repo.go — TÜM bakiye değişimleri buradan geçer
// Sorgular queries/wallet.sql içinde; burada yalnız sıralama ve iş kuralı var (bkz. §2.1)
func (r *WalletRepo) ApplyEntry(ctx context.Context, tx *sql.Tx, in port.LedgerInput) (money.Money, error) {
    // 1) Idempotency: aynı anahtar işlendiyse eski sonucu dön
    var after int64
    err := tx.QueryRowContext(ctx,
        `SELECT balance_after_minor FROM ledger_entries WHERE idempotency_key = $1`, in.Key).Scan(&after)
    if err == nil {
        return money.New(after, in.Currency), nil          // tekrar işleme YOK
    }

    // 2) Kullanıcı satırını KİLİTLE — bu satır olmadan çift harcama mümkündür
    var current int64
    if err := tx.QueryRowContext(ctx,
        `SELECT balance_minor FROM users WHERE id = $1 FOR UPDATE`, in.UserID).Scan(&current); err != nil {
        return money.Zero, err
    }

    // 3) Yeterlilik kontrolü KİLİT ALTINDA
    next := current + in.AmountMinor
    if next < 0 {
        return money.Zero, domain.ErrInsufficientBalance
    }

    // 4) Ledger + bakiye TEK transaction içinde
    if _, err := tx.ExecContext(ctx, `INSERT INTO ledger_entries (...) VALUES (...)`, ...); err != nil {
        return money.Zero, err
    }
    if _, err := tx.ExecContext(ctx,
        `UPDATE users SET balance_minor = $1 WHERE id = $2`, next, in.UserID); err != nil {
        return money.Zero, err
    }
    return money.New(next, in.Currency), nil
}
```

**Değişmezler:**
- `CHECK (balance_minor >= 0)` — veritabanı düzeyinde
- Bakiye değişimi **yalnız** `WalletRepo.ApplyEntry` üzerinden (kod incelemesi ve CI kuralıyla zorlanır)
- Her para işlemi bir `idempotency_key` taşır
- `SELECT ... FOR UPDATE` olmadan bakiye okunup yazılmaz

### 5.4 Kural: Dış çağrı transaction içinde yapılmaz

Mevcut `confirmPurchase` bir DB transaction'ı açıp içinde iki HTTP çağrısı yapıyor (10 sn zaman aşımıyla).
Sonuç: bağlantı havuzu tükenmesi, kilit birikmesi ve en kötüsü — süreç ölürse **sağlayıcıdan numara
alınmış ama kullanıcıdan tahsilat yapılmamış** olur.

**Doğru desen — provizyon / onay:**

```
T1 (kısa transaction) : teklifi tüket + bakiyeyi düş  → ledger: PURCHASE
   (transaction DIŞINDA) sağlayıcıya git              → numara al
T2 (kısa transaction) : başarılı → siparişi oluştur
                        başarısız → ledger: REFUND (aynı referans, farklı anahtar)
```

T1 ile T2 arasında süreç ölürse: `orphan-hold-reaper` işi tüketilmiş ama siparişi olmayan teklifleri
bulur, sağlayıcıya durumu sorar, ya siparişi oluşturur ya iade eder. Bu iş **idempotent** olmak zorundadır.

---

## 6. Veri modeli

Aşağıda tasarım niyeti verilmiştir; tam SQL `api/migrations/` altındadır.

```sql
-- ─────────── KİMLİK ───────────
users (
  id BIGSERIAL PK, public_id UUID UNIQUE,       -- dışarıya public_id verilir (sayaç sızdırmaz)
  email CITEXT UNIQUE, email_verified_at TIMESTAMPTZ,
  username CITEXT UNIQUE, password_hash TEXT,   -- argon2id
  balance_minor BIGINT NOT NULL DEFAULT 0 CHECK (balance_minor >= 0),
  status user_status_t NOT NULL DEFAULT 'PENDING_VERIFICATION',  -- ACTIVE|SUSPENDED|PENDING_VERIFICATION
  referral_code TEXT UNIQUE,                    -- v1.1 hazır
  referred_by_user_id BIGINT REFERENCES users(id),
  created_at, updated_at, deleted_at
)
roles (id, name UNIQUE, description)
permissions (id, code UNIQUE)                   -- 'deposits:approve', 'providers:write', ...
role_permissions (role_id, permission_id)       -- yeni yetki = veri, kod değil
user_roles (user_id, role_id)

-- ─────────── KATALOG (genişletilebilir, §4) ───────────
services   (id, code UNIQUE, name, name_tr, icon_url, is_visible, sort_order)
countries  (id, iso2 UNIQUE, name, name_tr, phone_code, is_visible)
operators  (id, code, country_id, name)         -- 'any' varsayılan

products (
  id BIGSERIAL PK,
  kind product_kind_t NOT NULL,                 -- SMS_ACTIVATION | SMS_RENTAL | (gelecek)
  service_id BIGINT REFERENCES services(id),
  country_id BIGINT REFERENCES countries(id),
  operator_id BIGINT REFERENCES operators(id),
  duration_minutes INT,                         -- NULL = tek seferlik
  attributes JSONB NOT NULL DEFAULT '{}',       -- tipe özgü, JSON Schema ile doğrulanır
  is_active BOOLEAN NOT NULL DEFAULT true,
  -- 🔴 NULLS NOT DISTINCT zorunlu: EMAIL_ACTIVATION'da country_id/operator_id NULL olur
  -- ve Postgres varsayılanı NULL'ları farklı sayar -> sınırsız çift satır
  UNIQUE NULLS NOT DISTINCT (kind, service_id, country_id, operator_id, duration_minutes,
                             verification_type, dimension_a_id),
  verification_type verification_type_t NOT NULL DEFAULT 'sms',  -- 'sms' | 'call'
  --   ⚠️ AYRI fiyat/stok ekseni: offers'ta path segmenti, aynı servis+ülke için farklı fiyat
  dimension_a_id BIGINT   -- genel ek boyut (e-posta ürününde 'domain'; SMS'te NULL)
)

providers (
  id, name UNIQUE,
  protocol provider_protocol_t NOT NULL,        -- SMS_ACTIVATE | FIVE_SIM  <- ADAPTÖR SEÇİMİ BURADAN
  base_url TEXT,
  api_key_enc BYTEA,                            -- AES-GCM ile şifreli
  is_active BOOLEAN DEFAULT false,
  priority INT DEFAULT 100,                     -- eşit fiyatta tercih sırası
  cost_multiplier NUMERIC(6,4) DEFAULT 1.0,     -- sağlayıcıya özel maliyet düzeltmesi
  cost_currency currency_t DEFAULT 'USD',
  account_balance_minor BIGINT DEFAULT 0,       -- SAĞLAYICIDAKİ bakiyemiz — MARJ DEĞİL
  capabilities JSONB DEFAULT '[]'               -- desteklenen product_kind listesi
)

provider_dimension_maps (                       -- ⭐ tek eşleştirme tablosu
  provider_id BIGINT, dimension TEXT,           -- 'service' | 'country' | 'operator' | ...
  local_id BIGINT, remote_code TEXT,
  PRIMARY KEY (provider_id, dimension, local_id),
  UNIQUE (provider_id, dimension, remote_code)
)

provider_offers (                               -- fiyat/stok önbelleği (kaynak doğruluk API'dir)
  provider_id BIGINT, product_id BIGINT,
  cost_minor BIGINT, cost_currency currency_t,
  stock INT, is_available BOOLEAN,
  synced_at TIMESTAMPTZ,
  PRIMARY KEY (provider_id, product_id)
)

-- ─────────── FİYATLANDIRMA ───────────
pricing_rules (                                 -- marj koda gömülmez
  id, scope pricing_scope_t,                    -- GLOBAL|SERVICE|COUNTRY|SERVICE_COUNTRY|PRODUCT
  service_id, country_id, product_id,           -- kapsama göre dolu
  margin_percent NUMERIC(6,2) NOT NULL,
  fixed_fee_minor BIGINT DEFAULT 0,
  min_price_minor BIGINT DEFAULT 0,
  is_active BOOLEAN, valid_from, valid_to
)
fx_rates (id, base currency_t, quote currency_t, rate NUMERIC(14,6), source, fetched_at)

price_quotes (                                  -- "gösterilen fiyat = tahsil edilen fiyat" güvencesi
  id BIGSERIAL PK, public_id UUID UNIQUE,
  user_id, product_id, provider_id,
  cost_minor BIGINT, cost_currency, fx_rate NUMERIC(14,6),
  sell_price_minor BIGINT NOT NULL,             -- TRY kuruş — SÖZLEŞME
  pricing_rule_id BIGINT,
  expires_at TIMESTAMPTZ NOT NULL,              -- now() + 120 sn
  consumed_at TIMESTAMPTZ                       -- tek kullanımlık
)

-- ─────────── SİPARİŞ ───────────
orders (                                        -- TÜM ürün tipleri için ortak
  id BIGSERIAL PK, public_id UUID UNIQUE,
  user_id, product_id, provider_id, quote_id,
  kind product_kind_t NOT NULL,
  remote_order_id TEXT NOT NULL,
  provider_activation_id BIGINT,     -- webhook YALNIZ activationId taşır -> UNIQUE + indeks
  identifier TEXT,                   -- telefon numarası veya e-posta adresi (genelleştirildi)
  verification_type verification_type_t NOT NULL DEFAULT 'sms',
  subtype SMALLINT NOT NULL DEFAULT 1,   -- 1=aktivasyon, 2=kiralama (v1.1 geriye dönük tutarlılık)
  provider_refund_status refund_status_t DEFAULT 'PENDING',
  provider_closed_at TIMESTAMPTZ,    -- FR-412: sağlayıcıda DELETE/finish yapıldı mı
  status order_status_t NOT NULL,               -- PENDING|COMPLETED|CANCELLED|REFUNDED|FAILED
  cost_micro BIGINT, cost_currency,             -- ⚠️ MİKRO-birim (6 hane) — ADR-026
  price_paid_minor BIGINT,                      -- TRY kuruş — tahsil edilen
  fx_rate NUMERIC(14,6),                        -- marj raporu için
  expires_at TIMESTAMPTZ,
  completed_at, cancelled_at, refunded_at,
  attributes JSONB DEFAULT '{}',
  UNIQUE (provider_id, remote_order_id),        -- çift kayıt DB düzeyinde imkânsız
  UNIQUE (provider_id, provider_activation_id)  -- webhook korelasyonu
)
CREATE INDEX ON orders (user_id, created_at DESC);
CREATE INDEX ON orders (status, expires_at) WHERE status = 'PENDING';   -- yoklama işi için

order_messages (                                -- aktivasyon: N mesaj (1 değil!), kiralık: N
  id, order_id, sender TEXT, body TEXT, code TEXT, received_at,
  provider_otp_id TEXT,                         -- webhook dedup anahtarı
  UNIQUE (order_id, provider_otp_id)
)
-- 🔴 KALICI OLMALI: sonlandırılmış/iade edilmiş aktivasyondan geçmiş mesaj OKUNAMAZ
--    (409 ACTIVATION_NOT_ACTIVE: "terminated/refunded and Otp cannot be retrieved").
--    Sağlayıcı bir arşiv değildir. -> FR-415
rental_details (                                -- v1.1 hazır, v1'de boş
  order_id PK, rent_start, rent_end, auto_renew BOOLEAN, renewed_count INT
)

-- ─────────── ÖDEME / BAKİYE YÜKLEME ───────────
deposit_methods (id, code, name, kind, config JSONB, is_active, sort_order)
   -- kind: BANK_TRANSFER | CRYPTO ;  config: {iban, receiver} veya {network, address}

deposits (
  id, public_id UUID UNIQUE, user_id,
  method_id BIGINT REFERENCES deposit_methods(id),
  amount_minor BIGINT, currency currency_t,     -- kullanıcının beyan ettiği
  credited_minor BIGINT,                        -- TRY olarak hesaba geçen
  fx_rate NUMERIC(14,6),                        -- USDT ise dönüşüm kuru
  status deposit_status_t,                      -- PENDING|COMPLETED|REJECTED|REFUNDED
  receipt_path TEXT,                            -- havale dekontu
  tx_hash TEXT, network TEXT,                   -- kripto
  reviewed_by_user_id, reviewed_at, rejection_reason,
  idempotency_key TEXT UNIQUE
)

-- ─────────── DESTEK / DENETİM ───────────
tickets (id, public_id, user_id, subject, priority, status, last_reply_at)
ticket_messages (id, ticket_id, user_id, body, is_staff, attachments JSONB)
audit_logs (id, actor_user_id, action, entity_type, entity_id, before JSONB, after JSONB, ip, user_agent, created_at)
sessions (id TEXT PK, user_id, ip, user_agent, created_at, last_seen_at, expires_at)  -- Redis'te; DB'de yalnız liste
referrals / referral_rules                      -- v1.1 hazır
```

### 6.1 Mevcut şemadan farklar

| Değişiklik | Gerekçe |
|---|---|
| `products` + `provider_dimension_maps` + `provider_offers` | Yeni ürün tipi eklemek tablo cerrahisi olmaktan çıkar (§4) |
| İki eşleştirme tablosu → tek `provider_dimension_maps` | `ProviderCountryMapping` + `ProviderPlatformMapping` birleşti |
| `services.code` eklendi | Mevcut kod `where: { service_code }` sorguluyor ama sütun **yok** → sorgular patlıyor |
| `providers.protocol` eklendi | Mevcut sistem `name.toLowerCase().includes('hero')` ile adaptör seçiyor; sağlayıcının adını değiştirmek sistemi sessizce bozar |
| `providers.account_balance_minor` ↔ `cost_multiplier` ayrıldı | Mevcut sistemde `balance` hem "sağlayıcıdaki bakiyemiz" hem "yüzde marj" olarak kullanılıyor (`memory.md` §3.4) |
| `pricing_rules` | Marj bir iş kararıdır, dağıtım gerektirmemeli |
| `price_quotes` | Fiyat garantisi + istemciden gelen `provider_id`'ye güvenmeme, tek hamlede |
| `ledger_entries` | Denetlenebilirlik, mutabakat, iade takibi |
| `audit_logs` | Ticari zorunluluk: admin kimin bakiyesini ne zaman değiştirdi |
| `UNIQUE (provider_id, remote_order_id)` | Aynı sağlayıcı siparişinin iki kez kaydedilmesi DB'de imkânsız |
| Tüm para alanları `BIGINT` (kuruş) | Float aritmetiği kaldırıldı |
| `public_id UUID` | Sıralı sayısal id'ler sayaç sızdırır, IDOR denemesini kolaylaştırır |
| Tutarlı `snake_case`, çoğul tablo adı | Mevcut şemada `user_id` / `userId` karışık, `Tickets` / `payments` karışık |
| `provider_activation_id` UNIQUE | Webhook yalnız `activationId` taşır — telefon, durum, `verificationType` **taşımaz** |
| `identifier` (eski `phone_number`) | E-posta ürününde adres; telefon-merkezli isim genelleştirildi |
| `verification_type` + `subtype` sütunları | `sms`/`call` ayrı fiyat ekseni; `subtype` v1.1 kiralık için geriye dönük tutarlılık |
| `cost_micro` (6 hane) | Sağlayıcı fiyatları 4 ondalıklı, `MaxPrice.minimum = 0.0067` — sent'e sığmaz (ADR-026) |
| `UNIQUE NULLS NOT DISTINCT` | E-posta ürününde `country_id`/`operator_id` NULL; Postgres varsayılanı çift satıra izin verirdi |
| `dimension_a_id` | E-posta `domain`'i **fiyat/stok taşıyan birincil seçim ekseni**; JSONB'ye koymak §4 kuralını ihlal ederdi |
| `provider_closed_at` | FR-412: her sipariş sağlayıcıda kapatılmalı |

---

## 7. Durum makineleri

Durum geçişleri kodda dağılmaz; tek yerde tanımlanır ve geçiş fonksiyonu dışında `status` alanına yazılmaz.

### 7.1 Sipariş (orders)

```
                    ┌──────────────┐
                    │   PENDING    │  numara alındı, kod bekleniyor
                    └──┬───┬───┬───┘
         kod geldi ────┘   │   └──── kullanıcı iptali / süre doldu
              ▼            │                        ▼
        ┌───────────┐      │                 ┌─────────────┐
        │ COMPLETED │      │                 │  CANCELLED  │
        └───────────┘      │                 └──────┬──────┘
         (terminal)   sağlayıcı hatası               │ iade işlendi
                           ▼                         ▼
                    ┌───────────┐  otomatik   ┌───────────┐
                    │  FAILED   │─── iade ───▶│ REFUNDED  │ (terminal)
                    └───────────┘             └───────────┘
```

| Geçiş | Tetikleyici | Yan etki |
|---|---|---|
| `→ PENDING` | Satın alma başarılı | Bakiye düşüldü (T1'de), sipariş kaydı (T2) |
| `PENDING → COMPLETED` | Sağlayıcıdan kod geldi | SSE olayı yayınla; para hareketi yok |
| `PENDING → CANCELLED` | Kullanıcı iptali veya `expires_at` geçti | Sağlayıcıya iptal bildir |
| `CANCELLED → REFUNDED` | İade ledger kaydı yazıldı | `REFUND`, tam `price_paid_minor` kadar |
| `PENDING → FAILED` | Sağlayıcı kalıcı hata | Otomatik `REFUND` |

**Kurallar:**
- 🔴 **Sağlayıcının `status` kodları bizim durum makinemize BAĞLANMAZ.** `ActivationStatusTypes`
  `[1,2,3,4,6,7,8,10]` için `x-enum-descriptions` yok; 1,2,3,4,7'nin anlamı bilinmiyor.
  Eşleme tek bir savunmacı fonksiyonda; bilinmeyen değer `PENDING` korur. (ADR-025)
- 🔴 **Terminal duruma geçiş sağlayıcıyı da kapatır** (FR-412): kod geldiyse `Finish()`,
  gelmediyse `Cancel()`. "Bırak süresi dolsun" **yasaktır** — süresi dolan aktivasyonun akıbeti
  spec'te tanımsız (❓H10).
- ⚠️ **v1.1 uyarısı:** `COMPLETED` terminal olması **kiralıkta kırılır** — kiralık numara ilk
  mesajdan sonra da aktiftir ve `prolong` terminal durumdan sonra gerçekleşir.
- Terminal durumdan çıkış yoktur. `COMPLETED` bir siparişe iade yapılamaz — kodda **ve** `CHECK`/tetikleyici ile korunur.
- İade **her zaman** `price_paid_minor` kadardır; anlık kurla yeniden hesaplanmaz.
- Süre dolumu istemciye güvenilerek değil, sunucudaki `expires_at` ile belirlenir.

### 7.2 Bakiye yükleme (deposits)

```
PENDING ──admin onayı──▶ COMPLETED ──itiraz/hata──▶ REFUNDED
   └────admin reddi────▶ REJECTED
```

`PENDING → COMPLETED` geçişi **idempotent** olmak zorundadır: aynı `idempotency_key` ile ikinci çağrı
bakiyeyi ikinci kez artırmaz. *Mevcut sistemdeki `approvePayment` bu kontrolü transaction dışında yaptığı
için yarış durumuna açıktır.*

---

## 8. Ana akışlar

### 8.1 Fiyat sorgusu (teklif alma)

```
GET /api/v1/catalog/quote?serviceId=..&countryId=..&operator=any
   │
   ├─ 1. Redis'ten USD/TRY kuru oku (TTL 5 dk). Yoksa fx_rates tablosundaki son değer.
   │     Kur 30 dakikadan eskiyse → 503, SATIŞ DURUR (yanlış kurla satmaktansa durmak doğrudur)
   │
   ├─ 2. Bu ürün için teklifi olan AKTİF sağlayıcıları bul (provider_dimension_maps + capabilities)
   │
   ├─ 3. Hepsine PARALEL fiyat/stok sor  (errgroup, 3 sn zaman aşımı, devre kesici)
   │     Yanıt vermeyen sağlayıcı elenir; sistem yavaş sağlayıcıyı beklemez
   │     ⚠️ Canlı offers sorgusu 429'a TABİDİR (spec'te 429 yalnız bu uç noktada tanımlı)
   │        -> provider_offers önbelleği + Retry-After uyumlu geri çekilme zorunlu
   │
   ├─ 4. stock > 0 olanlar arasında (cost × cost_multiplier) en düşüğü seç
   │     ✅ stok = counts.defaultPrice  (total DEĞİL: fiyat tavanı yok. physical DEĞİL:
   │        ayrı eksen, satışı sessizce engelliyordu — H16 kapandı, 2026-09-10)
   │     ⚠️ hangi fiyat katmanı (prices.default | retail | min) ücretlendirilir: BİLİNMİYOR (H6)
   │        maxPrice'ı default'a sabitlersek görünen stok counts.defaultPrice kadardır
   │     eşitlikte: priority, ayrıca offers.meta.order.deliverability ve stats.percent
   │        (düşük başarı oranı = yüksek iade = zarar)
   │
   ├─ 5. En spesifik pricing_rule uygula:
   │        PRODUCT > SERVICE_COUNTRY > SERVICE > COUNTRY > GLOBAL
   │        sell = ceil(cost_usd × multiplier × fx × (1+margin/100)) + fixed_fee
   │        sell = max(sell, min_price)           -- yuvarlama HER ZAMAN yukarı
   │
   ├─ 6. price_quotes kaydı oluştur (expires_at = now + 120 sn)
   │
   ▼
{ "quoteId": "uuid", "price": {"minor": 1250, "currency": "TRY", "formatted": "12,50 ₺"},
  "stock": 43, "expiresAt": "..." }
```

> ⭐ **Kritik:** İstemciye `providerId` ve `cost` **gönderilmez**. Yalnız `quoteId`. Bu, mevcut sistemdeki
> "kullanıcı istediği sağlayıcıyı ve fiyatı gövdede gönderebiliyor" açığını **yapısal olarak** kapatır.

### 8.2 Satın alma

```
POST /api/v1/orders   { "quoteId": "uuid" }     ← gövdede BAŞKA hiçbir fiyat/sağlayıcı bilgisi YOK
   │
   ├─ T1 (kısa transaction):
   │    a. price_quotes satırını SELECT ... FOR UPDATE ile kilitle
   │    b. Doğrula: var mı · bu kullanıcıya mı ait · süresi geçmiş mi · tüketilmiş mi  → değilse 409
   │    c. consumed_at = now()                       (tek kullanımlık)
   │    d. WalletRepo.ApplyEntry(PURCHASE, -sell_price, key = "order:"+quoteId)
   │    COMMIT
   │
   ├─ Sağlayıcı çağrısı (transaction DIŞINDA, 10 sn zaman aşımı, YENİDEN DENEME YOK — idempotent değil):
   │    providerPort.Purchase(ctx, offer)
   │      ⭐ maxPrice = teklifteki maliyet gönderilir (fixedPrice GÖNDERİLMEZ — ADR-027)
   │        Fiyat yükseldiyse satın alma gerçekleşmez → beklenmedik tahsilat
   │        sağlayıcı sınırında engellenir (ADR-018)
   │      ⚠️ Hangi hata koduyla geldiği modern uçta SPEC'TE YOK (WRONG_MAX_PRICE
   │        yalnız legacy'de belgeli) → adaptör 422/404 gövdesini ayrıştırır (H11/H12)
   │      ⭐ Yanıt DİZİ: data[0]; len != 1 → hata
   │
   ├─ T2 (kısa transaction):
   │    başarılı  → orders satırı oluştur (PENDING, expires_at = now + ttl)
   │    başarısız → WalletRepo.ApplyEntry(REFUND, +sell_price, key = "order:"+quoteId+":refund")
   │    COMMIT
   │
   ▼
{ "orderId": "uuid", "phoneNumber": "+90...", "expiresAt": "...", "balance": {...} }
```

T1–T2 arasında süreç ölürse `orphan-hold-reaper` (60 sn'de bir) devreye girer.

### 8.3 Kod bekleme — sağlayıcı webhook'u → SSE

> **Güncellendi (2026-09-08):** HeroSMS bir `sms-incoming` **webhook**'u sağlıyor
> ([provider-herosms.md](provider-herosms.md) §7.1). Bu, yoklamayı birincil mekanizma olmaktan
> çıkarıp güvenlik ağına dönüştürür. Zincir üç halkalıdır:

```
HeroSMS ──webhook──▶ Go API ──Redis Pub/Sub──▶ SSE ──▶ Tarayıcı
   (push)              (teyit)                    (push)
```

**1) Sağlayıcı webhook'u alınır**

```
POST /api/v1/webhooks/herosms/<128-bit tahmin edilemez segment>
{ "activationId": 123456, "phoneFrom": "89854", "service": "wa",
  "text": "Your code is 12345", "code": "12345", "country": 62,
  "receivedAt": "2026-09-08T10:15:30Z" }
```

> 🔴 **Webhook verisine ASLA doğrudan güvenilmez.** HeroSMS imza (HMAC) doğrulaması sunmuyor —
> spec'te hiçbir imza başlığı yok. Webhook URL'ini bilen herkes sahte "kod geldi" bildirimi
> gönderebilir. Bu yüzden webhook yalnız bir **tetikleyicidir**:
>
> ```
> [handler — 3 SANİYE bütçe]
>   → kaynak IP izin listesinde mi?   (84.32.223.53 / 185.138.88.87, gerçek peer IP)
>   → ham gövdeyi asynq kuyruğuna at
>   → ANINDA 200 dön                                        (ADR-024)
>
> [webhook-ingest işçisi — asenkron]
>   → dedup: (activationId, id) görüldü mü?                 (aynı SMS ≥8 kez gelebilir)
>   → activationId bizim siparişimiz mi?  değilse sessizce bırak + log
>   → GET /activations/{id}/otp/last ile SAĞLAYICIDAN TEYİT ← gerçek kod buradan  (ADR-022)
>   → teyit BOŞ dönerse durum DEĞİŞTİRİLMEZ, sipariş PENDING kalır
>   → kodu kendi regex'imizle çıkar (code alanı gelmeyebilir)
>   → siparişi güncelle → Redis Pub/Sub'a yayınla
> ```
>
> 🔴 **`GET /activations/{id}` diye bir uç nokta YOKTUR.** O yolda yalnız `delete` tanımlıdır
> (`jq '.paths["/activations/{activationId}"] | keys'` → `["delete"]`). Teyit `/otp/last` ile yapılır.
>
> Ayrıntı ve karşı önlemler: [provider-herosms.md](provider-herosms.md) §7.1 · ADR-019, ADR-022, ADR-024

**2) SSE ile tarayıcıya iletilir**

```
Tarayıcı:  const es = new EventSource('/api/v1/orders/{id}/stream')

Go tarafı:  GET /api/v1/orders/:id/stream
   ├─ sahiplik doğrula (WHERE public_id = $1 AND user_id = $2)
   ├─ Content-Type: text/event-stream, Cache-Control: no-cache, no-transform
   ├─ X-Accel-Buffering: no   +  her yazımda Flusher.Flush()
   ├─ Redis Pub/Sub kanalına abone ol: "order:{id}"
   ├─ 20 sn'de bir ": keepalive" gönder (vekil zaman aşımını önler)
   └─ context iptalinde temizle
```

**Webhook işleyicisinin uyması zorunlu kurallar**

| # | Kural | Neden |
|---|---|---|
| 1 | Handler **3 sn içinde 200 döner**; DB yazımı, teyit çağrısı ve Redis publish **kuyrukta** yapılır | Sağlayıcı zaman aşımı 3 sn; aşılırsa ≥7 tekrar |
| 2 | **At-least-once:** aynı SMS **en az 8 kez** gelebilir → işleyici idempotent | *"En az 7 kez"* yeniden deneme |
| 3 | **Kaynak IP izin listesi** (`84.32.223.53`, `185.138.88.87`), gerçek peer IP — `X-Forwarded-For` değil. IP'ler env'de, koda gömülmez | İmza doğrulaması yok |
| 4 | **Dedup anahtarı:** `(activationId, id)`; `id` yoksa `SHA-256(activationId+receivedAt+text)` | `id` spec'te `required` ama `properties`'te **tanımsız** |
| 5 | **Kendi OTP regex'imiz** — `code` gelmeyebilir, `text` nullable. Kod çıkarılamazsa sipariş `PENDING` kalır, **asla `COMPLETED` olmaz** | `code` webhook'ta `required` değil |
| 6 | **`verificationType: "call"`** dalında `code`/`text` null olabilir → **"kod gelmedi" sayılmaz** | Aksi halde hatalı iade |
| 7 | **İlk koddan sonra SSE kapanmaz** — `otpList` dizidir | İkinci/üçüncü mesaj gelebilir |
| 8 | Tanımadığı `activationId` → sessizce `200` + log | 3 URL slotu **hesap geneli**; prod/staging aynı hesaptaysa tüm olayları alır |

**3) Güvenlik ağı: yoklama (polling)**

`order-poller` işi kaldırılmaz, **sıklığı düşürülür (5 sn → 30 sn)** ve yalnız webhook'u gelmemiş
`PENDING` siparişleri **`GET /activations` ile toplu** (`size` max **25** → 100 sipariş = 4 sayfa) yoklar. Webhook kaybolursa, geç kalırsa veya HeroSMS panelindeki URL yanlış
ayarlanmışsa sistem yine çalışır — sadece 30 saniyeye kadar gecikir.

> **Neden üç halka:** Webhook hızlıdır ama teslim garantisi yoktur ve kaydı elle yapılır
> (API'de webhook yönetim uç noktası **yok** — [provider-herosms.md](provider-herosms.md) §7.1).
> Yoklama yavaştır ama güvenilirdir. İkisi birlikte hem hızlı hem dayanıklıdır.
> Tek başına yoklama seçilseydi sağlayıcıya giden istek sayısı kullanıcı sayısıyla
> orantılı büyürdü — mevcut prototipin sorunu tam olarak buydu.

**İstemci tarafı yedek:** `EventSource` çalışmazsa istemci 5 sn aralıklı `GET /orders/:id` yoklamasına
düşer. Kurallar: [frontend-contract.md](frontend-contract.md) §4.

### 8.4 İptal ve iade

```
POST /api/v1/orders/:id/cancel
  ├─ sahiplik + durum kontrolü (yalnız PENDING iptal edilebilir)
  ├─ sağlayıcıya iptal bildir — BAŞARISIZ OLSA BİLE DEVAM ET (kullanıcı beklememeli)
  ├─ T: status=CANCELLED → ApplyEntry(REFUND, +price_paid) → status=REFUNDED
  └─ sağlayıcı iptali başarısızsa attributes.provider_cancel_pending=true; ayrı iş takip eder
```

**Ürün ilkesi 1 gereği:** Sağlayıcı bize iade etmese bile kullanıcıya iade ederiz. Karşılanmayan iadeler
gider kalemi olarak raporlanır.

### 8.5 Bakiye yükleme

**Havale:** Kullanıcı tutar + dekont yükler → `deposits(PENDING)` → admin inceler →
onay: tek transaction içinde `deposits.status=COMPLETED` + `ApplyEntry(DEPOSIT, +amount, key=deposit:{id})` → audit log.

**USDT:** Kullanıcı ağ (TRC20/ERC20) seçer, gönderim yapar, **TX hash** girer → `deposits(PENDING)` →
admin blok gezgininden doğrular → onay anında USDT/TRY kuru `fx_rate` alanına yazılır ve TL karşılığı
`credited_minor` olarak hesaba geçer.

> Her iki akışta da onay **idempotenttir** ve **yalnız `deposits:approve` iznine sahip kullanıcı**
> tarafından, **POST** ile yapılır. *Mevcut sistemde bu bir GET ve yetki kontrolü yok.*

---

## 9. Sağlayıcı adaptör mimarisi

Sistemin genişleyebilirliğinin tamamı bu arayüze dayanır.

```go
// internal/port/provider.go
type ProviderPort interface {
    Protocol() ProviderProtocol
    Capabilities() []ProductKind                 // hangi ürün tiplerini destekliyor

    ListCountries(ctx context.Context, c Creds) ([]RemoteDimension, error)
    ListServices(ctx context.Context, c Creds) ([]RemoteDimension, error)
    GetPriceAndStock(ctx context.Context, c Creds, q PriceQuery) (*PriceResult, error)

    Purchase(ctx context.Context, c Creds, cmd PurchaseCmd) (*PurchaseResult, error)
    GetStatus(ctx context.Context, c Creds, remoteOrderID string) (*RemoteStatus, error)

    // ⭐ İKİ AYRI KAPATMA — aynı şey değiller (ADR-023)
    Cancel(ctx context.Context, c Creds, remoteOrderID string) error  // iade TALEP eder
    Finish(ctx context.Context, c Creds, remoteOrderID string) error  // iade YOK, başarıyla kapatır

    GetBalance(ctx context.Context, c Creds) (money.Money, error)
    ListMessages(ctx context.Context, c Creds, remoteOrderID string) ([]RemoteMessage, error)

    // yalnız SMS_RENTAL yeteneği olanlar — v1.1
    // ⚠️ time.Duration DEĞİL: sağlayıcı sabit enum kabul ediyor (24|72|168|... saat)
    Extend(ctx context.Context, c Creds, remoteOrderID string, hours RentDurationHours) error
}
```

**Kurallar:**
- **Adaptörün görevi çeviridir, karar vermek değil.** `"NO_NUMBERS"`, `"STATUS_WAIT_CODE"`,
  `{"activationId": ...}` gibi ham yanıtları normalize tiplere çevirir; hataları tipli hatalara
  haritalar: `ErrOutOfStock`, `ErrProviderAuth`, `ErrProviderTimeout`, `ErrProviderUnavailable`.
- **Adaptör seçimi `providers.protocol` alanından yapılır**, isimden değil.
- Her adaptör için **kaydedilmiş gerçek API yanıtlarıyla** (`api/testdata/fixtures/`) sözleşme testi yazılır.
  Sağlayıcı formatı değiştirirse test kırılır — sessizce bozulmaz.
- Tüm dış çağrılar ortak HTTP istemcisinden geçer: zaman aşımı, **yalnız idempotent işlemlerde** sınırlı
  yeniden deneme, **devre kesici**, sağlayıcı bazlı metrik.
- `Purchase` **asla yeniden denenmez** — idempotent değildir, iki numara alınmasına yol açar.

**Yeni sağlayıcı ekleme maliyeti hedefi: 1 gün.** Adım listesi `CLAUDE.md`'de reçete olarak durur.

### 9.1 HeroSMS (v1'in tek sağlayıcısı) — doğrulanmış

Protokol değeri: `HEROSMS_V1`. Tam entegrasyon referansı: **[provider-herosms.md](provider-herosms.md)**
(resmî OpenAPI 3.2.0 dokümanı + canlı doğrulama, 2026-09-08).

Adaptörü doğrudan etkileyen dört doğrulanmış kural:

| Kural | Neden |
|---|---|
| **Teyit `GET /{id}/otp/last`** | 🔴 Tekil `GET /activations/{id}` **YOK** — o yolda yalnız `delete` (ADR-022) |
| **`Cancel()` ve `Finish()` ayrı** | `DELETE` = *"iptal ve iade"*, `/finish` = *"para iadesi yapılmaz"* (ADR-023) |
| **Yanıt DİZİ:** `data[0]` okunur | `amount:1` olsa bile dizi. `len(data) != 1` → **hata** (parası çekilmiş ama siparişe dönmemiş numara) |
| Stok = `counts.defaultPrice` | Ölçüm (20.788 kombinasyon): `defaultPrice` = `map` merdiveninin `prices.default` altındaki kümülatif toplamı, **birebir**. `maxPrice` politikamızla aynı şeyi ölçer. `physical` ölçütken katalogun %28'i sessizce kapalıydı |
| `maxPrice` gönderilir, **`fixedPrice` GÖNDERİLMEZ** | `fixedPrice` = *"kesinlikle o fiyattan"* — tavan değil sabit fiyat (ADR-027) |
| TTL `expiredAt`'ten, **eksi güvenlik payı** | Varsayılan süre spec'te hiçbir yerde yok |
| **OTP alan adı normalizasyonu** | Üç kaynak, üç isim seti: `smsCode\|code` → `Code`, `smsText\|text` → `Body`, `receivedAt\|date` → `ReceivedAt`. İki tarih formatı (ISO8601 / RFC3339) |
| **Telefon esnek unmarshal** | Şema `integer` (`79991234567`) ama örnekler maskeli `string` (`"79********1"`) |
| **Legacy'de `StatusCode==200 → success` YASAK** | `NO_KEY`, `BAD_KEY`, `NO_NUMBERS`, `ERROR_SQL` hepsi **200 gövdesinde** düz metin |
| **Devre kesici yüzey bazında ayrı** | Legacy stub (katalog) çökünce modern satın alma kapanmamalı (ADR-028) |
| **İki limit ayrı:** hız (429, yalnız `offers`) ve eşzamanlılık (`403 CHANNELS_LIMIT` → semafor) | Farklı mekanizmalar |
| **`BANNED.info.scope`** | `global` → sağlayıcıyı kapat; `specific` → yalnız o (servis,ülke) çiftini kara listeye al |
| Sağlayıcı `status`'ü bizim durum makinemize **bağlanmaz** | `[1,2,3,4,6,7,8,10]` için açıklama yok; bilinmeyen → `PENDING` korunur (ADR-025) |
| `Purchase` asla yeniden denenmez | Idempotency yok **+ batch** → 10 numaraya kadar çift alım |
| `resellerUserId` = anlamsız kimlik (ULID) | ⚠️ Yanıtta geri **dönmüyor**; KVKK: e-posta/kullanıcı adı **asla** gönderilmez |

---

## 10. Kimlik ve yetkilendirme

### ADR-003: JWT yerine sunucu tarafı oturum

Mevcut sistem httpOnly çerezde JWT taşıyor. Sorunları: iptal edilemiyor (kullanıcı yasaklandığında token
1 saat daha geçerli), token ömrü (1 sa) ile çerez ömrü (24 sa) uyumsuz → kullanıcı girişte gibi görünüp
yönlendirme döngüsüne giriyor, rol bilgisi token'da bayatlıyor.

**Karar:** Redis'te saklanan opak oturum kimliği.

```
Set-Cookie: sid=<32 bayt rastgele>; HttpOnly; Secure; SameSite=Lax; Path=/; Max-Age=2592000
```

Tek alan adı + ters vekil topolojisi sayesinde CORS ve çapraz alan çerez sorunu **yoktur**.

- Anında iptal edilebilir (yasaklama, şifre değişimi → tüm oturumları düşür)
- Rol/izin her istekte taze okunur
- Kullanıcı "aktif oturumlarım" listesini görür ve uzaktan sonlandırabilir

Bedeli: her istekte bir Redis okuması (<1 ms). Zaten hız limiti için Redis'e gidiyoruz.

### Yetkilendirme — iki katman, ikisi de zorunlu

1. **İzin kontrolü (RBAC):** `RequirePermission("deposits:approve")` middleware'i.
   İzinler **veridir**, kod değil — yeni rol eklemek dağıtım gerektirmez.
2. **Sahiplik kontrolü:** Kaynağa erişim her zaman `WHERE ... AND user_id = $current` ile **sorgunun
   parçası** olarak yapılır; ayrı bir `if` değil. Böylece unutulması yapısal olarak zorlaşır.
   Depo katmanında kullanıcı kapsamı olmayan metot yazılmaz: `FindOrderForUser(ctx, orderID, userID)`.

> Mevcut sistemdeki üç kritik açık (ödeme onayı, profil güncelleme, sipariş sorgulama) tam olarak bu
> iki katmanın eksikliğinden doğuyor.

**Ek korumalar:**

| Konu | Karar |
|---|---|
| Durum değiştiren işlemler | Her zaman `POST`/`PATCH`/`DELETE` — **asla GET**. *(Mevcut sistemde ödeme onayı bir GET; bir `<img>` etiketiyle tetiklenebilir.)* |
| CSRF | `SameSite=Lax` + durum değiştiren isteklerde çift gönderim token'ı |
| Hız limiti | `/auth/*` (IP + hesap, kademeli gecikme), teklif sorgusu, sipariş oluşturma için ayrı limitler |
| Şifre | **argon2id** (bcrypt'ten geçiş), tek kütüphane |
| Oturum sabitleme | Girişte oturum kimliği yenilenir |
| reCAPTCHA | Kayıt + giriş. **Anahtar yoksa uygulama açılışta panic eder** *(mevcut sistemde anahtar eksik olduğu için giriş sessizce hep başarısız)* |
| E-posta doğrulama | Zorunlu; doğrulanmamış hesap satın alma yapamaz |

---

## 11. Hata taksonomisi

```
AppError
├── DomainError            (400/409) — iş kuralı ihlali, kullanıcıya gösterilir
│   ├── ErrInsufficientBalance · ErrQuoteExpired · ErrQuoteConsumed
│   ├── ErrInvalidStateTransition · ErrOutOfStock
├── AuthError              (401/403)
├── NotFoundError          (404)
├── RateLimitError         (429)
└── InfraError             (502/503) — kullanıcıya genel mesaj, log'a tam detay
    ├── ErrProviderUnavailable · ErrProviderAuth · ErrProviderNoBalance
    ├── ErrProviderRetryAfter(d)      ← 429 RATE_LIMIT · 425 TOO_EARLY · info.retry_after_seconds
    ├── ErrProviderConcurrencyLimit   ← 403 CHANNELS_LIMIT (info.max_allowed → semafor)
    ├── ErrProviderBanned(scope,until)← 403 BANNED (scope: global | specific)
    ├── ErrProviderOrderClosed        ← 409 ACTIVATION_NOT_ACTIVE
    └── ErrFxUnavailable · ErrMailer
```

> ⚠️ **Sağlayıcı hata zarfının alan adı `title`'dır, `code` değil:**
> `BaseErrorResponse = {title, details, info?}`. 34 farklı `title`; yalnız 8'i `enum` ile sabit,
> gerisi **sadece `examples` içinde** = sözleşme değil. Adaptör bilinmeyen `title`'ı
> `ErrProviderUnavailable`'a düşürür, panic etmez.

> **FR-414:** `Retry-After` başlığı veya `info.retry_after_seconds` varsa **sabit backoff
> kullanılmaz** — sağlayıcının verdiği süre beklenir.

**Kural:** Kullanıcıya asla ham hata mesajı gösterilmez.
*(Mevcut global handler `res.send("<h1>Hata!</h1><p>" + err.message + "</p>")` yapıyor — hem yansımalı
XSS hem bilgi sızıntısı.)*

Yanıt formatı sabit:
```json
{ "error": { "code": "INSUFFICIENT_BALANCE",
             "message": "Bakiyeniz yetersiz.",
             "requestId": "01JB2X..." } }
```
`requestId` her yanıtta döner ve log'da aynı değerle aranabilir — destek süreci için kritik.

---

## 12. Gözlemlenebilirlik

| Katman | Araç | Ne izlenir |
|---|---|---|
| Log | `log/slog` JSON + `request_id` | Her istek: yol, süre, durum, kullanıcı. Her sağlayıcı çağrısı: sağlayıcı, işlem, süre, sonuç |
| Hata | Sentry | İstisnalar, `request_id` ile ilişkili |
| Metrik | Prometheus `/metrics` | `orders_total{status}` · `provider_request_duration_seconds{provider,op}` · `provider_error_rate` · `fx_rate_age_seconds` · `ledger_drift_minor` · `sse_active_connections` |
| Denetim | `audit_logs` | Bakiye/rol/onay/sağlayıcı değişiklikleri — kim, ne zaman, öncesi/sonrası |
| Sağlık | `/healthz` (canlılık) · `/readyz` (DB+Redis) | Dağıtım ve izleme |

**Alarm eşikleri (v1):**
- Sağlayıcı hata oranı > %20 (5 dk penceresi)
- **Mutabakat sapması ≠ 0** (en kritik alarm)
- Kur yaşı > 30 dk
- Sipariş başarı oranı < %90 (15 dk)
- 5 dakikadan eski yetim provizyon var

**Log'a asla yazılmaz:** şifre, oturum kimliği, sağlayıcı API anahtarı, tam telefon numarası
(maskeli: `+9053****4521`), SMS kodu, TX hash'in tamamı,
🔴 **legacy sağlayıcı çağrılarının tam URL'i** (API anahtarı sorgu dizesinde gider).

**Ek metrikler:** `provider_webhook_received_total{provider}` ·
`provider_webhook_verify_failed_total` (sahte webhook göstergesi) ·
`provider_rate_limited_total` · `provider_concurrency_denied_total` · `provider_banned_total{scope}`

---

## 13. Arka plan işleri (asynq)

Her iş **idempotent** ve **tek örnek çalışacak şekilde kilitli** (Redis kilidi).

| İş | Sıklık | Görev |
|---|---|---|
| `fx-refresh` | 5 dk | USD/TRY kurunu tazele, `fx_rates`'e yaz |
| `webhook-ingest` | kuyruk | **Yeni.** Webhook gövdesini asenkron işle: dedup `(activationId,id)` → teyit `/{id}/otp/last` → kaydet → Redis yayınla (ADR-024) |
| `order-poller` | **30 sn** | **Güvenlik ağı** (birincil yol webhook, §8.3). Webhook'u gelmemiş `PENDING` siparişleri **`GET /activations`, `size=25` sayfalı** toplu yokla |
| `activation-reaper` | 60 sn | **Yeni.** `provider_closed_at IS NULL` olan terminal siparişleri sağlayıcıda `Cancel()`/`Finish()` ile kapat (FR-412) |
| `order-expirer` | 30 sn | `expires_at` geçmiş siparişleri iptal + iade et |
| `orphan-hold-reaper` | 60 sn | Tüketilmiş ama siparişsiz teklifleri çöz |
| `catalog-sync` | 6 saat | Ülke/servis/operatör listeleri — 🔴 **yalnız legacy'de var**. `getServicesList&lang=tr` ile Türkçe adlar. Ayrıca `custom-durations` (auth'suz, dakika) |
| `offers-sync` | 30 dk | `provider_offers` tazele — tek çağrıda 20.849 kombinasyon (~600 ms). **`verificationType` path segmenti → `sms` ve `call` için İKİ çağrı.** `Retry-After` onurlandıran geri çekilme + devre kesici **zorunlu** (429 yalnız burada tanımlı) |
| `provider-balance-sync` | 15 dk | Sağlayıcılardaki bakiyemizi güncelle; düşükse alarm |
| `provider-refund-retry` | 2 dk | `EARLY_CANCEL_DENIED` alan iptalleri yeniden dene; kalıcı reddi **gider** olarak işaretle ([provider-herosms.md](provider-herosms.md) §6.2) |
| `stats-sync` | Günlük | **Yeni.** `GET /activations/stats?date=` → ülke×servis başarı oranı; sağlayıcı skorlaması + kâr raporu girdisi |
| `ledger-reconcile` | Günlük 04:00 | Mutabakat kontrolü; sapmada alarm. `GET /activations/history` (`from`/`to` **zorunlu**, `size` max 25) `totals{sum,successCount}` verir |
| `session-cleanup` | Günlük | Süresi geçmiş oturum kayıtları |

> Mevcut sistemde `node-cron` bağımlılık listesinde ama **hiç kullanılmıyor**; senkron servisleri de
> hiçbir yerden çağrılmıyor. Katalog verisi elle güncellenmek zorunda.

---

## 14. Ön yüz tasarımı (Next.js)

> ⚠️ **Bu bölüm genel tasarımı anlatır.** Mobil responsive kuralları, tarayıcı uyumluluğu, API
> istemci sözleşmesi ve SSE sertleştirmesi **bağlayıcı** bir ayrı dokümandadır:
> **[frontend-contract.md](frontend-contract.md)** — ön yüz kodu yazmadan önce okunması zorunludur.

### 14.1 Görsel devamlılık

Mevcut "Duralux" koyu admin teması **görsel referanstır**, kod referansı değil. Tailwind ile yeniden
inşa edilir: aynı koyu palet (`#111827` gövde, `#1f2937` yüzey), aynı yerleşim (sol kenar çubuğu +
üst başlık), aynı kart/tablo dili. jQuery, DataTables, Select2, SweetAlert2 **taşınmaz**;
karşılıkları TanStack Table, Radix Select, kendi Dialog bileşenimiz olur.

Tasarım token'ları `web/src/app/globals.css` içinde CSS değişkeni olarak tanımlanır — tek yer.

### 14.2 Rota ve veri stratejisi

| Rota grubu | Render | Veri |
|---|---|---|
| `(public)` | Statik / ISR | Yok |
| `(auth)` | İstemci | POST → `/api/v1/auth/*` |
| `(panel)` | Sunucu bileşeni (ilk yükleme) + istemci (etkileşim) | Sunucuda çerezle fetch; etkileşimde TanStack Query |
| `(admin)` | Aynı, ek izin kontrolü | Sunucuda izin doğrulanır; **istemci kontrolü güvenlik değil, UX'tir** |

`middleware.ts` yalnız oturum çerezinin **varlığını** kontrol eder ve yönlendirir.
**Gerçek yetkilendirme her zaman Go tarafındadır** — Next.js katmanı asla güvenlik sınırı sayılmaz.

### 14.3 Satın alma ekranı

Mevcut `dashboard.ejs`'teki 922 satırlık tek dosya şu bileşenlere bölünür:

```
ServiceGrid          → servis seçimi
CountrySelect        → ülke seçimi (arama + bayrak)
QuotePanel           → useQuote() ile canlı fiyat + geri sayım (120 sn)
PurchaseButton       → POST /orders { quoteId }
CodeWaiter           → useOrderStream() ile SSE; geri sayım, kopyala, iptal
                       🔴 İptal butonu ilk 120 SANİYE PASİF (geri sayım gösterilir)
                          Sağlayıcı EARLY_CANCEL_DENIED (info.minActivationTime: 120) döner;
                          aksi halde sistematik yetim iptal kaydı üretilir ve kâr sızar (FR-416)
                       ⚠️ verificationType "call" dalında kod null gelebilir — ayrı UI
OrderHistoryTable    → TanStack Table
```

> ⚠️ **`POST /activations/{id}/replace` (numara değiştir) v1 kapsamına ALINMAZ** — ücretli mi,
> eski aktivasyon ne olur, iade var mı: **spec'te hiçbir bilgi yok** (gövde yok, açıklama yok).

`useOrderStream` hook'u: SSE bağlantısı, keepalive izleme, üstel geri çekilme ile yeniden bağlanma,
görünürlük/bfcache olaylarında senkronizasyon, `EventSource` çalışmıyorsa yoklamaya düşme — hepsi
tek yerde. Tam sözleşmesi [frontend-contract.md](frontend-contract.md) §4.2'de.

**Tüm HTTP çağrıları tek bir istemci sarmalayıcısından geçer** (`web/src/lib/api/client.ts`);
bileşenler `fetch`'i doğrudan çağırmaz. Zaman aşımı, çerez, CSRF, JSON olmayan yanıt ve hata
normalizasyonu orada tek yerde ele alınır — [frontend-contract.md](frontend-contract.md) §3.1.

---

## 15. Güvenlik tasarımı

| Konu | Karar |
|---|---|
| Sırlar | Yalnız ortam değişkeni; açılışta doğrulanır, **eksikse uygulama açılmaz**. Kodda sır yok. |
| Sağlayıcı API anahtarları | DB'de AES-GCM ile şifreli; anahtar ortam değişkeninde. Panelde maskeli. |
| Aktarım | Caddy ile otomatik TLS, HSTS, güvenli çerez bayrakları |
| Girdi | Her uç nokta için ayrı DTO + `validator` etiketleri; **istek gövdesi asla model yapısına doğrudan bağlanmaz** (mass assignment koruması) |
| Çıktı | React varsayılan olarak escape eder; `dangerouslySetInnerHTML` kod incelemesinde gerekçe ister |
| Dosya yükleme | MIME + **sihirli bayt** kontrolü, boyut limiti, web kökü **dışına** kayıt, rastgele isim, imzalı URL ile sunum. *(Mevcut sistem yalnız `mimetype.startsWith('image/')` kontrol ediyor — istemci başlığı sahtelenebilir.)* |
| Başlıklar | CSP, X-Frame-Options, X-Content-Type-Options (Caddy + Next headers) |
| Bağımlılık | `govulncheck` + `npm audit` CI'da |
| Veri saklama | KVKK: saklama süreleri tanımlı, silme talebi akışı, IP/UA log'ları 90 gün |
| Yedek | Günlük otomatik, şifreli, ayrı konumda; **geri yükleme tatbikatı zorunlu** |

---

## 16. Dağıtım

```
VPS (Ubuntu 24.04)
└── docker compose
    ├── caddy      :80/:443   → otomatik TLS, ters vekil
    ├── web        :3000      → Next.js (standalone çıktı)
    ├── api        :8080      → Go sunucu (dağıtık olmayan tek ikili)
    ├── worker                → Go asynq işçileri (ayrı konteyner, ayrı ölçeklenir)
    ├── postgres   :5432      → kalıcı hacim + günlük yedek
    └── redis      :6379      → kalıcılık açık (AOF)
```

**Caddyfile özü:**
```
site.com {
    handle /api/* { reverse_proxy api:8080 }
    handle        { reverse_proxy web:3000 }
}
```

> 🔴 **Elle panel adımı (otomatikleştirilemez):** Webhook URL'i HeroSMS panelinden girilir —
> API uç noktası **yoktur**, en fazla **3 HTTPS URL** slotu vardır ve slotlar **hesap genelidir**.
> Prod/staging aynı hesabı paylaşırsa her ortam tüm hesabın olaylarını alır → tanımadığı
> `activationId`'yi sessizce yok sayma kuralı (§8.3) bu yüzden zorunludur.

Dağıtım: GitHub Actions imajları oluşturur → registry'ye iter → VPS'te `docker compose pull && up -d`.
Migration'lar `api` konteyneri başlarken `goose up` ile çalışır; başarısız olursa konteyner ayağa kalkmaz.

---

## 17. Karar kaydı (ADR)

| # | Karar | Gerekçe | Reddedilen |
|---|---|---|---|
| ADR-001 | Go backend | Paralel sağlayıcı sorgusu, SSE, tek ikili dağıtım, güçlü tip sistemi, düşük bellek | Node/TS |
| ADR-002 | Gin | En çok öğrenme materyali (Go'ya yeni başlangıç için belirleyici) | Echo (denk), chi (daha saf, daha az rehber) |
| ADR-003 | Sunucu tarafı oturum | İptal edilebilirlik, taze rol, tek alan adında sürtünmesiz | JWT — iptal edilemiyor |
| ADR-004 | **sqlc + pgx** (revize edildi 2026-09-08) | İlk karar GORM'du; gerekçesi geliştiricinin öğrenme hızıydı. Projeyi Claude geliştireceği için o gerekçe düştü ve geriye yalnız ORM'un örtük davranış riski kaldı. sqlc ile her sorgu açık SQL — para sisteminde denetlenebilirlik kazancı belirleyici. | GORM (örtük davranışlar, korkuluk listesi gerektiriyordu); Drizzle-benzeri ORM'lar (Go'da olgun karşılığı yok) |
| ADR-005 | Para = `int64` kuruş | Float aritmetiği kabul edilemez | `NUMERIC` okuma/yazma — Go tarafında yine dönüşüm riski |
| ADR-006 | Ledger tabanlı bakiye | Denetlenebilirlik, mutabakat, iade takibi | Doğrudan `balance` güncelleme — mevcut sistemin hatası |
| ADR-007 | Sunucu tarafı `price_quotes` | Fiyat garantisi + istemciye güvenmeme | İstemciden `provider_id` + fiyat — mevcut açık |
| ADR-008 | Provizyon/onay deseni | Dış çağrı transaction içinde tutulamaz | Uzun transaction — mevcut `confirmPurchase` |
| ADR-009 | `protocol` ile adaptör seçimi | İsme bağlı seçim kırılgan | `name.includes()` — mevcut yöntem |
| ADR-010 | Genel katalog modeli (§4) | Yeni ürün tipi = veri, tablo cerrahisi değil | Sabit `country × service` şeması — mevcut sistemin sınırı |
| ADR-011 | `pricing_rules` tablosu | Marj iş kararıdır, dağıtım gerektirmemeli | Koda gömülü sabit — mevcut sistemde marj hiç uygulanmıyor |
| ADR-012 | SSE (WebSocket değil) | Tarayıcıya tek yönlü bildirim için yeterli, daha sade, vekil dostu | WebSocket — gereksiz karmaşıklık |
| ADR-013 | Sunucu tarafı toplu yoklama (güvenlik ağı) | Sağlayıcı istek sayısı kullanıcı sayısından bağımsız. ADR-020 ile webhook birincil oldu, bu yedek. | İstemci başına doğrudan yoklama — mevcut prototipin yöntemi |
| ADR-014 | Tek alan adı + ters vekil | CORS ve çapraz alan çerez sorunu yok | Ayrı alan adları — daha çok yapılandırma hatası riski |
| ADR-015 | Monorepo | API sözleşmesi değiştiğinde iki taraf aynı commit'te | İki repo — tek geliştiricide gereksiz yük |
| ADR-016 | OpenAPI tek sözleşme | Go iskeleti + TS tipleri tek kaynaktan üretilir | Elle senkron tutma — kaçınılmaz sapma |
| ADR-017 | Yalnız TRY cüzdan | Mevcut sistemin davranışı; muhasebe en basit | Çoklu cüzdan — ledger karmaşıklığı v1'e değmez |
| **ADR-018** | **Her satın almada `maxPrice` gönderilir** | Fiyat bütünlüğü yalnız bizim kodumuzda değil, **sağlayıcı sınırında** da zorlanır: fiyat yükselmişse satın alma hiç gerçekleşmez. Kullanıcıdan beklenmedik tutar çekilmesi yapısal olarak imkânsız hale gelir. | Yalnız uygulama içi kontrol — sağlayıcı fiyatı çağrı anında değişirse yakalanmaz |
| **ADR-019** | **Webhook bir tetikleyicidir, veri kaynağı değil** | HeroSMS imza doğrulaması sunmuyor. Gelen bildirim üzerine kod **API'den teyit edilir**; webhook gövdesindeki `code` alanına güvenilmez. Sahte bildirimle ücretsiz kod alınması engellenir. | Webhook verisini doğrudan yazmak — imzasız uç noktada kabul edilemez |
| **ADR-020** | **Webhook birincil, yoklama güvenlik ağı** | Webhook hızlı ama teslim garantisi ve API'den kaydı yok; yoklama yavaş ama güvenilir. İkisi birlikte hem hızlı hem dayanıklı. | Yalnız yoklama (sağlayıcı isteği kullanıcıyla orantılı büyür); yalnız webhook (sessiz kayıp) |
| ADR-021 | **HeroSMS için modern REST, legacy zorunlu tamamlayıcı** | Modern API tipli hata, `maxPrice`, `expiredAt`, sayfalama sunuyor; katalog çağrıları (`getCountries`, `getServicesList`) yalnız legacy'de var. | Tamamen legacy (eski prototipin yolu) — zengin yetenekler kaybedilir |

---

**İlgili:** [intent.md](intent.md) · [trd.md](trd.md) · [provider-herosms.md](provider-herosms.md) · [frontend-contract.md](frontend-contract.md) · [roadmap.md](roadmap.md) · [memory.md](memory.md) · [../CLAUDE.md](../CLAUDE.md)
