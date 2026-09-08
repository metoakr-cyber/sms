# provider-herosms.md — HeroSMS Sağlayıcı Entegrasyon Referansı

> **Doküman amacı:** HeroSMS adaptörünü yazmak için gereken bilgi.
> **İşaret kuralı:** ✅ = spec'ten birebir alıntı · ⚠️ = çıkarım veya belirsiz · 🔴 = tuzak · ❓ = canlı test gerekli.
> Bir şey spec'te yoksa öyle yazar; uydurulmaz.
>
> **Durum:** doğrulanmış (spec) + kısmen çıkarım · **Son güncelleme:** 2026-09-08
> **Kaynak:** `https://hero-sms.com/docs/v1/openapi.json` — OpenAPI 3.2.0, v1.0, 45 path, 57 şema
> **Canlı doğrulama:** 2026-09-08, gerçek hesap, yalnız okuma çağrıları
> **Derinlik analizi:** 104 ajanlı çapraz doğrulama turu (her mimari iddia 2 bağımsız hasım doğrulayıcıdan geçti)

---

## 0. Yönetici özeti — tasarımı değiştiren bulgular

| # | Bulgu | Etki |
|---|---|---|
| 1 | ✅ **Modern REST API var** (`/api/v1`, 22 op); legacy `?action=` (25 op) geriye dönük uyumluluk | Adaptör modern birincil; **legacy zorunlu** (katalog + bakiye modern'de yok) |
| 2 | 🔴 **Tekil `GET /activations/{id}` YOKTUR** — o yolda yalnız `delete` | Teyit `/otp/last` ile yapılır → **ADR-022** |
| 3 | ✅ **`sms-incoming` webhook'u** + **kaynak IP'ler spec'te yazılı** | Yoklama yedeğe düşer; IP izin listesi mümkün |
| 4 | 🔴 **Webhook yanıt zaman aşımı 3 sn**, ≥7 yeniden deneme | Handler senkron iş yapamaz → **ADR-024** |
| 5 | ✅ **`maxPrice`** kabul ediliyor · 🔴 **`fixedPrice` "tam o fiyat" demek** | `maxPrice` gönderilir, **`fixedPrice` gönderilmez** → **ADR-027** |
| 6 | ⚠️ **`counts.physical` seçimi canlı gözleme dayanır** — spec üç sayacı da açıklamıyor | Karar korunur ama çıkarım olarak işaretli |
| 7 | ✅ **`expiredAt` yanıttan gelir**; varsayılan süre spec'te hiç yok | TTL koda gömülmez |
| 8 | ✅ **Kiralık = aynı uç nokta + `duration`**, `subtype: 2` | Birleşik `orders` doğru |
| 9 | 🔴 **İptal iki eşikli:** ilk **120 sn** yasak, sonra **20 dk** penceresi, OTP geldiyse hiç | UI'de iptal butonu 120 sn pasif → **FR-416** |
| 10 | 🔴 **`Cancel` ≠ `Finish`** — biri iade eder, diğeri etmez | `ProviderPort`'a `Finish()` → **ADR-023** |
| 11 | 🔴 **OTP alan adları üç farklı isim seti taşıyor** | Normalizasyon zorunlu → §7.3 |
| 12 | ⚠️ **Para birimi ÇIKARIM** — fiyat/stok/bakiye uç noktaları `currency` **döndürmüyor** | ❓H6b |
| 13 | ✅ **İki ayrı limit:** hız (429, yalnız `offers`) ve **eşzamanlılık** (`403 CHANNELS_LIMIT`) | Ayrı ele alınır → **FR-413** |
| 14 | ✅ **`BANNED` iki kapsamlı** (`global` \| `specific`) | Devre kesici granülaritesi → **ADR-028** |

---

## 1. Sunucular ve kimlik doğrulama

| Yüzey | URL | Auth | Op sayısı |
|---|---|---|---|
| **Modern REST** | `https://hero-sms.com/api/v1` | `Authorization: ApiKey <token>` | 22 |
| **Legacy uyumluluk** | `https://hero-sms.com/stubs/handler_api.php` | `?api_key=<token>` | 25 |
| Auth'suz | `/api/v1/classifiers/activations/custom-durations` | — (`security` bloğu yok) | 1 |

> ❓**H9:** Aynı anahtarın her iki yüzeyde de çalıştığı **canlı doğrulandı** (bu turda:
> `GET /api/v1/activations` ve `?action=getBalance` aynı anahtarla 200 döndü) ✅ — tek
> `providers.api_key_enc` alanı yeterli.

> 🔴 **Legacy çağrı URL'leri log'a yazılmaz.** API anahtarı sorgu dizesinde gider; vekil ve
> erişim log'larına düşer. → `design.md` §12

---

## 2. Para birimi — ⚠️ ÇIKARIM

`Currency` şeması: ISO 4217 sayısal, `enum: [840, 978, 156]`, `default: 840` (USD).

> ⚠️ **Ama fiyat, stok ve bakiye uç noktalarının HİÇBİRİ `currency` döndürmüyor.**
> `Currency` yalnız 6 yerde geçiyor: `/activations/history` item · `EmailActivation` ·
> `ActiveActivation` · `ActivationsHistory` · `successfulNumberv2Response` · `reactivationPriceResponse`.
> `ActivationsOffersSchema`, `PricesByCountry` ve modern `ActivationSchema` içinde **yok**.
> `?action=getBalance` yanıtı `type: string`, birim belirtmiyor.
>
> USD çıkarımı yalnız `Currency.default = 840` üzerinden yapılmıştır. → ❓**H6b**
>
> ⚠️ Ek çelişki: `Currency` enum'u `[840,978,156]` derken legacy `qpSAOptionalCurrency`
> parametresi `[643,840,978,156]` (RUB dahil) diyor — **spec kendiyle çelişiyor**.

---

## 3. Katalog

### 3.1 Ölçülen boyutlar (canlı, 2026-09-08)

| Veri | Değer | Süre |
|---|---|---|
| Ülke | **195** | 120 ms |
| Servis | **811** | 287 ms |
| Ülke × servis | **20.849** | — |
| Tüm fiyat/stok | **1,09 MB, tek çağrı** | **582 ms** |

> Katalog senkronu ülke başına döngü gerektirmiyor. *(Eski prototip 195 ayrı istek atıyordu.)*
> ❓**H20:** Filtresiz `offers` gerçekten tümünü mü döndürüyor — `meta.total = null`, teyit edilmeli.

### 3.2 Katalog uç noktaları — 🔴 legacy'ye mahkûm

Ülke, servis, operatör adları ve bakiye **modern REST'te HİÇ YOK**:

| Veri | Uç nokta | Not |
|---|---|---|
| Ülkeler | `?action=getCountries` | ⚠️ Spec şeması `rent` alanını içermiyor ama **canlı döndürüyor** — spec canlıdan geride |
| Servisler | `?action=getServicesList` | ✅ **`lang=tr` destekliyor** → servis adları Türkçe alınabilir |
| Operatörler | `?action=getOperators` | Yalnız **isim listesi**, fiyat yok |
| Bakiye | `?action=getBalance` | Düz metin `ACCESS_BALANCE:0.5632` |
| Kiralık fiyat/stok | `?action=serviceCountRent`, `getRentServicesAndCountries` | Modern `offers`'ta `duration` boyutu **yok** |
| Özel süreler | `GET /classifiers/activations/custom-durations` | ✅ Modern, **auth'suz**, **DAKİKA** cinsinden |

**Türkiye = `62`** · Operatörler: `turkcell`, `turk_telekom`, `vodafone`
**Ülke şeması:** `{id, eng, rus, chn, rent, retry, visible}` — `rent: 1` = kiralık destekli

**Servis kodları:** `wa`=Whatsapp · `ig`=Instagram+Threads · `go`=Google/YouTube/Gmail ·
`fb`=facebook · `am`=Amazon · `tg`=Telegram · `full`=Full rent
`ServiceId` pattern: `^[a-zA-Z]{2,4}$` · `CountryId`: integer 0..999

> 🔴 **Legacy katalog uçlarında `4xx` HİÇ TANIMLI DEĞİL.** `?action=getCountries` yalnız `200`
> tanımlar; `NO_KEY` / `BAD_KEY` **200 gövdesinin içinde düz metin** olarak gelir.
> **`if StatusCode == 200 { success }` legacy yolunda YASAK.**

### 3.3 Fiyat/stok — `GET /activations/offers/{verificationType}`

`verificationType` ∈ `["sms", "call"]` — **path segmenti**, filtre değil (iki ayrı çağrı gerekir).
İsteğe bağlı: `?services=tg,go,ig&countries=6,33,50`

```json
{ "data": { "<servisKodu>": { "<ülkeId>": {
    "prices": { "default": 0.80, "retail": 0.96, "min": 0.96 },
    "counts": { "total": 2219, "physical": 1953, "defaultPrice": 2219 },
    "map":    { "0.9600": 2219 }
}}}}
```

| Alan | Durum |
|---|---|
| `prices.default` / `retail` / `min` | ⚠️ **Üçünün de `description` alanı BOŞ.** Hangisinden ücretlendirildiğimiz **bilinmiyor** → ❓**H6**. `MinPrice` = *"Mümkün olan en düşük **kişisel** fiyat"* — hesaba özel fiyatlandırma ima ediyor. Legacy `TopCountriesOneService`'te `price=0.045` vs `retail_price=0.2` → **gerçekten ayrışabiliyorlar** |
| `counts.total` / `physical` / `defaultPrice` | ⚠️ **Üçünün de `description`'ı YOK.** `physical` seçimimiz **canlı gözleme** dayanır, spec'e değil → ❓**H16** |
| `map` | Fiyat→adet merdiveni. `maxPrice`'ı `default`'a sabitlersek görünen stok `counts.defaultPrice` kadardır, `total` kadar değil |
| `meta.order.deliverability.countries` | ✅ Ülke sıralaması için bedava teslim-edilebilirlik sinyali |

**Canlı ölçüm — 🔴 en tehlikeli tuzak:**
```
WhatsApp × Türkiye → { "cost": 1.44, "count": 56964, "physicalCount": 0 }
```
`count` 56.964 ama **gerçek stok 0**. Eski prototip `count` kullanıyordu → kullanıcıdan para
çekilir, sağlayıcı boş döner. **Stok = `counts.physical` / `physicalCount`.** → `trd.md` FR-306

> ⚠️ Legacy `?action=getPrices`, `getTopCountriesByService`, `getTopCountriesByServiceRank`
> **`deprecated: true`** — *"Yöntem kullanımdan kaldırıldı. GET activations/offers kullanın."*

---

## 4. Satın alma — `POST /api/v1/activations`

`operationId: buyActivationBatch` — **toplu** satın alma.

```json
{
  "service": "wa",           // ZORUNLU  ^[a-zA-Z]{2,4}$
  "country": 62,             // ZORUNLU  0..999
  "amount": 1,               // ZORUNLU  1..10
  "operator": "any",         // ops.     varsayılan "any"
  "maxPrice": 1.50,          // ops.     ⭐ minimum 0.0067
  "fixedPrice": false,       // ops.     🔴 GÖNDERİLMEZ — bkz. aşağı
  "duration": 24,            // ops.     KİRALIK: 24|72|168|336|720|1440|2160|4320 saat
  "verificationType": "sms", // ops.     "sms" | "call"
  "resellerUserId": "01J..." // ops.     ^[a-zA-Z0-9_.@-]{1,36}$  (ULID uyar)
}
```

**Yanıt `200` — 🔴 HER ZAMAN DİZİ:**
```json
{ "data": [ { "id": 1, "status": 4, "phone": 440959999999, "service": "fb",
              "country": 44, "countryPhoneCode": 7, "operator": "axis",
              "price": 1.05, "otpList": [],
              "createdAt": "...", "expiredAt": "...",
              "verificationType": "sms", "subtype": 1 } ] }
```

| Konu | Kural |
|---|---|
| 🔴 **Yanıt dizi** | `amount: 1` gönderilse bile `data[0]`. `len(data) == 0` veya `> 1` → **hata** (aksi halde parası çekilmiş ama siparişe dönmemiş numara) |
| ⭐ **`maxPrice`** | Her satın almada gönderilir. Fiyat garantisini sağlayıcı sınırında zorlar |
| 🔴 **`fixedPrice` GÖNDERİLMEZ** | *"Satın alma **kesinlikle** iletilen maksimum fiyat üzerinden yapılır"* — bu bir tavan değil **sabit fiyat**. Piyasa düşse bile tavan öderiz → **ADR-027** |
| ⚠️ **`WRONG_MAX_PRICE` modern uçta TANIMSIZ** | Bu `title` yalnız legacy `?action=getNumber`'ın 400'ünde belgeli (`info.min` ile). Modern tarafta fiyat aşımının hangi kodla geldiği **spec'te yok** → adaptör 422/404 gövdesini ayrıştırmalı. ❓**H11/H12** |
| 🔴 **`NO_NUMBERS` / `NO_BALANCE` modern uçta TANIMSIZ** | Modern yanıtlar yalnız `[200,401,404,422,500]`. Legacy'de ikisi de açık (`402 NO_BALANCE`, `404 NO_NUMBERS`). **En sık iki hatamızın modern karşılığı bilinmiyor** → ❓**H11, H12** |
| ✅ **`expiredAt`** | TTL yanıttan. Varsayılan süre spec'te **hiçbir yerde yok**; `custom-durations` yalnız istisnaları verir |
| ✅ **`subtype`** | 1 = aktivasyon, 2 = kiralama |
| ⚠️ **`phone` tipi** | `ActivationPhone` = **integer** (`79991234567`, "işaretsiz `+`") ama örneklerde maskeli **string** (`"79********1"`) → esnek unmarshal |
| ⚠️ **`resellerUserId` geri dönmüyor** | `ActivationSchema` içinde bu alan **yok** → FR-408 korelasyonu için tek başına yetmez. ❓**H17** |
| 🔴 **Idempotency YOK** | Spec'te "idempot" 0 kez geçiyor; tanımlı tek istek başlığı `Authorization`. Batch olduğu için yeniden deneme **10 numaraya kadar** çift alım. **Asla yeniden denenmez** |

---

## 5. Yaşam döngüsü uç noktaları

| İşlem | Uç nokta | Not |
|---|---|---|
| Satın al | `POST /activations` | Batch, yanıt dizi |
| Aktif liste | `GET /activations` | ✅ `size` **max 25**; her kayıtta tam `otpList` |
| Son OTP | `GET /activations/{id}/otp/last` | Tekil obje — **teyit için tercih edilen** |
| Tüm OTP | `GET /activations/{id}/otp` | Dizi, sayfalama yok |
| **İptal + İADE** | `DELETE /activations/{id}` | `204` |
| **Tamamla, İADE YOK** | `POST /activations/{id}/finish` | `204` |
| Numara değiştir | `POST /activations/{id}/replace` | ⚠️ **Ücretli mi, iade var mı: spec'te HİÇBİR bilgi yok** → v1 kapsam dışı, ❓H19 |
| Yeniden aktive | `POST /activations/{id}/reactivate` (+`/options`) | Ücretli; **ön koşul: `finish` yapılmış olmalı** |
| Uzat (kiralık) | `POST /activations/{id}/prolong` (+`/options`, `/history`) | `425 TOO_EARLY` + `Retry-After` |
| Geçmiş | `GET /activations/history` | ✅ `from`/`to` **ZORUNLU**; `totals{sum, successCount}` |
| İstatistik | `GET /activations/stats?date=` | ülke→servis→`{count, success, percent}` — sağlayıcı skorlaması |

> 🔴 **TEKİL DURUM SORGUSU YOK.** `jq '.paths["/activations/{activationId}"] | keys'` → `["delete"]`
> Tekil teyit `/otp/last` (tercih) veya legacy `?action=getStatusV2` (yanıt tipi kararsız) ile yapılır.

### 5.1 Durum kodları

`ActivationStatusTypes`: `[1, 2, 3, 4, 6, 7, 8, 10]` — 🔴 **`x-enum-descriptions` YOK.**

Yalnız `ActivationHistoryStatus` alt kümesi açıklamalı:

| Kod | Anlam |
|---|---|
| 4 | (tüm örneklerde aktif aktivasyonun durumu — ⚠️ açıklama yok) |
| **6** | ✅ Başarıyla tamamlandı |
| **8** | ❌ İptal edildi |
| **10** | 💰 İade edildi |

> 🔴 **1, 2, 3, 7'nin anlamı bilinmiyor** → ❓**H1**.
> **Kendi durum makinemiz sağlayıcı `status`'üne BAĞLANMAZ** — eşleme tek bir fonksiyonda,
> savunmacı; bilinmeyen değer `PENDING` korur. → **ADR-025**

### 5.2 Legacy `setStatus`

`3` = SMS tekrar iste · `6` = tamamla (iade yok) · **`8` = iptal + iade**

---

## 6. İptal ve iade — 🔴 en kritik bölüm

> Ürün kuralımız: **kod gelmezse ücret iade edilir.** Bizim tarafta koşulsuz; sağlayıcı tarafında koşullu.

### 6.1 İptal koşulları — **iki ayrı eşik**

`DELETE` açıklaması, birebir:
> *"Aktivasyon iptali ve iade. İptal yalnızca aktivasyon hiç OTP almamışsa **veya** 24 saat ve
> üzeri süreli aktivasyonda 20 dakikadan az süre geçmişse mümkündür. Aksi halde
> `POST /activations/{activationId}/finish` çağırın."*

**Ret sebepleri — üçü farklı davranış gerektirir:**

| `title` | Anlam | Bizim davranışımız |
|---|---|---|
| **`EARLY_CANCEL_DENIED`** | ✅ `info: {minActivationTime: 120}` — **ilk 120 saniye iptal edilemez** | **Kuyruğa al, 120 sn sonra tekrar dene** |
| **`FREE_CANCELLATION_EXPIRED`** | ✅ *"time limit exceeded (20 minutes)"* | **Kalıcı red → gider** |
| **`OTP_RECEIVED`** | ✅ OTP geldiyse iptal yok | **Kalıcı red → gider** (zaten kod teslim edildi) |
| `NEW_OTP_RECEIVED` | `info.data` = OTP listesi; onay isteniyor | Kodu al, `finish` çağır |
| `ACTIVATION_NOT_ACTIVE` | *"terminated/refunded and Otp cannot be retrieved"* | Zaten kapanmış |

> ⚠️ **İki eşiğin birlikte nasıl işlediği belirsiz:** "hiç OTP almamışsa **VEYA** 24sa+ aktivasyonda
> 20 dk'dan az" — bu VE mi VEYA mı? Kısa süreli normal SMS aktivasyonunda 20 dk tavanı var mı?
> → ❓**H3**
>
> ⚠️ `minActivationTime: 120` **sabit mi, servis/ülkeye göre değişken mi** bilinmiyor → ❓**H2**

> 🔴 **Modern `DELETE` / `finish` için `409` TANIMLI DEĞİL** (`[204,401,404,422,500]`).
> Yukarıdaki iş kuralları yalnız **legacy** 409'unda belgeli. Adaptör **hem 409 hem 422**
> gövdesini ayrıştırmalı. → ❓**H13**

### 6.2 Bizim iade akışımız

```
Kullanıcı iptal etti  VEYA  expires_at geçti
   │
   ├─ 1. KULLANICIYA İADE (ledger REFUND)   ← KOŞULSUZ, HEMEN, beklemeden
   │
   └─ 2. Sağlayıcıyı KAPAT (asenkron, ZORUNLU — FR-412)
         ├─ kod gelmedi  → DELETE   (iade talebi)
         │    ├─ 204                     → provider_refund_status = REFUNDED
         │    ├─ EARLY_CANCEL_DENIED     → RETRY_SCHEDULED (120 sn sonra)
         │    └─ FREE_CANCELLATION_EXPIRED / OTP_RECEIVED → DENIED (gider)
         └─ kod geldi    → finish   (iade yok, sağlayıcıya doğru sinyal)
```

> 🔴 **FR-412: Her `PENDING` sipariş MUTLAKA `DELETE` veya `finish` ile kapatılır.**
> "Bırak süresi dolsun" stratejisi spec tarafından desteklenmiyor; `DELETE` açıklaması açıkça
> `finish`'e yönlendiriyor. Süresi dolan aktivasyonun akıbeti **spec'te yok** → ❓**H10**

> ⚠️ ❓**H18:** `status = 10` (*"Activation blocked. Funds refunded."*) sağlayıcı tarafından
> **tek taraflı** tetiklenebiliyor. Webhook'la bildiriliyor mu bilinmiyor. Yakalanmazsa
> sağlayıcı bize iade eder ama biz kullanıcıya etmeyiz → kasada kalır. `activations/history`
> taraması gerekebilir.

---

## 7. SMS kodunun alınması

### 7.1 Webhook `sms-incoming` — birincil

```
POST <bizim URL>
{ "activationId": 123456, "id": ???, "phoneFrom": "89854", "service": "wa",
  "text": "Your code is 12345", "code": "12345", "country": 62,
  "receivedAt": "2026-09-08T10:15:30.000000Z" }
```

| Konu | Spec'te yazan |
|---|---|
| ✅ **Kaynak IP'ler** | **`84.32.223.53`, `185.138.88.87`** *(IPv6 karşılığı ve değişim politikası spec'te yok)* |
| 🔴 **Yanıt zaman aşımı** | **3 saniye** — aşılırsa yeniden deneme fırtınası |
| 🔴 **Yeniden deneme** | *"En az 7 kez"*, 20–30 sn arayla, ≥3 dk boyunca → aynı SMS **en az 8 kez** gelebilir. ⚠️ Spec kendiyle çelişiyor: webhook `default` yanıtı *"en fazla 7"* diyor |
| ⚠️ **URL kaydı** | Hesap panelinden, **en fazla 3 HTTPS URL**. API uç noktası **YOK** |
| 🔴 **İmza doğrulaması** | **YOK** — HMAC, imza başlığı, paylaşılan sır: hiçbiri spec'te geçmiyor |
| 🔴 **`id` alanı** | `required` listesinde **AMA `properties` içinde TANIMSIZ** (spec hatası) → tipi bilinmiyor. ❓**H14** |
| ⚠️ **`code` `required` DEĞİL** | Hem `code` hem `text` **nullable** (`type: ["string","null"]`) |

**Zorunlu karşı önlemler:**

1. **IP izin listesi** — gerçek peer IP'ye bakılır, `X-Forwarded-For`'a **değil**. IP'ler env/DB'de.
2. **Tahmin edilemez yol:** `/api/v1/webhooks/herosms/<128-bit rastgele>`
3. **Handler senkron iş yapmaz** — ham gövdeyi `asynq` kuyruğuna atar, **anında 200 döner**. → **ADR-024**
4. **Gövdedeki `code`'a güvenilmez** — teyit `GET /activations/{id}/otp/last` ile. → **ADR-022**
   *Teyit boş dönerse durum **DEĞİŞTİRİLMEZ**, sipariş `PENDING` kalır.*
5. **Dedup:** birincil `(activationId, id)`; `id` yoksa `SHA-256(activationId + receivedAt + text)`.
   Go tarafında `id` için toleranslı tip (string veya number).
6. **Tanımadığı `activationId`** → sessizce `200`, log. *(3 URL slotu hesap geneli; prod/staging
   aynı hesaptaysa her ortam tüm hesabın olaylarını alır.)*
7. **Kendi OTP ayrıştırıcımız (regex)** — `code` gelmeyebilir. Kod çıkarılamazsa sipariş
   `PENDING` kalır, **asla `COMPLETED` olmaz**.
8. **`verificationType: "call"` dalı:** `code`/`text` null gelebilir; bu **"kod gelmedi" sayılmaz**.
9. **İlk koddan sonra SSE kapanmaz** — `otpList` dizidir, ikinci/üçüncü mesaj gelebilir.

### 7.2 Yoklama — güvenlik ağı

| Uç nokta | Not |
|---|---|
| `GET /activations/{id}/otp/last` | ✅ Tekil teyit — **tercih edilen** |
| `GET /activations/{id}/otp` | Tüm OTP'ler; ❓**H15** OTP gelmemişken ne döner (boş dizi mi 404 mü) bilinmiyor |
| `GET /activations` | Toplu yoklama, **`size` max 25** → 100 bekleyen sipariş = 4 sayfa/tur |
| Legacy `?action=getStatusV2&id=` | ⚠️ Yanıt tipi kararsız: JSON obje **veya** düz string `STATUS_CANCEL` |

> 🔴 **`GET /activations/{id}` KULLANILMAZ — böyle bir uç nokta YOK.**

### 7.3 🔴 OTP alan adı normalizasyonu

**Üç kaynak, üç isim seti, iki tarih formatı:**

| Kaynak | Kod | Metin | Zaman |
|---|---|---|---|
| Modern `/otp`, `/otp/last` | `smsCode` | `smsText` | `receivedAt` (ISO8601) |
| Legacy `getAllSms`, `getStatusV2` | `code` | `text` | `date` (**RFC3339**) |
| Webhook | `code` | `text` | `receivedAt` (ISO8601) |

Adaptör hepsini tek iç tipe indirger: `Code`, `Body`, `ReceivedAt`.

> Bu, `memory.md` §3.2'deki **"alan adı uyuşmazlığı = sessiz veri kaybı"** dersinin birebir
> tekrarı riskidir. Eski prototip tam olarak bu yüzden kırılmıştı.

---

## 8. Kiralık numara (v1.1)

| Öğe | Bulgu |
|---|---|
| Satın alma | ✅ `POST /activations` + `duration` — aynı uç nokta |
| Süreler | `RentDuration` enum: `24, 72, 168, 336, 720, 1440, 2160, 4320` saat |
| ⚠️ Süre çelişkisi | `BAD_DURATION` hatası `{min:24, max:720, available_durations:[24,72,168,720]}` döndürüyor; `serviceCountRent` örneği **2/12/48** saat kullanıyor. **Spec üç yerde kendiyle çelişiyor** → süreler **canlıdan** öğrenilir |
| Ayırt edici | `subtype: 2` |
| Fiyat/stok | 🔴 **Yalnız legacy** — modern `offers` şemasında `duration` boyutu **YOK** → hibrit akış |
| Çoklu mesaj | `otpList[]` + `GET /activations/{id}/otp` |
| Uzatma | `POST /activations/{id}/prolong` — ⚠️ **`prolong` ≠ `reactivate` ≠ `rent`**, üç ayrı mekanizma; `ACTION_NOT_AVAILABLE`: *"Try '/reactivate' method."* |

> ⚠️ `ProviderPort.Extend(d time.Duration)` imzası **yanlış** — sağlayıcı keyfi süre kabul etmiyor,
> sabit enum istiyor. → **ADR-023** kapsamında düzeltilecek.

---

## 9. Hata kodları

**Zarf:** `BaseErrorResponse = { title, details, info? }` — 🔴 alan adı **`code` değil `title`**.
34 farklı `title`; yalnız 8'i `enum` ile sabit, gerisi **sadece `examples` içinde** = sözleşme değil.

| `title` | HTTP | Bizim tipimiz | Davranış |
|---|---|---|---|
| `NO_NUMBERS` | 404 (legacy) | `ErrOutOfStock` | Başka sağlayıcı / "stok yok" |
| `NO_BALANCE` | 402 (legacy) | `ErrProviderNoBalance` | 🚨 Admin alarmı |
| `BAD_KEY` / "Unauthenticated." | 401 | `ErrProviderAuth` | 🚨 Admin alarmı |
| `WRONG_MAX_PRICE` | 400 (legacy) | `ErrPriceChanged` | Teklifi geçersiz kıl |
| `WRONG_SERVICE` / `WRONG_COUNTRY` | 422 | `ErrProviderMapping` | 🚨 Eşleştirme bozuk |
| **`RATE_LIMIT`** | **429** | `ErrProviderRetryAfter` | ✅ `Retry-After` başlığına uy — **yalnız `offers`'ta tanımlı** |
| **`TOO_EARLY`** | **425** | `ErrProviderRetryAfter` | `Retry-After` |
| **`FREEZE_PERIOD_NOT_REACHED`** | — | `ErrProviderRetryAfter` | `info.retry_after_seconds` |
| **`CHANNELS_LIMIT`** | **403** | `ErrProviderConcurrencyLimit` | `info: {current_threads, max_allowed}` → **semafor** |
| **`BANNED`** | **403** | `ErrProviderBanned(scope)` | `info: {scope, banned_until, retry_after_seconds}`. `global` → sağlayıcıyı kapat; `specific` → yalnız o (servis,ülke) çiftini kara listeye al |
| `EARLY_CANCEL_DENIED` | 409 (legacy) | `ErrCancelTooEarly` | `info.minActivationTime` sonra tekrar dene |
| `FREE_CANCELLATION_EXPIRED` / `OTP_RECEIVED` | 409 (legacy) | `ErrCancelDenied` | Kalıcı red → gider |
| `ACTIVATION_NOT_ACTIVE` | 409 | `ErrProviderOrderClosed` | Zaten kapanmış |
| `OFFER_NOT_FOUND` | 404 | `ErrOutOfStock` | — |
| `SERVER_ERROR` / `ERROR_SQL` | 500 | `ErrProviderUnavailable` | Devre kesici |

**Legacy düz metin (HTTP 200 gövdesinde!):**
`ACCESS_BALANCE:0.5632` · `ACCESS_NUMBER:id:phone` · `ACCESS_CANCEL` · `ACCESS_ACTIVATION` ·
`STATUS_OK:code` · `STATUS_WAIT_CODE` · `STATUS_CANCEL` · `NO_KEY` · `BAD_KEY` · `BAD_ACTION` ·
`NO_NUMBERS` · `ERROR_SQL` · `OPERATORS_NOT_FOUND` · `EARLY_CANCEL_DENIED`

---

## 10. Limitler

| Limit | Durum |
|---|---|
| **Hız limiti** | ⚠️ `429 RATE_LIMIT` + `Retry-After` **yalnız `GET /activations/offers/{verificationType}`** için tanımlı. Sayısal değer yok. Diğer 46 uç noktada 429 **yok** → ❓**H4** |
| **Eşzamanlılık limiti** | ✅ **Ayrı mekanizma:** `403 CHANNELS_LIMIT`, `info: {current_threads, max_allowed}`. İstek/sn değil, **eşzamanlı aktivasyon** limiti → semafor. → **FR-413** |
| **Sayfalama** | ✅ Modern `size` **max 25**. Legacy `getHistory`: açıklamada "maks. 100", şemada kısıt yok; `getAllSms`: kısıtsız |
| **Idempotency** | ❌ **Yok** |

> Spec'teki **tek yanıt başlığı** `Retry-After`. `X-RateLimit-*` yok.
> **FR-414:** `429`, `425` ve `info.retry_after_seconds` içeren hatalarda **sabit backoff kullanılmaz**.

---

## 11. Diğer ürün tipleri

| Uç nokta grubu | Not |
|---|---|
| `/emails` (7 op) | E-posta aktivasyonu. Eksen `(site, domain)` — **ülke/operatör yok**. `EmailActivationStatus` açıklamalı (3=bekliyor, 4=iptal, 5=alındı, 6=tamamlandı, **7=iade**). ⚠️ **Webhook YOK**. 🔴 `site` kataloğunu listeleyen uç nokta **yok** — `catalog-sync` bu boyutu otomatik keşfedemez |
| `verificationType: "call"` | Sesli doğrulama. Aynı satın alma uç noktası, ama `offers`'ta **ayrı path segmenti** → aynı servis+ülke için farklı fiyat/stok |

> ⚠️ Bu iki ürün `design.md` §4'teki katalog modelini **kısmen** zorluyor:
> `products` UNIQUE kısıtı ve tipli sütunlar telefon-merkezli. Düzeltmeler `design.md` §6'da.

---

## 12. Canlı test gerekenler

| # | Soru | Para | Öncelik |
|---|---|---|---|
| **H1** | `ActivationStatusTypes` 1,2,3,7 anlamları | ✅ | Yüksek |
| **H2** | `minActivationTime: 120` sabit mi, değişken mi | ✅ | Yüksek |
| **H3** | Ücretsiz iptal penceresi: "VE" mi "VEYA" mı; kısa aktivasyonda 20 dk tavanı var mı | ✅ | Yüksek |
| **H4** | Modern `/activations` hız limiti | ❌ | Orta |
| **H6** | `prices.default` vs `retail` vs `min` — hangisinden ücretlendiriliyoruz | ✅ | Yüksek |
| **H6b** | **Para birimi gerçekten USD mi?** `?action=getNumberV2` yanıtında `currency` **var** — oradan teyit | ✅ | **Yüksek** |
| **H10** | `expiredAt` geçince otomatik iade var mı, yoksa yanıyor mu | ✅ | **Yüksek** |
| **H11** | Modern `POST /activations` stok yokken hangi kod + `title` | ❌ | **Yüksek** |
| **H12** | Modern `POST /activations` bakiye yetersizken hangi kod | ❌ | **Yüksek** |
| **H13** | Modern `DELETE` iptal reddinde 409 mu 422 mi | ✅ | **Yüksek** |
| **H14** | Webhook `id` alanı nedir + **imza/secret başlığı geliyor mu** (ham HTTP başlıkları kaydedilir) | ✅ | **Yüksek** |
| **H15** | `/otp` OTP gelmemişken ne döner; `/otp/last`'ın 400'ü ne zaman | ✅ | **Yüksek** |
| **H16** | `counts.physical` / `total` / `defaultPrice` gerçek anlamı; `map` değerleri adet mi | ❌ | Orta |
| **H17** | `resellerUserId` geri dönüyor mu | ✅ | Orta |
| **H18** | `status=10` tek taraflı iade nasıl bildiriliyor | ❌ | Orta |
| **H19** | `replace` ücretli mi | ✅ | Düşük |
| **H20** | Filtresiz `offers` tümünü mü döndürüyor | ❌ | Düşük |
| **H21** | `size > 25` gönderilirse 422 mi sessiz kırpma mı | ❌ | Düşük |

> **Maliyet:** para harcayan 11 test × ~$0,10–1,50. **Mevcut bakiye $0,5632 — yetersiz.**
> M3 fixture çalışmasından önce yükleme gerekli (roadmap G3).
> Tüm ham yanıtlar `api/testdata/fixtures/herosms/` altına kaydedilir (NFR-804).

> 🔴 **Fixture'lar spec'ten DEĞİL canlı yanıtlardan üretilir.** Spec canlıdan geride:
> `Country.rent` şemada yok ama canlı döndürüyor · `GET /activations`'ta `meta` şemada yok ama
> canlı döndürüyor · legacy `application/json` ilan ediyor ama `text/html` geliyor.

---

## 13. Adaptör tasarım kararları

```go
const ProtocolHeroSMSV1 ProviderProtocol = "HEROSMS_V1"
```

| Karar | Gerekçe | ADR |
|---|---|---|
| Modern REST birincil, **legacy zorunlu** | Katalog + bakiye modern'de yok | ADR-021 |
| Teyit `GET /{id}/otp/last` | Tekil `GET /activations/{id}` **yok** | **ADR-022** |
| `Cancel()` **ve** `Finish()` ayrı metotlar | `DELETE` iade eder, `finish` etmez | **ADR-023** |
| Webhook handler kuyruğa atar, anında 200 | 3 sn zaman aşımı | **ADR-024** |
| Sağlayıcı `status`'ü bizim durum makinemize bağlanmaz | 1,2,3,4,7 anlamı bilinmiyor | **ADR-025** |
| Maliyet **mikro-birimde** (6 hane) saklanır | Fiyatlar 4 ondalıklı, `MaxPrice.minimum = 0.0067` | **ADR-026** |
| `maxPrice` gönderilir, **`fixedPrice` gönderilmez** | `fixedPrice` = tam fiyat, tavan değil | **ADR-027** |
| Devre kesici **yüzey + ban kapsamı** bazında | Legacy çökünce modern kapanmamalı; `scope: specific` | **ADR-028** |
| `operator` boyutu fiyatlanmaz, `any` sabitlenir | Operatör kırılımlı fiyat/stok hiçbir uçta yok | **ADR-029** |
| Stok = `counts.physical` | ⚠️ **Canlı gözleme dayanır**, spec açıklamıyor | — |
| `Purchase` asla yeniden denenmez | Idempotency yok + batch | — |
| `data[0]` okunur, `len != 1` → hata | Yanıt her zaman dizi | — |

---

**İlgili:** [design.md](design.md) §9 · [trd.md](trd.md) §3-4 · [memory.md](memory.md) §4 · [roadmap.md](roadmap.md) M3
