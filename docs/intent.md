# intent.md — Neden Var, Ne Yapıyor, Ne Yapmıyor

> **Doküman amacı:** "Ne inşa ediyoruz ve neden" sorusunun tek doğruluk kaynağı.
> Teknik bir tartışma çıkmaza girdiğinde buraya dönülür. Kod değişir, mimari değişir; intent nadiren değişir.
>
> **Durum:** v1 · **Son güncelleme:** 2026-09-08 · **Sahip:** ikmetrik

---

## 1. Tek cümle

**Kullanıcıların WhatsApp, Telegram, Instagram gibi servislerin SMS doğrulamasını, ön ödemeli TL bakiyesiyle, birden fazla numara sağlayıcısını soyutlayan tek bir Türkçe panel üzerinden tamamlayabildiği bir SaaS platformu.**

## 2. Çözülen problem

Sanal numara (SMS activation) piyasasında son kullanıcı için üç somut sıkıntı var:

1. **Fiyat dağınık.** Aynı ülke + aynı servis için sağlayıcılar arasında ciddi fiyat farkı olabiliyor ve fark saat içinde değişiyor. Kullanıcı elle karşılaştıramıyor.
2. **Stok oynak.** Bir sağlayıcıda "0 stok" olan bir kombinasyon diğerinde dolu olabiliyor. Tek sağlayıcıya bağlı kullanıcı hizmet alamıyor.
3. **Ödeme ve dil bariyeri.** Sağlayıcıların çoğu yabancı; dolar bazlı ödeme, İngilizce/Rusça arayüz, destek yok. Türkiye'deki kullanıcı için giriş engeli yüksek.

**Çözümümüz:** Kullanıcı tek bir Türkçe panelde TL bakiyesiyle çalışır. Arka planda uygun sağlayıcıları sorgular, en ucuz + stoklu olanı seçeriz. Kullanıcı hangi sağlayıcıdan aldığını bilmek zorunda değildir; bu bizim işimiz.

## 3. Kullanıcılar ve işleri (Jobs To Be Done)

### 3.1 Son kullanıcı (birincil)
Tipik profil: e-ticaret satıcısı, sosyal medya yöneticisi, yazılımcı, test ekibi.

| # | İş | Başarı ölçütü |
|---|---|---|
| JTBD-1 | "Bir WhatsApp hesabı açmak için numara lazım, 2 dakikada halletmeliyim." | Girişten koda kadar medyan süre < 90 sn |
| JTBD-2 | "Ne kadara mal olacağını satın almadan net görmek istiyorum." | Gösterilen fiyat = tahsil edilen fiyat, %100 |
| JTBD-3 | "Kod gelmezse param yanmasın." | İptalde iade oranı %100, iade süresi < 5 sn |
| JTBD-4 | "Bakiyemi kolayca yükleyebileyim." | Havale/USDT: onay < 4 saat (mesai içi) |
| JTBD-5 | "Sorun olursa birine yazabileyim." | Ticket ilk yanıt < 4 saat (mesai içi) |

### 3.2 Operatör / Admin (bizim taraf)

| # | İş | Başarı ölçütü |
|---|---|---|
| JTBD-6 | "Yeni bir sağlayıcıyı kod yazmadan tanımlayabileyim." | Panelden ekleme + eşleştirme; kod yalnız yeni *protokol* için |
| JTBD-7 | "Hangi işlemden ne kazandığımı görmek isterim." | Sipariş bazında maliyet / satış / marj raporu |
| JTBD-8 | "Bakiye yükleme taleplerini güvenle onaylayabileyim." | Onay idempotent, audit log'lu, geri alınabilir |
| JTBD-9 | "Bir sağlayıcı çökerse anlamalıyım." | Sağlayıcı hata oranı > %20'de alarm |
| JTBD-10 | "Fiyat marjını servis/ülke bazında ayarlayabileyim." | Panelden kural yönetimi, dağıtım gerekmez |

### 3.3 Şimdilik kapsam dışı kullanıcı
**API müşterisi / bayi (reseller).** v1'de yok. Veri modeli buna engel olmayacak şekilde tasarlanır.

## 4. Değer önerisi ve farklılaşma

| Rakip yaklaşımı | Bizim yaklaşımımız |
|---|---|
| Tek sağlayıcının bayisi olmak | **Sağlayıcı soyutlaması** — fiyat ve stok riski dağıtılabilir |
| Dolar bazlı fiyat, kur riski kullanıcıda | **TL bazlı kilitli teklif** — kur riskini biz taşırız |
| "Kod gelmedi"de muhatap yok | **Otomatik iade** — süre dolumunda/iptalde bakiye anında geri yüklenir |
| İngilizce/Rusça arayüz | **Tamamen Türkçe** panel ve destek |

> ⚠️ **Dürüst kayıt:** Bugün yalnız **bir** aktif sağlayıcımız var (HeroSMS). "Çok sağlayıcılı arbitraj"
> v1 lansmanında bir **pazarlama vaadi değildir** — v1'de kurduğumuz şey bu yeteneği mümkün kılan
> *mimaridir*. İkinci sağlayıcı devreye girene kadar bu farklılaşmayı iletişimde kullanmayız.
> İlgili roadmap kalemi: `M7`.

**Savunulabilirlik:** "Ucuz numara satmak" tek başına savunulabilir değil. Savunulabilir olan, *sağlayıcı adaptör katmanı*: yeni sağlayıcı eklemek bizde bir gün sürerken rakipte mimari değişiklik gerektirir.

## 5. İş modeli ve birim ekonomi

```
Satış fiyatı (TL) = yukarı_yuvarla(
    sağlayıcı_maliyeti_USD × sağlayıcı_düzeltmesi × USD/TRY × (1 + marj%) + sabit_bedel
  ) , en az minimum_fiyat
```

Kritik noktalar:

- **Marj servis ve ülke bazında ayarlanabilir.** Tek global yüzde yetersizdir: WhatsApp/TR yüksek talepli ve rekabetçi (düşük marj), niş kombinasyonlar yüksek marj taşır. Bu bir *iş kararıdır*, koda gömülmez — `PricingRule` tablosunda yaşar.
- **Kur riski bizde.** Maliyet USD, gelir TL. Yönetimi: (a) teklif geçerlilik süresi kısa (120 sn), (b) kur güvenlik tamponu, (c) 5 dakikada bir tazeleme, (d) kur bayatlarsa satış durur.
- **İade riski maliyettir.** Kullanıcıya iade ettiğimizde sağlayıcı bize her zaman iade etmez. Karşılanmayan iadeler gider olarak raporlanır.
- **Ön ödemeli bakiye gelir değil, borçtur.** Kullanıcı bakiyesi harcandığında gelire dönüşür. Muhasebe modeli bunu ayırır (`design.md` §5).

> **Ürün ilkesi:** Bir işlemin kârlı olup olmadığı satın alma anında hesaplanabilir olmalı.
> Marjı ölçemediğimiz bir akışı canlıya almayız.

## 6. Ürün ilkeleri

Tartışmayı bitiren kurallar. Bir tasarım kararı bunlardan biriyle çelişiyorsa karar yanlıştır.

1. **Kullanıcının parası kutsaldır.** Belirsizlikte kullanıcı lehine karar verilir.
2. **Gösterilen fiyat = tahsil edilen fiyat.** İstisnasız. Fiyat değiştiyse işlem reddedilir, sessizce farklı tutar çekilmez.
3. **Her para hareketi kayıtlı ve geri izlenebilir.** Bakiye bir sayı değil, bir defterin sonucudur.
4. **Sağlayıcı arızası kullanıcının sorunu değildir.** Hata → otomatik iade; kullanıcıya ham API hatası gösterilmez.
5. **Admin yetkisi varsayılan değil istisnadır.** Her uç nokta kapalı doğar.
6. **Kullanıcı kendi verisinden başkasını göremez.** Sahiplik kontrolü opsiyonel katman değil, sorgunun parçasıdır.
7. **Türkçe arayüz, İngilizce kod.** Kullanıcıya giden her metin Türkçe; değişken/tablo/fonksiyon adı İngilizce.
8. **Yeni ürün tipi eklemek veri işidir, tablo cerrahisi değil.** Katalog modeli genel tutulur.

## 7. Kapsam

### v1 kapsamında (çalışır halde)
- E-posta + şifre ile kayıt/giriş, e-posta doğrulama, şifre sıfırlama
- Rol/izin tabanlı yetkilendirme
- TL bakiye cüzdanı (ledger tabanlı), hareket dökümü
- Bakiye yükleme: **banka havalesi (dekont)** + **USDT TRC20/ERC20 (TX hash)** — ikisi de admin onaylı
- Ülke × servis kataloğu, canlı fiyat + stok, kilitli teklif (quote)
- Sipariş oluşturma, numara teslimi, **sağlayıcı webhook'u → SSE ile anlık kod bildirimi**
- İptal ve otomatik iade, süre dolumunda otomatik iade
- Sipariş geçmişi, destek talebi (ticket)
- Admin: kullanıcı, sağlayıcı, eşleştirme, fiyat kuralları, bakiye talepleri, ticket, audit log
- KVKK aydınlatma metni, kullanım şartları

### v1'de **şeması hazır**, özelliği kapalı (v1.1'de açılacak)
Bu ikisi veri modelinde ve sağlayıcı arayüzünde **baştan** yer alır; sonradan eklemek göç gerektirmez.

| Özellik | v1'de olan | v1.1'de eklenecek |
|---|---|---|
| **Kiralık numara** (uzun süreli, çok mesajlı) | `ProductType.RENTAL`, `rental_details`, `order_messages`; `ProviderPort.Extend`/`ListMessages`. **Sağlayıcı desteği doğrulandı:** aynı satın alma uç noktası + `duration` (1–180 gün) | UI, süre bazlı fiyatlandırma, uzatma akışı |
| **Referans / affiliate** | `referrals`, `referral_rules` tabloları; `LedgerType.COMMISSION` | Davet linki, komisyon hesaplama işi, raporlama |

### v1 kapsamı dışında (bilinçli)
| Dışarıda | Neden |
|---|---|
| Bayi / genel REST API | Çekirdek akış oturmadan API sözleşmesi dondurulmaz |
| Kart ile ödeme | Üye işyeri hesabı tüzel/şahıs şirketi gerektirir — bkz. §9 |
| Otomatik kripto doğrulama (zincir izleme) | Adres türetme, onay sayısı, reorg yönetimi ciddi altyapı; v1'de manuel onay |
| E-posta / sesli arama doğrulama ürünü | Sağlayıcı destekliyor (`/emails`, `verificationType: "call"`) ve katalog modelimize şema değişikliği olmadan sığıyor — ama v1 odağı dağıtmamalı |
| Mobil uygulama | Responsive web yeterli |
| Çoklu dil içeriği | i18n *altyapısı* v1'de kurulur, çeviri v2 |
| Kendi SIM havuzu | Tamamen farklı iş modeli |

## 8. Başarı ölçütleri

**Teknik (v1 çıkışında ölçülebilir):**
- Sipariş başarı oranı (numara teslim / denenen) ≥ %95
- Kod teslim oranı (kod geldi / numara alındı) ≥ %70 — kalanı iade
- Fiyat sorgusu p95 < 1.5 sn
- **Muhasebe tutarlılığı: `Σ ledger == user.balance`, her kullanıcı, her gün — sapma 0**
- Kritik akışlarda (para, yetki, sipariş) test kapsamı ≥ %80
- Planlanmamış kesinti < ayda 1 saat

**Ürün:**
- Kayıt → ilk satın alma dönüşümü ≥ %25
- 30 günlük elde tutma ≥ %40
- Ticket / sipariş oranı ≤ %5
- İşlem başına brüt marj ≥ %25

## 9. Kısıtlar ve riskler

| Risk | Etki | Karşılık |
|---|---|---|
| 🔴 **Yasal yapı yok** — şirket kurulmayacak. Türkiye'de kart ödemesi üye işyeri hesabı gerektirir (vergi levhası şart); **kripto ile mal/hizmet bedeli tahsilatı 2021 tarihli yönetmelikle yasaktır**; şahsi hesaba ticari tahsilat kayıt dışı faaliyettir. | **Lansmanı engeller** | Yazılım eksiksiz kurulur. **Lansman `M9` kapısında durur** ve ödeme kanalı kararı orada verilir. Teknik plan bundan etkilenmez. Karar seçenekleri roadmap `M9`'da listelidir. |
| 🔴 **Tek sağlayıcı** — HeroSMS kapanırsa hizmet durur | Yüksek | Adaptör mimarisi v1'de kurulur; `M7`'de ikinci sağlayıcı hedeflenir. En az 2 aktif sağlayıcı ticari lansmanın ön koşuludur. |
| 🟠 **Kur şoku** | Orta | Kısa teklif süresi + tampon + sık tazeleme + bayat kurda satış durdurma |
| 🟠 **Kötüye kullanım** — platformun dolandırıcılık/spam için kullanılması | Yüksek | Kullanım şartlarında yasaklı kullanım; hız limiti; şüpheli desen tespiti; KVKK uyumlu log saklama |
| 🟠 **Kodu Claude geliştiriyor, sahibi okumuyor** | Orta | Doküman-öncelikli çalışma bu yüzden zorunlu: `trd.md` kabul kriterleri (KK-*) sistemin doğruluğunun tek dış ölçüsüdür. Her kritik akış testle ispatlanır; "çalışıyor" iddiası test çıktısıyla desteklenir. |
| 🟠 **Ara onay yok — sapma geç fark edilir** | Orta | Sahip uçtan uca sonucu görecek. Telafi: `intent.md` §11 "bitti" listesi ve `trd.md` KK maddeleri sözleşme işlevi görür; belirsizlikte varsayım açıkça yazılır ve teslimde raporlanır. |

## 10. Neden yeniden yazıyoruz

Mevcut sistem (`main`, 127 commit, son commit 2026-04-09, canlıya hiç alınmadı) prototip olarak amacına ulaştı: iş modelinin çalışabilirliğini gösterdi. Ticari kullanıma uygun değil. Somut nedenler:

**Güvenlik ve para bütünlüğü:**
- Giriş yapmış herhangi bir kullanıcı `/admin/user/payments/approve/:id` ile **kendi ödemesini onaylayıp bakiyesini kendisi yükleyebiliyor** (yalnız `ensureAuthenticated` var, `ensureAdmin` yok).
- Giriş yapmış herhangi bir kullanıcı `/admin/user/profile/update/:id` ile **başka bir hesabın e-postasını ve şifresini değiştirebiliyor** — hesap ele geçirme.
- Sipariş sorgusunda sahiplik kontrolü yok → **bir kullanıcı başkasının SMS kodunu okuyabiliyor**.
- Bakiye güncellemeleri kilitsiz oku-değiştir-yaz döngüsüyle, float aritmetiğiyle yapılıyor → **eşzamanlı isteklerde çift alacak / kayıp güncelleme**.
- Kâr marjı hesaplanıyor ama kullanılmıyor → **platform maliyetine satıyor**.
- Veritabanı şifresi (`config/database.json`) ve Shopier anahtarları git geçmişinde.

**Teknik sürdürülemezlik:**
- Kodun önemli bir kısmı hiç çalışmıyor: `SmsCountry`, `ServiceSetting`, `SmsInventory` modelleri 17 yerde kullanılıyor, hiç tanımlanmamış → `SmsManager`, `SmsSyncService`, `smsController` ve sağlayıcı otomatik eşleştirmenin tamamı ölü kod.
- İki ORM (Sequelize + yarım Prisma), iki bcrypt, hiç kullanılmayan Mongoose ve node-cron.
- Katman yok: HTTP, iş mantığı, dış API ve DB erişimi aynı fonksiyonda.
- 922 satırlık view, içinde tüm satın alma mantığı gömülü inline script.
- Test yok, lint yok, CI yok, `.env.example` yok, README yok.

> Bu bir "kötü kod" eleştirisi değil, **olgunluk seviyesi** tespitidir. Prototip aşamasında bu tercihler
> makuldü; ticari ürün aşamasında değil. Bulguların dosya:satır düzeyinde tam listesi `memory.md` §3'te.

## 11. v1 "bitti" tanımı

- [ ] Kullanıcı kayıt olup, bakiye yükleyip (admin onaylı), numara alıp, kodu SSE ile görüp, kalan bakiyesini doğru görebiliyor — uçtan uca, üretim ortamında
- [ ] `Σ ledger == user.balance` mutabakatı sıfır sapma; günlük cron ile kontrol ediliyor ve sapmada alarm üretiyor
- [ ] Her uç nokta için yetki testi var; sahiplik kontrolü olmayan uç nokta yok
- [ ] Gösterilen fiyat = tahsil edilen fiyat, testle ispatlanmış
- [ ] Sağlayıcı adaptörü sözleşme testleriyle kaplı; sahte adaptörle tüm akış ağsız çalışıyor
- [ ] **Sahte webhook isteği siparişi tamamlayamıyor** (KK-410) — teyit zinciri çalışıyor
- [ ] Kritik akışlar ≥ %80 test kapsamı; CI yeşil
- [ ] Sırların hiçbiri kodda veya git geçmişinde değil; hepsi rotate edilmiş
- [ ] Yapılandırılmış log + hata takibi + metrik + alarm çalışıyor
- [ ] KVKK aydınlatma metni ve kullanım şartları yayında
- [ ] Yedekleme alınıyor ve **geri yükleme bir kez test edilmiş**
- [ ] `README.md` var: bir geliştirici sıfırdan 15 dakikada ayağa kaldırabiliyor
- [ ] **Uçtan uca akış dört tarayıcı projesinde de (chromium/webkit × masaüstü/mobil) yeşil**
- [ ] **Gerçek bir iPhone ve gerçek bir Android cihazda elle doğrulandı** — özellikle SSE'nin
      arka plandan dönüşte toparlanması
- [ ] **Lansman kapısı `M9` açıkça değerlendirilmiş** ve karar kayda geçmiş

---

**İlgili:** [design.md](design.md) · [trd.md](trd.md) · [frontend-contract.md](frontend-contract.md) · [roadmap.md](roadmap.md) · [memory.md](memory.md) · [../CLAUDE.md](../CLAUDE.md)
