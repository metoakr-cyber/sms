# roadmap.md — Yapım Sırası

> **Doküman amacı:** v1'e giden inşa sırasını ve her adımın **çıkış kriterlerini** tanımlar.
> Bir kilometre taşı, çıkış kriterlerinin tamamı sağlanmadan "bitti" sayılmaz.
>
> **Durum:** v1 · **Son güncelleme:** 2026-09-08
>
> ⚠️ **Bu doküman takvim içermez.** Kodu Claude yazıyor; "2 hafta" gibi tahminler anlamsız.
> Önemli olan **sıra** ve **çıkış kriterleri**. Sıra bağımlılıkla belirlenir, tercihle değil.

---

## Sıra ve neden bu sıra

```
M0  Temel            ─┐  iskelet, CI, yapılandırma
M1  Kimlik            │  katman mimarisi gerçek bir özellik üzerinde kurulur
M2  Para ve ledger    │  ⭐ altındaki her şey buna yaslanır
M3  Sağlayıcı         │  fiyat buradan gelir
M4  Fiyatlandırma     │  M2 (para) + M3 (maliyet) olmadan yazılamaz
M5  Sipariş + SSE     │  ⭐ ürünün kalbi; M2+M3+M4 hazır olmalı
M6  Panel + ödeme     │  altındaki her şey oturmuş olmalı
M7  Sertleştirme      │  kırmaya çalışma, ikinci sağlayıcı
M8  Üretim            ─┘  dağıtım, izleme, yedek
M9  Lansman kapısı    ─── ticari/yasal karar (kod işi değil)
```

**Bağımlılık gerekçeleri:**
- **M2 → M5:** Sipariş akışı bakiye rezerve eder. Ledger olmadan sipariş yazılırsa, para mantığı
  siparişin içine sızar ve sonradan ayrılamaz. *Eski prototipin hatası tam olarak buydu.*
- **M3 → M4:** Fiyat sağlayıcıdan gelir. Sağlayıcı soyutlaması olmadan fiyat motoru tek sağlayıcıya
  gömülür.
- **M6 en sonda:** Panel, altındaki her şeyin sözleşmesinin oturmuş olmasını gerektirir.
  *Eski prototipte panel altyapıdan önce yazıldı; 922 satırlık view bu yüzden oluştu.*

---

## M0 — Temel

**Amaç:** Üzerine inşa edilecek iskelet.

- [ ] Monorepo: `api/` · `web/` · `docs/` · `deploy/`
- [ ] `docker-compose.yml` (yerel): postgres 16 + redis 7
- [ ] Go modülü · `golangci-lint` · `gofumpt` · `Makefile`
- [ ] **sqlc** kurulumu (`sqlc.yaml`, `queries/`, üretim hedefi `internal/db/`)
- [ ] **goose** migration altyapısı
- [ ] Next.js 15 + Tailwind 4 + shadcn/ui; Duralux koyu paletinin CSS token'a çevrilmesi
- [ ] **`web/src/lib/api/client.ts`** — tek HTTP sarmalayıcısı (`frontend-contract.md` §3.1)
      *ilk `fetch` yazılmadan önce*
- [ ] Mobil taban stiller: `dvh` yedeği, `safe-area`, 16px girdi kuralı
- [ ] **Playwright 4 proje** (chromium/webkit × masaüstü/mobil)
- [ ] CI: `sqlc diff` → `go vet` → `golangci-lint` → `go test` → `tsc` → `eslint` → `next build`
- [ ] `config` paketi: ortam değişkenleri Zod benzeri katı doğrulamayla — **eksikse süreç başlamaz**
- [ ] `.env.example` + `README.md`

### Çıkış kriteri
- `make dev` tek komutla postgres + redis + api + web ayağa kaldırıyor
- `/healthz` 200, Next.js ana sayfa açılıyor, CI yeşil

---

## M1 — Kimlik ve yetkilendirme

**Amaç:** Katman mimarisini gerçek bir özellik üzerinde kurmak. Kimlik seçildi çünkü tüm
katmanlara dokunuyor ama iş mantığı basit.

- [ ] Migration: `users` · `roles` · `permissions` · `role_permissions` · `user_roles` · `sessions`
- [ ] Katman iskeleti: `domain` / `port` / `service` / `adapter` / `transport`
- [ ] `openapi.yaml` + `oapi-codegen` (Go) + `openapi-typescript` (web) üretim boru hattı
- [ ] Kayıt · e-posta doğrulama · giriş · çıkış · şifre sıfırlama → **FR-100…FR-104**
- [ ] argon2id · Redis oturum katmanı · oturum yenileme
- [ ] RBAC middleware (`RequirePermission`) → **FR-105**
- [ ] `requestid` · `recover` · `ratelimit` · `csrf` middleware'leri
- [ ] Hata taksonomisi + tek biçim yanıt (`design.md` §11) · `log/slog` JSON
- [ ] E-posta gönderimi (Resend) + SPF/DKIM  ⚠️ *bkz. "Senden gerekenler" G2*
- [ ] Next.js: giriş/kayıt/şifre sıfırlama ekranları, `middleware.ts` yönlendirmesi
- [ ] `testcontainers-go` entegrasyon test altyapısı

### Çıkış kriteri
- Uçtan uca kayıt → doğrulama e-postası → giriş → panel
- **Yetki matrisi testleri:** her uç nokta için 401 / 403 / 404 senaryoları yeşil
- Askıya alınan kullanıcının oturumu anında düşüyor (**KK-104**)

---

## M2 — Para ve ledger ⭐

**Amaç:** Sistemin en kritik parçası. Buradaki bir hata gerçek para kaybettirir.
**Bu faz bitmeden para harcayan hiçbir özellik yazılmaz.**

- [ ] `domain/money`: `int64` kuruş, para birimi güvenliği, yuvarlama
- [ ] Migration: `ledger_entries` + `CHECK (balance_minor >= 0)` + değişmezlik tetikleyicisi
- [ ] `queries/wallet.sql`: `FOR UPDATE` kilidi, idempotency arama, ledger yazımı
- [ ] `WalletRepo.ApplyEntry` (`design.md` §5.3) → **FR-200…FR-206**
- [ ] `ledger-reconcile` işi + `ledger_drift_minor` metriği + alarm
- [ ] Admin manuel düzeltme (`ADJUSTMENT`, not zorunlu) → **FR-504**
- [ ] `audit_logs` tablosu ve yazma yardımcısı

### Çıkış kriteri — **hepsi testle ispatlanmış**
- **KK-201** 100 eşzamanlı aynı-anahtar çağrısı bakiyeyi bir kez değiştiriyor
- **KK-202** 100 TL ile 50 TL'lik 10 eşzamanlı harcamadan tam 2'si geçiyor
- **KK-203** Negatif bakiye veritabanınca reddediliyor
- **KK-204** Ledger `UPDATE`/`DELETE` denemesi hata veriyor
- **KK-200** `Σ ledger == balance` sıfır sapma
- `domain/money` kapsamı ≥ %95

> Bu fazın aceleye gelmesi sistemin en pahalı hatasıdır. Kapsam kısılmaz.

---

## M3 — Sağlayıcı katmanı

**Amaç:** Genişletilebilir katalog + HeroSMS adaptörü.
**Referans:** [provider-herosms.md](provider-herosms.md) — doğrulanmış API sözleşmesi.

- [ ] Migration: `services` · `countries` · `operators` · `products` · `providers` ·
      `provider_dimension_maps` · `provider_offers`
- [ ] `ProviderPort` arayüzü (`design.md` §9)
- [ ] **`FakeProvider` önce yazılır** — tüm akış ağsız test edilebilir olsun
- [ ] `HEROSMS_V1` adaptörü: modern REST birincil, legacy yalnız `getCountries`/`getServicesList`
- [ ] API anahtarı şifreleme (AES-GCM)
- [ ] Ortak HTTP istemcisi: zaman aşımı · devre kesici · sağlayıcı metrikleri
- [ ] `catalog-sync` + `offers-sync` işleri
      *(ölçüldü: tüm katalog tek çağrıda 20.849 kombinasyon / ~600 ms — ülke döngüsü gereksiz)*
- [ ] `api/testdata/fixtures/herosms/` — gerçek yanıtlar + sözleşme testleri
- [ ] 💰 **Canlı doğrulama turu** → `provider-herosms.md` §12'deki **H1–H21** listesi
      (11'i para harcıyor: durum kodları, iptal eşikleri, fiyat katmanı, para birimi teyidi,
      stok yokken hangi hata, süre dolunca ne olur). ⚠️ *"Senden gerekenler" G3*
- [ ] Adaptör: **`Cancel()` + `Finish()` ayrı** (ADR-023) · teyit `/{id}/otp/last` (ADR-022) ·
      **OTP alan adı normalizasyonu** (üç isim seti) · yanıt dizisi `data[0]` ·
      `maxPrice` evet / `fixedPrice` hayır (ADR-027) · semafor (FR-413) · `Retry-After` (FR-414)

### Çıkış kriteri
- `FakeProvider` ile tüm katalog akışı ağsız çalışıyor
- Gerçek hesapla 195 ülke / 811 servis çekiliyor, eşleştirmeler doluyor
- **Stok `counts.physical`'den okunuyor**; `count` hiçbir yerde stok olarak kullanılmıyor (**KK-306**)
- Sözleşme testleri fixture'larla yeşil
- 3 sn'de yanıt vermeyen sağlayıcı eleniyor, sistem beklemiyor

---

## M4 — Fiyatlandırma ve teklif

**Amaç:** "Gösterilen fiyat = tahsil edilen fiyat" güvencesi.

- [ ] Migration: `pricing_rules` · `fx_rates` · `price_quotes`
- [ ] `domain/pricing`: kapsam önceliği, yukarı yuvarlama, sabit noktalı aritmetik → **FR-303, FR-304**
- [ ] Kur sağlayıcısı + `fx-refresh` işi + Redis önbelleği + **bayat kurda satış durdurma** → **FR-302**
      ⚠️ *bkz. "Senden gerekenler" G4*
- [ ] `GET /catalog/quote` → **FR-305, FR-306**
- [ ] Paralel sağlayıcı sorgusu (`errgroup`, zaman aşımı, kısmi başarı)

### Çıkış kriteri
- **KK-304** Altın test beklenen kuruşu tam veriyor
- **KK-305** Yanıtta `providerId` ve `cost` **yok** — testle doğrulanmış
- **KK-302** Kur 30 dk'dan eskiyse `503`
- Beş kapsam önceliğini doğrulayan birim testi yeşil

---

## M5 — Sipariş akışı, webhook, SSE ⭐

**Amaç:** Ürünün kalbi. Para ile dış dünyanın buluştuğu yer.

- [ ] Migration: `orders` · `order_messages` · `rental_details` (v1.1 için boş)
- [ ] `domain/order`: durum makinesi, geçerli geçiş tablosu (`design.md` §7.1)
- [ ] `POST /orders`: T1 → sağlayıcı → T2 provizyon/onay → **FR-400, FR-401, FR-402**
      **`maxPrice` her çağrıda gönderilir** (ADR-018)
- [ ] **Webhook uç noktası** `POST /webhooks/herosms/{secret}` → **FR-410**
      tahmin edilemez yol · **IP izin listesi** (`84.32.223.53`, `185.138.88.87`) ·
      **3 sn içinde 200** · dedup `(activationId, id)` · kendi OTP regex'imiz
- [ ] `webhook-ingest` işi (asenkron teyit: `/{id}/otp/last`) → **ADR-024**
- [ ] `activation-reaper` işi — her sipariş sağlayıcıda kapatılır → **FR-412**
      ⚠️ *bkz. "Senden gerekenler" G5*
- [ ] Redis Pub/Sub + SSE + keepalive → **FR-403, FR-411**
- [ ] `order-poller` (30 sn, `GET /activations` `size=25` sayfalı) → **FR-404**
- [ ] `order-expirer` (otomatik iptal + iade) → **FR-405**
- [ ] `orphan-hold-reaper` → **FR-408**
- [ ] İptal + iade → **FR-406** · `provider-refund-retry` + `provider_refund_status` → **FR-406b**
- [ ] **SSE sertleştirme** (`frontend-contract.md` §4): keepalive izleme, üstel geri çekilme,
      `visibilitychange`/`pageshow` senkronu, yoklama yedeği, terminal durumda kapatma
- [ ] Caddy `flush_interval -1` + **yerel HTTPS/HTTP2 ile doğrulama**
      *(bu tuzak yerelde HTTP/1.1'de fark edilmez)*
- [ ] Next.js satın alma ekranı: `ServiceGrid` → `CountrySelect` → `QuotePanel` →
      `PurchaseButton` → `CodeWaiter` (`useOrderStream`) → `OrderHistoryTable`
- [ ] **İptal butonu ilk 120 sn pasif** (süre sunucudan) → **FR-416**
- [ ] Çok mesajlı sipariş: SSE ilk koddan sonra kapanmaz → **FR-415**

### Çıkış kriteri
- Uçtan uca: bakiye tanımla → numara al → SSE ile kod gelsin → geçmişte görünsün
- **KK-400** Sağlayıcı hatasında bakiye tam iade, sipariş oluşmuyor
- **KK-402** Aynı teklifle iki eşzamanlı satın almadan tam biri geçiyor
- **KK-403** Başkasının akışına bağlanma denemesi 404
- **KK-405** Süre dolumunda kullanıcı hiçbir şey yapmadan iadesini alıyor
- **KK-408** T1 sonrası süreç öldürüldüğünde para kaybolmuyor (kaos testi)
- **KK-410** Sahte webhook siparişi tamamlayamıyor — teyit olmadan durum değişmiyor
- **KK-411** Webhook devre dışıyken kod en geç 30 sn'de yoklamayla geliyor
- **KK-412** Hiçbir terminal sipariş sağlayıcıda kapatılmamış aktivasyon bırakmıyor
- **KK-415** İki SMS gelen sipariş ikisini de saklıyor ve gösteriyor
- **KK-416** Satın almadan 10 sn sonra iptal butonu pasif, geri sayım gösteriyor

---

## M6 — Panel, bakiye yükleme, destek

**Amaç:** Sistemi işletilebilir hale getirmek.

- [ ] Migration: `deposit_methods` · `deposits` · `tickets` · `ticket_messages`
- [ ] Havale ile yükleme + güvenli dosya yükleme (sihirli bayt, web kökü dışı) → **FR-500**
- [ ] USDT ile yükleme (TX hash, manuel onay) → **FR-501**  ⚠️ *G6*
- [ ] Onay/red — **POST**, idempotent, izinli, audit log'lu → **FR-502, FR-503**
- [ ] Destek talebi sistemi → **FR-600**
- [ ] Admin ekranları: kullanıcılar · sağlayıcılar · boyut eşleştirme · fiyat kuralları
      (canlı önizlemeli) · bakiye talepleri · denetim kaydı → **FR-700…FR-705**
- [ ] Kullanıcı paneli: profil · cüzdan · hareket dökümü · sipariş geçmişi · talepler
- [ ] i18n sözlüğü (`messages/tr.json`) → **NFR-806**
- [ ] Her ekran için `frontend-contract.md` §9 kontrol listesi

### Çıkış kriteri
- Admin bir bakiye talebini onaylayabiliyor; işlem audit log'da ve ledger'da
- **KK-502** Yetkisiz onay 403; 100 eşzamanlı onay bakiyeyi bir kez artırıyor
- **KK-500** Sahte MIME başlıklı dosya reddediliyor; yüklenen dosya doğrudan URL ile açılamıyor
- **KK-809** 320 px'te hiçbir sayfada yatay kaydırma yok
- **KK-810** Uçtan uca testler dört Playwright projesinde yeşil
- Görsel olarak Duralux temasına sadık

---

## M7 — Sertleştirme

**Amaç:** Sistemi kırmaya çalışmak.

- [ ] İkinci sağlayıcı adaptörü + sözleşme testleri  ⚠️ *G7*
- [ ] **Kaos testi:** bir sağlayıcı kapatıldığında sistem ayakta mı, fiyat diğerinden mi geliyor
- [ ] Yük testi (k6, 50 eşzamanlı) → **NFR-800**
- [ ] Güvenlik geçişi: her uç nokta için 401/403/404 matrisi
- [ ] `govulncheck` + `npm audit` temiz
- [ ] **Gerçek cihaz turu:** iPhone + Android, yavaş 3G, uçak modu → `frontend-contract.md` §7.2

### Çıkış kriteri
- İki sağlayıcı aktif; biri kapatıldığında sipariş akışı kesintisiz
- Yük testi hedefleri karşılanıyor · kritik/yüksek güvenlik açığı yok

---

## M8 — Üretime hazırlık

- [ ] VPS kurulumu · Docker Compose · Caddy · otomatik TLS  ⚠️ *G8*
- [ ] Dağıtım boru hattı (GitHub Actions → registry → VPS)
- [ ] Sentry · Prometheus · alarm kuralları (`design.md` §12)
- [ ] Otomatik yedekleme + **geri yükleme tatbikatı** → **NFR-808**
- [ ] Çalışma kitabı: sağlayıcı çöktü · kur bayat · mutabakat sapması · disk doldu
- [ ] KVKK aydınlatma metni · kullanım şartları · çerez politikası
- [ ] Lighthouse bütçe kontrolü CI'da → **NFR-812** · erişilebilirlik → **NFR-805**

### Çıkış kriteri
- `intent.md` §11 "bitti" listesinin tamamı işaretli
- Yedekten geri yükleme **bir kez gerçekten yapılmış**
- Bir alarm bilerek tetiklenmiş ve bildirim ulaşmış

---

## M9 — Lansman kapısı

> Geliştirme fazı **değil**, ticari/yasal karar noktası. `intent.md` §9'da kayıtlı:
> şirket kurulmayacak.

| Kanal | Durum |
|---|---|
| Kart ödemesi | ❌ Üye işyeri hesabı vergi levhası ister |
| Kripto (USDT) tahsilat | ❌ 2021 tarihli yönetmelikle mal/hizmet bedeli olarak kripto kabulü yasak |
| Şahsi hesaba havale | ⚠️ Kayıt dışı ticari faaliyet; vergi yükümlülüğü doğar |

**Seçenekler:** (1) şahıs şirketi kur → tüm kanallar açılır · (2) kapalı/davetli kullanım,
bakiye elle tanımlanır · (3) portföy/vitrin, sahte sağlayıcı + simüle ödeme · (4) beklemede tut.

Karar `memory.md` §1'e tarihiyle yazılır.

---

## Senden gerekenler

> Bunlar **benim yapamayacağım** işler. Her biri belirtilen kilometre taşını bloke eder.
> Sırası geldiğinde hatırlatacağım; şimdiden hazırlamanız işi hızlandırır.

| # | Ne gerekiyor | Ne zaman | Neden ben yapamıyorum |
|---|---|---|---|
| **G1** | 🔴 **HeroSMS API anahtarını yenile** — mevcut anahtar 2026-09-08'de sohbete düz metin girdi | **Şimdi** | Panel erişimi sizde |
| **G2** | E-posta sağlayıcısı hesabı (Resend önerilir) + alan adı SPF/DKIM kaydı | M1 | Hesap açma + DNS erişimi |
| **G3** | HeroSMS hesabına bakiye (~$5–10) — **H1–H21 canlı testleri için** (11 test para harcıyor). Mevcut bakiye **$0,5632, yetersiz** | M3 | Ödeme |
| **G4** | Kur kaynağı kararı: TCMB (ücretsiz, günlük) vs ücretli FX API | M4 | Ürün/maliyet kararı |
| **G5** | HeroSMS panelinden **webhook URL'i ayarla** — API ile kaydedilemiyor, **en fazla 3 HTTPS URL**, slotlar **hesap geneli** (prod/staging paylaşımı planlanmalı). *Kaynak IP'ler spec'te yazılı, sormaya gerek yok.* | M5 | Panel erişimi |
| **G6** | USDT alım adresleri (TRC20 + ERC20) | M6 | Cüzdan sizin |
| **G7** | İkinci sağlayıcı hesabı (5sim veya SMS-Activate) | M7 | Hesap açma + ödeme |
| **G8** | VPS + alan adı (Hetzner/DigitalOcean; alan adı DNS'i) | M8 | Hesap + ödeme |
| **G9** | Gerçek iPhone/Android ile kabul testi | M7 | Fiziksel cihaz ✅ *mevcut* |
| **G10** | Lansman kapısı kararı | M9 | Ticari/yasal karar |

---

## v1.1 ve sonrası (sıralı öneri)

| # | Özellik | Neden bu sırada |
|---|---|---|
| 1 | **Kiralık numara** | Şeması hazır, sağlayıcı desteği doğrulandı; ortalama sipariş değerini en çok artıran özellik |
| 2 | **Referans/affiliate** | Şeması hazır; büyümeyi ücretsiz hızlandırır |
| 3 | Otomatik kripto doğrulama | Manuel onay operasyonel darboğaz olduğunda |
| 4 | Üçüncü sağlayıcı | Fiyat rekabeti ve dayanıklılık |
| 5 | E-posta / sesli doğrulama ürünü | Sağlayıcı destekliyor, katalog şeması hazır |
| 6 | Bayi / API erişimi | HeroSMS'te `resellerUserId` zaten var |
| 7 | İngilizce arayüz | i18n altyapısı hazır; yalnız çeviri |

---

## Risk kaydı

| # | Risk | Olasılık | Etki | Karşılık | Erken uyarı |
|---|---|---|---|---|---|
| R1 | Yasal yapı yok — tahsilat yapılamaz | Kesin | Lansmanı engeller | M9 kapısı; teknik plan etkilenmez | — (bilinen) |
| R2 | **Ara onay yok** — beklenti/yorum farkı sonda çıkar | Orta | Geri alınacak iş | `trd.md` KK maddeleri sözleşme; varsayımlar açıkça yazılır ve teslimde raporlanır | Teslimde "bunu böyle istememiştim" |
| R3 | HeroSMS hız limiti bilinmiyor (S17) | Orta | Katalog senkronu engellenebilir | Kademeli yük testi; destek birimine sor | 429 yanıtları |
| R4 | SSE ters vekil arkasında tamponlanıyor | Orta | UX bozulur | M5'te erken test; yoklama yedeği | Yerelde çalışıp üretimde gecikme |
| R4b | iOS Safari arka planda SSE'yi sessizce öldürüyor | **Yüksek** | Kullanıcı kodu göremez | `visibilitychange`/`pageshow` senkronu; **gerçek cihaz testi** | Yalnız gerçek iPhone'da görülür |
| R4c | HTTP/1.1'de 6 bağlantı sınırı | Orta | Çok sekmede donma | HTTP/2 zorunlu; yerelde HTTPS ile doğrula | Yerelde asla görülmez |
| R5 | Tek sağlayıcı — HeroSMS çökerse hizmet durur | Orta | Yüksek | M7'de ikinci sağlayıcı | Sağlayıcı hata oranı alarmı |
| R6 | Kur şoku ile zararına satış | Düşük | Orta | Tampon + kısa teklif süresi + bayat kurda durdurma | Marj düşüşü |
| R7 | Kötüye kullanım (spam/dolandırıcılık) | Orta | Yüksek | Kullanım şartları · hız limiti · desen tespiti | Anormal sipariş yoğunluğu |
| R11 | Webhook'ta imza yok — sahte kod bildirimi | Orta | Ücretsiz kod alınabilir | Tahmin edilemez URL + **IP izin listesi** (spec'te yazılı) + `/otp/last` teyidi (ADR-022) | KK-410 testi |
| R12 | Sağlayıcı iptal penceresi dar (120 sn / 20 dk / OTP) — iade alamayabiliriz | **Yüksek** | Karşılanmayan her iade doğrudan zarar | Kullanıcıya iade koşulsuz; iptal butonu 120 sn pasif (FR-416); `provider-refund-retry`; gider olarak ölç | Kâr raporunda iade gideri |
| R14 | Spec canlıdan geride, belgesiz davranışlar var (H1–H21) | **Yüksek** | Adaptör yanlış varsayımla yazılabilir | Fixture'lar **canlıdan**; M3'te 21 maddelik doğrulama turu; belirsizde savunmacı davran (bilinmeyen durum → `PENDING`) | Sözleşme testi kırılması |
| R13 | Webhook URL'i API ile kaydedilemiyor | Orta | Geliştirme/üretim ayrımı zor | Ortam başına ayrı URL veya hesap | M8 |

---

## Çalışma disiplini

- **Çıkış kriterleri pazarlığa açık değildir.** Bir kriter sağlanamıyorsa faz kapanmaz.
- **Test yazılmadan özellik bitmiş sayılmaz** — özellikle M2 ve M5.
- **`main` her zaman dağıtılabilir.** Özellik dalları kısa ömürlü.
- **Her kararı `memory.md` §1'e yaz**, gerekçesiyle.
- **Belirsizlikte varsayım açıkça yazılır** ve teslim raporunda belirtilir — ara onay olmadığı için
  bu, sapmayı yakalamanın tek yolu.
- **Commit mesajları anlamlı olur.** *(Eski repoda 127 commit'in tamamı "fix".)*

---

**İlgili:** [intent.md](intent.md) · [design.md](design.md) · [trd.md](trd.md) · [provider-herosms.md](provider-herosms.md) · [frontend-contract.md](frontend-contract.md) · [memory.md](memory.md) · [../CLAUDE.md](../CLAUDE.md)
