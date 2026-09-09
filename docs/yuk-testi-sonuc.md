# Yük testi sonucu — NFR-800

**Koşum tarihi:** 2026-09-09 09:09–09:22 (Europe/Istanbul)
**Koşan:** `./scripts/yuk-testi.sh kos` · k6 v2.2.0
**Ham veri:** `load/sonuclar/` — k6 özetleri, ham CSV, kaynak izi ve koşum
transkripti (`kosum-transkript.log`). Bu dizin **depoya girmez**
(`load/sonuclar/.gitignore`); kalıcı kayıt bu belgedir.

> **Bir uyarı, dürüstlük gereği:** raporun yazımı sırasında betik yanlışlıkla
> bir kez daha çağrıldı ve ölçüm sunucusunun `api.log` dosyasını
> **sıfırladı**. Aşağıdaki sunucu tarafı hata sayıları (§3.4, §6.1, §6.3) o
> dosya hâlâ dolu iken sayılmıştır ve `kosum-transkript.log` ile
> `ham-satinalma.csv` üzerinden bağımsız olarak doğrulanabilir
> (477 × `503` ham CSV'de, 684 × ilerletme hatası transkriptte). `api.log`
> dosyasının kendisi artık o koşuma ait değildir.

Bu belge **ölçüm tutanağıdır**. Buradaki her sayı `load/sonuclar/` altındaki bir
dosyadan gelir; hiçbiri tahmin değildir. Ölçülemeyen şey "ölçülmedi" diye yazar,
"muhtemelen yeterli" diye yazmaz.

---

## 1. Kısa hüküm

| NFR-800 hedefi | Ölçüm (p95) | Sonuç |
|---|---|---|
| Fiyat teklifi `GET /catalog/quote` < 1500 ms | **21,2 ms** | ✅ 70× marj |
| Satın alma `POST /orders` < 3000 ms | **16,1 ms** | ✅ 186× marj |
| Diğer API okuma uçları < 200 ms | **2,8 – 8,9 ms** | ✅ 22–70× marj |
| Panel sayfa yükleme (TTFB) < 400 ms | **ölçülmedi** | ⚠️ bkz. §7 |
| KK-800: 50 eşzamanlı kullanıcıda hedefler karşılanır | 50 VU | ✅ |

**Gecikme hedefleri rahatça tutuyor.** Sistemin bu yükte gecikme sorunu yok.

**Ama koşum yeşil bitmedi:** `satinalma` senaryosu %31,97 hata oranıyla düştü.
Sebep gecikme değil, **FAKE sağlayıcının 100 USD'lik bütçesinin tükenmesi**
(§4.4). Gerçek bir başarım sorunu değil, ölçüm düzeneğinin sınırı — ama bu
yolu açmak **üç gerçek kusur** ortaya çıkardı: §6.1 (eşlenmemiş DB hatası →
`500`), §6.2 (FAKE sıra çakışması), §6.3 (sağlayıcıda açık kalan numara).
Kaynak ve sorgu incelemesi dört bulgu daha ekledi (§6.4–§6.7).

**En önemli sonuç — defter:**

> Koşum boyunca **1.431 satın alma denemesi** ve **1.161 telafi iadesi**
> yazıldı (toplam 2.592 defter kaydı). Koşum sonunda `wallet:reconcile`
> **86 kullanıcının tamamında `drift_count=0`** raporladı.
> **Yük altında defter bozulmadı.**

---

## 2. Ne, nerede, neyle ölçüldü

### Donanım ve sürümler

| | |
|---|---|
| Makine | Apple **M2 Max** (Mac14,5), 12 çekirdek, 32 GB RAM |
| İşletim sistemi | macOS 26.6.1 (Darwin 25.6.0), arm64 |
| Go | go1.26.4 darwin/arm64 |
| PostgreSQL | 16.15 (Docker 29.7.2, port 55432) |
| Redis | 7.4.10 (Docker, port 56379, **db 3**) |
| k6 | v2.2.0 |

Yük üreteci (k6) ve ölçülen sistem **aynı makinede** koştu. Ağ gecikmesi
yoktur; ölçülen süreler saf sunucu işleme süresidir. Gerçek dağıtımda
istemci↔sunucu RTT bunun üstüne biner.

### Düzenek

- Ölçüm için **ayrı bir API süreci** başlatıldı (`:8191`), `make dev`'in
  8091'i kullanılmadı. Sağlayıcı **FAKE**; süreç içi stok ve bakiye taşıdığı
  için paylaşılan bir süreçte ölçüm tekrarlanabilir olmazdı.
- Sağlayıcı Redis'i **db 3**'e alındı: `scripts/smoke-auth.sh` paylaşılan
  geliştirme Redis'inde `FLUSHDB` çağırıyor ve koşumun ortasında 50 oturumu
  silebiliyor.
- Veritabanı: **geliştirme** veritabanı `smsplatform`. Betikler veri silmez
  (`scripts/yuk-testi_test.sh#silme_ifadesi_yok` bunu her koşumda doğrular).
- Yük profili **kademeli**: 0 → 10 (20 sn) → 10 sabit (40 sn) → 50 (20 sn) →
  50 sabit (60 sn) → 0. Ani yük değil.
- Ölçümden önce **200 istekle ısınma** yapıldı (bağlantı kurulumu p99'u
  yanıltmasın diye). Isınma istekleri sonuçlara girmez.
- Her senaryo **ayrı bir k6 koşumudur**. Tek koşumda karıştırmak, uç nokta
  bazlı p95 hedefleriyle karşılaştırmayı imkânsız kılardı.
- Koşum **öncesi ve sonrası** `wallet:reconcile` çalıştırıldı; başlangıçtaki
  sapma koşumu durdurur (yük altındaki sapmayı ancak temiz bir başlangıç
  kanıtlayabilir).

### ⚠️ Ölçümün bilinen kirliliği

Koşum sırasında aynı makinede `make dev` (Go API :8091 + Next.js :3000)
**ayakta ve boştaydı**. Trafik almadı, ama:

- Veritabanına **6 boşta bağlantı** tutuyordu — §5'teki `pg_stat_activity`
  sayılarından bu 6 düşülmelidir.
- 12 çekirdeğin bir kısmını (Next.js dev sunucusu) elinde tutuyordu.

Yani buradaki gecikmeler, tamamen izole bir makinede **daha iyi** olurdu;
tersi değil. Sayılar bu yönde muhafazakârdır.

---

## 3. Gecikme sonuçları

Plato satırları **50 eşzamanlı kullanıcının sabit tutulduğu 60 saniyelik
pencereden** (koşumun 80.–140. saniyesi) hesaplandı — kademe boyunca
ortalama almak, düşük yüklü ilk dakikayı sonuca karıştırır ve p95'i
olduğundan iyi gösterir.

### 3.1 Katalog gezinme (oturumsuz)

| Uç | p50 | p95 | p99 | maks | hedef |
|---|---|---|---|---|---|
| `GET /catalog/services-in-stock` | 2,03 ms | **3,86 ms** | 6,71 ms | 20,65 ms | < 200 ms |
| `GET /catalog/countries` | 1,41 ms | **2,83 ms** | 5,01 ms | 11,14 ms | < 200 ms |

3.522 istek · hata **%0,00** · plato **39,8 istek/sn**

### 3.2 Oturumlu panel

| Uç | p50 | p95 | p99 | maks | hedef |
|---|---|---|---|---|---|
| `GET /me` | 3,47 ms | **8,87 ms** | 11,45 ms | 30,36 ms | < 200 ms |
| `GET /wallet/balance` | 2,47 ms | **5,31 ms** | 7,66 ms | 32,66 ms | < 200 ms |
| `GET /orders?limit=20` | 3,03 ms | **5,43 ms** | 7,60 ms | 21,05 ms | < 200 ms |

8.691 istek · hata **%0,00** · plato **99,2 istek/sn**

`/me` üçünün en yavaşı (p95 8,87 ms): kullanıcı + izin (RBAC) birleşimi
okuyor. Hedefin 22 katı altında; şu an bir sorun değil.

### 3.3 Fiyat teklifi — `GET /catalog/quote`

| p50 | p95 | p99 | maks | hedef |
|---|---|---|---|---|
| 13,31 ms | **21,21 ms** | 25,59 ms | 42,61 ms | < 1500 ms |

3.596 istek · hata **%0,00** · plato **41,1 istek/sn** · 429 (hız limiti): **0**

> Bu uç okuma uçlarından ~4 kat yavaş, çünkü kur çevrimi + fiyat kuralı +
> teklif satırı yazma işini yapıyor. Yine de hedefin **70 katı** altında.
> **Dikkat:** FAKE sağlayıcı bellek içidir. Gerçek sağlayıcıda buraya bir
> HTTP çağrısı eklenir ve asıl gecikmeyi o belirler — bu ölçüm sağlayıcı
> gecikmesini **içermez** (§7).

### 3.4 Satın alma — `POST /orders`

| Uç | p50 | p95 | p99 | maks | hedef |
|---|---|---|---|---|---|
| `GET /catalog/quote` | 12,50 ms | **19,42 ms** | 22,68 ms | 26,18 ms | < 1500 ms |
| `POST /orders` | 10,83 ms | **16,12 ms** | 19,09 ms | 24,49 ms | < 3000 ms |

746 döngü · **269 başarılı sipariş** · plato 16,5 istek/sn
**hata oranı %31,97 — eşik düştü.**

Hata dağılımı (ham CSV'den, `load/sonuclar/ham-satinalma.csv`):

| Durum | Adet | Sebep |
|---|---|---|
| `200` | 269 | başarılı |
| `503 PROVIDER_UNAVAILABLE` | 477 | **FAKE sağlayıcı bakiyesi tükendi** |
| `500` | **0** | — |

477 hatanın tamamı tek bir sebeptendir ve sunucu log'unda aynen görünür:

```
cause="provider: sağlayıcı bakiyesi yetersiz"   ×477
code=PROVIDER_UNAVAILABLE                        ×477
```

Yani **gecikme hedefi tuttu, bütçe tükendi**. Ayrıntı ve neden §4.4'te.

### 3.5 SSE — eş zamanlı açık akış

Bu senaryo gecikme değil **kapasite** ölçer: 50 akış 45 saniye boyunca açık
tutuldu.

| | |
|---|---|
| Eş zamanlı akış | 50 |
| Tutulan süre | p50 45,008 sn · maks 45,011 sn |
| `sse_hata` (akış hiç kurulamadı) | **0** |
| Kopan / düşen akış | **0** |

Elli akışın hepsi 45 saniyenin tamamı boyunca açık kaldı. Kaynak maliyeti
§5.2'de.

---

## 4. Defter mutabakatı — koşumun asıl sınavı

> Yük altında bozulan bir defter, yavaş bir sistemden **çok daha ciddi** bir
> sorundur. Bu yüzden mutabakat bir bilgi satırı değil, koşumun geçme
> koşuludur (`scripts/yuk-testi.sh` → `mutabakat`).

### 4.1 Sonuç

```
─── mutabakat (başlangıç) ───
    INFO mutabakat tamam drift_count=0
─── mutabakat (koşum sonrası) ───
    INFO mutabakat tamam drift_count=0
```

`ListReconciliationDrift` **her kullanıcı için** Σ `ledger_entries` ile
`users.balance_minor` değerini karşılaştırır. 86 kullanıcının hiçbirinde
sapma yok.

### 4.2 Koşumun defterde ürettiği hareket

| Kayıt tipi | Adet | Toplam (kuruş) |
|---|---|---|
| `PURCHASE` | 1.431 | −1.164.484 |
| `REFUND` | 1.161 | +888.898 |

Aritmetik **tam kapanıyor** ve telafi yolunun doğru çalıştığını kanıtlıyor:

```
1.431 satın alma denemesi
  = 685 (sıra ilerletme, §6.2)  +  746 (satinalma senaryosu döngüsü)

1.161 iade
  = 684 (ilerletmede yazılamayan sipariş)  +  477 (bakiye yetersiz)

1.431 − 1.161 = 270 kalıcı borç
  = 269 başarılı sipariş  +  1 (ilerletmenin son, başarılı satın alması)
```

**Başarısız her satın alma denemesi tam olarak bir telafi iadesi üretti.**
Yetim provizyon (`orphan hold`) sayısı sıfır; `api.log` içinde
"iade yazılamadı" satırı yok.

### 4.3 Bu ne kanıtlar, ne kanıtlamaz

**Kanıtlar:** 50 eşzamanlı kullanıcı, 1.431 para hareketi ve iki farklı hata
yolu (DB yazma hatası + sağlayıcı hatası) altında provizyon/telafi deseni
tutarlı kaldı. `SELECT … FOR UPDATE` + tek transaction sınırı, bu yükte
yarış durumu üretmedi.

**Kanıtlamaz:** Mutabakat **iç tutarlılığı** ölçer (defter == bakiye).
Kullanıcıdan *yanlış tutar* tahsil edilseydi ve aynı yanlış tutar deftere de
yazılsaydı, mutabakat yine temiz görünürdü. Fiyat doğruluğu ayrı bir sınavdır
(altın fiyat testleri, `domain/pricing`).

### 4.4 Neden bütçe tükendi

FAKE sağlayıcı **100 USD** ile başlar (`adapter/provider/fake/fake.go`,
`BalanceMicro: 100_000_000`) ve bu değer koda gömülüdür; ortam değişkeniyle
büyütülemez. Senaryonun kullandığı beş kombinasyonun ortalama maliyeti
~0,150 USD → **süreç ömrü boyunca ~660 satın alma**.

Bu koşumda bütçenin **%60'ı ölçümden önce** §6.2'deki sıra ilerletme adımına
gitti (684 × 0,088 USD = 60,2 USD). Kalan ~39,8 USD, 269 siparişten sonra
bitti ve geri kalan 477 istek 503 aldı.

Yani **hata oranı sistemin bir özelliği değil, düzeneğin bir sınırıdır** —
ama düzeneğin bu sınıra girmesinin sebebi gerçek bir kusurdur (§6.2).

---

## 5. Kaynak kullanımı ve darboğaz

`load/olcum.sh` koşum boyunca 2 saniyede bir örnekledi
(`load/sonuclar/kaynak.csv`). Tepe değerler:

| Senaryo | goroutine | RSS | açık fd | pg bağlantı¹ | pg aktif | idle in tx | redis istemci | pubsub kanal |
|---|---|---|---|---|---|---|---|---|
| katalog | 72 | 34,5 MB | 68 | 12 (→ **6**) | 0 | 0 | 6 | 1 |
| panel | 72 | 37,5 MB | 71 | 12 (→ **6**) | 0 | 0 | 6 | 0 |
| teklif | 77 | 39,3 MB | 77 | 14 (→ **8**) | 1 | 0 | 10 | 0 |
| satinalma | 75 | 41,2 MB | 79 | 14 (→ **8**) | 1 | 1 | 10 | 0 |
| **sse** | **262** | 41,5 MB | **151** | 26 (→ **20**) | 6 | 0 | **72** | 50 |

> ¹ `pg_stat_activity`'deki ham sayı; parantez içi, boşta duran `make dev`
> sunucusunun 6 bağlantısı düşülmüş hâli (§2). Bu 6'nın sabit olduğu koşumdan
> sonra üç kez örneklenerek doğrulandı.

### 5.1 Veritabanı havuzu

Havuz `api/internal/adapter/postgres/pool.go` içinde **`MaxConns = 20`** ile
sabittir (ortam değişkeni yoktur).

- **İstek/yanıt senaryolarında havuz rahat:** 50 eşzamanlı kullanıcıda en
  fazla 8 bağlantı kullanıldı — **tavanın %40'ı**. Tükenme yok.
- **SSE senaryosunda havuz tavana dayandı:** toplam 26 bağlantının 6'sı
  `make dev`'e ait → ölçüm sunucusu **20/20**. (Çıkarım `make dev`'in tam
  olarak 6 tutmasına dayanır; 7 tutuyorsa 19'dur. Her hâlükârda tavanda ya
  da bir eksiğinde.) Sebep, `constant-vus` ile 50 akışın **aynı anda**
  açılması ve her birinin abone olmadan önce sahiplik doğrulaması
  (`h.svc.Get`) için bir sorgu çalıştırmasıdır (`handler/order.go` →
  `Stream`).
- 200 akışlık ek koşumda da aynı tablo: toplam 27 → ölçüm sunucusu ~20.
  Havuz **akış sayısıyla büyümüyor**; darboğaz akışların kendisi değil,
  açılış anındaki sorgu patlaması.
- Bu ani yükte bile **hata alınmadı** (`sse_hata=0`): pgxpool istekleri
  kuyruğa aldı, reddetmedi. Ama tavan görüldü — 50 akış, tavanın tamamını
  kullanan ilk yüktür.
- `idle in transaction` tepe değeri **1**. Değişmez #5 (transaction içinde
  HTTP çağrısı yok) yük altında da tutuyor; açık kalmış transaction yok.

### 5.2 SSE'nin bağlantı başına maliyeti

İki ölçüm noktası. İkincisi ölçeklemeyi görebilmek için ayrıca koşuldu
(`SSE_VU=200 ./scripts/yuk-testi.sh sse`, 09:34–09:36):

| Akış | goroutine | açık fd | redis istemci | pubsub kanal | RSS tepe | kaynak izi |
|---|---|---|---|---|---|---|
| 50 | 262 | 151 | 72 | 50 | 41,5 MB | `kaynak.csv` |
| 200 | 1.012 | 451 | 222 | 50 | 65,7 MB | `kaynak-sse200.csv` |

İki koşumda da `sse_hata = 0`: 200 akışın hepsi 45 saniyenin tamamı boyunca
açık kaldı, hiçbiri düşmedi.

Eğim (150 akışlık fark):

| Kaynak | Akış başına |
|---|---|
| goroutine | **+5,0** |
| açık dosya tanıtıcısı | **+2,0** (1 istemci soketi + 1 Redis soketi) |
| **Redis bağlantısı** | **+1,0** |
| RSS | ~+0,15 MB |

**Akış başına bir Redis bağlantısı**, `adapter/redis/orderbus.go` içindeki
`b.client.Subscribe(...)` çağrısının doğrudan sonucudur: go-redis her
abonelik için ayrılmış bir bağlantı açar. Kanal sayısı akış sayısıyla değil,
**sipariş** sayısıyla artar (200 akış / 50 sipariş → 50 kanal); bağlantı
sayısı ise akışla birebir artar.

Ayrıca doğrulandı: **aynı siparişe** açılan 5 akış → Redis istemcisi 3'ten
12'ye çıktı, pubsub kanalı 1'de kaldı. Yani bir kullanıcının aynı siparişi
beş sekmede açması beş Redis bağlantısı tüketir.

**Tavan:** Redis `maxclients = 10000` (ölçüldü). Eğimi uygularsak tek
Redis örneğiyle **~9.900 eş zamanlı akış** üst sınırdır. İşletim sistemi fd
sınırı (`ulimit -n = 1048576`) bu ölçekte darboğaz değildir; **Redis
bağlantı sayısı darboğazdır.**

### 5.3 Bellek ve goroutine

İstek/yanıt yükünde RSS 34–41 MB, goroutine 72–77 arasında kaldı; 50 VU'da
kayda değer bir artış yok. Sızıntı belirtisi görülmedi (RSS senaryolar
boyunca düz seyretti, koşum sonunda yükselmiş kalmadı).

### 5.4 Sağlayıcı hız limiti

FAKE sağlayıcıda hız limiti yoktur; bu koşum **sağlayıcı hız limitini
ölçmez**. Gerçek sağlayıcı (HeroSMS) için `docs/provider-herosms.md`
§"Hız limiti" iki ayrı mekanizma tanımlar:

- **`429 RATE_LIMIT` + `Retry-After`** — yalnız
  `GET /activations/offers/{verificationType}` ucunda tanımlı, **sayısal
  değeri belgelenmemiş**. Diğer 46 uçta 429 yok (açık soru **H4**).
- **`403 CHANNELS_LIMIT`** — istek/sn değil, **eş zamanlı aktivasyon**
  sınırı; yanıt `info: {current_threads, max_allowed}` taşır ve
  **semaforla** karşılanması gerekir (**FR-413**).

Yani gerçek sağlayıcıda tavanı belirleyen şey istek/sn değil,
**aynı anda açık aktivasyon sayısıdır** ve bu sayı sözleşmede sabit değil,
sağlayıcı yanıtından okunur. Bu koşum o yolu hiç çalıştırmadı.

---

## 6. Bulgular

> **Güncelleme (2026-09-09, koşumdan hemen sonra):** §6.1 ve §6.2 DÜZELTİLDİ
> ve ikisi de sabotajla doğrulanmış regresyon testine bağlandı
> (`order_integration_test.go#TestDuplicateRemoteIDIsNotInternalError`,
> `fake_test.go#TestRemoteOrderIDIsUniquePerProcess`). §6.3 ve §6.5
> [TESLIM.md](TESLIM.md) "Bilinen eksikler" tablosuna taşındı. §6.4, §6.6, §6.7
> açık; aşağıda oldukları gibi duruyor.

### 6.1 🔴 `orders_remote_uniq` ihlali kullanıcıya `500` olarak dönüyor

`Service.Create` → `persist` adımında oluşan veritabanı hatası
(`api/internal/service/order/service.go:187`) **tipli hataya çevrilmeden**
yukarı verilir. `transport/http/response.go` bunu tanıyamaz, `apperr.Internal`
ile sarar ve **HTTP 500 / `code=INTERNAL`** yazar.

Bu koşumda 684 kez gözlendi:

```
cause="ERROR: duplicate key value violates unique constraint
       \"orders_remote_uniq\" (SQLSTATE 23505)"
code=INTERNAL  status=500  path=/api/v1/orders
```

**Neden önemli:** NFR-801 "sağlayıcı çökmesi kullanıcıya 500 olarak
yansımaz" diyor. Buradaki kaynak sağlayıcı değil veritabanı, ama sonuç aynı:
öngörülebilir bir çakışma, "sunucu hatası" olarak raporlanıyor. 500'ler
Sentry'de gerçek arızalarla aynı kovaya düşer ve alarm gürültüsü yaratır.
Kullanıcı **doğru şekilde** iade alıyor (§4.2) — kayıp para yok — ama durum
kodu ve gözlemlenebilirlik yanlış.

**Öneri:** `persist` içindeki `23505`'i yakalayıp tipli bir hataya çevirin
(çakışma yeniden denenebilir bir durumdur; sağlayıcıdaki numara zaten
alınmıştır). En azından `409`/`503` sınıfına düşmeli, `500`'e değil.

### 6.2 🔴 FAKE sağlayıcının sıra sayacı süreç içi — tekrarlı yük testini kilitliyor

`adapter/provider/fake/operations.go` uzak sipariş kimliğini
`fmt.Sprintf("fake-%d", p.seq)` ile üretir ve `p.seq` **her süreç açılışında
0'a döner**. Kalıcı geliştirme veritabanında önceki koşumların satırları
durduğu için ikinci koşum ilk istekten itibaren §6.1'deki çakışmayı alır.

Ölçülen etki: koşumdan önce veritabanında en büyük kimlik **fake-684** idi;
`POST /orders` **684 kez** üst üste 500 döndü ve satın alma senaryosu
ölçülemez hâle geldi.

**Geçici çözüm uygulandı** (`scripts/yuk-testi.sh` → `sira_ilerlet`):
satın alma senaryosundan önce, sayaç veritabanındaki en büyük değerin üstüne
çıkana kadar en ucuz kombinasyonla satın alma denenir. Bu koşumda **685
deneme** sürdü.

**Bu çözüm sürdürülebilir değil** ve raporda böyle durmalıdır:

- İlerletmenin bedeli **~60,3 USD** oldu (685 × 0,088) — FAKE bütçesinin
  %60'ı. Sağlayıcı çağrısı **başarılı olduğu** için bakiye her denemede
  düştü; yazma hatası ondan sonra geldi. Ölçüm penceresi bu yüzden 269
  siparişte bitti (§4.4).
- Her koşum veritabanındaki en büyük kimliği **büyütür**. Bu koşumdan sonra
  954 sipariş var; bir sonraki ilerletme 954 × 0,088 = 84 USD tutacak ve
  100 USD'lik bütçeye **sığmayacak**. Yani **bu düzenek bir koşum daha
  dayanır.**

**Kalıcı çözüm (bir satır, bu görevin kapsamı dışında):** FAKE sağlayıcı
kimliğe süreç başına benzersiz bir ek koysun — örn.
`fake-<açılış-nonce>-<seq>`. `api/internal/adapter/provider/fake/operations.go:105`
(`Purchase` içindeki `id := fmt.Sprintf("fake-%d", p.seq)` satırı).

### 6.3 🟠 Yazılamayan sipariş, sağlayıcıda **açık numara** bırakıyor

`persist` düştüğünde `abandonRemote` numarayı kapatmaya çalışır, ama
sağlayıcı ilk 120 saniye içinde iptali kabul etmez (HeroSMS
`info.minActivationTime`; FAKE aynı kısıtı taklit eder). Sonuç:

```
msg="sipariş yazılamadı ve sağlayıcıdaki numara kapatılamadı — ELLE BAKILMALI"
     remote_order_id=fake-N
     err="provider: asgari bekleme süresi dolmadı — 1m59.99s sonra tekrar deneyin"
```

Bu koşumda **684 kez**. Log doğru şeyi söylüyor ama **kimse toplamıyor**:
sipariş satırı hiç yazılmadığı için `orders_refund_retry_idx` üzerinden koşan
iade yeniden deneme işi bu numaraları göremez. Kullanıcının parası iade
edildi (§4.2), ama **sağlayıcıdaki numara TTL'i (FAKE'te 20 dakika) dolana
kadar açık kalır** ve `Cancel` bir daha hiç denenmez.

FAKE'te bakiye hiç geri gelmez. Gerçek sağlayıcıda süresi dolan bir
aktivasyonun otomatik iade edilip edilmediği `docs/provider-herosms.md`'de
**doğrulanmamıştır** — edilmiyorsa bu doğrudan para kaybıdır, ediliyorsa bile
numara ve envanter boşuna bloke edilmiş olur. Geliştirmede FAKE bakiyesi
önemsizdir; **üretimde bu gerçek paradır.**

**Öneri:** Sağlayıcıda alınmış ama yerelde yazılamamış numaralar için kalıcı
bir "yetim uzak sipariş" kaydı (veya gecikmeli bir kuyruk işi) gerekir; 120
saniye sonra `Cancel` tekrar denenmelidir.

### 6.4 🟡 `/orders` hız limiti belge ile uyuşmuyor

| Kaynak | Değer |
|---|---|
| `docs/trd.md` NFR-802 | `/orders` **10 istek/dk/kullanıcı** |
| `api/internal/transport/http/router.go:255` | `Limit: 20, Window: time.Minute` |

Kod, belgenin **iki katı** izin veriyor. Bu, her isteği gerçek para harcayan
bir uçtur; hangisinin doğru olduğu bir karar meselesidir, ama ikisinin farklı
olması bir kusurdur. (`/catalog/quote` 60/dk ve `/auth/*` 30/dk/IP değerleri
belgeyle **uyuşuyor**.)

### 6.5 🟡 SSE akış başına bir Redis bağlantısı tüketiyor, üst sınır yok

§5.2'deki ölçüm: akış başına 1,0 Redis bağlantısı, 5,0 goroutine, 2 fd.
Ne kullanıcı başına ne de sunucu genelinde eş zamanlı akış sınırı var
(`handler/order.go` → `Stream`). Sahiplik kontrolü kullanıcıyı **kendi**
siparişleriyle sınırlar, ama **aynı siparişe kaç akış açabileceğini
sınırlamaz** — ölçüldü: 5 sekme = 5 Redis bağlantısı, 1 kanal.

Bugünkü kullanıcı sayısında sorun değil. Redis `maxclients=10000` ile
~9.900 akışlık bir tavan var ve bu tavan **tüm uygulama için** ortaktır:
akışlar dolduğunda sıradan Redis işlemleri (oturum okuma!) de bağlantı
bulamaz.

**Öneri:** (a) kullanıcı başına eş zamanlı akış sınırı, (b) tek bir paylaşılan
abonelik üzerinden çoğullama (aynı `order_id` için tek Redis aboneliği, N
istemci), veya (c) `sse_active_connections` metriğine alarm — bu metrik
`design.md` §12'de zaten tanımlı.

### 6.6 🟡 `provider_offers` üzerinde tam tarama; tablo şişkin

Katalog ızgarasının sorgusu (`ListServicesWithStock`) için `EXPLAIN ANALYZE`
§8'de. Özet: `provider_offers` üzerinde **Seq Scan**, **338 blok okuma**,
**12 canlı satır**.

Tablo 12 satır için **2.704 kB / 338 sayfa** yer kaplıyor (indeksler ayrıca
2.136 kB). Mevcut kısmi indeks `provider_offers_product_idx (product_id,
cost_micro) WHERE is_available`, sorgunun `is_available AND stock > 0`
süzgecine `product_id` olmadan hizmet edemez.

Bugün maliyeti 1,7 ms; önemsiz. Ama sorgunun kendi yorumu üretimde
**9.768 stoklu servis×ülke kombinasyonu** olduğunu söylüyor — bu tarama o
ölçekte katalog sayfasının maliyet merkezi olur.

**Öneri:** (a) `provider_offers` için `VACUUM FULL` (dev'de şişkinliği
temizler), (b) üretim ölçeğinde `(is_available, stock)` üzerinde kısmi bir
indeks ölçün, (c) katalog özetini önbelleğe alın — bu sorgu oturumsuz
trafiğin tamamının gördüğü sorgudur.

### 6.7 🟡 Yük testi kapıları birleştirme kapısında koşmuyor

`scripts/yuk-testi_test.sh` dört kapıyı doğruluyor (dev dışı ortam reddi,
mutabakat sapması, karşı sınav, yıkıcı SQL yokluğu) ama `scripts/check.sh`
onu **çağırmıyor**. Kapının kendisi denetlenmiyorsa kapı değildir.

`scripts/check.sh` bu görevin dosya listesinde değil — **değişiklik
bildiriliyor, yapılmıyor.** Eklenecek satır:

```bash
step "yük testi kapıları"   ./scripts/yuk-testi_test.sh
```

---

## 7. Ne ölçülMEDİ

Raporun en önemli bölümü budur; buradaki hiçbir şey için "muhtemelen yeterli"
denemez.

1. **Sistemin doygunluk noktası.** Senaryolar gerçek kullanıcı gibi
   *düşünme süresi* içerir (0,5–6 sn); istek hızını sistemin kapasitesi
   değil, bu bekleme belirledi. 50 VU'da sunucu 40–99 istek/sn gördü ve
   veritabanı havuzunun %40'ını kullandı. **Sistemin kırılma noktası
   bulunamadı** — yalnız "50 eşzamanlı kullanıcıda hedefler tutuyor"
   gösterildi (KK-800'ün istediği tam olarak budur).
   ⚠️ **CPU hiç ölçülmedi:** `load/olcum.sh` goroutine, RSS, fd, Postgres
   ve Redis sayar; işlemci kullanımı örneklenmiyor. "Yük hafifti" demek
   için bir dayanağımız yok, yalnız "hedefler tuttu" diyebiliriz.
2. **Gerçek sağlayıcı gecikmesi.** Tüm ölçüm FAKE (bellek içi) sağlayıcıyla
   yapıldı. `POST /orders` p95'i 16 ms; gerçek sağlayıcıda buraya bir HTTP
   çağrısı eklenir ve **hedefi belirleyen o olur**. 3000 ms bütçesinin
   ~16 ms'i bizim, gerisi sağlayıcınındır.
3. **Panel sayfa yükleme (TTFB) < 400 ms.** Bu hedef Next.js sunucu
   bileşenlerine aittir; ölçüm yalnız Go API'sini kapsadı. Web tarafı
   ölçülmedi.
4. **Sağlayıcı hız/eşzamanlılık limiti.** §5.4 — FAKE'te yok, gerçekte
   `403 CHANNELS_LIMIT` semaforu (FR-413) hiç sınanmadı.
5. **Uzun süreli dayanıklılık (soak).** En uzun koşum 2,5 dakikadır. Bellek
   sızıntısı, bağlantı sızıntısı ve `price_quotes` birikmesi ancak saatlik
   bir koşumda görünür.
6. **Ağ gecikmesi, TLS, ters vekil.** k6 ve sunucu aynı makinede, düz HTTP.
   Caddy (`flush_interval -1`) ve HTTP/2 zorunluluğu (NFR-811) yolun dışında
   kaldı — **SSE'nin gerçek dağıtımdaki davranışı bu koşumla doğrulanmadı.**
7. **Postgres/Redis çökme davranışı** (NFR-801). Kaos senaryosu koşulmadı.

---

## 8. `EXPLAIN ANALYZE` — en pahalı okuma sorgusu

Aday seçimi: oturumsuz trafiğin tamamının gördüğü ve en çok tabloyu
birleştiren sorgu, `ListServicesWithStock` (`api/queries/catalog.sql:172`).

```
->  Seq Scan on provider_offers o
      (cost=0.00..338.15 rows=10 width=24)
      (actual time=0.017..1.738 rows=11 loops=1)
    Filter: (is_available AND (stock > 0))
    Rows Removed by Filter: 1
    Buffers: shared hit=338
->  Bitmap Heap Scan on products p   (actual time=0.001..0.001 rows=1 loops=11)
      Recheck Cond: (o.product_id = id)
      ->  Bitmap Index Scan on products_pkey
->  Seq Scan on services s   (actual time=0.002..0.048 rows=6 loops=1)
->  Seq Scan on countries c  (actual time=0.013..0.013 rows=5 loops=1)

Planning Time: 3.318 ms
Execution Time: 2.080 ms
```

Okunuşu:

- **Yürütmenin %84'ü tek bir tam taramada** (1,738 ms / 2,080 ms):
  `provider_offers` üzerinde 338 blok okunuyor, geriye 11 satır kalıyor.
- `services` ve `countries` taramaları önemsiz (6 ve 5 satır).
- **Planlama, yürütmeden uzun** (3,3 ms > 2,1 ms). Uygulamada pgx hazırlanmış
  ifade kullandığı için bu maliyet amortize olur; `psql` ile ölçüldüğünde
  görünür. Bu yüzden uçtan uca p95 (3,86 ms) bu sayılara yakın çıkıyor.
- Şişkinlik ölçüldü: `pg_relation_size = 2.704 kB`, **338 sayfa**,
  `n_live_tup = 12`, `n_dead_tup = 0`, `last_autovacuum` **hiç çalışmamış**.
  Yani sayfalar canlı satırla değil, boşlukla dolu.

Karşılaştırma — panelin sipariş listesi sağlıklı (indeks kullanıyor):

```
->  Bitmap Index Scan on orders_user_idx  (actual time=0.070..0.070 rows=32)
      Index Cond: (user_id = 361)
Execution Time: 0.471 ms
```

---

## 9. Sıradaki adımlar

| # | İş | Öncelik | Bulgu |
|---|---|---|---|
| 1 | FAKE sağlayıcı kimliğine süreç nonce'u ekle | 🔴 | §6.2 |
| 2 | `persist` içindeki `23505`'i tipli hataya çevir (500 olmasın) | 🔴 | §6.1 |
| 3 | Yazılamayan siparişin uzak numarasını 120 sn sonra kapatacak kalıcı kayıt/kuyruk | 🟠 | §6.3 |
| 4 | `/orders` hız limitini belge ile hizala (10 mi 20 mi?) | 🟡 | §6.4 |
| 5 | Kullanıcı başına eş zamanlı SSE sınırı **veya** abonelik çoğullama | 🟡 | §6.5 |
| 6 | `provider_offers` şişkinliği + üretim ölçeğinde indeks ölçümü | 🟡 | §6.6 |
| 7 | `scripts/check.sh`'a `yuk-testi_test.sh` adımını ekle | 🟡 | §6.7 |
| 8 | Gerçek sağlayıcıya karşı staging'de teklif/satın alma ölçümü | — | §7.2 |
| 9 | Caddy + HTTP/2 arkasında SSE ölçümü (gerçek cihazda) | — | §7.6 |

---

## 10. Tekrar üretmek için

```bash
YONETICI_PAROLA='…' ./scripts/yuk-testi.sh          # tohum + 5 senaryo + mutabakat
./scripts/yuk-testi.sh kos                          # mevcut tohumla yalnız koşum
./scripts/yuk-testi.sh satinalma                    # tek senaryo
./scripts/yuk-testi.sh mutabakat                    # yalnız defter kapısı
```

Ayrıntı, ayarlar ve çıktı dosyalarının anlamı: [`load/README.md`](../load/README.md).

**Uyarı:** §6.2'deki sıra ilerletme yüzünden `satinalma` senaryosu bu
düzenekte **bir koşum daha** dayanır. Öncesinde 1. maddedeki düzeltme
yapılmalıdır.
