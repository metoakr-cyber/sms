# Yük testi (k6)

Bu dizin, NFR-800 hedeflerine karşı **ölçüm** yapar. Amaç "sistem hızlı mı" sorusundan
önce şu ikisini yanıtlamaktır:

1. **Yük altında defter bozuluyor mu?** (Σ `ledger_entries` == `users.balance_minor`)
2. **Nerede tıkanıyoruz?** — hangi uç, hangi kaynak (veritabanı havuzu, Redis, goroutine).

Ölçülen hedefler `docs/trd.md` → **NFR-800**'den gelir, burada tekrar yazılmaz.
Sonuçlar: [`docs/yuk-testi-sonuc.md`](../docs/yuk-testi-sonuc.md).

---

## Nasıl koşulur

```bash
# gereksinimler: k6, docker (postgres+redis ayakta), jq
brew install k6

# tam koşum — tohum + 5 senaryo + mutabakat kapısı
YONETICI_PAROLA='…' ./scripts/yuk-testi.sh

# yalnız tohum (kullanıcı + bakiye + SSE için açık sipariş)
YONETICI_PAROLA='…' ./scripts/yuk-testi.sh tohum

# tek senaryo (tohum hazırsa)
./scripts/yuk-testi.sh teklif

# yalnız defter mutabakatı
./scripts/yuk-testi.sh mutabakat
```

`YONETICI_PAROLA` **ortam değişkenidir**; betikte, README'de ve rapor dosyasında
yazılı değildir (Değişmez #11).

### Ayarlar

| Değişken | Varsayılan | Ne işe yarar |
|---|---|---|
| `YUK_PORT` | `8191` | Ölçüm için başlatılan API örneğinin portu |
| `KULLANICI_SAYISI` | `50` | Tohumda üretilecek test kullanıcısı sayısı |
| `BAKIYE_KURUS` | `100000` | Kullanıcı başına yüklenecek bakiye (1.000,00 TL) |
| `SENARYOLAR` | `katalog panel teklif satinalma sse` | Koşulacak senaryolar |
| `SSE_VU` / `SSE_SURE` | `50` / `45` | Aynı anda açık akış sayısı ve tutma süresi |
| `SATINALMA_BEKLEME` | `6` | Satın alma senaryosunda döngü arası bekleme (sn) |
| `YUK_RECONCILE_CMD` | `go run ./cmd/cli wallet:reconcile` | Mutabakat komutu (derlenmiş ikili verilebilir) |

---

## Senaryolar

| # | Ad | Ne yapar | NFR-800 hedefi |
|---|---|---|---|
| 1 | `katalog` | Oturumsuz: `/catalog/services-in-stock`, `/catalog/countries` | p95 < 200 ms |
| 2 | `panel` | Oturumlu: `/me`, `/wallet/balance`, `/orders` | p95 < 200 ms |
| 3 | `teklif` | `GET /catalog/quote` — sağlayıcıya canlı fiyat/stok sorar | p95 < 1500 ms |
| 4 | `satinalma` | teklif + `POST /orders` — para hareketi + sağlayıcı çağrısı | p95 < 3000 ms |
| 5 | `sse` | `GET /orders/:id/stream` — N akış aynı anda açık tutulur | kapasite ölçümü |

Yük profili **kademelidir**: 0 → 10 → 50 kullanıcı (`load/ortak.js` → `KADEMELI`).
Ani yük yerine kademe, her seviyede sistemin durulmasına zaman tanır; ilk saniyelerdeki
bağlantı kurulumu p99'u yanıltmasın diye koşumdan önce 200 istekle **ısınma** yapılır.

Senaryolar **ayrı k6 koşumları** hâlinde çalışır. Tek koşumda karıştırmak, uç nokta
bazlı p95 hedefleriyle karşılaştırmayı imkânsız kılar: yavaş bir uç, hızlı olanın
örneklerini seyrelterek toplam p95'i olduğundan iyi gösterir.

---

## Çıktılar (`load/sonuclar/`)

| Dosya | İçerik |
|---|---|
| `ozet-<senaryo>.json` | k6 özet metrikleri (p50/p90/p95/p99, hata oranı, istek/sn) |
| `ham-<senaryo>.csv` | k6 ham örnekleri |
| `k6-<senaryo>.log` | k6 konsol çıktısı (eşik sonuçları dahil) |
| `kaynak.csv` | 2 sn'de bir: goroutine, RSS, açık fd, `pg_stat_activity`, Redis istemci/kanal |
| `kaynak-onceki.csv` | Bir önceki koşumun kaynak izi (her koşumda döndürülür) |
| `api.log` | Ölçüm için başlatılan API örneğinin log'u — **her koşumda sıfırlanır** |
| `kullanicilar.json` | Tohum kullanıcıları ve oturum çerezleri |
| `siparisler.json` | SSE için açık bırakılmış sipariş kimlikleri |

`kaynak.csv` olmadan k6 çıktısı yalnız "ne kadar sürdü" der, "neden" demez.
Darboğaz teşhisi bu dosyadan çıkar.

> ⚠️ **Koşum çıktısını saklamak isterseniz kendiniz yönlendirin.** Betik her
> koşumda `api.log`'u sıfırlar ve `kaynak.csv`'yi `kaynak-onceki.csv` üzerine
> döndürür; iki koşum, ikinci koşumun kanıtını götürür.
> `./scripts/yuk-testi.sh kos 2>&1 | tee load/sonuclar/kosum-transkript.log`
> ile transkripti ayrıca kaydedin. Bu dizin `.gitignore`'dadır; kalıcı kayıt
> `docs/yuk-testi-sonuc.md` içindedir.

---

## Sonuç nasıl okunur

### Çıkış kodu — önce buna bakın

| Kod | Anlamı | Ne yapmalı |
|---|---|---|
| `0` | Tüm eşikler tuttu, defter mutabık | — |
| `1` | Bir senaryo NFR-800 eşiğini tutmadı | `k6-<senaryo>.log` → `THRESHOLDS` bloğu |
| `2` | **Defter bozuldu (Σ defter ≠ bakiye)** | 🔴 Gecikmeden önce bu incelenir |

Çıkış kodu `2`, `1`'den **daha ciddidir**. Yavaş bir sistem düzeltilir; sapmış
bir defter para kaybıdır.

### 1. Eşikler — `k6-<senaryo>.log`

Dosyanın sonundaki `█ THRESHOLDS` bloğu her hedefi ✓/✗ ile gösterir. Hedefler
`load/senaryo.js` → `esikler` içinde tanımlıdır ve **`docs/trd.md` NFR-800'den
gelir**; orada değişirse burada da değişmelidir.

### 2. Hata varsa — hangi durum kodu?

Toplam hata oranı bir şey anlatmaz; **dağılımı** anlatır:

```bash
awk -F, 'NR>1 && $1=="http_req_duration" {print $10, $14}' \
  load/sonuclar/ham-satinalma.csv | sed 's|?.*||' | sort | uniq -c | sort -rn
```

| Durum | Genellikle | Bakılacak yer |
|---|---|---|
| `429` | Hız limiti — VU sayısı tohum kullanıcı sayısını aşmış | `KULLANICI_SAYISI`, bekleme süreleri |
| `503` | Sağlayıcı: bakiye/stok tükendi | `api.log` → `code=PROVIDER_UNAVAILABLE` |
| `500` | 🔴 **Eşlenmemiş hata** — her zaman bir kusurdur | `api.log` → `cause=` alanı |
| `401` | Tohum oturumları ölmüş (Redis sıfırlanmış) | `./scripts/yuk-testi.sh tohum` |

Sunucu tarafındaki gerçek sebep her zaman `api.log` içindedir:

```bash
grep -o 'cause="[^"]*"' load/sonuclar/api.log | sort | uniq -c | sort -rn
```

### 3. Gecikme — plato penceresini kullanın

k6'nın özet p95'i **kademenin tamamının** ortalamasıdır ve düşük yüklü ilk
dakikayı da içerir; hedefe göre iyimserdir. Gerçek "50 eşzamanlı kullanıcı"
sayısı için koşumun **80.–140. saniyesini** süzün (bkz.
`docs/yuk-testi-sonuc.md` §3).

### 4. Darboğaz — `kaynak.csv`

```bash
awk -F, 'NR>1 {e=$2; if($3+0>g[e])g[e]=$3+0; if($6+0>p[e])p[e]=$6+0;
               if($9+0>r[e])r[e]=$9+0}
  END{for(e in g) printf "%-10s goroutine=%d pg=%d redis=%d\n", e, g[e], p[e], r[e]}' \
  load/sonuclar/kaynak.csv
```

| Sütun | Neye bakılır |
|---|---|
| `pg_toplam` | Havuz tavanı **20**'dir (`postgres/pool.go`). Tavana dayanıyorsa kuyruk oluşuyor demektir. |
| `pg_idle_tx` | 0'dan büyük ve **kalıcıysa** 🔴: transaction içinde dış çağrı yapılıyor olabilir (Değişmez #5). |
| `redis_istemci` | SSE'de akış başına **~1** artar. Redis `maxclients` (10000) gerçek tavandır. |
| `goroutine` | SSE'de akış başına **~5** artar. İstek/yanıt yükünde düz kalmalıdır. |
| `rss_mb` | Senaryolar boyunca yükselip inmeli; koşum sonunda yüksek kalıyorsa sızıntı şüphesi. |

> `pg_toplam`, aynı veritabanına bağlı **tüm** süreçleri sayar. `make dev`
> ayaktaysa onun havuzu da (2–20 bağlantı) bu sayının içindedir; ölçümden
> önce kapatmak en temizidir.

### 5. Defter — her zaman son bakılan değil, ilk bakılan

```bash
./scripts/yuk-testi.sh mutabakat
```

`drift_count=0` beklenir. Sıfırdan farklıysa gecikme sayıları önemsizdir.

---

## Bilinmesi gerekenler

**Ayrı sunucu örneği.** Betik kendi API sürecini `YUK_PORT` (8191) üzerinde başlatır;
`make dev`'in 8091'ine dokunmaz. Sebep iki tane: aynı makinede iki yük birbirinin
gecikmesini ölçer, ve FAKE sağlayıcının stok/bakiyesi **süreç içidir** — paylaşılan
bir süreçte ölçüm tekrarlanabilir olmaz.

**FAKE sağlayıcının bütçesi sonludur.** `adapter/provider/fake` 100 USD ile başlar
ve bu değer **koda gömülüdür** (ortam değişkeni yok). `satinalma` senaryosunun beş
kombinasyonunun ortalama maliyeti ~0,150 USD → **süreç ömrü boyunca ~660 satın
alma**. Bunun üstünde `ErrProviderNoBalance` → HTTP `503` gelir ve ölçülen şey hata
yolu olur. Bütçe süreçle birlikte sıfırlanır; daha uzun bir koşum için API'yi
yeniden başlatın.

**🔴 Satın alma senaryosunun ön koşulu: FAKE sıra ilerletme.** FAKE sağlayıcı uzak
sipariş kimliğini süreç içi bir sayaçtan üretir (`fake-1`, `fake-2`, …) ve sayaç
her açılışta sıfırlanır. Kalıcı geliştirme veritabanında önceki koşumların satırları
durduğu için `orders_remote_uniq` çakışır ve `POST /orders` **500** döner.
`scripts/yuk-testi.sh` → `sira_ilerlet` bunu, sayacı veritabanındaki en büyük
kimliğin üstüne çıkararak aşar.

> Bu bir **geçici çözümdür ve bedeli vardır**: her ilerletme denemesi sağlayıcıdan
> gerçek bir numara alır ve FAKE bütçesinden düşer. Ölçüm penceresi bu kadar
> küçülür. Dahası ilerletmenin maliyeti her koşumda **büyür** — düzenek bir iki
> koşum sonra 100 USD'lik bütçeye sığmaz. Kalıcı çözüm FAKE sağlayıcıdadır ve
> `docs/yuk-testi-sonuc.md` §6.2'de tarif edilmiştir.

**Hız limitleri gerçek bir tavandır.** `/auth/*` **30 istek/dk/IP**,
`/catalog/quote` 60/dk/kullanıcı, `/orders` 20/dk/kullanıcı — bu son değer
`router.go`'dan okunmuştur; `docs/trd.md` NFR-802 **10** diyor (uyuşmazlık:
`docs/yuk-testi-sonuc.md` §6.4). Tohum
bunlara çarpmamak için yavaşlar — 50 kullanıcının oturumunu açmak tek başına
~2 dakika sürer. Bu bir yavaşlık değil, **ölçülen sistemin bir özelliğidir** ve
rapora böyle girer.

**Tohum kullanıcıları API ile kaydedilmez.** `/auth/*` limiti yüzünden kullanıcı
satırları SQL ile üretilir; parola hash'i, API ile kaydedilmiş bir *şablon*
kullanıcıdan kopyalanır (yani uygulamanın kendi ürettiği hash biçimi). **Bakiye
buna dahil değildir**: bakiye yalnız `POST /admin/users/:id/balance` üzerinden,
yani `WalletRepo.ApplyEntry` ve defter üzerinden verilir (Değişmez #2). Aksi hâlde
testin ölçmek istediği mutabakat en baştan anlamsız olurdu.

**SSE ölçümü gecikme değil kapasite ölçer.** k6'nın SSE modülü yok; uzun zaman
aşımlı bir `GET` aynı sunucu maliyetini üretir (bağlantı başına bir goroutine ve
bir **Redis aboneliği**). k6 bu isteği "zaman aşımı" sayar; başarı ölçütü
`sse_hata` sayacıdır, `http_req_failed` değil.

---

## Temizlik

Betikler **veri silmez** (`scripts/yuk-testi_test.sh#silme_ifadesi_yok` bunu her
koşuda doğrular). Test kullanıcıları `yuk-<koşum>-<n>@yuk.test` desenindedir ve
geliştirme veritabanında kalır. Silmek isterseniz — ve yalnız geliştirme
veritabanında — `ledger_entries` değişmez olduğu için sıra önemlidir; ilgili
kaçış kapısı `smoke-auth.sh` içinde tarif edilmiştir. Kalıcı bırakmak da
zararsızdır: mutabakat kapısı bu kullanıcıları da denetler.
