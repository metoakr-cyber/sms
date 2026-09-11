# trd.md — Teknik Gereksinim Dokümanı

> **Doküman amacı:** Numaralandırılmış, **test edilebilir** gereksinimler. Bir özellik "bitti" sayılabilmesi
> için buradaki kabul kriterlerini karşılamalıdır. `design.md` "nasıl"ı, bu doküman "tam olarak ne"yi tanımlar.
>
> **Durum:** v1 · **Son güncelleme:** 2026-09-08 · **Önkoşul:** [intent.md](intent.md) · [design.md](design.md)

**Okuma kılavuzu:** `ZORUNLU` = v1'de olmalı · `ÖNERİLEN` = güçlü tercih, gerekçeli sapılabilir ·
`v1.1` = şeması v1'de, özelliği sonra. Her gereksinimin **KK** (Kabul Kriteri) satırı testin ne
doğrulayacağını söyler.

---

## 1. Kimlik ve hesap yönetimi

### FR-100 · Kayıt `ZORUNLU`
Kullanıcı e-posta, kullanıcı adı ve şifre ile kayıt olur.
- Kullanıcı adı: 3–32 karakter, `[a-zA-Z0-9_]`, benzersiz (büyük/küçük harf duyarsız)
- E-posta: RFC 5322 geçerli, benzersiz (büyük/küçük harf duyarsız — `CITEXT`)
- Şifre: en az 10 karakter; en yaygın 10.000 şifre listesinde olmamalı
- Şifre **argon2id** ile hash'lenir (`m=64MB, t=3, p=4`)
- Kayıt sonrası durum `PENDING_VERIFICATION`; doğrulama e-postası gönderilir
- reCAPTCHA doğrulaması zorunlu

> **KK-100:** Aynı e-posta ile ikinci kayıt 409 döner. Şifre veritabanında düz metin olarak
> **hiçbir sorguda** görünmez. Zayıf şifre 422 ve Türkçe hata mesajı döner.

### FR-101 · E-posta doğrulama `ZORUNLU` *(akış), `KAPI DEĞİL` (yetki)*
Tek kullanımlık, 24 saat geçerli, kriptografik olarak rastgele token. Kayıt sonrası doğrulama
e-postası gönderilir ve `/dogrula` ucu çalışır.

> 🔴 **DOĞRULAMA HİÇBİR İŞLEMİ ENGELLEMEZ** — kullanıcı kararı, 11 Eylül 2026.
> ~~Doğrulanmamış kullanıcı satın alma ve bakiye yükleme yapamaz.~~
> Doğrulanmamış hesap da numara alabilir, bakiye yükleyebilir, yorum yazabilir.
> `middleware.RequireVerifiedEmail` kodda duruyor ve sınanıyor ama **hiçbir rotaya bağlı
> değildir**; geri açmak beş satır eklemektir (`router.go`).
> Karşılığında alınan risk ve gerekçe: [memory.md](memory.md) §1.

> **KK-101:** Kullanılmış token ikinci kez 410 döner. Doğrulanmamış kullanıcının
> `POST /orders` isteği **başarılı olur** — 403 `EMAIL_NOT_VERIFIED` dönerse kapı
> yanlışlıkla geri gelmiş demektir (`scripts/smoke-auth.sh` bunu sınar).

### FR-102 · Giriş `ZORUNLU`
E-posta + şifre + reCAPTCHA. Başarıda Redis oturumu oluşturulur, `sid` çerezi yazılır.
- Çerez: `HttpOnly; Secure; SameSite=Lax; Path=/; Max-Age=30 gün`
- Girişte oturum kimliği **yenilenir** (oturum sabitleme koruması)
- Hatalı e-posta ve hatalı şifre **aynı** mesajı döndürür (hesap sayımı engellenir)
- `SUSPENDED` kullanıcı giriş yapamaz

> **KK-102:** 5 başarısız denemeden sonra o hesap için 15 dakika kilit; IP başına ayrı kademeli
> gecikme. reCAPTCHA anahtarı tanımsızsa **uygulama açılışta hata verip durur** (sessizce çalışmaz).

### FR-103 · Şifre sıfırlama `ZORUNLU`
E-posta ile tek kullanımlık, 1 saat geçerli token. Sıfırlama sonrası **o kullanıcının tüm oturumları düşer.**

> **KK-103:** Var olmayan e-posta için de "gönderildi" mesajı döner (hesap sayımı engellenir).
> Sıfırlama sonrası eski `sid` çerezi 401 alır.

### FR-104 · Oturum yönetimi `ZORUNLU`
Kullanıcı aktif oturumlarını (IP, tarayıcı, son görülme) listeler ve uzaktan sonlandırır.
Admin bir kullanıcıyı askıya aldığında **tüm oturumları anında düşer**.

> **KK-104:** Askıya alınan kullanıcının açık sekmesindeki bir sonraki istek 401 döner.

### FR-105 · Rol ve izin `ZORUNLU`
İzinler veritabanında satırdır, kodda sabit değil. v1 rolleri: `user`, `admin`.
İzin kodları: `users:read|write` · `deposits:read|approve` · `providers:read|write` ·
`pricing:read|write` · `tickets:read|reply` · `orders:read_all` · `audit:read`

> **KK-105:** Yeni bir izin eklemek için yalnız satır eklenir; kod dağıtımı gerekmez.
> İzni olmayan kullanıcının admin uç noktasına isteği 403 döner.

---

## 2. Cüzdan ve muhasebe

### FR-200 · Ledger tabanlı bakiye `ZORUNLU`
Her bakiye değişimi bir `ledger_entries` kaydı üretir. `users.balance_minor` türetilmiş önbellektir.
Tüm para değerleri `int64` **kuruş**.

> **KK-200:** `SELECT SUM(amount_minor) FROM ledger_entries WHERE user_id=X` sonucu
> `users.balance_minor` ile **her zaman** eşittir. Bunu doğrulayan bir entegrasyon testi vardır.

### FR-201 · İdempotency `ZORUNLU`
Her ledger kaydı benzersiz bir `idempotency_key` taşır. Aynı anahtarla ikinci çağrı yeni kayıt
oluşturmaz, mevcut sonucu döner.

> **KK-201:** Aynı `deposit:{id}` anahtarıyla 100 eşzamanlı onay çağrısı yapıldığında bakiye
> **yalnız bir kez** artar. (Test: `t.Parallel()` + `errgroup`)
>
> **Webhook idempotency'si de bu kapsamdadır:** aynı SMS sağlayıcı tarafından **en az 8 kez**
> gönderilebilir; `webhook-ingest` işleyicisi `(activationId, id)` anahtarıyla dedup yapar.

### FR-202 · Eşzamanlılık güvenliği `ZORUNLU`
Bakiye okuma+yazma `SELECT ... FOR UPDATE` kilidi altında, tek transaction içinde yapılır.

> **KK-202:** Bakiyesi 100 TL olan kullanıcı için 50 TL'lik 10 eşzamanlı satın alma denendiğinde
> **tam 2 tanesi** başarılı olur, bakiye 0 olur, negatife düşmez.

### FR-203 · Negatif bakiye yasağı `ZORUNLU`
`CHECK (balance_minor >= 0)` veritabanı kısıtı + uygulama kontrolü.

> **KK-203:** Kısıtı ihlal eden doğrudan SQL `UPDATE` denemesi veritabanı tarafından reddedilir.

### FR-204 · Ledger değişmezliği `ZORUNLU`
`ledger_entries` üzerinde `UPDATE` ve `DELETE` veritabanı tetikleyicisiyle engellenir.
Düzeltme yalnız ters `ADJUSTMENT` kaydı ile yapılır.

> **KK-204:** `UPDATE ledger_entries SET amount_minor=0` denemesi hata verir.

### FR-205 · Günlük mutabakat `ZORUNLU`
Her gün 04:00'te tüm kullanıcılar için `Σ ledger == balance` kontrolü. Sapmada alarm; **otomatik
düzeltme yapılmaz**.

> **KK-205:** Elle bozulmuş bir bakiye, bir sonraki mutabakat çalışmasında tespit edilir ve
> `ledger_drift_minor` metriği sıfırdan farklı olur.

### FR-206 · Hareket dökümü `ZORUNLU`
Kullanıcı kendi ledger hareketlerini tarih aralığı ve tip filtresiyle, sayfalı görür.
Her satır: tarih, tip, tutar, işlem sonrası bakiye, ilgili sipariş/yükleme bağlantısı.

> **KK-206:** Kullanıcı başka bir kullanıcının hareketini **hiçbir parametreyle** göremez.

---

## 3. Katalog ve fiyatlandırma

### FR-300 · Ürün kataloğu `ZORUNLU`
Ürün = `(kind, service, country, operator, duration)`. `SMS_ACTIVATION` v1'de aktif.
`SMS_RENTAL` **arka uçta v1'de tamamlandı** (FR-417…FR-420, `rental-poller`/`rental-closer`
işleri koşuyor) ama ürün satırı üretilmesi `catalog:rentals` senkronuna bağlıdır ve **ön yüzü
henüz yoktur** — bkz. TESLIM.md §3.

> **KK-300:** Yeni bir `product_kind` eklemek `orders`, `ledger_entries` ve `price_quotes`
> tablolarında **hiçbir şema değişikliği** gerektirmez.
>
> ⚠️ **Kısmi kısıt:** `products` tablosunun tipli sütunları ve UNIQUE kısıtı telefon-merkezlidir.
> E-posta ürünü gibi farklı eksenli bir tip eklenirken `products` **değişebilir** — bu yüzden
> `dimension_a_id` genel boyut sütunu ve `UNIQUE NULLS NOT DISTINCT` baştan konmuştur
> ([design.md](design.md) §6).

### FR-301 · Boyut eşleştirme `ZORUNLU`
`provider_dimension_maps` tablosu yerel boyut değerlerini sağlayıcı kodlarına çevirir.
Admin panelinden elle düzenlenebilir ve `catalog-sync` işiyle otomatik doldurulabilir.

> **KK-301:** Yeni bir boyut tipi (`operator`, `duration`) eklemek **yeni tablo gerektirmez**.

### FR-302 · Kur yönetimi `ZORUNLU`
USD/TRY kuru 5 dakikada bir tazelenir, Redis'te önbelleklenir, `fx_rates` tablosuna yazılır.
Yapılandırılabilir bir güvenlik tamponu (`FX_SAFETY_MARGIN`, varsayılan %2) uygulanır.

> **KK-302:** Kur **30 dakikadan eskiyse** teklif ve satın alma uç noktaları
> `503 FX_UNAVAILABLE` döner. Kur kaynağı çökse bile sistem yanlış fiyatla satış yapmaz.

### FR-303 · Fiyat kuralları `ZORUNLU`
Marj `pricing_rules` tablosunda; kapsam öncelik sırası:
`PRODUCT > SERVICE_COUNTRY > SERVICE > COUNTRY > GLOBAL`. En spesifik aktif kural kazanır.
Her kural: `margin_percent`, `fixed_fee_minor`, `min_price_minor`.

> **KK-303:** WhatsApp/TR için özel kural tanımlandığında yalnız o kombinasyon etkilenir;
> diğerleri `GLOBAL` kuralı kullanmaya devam eder. Bir birim testi beş kapsamın önceliğini doğrular.

### FR-304 · Fiyat hesaplama `ZORUNLU`
```
sell = max( ceil( cost_usd × provider.cost_multiplier × fx × (1 + margin/100) ) + fixed_fee ,
            min_price )
```
Yuvarlama **her zaman yukarı** (kuruş). Ara hesaplarda ondalıklı sayı kullanılmaz; `int64` + oran
çarpımı `math/big.Rat` veya sabit noktalı aritmetikle yapılır.

> **KK-304:** `cost=0.35 USD, multiplier=1.0, fx=43.20, margin=40, fixedFee=0, minPrice=0`
> girdisi için sonuç **tam olarak** `2117` kuruş (21,17 ₺) döner. Altın test olarak sabitlenir.
>
> ⚠️ **Para birimi bir ÇIKARIMDIR.** HeroSMS'in fiyat, stok ve bakiye uç noktaları `currency`
> **döndürmüyor**; USD varsayımı yalnız `Currency.default = 840` üzerinden yapılmıştır → ❓H6b.
> Test geçerlidir ama varsayım açıkça kayıtlıdır.

### FR-305 · Fiyat teklifi (quote) `ZORUNLU`
`GET /catalog/quote` bir `price_quotes` kaydı oluşturur ve **yalnız** `quoteId`, fiyat, stok ve
son geçerlilik zamanını döner.
- Geçerlilik: **120 saniye**
- Tek kullanımlık (`consumed_at`)
- **`providerId` ve `cost` istemciye asla gönderilmez**

> **KK-305:** API yanıtında `providerId` veya maliyet bilgisi bulunmadığını doğrulayan bir test vardır.
> Süresi geçmiş teklifle satın alma `409 QUOTE_EXPIRED` döner.

### FR-306 · Sağlayıcı seçimi `ZORUNLU`
Aktif ve bu ürünü destekleyen tüm sağlayıcılara **paralel** sorulur (3 sn zaman aşımı).
Yanıt vermeyen elenir. `stock > 0` olanlar arasında düzeltilmiş maliyeti en düşük olan seçilir;
eşitlikte `providers.priority`.

> 🔴 **Stok tanımı (HeroSMS):** `counts.defaultPrice`.
> `counts.total` / `count` alanı **stok değildir** ve kullanılmaz — fiyat tavanı yoktur.
> ([provider-herosms.md](provider-herosms.md) §3.3)
>
> **Ölçümle kanıtlandı (10 Eylül 2026, 20.788 kombinasyon):** `counts.defaultPrice`, `map`
> merdiveninin `prices.default` ve altındaki kümülatif toplamına **20.788/20.788 eşittir**.
> Ölçüt `maxPrice = prices.default` politikamızla aynı şeyi ölçer — bu bir çıkarım değil.
>
> ⚠️ **`physical` ölçüt DEĞİLDİR.** 2026-09-08'de öyle seçilmişti (❓H16, canlı gözleme
> dayanıyordu) ve **satışı sessizce engelliyordu**: katalogun %28'i (5.910 kombinasyon)
> satılabilirken "stok yok" görünüyordu, Türkiye'de ise 123 kombinasyonun hiçbirinde pozitif
> değildi. `physical > 0` iken `defaultPrice = 0` olan 208 kombinasyon ölçüldü — hepsinde
> merdivenin en ucuz basamağı varsayılan fiyatın **üstünde**, yani `maxPrice`'ımızla zaten
> alınamazlar. Seçim skoruna `stats.percent` ve `offers.meta.order.deliverability` opsiyonel
> girdi olarak eklenebilir (düşük başarı oranı = yüksek iade = zarar).

> **KK-306:** İki sahte sağlayıcıdan biri 5 sn gecikirse, teklif 3 sn içinde diğeriyle döner.
> Hiçbiri yanıt vermezse `503 NO_PROVIDER_AVAILABLE`.
> **Ek test:** `counts.defaultPrice = 0` olan bir kombinasyon teklife **hiç girmez** — `total`
> ne kadar yüksek olursa olsun.
> → `api/internal/adapter/provider/herosms/herosms_stok_test.go#TestStokVarsayilanFiyatSayacindanGelir`

---

## 4. Sipariş akışı

### FR-400 · Satın alma `ZORUNLU`
`POST /orders` **yalnız** `{ quoteId }` alır. Gövdede fiyat, sağlayıcı veya ürün bilgisi kabul edilmez
(gönderilirse yok sayılır, uç nokta `additionalProperties: false` ile katı doğrulanır).

Akış: T1 (teklif kilitle + tüket + bakiye düş) → sağlayıcı çağrısı (transaction dışı) →
T2 (sipariş oluştur **veya** iade).

> **KK-400:** Sağlayıcı hata verdiğinde kullanıcının bakiyesi işlem öncesi değerine döner ve
> ledger'da bir `PURCHASE` + bir `REFUND` kaydı bulunur. Sipariş kaydı oluşmaz.

### FR-401 · Fiyat bütünlüğü `ZORUNLU`
Tahsil edilen tutar, teklifteki `sell_price_minor` ile **birebir** aynıdır. Satın alma anında fiyat
yeniden hesaplanmaz.

**Sağlayıcı sınırında zorlama:** Her satın alma çağrısında teklifteki maliyet `maxPrice` parametresi
olarak sağlayıcıya gönderilir. Fiyat yükselmişse satın alma **hiç gerçekleşmez** ve kullanıcıya
iade yapılır — beklenmedik tutar tahsili yapısal olarak imkânsızdır. ([provider-herosms.md](provider-herosms.md) §4 · ADR-018)

> 🔴 **`fixedPrice` GÖNDERİLMEZ.** *"Satın alma **kesinlikle** iletilen maksimum fiyat üzerinden
> yapılır"* — bu bir tavan değil **sabit fiyat**; piyasa fiyatı düşse bile tavan öderiz (ADR-027).
>
> ⚠️ `WRONG_MAX_PRICE` hatası **modern uç nokta için tanımlı değildir** (yalnız legacy
> `?action=getNumber`'ın 400'ünde belgeli). Modern tarafta fiyat aşımının hangi kodla geldiği
> **canlı test gerektirir** (❓H11/H12); adaptör 422/404 gövdesini ayrıştırır.

> **KK-401:** Teklif alındıktan sonra `pricing_rules` ve kur değiştirilse bile tahsil edilen tutar
> değişmez. Bunu doğrulayan bir entegrasyon testi vardır.
> **Ek test:** Sahte sağlayıcı `WRONG_MAX_PRICE` döndürdüğünde sipariş oluşmaz, bakiye tam iade edilir
> ve kullanıcıya "fiyat değişti, lütfen tekrar deneyin" mesajı gösterilir.

### FR-402 · Çift sipariş koruması `ZORUNLU`
`UNIQUE (provider_id, remote_order_id)` kısıtı. Ayrıca teklif tek kullanımlıktır.

> **KK-402:** Aynı `quoteId` ile iki eşzamanlı satın alma isteğinden **tam biri** başarılı olur,
> diğeri `409 QUOTE_CONSUMED` alır.

### FR-403 · Sipariş durumu ve SSE `ZORUNLU`
`GET /orders/:id/stream` SSE akışı açar. Kod geldiğinde `code` olayı yayınlanır.
- Keepalive: 20 saniyede bir yorum satırı
- Sahiplik kontrolü sorguda: `WHERE public_id = $1 AND user_id = $2`
- `EventSource` başarısızsa istemci 5 sn aralıklı yoklamaya düşer

> **KK-403:** Kullanıcı A, kullanıcı B'nin sipariş akışına bağlanmaya çalıştığında `404` alır
> (403 değil — kaynağın varlığını sızdırmamak için).
>
> ⚠️ `verificationType: "call"` siparişlerinde sağlayıcı `code` ve `text` alanlarını **null**
> döndürebilir; bu **"kod gelmedi" sayılmaz** ve hatalı iade tetiklemez.

### FR-404 · Sunucu tarafı yoklama `ZORUNLU`
`order-poller` işi `PENDING` siparişleri **`GET /activations` ile toplu** yoklar — **30 saniyede bir**
(webhook birincil, bu güvenlik ağı; ADR-020). `size` **max 25** olduğu için sayfalama zorunlu.
İstemci sağlayıcıya doğrudan gitmez.

> **KK-404:** 100 eşzamanlı bekleyen sipariş varken sağlayıcıya giden istek sayısı istemci
> sayısıyla **orantılı değildir** ve **tur başına ≤ 4 istek**tir (100 ÷ 25).

### FR-405 · Süre dolumu `ZORUNLU`
`orders.expires_at` **sağlayıcının satın alma yanıtındaki `expiredAt` alanından** alınır; koda
gömülmez. `order-expirer` işi (30 sn) süresi geçenleri iptal eder ve **tam iade** yapar.

> *Eski prototip 600 saniyeyi koda gömmüştü. HeroSMS yanıtı `expiredAt` içeriyor.*
> ⚠️ **Varsayılan süre spec'te hiçbir yerde yazmıyor**; `custom-durations` yalnız *istisnaları*
> dakika cinsinden verir. `orders.expires_at` = `expiredAt` **eksi güvenlik payı**.
>
> 🔴 **Süre dolduğunda sağlayıcının otomatik iade yapıp yapmadığı spec'te YOK** (❓H10).
> Bu yüzden `order-expirer` **her zaman açık bir `Cancel()`/`Finish()` çağırır** — "bırak süresi
> dolsun" stratejisine güvenilmez (FR-412).

> **KK-405:** Süre dolduğunda kullanıcı hiçbir işlem yapmasa bile bakiyesi geri yüklenir.
> İstemcinin gönderdiği süre bilgisi **hiçbir kararda kullanılmaz**.

### FR-406 · İptal ve iade `ZORUNLU`
Yalnız `PENDING` sipariş iptal edilebilir. İade tutarı **her zaman** `price_paid_minor`.

**Kullanıcıya iade koşulsuzdur ve anında yapılır.** Sağlayıcıya iptal bildirimi ayrı ve asenkron
bir adımdır; sonucu kullanıcının iadesini etkilemez.

```
kullanıcı iptali VEYA süre dolumu
   ├─ 1. ledger REFUND (+price_paid_minor)   ← KOŞULSUZ, HEMEN
   └─ 2. sağlayıcıya iptal bildir (asenkron)
         ├─ 204 başarılı                  → REFUNDED
         ├─ EARLY_CANCEL_DENIED           → RETRY_SCHEDULED  (info.minActivationTime, tipik 120 sn)
         ├─ FREE_CANCELLATION_EXPIRED     → DENIED (20 dk penceresi doldu) → GİDER
         ├─ OTP_RECEIVED                  → DENIED (kod zaten teslim edildi) → GİDER
         └─ ağ hatası                     → RETRY_SCHEDULED
```

> 🔴 **Kod GELDİYSE `Cancel()` değil `Finish()` çağrılır** — `POST /activations/{id}/finish`
> (*"para iadesi yapılmaz"*). Sağlayıcıya doğru sinyal vermek ve aktivasyonu açık bırakmamak için
> zorunludur (FR-412, ADR-023).

Yeni alanlar: `orders.provider_refund_status` (`PENDING|REFUNDED|DENIED|RETRY_SCHEDULED`) ve
`orders.provider_refund_amount_minor`.

> **KK-406:** Sağlayıcı iptal çağrısı hata verdiğinde kullanıcı **yine de tam iadesini alır**.
> `EARLY_CANCEL_DENIED` alan sipariş kuyruğa alınır ve tekrar denenir; kalıcı reddedilen
> iade gider olarak raporlanır.

### FR-406b · Sağlayıcı iade mutabakatı `ZORUNLU`
`provider-refund-retry` işi (2 dk) `RETRY_SCHEDULED` siparişleri yeniden dener. Kalıcı olarak
reddedilen iadeler `DENIED` işaretlenir ve kâr raporunda gider kalemi olur.

> **KK-406b:** Sağlayıcının iptal penceresi dardır ([provider-herosms.md](provider-herosms.md) §6.1):
> ilk **120 saniye** iptal edilemez, sonra **20 dakikalık** ücretsiz iptal penceresi vardır,
> OTP alınmışsa hiç edilemez. Bu durumda kullanıcı zaten iadesini almıştır; fark bizim
> zararımızdır ve **ölçülebilir olmalıdır**.
>
> ⚠️ Modern `DELETE`'in bu ret durumlarında **409 mu 422 mi döndürdüğü spec'te tanımsız**
> (`[204,401,404,422,500]`); iş kuralları yalnız legacy 409'unda belgeli. Retry işi
> **hem 409 hem 422** gövdesini ayrıştırmalıdır. → ❓H13

### FR-407 · Çift iade koruması `ZORUNLU`
`COMPLETED` veya `REFUNDED` bir siparişe iade yapılamaz.

> **KK-407:** İade edilmiş bir siparişe ikinci iade denemesi `409 INVALID_STATE_TRANSITION` döner
> ve ledger'da ikinci kayıt oluşmaz.

### FR-408 · Yetim provizyon kurtarma `ZORUNLU`
`orphan-hold-reaper` (60 sn): tüketilmiş ama siparişi olmayan teklifleri bulur, sağlayıcıya sorar,
siparişi oluşturur veya iade eder.

> **KK-408:** T1 sonrası süreç öldürüldüğünde (test: sağlayıcı çağrısında panic), bir sonraki
> çalışmada kullanıcının parası ya siparişe dönüşür ya iade edilir — **kaybolmaz**.

### FR-409 · Sipariş geçmişi `ZORUNLU`
Kullanıcı kendi siparişlerini sayfalı, filtreli (durum, tarih, servis) görür. Admin `orders:read_all`
izniyle tümünü görür.

### FR-410 · Sağlayıcı webhook'u `ZORUNLU`
`POST /api/v1/webhooks/herosms/<secret-segment>` uç noktası HeroSMS'in `sms-incoming` bildirimlerini alır.

**Güvenlik gereksinimleri — hepsi zorunlu:**
1. URL yolu **tahmin edilemez** olmalı: en az 128 bit rastgelelik, ortam değişkeninden
2. **Webhook gövdesindeki `code` alanına ASLA güvenilmez.** Bildirim yalnız tetikleyicidir;
   kod **`GET /activations/{id}/otp/last`** ile sağlayıcıdan teyit edilerek alınır (ADR-019, ADR-022).
   🔴 *Tekil `GET /activations/{id}` diye bir uç nokta **yoktur** — o yolda yalnız `delete` tanımlı.*
   Teyit **boş dönerse durum DEĞİŞTİRİLMEZ**, sipariş `PENDING` kalır.
3. `activationId` bizim bir siparişimize ait değilse sessizce `200` dönülür ve log'lanır
4. Uç noktada hız limiti
5. Her zaman `200` dönülür (aksi halde sağlayıcı ≥7 kez daha dener)
6. Kimlik doğrulama middleware'i **uygulanmaz** (sağlayıcı oturum taşımaz)
7. **Kaynak IP izin listesi ZORUNLU:** `84.32.223.53`, `185.138.88.87` — env'den okunur,
   **gerçek peer IP**'ye bakılır, `X-Forwarded-For`'a değil
8. **Handler 3 saniye içinde `200` döner**; dedup, teyit ve yazım `webhook-ingest` kuyruğunda
   asenkron yapılır (ADR-024). Sağlayıcı zaman aşımı 3 sn'dir
9. **Dedup:** birincil anahtar `(activationId, id)`; `id` gelmezse
   `SHA-256(activationId + receivedAt + text)`. Aynı SMS **en az 8 kez** gelebilir
10. **Kod çıkarımı kendi regex'imizle** yapılır — `code` alanı `required` değil ve `text` nullable.
    Kod çıkarılamazsa sipariş `PENDING` kalır, **asla `COMPLETED` olmaz**

> **KK-410:** Gövdesinde geçerli bir `code` bulunan **sahte** bir webhook isteği (izin listesindeki
> bir IP'den gelmiş gibi) gönderildiğinde sipariş **tamamlanmaz** — sistem `/otp/last` ile teyit
> ister ve teyit boş dönerse durumu değiştirmez. İzin listesi dışı IP'den gelen istek hiç işlenmez.
> Bu testi geçmeden webhook uç noktası üretime alınmaz.

> ⚠️ HeroSMS **imza (HMAC) doğrulaması sunmuyor**, webhook URL'i **API ile kaydedilemiyor**
> (panelden elle, en fazla **3 HTTPS URL**, slotlar **hesap geneli**) ve webhook gövdesindeki
> `id` alanı `required` listesinde olmasına rağmen `properties` içinde **tanımsızdır**.
> ([provider-herosms.md](provider-herosms.md) §7.1)

### FR-411 · Kod teslim zinciri `ZORUNLU`
Kod üç halkalı bir zincirle kullanıcıya ulaşır:
`sağlayıcı webhook → (teyit) → Redis Pub/Sub → SSE → tarayıcı`

Güvenlik ağı: `order-poller` işi 30 saniyede bir, webhook'u gelmemiş `PENDING` siparişleri yoklar.

**Ek kurallar:**
- Teyit **boş dönerse durum değiştirilmez**
- 🔴 **SSE ilk koddan sonra KAPANMAZ** — `otpList` bir dizidir, ikinci/üçüncü mesaj gelebilir
  (`canGetAnotherSms`). Akış terminal durumda kapanır, ilk kodda değil
- `verificationType: "call"` dalında `code`/`text` **null olabilir** → "kod gelmedi" sayılmaz

> **KK-411:** Webhook hiç gelmezse kod en geç 30 saniye içinde yoklama ile bulunur ve SSE'ye
> yayınlanır. Bu test webhook uç noktası devre dışı bırakılarak koşulur.
> **Ek test:** Aynı siparişe ikinci bir SMS geldiğinde SSE ikinci kodu da yayınlar.

### FR-412 · Aktivasyon kapatma garantisi `ZORUNLU`
Terminal duruma geçen **her** sipariş, sağlayıcı tarafında da mutlaka kapatılır:
- Kod geldi → `POST /activations/{id}/finish` (iade yok)
- Kod gelmedi → `DELETE /activations/{id}` (iade talebi)

**"Bırak süresi dolsun" stratejisi yasaktır** — `DELETE` açıklaması açıkça `finish`'e yönlendiriyor
ve süresi dolan aktivasyonun akıbeti spec'te tanımsız. `activation-reaper` işi (60 sn)
`provider_closed_at IS NULL` olan terminal siparişleri bulup kapatır.

> **KK-412:** Hiçbir terminal sipariş, sağlayıcı tarafında kapatılmamış bir aktivasyon bırakmaz.
> Test: 20 sipariş oluştur, çeşitli yollarla sonlandır, `provider_closed_at IS NULL` sayısı **0**.

### FR-413 · Sağlayıcı eşzamanlılık limiti `ZORUNLU`
Satın alma çağrıları **semafor** ile sınırlanır. `403 CHANNELS_LIMIT` alındığında
`info.max_allowed` okunup semafor boyutu güncellenir. Bu, hız limitinden (`429`) **ayrı** bir
mekanizmadır ve ayrı ele alınır.

> **KK-413:** Semafor boyutu aşıldığında istekler kuyruğa alınır, sağlayıcıya `CHANNELS_LIMIT`
> aldıracak şekilde gönderilmez.

### FR-414 · `Retry-After` uyumu `ZORUNLU`
`429`, `425` ve `info.retry_after_seconds` içeren hatalarda **sabit backoff kullanılmaz** —
sağlayıcının verdiği süre beklenir. `Retry-After`, spec'teki **tek** yanıt başlığıdır.

> **KK-414:** `Retry-After: 60` dönen sahte bir sağlayıcı yanıtında sistem 60 saniye bekler,
> kendi backoff değerini kullanmaz.

### FR-415 · Çok mesajlı sipariş `ZORUNLU`
`order_messages` bir siparişe ait **N** mesaj tutar. Sipariş görünümü "son kod" ile "tüm kodlar"
ayrımı yapar; SSE ilk koddan sonra yayına devam eder.

**Mesajlar bizde kalıcıdır:** sonlandırılmış veya iade edilmiş bir aktivasyondan geçmiş mesaj
**okunamaz** (`409 ACTIVATION_NOT_ACTIVE`: *"terminated/refunded and Otp cannot be retrieved"*).
Sağlayıcı bir arşiv değildir.

> **KK-415:** Bir siparişe iki SMS geldiğinde ikisi de saklanır ve arayüzde görünür.
> Sipariş kapatıldıktan sonra da geçmiş mesajlar okunabilir.

### FR-416 · İptal penceresi arayüz kısıtı `ZORUNLU`
İptal butonu satın almadan sonra **en az 120 saniye pasif** kalır; kalan süre **sunucudan** gelir
(istemci sayacından değil). Aksi halde sistematik olarak reddedilen iptal kaydı üretilir ve
karşılanmayan iade olarak kâr sızar.

> **KK-416:** Satın almadan 10 saniye sonra iptal butonu devre dışıdır ve geri sayım gösterir.
> ⚠️ `minActivationTime: 120` değerinin sabit mi servis/ülkeye göre değişken mi olduğu
> **spec'te yok** (❓H2) → değer sağlayıcı yanıtından okunur, sabit kodlanmaz.

### FR-417 · Kiralıkta ürün SÜREDİR `ZORUNLU`
Kiralık siparişe gelen ilk SMS dönemi **bitirmez**: sipariş `PENDING → ACTIVE` olur, sağlayıcıda
kapatılmaz ve dönem boyunca gelen her mesajı almaya devam eder. Dönem sonunda `EndRental`
siparişi `COMPLETED` yapar ve **deftere hiçbir kayıt yazmaz** — kod gelmemiş olması iade sebebi
değildir, çünkü satılan şey koddur değil süredir.

> **KK-417a:** Kiralığa üç mesaj teslim edilir; üçü de kaydedilir, üçü de SSE'den yayınlanır ve
> sağlayıcıya hiçbir kapatma çağrısı gitmez.
> **KK-417b:** Hiç mesaj almamış bir kiralık dönem sonunda `COMPLETED` olur, `REFUND` defter
> satırı **sıfırdır** ve bakiye değişmez.

### FR-418 · Kiralık iptal ve iade penceresi `ZORUNLU`
Kiralık sipariş yalnız satın almadan sonraki **15 dakika** içinde ve **hiç mesaj gelmemişken**
iptal edilebilir. Pencerenin üst sınırı sipariş satırında (`refundable_until`) saklanır;
**boş bırakılamaz** — hem veritabanı kısıtı (`order_rental_has_refund_window`) hem domain
kontrolü bunu zorlar. Eksik veri "sınır yok" değil **"izin yok"** anlamına gelir.

> **KK-418a:** 29 gün kullanılmış bir kiralığın iptal isteği reddedilir ve deftere hiçbir kayıt
> yazılmaz — `refundable_until` NULL olsa bile.
> **KK-418b:** Kiralık bir sipariş satırının `refundable_until` alanı `NULL` yapılamaz (23514).

### FR-419 · Sağlayıcı ödenen süreyi teslim etmeli `ZORUNLU`
Satın alma yanıtı sipariş yazılmadan **önce** doğrulanır: `subtype` kiralığı teyit etmeli ve
`expiredAt` ödenen dönemi (tolerans **5 dakika**) kapsamalı. Sağlamıyorsa sipariş **hiç
yazılmaz**; numara sağlayıcıda kapatılır ve para iade edilir.

> **KK-419a:** 720 saatlik kiralık istenip 20 dakikalık aktivasyon dönerse `POST /orders`
> hata verir, sipariş satırı oluşmaz, bakiye satın alma öncesine döner ve `Σ defter == bakiye`
> korunur.
> **KK-419b:** 1 dakikalık sapma satın almayı **düşürmez** (yanlış alarm üretmez).

### FR-420 · Kiralık yoklaması açlığa düşmez `ZORUNLU`
`rental-poller` turu **dönüşümlüdür**: sıralama `last_polled_at NULLS FIRST` ve her tur
işlediği satırları damgalar. Sabit sıralamalı bir `LIMIT`, dönem boyunca değişmeyen bir kümede
limit üstündeki kiralıkları hiç yoklamazdı.

> **KK-420:** Limitin iki katı canlı kiralıkla iki tur koşulduğunda, **en yeni** kiralık da
> yoklanmış olur.

---

## 5. Bakiye yükleme

### FR-500 · Havale ile yükleme `ZORUNLU`
Kullanıcı tutar + açıklama + **dekont görseli** yükler. `deposits(PENDING)` oluşur.
- Dosya: JPEG/PNG/PDF, ≤ 5 MB, **sihirli bayt** doğrulaması (uzantı ve MIME başlığı yeterli değil)
- Web kökü **dışına** kaydedilir, rastgele isimle; erişim imzalı URL ile

> **KK-500:** `.php` uzantılı, `image/jpeg` MIME başlığı gönderilen bir dosya reddedilir.
> Yüklenen dosya doğrudan bir URL ile servis edilemez.

### FR-501 · USDT ile yükleme `ZORUNLU`
Kullanıcı ağ (TRC20/ERC20) seçer, gösterilen adrese gönderir, **TX hash** girer.
Onay anında USDT/TRY kuru kaydedilir; TL karşılığı `credited_minor` olur.

> **KK-501:** Aynı `tx_hash` ile ikinci bir yükleme talebi `409` döner.

### FR-502 · Onay `ZORUNLU`
Yalnız `deposits:approve` izniyle, **POST** ile, idempotent. Tek transaction:
`deposits.status=COMPLETED` + `ledger DEPOSIT` + `audit_log`.

> **KK-502:** Yetkisiz kullanıcının onay isteği 403. Aynı yüklemeye 100 eşzamanlı onay çağrısı
> bakiyeyi **bir kez** artırır. `GET` ile onay uç noktası **mevcut değildir**.

### FR-503 · Red `ZORUNLU`
Red nedeni zorunlu, kullanıcıya gösterilir, audit log'a yazılır. Bakiye değişmez.

### FR-504 · Manuel bakiye düzeltme `ZORUNLU`
Admin `ADJUSTMENT` tipiyle bakiye ekleyip düşebilir. Not alanı **zorunlu**. Audit log'a yazılır.

> **KK-504:** Düzeltme kaydında `created_by_user_id` ve `note` boş olamaz.

---

## 6. Destek (ticket)

### FR-600 · Talep oluşturma / yanıtlama `ZORUNLU`
Kullanıcı konu + öncelik + mesaj ile talep açar. Admin `tickets:reply` iznine sahipse yanıtlar.
Durum: `OPEN → ANSWERED → USER_REPLIED → CLOSED`.

> **KK-600:** Personel yanıtı `is_staff = true` ile kaydedilir. *(Mevcut sistemde bu bayrak var olmayan
> bir alandan okunduğu için her zaman `false`.)* Kullanıcı başkasının talebini göremez ve yanıtlayamaz.

### FR-601 · Müşteri yorumu — gönderme `ZORUNLU`
Oturum açmış ve **e-postası doğrulanmış** kullanıcı, panelinden 1–5 puan + 10–1000 karakter metin
ile yorum gönderir. Yorum her zaman `PENDING` başlar; durum istemciden alınmaz.

**Kabul kriterleri**
- Kullanıcının aynı anda **en fazla bir** `PENDING` yorumu olabilir; ikinci gönderim `409` döner.
  Sınır kısmi benzersiz indekstedir — N eşzamanlı istekten tam olarak biri yazılır.
  → `internal/service/review/review_integration_test.go#TestConcurrentSubmitsLeaveOnePending`
- Uzunluk **karakter (rune)** ile ölçülür; 1000 Türkçe karakterlik yorum kabul edilir.
  → `dto/review_test.go#TestTurkishBodyIsMeasuredInRunes`
- Gövdeye `"status":"APPROVED"` konsa bile yorum `PENDING` kaydedilir.
  → `handler/review_integration_test.go#TestClientCannotSubmitApprovedReview`

### FR-602 · Müşteri yorumu — moderasyon `ZORUNLU`
Yönetici `reviews:read` ile kuyruğu görür, `reviews:moderate` ile karar verir.
Durum: `PENDING → APPROVED | REJECTED`, ayrıca `APPROVED → REJECTED` (yayından kaldırma).
`REJECTED` **terminaldir**. Onay ve red **POST**'tur (değişmez #8).

**Kabul kriterleri**
- Red gerekçesi zorunludur (`422`) ve kullanıcıya gösterilir; onayda gerekçe alanı temizlenir.
  → `handler/review_integration_test.go#TestRejectRequiresReason`
- Geçersiz geçiş hem serviste hem **veritabanı tetikleyicisinde** reddedilir.
  → `internal/service/review/review_integration_test.go#TestInvalidReviewTransitionIsRejectedByDB`
- Onaylanmış yorumun metni ve puanı değiştirilemez (DB tetikleyicisi).
  → `internal/service/review/review_integration_test.go#TestReviewBodyIsImmutable`
- Yetki matrisi: oturumsuz `401` · izinsiz `403` · başkasının yorumu `404`.
  Yalnız `reviews:read` izni onay/red için **yetmez**.
  → `handler/review_integration_test.go#TestReviewAuthMatrix`

> **KK-601:** Sitede gösterilen yorum yanıtında **kullanıcı e-postası bulunmaz** ve sorgu o sütunu
> seçmez. Yalnız kullanıcı adı, puan, metin ve yayın tarihi döner.
> → `handler/review_integration_test.go#TestPublicReviewsNeverExposeEmail`

### FR-603 · Müşteri yorumu — sitede gösterim `ZORUNLU`
`GET /catalog/reviews` oturumsuzdur ve **yalnız `APPROVED`** yorumları döner. Bir kullanıcının
birden fazla onaylı yorumu varsa yalnız **en sonu** gösterilir (`DISTINCT ON (user_id)`).

**Kabul kriterleri**
- Onay bekleyen ve reddedilen yorum sitede **hiç** görünmez.
  → `handler/review_integration_test.go#TestRejectedReviewNeverReachesTheSite`
- Onaylı yorum yoksa arayüz bölümü **hiç render edilmez**; yer tutucu, iskelet ya da örnek yorum
  gösterilmez. Tohum verisi olarak sahte yorum **eklenmez**.
- Ortalama puan JSON'da kayan nokta değil, onda birlik tam sayı olarak taşınır (`averageX10`).

---

## 7. Yönetim paneli

### FR-700 · Kullanıcı yönetimi `ZORUNLU`
Listeleme (arama/filtre), detay, rol atama, askıya alma, bakiye düzeltme. Her değişiklik audit log'lu.

### FR-701 · Sağlayıcı yönetimi `ZORUNLU`
Ekleme/düzenleme: ad, **protokol**, base URL, API anahtarı (şifreli saklanır, maskeli gösterilir),
aktiflik, öncelik, maliyet çarpanı, yetenekler.

> **KK-701:** API anahtarı hiçbir API yanıtında veya HTML kaynağında tam olarak görünmez.

### FR-702 · Boyut eşleştirme ekranı `ZORUNLU`
Ülke ve servis eşleştirmelerini elle düzenleme + `catalog-sync` ile otomatik doldurma.
Eşleşmeyen kayıtlar açıkça listelenir.

> ⚠️ Ülke/servis/operatör **adları yalnız legacy uç noktalarından** gelir (modern REST'te yok).
> `?action=getServicesList&lang=tr` Türkçe adları destekler.
> **Operatör boyutu fiyatlanamaz** — `operator = 'any'` sabitlenir (ADR-029).

### FR-703 · Fiyat kuralı yönetimi `ZORUNLU`
Kural ekleme/düzenleme + **canlı önizleme**: seçilen ürün için mevcut maliyetle satış fiyatı gösterilir.

### FR-704 · Bakiye talepleri `ZORUNLU`
Bekleyen yüklemeler, dekont/TX görüntüleme, onay/red.

### FR-705 · Denetim kaydı `ZORUNLU`
`audit:read` izniyle: aktör, işlem, varlık, öncesi/sonrası, IP, zaman. Filtrelenebilir.

### FR-706 · Kâr raporu `ÖNERİLEN`
Tarih aralığında: sipariş sayısı, toplam maliyet (TL karşılığı), toplam satış, brüt marj,
iade tutarı, sağlayıcı bazlı kırılım.

---

## 8. Fonksiyonel olmayan gereksinimler

### NFR-800 · Başarım `ZORUNLU`
| Uç nokta | p95 hedefi |
|---|---|
| Fiyat teklifi (`/catalog/quote`) | < 1500 ms |
| Satın alma (`/orders`) | < 3000 ms |
| Panel sayfa yükleme (TTFB) | < 400 ms |
| Diğer API okuma uç noktaları | < 200 ms |

> **KK-800:** Yük testinde (k6, 50 eşzamanlı kullanıcı) hedefler karşılanır.

### NFR-801 · Güvenilirlik `ZORUNLU`
- Sağlayıcı çökmesi kullanıcıya 500 olarak yansımaz; tipli hata + Türkçe mesaj döner
- Redis çökerse: oturumlar düşer (kabul edilebilir), **para işlemleri etkilenmez** (Postgres'te)
- Postgres çökerse: sistem `503` döner, **yanlış veri yazmaz**

### NFR-802 · Güvenlik `ZORUNLU`
- Tüm sırlar ortam değişkeninde; açılışta doğrulanır, eksikse **süreç başlamaz**
- Sağlayıcı API anahtarları DB'de AES-GCM ile şifreli
- Durum değiştiren tüm işlemler `POST`/`PATCH`/`DELETE`
- CSRF koruması aktif
- Hız limiti: `/auth/*` **30 istek/dk/IP** · `/catalog/quote` 60/dk/kullanıcı · `/orders` 10/dk/kullanıcı
  > ⚠️ IP bazlı limit **kaba bir emniyet supabıdır**. Türkiye'de mobil operatörler binlerce
  > aboneyi tek IP'nin (CGNAT) arkasına koyar; sıkı bir IP limiti meşru kullanıcıları kilitler.
  > Kaba kuvvete karşı **asıl savunma hesap bazlı kilittir**: 5 başarısız deneme / 15 dk (KK-102).
- `govulncheck` ve `npm audit` CI'da; yüksek/kritik açık derlemeyi durdurur
- **Webhook uç noktası:** IP izin listesi + tahmin edilemez yol + auth middleware yok +
  her zaman `200` + hız limiti (FR-410)
- **`resellerUserId`** olarak sağlayıcıya **anlamsız kimlik** (ULID) gönderilir; e-posta veya
  kullanıcı adı **asla** — KVKK envanterine giren bir kişisel-veri kanalıdır
- **Legacy sağlayıcı çağrılarının tam URL'i log'a yazılmaz** (API anahtarı sorgu dizesinde)

> **KK-802:** Yetkilendirme testleri **her** uç nokta için vardır: (a) oturumsuz → 401,
> (b) yetkisiz rol → 403, (c) başkasının kaynağı → 404.

### NFR-803 · Gözlemlenebilirlik `ZORUNLU`
JSON log + `request_id`; Sentry; Prometheus `/metrics`; `/healthz` + `/readyz`.
`design.md` §12'deki alarmlar tanımlı ve test edilmiş.

### NFR-804 · Test `ZORUNLU`
| Alan | Hedef |
|---|---|
| `internal/domain/**` (para, fiyat, durum) | ≥ %95 |
| `internal/service/**` (kullanım senaryoları) | ≥ %80 |
| Sağlayıcı adaptörleri | Kaydedilmiş yanıtlarla sözleşme testi, %100 uç nokta kapsamı |
| Uçtan uca | Kayıt → yükleme → satın alma → kod → iade akışı |

> **KK-804:** CI'da tüm testler yeşil olmadan `main`'e birleştirme yapılamaz.

### NFR-805 · Erişilebilirlik `ÖNERİLEN`
WCAG 2.1 AA: klavye ile tam gezinme, odak göstergesi, form etiketleri, kontrast oranı ≥ 4.5:1,
SSE ile gelen kodun ekran okuyucuya duyurulması (`aria-live`).
> Ayrıntı: [frontend-contract.md](frontend-contract.md) §6

### NFR-806 · i18n `ZORUNLU`
Kullanıcıya görünen **tüm** metinler `web/messages/tr.json` içinde. Bileşenlerde sabit metin yok.
Para/tarih biçimlemesi tek bir modülde.

> **KK-806:** Bir lint kuralı, JSX içinde sözlükten gelmeyen Türkçe metin bulursa uyarır.

### NFR-807 · Veri saklama ve KVKK `ZORUNLU`
- IP ve tarayıcı bilgisi 90 gün
- Hesap silme talebi: kişisel veriler anonimleştirilir, **ledger kayıtları saklanır** (mali kayıt)
- Aydınlatma metni ve açık rıza akışı kayıt ekranında

### NFR-808 · Yedekleme `ZORUNLU`
Günlük otomatik Postgres yedeği, şifreli, ayrı konumda, 30 gün saklama.
**Geri yükleme yılda en az bir kez test edilir ve sonucu kayda geçer.**

---

### NFR-809 · Mobil responsive `ZORUNLU`
Uygulama **mobile-first** yazılır. Test genişlikleri: 320 · 390 · 430 · 768 · 1440 px.
- 320 px'te **yatay kaydırma olmamalı**
- Tüm dokunma hedefleri ≥ 44×44 px
- Girdi alanları ≥ 16px font (iOS yakınlaşmasını önler)
- Tablolar `< 768px` altında kart listesine dönüşür — yatay kaydırılan tablo kabul edilmez
- Sabit alt öğelerde `env(safe-area-inset-bottom)` uygulanır
- `100dvh` kullanılır, `100vh` yalnız yedek

> **KK-809:** Playwright `webkit-mobile` (iPhone 14) projesinde tüm uçtan uca akışlar geçer.
> Otomatik kontrol: her sayfada `scrollWidth <= clientWidth` (320 px'te).
> Ayrıntılı kurallar: [frontend-contract.md](frontend-contract.md) §2

### NFR-810 · Tarayıcı uyumluluğu `ZORUNLU`
Desteklenen: Safari/iOS Safari 16.4+ · Chrome/Edge son 2 · Firefox son 2 · Samsung Internet 23+.
- Tüm HTTP çağrıları **tek bir istemci sarmalayıcısından** geçer (`web/src/lib/api/client.ts`)
- Sarmalayıcı zorunlu davranışları: zaman aşımı (15 sn), `AbortController`, `credentials` açık,
  JSON olmayan yanıtın (vekil HTML hatası) yakalanması, hata normalizasyonu, koşullu `Content-Type`
- **Mutasyonlar asla otomatik yeniden denenmez;** `GET` üstel geri çekilme ile 3 kez
- Tarih ayrıştırma yalnız RFC 3339 (Safari boşluklu formatı ayrıştıramaz)
- Para ve tarih biçimleme tek modülde, `timeZone` açıkça `Europe/Istanbul`

> **KK-810:** Uçtan uca testler `chromium-desktop`, `webkit-desktop`, `chromium-mobile`,
> `webkit-mobile` projelerinin **dördünde de** geçer. Sunucu 502 HTML döndürdüğünde kullanıcı
> anlamlı Türkçe hata görür, `SyntaxError` almaz.
> Ayrıntılı kurallar: [frontend-contract.md](frontend-contract.md) §3, §5

### NFR-811 · SSE dayanıklılığı `ZORUNLU`
Kod bekleme akışı aşağıdakilerin **tamamını** karşılamadan üretime alınmaz:
- **HTTP/2 zorunlu** (HTTP/1.1'de origin başına 6 bağlantı sınırı uygulamayı kilitler)
- Caddy `flush_interval -1` + Go `X-Accel-Buffering: no` + her yazımda `Flusher.Flush()`
- Sunucu 20 sn'de bir keepalive; istemci 25 sn sessizlikte bağlantıyı ölü sayar
- İstemci: üstel geri çekilme ile yeniden bağlanma (1→2→4→8s, üst sınır 15s)
- `visibilitychange` ve `pageshow` (bfcache) olaylarında REST ile senkron + yeniden bağlanma
- **Yoklama yedeği:** 10 sn içinde akış kurulamazsa 5 sn aralıklı `GET` ile devam edilir
- Kalan süre **her zaman** sunucudaki `expiresAt` ile hesaplanır, yerel sayaçla değil
- Terminal durumda bağlantı kapatılır (asılı bağlantı bırakılmaz)

> **KK-811:** Gerçek iPhone'da numara alındıktan sonra uygulama 2 dakika arka plana atılıp geri
> dönüldüğünde kod görünür. SSE ağ katmanında engellendiğinde kod yoklama ile yine gelir.
> Kabul testi listesi: [frontend-contract.md](frontend-contract.md) §4.4

### NFR-812 · Performans bütçesi (mobil, 4G) `ZORUNLU`
LCP < 2,5 sn · INP < 200 ms · CLS < 0,1 · ilk JS paketi (gzip) < 150 KB.

> **KK-812:** CI'da Lighthouse bütçe kontrolü; bütçe aşılırsa derleme uyarı verir.

### NFR-813 · Teknik SEO `ZORUNLU`
Genel (oturumsuz) sayfalar arama motorları için eksiksiz işaretlenir; oturum
arkasındaki her şey **indekslenmez**.

- `(panel)` ve `(admin)` rota grupları `robots: { index: false, follow: false }`
  taşır **ve** `robots.txt` ile engellenir
- Her genel sayfada benzersiz `title` + `description` + `canonical`
- `sitemap.xml` ve `robots.txt` dinamik üretilir; sitemap **yalnız** genel sayfaları içerir
- JSON-LD: `Organization`, `WebSite`, `BreadcrumbList`, fiyat sayfalarında `Product`+`Offer`
- Genel sayfalar **SSG/ISR** ile render edilir (istemci tarafı içerik indekslenmez)
- Anlamsal HTML, tek `h1`, hiyerarşik başlıklar, görsellerde `alt`

> **KK-813:** Lighthouse SEO puanı **100** (mobil ve masaüstü). Otomatik testler:
> (a) hiçbir panel yolu `sitemap.xml`'de yok, (b) panel sayfaları `noindex` taşıyor,
> (c) iki genel sayfa aynı `title`'ı taşımıyor, (d) JS devre dışıyken genel sayfa
> içeriği görünüyor.
> Ayrıntılı kurallar: [frontend-contract.md](frontend-contract.md) §10

### NFR-814 · Programatik sayfa kalitesi `ZORUNLU`
Servis×ülke kombinasyonları için üretilen sayfalar yalnız şu üçü sağlanırsa
yayınlanır: (1) gerçek fiyat/stok verisi, (2) o kombinasyona özgü en az 150 kelime
özgün içerik, (3) stok mevcut. Aksi halde sayfa üretilmez veya `noindex` alır.

> **Gerekçe:** Şablonla üretilmiş, yalnız ad değişen binlerce sayfa Google'ın
> "doorway pages" ve "thin content" politikalarına girer; sonuç sıralama değil
> **cezadır**. v1'de en çok aranan ~50 kombinasyon elle içerikle üretilir.

---

## 9. API sözleşmesi

Kaynak doğruluk: `api/openapi/openapi.yaml`. Go sunucu iskeleti ve Next.js tipleri **oradan üretilir**.

### Genel kurallar
- Taban yol: `/api/v1`
- Kimlik: `sid` çerezi (Authorization başlığı v1'de yok)
- Para: `{ "minor": 1250, "currency": "TRY", "formatted": "12,50 ₺" }` — **asla çıplak ondalık sayı**
- Zaman: RFC 3339 UTC
- Kimlikler: dışarıda **UUID** (`public_id`), asla sayısal `id`
- Hata: `{ "error": { "code", "message", "requestId" } }`
- Sayfalama: `?limit=&cursor=` → `{ "items": [], "nextCursor": null }`
- Katı doğrulama: bilinmeyen alan içeren gövde `422` döner

### Uç noktalar (özet)

```
POST   /api/v1/auth/register              FR-100
POST   /api/v1/auth/verify-email          FR-101
POST   /api/v1/auth/login                 FR-102
POST   /api/v1/auth/logout
POST   /api/v1/auth/password/forgot       FR-103
POST   /api/v1/auth/password/reset        FR-103
GET    /api/v1/me
GET    /api/v1/me/sessions                FR-104
DELETE /api/v1/me/sessions/:id            FR-104

GET    /api/v1/catalog/services           FR-300
GET    /api/v1/catalog/countries          FR-300
GET    /api/v1/catalog/quote              FR-305   ⟵ providerId/cost DÖNMEZ
POST   /api/v1/orders                     FR-400   ⟵ gövde: { quoteId } SADECE
GET    /api/v1/orders                     FR-409
GET    /api/v1/orders/:id
GET    /api/v1/orders/:id/stream          FR-403   (text/event-stream)
POST   /api/v1/webhooks/herosms/{secret}  FR-410   ⟵ kimliksiz, teyitli, her zaman 200
POST   /api/v1/orders/:id/cancel          FR-406

GET    /api/v1/wallet/balance
GET    /api/v1/wallet/entries             FR-206
GET    /api/v1/wallet/deposit-methods
POST   /api/v1/wallet/deposits            FR-500/501

GET    /api/v1/tickets                    FR-600
POST   /api/v1/tickets
GET    /api/v1/tickets/:id
POST   /api/v1/tickets/:id/messages

GET    /api/v1/catalog/reviews            FR-603   ⟵ OTURUMSUZ, yalnız APPROVED
POST   /api/v1/reviews                    FR-601
GET    /api/v1/reviews/mine               FR-601
GET    /api/v1/reviews/:id                FR-601

--- admin (izin gerektirir) ---
GET    /api/v1/admin/users                users:read
PATCH  /api/v1/admin/users/:id            users:write
POST   /api/v1/admin/users/:id/balance    users:write   FR-504
GET    /api/v1/admin/deposits             deposits:read
POST   /api/v1/admin/deposits/:id/approve deposits:approve  FR-502  ⟵ POST, GET DEĞİL
POST   /api/v1/admin/deposits/:id/reject  deposits:approve  FR-503
GET    /api/v1/admin/providers            providers:read
POST   /api/v1/admin/providers            providers:write
PATCH  /api/v1/admin/providers/:id        providers:write
GET    /api/v1/admin/dimension-maps       providers:read    FR-702
PUT    /api/v1/admin/dimension-maps       providers:write
POST   /api/v1/admin/providers/:id/sync   providers:write
GET    /api/v1/admin/pricing-rules        pricing:read      FR-703
POST   /api/v1/admin/pricing-rules        pricing:write
POST   /api/v1/admin/pricing-rules/preview pricing:read
GET    /api/v1/admin/reviews              reviews:read      FR-602
POST   /api/v1/admin/reviews/:id/approve  reviews:moderate  FR-602  ⟵ POST, GET DEĞİL
POST   /api/v1/admin/reviews/:id/reject   reviews:moderate  FR-602
GET    /api/v1/admin/audit-logs           audit:read        FR-705
GET    /api/v1/admin/reports/profit       orders:read_all   FR-706
```

### SSE olay formatı
```
event: status
data: {"orderId":"uuid","status":"PENDING","expiresAt":"..."}

event: code
data: {"orderId":"uuid","status":"COMPLETED","code":"123456","fullText":"..."}

event: cancelled
data: {"orderId":"uuid","status":"REFUNDED","refunded":{"minor":1250,"currency":"TRY"}}

: keepalive
```

---

## 10. Doğrulama kuralları (özet)

| Alan | Kural |
|---|---|
| `username` | 3–32, `^[a-zA-Z0-9_]+$`, benzersiz (BH duyarsız) |
| `email` | RFC 5322, ≤ 254, benzersiz (BH duyarsız) |
| `password` | ≥ 10 karakter, yaygın şifre listesinde olmamalı |
| `quoteId` | Geçerli UUID, kullanıcıya ait, süresi geçmemiş, tüketilmemiş |
| `amount` (yükleme) | ≥ 10,00 ₺ · ≤ 50.000,00 ₺ · tam sayı kuruş |
| `tx_hash` | Ağa göre biçim kontrolü; benzersiz |
| Dekont | JPEG/PNG/PDF, ≤ 5 MB, sihirli bayt doğrulaması |
| `margin_percent` | 0 ≤ x ≤ 1000, iki ondalık |
| Ticket konusu | 5–120 karakter |
| Ticket mesajı | 1–5000 karakter |
| Sayfalama `limit` | 1–100, varsayılan 20 |

---

## 11. v1.1 için hazır bırakılanlar

| Öğe | v1'de var olan | v1.1'de yapılacak |
|---|---|---|
| Kiralık numara | **Arka uç yaşam döngüsü TAMAM** (FR-417…FR-420): `ACTIVE` durumu, `rental_details`, dönem sonu iadesizliği, 15 dk iptal penceresi, dönüşümlü yoklama. **Doğrulandı:** HeroSMS'te kiralık = aynı satın alma uç noktası + `duration` (24..4320 sa), `subtype: 2` | UI (panel `ACTIVE`'i tanımıyor), süre bazlı fiyatlandırma, `prolong` akışı |
| Referans/affiliate | `users.referral_code`, `users.referred_by_user_id`, `referrals`, `referral_rules`, `LedgerType.COMMISSION` | Davet linki, komisyon hesaplama işi, raporlama ekranı |
| İkinci sağlayıcı | `providers.protocol`, adaptör kayıt defteri, sözleşme testi altyapısı | 5sim adaptörü + gerçek hesap |
| Kart ödemesi | `deposit_methods.kind` genişletilebilir | Üye işyeri entegrasyonu (yasal ön koşul — `roadmap.md` M9) |
| E-posta / sesli doğrulama ürünü | `products.kind` yeni değer alır; HeroSMS `/emails` ve `verificationType: "call"` destekliyor — **şema değişikliği gerekmez** | Katalog, fiyatlandırma, UI |

---

**İlgili:** [intent.md](intent.md) · [design.md](design.md) · [provider-herosms.md](provider-herosms.md) · [frontend-contract.md](frontend-contract.md) · [roadmap.md](roadmap.md) · [memory.md](memory.md) · [../CLAUDE.md](../CLAUDE.md)
