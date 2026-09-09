# Teslim notu

> Bu belge **devralan kişi** içindir. Neyin çalıştığını, neyin çalışmadığını ve
> ilk gün nelere çarpacağınızı anlatır. Övgü yok; eksikler açıkça yazılı.
>
> Kurulum → [../deploy/README.md](../deploy/README.md) · Arıza → [runbook.md](runbook.md)

---

## 1. Ne teslim ediliyor

Sanal numara (SMS doğrulama) satış platformu. Kullanıcı ön ödemeli TL bakiyesiyle
servis ve ülke seçip numara alır, gelen kodu görür; kod gelmezse ücreti otomatik
iade edilir.

**Çalışan uçtan uca akışlar** — gerçek sunucuya karşı doğrulandı, birim testi değil:

| Akış | Doğrulama |
|---|---|
| Kayıt → e-posta doğrulama → giriş | duman testi, 29 senaryo |
| Bakiye yükleme talebi → dekont → yönetici onayı → deftere yazım | canlı; 20 eşzamanlı onay → **tek** kayıt |
| Numara alma → SSE ile kod → iptal → tam iade → sağlayıcıda kapatma | canlı, gerçek parayla (0,70 ₺) |
| Fiyat kuralı yazma → canlı önizleme → satış fiyatının değişmesi | canlı |
| Mutabakat `Σ defter == bakiye` | sapma sıfır |

---

## 2. 🔴 Üretime çıkmadan ÖNCE yapılması gerekenler

Bunlar **kod işi değil**, sizin yapmanız gerekenler:

1. **HeroSMS API anahtarını döndürün.** Geliştirme sırasında anahtar iki kez
   sohbete düştü. Sağlayıcı panelinden yenileyip yönetim panelinden girin
   (*Sağlayıcılar → API anahtarı*). Anahtar yanıtta hiç dönmez, yalnız maskeli
   önizleme gösterilir.

2. **Sırları üretin.** `deploy/.env.prod.example` içindeki her `DOLDUR:`
   satırının yanında üretme komutu var. `SESSION_SECRET` ve `ENCRYPTION_KEY`
   32 bayt olmalı; **`ENCRYPTION_KEY` kaybolursa veritabanındaki şifreli
   sağlayıcı anahtarları çözülemez** — yedekleyin.

3. **`TRUSTED_PROXIES` değerini ağınıza göre verin.** Boş bırakılırsa süreç
   başlamaz (kasıtlı). Varsayılan compose ağı için `172.16.0.0/12`.

4. **Yasal metinleri yazdırın.** `/gizlilik` ve `/kullanim-sartlari` sayfaları
   **bilerek boş**; yerinde "hukuki inceleme sonrası yayımlanacak" uyarısı var.
   Yasal metin uydurulmaz. KVKK aydınlatma metni de gerekli.

5. **Servis logolarını geri yükleyin.** Geliştirme kataloğu silindiğinde
   logolar da gitti, ama üretilmiş atamalar depoda duruyor
   (`web/scripts/servis-logolari.sql`, 517 servis). Sağlayıcıyı ekleyip
   katalog senkronunu yaptıktan sonra:

   ```bash
   psql "$DATABASE_URL" -f web/scripts/servis-logolari.sql
   # yeni servisler için yeniden üretmek isterseniz:
   cd web && node scripts/servis-logolari.mjs --uygula
   ```

6. **Geri yükleme tatbikatı yapın.** Yedekleme betiği yazıldı ve kapıları
   sabotajla test edildi, ama **gerçek bir geri yükleme hiç denenmedi**.
   `make restore-drill YEDEK=...` ile tatbikat veritabanına yükleyin ve
   `make reconcile` ile doğrulayın. Denenmemiş yedek, yedek değildir.

---

## 3. Bilinen eksikler — bilerek yapılmadı

| Eksik | Etkisi | Neden yapılmadı |
|---|---|---|
| **İkinci sağlayıcı** (M7) | Tek sağlayıcıya bağımlılık: HeroSMS çökerse satış durur | Adaptör arayüzü hazır, ikinci sağlayıcı ~1 gün |
| **next-intl / çoklu dil** | Metinler bileşenlere sabit yazılı (Değişmez #14 ihlali) | Proje çapında karar; tüm ekranlar bugün böyle |
| **ESLint yapılandırması** | `make lint`in eslint adımı çalışmıyor | Depoda `eslint.config.*` yok |
| **Sağlayıcıda açık kalan numara** | Sipariş yazılamazsa numara ilk 120 sn iptal edilemiyor ve kayıt kalmadığı için yeniden deneme işi onu göremiyor — üretimde gerçek para | Yük testi ortaya çıkardı; kalıcı "yetim uzak sipariş" kaydı gerekiyor |
| **SSE bağlantı sınırı** | Her akış ayrılmış bir Redis bağlantısı tutuyor; kullanıcı başına sınır yok | Ölçüldü (akış başına 1,0); tek sunucuda bugün sorun değil |
| **Gerçek cihaz turu** | iOS Safari davranışı emülasyonla doğrulandı, cihazla değil | Ortamda cihaz yok |
| **Boyut eşleştirme ekranı** (FR-702) | Ülke/servis eşleştirmesi elle düzenlenemiyor | Katalog senkronu otomatik dolduruyor; elle düzeltme CLI'dan |
| **Kâr raporu** (FR-706) | Marj/kâr raporu yok | `ÖNERİLEN` işaretli, zorunlu değil |
| **Redis dağıtık kilidi** | İkinci bir sunucu örneği eklenirse işler çift koşar | Tek sunucu tasarımı; ikinci örnekten ÖNCE gerekli |

---

## 4. 🟡 Bilinen davranışlar — hata değil, karar

- **Fiyat kuralı "güncellenmez", yenisi yazılır.** Aynı kapsama yeni kural
  eskisini aynı transaction içinde devreden çıkarır. Geçmiş kural silinmez ki
  eski siparişlerin hangi kuralla fiyatlandığı izlenebilsin.
- **Son GLOBAL kural pasifleştirilemez** (409). Kalmazsa her teklif
  `NO_PRICING_RULE` ile düşer: site açık kalır ama hiçbir şey satılamaz.
- **Fiyat önizlemesi önbellekteki maliyeti kullanır** (`costSource: CACHE`).
  Aktivasyon satın almasında canlı maliyet sorulduğu için gerçek fiyat
  önizlemeden farklı çıkabilir; ekranda bu uyarı gösteriliyor.
- **İade edilmiş siparişe gecikmeli gelen kod kullanıcıya gösterilmez.**
  Kayıt veritabanında durur (iade tartışmasının kanıtı odur) ama üç kanalın
  üçünde de gizlenir. Aksi hâlde kullanıcı hem parayı hem numarayı alırdı.
- **Türkiye numarası satılamıyor.** Sağlayıcının API'si Türkiye için 122
  serviste de `physical = 0` döndürüyor (kıyas: 195 ülkenin 67'sinde > 0).
  Ülke listede en üstte ama **seçilemez** durumda. Ayrıntı: [memory.md](memory.md) §H23.

---

## 5. İlk gün: yönetici ne yapabilir, ne yapamaz

**Arayüzden yapılabilir:**
kullanıcı arama/askıya alma · bakiye düzeltme · bakiye taleplerini onaylama/reddetme ·
dekont görüntüleme · ödeme yöntemi ekleme/düzenleme/aktifleştirme ·
sağlayıcı ekleme/ayar/API anahtarı/katalog senkronu · fiyat kuralı yazma ve
canlı önizleme · denetim kaydını okuma

**Yalnız CLI'dan yapılabilir** (`cd api && go run ./cmd/cli`):

| İş | Komut |
|---|---|
| İlk yönetici rolünü atama | `admin:grant --email=... --role=admin` |
| Kur tazeleme (işçi zaten 10 dk'da bir yapıyor) | `fx:sync` |
| Kiralık katalog senkronu | `catalog:rentals --provider=herosms` |
| Servis logosu ayarlama | `catalog:icon --service=... --url=...` |
| Mutabakat | `wallet:reconcile` |

> İlk yönetici rolü **zorunlu olarak CLI'dan** atanır: kendi kendine rol
> verebilen bir uç, yetki modelinin tamamını anlamsız kılardı.

---

## 6. Testlerin gerçekten kapsadığı

Sayı vermek iddiadan iyidir:

```bash
make check          # 11 adım: garanti yorumları · sqlc · vet ×2 · derleme ·
                    # birim · entegrasyon · duman · web tip · web derleme
cd web && AUDIT_EMAIL=... AUDIT_PASSWORD=... node scripts/responsive-check.mjs
```

- **Entegrasyon testleri gerçek PostgreSQL'e karşı koşar** (SQLite taklidi yok —
  `FOR UPDATE` davranışı farklıdır).
- **Para yollarında eşzamanlılık testi zorunludur** ve vardır: bakiye düzeltme,
  bakiye onayı, talep oluşturma, sipariş iadesi.
- **Her yeni regresyon testi sabotajla doğrulandı**: düzeltme geçici olarak geri
  alınır, testin *gerçekten* düştüğü görülür. Düşmeyen test, test değildir.
  Bu turda iki test sabotajı geçti ve **testler güçlendirildi**, kod değil.

- **Uçtan uca akış testleri**: 19 akış × 4 tarayıcı projesi = **76 test**,
  hepsi geçiyor. `webkit-mobile` dahil (iOS'ta tüm tarayıcılar WebKit).
  Kendi veritabanı ve portlarıyla izole koşar: `./scripts/e2e.sh`
- **Yük testi**: k6, 50 eşzamanlı kullanıcı. Ölçüm tutanağı
  [yuk-testi-sonuc.md](yuk-testi-sonuc.md). En önemli sonuç: **yük altında
  2.592 defter kaydı yazıldı ve mutabakat sapması sıfır kaldı.**
- **SEO denetimi**: `cd web && node scripts/seo-check.mjs` — benzersiz başlık,
  canonical, og:image gerçek boyutu, sitemap'te gizli yol olmaması, noindex.

**Kapsanmayanlar:** ikinci sağlayıcı sözleşme testi, gerçek cihaz turu,
uzun süreli dayanıklılık (soak) koşumu, Caddy arkasında SSE ölçümü.

---

## 7. Bu projede uyulan kurallar

`CLAUDE.md` bağlayıcı değişmezleri listeler. En çok işe yarayan üçü:

1. **Yorum garanti veriyorsa testi olmalı.** `scripts/check-guarantees.py` bunu
   zorlar ve **atfın gerçek olduğunu da doğrular** — var olmayan bir teste atıf
   denetimi düşürür. (Bu denetleyicinin kendisi bir kez atlatıldı; §3.15.)
2. **Commit kapısı.** `scripts/commit.sh` `check.sh` geçmeden commit etmez.
   Depoda üç kez başarısız kontrollerle commit atıldığı için kodda.
3. **Testler ayrı veritabanı kullanır** ve adı `_test` ile bitmiyorsa
   **hiç başlamaz** — entegrasyon testleri `DELETE FROM` yapar.
