# memory.md — Proje Belleği

> **Doküman amacı:** Koddan okunamayan bilgi. Neden şu karar verildi, hangi tuzağa düşüldü, sağlayıcı
> API'si neden böyle davranıyor, hangi soru hâlâ açık. Yeni bir oturum (veya yeni bir kişi) buraya
> bakarak bağlamı yeniden kurabilmeli.
>
> **Durum:** yaşayan doküman · **Son güncelleme:** 2026-09-08

---

## 1. Karar günlüğü

Her önemli karar tarihiyle ve gerekçesiyle. **Karar değiştiğinde eskisi silinmez, üstü çizilir ve
altına yenisi yazılır** — geçmiş kararın gerekçesi gelecekte işe yarar.

### 2026-09-08 · Yeniden yazma kararı
Mevcut sistem (127 commit, son commit 2026-04-09, canlıya hiç alınmadı) prototip olarak amacına
ulaştı ama ticari kullanıma uygun değil. Kritik güvenlik açıkları, çalışmayan kod bölümleri ve
katmansız mimari nedeniyle **kademeli refactor yerine sıfırdan yeni repo** seçildi.
→ Ayrıntılı gerekçe: [intent.md](intent.md) §10, bulgular §3.

### 2026-09-08 · Stack: Go + Next.js
İlk turda TypeScript + Express + Prisma + EJS kararlaştırıldı, sonra **Go (backend) + Next.js
(frontend)** olarak değiştirildi. Gerekçe: kullanıcı Go öğrenmek istiyor; paralel sağlayıcı sorgusu,
SSE ve arka plan işçileri için Go doğru araç. Bedeli kabul edildi: iki yeni teknoloji, v1 süresi
~10 haftadan ~16 haftaya çıktı.
→ [design.md](design.md) ADR-001

### ~~2026-09-08 · GORM seçildi, korkuluklarla~~ — ⛔ AYNI GÜN İÇİNDE GEÇERSİZ KALDI
~~Kullanıcı GORM'u tercih etti (Sequelize/Eloquent'e benzerliği, öğrenme hızı). ORM'un örtük
davranışlarının para sisteminde risk olduğu belirtildi; karar korunarak "para yollarında ham SQL,
CRUD'da GORM" kuralı konuldu.~~

**Neden düştü:** Gerekçe açıkça *kullanıcının öğrenme hızı*ydı. Kullanıcı kodu kendisinin
yazmayacağını belirtince gerekçe kalmadı, yalnız maliyeti kaldı. Yerine **sqlc** geçti
(aşağıdaki "Geliştiriciyi Claude yapıyor" kaydına bakın).
→ [design.md](design.md) §2.1, ADR-004

### 2026-09-08 · Cüzdan yalnız TRY
Mevcut sistemin davranışı korundu: kullanıcı TL görür, TL harcar. USDT yatırımları yatırma anındaki
kurla TL'ye çevrilir ve kur kayda geçer. Çoklu para birimli cüzdanın ledger karmaşıklığı v1'e değmez.
→ ADR-017

### 2026-09-08 · Genel katalog modeli
Kullanıcının mevcut veritabanından temel şikâyeti: *"yeni servis eklerken tablo değiştirmek zorunda
kalıyorum."* Buna cevaben `products` + `provider_dimension_maps` + `provider_offers` üçlüsü
tasarlandı. İki ayrı eşleştirme tablosu tek tabloda birleşti. Yeni boyut = yeni satır, yeni tablo değil.
→ [design.md](design.md) §4, ADR-010

### 2026-09-08 · Kiralık numara ve referans: şema v1'de, özellik v1.1'de
İkisi de veri modelinde ve `ProviderPort` arayüzünde baştan yer alır ama v1'de kullanıcıya açılmaz.
Böylece lansman gecikmez, sonradan eklemek göç gerektirmez.
→ [intent.md](intent.md) §7, [trd.md](trd.md) §11

### 2026-09-08 · Lansman kapısı açık bırakıldı
Şirket kurulmayacağı bilgisi alındı. Türkiye'de kart ödemesi üye işyeri hesabı (vergi levhası)
gerektirir; kripto ile mal/hizmet bedeli tahsilatı 2021 tarihli yönetmelikle yasaktır; şahsi hesaba
ticari tahsilat kayıt dışı faaliyettir. Karar: **yazılım eksiksiz kurulur, lansman kararı `M9`
kapısında verilir.** Teknik plan bundan etkilenmez.
→ [roadmap.md](roadmap.md) M9

### 2026-09-08 · Stack değişti: Go + Next.js (react/nextjs/tailwind + Go backend)
İlk turda TypeScript + Express + Prisma + EJS kararlaştırıldı. Kullanıcı sonra Go (backend) +
Next.js/React/Tailwind (frontend) istedi; görsel olarak mevcut Duralux koyu teması korunacak ama
Bootstrap/jQuery yerine Tailwind ile yeniden inşa edilecek. Kullanıcı Go'ya yeni.
Sonuç: v1 tahmini ~10 haftadan **~16 haftaya** çıktı, `roadmap.md` M0'a öğrenme fazı eklendi.
→ ADR-001, ADR-002

### 2026-09-08 · Shopier ve kart ödemesi v1'den çıkarıldı
Kullanıcı Shopier kullanmayacağını belirtti. v1 ödeme kanalları: **banka havalesi (dekont)** ve
**USDT TRC20/ERC20 (TX hash, manuel onay)**. Kart ödemesi `deposit_methods` ile genişletilebilir
kalıyor ama v1 kapsamı dışında ve yasal ön koşula bağlı (M9).

### 2026-09-08 · HeroSMS'in MODERN REST API'si keşfedildi
Kullanıcı resmî doküman adresini paylaştı: `https://hero-sms.com/tr/api`.
Beklenenin aksine HeroSMS yalnız legacy SMS-Activate protokolü sunmuyor — **OpenAPI 3.2.0 ile
belgelenmiş modern bir REST API'si var** (`https://hero-sms.com/api/v1`, 45 uç nokta, 57 şema).
Eski prototipimiz legacy protokolü kullanıyordu.
**Karar:** Adaptör modern REST'i birincil kullanır, legacy yalnız karşılığı olmayan katalog
çağrıları için. Protokol değeri `HEROSMS_V1`.
→ ADR-021 · tam referans: [provider-herosms.md](provider-herosms.md)

### 2026-09-08 · Webhook birincil, yoklama güvenlik ağı oldu
HeroSMS `sms-incoming` webhook'u sağlıyor. Bu, `design.md` §8.3'ü ve §13'teki `order-poller`
işini değiştirdi: webhook birincil yol, yoklama 5 sn'den **30 sn'ye** düşürülüp güvenlik ağına
dönüştürüldü.
**Kritik kısıt:** HeroSMS **imza (HMAC) doğrulaması sunmuyor** ve webhook URL'i API ile
kaydedilemiyor (panelden elle). Bu yüzden webhook bir *tetikleyici* olarak ele alınır; kod
her zaman sağlayıcıdan teyit edilir.
> ⚠️ **Aynı gün düzeltildi:** teyit mekanizması başta `GET /activations/{id}` yazılmıştı — böyle bir
> uç nokta **yok**. Doğrusu `GET /activations/{id}/otp/last` (ADR-022, §4.1 hata tablosu).
→ ADR-019, ADR-020 · `trd.md` FR-410

### 2026-09-08 · `maxPrice` ile sağlayıcı sınırında fiyat garantisi
`POST /activations` bir `maxPrice` parametresi kabul ediyor. Teklifimizdeki maliyeti buraya
göndererek, fiyat yükselmişse satın almanın **hiç gerçekleşmemesini** sağlıyoruz. Bu, "gösterilen
fiyat = tahsil edilen fiyat" ilkesini yalnız kendi kodumuzda değil sağlayıcı sınırında da zorluyor.
→ ADR-018 · `trd.md` FR-401

### 2026-09-08 · Gerçek iOS cihaz mevcut
Kullanıcının gerçek iOS cihazı var. `roadmap.md` M5'teki SSE kabul testi (arka plandan dönüşte
akışın toparlanması) **simülatörle değil gerçek cihazla** doğrulanabilir. S11 kapandı.

### 2026-09-08 · Geliştiriciyi Claude yapıyor — üç karar değişti
Kullanıcı kodu kendisi yazmayacağını ve Go öğrenmek istemediğini belirtti. Sonuçları:
1. **`roadmap.md`'den takvim kaldırıldı.** Süre tahminleri insan geliştirici varsayımına dayanıyordu.
   Yerine teslim sırası + çıkış kriterleri + "senden gerekenler" listesi kondu.
2. **M0'daki Go öğrenme fazı ve `intent.md`'deki "Go öğrenme eğrisi" riski silindi.**
3. **ADR-004 revize edildi: GORM → sqlc.** GORM'un gerekçesi açıkça "kullanıcının öğrenme hızı"ydı;
   o gerekçe düşünce geriye yalnız ORM'un örtük davranış riski kaldı. sqlc ile her sorgu açık SQL
   olduğu için `design.md` §2.1'deki korkuluk listesi (para yollarında ham SQL, `Save` yasak,
   `AutoMigrate` yasak) **gereksizleşti** ve kaldırıldı.
   *Gin değiştirilmedi: gerekçesi de öğrenme materyaliydi ama Gin zaten iyi bir seçim.*

### 2026-09-08 · Ara onay olmadan çalışma
Kullanıcı "sadece çalışan bir sonuç göster, arayı sorma" dedi. Bu, beklenti/yorum farkının ancak
teslimde ortaya çıkması riskini taşıyor (risk R2). Telafi mekanizması: `trd.md` kabul kriterleri
(KK-*) sözleşme işlevi görür, kritik akışlar testle ispatlanır, belirsizlikte varsayım açıkça
yazılıp teslim raporunda belirtilir.

### 2026-09-08 · Kur sağlayıcısı seçildi: TCMB
Resmî, ücretsiz, anahtarsız ve Türkiye'de muhasebe referansı.
**ForexSelling (döviz satış)** kullanılıyor: döviz cinsinden mal alıyoruz, yani
dövizi satın alıyoruz. Alış kuru maliyeti olduğundan düşük gösterir ve marjı sessizce yer.

⚠️ **Sınırı:** günde bir kez (iş günü ~15:30) güncellenir, gün içi dalgalanmayı
yakalamaz. Bu yüzden `FX_SAFETY_MARGIN_PCT` (varsayılan %2) **zorunludur** —
kur riski bizde. Gün içi hassasiyet gerekirse ücretli bir FX API'ye geçilir.
S4 kapandı.

### 2026-09-08 · İki kez başarısız testle commit atıldı — kalıcı önlem
Kayan test çıktısına bakıp "geçti" varsayma hatası iki kez tekrarlandı.
Kök nedenlerden biri gerçekti (entegrasyon testleri paralel koşup birbirini
kırıyordu → `-p 1`), ama asıl sorun süreçti.

**Önlem:** `scripts/check.sh` — tek kapı, tek özet satırı, düşerse sıfırdan
farklı kod. `make check` bunu çalıştırır. Göz kaçırılamaz.

**Ders:** Uzun çıktı üreten bir doğrulama adımı, doğrulama değildir.

### ~~⬜ Açık: kur (FX) sağlayıcısı~~ — ✅ TCMB seçildi (yukarıdaki kayda bakın)

### ⬜ Açık: e-posta sağlayıcısı seçilmedi
Altyapı yok. M1'de karar verilecek. Öneri: Resend (basit API, iyi teslim edilebilirlik) veya Postmark.
Alan adı SPF/DKIM ayarı şart — aksi halde doğrulama e-postaları spam'e düşer.

### ⬜ Açık: ikinci sağlayıcı seçilmedi
M7'de karar verilecek. Aday: 5sim (mevcut kodda yarım adaptör var) veya SMS-Activate
(HeroSMS ile aynı protokol ailesinden → adaptör yeniden kullanılabilir).

---

## 2. Domain sözlüğü

Kod ve konuşma dilinde **aynı** terimler kullanılır. Karışıklık buradan çözülür.

| Terim | Kod karşılığı | Anlamı |
|---|---|---|
| **Servis** | `Service` | Doğrulama yapılacak hedef platform (WhatsApp, Telegram, Instagram) |
| **Ülke** | `Country` | Numaranın ait olduğu ülke |
| **Operatör** | `Operator` | GSM operatörü; `any` = fark etmez |
| **Ürün** | `Product` | Satılabilir birim (SKU): `(kind, servis, ülke, operatör, süre)` |
| **Ürün tipi** | `ProductKind` | `SMS_ACTIVATION` (tek seferlik) · `SMS_RENTAL` (kiralık, v1.1) |
| **Sağlayıcı** | `Provider` | Numarayı bize satan üst kaynak (HeroSMS, 5sim) |
| **Protokol** | `ProviderProtocol` | Sağlayıcının API ailesi (`SMS_ACTIVATE`, `FIVE_SIM`) — **adaptör bundan seçilir** |
| **Boyut eşleştirme** | `ProviderDimensionMap` | Yerel bir değerin sağlayıcıdaki kod karşılığı |
| **Teklif (offer)** | `ProviderOffer` | Sağlayıcının bir ürün için fiyat/stok anlık görüntüsü (önbellek) |
| **Fiyat teklifi (quote)** | `PriceQuote` | Kullanıcıya verilen, 120 sn geçerli, tek kullanımlık **fiyat sözleşmesi** |
| **Sipariş** | `Order` | Satın alınmış bir ürün örneği |
| **Uzak sipariş kimliği** | `remote_order_id` | Sağlayıcı tarafındaki sipariş kimliği (`activationId`) |
| **Ledger / defter** | `LedgerEntry` | Değişmez para hareketi kaydı — **bakiyenin kaynağı** |
| **Kuruş (minor)** | `int64` | Tüm para değerlerinin saklanma birimi. 12,50 ₺ = `1250` |
| **Provizyon/onay** | — | Bakiyeyi düş → dış çağrı → onayla veya iade et deseni |
| **Yetim provizyon** | — | Bakiyesi düşülmüş ama siparişi oluşmamış işlem |
| **Mutabakat** | reconcile | `Σ ledger == balance` kontrolü |
| **Bakiye yükleme** | `Deposit` | Kullanıcının cüzdanına para ekleme talebi |

**İsimlendirme kuralı:** Veritabanı `snake_case` + çoğul tablo adı. Go `PascalCase` dışa açık /
`camelCase` içeride. TypeScript `camelCase`. API JSON `camelCase`.
*(Mevcut sistemde `user_id` / `userId`, `Tickets` / `payments` karışıktı — bu tekrarlanmayacak.)*

---

## 3. Eski sistemden çıkarılan dersler

> Bu bölüm bir eleştiri listesi değil, **tuzak haritasıdır**. Her madde yeni sistemde bir tasarım
> kararına karşılık gelir. Dosya:satır referansları eski repodadır (`main`, commit `2d1fbfd`).

### 3.1 Tanımsız modeller — kodun büyük kısmı hiç çalışmıyor

`SmsCountry`, `ServiceSetting`, `SmsInventory`, `UserRole` modelleri **17 yerde** kullanılıyor ama
`models/` altında **hiç tanımlanmamış**. Sequelize destructuring `undefined` döndürüyor, ilk kullanımda
`TypeError` atıyor.

Etkilenen ve **hiç çalışmayan** dosyalar:
`services/CountrySync.js` · `services/PriceSync.js` · `services/ServiceSync.js` ·
`services/SmsSyncService.js` · `controllers/smsController.js` ·
`controllers/providerController.js` (`platformMappingIndex`, `autoMappingHera`, `autoMappingServicesHero`)

**Ders:** Tip sistemi olmadan bu hatalar çalışma zamanına kadar gizlenir. → **ADR-001 (Go, statik tip).**

### 3.2 Alan adı uyuşmazlığı — sessiz veri kaybı

`services/SmsManager.js` `SmsOrder` kaydı oluştururken şu alanları yazıyor:
`userId`, `providerId`, `service`, `phone`, `price_sold`, `order_id`, `code`
Gerçek model/migration alanları: `user_id`, `provider_id`, `service_id`, `phone_number`,
`price`, `remote_order_id`, `sms_code`

Sequelize bilinmeyen alanları **sessizce yok sayar** → kayıt eksik yazılırdı (dosya zaten çağrılmıyor).
Ayrıca `controllers/orderController.js` `buyNumber` fonksiyonunu import ediyor ama `SmsManager`
`buyNumberSmart` ihraç ediyor → `buyNumber is not a function`.

**Ders:** ORM'un sessiz davranışları para sisteminde kabul edilemez. Yeni sistemde ORM yok —
sqlc ile her sorgu açık SQL. → **[design.md](design.md) §2.1**

### 3.3 Yetkilendirme açıkları — üçü de kritik

| Dosya:satır | Açık |
|---|---|
| `routes/userRoutes.js:34` | `GET /admin/user/payments/approve/:id` yalnız `ensureAuthenticated` → **her kullanıcı kendi ödemesini onaylayıp bakiyesini kendi yükleyebilir** |
| `routes/userRoutes.js:37` | `POST .../payments/reject/:id` aynı sorun → başkasının ödemesi reddedilebilir |
| `routes/userRoutes.js:29` | `POST /admin/user/profile/update/:id` yalnız `ensureAuthenticated`; `updateProfile` `req.params.id`'yi kullanıyor → **herhangi bir kullanıcı başka bir hesabın e-postasını ve şifresini değiştirebilir (hesap ele geçirme)** |
| `controllers/userPanelController.js:589` | `getOrderStatus` sipariş sahipliğini kontrol etmiyor → **bir kullanıcı başkasının SMS kodunu okuyabilir** |
| `controllers/userPanelController.js:660` | `setOrderStatus` de sahiplik kontrol etmiyor → başkasının siparişi iptal edilebilir |

Ayrıca ödeme onayı bir **GET** isteği → bir `<img src="...approve/12">` etiketiyle tetiklenebilir.

**Ders:** Yetki iki katmanlı ve **sorgunun parçası** olmalı. → **[design.md](design.md) §10.**

### 3.4 `provider.balance` anlam çakışması

Migration'da `SmsProviders.balance` "sağlayıcıdaki bakiyemiz" olarak tanımlı. Ama:
- `userPanelController.js:~440` → yüzde **marj** olarak kullanıyor: `cost + (cost * balance)/100`
- `smsController.js:~57` → **çarpan** olarak kullanıyor: `cost * balance`
- `providerController.js` → panelden serbest sayı olarak giriliyor

Aynı sütun üç farklı anlamda → fiyatlar tutarsız.

**Ders:** Bir sütunun tek bir anlamı olur. → Yeni şemada `account_balance_minor` (bakiye) ve
`cost_multiplier` (düzeltme) **ayrı sütunlar**; marj ise `pricing_rules` tablosunda.

### 3.5 Marj hesaplanıyor ama uygulanmıyor — sistem maliyetine satıyor

`controllers/userPanelController.js:384` → `const myProfitMargin = 1.5;` tanımlanıyor
`controllers/userPanelController.js:464` → `const finalUserPrice = bestOffer.cost;` ← **marj kullanılmıyor**
`controllers/userPanelController.js:342` → `const profit = 50;` de hiç kullanılmıyor

**Ders:** Fiyatlandırma tek bir saf fonksiyonda toplanmalı ve **altın testle** sabitlenmeli.
→ **FR-304, KK-304.**

### 3.6 Para aritmetiği ve eşzamanlılık

- Bakiye `DECIMAL(10,2)`, JS'e string gelir, `parseFloat` ile float'a çevrilir, aritmetik float'ta
  yapılır, geri yazılır → yuvarlama hataları birikir
- Kilit yok, `SELECT ... FOR UPDATE` yok → **kayıp güncelleme**
- `approvePayment` (`userPanelController.js:~240`): `if (payment.status === 'completed')` kontrolü
  transaction dışında → **TOCTOU yarışı**, iki paralel istek iki kez bakiye yükler
- `user.save()` ve `payment.save()` ayrı ayrı, transaction yok → biri başarısız olursa tutarsız durum

**Ders:** → **ADR-004 (int64 kuruş), ADR-006 (ledger), FR-201, FR-202.**

### 3.7 Uzun transaction içinde dış çağrı

`confirmPurchase` (`userPanelController.js:482`) bir transaction açıyor ve içinde **iki HTTP çağrısı**
yapıyor (`:506` fiyat kontrolü, `:535` numara alma, 10 sn zaman aşımı).

Sonuçlar: bağlantı havuzu tükenmesi, kilit birikmesi ve en kötüsü — süreç `:535` ile `commit`
arasında ölürse **sağlayıcıdan numara alınmış ama kullanıcıdan tahsilat yapılmamış** olur.

`setOrderStatus` (`:660`) daha ince bir hata taşıyor: `status == 8` ve kullanıcı bulunamadığında
`t.rollback()` yapıp yine de `success: true` dönüyor — oysa sağlayıcıdaki iptal **zaten gerçekleşmiş**.

**Ders:** → **ADR-008 (provizyon/onay deseni), FR-408 (yetim provizyon kurtarma).**

### 3.8 İstemciye güven

`getLivePrice` istemciye `provider_id` döndürüyor; `confirmPurchase` bunu **gövdeden** okuyup
kullanıyor (`service_id`, `country_id`, `operator` da öyle). Kullanıcı istediği sağlayıcıyı seçebilir.
Fiyat satın alma anında yeniden hesaplanıyor — iyi niyet, ama kur iki çağrı arasında değişirse
**gösterilenden farklı tutar tahsil edilir**.

**Ders:** → **ADR-007 (sunucu tarafı `price_quotes`), FR-305, FR-401.**

### 3.9 Kur yönetimi

`getUsdToTryRate` (`userPanelController.js:363`) her fiyat sorgusunda ve her satın almada anahtarsız
ücretsiz bir API'yi çağırıyor; **önbellek yok, zaman aşımı yok**, hata durumunda sabit `43.64`
(`:369`) dönüyor — bu değer koda gömülü ve bayat.

**Ders:** → **FR-302 (önbellek + tampon + bayat kurda satış durdurma).**

### 3.10 Sessiz çalışmama: reCAPTCHA

`authController.login` `process.env.RECAPTCHA_SECRET_KEY` kullanıyor ama `.env` dosyasında
**yalnız `JWT_SECRET` ve `DATABASE_URL` var**. Google'a `secret=undefined` gidiyor,
`googleResponse.data.success` false dönüyor → **giriş her zaman 400 veriyor.**

**Ders:** Yapılandırma açılışta doğrulanmalı, eksikse **süreç başlamamalı**.
→ **NFR-802, [design.md](design.md) §15.**

### 3.11 Oturum ömrü uyuşmazlığı

`authController.js:55` → JWT `expiresIn: '1h'`
`authController.js:62` → çerez `maxAge: 24 saat`

Kullanıcı 1 saat sonra token'ı geçersiz ama çerezi duruyor → sonsuz yönlendirme döngüsü.
Ayrıca JWT iptal edilemiyor: kullanıcı yasaklandığında token 1 saat daha geçerli.

**Ders:** → **ADR-003 (sunucu tarafı oturum).**

### 3.12 Hata gösterimi

`app.js:93` → `res.status(500).send("<h1>Hata!</h1><p>" + err.message + "</p>")`
Kaçırılmamış (unescaped) hata mesajı → **yansımalı XSS** + yığın/SQL detayı sızıntısı.

**Ders:** → **[design.md](design.md) §11 (tipli hata + genel mesaj + `requestId`).**

### 3.13 Sızmış sırlar

- `config/config.json` ve `config/database.json` **git'te takipli** (`.gitignore`'a sonradan eklenmiş,
  takipten çıkarılmamış) → **PostgreSQL şifresi repo geçmişinde**
- `controllers/userPanelController.js:7` → **Shopier API anahtarı ve secret'ı kodda düz metin**
- 7 adet `.DS_Store` dosyası takipli

**Yapılacak:** İkisi de **rotate edilmeli** (yeni repoya taşınmayacak olsa bile, eski repo geçmişinde
kalıyorlar). → `roadmap.md` M0.

### 3.13b ÇÖZÜLDÜ: katalog testlerindeki kararsız düşüş

`check.sh` bir kez `service/catalog` paketindeki altı testin tamamıyla düşmüştü;
sonraki koşular geçmişti ve sebep bulunamamıştı.

**Sebep:** katalog testlerinin temizliği `DELETE FROM products` yapıyor ama
`price_quotes.product_id` bu tabloya RESTRICT bir FK ile bağlı. Başka bir
paketten kalan TEK bir teklif satırı, ortak `setup` içindeki bu silmeyi FK
ihlaliyle düşürüyor ve paketteki bütün testler aynı anda patlıyor. Kalıntı
teklif bazen oluyor bazen olmuyordu — "kararsız test" görüntüsü buradan
geliyordu.

**Düzeltme:** `price_quotes` bağımlı tablo olarak ÖNCE siliniyor.

**Kendi sürecimdeki hata — kayda değer:** bu hipotez ilk seferinde kurulmuştu
("kalıntı `price_quotes` temizliği düşürüyor") ama yanlış test edildi: `products`
yerine `providers` FK'sı denendi, test geçti ve hipotez **çürütülmüş sayıldı.**
Doğru hipotez yanlış deneyle elendi. Ders: bir hipotezi çürütürken, deneyin
hipotezin TAM İFADESİNİ sınadığından emin ol — yakın bir varyantını değil.

**Teşhisi mümkün kılan:** `check.sh` artık her adımın tam çıktısını
`.check-logs/` altına yazıyor. İlk seferinde çıktı atılmıştı ve elde tahminden
başka bir şey yoktu.

### 3.14 Yönlendirme ve rota tutarsızlıkları

| Yer | Sorun |
|---|---|
| `routes/adminRoutes.js:34` | `router.post('users/assign-role', ...)` — **baştaki `/` eksik** → rota `/adminusers/assign-role` olarak kayıtlanıyor, ölü |
| `controllers/AdminDepositController.js:25,42,45` | `/admin/user/deposit-methods` adresine yönlendiriyor ama gerçek rota `/admin/deposit-methods` → her kayıttan sonra 404 |
| `routes/webRoutes.js` | `res.render('index')` ve `res.render('../web/blog')` karışık kullanım |
| Genel | Kullanıcı özellikleri `/admin/user/*` altında, view'lar `views/admin/users/*` içinde — admin/kullanıcı sınırı modellenmemiş |

### 3.15 Diğer

- `controllers/userPanelController.js:214` → `bcrypt.genSalt` çağrılıyor ama dosyada **`bcrypt` import edilmemiş**
  → şifre değiştirme her zaman çöküyor
- `controllers/ticketController.js:60` → `res.locals.user.role` okunuyor; Sequelize kullanıcı nesnesinde
  böyle bir alan **yok** → `isAdmin` her zaman `false`, admin yanıtları kullanıcı yanıtı sayılıyor
- `app.js` → `express.json()` **iki kez** kayıtlı
- `node-cron` bağımlılıkta ama **hiç kullanılmıyor**; senkron servisleri hiçbir yerden çağrılmıyor
  → katalog verisi elle güncellenmek zorunda
- `mongoose`, `prisma`, `@prisma/client`, `pg-hstore` kullanılmıyor; `bcrypt` **ve** `bcryptjs` birlikte
- `tailwindcss@3` + `@tailwindcss/cli@4` sürüm çakışması
- `prisma/schema.prisma` boş bir iskelet (datasource var, model yok) — yarım kalmış ORM göçü
- `middlewares/UserStatusCheckMiddleware.js` `req.user.status` okuyor, `req.user` yoksa çöküyor
- `views/admin/dashboard.ejs` 922 satır; satın alma, yoklama ve iptal mantığının tamamı inline `<script>`
- Test yok, lint yok, CI yok, `README.md` yok, `.env.example` yok
- 127 commit'in tamamının mesajı `"fix"`

---

### 3.12 Yorum "kullanılmaz" diyordu, kod tam da onu yapıyordu

`scripts/smoke-auth.sh` başında büyük harfle "TRUNCATE ... CASCADE KULLANILMAZ"
yazıyordu ve gerekçesi de doğruydu: cascade `pricing_rules`a iniyor, varsayılan
GLOBAL fiyat kuralı siliniyor. Betiğin 113. satırında ise tam olarak
`TRUNCATE ledger_entries, users RESTART IDENTITY CASCADE` duruyordu.

Sonuç: testler yeşil geçiyor, ardından uygulama açıldığında her fiyat teklifi
`NO_PRICING_RULE` ile düşüyordu. Kontrolleri koşan kişi "her şey geçti" görüyor,
uygulamayı açan kişi bozuk bir sistem buluyordu.

Bu Örüntü A'nın (bkz. §3.11) kendi araçlarımızdaki hâlidir. `check-guarantees.py`
kabuk betiklerini zaten tarıyordu ama garanti sözcükleri listesinde
"kullanılmaz/yapılmaz" yoktu — yani denetleyici doğru dosyaya bakıp yanlış
kelimeyi arıyordu. Liste genişletildi ve 10 yorum daha teste bağlandı.

**Ders:** "X yapılmaz" en az "asla" kadar bağlayıcı bir sözdür. Bir denetleyici
kurunca, onun NEYİ KAÇIRDIĞINI da düşünmek gerekir; kapsama alanı ile kelime
listesi ayrı iki eksiktir.

### 3.13 Kontroller geçti ≠ sistem çalışıyor

Aynı turda üç ayrı "testler geçiyor ama uygulama açılmıyor" durumu çıktı:

1. `godotenv.Load(".env", "../.env")` ilk dosya yoksa hemen dönüyor — `make dev`
   `api/` dizininden koştuğu için kökteki `.env` HİÇ okunmuyordu. Sunucu
   "DATABASE_URL tanımsız" diyerek ölüyordu, dosya oracıkta dururken.
2. Duman testi kendi sunucusunu başlatır; geliştirme sunucusu ayaktayken port
   dolu olduğu için "sunucu başlamadı" diye sebebi söylemeyen bir hata veriyordu.
3. `check.sh`e eklenen `npm run build`, çalışan `npm run dev` ile aynı `.next`
   dizinini ezip tarayıcıda `__webpack_modules__[moduleId] is not a function`
   üretiyordu — kodda hiçbir sorun yokken.

Üçü de aynı kökten: **kontroller, sistemin çalışır hâlinden habersiz koşuyordu.**
Çözümler sırasıyla: her yolu ayrı yükle, portu önceden kontrol edip açık söyle,
üretim derlemesini ayrı dizine yap (`NEXT_DIST_DIR`).

### 3.15 Denetleyici, atıf yapılan testin VAR OLDUĞUNU doğrulamıyordu — ÇÖZÜLDÜ

`check-guarantees.py` bir garanti yorumunda `test:` yazısının **varlığını**
arıyordu, işaret ettiği testin gerçekten bulunduğunu değil. Sonuç: garanti
veren bir yoruma uydurma bir test adı yazmak denetimi geçiriyordu.

Bunu ben yaptım. Webhook ve yönetim kodundaki altı garanti yorumu referanssız
kaldığında hepsine toplu olarak referans ekledim; ikisi (
`TestSavingProviderSettingsDoesNotEraseAPIKey`,
`TestHandlerDoesNoWorkBeyondEnqueue`) var olmayan testlere işaret ediyordu.
Denetim yeşile döndü, garantiler dayanaksızdı — denetleyicinin önlemek için
yazıldığı hatanın (§3.12) tam olarak kendisi.

Denetleyici artık depodaki tüm test adlarını indeksliyor ve her atfı üç
düzeyde doğruluyor: test adı var mı, atfedilen dosyada mı, yol verilmişse o
dosya diskte mi. Açılışta **20 kırık atıf** çıktı:

| tür | sayı | örnek |
|---|---|---|
| yol indeks dışıydı (yanlış alarm) | 14 | `scripts/smoke-auth.sh` |
| test başka dosyaya taşınmış | 4 | `ratelimit_test.go` → `middleware_test.go` |
| **test hiç yoktu** | 2 | `TestSessionListDoesNotLeakToken`, `TestAdjustIsIdempotent` |

Son ikisi benim eklediklerimden önce de oradaydı: oturum listesinin ham
taşıyıcı token'ı sızdırmadığı ve elle bakiye düzeltmesinin idempotent olduğu
— ikisi de yazılı garantiydi, ikisinin de testi yoktu.

**Ders:** bir kontrolün geçmesi, kontrol ettiğini sandığınız şeyin doğru
olduğu anlamına gelmez. Kontrolü de sabote edin.

### 3.14 Geliştirme veritabanı testlerle paylaşılıyordu — ÇÖZÜLDÜ

`make check` koşulduğunda geliştirme hesabı, sağlayıcı kaydı ve tüm katalog
siliniyordu: entegrasyon testleri `DELETE FROM providers` / `DELETE FROM users`
yapar — ki doğrusu budur, test kendi ön koşulunu kurmalıdır. Sorun testlerde
değil, **aynı veritabanını paylaşmalarındaydı.** Tek bir oturumda beş kez
yaşandı; her seferinde 20 saniyelik katalog senkronu tekrarlandı.

Çözüm: testler `smsplatform_test` veritabanını kullanır. `check.sh` yoksa
oluşturur, migration'ları uygular ve `DATABASE_URL`i dışa aktarır.

Bunu yaparken **iki tuzak** çıktı:

1. `${DATABASE_URL/\/smsplatform?/...}` — bash desen değiştirmede `?` bir
   JOKER karakterdir. URL bozuldu ve `smsplatform_test` bir HOSTNAME olarak
   yorumlandı. Sabit dize değişimi için `sed` kullanılıyor.
2. `smoke-auth.sh` başında `set -a && source .env` vardı ve çağıranın verdiği
   `DATABASE_URL`i **eziyordu**. Duman testi bu yüzden hâlâ geliştirme
   kullanıcılarını siliyordu. Artık mevcut ortam kazanır, `.env` yalnız
   boşlukları doldurur — Go tarafındaki `config.LoadDotEnv` ile aynı öncelik.
   **İki yerde iki farklı öncelik olamaz.**

Kanıt: `check.sh` öncesi ve sonrası kullanıcı/sağlayıcı/teklif sayıları aynı.

## 4. Sağlayıcı API notları

### 4.1 HeroSMS — ✅ spec doğrulandı + ⚠️ kısmen çıkarım

> Tam referans: **[provider-herosms.md](provider-herosms.md)** — resmî OpenAPI 3.2.0 (45 path,
> 57 şema) + canlı doğrulama + **104 ajanlı çapraz doğrulama turu** (her mimari iddia 2 bağımsız
> hasım doğrulayıcıdan geçirildi).

**Eski "doğrulanması gereken noktalar" listesinin GÜNCEL durumu:**

| Soru | Cevap |
|---|---|
| Para birimi? | ⚠️ **ÇIKARIM — spec'te yazmıyor.** Fiyat/stok/bakiye uç noktalarının **hiçbiri** `currency` döndürmüyor; USD yalnız `Currency.default = 840`'tan çıkarıldı → ❓H6b |
| Numara geçerlilik süresi? | ✅ `ActivationSchema.expiredAt`. ⚠️ **Varsayılan süre spec'te hiç yok** |
| Hız limiti? | ⚠️ **Kısmen:** `429 RATE_LIMIT` + `Retry-After` **yalnız `offers`** için tanımlı; sayısal değer yok. Ayrıca **ayrı bir eşzamanlılık limiti** var: `403 CHANNELS_LIMIT` |
| Tam hata listesi | ✅ Tam — `provider-herosms.md` §9. Zarf alanı **`title`**, `code` değil |
| Modern mi legacy mi? | ✅ Modern birincil, **legacy ZORUNLU** — katalog, bakiye ve kiralık fiyat/stok modern'de **hiç yok** |
| Kiralık uç noktaları? | ✅ Satın alma + uzatma modern'de; **fiyat/stok yalnız legacy'de** |
| İptal sonrası iade? | ⚠️ **İki eşikli:** ilk **120 sn** yasak (`minActivationTime`), sonra **20 dk** penceresi, OTP geldiyse hiç. İkisinin birlikte işleyişi belirsiz → ❓H2/H3 |
| Webhook kaynak IP'leri? | ✅ **Spec'te yazılı:** `84.32.223.53`, `185.138.88.87` |
| Sayfalama azami boyut? | ✅ **25** (modern) |
| Idempotency? | ❌ **Yok** — spec'te "idempot" 0 kez geçiyor |
| `ActivationStatusTypes` anlamları? | ❌ **[1,2,3,4,6,7,8,10] için `x-enum-descriptions` YOK.** Yalnız 6/8/10 açıklamalı → ❓H1 |

**🔴 Bu turda ortaya çıkan HATALAR (dokümanlarımızda düzeltildi):**

| Hata | Nerede | Düzeltme |
|---|---|---|
| **`GET /activations/{id}` uç noktası kullanılıyordu — SPEC'TE YOK** | `design.md` §8.3, `trd.md` FR-410/KK-410, `provider-herosms.md` §7.2, ADR-019 | → `GET /{id}/otp/last` · **ADR-022**. *ADR-019'un kararı doğru, mekanizması var olmayan bir çağrıya dayanıyordu* |
| `order-poller` sıklığı **üç yerde çelişiyordu** (5 sn / 30 sn / 5 sn) | §13 vs §8.3 vs FR-404 vs FR-411 | **30 sn**'de birleştirildi |
| `Cancel()` tek başına yetersizdi | `ProviderPort` | `Finish()` eklendi · **ADR-023** |
| `fixedPrice` "tavan" sanılıyordu | ADR-018 | **"tam o fiyat" demek** → gönderilmeyecek · **ADR-027** |
| `Extend(d time.Duration)` imzası yanlıştı | `ProviderPort` | Sağlayıcı **sabit enum** istiyor → `RentDurationHours` |
| Maliyet sent'te saklanacaktı | §5.1 | Fiyatlar 4 ondalıklı, `MaxPrice.min = 0.0067` → **mikro-birim** · **ADR-026** |
| `products` UNIQUE kısıtı e-posta ürününde çift satıra izin verirdi | §6 | `NULLS NOT DISTINCT` |

**Yeni sürprizler:**
- 🔴 **OTP alan adları ÜÇ farklı isim seti:** modern `{smsCode, smsText, receivedAt}` ·
  legacy `{code, text, date}` (RFC3339!) · webhook `{code, text, receivedAt}` (ISO8601).
  **§3.2'deki "alan adı uyuşmazlığı = sessiz veri kaybı" dersinin birebir tekrarı riski.**
- 🔴 **Webhook yanıt zaman aşımı 3 saniye**, ≥7 yeniden deneme → aynı SMS **≥8 kez** gelebilir
- 🔴 **Webhook `id` alanı `required` ama `properties`'te TANIMSIZ** (spec hatası) → ❓H14
- 🔴 **`POST /activations` yanıtı DİZİ** — `amount:1` olsa bile `data[0]`
- 🔴 **Modern satın almada `NO_NUMBERS` / `NO_BALANCE` TANIMSIZ** — en sık iki hatamızın
  modern karşılığı bilinmiyor → ❓H11/H12
- 🔴 **Modern `DELETE`/`finish` için `409` TANIMSIZ** — iş kuralları yalnız legacy'de → ❓H13
- ⚠️ **`code` webhook'ta `required` DEĞİL**, `code`/`text` nullable; `verificationType: "call"`
  aktivasyonlarında ikisi de null olabilir → **hatalı iade riski**
- ⚠️ **Bir aktivasyona BİRDEN FAZLA SMS gelebilir** (`otpList` dizi, `canGetAnotherSms`)
- ⚠️ **Sonlandırılmış aktivasyondan geçmiş mesaj okunamaz** → mesajlar **bizde kalıcı** (FR-415)
- ⚠️ **`operator` boyutu fiyatlanamıyor** — hiçbir uçta operatör kırılımlı fiyat/stok yok · ADR-029
- ⚠️ **Kiralık süreleri spec'te ÜÇ yerde çelişiyor** → canlıdan öğrenilir
- ⚠️ **Spec canlıdan geride:** `Country.rent` şemada yok ama canlı döndürüyor; `GET /activations`
  `meta` şemada yok ama döndürüyor; legacy `application/json` ilan edip `text/html` gönderiyor
  → **fixture'lar spec'ten değil CANLIDAN üretilir**
- ✅ **`resellerUserId`** → yetim provizyon korelasyonu için (ama yanıtta geri dönmüyor, ❓H17)
- ✅ **`GET /activations/stats`** → sağlayıcı seçim skorlaması için başarı oranı kaynağı
- ✅ **`BANNED.info.scope`** `global` \| `specific` → devre kesici granülaritesi · ADR-028
- ⚠️ **`/emails` (7 op)** ve **`call`** iki ek ürün tipi; e-posta için **webhook yok** ve
  `site` kataloğunu listeleyen uç nokta **yok**

### 4.2 5sim (protokol: `FIVE_SIM`) — v1.1, doğrulanmadı

Mevcut eski kodda yarım bir adaptör var (`controllers/smsController.js`):
- Kimlik: `Authorization: Bearer <api_key>` başlığı (sorgu parametresi değil)
- Fiyat: `GET https://5sim.net/v1/guest/prices?product=<servis>`
- Yanıt: `{ "<ülkeAdı>": { "<operatör>": { "<servis>": { "cost": 0.5, "count": 20 } } } }`
  — HeroSMS'ten farklı olarak **ülke adı** (kod değil) ve **operatör kırılımı** var

**Ders:** Bu fark, `provider_dimension_maps` tablosunun neden `dimension` sütunu taşıdığını
gösteriyor: 5sim'de `country` boyutu `"turkey"` gibi bir metin, HeroSMS'te `62` gibi bir sayı.

> ⚠️ Bu bilgiler eski koddan çıkarımdır, **doğrulanmamıştır**. İkinci sağlayıcı `M7`'de seçilecek.
> HeroSMS SMS-Activate uyumlu olduğu için, ikinci sağlayıcı olarak **SMS-Activate** seçilirse
> legacy adaptörü büyük ölçüde yeniden kullanılabilir.

## 5. Bilinen tuzaklar (yeni sistemde dikkat)

| Tuzak | Neden dikkat | Nerede ele alındı |
|---|---|---|
| **SSE ters vekil arkasında tamponlanır** | Caddy/Nginx varsayılan olarak yanıtı tamponlar → kod ekranda geç görünür | Caddy `flush_interval -1` + `X-Accel-Buffering: no` başlığı. M5'te **erken** test et |
| **sqlc üretimi bayatlayabilir** | `queries/*.sql` değişip `make gen` unutulursa kod eski SQL'i çalıştırır | CI'da `sqlc diff` — üretim güncel değilse derleme düşer |
| **Dinamik filtreyi string birleştirme ile kurma isteği** | SQL enjeksiyonu ve okunamaz kod | `sqlc.narg()` + `COALESCE` deseni — `design.md` §2.1 |
| **Go'da `defer tx.Rollback()`** | `Commit` sonrası `Rollback` hata döner ama zararsız — **hatayı yutma** | Standart desen: `defer func(){ if err != nil { tx.Rollback() } }()` |
| **Go'da `for` döngüsünde goroutine** | Go 1.22 öncesi döngü değişkeni paylaşılırdı; 1.22+ düzeldi ama `errgroup` ile yine dikkat | `errgroup.Group` kullan, sonuçları kanal veya indeksli dilim ile topla |
| **`context` iptali dış çağrıyı iptal etmez** | HTTP istemcisine `ctx` verilmezse zaman aşımı işlemez | Her dış çağrı `http.NewRequestWithContext` |
| **`Purchase` yeniden denenirse iki numara alınır** | İdempotent değil | Yeniden deneme **yasak** — `design.md` §9 |
| **Next.js `middleware.ts` güvenlik sınırı değil** | Yalnız yönlendirme yapar; atlanabilir | Gerçek yetki her zaman Go tarafında — `design.md` §14.2 |
| **Para JSON'da float'a dönüşür** | `12.50` JavaScript'te `12.499999...` olabilir | API'de her zaman `{minor, currency, formatted}` — `trd.md` §9 |
| **`CITEXT` uzantısı gerekli** | E-posta/kullanıcı adı büyük-küçük harf duyarsız benzersizlik için | İlk migration'da `CREATE EXTENSION citext` |
| 🔴 **HeroSMS `count` sahte stok gösterir** | WhatsApp×TR: `count=56964` ama `physicalCount=0`. `count` kullanılırsa kullanıcıdan para çekilir, sağlayıcı boş döner | Stok **her zaman** `counts.physical`/`physicalCount` — `trd.md` FR-306 |
| 🔴 **HeroSMS webhook'unda imza yok** | URL'i bilen herkes sahte "kod geldi" gönderebilir | IP izin listesi (`84.32.223.53`, `185.138.88.87`) + kod `GET /{id}/otp/last` ile teyit — ADR-019, ADR-022 |
| **`POST /activations` idempotent değil** | Yeniden deneme iki numara alır, iki kez ücretlendirir | Asla yeniden deneme — `CLAUDE.md` değişmez #6 |
| **Legacy yanıtlar `text/html` döner** | JSON ayrıştırıcıya verilirse patlar (`ACCESS_BALANCE:0.5632`) | Legacy çağrılar ayrı ayrıştırıcıdan geçer |
| **İptal penceresi dar ve koşullu** | İlk **120 sn** yasak → sonra **20 dk** penceresi → OTP geldiyse hiç | Kullanıcıya iade koşulsuz; iptal butonu 120 sn pasif (FR-416); sağlayıcı iadesi ayrı takip |
| 🔴 **`GET /activations/{id}` diye bir uç nokta yok** | Sezgisel görünüyor ama spec'te o yolda yalnız `delete` var — dokümanlarımıza bir kez sızdı | Teyit `/{id}/otp/last` ile (ADR-022) |
| 🔴 **Webhook 3 sn'de 200 istiyor** | Handler'da DB+teyit yapılırsa zaman aşımı → aynı SMS 8 kez işlenir | Ham gövde kuyruğa, anında 200 (ADR-024) |
| 🔴 **`POST /activations` yanıtı dizi** | `amount:1` olsa bile. Tekil obje beklenirse nil deref | `data[0]`; `len != 1` → hata |
| 🔴 **OTP alan adları üç farklı isim seti** | §3.2'deki sessiz veri kaybı hatasının aynısı | Adaptörde tek normalizasyon noktası (`provider-herosms.md` §7.3) |
| ⚠️ **`fixedPrice` "tavan" değil "sabit fiyat"** | Piyasa düşse bile tavan ödenir | Gönderilmez (ADR-027) |
| ⚠️ **Spec canlıdan geride** | Şemada olmayan alanlar canlı geliyor | Fixture'lar **canlıdan** üretilir (NFR-804) |
| **HTTP/1.1'de origin başına 6 bağlantı** | Açık SSE bağlantısı slot tutar; 6+ sekmede uygulama kilitlenir | **HTTP/2 zorunlu.** Yerelde HTTP/1.1 olduğu için tuzak fark edilmez → yerel HTTPS ile test — `frontend-contract.md` §4.1 |
| **iOS Safari arka planda SSE'yi öldürür** | Kullanıcı geri döndüğünde akış ölü, **hata yok**, kod hiç gelmez | `visibilitychange` + `pageshow` → REST ile senkronla, akışı yeniden kur |
| **Vekil 502'de HTML döner** | `res.json()` `SyntaxError` atar, kullanıcı anlamsız hata görür | İstemci sarmalayıcısı `content-type` kontrol eder — `frontend-contract.md` §3.1 |
| **Safari boşluklu tarihi ayrıştıramaz** | `new Date("2026-09-08 10:00")` → `Invalid Date` | Yalnız RFC 3339 (`T` ve `Z` ile) |
| **iOS'ta <16px girdi yakınlaştırır** | Alana dokununca ekran zıplar, form kullanılamaz | Tüm girdilerde `text-base` (16px) minimum |
| **`100vh` iOS'ta adres çubuğunu saymaz** | Sayfa altı kırpılır | `100dvh` + `@supports` yedeği |
| **`fetch`'in zaman aşımı yoktur** | Mobil ağ koparsa arayüz sonsuza kadar yükleniyor kalır | `AbortController` + 15 sn zaman aşımı — istemci sarmalayıcısında |
| **`navigator.onLine` güvenilmez** | İnternetsiz Wi-Fi'de `true` döner | Yalnız ipucu; gerçek karar başarısız isteğe göre |
| **Yerel geri sayım sekme donunca durur** | Kullanıcı "2 dakikam vardı" der, süre çoktan dolmuş | Kalan süre **her zaman** sunucudaki `expiresAt`'ten hesaplanır |

---

## 6. Açık sorular

| # | Soru | Kimden | Ne zaman gerekli |
|---|---|---|---|
| ~~S1~~ | ~~HeroSMS resmî API dokümanı~~ | ✅ Alındı, 104 ajanlı turla analiz edildi → [provider-herosms.md](provider-herosms.md) | — |
| **S2** ⚠️ | **YENİDEN AÇILDI** — para birimi bir **çıkarımdır**; fiyat/stok/bakiye uç noktaları `currency` döndürmüyor | Canlı: `?action=getNumberV2` yanıtında `currency` **var** (❓H6b) | **M3** |
| ~~S3~~ | ~~Numara geçerlilik süresi~~ | ✅ **Evet**, yanıttaki `expiredAt` | — |
| S4 | Kur (FX) sağlayıcısı: TCMB mi ücretli API mi? | Karar | M4 |
| S5 | E-posta sağlayıcısı ve alan adı | Karar | M1 |
| S6 | Alan adı alındı mı? TLS için gerekli | Karar | M8 |
| S7 | İkinci sağlayıcı hangisi? | Karar | M7 |
| S8 | Hedef kâr marjı yüzdesi (başlangıç değeri) | Karar | M4 |
| S9 | VPS sağlayıcısı ve boyutu | Karar | M8 |
| S10 | Lansman kapısı kararı | Karar | M9 |
| ~~S11~~ | ~~Gerçek iOS cihaz erişimi~~ | ✅ **Var** | — |
| ~~S12~~ | ~~İptal asgari bekleme~~ | ✅ **120 sn** (`info.minActivationTime`). Sabit mi değişken mi → ❓H2 | — |
| ~~S13~~ | ~~Ücretsiz iptal penceresi~~ | ✅ **20 dakika** (`FREE_CANCELLATION_EXPIRED`). "VE mi VEYA mı" → ❓H3 | — |
| ~~S16~~ | ~~Webhook kaynak IP'leri~~ | ✅ **Spec'te yazılı:** `84.32.223.53`, `185.138.88.87` | — |
| **S14** | `prices.min`/`default`/`retail` — hangisinden ücretlendiriliyoruz? | Canlı satın alma (❓H6) | **M3 — yüksek** |
| **S15** | `ActivationStatusTypes` 1,2,3,7 anlamı | Canlı yaşam döngüsü (❓H1) | **M3 — yüksek** |
| **S17** | Modern `/activations` hız limiti (429 yalnız `offers`'ta) | Kademeli yük / destek | Orta |
| **S18** | Webhook `id` alanı nedir + **imza/secret başlığı geliyor mu** | İlk gerçek webhook'un ham başlıkları (❓H14) | **M5 — güvenlik** |
| **S19** | Modern `POST /activations` stok/bakiye yokken hangi kod + `title` | Canlı (❓H11/H12) | **M3 — yüksek** |
| **S20** | Modern `DELETE` iptal reddinde 409 mu 422 mi | Canlı (❓H13) | **M3 — yüksek** |
| **S21** | `expiredAt` geçince otomatik iade var mı | Canlı (❓H10) | **M3 — yüksek** |

> **Tam canlı test listesi (H1–H21) ve maliyet tahmini:** [provider-herosms.md](provider-herosms.md) §12

---

### H22 — Kiralık `count` gerçek stok mu?

`?action=serviceCountRent` yanıtındaki `count` alanının gerçek envanter mi
yoksa aktivasyon tarafındaki `total` gibi bir HAVUZ SAYACI mı olduğu spec'te
BELİRTİLMEMİŞ.

Bu önemli çünkü aktivasyon tarafında tam bu tuzak yaşandı: `counts.total`
14.048 gösterirken gerçek stok (`counts.physical`) 0'dı ve eski prototip
kullanıcıdan para çekip boş dönüyordu. Kiralık yanıtında `physical` benzeri
bir ayrım YOK — yalnız `count` var.

**Bugünkü davranış:** `count` olduğu gibi stok sayılıyor.
**Nasıl anlaşılır:** stokta göründüğü hâlde `NO_NUMBERS` ile düşen ilk kiralık
satın alma bunu ortaya çıkarır. O anda `count` bir havuz sayacıdır ve kiralık
stok gösterimi güvenilmez demektir.
**Ne yapılmalı:** ilk canlı kiralık satın almadan sonra bu madde kapatılmalı.

---

### H23 — Türkiye numarası: sağlayıcının sitesi veriyor, API'si vermiyor

**Soru (kullanıcıdan, 2026-09-08):** "Kendi sitesinde HeroSMS Türkiye numarası
veriyor." Bizim panelde Türkiye "stok yok" görünüyor. Hangisi doğru?

**Ölçüm (canlı, 2026-09-08, salt okunur):**

| kaynak | Türkiye (ülke kodu 62) |
|---|---|
| `/activations/offers/sms` toplu uç | 122 servis · **Σ physical = 0** · Σ total = 9.828.957 |
| `GetPriceAndStock` tek tek (abb, ah, fb, vi, tg, wa, ot) | hepsinde **physical = 0** |
| `?action=serviceCountRent` (wa, tg, ot, vi, fb) | Türkiye teklifi **YOK** (0 kayıt) |
| ülke kaydı bayrakları | `rent: true`, `retry: true` |

Kıyas — `physical` seyrek dolan bir alan DEĞİL: **195 ülkenin 67'sinde > 0**.
İngiltere Σ 12.388.568 · Endonezya Σ 10.632.444 · Brezilya Σ 4.862.433.
Yani alan çalışıyor; sıfır olan yalnız Türkiye.

**Sağlayıcının kendi sitesindeki sayaçlar** (hero-sms.com, giriş yapmadan):
"Google,youtube,Gmail — 64.619.676 adet", "Whatsapp — 21.944.479 adet". Kendi
ipucu metinleri şunu diyor: *"Fiziksel numaralar — 8.721.190 adet, sanal
numaralar — **-8.642.940** adet"*. **Negatif** bir sayı. Site iki sayacı
birbirinden çıkarıyor ve eksi değer üretiyor — bu sayılar envanter değil.

**Sonuç:** API üzerinden bu hesaba Türkiye numarası SATILMIYOR. Sitedeki
"Türk sanal numara" ifadesi ya havuz sayacına dayanıyor ya da API'ye açılmayan
ayrı bir stoktan geliyor.

**Kesin kanıt eksik:** tek belirleyici test, Türkiye için gerçek bir satın alma
denemesidir (en ucuz servis ≈ 0,05 USD). `NO_NUMBERS` dönerse `physical` doğru;
numara gelirse FR-306 kuralı yanlış ve satılabilir stoğu gizliyoruz demektir.
**Kullanıcı onayı bekliyor.**

**Bugünkü davranış:** Türkiye ülke listesinde **en üstte** ama seçilemez
(`disabled`), etiketi "şu an stok yok". Kiralamada Türkiye hiç listelenmiyor —
sağlayıcıda kiralık Türkiye ürünü yok.

## 7. Bu dokümanı güncelleme kuralı

- **Bir karar verildiğinde** → §1'e tarihiyle ve gerekçesiyle yaz
- **Bir sağlayıcı davranışı keşfedildiğinde** → §4'e yaz, fixture kaydet
- **Bir tuzağa düşüldüğünde** → §5'e yaz (bir kez düşülen tuzağa ikinci kez düşülmez)
- **Bir soru cevaplandığında** → §6'dan çıkar, ilgili bölüme taşı
- **Eski bir karar değiştiğinde** → eskisini silme, üstünü çiz ve altına yenisini yaz

---

**İlgili:** [intent.md](intent.md) · [design.md](design.md) · [trd.md](trd.md) · [provider-herosms.md](provider-herosms.md) · [frontend-contract.md](frontend-contract.md) · [roadmap.md](roadmap.md) · [../CLAUDE.md](../CLAUDE.md)
