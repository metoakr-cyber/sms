---
# ══════════════════════════════════════════════════════════════════════════
# MAKİNE OKUNUR JETON BLOĞU
#
# DOĞRULUK KAYNAĞI `web/src/app/globals.css`'tir. Bu blok taşınabilir bir
# dışa aktarımdır. Bir jeton orada değişirse İKİSİ BİRDEN güncellenir.
# `durum: mevcut` = bugün kodda var, ölçüldü.  `durum: eklenecek` = yok, §2'de
# gerekçesi ve kaynağı yazılı. Uydurulmuş değer yoktur.
# ══════════════════════════════════════════════════════════════════════════
ad: Onay360 Tasarım Sistemi
kapsam: web/src/**
modlar: [operate, persuade]

renk:
  durum: mevcut
  kaynak: web/src/app/globals.css:7-177
  yuzey:    { bg: var(--bg), surface: var(--surface), raised: var(--raised), border: var(--border) }
  metin:    { text: var(--text), muted: var(--muted) }
  marka:    { panel: --color-brand-500 (#3454d1), pazarlama: --color-onay-500 (#1a7fd4) }
  durumlar: { ok: "#17c666", warn: "#ffa21d", bad: "#ea4d4d", info: "#3dc7be" }
  not: "brand-* panel ve yönetimde kalır; onay-* pazarlama yüzeyinindir. globals.css:34-40"

golge:
  durum: mevcut
  kaynak: web/src/app/globals.css:106-108
  golge-1: "0 1px 2px rgb(0 0 0 / 0.30)"      # yapışkan başlık altı
  golge-2: "0 4px 14px -4px …"                 # açılır katman
  golge-3: "0 18px 40px -12px …"               # YALNIZ modal
  craft-floor-dogrulamasi: "üçünde de offset ≠ 0 ve blur > 0 — craft-floor.md:10 geçti"

tipografi:
  durum: kismen                                 # aile var, ölçek yok
  aile: var(--font-sans)                        # globals.css:57
  olcek_orani: "1.11–1.25"                      # operate.md:15 hedefi 1.125–1.2
  adimlar:                                      # Tailwind varsayılanları — config değişmez
    xs:   { px: 12, kullanim: "YALNIZ rozet ve ikincil dipnot. VERİ DEĞİL." }
    sm:   { px: 14, kullanim: "tablo gövdesi, ikincil metin — Operate tabanı" }
    base: { px: 16, kullanim: "gövde metni; TÜM girdi alanları (iOS sınırı)" }
    lg:   { px: 18, kullanim: "kart/bölüm başlığı (h2)" }
    xl:   { px: 20, kullanim: "BUGÜN 0 KULLANIM — eksik basamak" }
    "2xl":{ px: 24, kullanim: "sayfa başlığı (h1), mobil" }
    "3xl":{ px: 30, kullanim: "sayfa başlığı (h1), md: üstü" }
  satir_uzunlugu: { duz_metin: "65-75ch", tablo: "120ch+ serbest" }   # craft-floor.md:12 · operate.md:16
  tracking_tabani: "-0.04em"                                          # craft-floor.md:12
  tabular_nums: "para/sayı/tarih sütunlarında ZORUNLU"                # craft-floor.md:15

bosluk:
  durum: mevcut-ama-kuralsiz
  taban: "4px (Tailwind varsayılanı)"           # layout.md:49 "4-unit base"
  izinli_adimlar: [1, 2, 3, 4, 5, 6, 8, 10, 12]  # gap-1 … gap-12
  yasak_adimlar: ["0.5", "1.5", "2.5", "3.5"]    # yarım basamaklar — ızgarayı bozar
  kural: "başlık ÜSTÜ boşluk > başlık ALTI boşluk"   # craft-floor.md:11

yaricap:
  durum: mevcut-ama-kuralsiz
  lg:   { px: 8,  kullanim: "rozet, küçük etiket" }
  xl:   { px: 12, kullanim: "buton, girdi, satır kartı — yönetimin ANA yarıçapı" }
  "2xl":{ px: 16, kullanim: "Card, modal" }
  full: { kullanim: "avatar, nokta göstergesi" }

hareket:
  durum: eklenecek                              # globals.css'te 0 jeton, 6 farklı süre dolaşımda
  kaynak: /tmp/emilskills/skills/review-animations/STANDARDS.md:32-34,49,59
  ease-out:    "cubic-bezier(0.23, 1, 0.32, 1)"     # VARSAYILAN — giren/çıkan
  ease-in-out: "cubic-bezier(0.77, 0, 0.175, 1)"    # ekran içinde yer değiştiren
  ease-drawer: "cubic-bezier(0.32, 0.72, 0, 1)"     # alttan yükselen sayfa
  sure-basma:  "160ms"    # STANDARDS.md:59 birebir
  sure-ipucu:  "150ms"    # 125-200ms aralığından seçildi
  sure-menu:   "200ms"    # RECIPES.md:36 birebir (dropdown)
  sure-katman: "250ms"    # RECIPES.md:88 birebir (modal) = Operate tavanı
  tavan: "250ms"          # operate.md:41 (150-250) ∩ STANDARDS.md:49 (<300)
  basma_olcegi: "scale(0.97)"                       # STANDARDS.md:59, aralık 0.95-0.98
  animasyonlanabilir: [transform, opacity, clip-path, "height (yalnız akordeon)"]

z_index:
  durum: eklenecek        # bugün 4 değer, 1 çakışma (modal z-50 ↔ atlama bağlantısı z-50)
  taban: 0
  yapiskan: 20            # site-nav + admin-shell + panel-shell — ÜÇÜ DE AYNI olmalı
  acilir: 40
  ortu: 60
  katman: 70
  toast: 80
  atlama: 90              # atlama bağlantısı HER ŞEYİN üstünde

kirilim:
  durum: mevcut
  kaynak: docs/frontend-contract.md:62
  degerler: { sm: 640, md: 768, lg: 1024, xl: 1280 }
  test_genislikleri: [320, 390, 430, 768, 1440]
  ozellestirme: yasak

erisilebilirlik:
  kontrast_govde: "4.5:1"
  kontrast_buyuk_metin: "3:1"
  kontrast_kontrol_ve_odak: "3:1"
  dokunma_hedefi: "44x44px"
  hedefler_arasi_bosluk: "8px"
  girdi_min_font: "16px"
---

# tasarim-sistemi.md

Bu dosya `CLAUDE.md`'nin **tasarım karşılığıdır**. Tartışma değil kural içerir; gerekçe uzunsa
`docs/` altına link verir. Her kural ölçülebilirdir — "güzel dursun" diye bir madde yoktur.

**Amaç:** ileride eklenecek her yeni ekran, bu dosyayı okuyan bir insan ya da ajan tarafından
mevcut ekranlarla **%100 uyumlu** yazılabilsin.

> Bu doküman `docs/frontend-contract.md`'yi **değiştirmez, daraltır**. Çelişki halinde
> `frontend-contract.md` ve `CLAUDE.md` Değişmezleri kazanır; bu dosya onların üstüne
> görsel/hareket kararları ekler.

**Kaynaklar.** Her sayısal değer ya mevcut koddan ölçülmüştür ya da şu iki referanstan alınmıştır:
`/tmp/emilskills` (Emil Kowalski — hareket standartları) · `/tmp/impeccable` (Impeccable — mod
ayrımı ve craft floor). Kaynak satır numaraları kuralın yanındadır. **Kaynaksız sayı yoktur.**

---

## 1. Kapsam ve mod

Bu ürünün iki farklı yüzeyi vardır ve **aynı kurallara tabi değildir**. Mod ayrımı
Impeccable'ın kendi ayrımıdır (`.pi/skills/impeccable/reference/operate.md`).

| Mod | Yollar | Kullanıcı ne yapıyor | Neyi kazanmalı |
|---|---|---|---|
| **Operate** | `/yonetim/**` · `/panel/**` | Bir görevi bitiriyor (talebi onayla, numara al) | **Taranabilirlik, tutarlılık, hız** |
| **Persuade/Read** | `/` · `/fiyatlar` · `/servisler` · `/sss` · `/blog` · `/hakkimizda` · hukuki sayfalar | İkna oluyor veya okuyor | İfade, ritim, marka |

### 1.1 Operate modunun tek cümlesi

> **Araç görevin içinde kaybolmalı.** — `operate.md:9`
> Ürün arayüzünün başarısızlık biçimi düzlük değil, **amaçsız tuhaflıktır**.

Bunun ölçülebilir karşılığı:

| Operate'te ÜSTÜN | Operate'te ALTTA |
|---|---|
| Taranabilirlik · tutarlılık · yerel beklentiler · gerçek kullanım sahnesi | İfade (expression) |

`impeccable/SKILL.md:37` bu sıralamayı açıkça yazar. **Yönetim panelinde bir tasarım kararı
"daha etkileyici" diye savunulamaz.** Savunma "daha hızlı taranıyor", "daha tutarlı",
"daha az tıklama" olmak zorundadır.

### 1.2 🔴 Yönetim panelinde görkem aranmaz

Emil'in sıklık tablosu (`animate/SKILL.md:33-38`, birebir) bu işin yönünü belirler:

| Sıklık | Karar |
|---|---|
| Günde 100+ (klavye kısayolu, sekme gezintisi, tablo satırı) | **Animasyon yok. Asla.** "Stop here." |
| Günde onlarca (hover, liste gezinme) | Ya fark edilemeyecek kadar hızlı, ya hiç |
| Ara sıra (modal, drawer, toast) | Standart animasyon |
| Nadir / ilk kez (kurulum, boş durum, kutlama) | Delight bütçesi **yalnız burada** yaşar |

Emil aynı belgede: *"a professional dashboard should be crisp and fast."*
(`emil-design-eng/SKILL.md:577`)

**Kazanç nerede aranır:** tipografi ölçeği, boşluk ritmi, tablo okunabilirliği, tutarlılık,
tarayıcı yüzeyleri (§8). **Nerede aranmaz:** giriş animasyonu, stagger, sayfa geçişi, parallax.

### 1.3 İki mod arası taşıma yasağı

Pazarlama yüzeyinde meşru olan üç desen `/yonetim` ve `/panel`'e **taşınmaz**:

| Desen | Nerede | Neden Operate'e girmez |
|---|---|---|
| `[data-belir]` belirme animasyonu (550ms) | globals.css:344-349 | 550ms, Operate tavanının (250ms) 2,2 katı |
| `.servis-listesi > li` 3px renkli sol kenarlık | globals.css:359 | `craft-floor.md:35` — kart/liste/uyarıda 1px üstü renkli kenarlık reddedilir |
| `.nabiz` 2,2s sonsuz döngü | globals.css:373-380 | Bilgi taşımayan sürekli hareket; sıklık tablosunun ilk satırı |
| `.kahraman-isik` · `.gradyan-*` · `.vurgu-metin` | globals.css:268-285 | Gradyan metin `craft-floor.md:33` ile reddedilir |

---

## 2. Jetonlar

Bugün `globals.css` **69 benzersiz jeton** taşıyor ve bunların **57'si renktir**. Yani mevcut
sistem bir *renk paleti*dir. Boşluk, yarıçap, hareket ve z-index ölçeklerinin **beşi de sıfırdır**
— bu yüzden 10 yönetim ekranı ~1.030 satırlık mekanik tekrarla ayakta duruyor.

### 2.1 Mevcut ve korunanlar

| Grup | Adet | Karar |
|---|---|---|
| Renk ölçekleri (`--color-*`) | 36 | **Korunur.** `brand-*` panel/yönetim, `onay-*` pazarlama (globals.css:34-40) |
| Anlamsal yüzeyler (`--bg --surface --raised --border --text --muted`) | 6 | **Korunur.** Yönetimde renk BU ALTISINDAN alınır |
| Durum renkleri (`ok warn bad info`) | 4 | **Korunur.** §5.4'te tek eşleme tablosu |
| Vurgu aileleri (`--v-*`) | 12 | Pazarlamada kalır; yönetimde kullanılmaz |
| Gölge (`--golge-1/2/3`) | 3 | **Korunur.** Yönetimde kullanımı §2.4'te sınırlanır |
| Gradyan (`--gradyan-*`) | 7 | Pazarlamada kalır (§1.3) |

> **Renk uyarısı — ölçülmüş tuzak.** `globals.css:127` yorumu: açık temada `--muted` başlangıçta
> `#64748b` idi ve `--bg` üzerinde **4,44:1** veriyordu — sınırın altında. Ölçülünce çıktı.
> **Yeni bir gri metin rengi eklenmez; `--muted` kullanılır.**

### 2.2 🔴 Hareket jetonları — EKLENECEK

Bugün yönetim panelinde **6 farklı süre ve 4 farklı eğri** jetonsuz dolaşıyor, ve
10 ekranın tamamında **tek bir `transition`** var (`denetim/page.tsx:351`, bir `▾` döndürüyor).
Panel "az animasyonlu" değil, **jetonsuz** — doğru davranışı üretmenin tek yolu değeri her
bileşende elle tekrar yazmak.

```css
:root {
  /* Eğriler — Emil, review-animations/STANDARDS.md:32-34 (birebir, yaklaştırma yok) */
  --ease-out:    cubic-bezier(0.23, 1, 0.32, 1);    /* VARSAYILAN: giren/çıkan her şey */
  --ease-in-out: cubic-bezier(0.77, 0, 0.175, 1);   /* ekranda yer değiştiren/biçim değiştiren */
  --ease-drawer: cubic-bezier(0.32, 0.72, 0, 1);    /* alttan yükselen sayfa (mobil modal) */

  /* Süreler */
  --sure-basma:  160ms;   /* STANDARDS.md:59 birebir · buton/satır :active         */
  --sure-ipucu:  150ms;   /* 125-200ms aralığından · tooltip, küçük popover        */
  --sure-menu:   200ms;   /* RECIPES.md:36 birebir · açılır liste, akordeon, sekme */
  --sure-katman: 250ms;   /* RECIPES.md:88 birebir · modal + arka planı            */
}
```

**Eğri seçim sırası** (`STANDARDS.md:20-25`) — bu bir karar ağacıdır, tercih değil:

| Durum | Eğri |
|---|---|
| Giren veya çıkan | `--ease-out` |
| Ekranda hareket eden / biçim değiştiren | `--ease-in-out` |
| Yalnız renk / hover | `ease` (yerleşik) |
| Sabit hızlı hareket (ilerleme çubuğu) | `linear` |
| Kararsız kaldıysan | `--ease-out` |

🔴 **`ease-in` arayüzde ASLA kullanılmaz.** *"It starts slow, which makes the interface feel
sluggish… ease-in delays the initial movement — the exact moment the user is watching most
closely."* (`emil-design-eng/SKILL.md:121`). `review-animations/SKILL.md:27` bunu bir **blok
sebebi** sayar: görüldüğü yerde iş durur.

🔴 **Yaklaştırılmış eğri yazılmaz.** `animate/SKILL.md:24`: *"Never invent
`cubic-bezier(0.4, 0, 0.2, 1)` because it looks familiar."* Bugün `globals.css:347`'de
`cubic-bezier(.22,.61,.36,1)` var — onaylı üç jetondan hiçbiri değil. Pazarlamada kalır,
yönetime **kopyalanmaz**.

### 2.3 Boşluk ölçeği

Bugün yönetim panelinde `gap` 7 basamak, `mt` 6 basamak, dolgu 10 basamak kullanıyor —
yarım basamaklar (`gap-1.5`, `mt-0.5`, `px-3.5`, `py-2.5`) dahil.

**Kural: Tailwind'in 4px tabanı korunur, yarım basamaklar kullanılmaz.**
`layout.md:49` — 4 birimlik taban, 8'e dayalı bir ölçeğin kaçırdığı orta adımları verir.

| Adım | px | Ne için |
|---|---|---|
| `1` | 4 | ikon–metin arası |
| `2` | 8 | ilişkili öğeler (en sık) |
| `3` | 12 | kart içi satırlar |
| `4` | 16 | bölüm içi ayrım (en sık) |
| `5` / `6` | 20 / 24 | kart dolgusu, bölümler arası |
| `8` / `10` / `12` | 32 / 40 / 48 | ekran blokları, sayfa üst-altı |

🔴 **Ritim kuralı (ölçülebilir):** *"tight groups, generous separation, more space above a
heading than below it. **Read the computed values.**"* — `craft-floor.md:11`
→ Bir başlığın `margin-top`'u `margin-bottom`'undan **büyük** olmalı; grup içi aralık,
gruplar arası aralığın **en fazla yarısı** olmalı. Bu DevTools'tan **okunur**, tahmin edilmez.

### 2.4 Yarıçap ve gölge

Yarıçap bugün 5 keyfi değerde. Ölçüm: `rounded-xl` 42 kullanım, `rounded-lg` 3,
`rounded-2xl` 2, `rounded` 2. Yani fiili ölçek zaten `xl` etrafında toplanmış.

| Jeton | px | Kullanım |
|---|---|---|
| `rounded-lg` | 8 | rozet, küçük etiket |
| `rounded-xl` | 12 | **buton, girdi, satır kartı — yönetimin ana yarıçapı** |
| `rounded-2xl` | 16 | `Card`, modal |
| `rounded-full` | — | avatar, durum noktası |

**Gölge — Operate modunda kısıtlı.** Üç mevcut gölge `craft-floor.md:10` testinden geçiyor
(hepsinde offset ≠ 0 ve blur > 0), ama yönetimde her karta serpilmez:

| Jeton | Yönetimde tek meşru kullanım |
|---|---|
| `--golge-1` | Yapışkan başlığın altı |
| `--golge-2` | Açılır katman (popover, dropdown) |
| `--golge-3` | **Yalnız modal** |
| *(hiçbiri)* | **Kartlar.** Yönetim kartı derinlik değil **kenarlık** kullanır — `operate.md:57` yoğunluğa izin verir, derinliğe değil |

### 2.5 z-index ölçeği — EKLENECEK

Bugün 4 değer var ve **iki tutarsızlık ölçüldü**:
- `site-nav.tsx:46` **z-40** ↔ `admin-shell.tsx:68` ve `panel-shell.tsx:106` **z-30** — aynı rol (yapışkan başlık), iki katman.
- `app/layout.tsx:81` atlama bağlantısı **z-50** ↔ `modal.tsx:91` modal örtüsü **z-50** — **çakışma.** DOM sırası gereği modal kazanır; modal açıkken odaklanan "İçeriğe atla" bağlantısı görünmez.

Frontmatter'daki altı basamaklı ölçek (`--z-taban … --z-atlama`) bu iki hatayı da kapatır.
**Atlama bağlantısı her zaman en üsttedir** — klavye kullanıcısının kaçış yolu hiçbir katmanın
altında kalamaz.

---

## 3. Tipografi

### 3.1 Ölçek — Tailwind varsayılanları, config değişmez

`operate.md:14` — **sabit rem ölçeği, akışkan (`clamp`) değil.** Ürün arayüzünde küçülen bir
başlık daha kötü görünür. `operate.md:13` — **tek aile yeter**; ürün arayüzü display/body
eşleşmesine ihtiyaç duymaz.

| Adım | px | Anlam | Satır yüksekliği |
|---|---|---|---|
| `text-xs` | 12 | 🔴 **YALNIZ rozet ve ikincil dipnot. VERİ DEĞİL.** | `leading-normal` |
| `text-sm` | 14 | **Operate tabanı:** tablo gövdesi, ikincil metin, etiket | `leading-normal` (1.5) |
| `text-base` | 16 | Gövde metni · **TÜM girdi alanları (zorunlu)** | `leading-normal` |
| `text-lg` | 18 | Kart / bölüm başlığı (`h2`) | `leading-snug` |
| `text-xl` | 20 | Alt başlık — **bugün 0 kullanım, eksik basamak** | `leading-snug` |
| `text-2xl` | 24 | Sayfa başlığı (`h1`), mobil | `leading-tight` |
| `text-3xl` | 30 | Sayfa başlığı (`h1`), `md:` üstü | `leading-tight` |
| `text-4xl`+ | 36+ | **Yalnız pazarlama.** Operate'te yasak | — |

Adım oranları: 1.14 · 1.125 · 1.11 · 1.2 · 1.25 — `operate.md:15`'in istediği 1.125–1.2 bandında.

### 3.2 🔴 En büyük tek kazanç: `text-xs`'i veriden çıkarmak

**Ölçüm:** `/yonetim` + ortak bileşenlerde `text-xs` (12px) **100 kullanım**, `text-sm` 81.
Yani panelin en çok kullanılan boyutu 12px ve taşıdığı şey mobil kart `<dl>` değerleri,
tablo alt satırları, sayfalama sayacı, `requestId` ve tablo `<thead>` — yani **operatörün
okuduğu asıl veri**.

`harden.md:81` ikincil metin tabanını **14px** olarak verir ve "yalnız gerçekten ikincil
olan için" der. Operate modunun taranabilirlik önceliğiyle birleşince kural nettir:

> **Veri taşıyan hiçbir metin `/yonetim` ve `/panel`'de 14px'in altına inmez.**
> `text-xs` yalnız `Badge` içinde ve gerçekten ikincil dipnotlarda meşrudur.

Bu tek değişiklik hiçbir animasyon gerektirmez ve panelin okunabilirliğini en çok artıran
maddedir.

### 3.3 Ağırlık ve hiyerarşi

**Ölçüm:** `font-medium` 99 kullanım, `font-semibold` 31, `font-bold` 12. Yani hiyerarşi
neredeyse tamamen **boyutla** kuruluyor, ağırlıkla değil — yoğun tabloda tersi gerekir.

| Rol | Ağırlık |
|---|---|
| Gövde, tablo hücresi | `font-normal` |
| Etiket, tablo başlığı, vurgulanan değer | `font-medium` |
| Bölüm başlığı (`h2`) | `font-semibold` |
| Sayfa başlığı (`h1`) | `font-bold` |

**Aynı hiyerarşi seviyesi iki farklı boyutta yazılmaz.** Bugün `h2` sekiz ekranda
`text-lg font-semibold`, iki ekranda (`yonetim/page.tsx:27,39`) çıplak `font-semibold`
(14px'e miras kalıyor). Bu bir tutarlılık hatasıdır.

### 3.4 Satır uzunluğu ve tracking

| Kural | Değer | Kaynak |
|---|---|---|
| Düz metin (paragraf) satır uzunluğu | **65–75ch** | `craft-floor.md:12` |
| Tablo / yoğun veri | **120ch+ serbest** | `operate.md:16` |
| En negatif `letter-spacing` | **-0.04em** | `craft-floor.md:12` |
| Gerçek metinle test | Her kırılımda **gerçek kopya**, Lorem değil; taşanı düzelt | `craft-floor.md:12` |

### 3.5 🔴 Tabular rakamlar — bu bir para sistemidir

> `font-variant-numeric: tabular-nums`, para / sayı / tarih taşıyan **her** tablo sütununda
> ve her hizalı sayı listesinde **zorunludur**.

`craft-floor.md:15` bunu "modellerin en güvenilir biçimde atladığı şey" olarak işaretler.
Orantılı rakamlarda `1` diğer rakamlardan dardır; alt alta gelen tutarlar hizasız kayar ve
bir bakiye tablosunda bu **en görünür craft hatasıdır**.

**Ölçüm:** `tabular-nums` bugün web genelinde 6 yerde kullanılıyor — **`/yonetim` altında sıfır.**

Biçimleme yine yalnız `web/src/lib/format.ts` içinde kalır (CLAUDE.md #1, #19);
`tabular-nums` bir **CSS sunum kararıdır**, biçimleme değil — tabloyu tanımlayan tek sınıfta
durur, her hücreye tek tek yazılmaz.

---

## 4. Hareket kuralları

### 4.1 Süre bütçesi

İki referans burada çelişir ve çelişki **çözülmüştür**:

| Kaynak | Der ki |
|---|---|
| Emil, bileşen tablosu (`STANDARDS.md:43-46`) | modal/drawer **200–500ms** |
| Emil, üst kural (`STANDARDS.md:49`) | *"UI animations stay under 300ms"* |
| Impeccable genel (`animate.md:58`) | layout/overlay geçişi 300–500ms |
| **Impeccable Operate (`operate.md:41`)** | **"150–250 ms on most transitions"** |

**Kazanan: 150–250 ms.** Gerekçe: Emil'in KENDİ modal reçetesi (`RECIPES.md:88`) zaten
**250ms** yazıyor — tablo 500'e izin verse de reçete 250 seçiyor. Impeccable'ın 300–500ms
satırı mod-nötr; aynı dosyanın `animate.md:10` satırı Operate için kendini zaten sınırlıyor.
Üç kaynak aynı sayıda buluşuyor.

> **`/yonetim` ve `/panel`'de hiçbir geçiş 250ms'i aşmaz.**
> Tek istisna: mobilde parmakla sürüklenerek kapatılan alttan sayfa — `--ease-drawer`, 500ms.

### 4.2 🔴 YÖNETİM PANELİ: neyin animasyon ALMAYACAĞI

Bu tablo, Emil'in sıklık tablosunun (`animate/SKILL.md:33-38`) bu panele uygulanmış halidir.

| Yüzey | Sıklık kademesi | **Karar** |
|---|---|---|
| Sekme / gezinti geçişi | 100+/gün | **Animasyon yok** |
| Tablo satırı hover ve satır girişi | 100+/gün | **Animasyon yok.** Renk değişimi anlık veya `ease` ile ≤150ms |
| Sayfalama (Önceki/Sonraki) | 100+/gün | **Animasyon yok.** İçerik anında değişir |
| Sayfa yükleme / route geçişi | her ekran | **Animasyon yok** — `operate.md:43`: *"No orchestrated page-load sequences."* |
| Liste/kart ızgarası stagger | her ekran | **YASAK.** §4.4 |
| Buton / satır `:active` basma | onlarca/gün | ✅ `transform: scale(0.97)` · `--sure-basma` · `--ease-out` |
| Açılır liste, akordeon, satır genişletme | onlarca/gün | ✅ `--sure-menu` · `--ease-out` · **transition, keyframes değil** |
| Modal / alttan sayfa | ara sıra | ✅ `--sure-katman` · `--ease-out` · `scale(0.96)` + `opacity` |
| Toast | ara sıra | ✅ `--sure-katman` · girdiği kenardan çıkar |
| Boş durum / ilk kurulum ekranı | nadir / ilk kez | ✅ Delight bütçesi **yalnız burada** |

**Basma geri bildirimi — bugün bozuk, düzeltilmesi gereken.**
`ui.tsx:24` şunu yazıyor: `transition-colors active:scale-[0.985]`.
İki hata var: (a) `transform` geçiş listesinde **yok**, bu yüzden ölçek hem basışta hem
bırakışta **anlık sıçrıyor**; (b) `0.985` Emil'in 0.95–0.98 aralığının dışında.
Doğrusu `STANDARDS.md:59`'da birebir yazılı: `transform: scale(0.97)`, `transition: transform 160ms ease-out`.

> 🔴 **Tailwind sınıf sırası tuzağı — belgelenmiş, hâlâ canlı.** `globals.css:287-296`
> uyarısı: `Button` tabanı `transition-colors` yazdığı için üstüne `transition-all`
> **eklemek işe yaramaz**; Tailwind çakışmayı sınıf sırasıyla değil üretilen CSS sırasıyla
> çözer. Butona `className` ile geçiş eklemek **sessizce başarısız olur**. Düzeltme
> `ui.tsx` içindeki taban dizgede yapılmalıdır.

### 4.3 Animasyonlanabilir özellikler — kapalı liste

Referanslar burada da çelişir. `craft-floor.md:13` *"Reach past transform and opacity: blur,
backdrop-filter, clip-path, mask, and shadow…"* der; Emil dört ayrı yerde
*"**Only animate transform and opacity**"* (`emil-design-eng/SKILL.md:479`) der.

**Kazanan: Emil.** Üç gerekçe:
1. Impeccable kendi kendini çürütüyor — `operate.md:47` ürün kısıtlarının ilk maddesi:
   *"Decorative motion that doesn't convey state."* Ve `impeccable/SKILL.md:37`: Operate'te
   ifade en alttadır.
2. **Ölçek.** `talepler` 859 satır, `fiyatlar` 773, `saglayicilar` 714 — yüzlerce satırlık
   tablolar. `blur`, `backdrop-filter` ve `box-shadow` her karede paint tetikler.
   Impeccable'ın kendi şartı zaten *"when they stay smooth"* — 800 satırda kalmazlar.
3. Impeccable'ın kendi denetimi bunu bulgu sayar (`audit.md:28`): *"unbounded blur/filter/shadow
   effects."*

> **`/yonetim` ve `/panel`'de animasyonlanabilir özellikler:**
> `transform` · `opacity` · `clip-path` · `height` (**yalnız** akordeon — transform karşılığı yok)
> Başkası yok. `backdrop-filter`, `box-shadow`, `mask`, `width`, `margin`, `top/left` → **hayır.**

### 4.4 Stagger — yönetimde yasak

Emil `30–80ms` stagger önerir ama kendi reçetesi kapsamı daraltıyor (`RECIPES.md:172`):
*"For a list or grid the user sees occasionally — **not for a list they scroll past all day.**"*
Yönetim tabloları tam olarak ikincisidir. `operate.md:43` de aynı yöne çeker.

> **Yönetim tablolarında ve kart listelerinde stagger yoktur.** Stagger yalnız kullanıcının
> ara sıra gördüğü, ≤6 öğelik bir grupta ve toplam gecikme ≤240ms olacak şekilde meşrudur.

### 4.5 `prefers-reduced-motion` — bugün YANLIŞ uygulanıyor

`globals.css:390-395` şu an global joker seçiciyle her geçişi `0.01ms`'e indiriyor.
Her iki referans da bunu **açıkça bulgu sayar**:

- Emil (`animate/SKILL.md:161`): *"Reduced motion means **fewer and gentler** animations, not zero — keep transitions that aid comprehension, remove movement and position changes."*
- Impeccable (`audit.md:15`): *"flag a global `0.01ms` kill that destroys useful feedback"*

Global kill, `:active` basma geri bildirimini de öldürür — oysa basma geri bildirimi
**kavrayışa hizmet eder**, hareket değildir.

> **Doğru desen:** konum hareketi (translate, slide, parallax) kaldırılır;
> **opacity, renk ve `:active` ölçek geri bildirimi korunur.**

Dosyadaki Türkçe yorum (satır 386-390) global kill'in neden yetmediğini zaten doğru teşhis
etmiş ve `[data-belir]` için hedefli bir düzeltme eklemiş — ama joker kural hâlâ orada.
🔴 Bu düzeltme pazarlama yüzeyini de etkiler; §11.3'e bakınız.

---

## 5. Bileşen sözleşmeleri

### 5.1 Zorunlu durum matrisi

`operate.md:32`: *"Every interactive component has: default, hover, focus, active, disabled,
loading, error. **Don't ship with half of these.**"*

| Bileşen | default | hover | focus-visible | active | disabled | loading | error |
|---|:--:|:--:|:--:|:--:|:--:|:--:|:--:|
| `Button` | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | — |
| `Field` / `Select` / `Textarea` | ✓ | ✓ | ✓ | — | ✓ | — | ✓ |
| Tablo satırı | ✓ | ✓ | ✓ | ✓ | — | — | — |
| `Pagination` | ✓ | ✓ | ✓ | ✓ | ✓ | — | — |
| Toggle / checkbox | ✓ | ✓ | ✓ | ✓ | ✓ | — | ✓ |

Ek zorunlular:
- **Yükleme iskeletle gösterilir, içerik ortasında spinner ile değil** (`operate.md:34`).
  Bugün `ui.tsx` hem `Skeleton` (satır 150) hem `Spinner` (49) sunuyor ve ikisi karışık
  kullanılıyor. **Kural:** içerik alanı → `Skeleton`; buton içi → `Spinner`.
- **Boş durum arayüzü öğretir**, "burada bir şey yok" demez (`operate.md:35`).
- **Açılır katman kabından kaçar** (`operate.md:37`): `overflow-x/y` atası olan bir kapsayıcı
  içindeki mutlak konumlu dropdown **kırpılır**. Tablo kapsayıcıları içindeki her açılır
  öğe bu yüzden test edilir.

### 5.2 Mevcut 9 ilkel — sözleşme ve bilinen eksik

Doküman **var olan** bileşenleri anlatır; hayali bir onuncuyu değil (`document.md` pitfall).

| İlkel | Bugünkü API | Bağlayıcı sözleşme | Bilinen eksik |
|---|---|---|---|
| `Button` | `variant: primary\|ghost\|danger\|outline` · `size: md\|sm` · `loading` · `fullWidth` | Her iki boy `min-h-11` (44px). `loading` iken `aria-busy` + `disabled` | `transform` geçişi yok (§4.2) · ikon-yalnız varyantı yok · `secondary` yok · `href` yok → 5 yerde `<Link><Button>` sarmalaması |
| `Card` | `HTMLAttributes<div>` | `surface rounded-2xl border p-4 md:p-6` | Dolgu/yoğunluk kontrolü yok → mobil satır kartı **16 kez** elle `raised rounded-xl border p-3` yazılmış |
| `Field` | `label` · `error` · `hint` | `htmlFor`+`useId` · `aria-invalid` · `aria-describedby` · `text-base` (16px) | **Yalnız `<input>`.** `<select>`/`<textarea>` kapsam dışı → **21 kez** elle `<label>` sarmalayıcı |
| `Alert` | `tone: bad\|ok\|warn\|info` | — | 🔴 `role="alert"` **sabit yazılı** (ui.tsx:122) ve varsayılan ton `bad`. §7.4 |
| `Badge` | `tone: neutral\|ok\|warn\|bad\|brand` | `whitespace-nowrap` | `className` kabul etmiyor · `Alert`'te `neutral` yok, `Badge`'de `info` yok — **iki ilkel iki farklı sözlük konuşuyor** |
| `Skeleton` | `className` | `aria-hidden` | Satır/tablo iskeleti yok → 4 farklı yükseklik (`h-12/h-20/h-24/h-28`), 4 farklı adet |
| `Empty` | `title` · `hint` | — | **Eylem (CTA) slotu yok** → "Henüz sağlayıcı yok" ekranında "Sağlayıcı ekle" gösterilemiyor |
| `Spinner` | `className` | `aria-hidden` | — |
| `Modal` | `open` · `onClose` · `title` · `children` | Odak tuzağı · `Esc` · iOS kaydırma kilidi + konum geri yükleme · `role="dialog"` `aria-modal` | Alt bilgi (footer) yok · `size` yok · `initialFocus` yok |

**`Badge` genişlik kuralı — ölçülmüş ihlal.** `Badge` `whitespace-nowrap` taşır.
`yonetim/bakiye/page.tsx:161` içine bir **cümle** konmuş:
`"Bu işlem zaten uygulanmıştı — tekrar yazılmadı"`. 320px'te yatay taşma üretir.
> **Rozet bir etikettir, bir cümle değildir.** İçeriği 1–3 kelimeyi aşarsa `Alert`'e taşınır.

### 5.3 🔴 Yazılacak bileşenler — öncelik sırasıyla

Bu liste tahmin değil, **ölçülmüş tekrarın** karşılığıdır. Sayılar `grep` ile doğrulandı.

| # | Bileşen | Bugün kaç kopya | Kanıt |
|---|---|---|---|
| **P0-1** | `DataTable<T>` + mobil kart projeksiyonu | **7 ekran çift render** | `denetim` `destek` `fiyatlar` `kullanicilar` `odeme-yontemleri` `saglayicilar` `talepler` — ~683 satır ikiz kod |
| **P0-2** | `ErrorState` | **11 birebir kopya** | 8 `/yonetim` + 3 `/panel`; `requestId` biçimini değiştirmek 11 dosya demek |
| **P0-3** | `Pagination` | **5 kopya** | `PAGE` sabiti tutarsız: 25 / 20 |
| **P0-4** | `Select` + `Textarea` (Field ailesi) | **5 yerel `Chevron` + 6 `selectClass`** | `globals.css:250` `.select-ok` tam bu tekrarı önlemek için yazılmış — **6 çağrı yerinden yalnız 3'ü kullanıyor** |
| **P0-5** | `StatusBadge` | **6 `statusTone` kopyası** | Dönüş tipleri farklı; biri fonksiyon yerine `Record` |
| **P0-6** | `PageHeader` | **10/10 ekran** | `text-2xl font-bold tracking-tight md:text-3xl` birebir kopyalanmış |
| **P0-7** | `ConfirmDialog` | ~5 | `useDialogFocus` **iki farklı imzayla** iki dosyada |
| P1-8 | `Toolbar` / `FilterBar` | 7 ekran | Etkin süzgeç rozeti yalnız `denetim`'de |
| P1-9 | `SearchInput` | 1 (elle debounce) | `talepler` `denetim` `destek` `yorumlar` da isteyecek |
| P1-10 | `KeyValueList` | 10 `<dl>` | Mobil kart gövdelerinin tamamı |
| P1-11 | `Toast` | **0** | 🔴 Yönetimde `aria-live` **sıfır**; para yazan bir işlemden sonra ekran okuyucuya hiçbir şey duyurulmuyor |
| P1-12 | `CopyButton` | 3 ayrı uygulama | |
| P1-13 | `SkeletonTable` | — | 4 farklı yükseklik yerine tablo şemasından türeyen iskelet |

**Elenenler — eklemeyin:** `Drawer/Sheet` (`Modal` mobilde zaten alttan yükselen sayfa;
`size` prop'u yeter) · `StatCard` (bugün yönetimde metrik karosu yok ve `/admin/orders`
uç noktası **mevcut değil** — veri olmadan kart yapılmaz).

### 5.4 Durum → ton eşlemesi — TEK tablo

Bugün 6 `statusTone` kopyası var, ikisi farklı dönüş tipi kullanıyor, iki ekran haritayı
hiç kurmayıp satır içi `p.isActive ? 'ok' : 'neutral'` yazıyor.

| Anlam | Ton | Örnek |
|---|---|---|
| Tamamlandı, onaylandı, aktif | `ok` | `COMPLETED`, `APPROVED`, `isActive` |
| Bekliyor, dikkat gerektiriyor | `warn` | `PENDING`, `USER_REPLIED` |
| Reddedildi, başarısız, eksik | `bad` | `REJECTED`, `FAILED`, `!hasApiKey` |
| Nötr, kapalı, arşiv | `neutral` | `CLOSED`, `isActive === false` |

🔴 **`brand` tonu bugün beş farklı anlam taşıyor** — durum (`destek:93`), rol
(`kullanicilar:259`), yetenek etiketi (`saglayicilar:175`), önizleme kaynağı (`fiyatlar:535`)
ve sayaç (`denetim:297`). **Aynı renk beş şey söylüyorsa hiçbir şey söylemiyordur.**
Bu ayrımın nasıl yapılacağı §11.6'da açık karar olarak duruyor.

---

## 6. Tablo deseni

Bu projede en çok tekrarlanan yapı budur: **7 tablo, 7 mobil kart ikizi, ~683 satır kopya.**
Ve kopyalama zaten **etiket ayrışması** üretmiş — aynı veri iki yerde farklı yazılmış:

| Ekran | Mobil kart | Masaüstü tablo |
|---|---|---|
| `saglayicilar` | "Anahtar kurulu değil" | "Kurulu değil" |
| `saglayicilar` | "Katalogu senkronla" | "Senkronla" |
| `talepler` | "İncele ve karar ver" | "İncele" |

Tek bir `DataTable` bunu **yapısal olarak imkânsız** kılar.

### 6.1 Bağlayıcı kurallar

1. **`< md` altında tablo yoktur — kart vardır.** (CLAUDE.md #17 · `frontend-contract.md §2.5`)
   `overflow-x: auto` bir çözüm değil, bir ertelemedir.
   ✅ **Bugün 7/7 tablo bu kurala uyuyor** ve `/yonetim` altında `overflow-x` **0 kullanım**.
   Bu panelin en sağlam yanıdır; **bozulmamalıdır**.
2. **Aynı veri iki kez elle yazılmaz.** Sütun tanımı tek kaynaktır; kart sunumu ondan türer.
3. **Sütun önceliği veriden gelir, ekran yazarının insafına bırakılmaz.** Her sütun bir
   `oncelik` taşır: `1` = her zaman görünür + mobil kartın üst satırı · `2` = `md:` üstü ·
   `3` = `lg:` üstü.
   🔴 **Ölçülmüş risk:** `saglayicilar` 8 sütun, `fiyatlar` 7, `kullanicilar` 7 ve
   **hiçbirinde `lg:table-cell` gizlemesi yok**; oysa `talepler` (7→5) ve
   `odeme-yontemleri` (6→4) gizliyor. 768px'te 8 sütun, yatay kaydırma yasağının sınırındadır.
4. **Para, sayı ve tarih sütunları `tabular-nums` taşır** ve sağa hizalanır (§3.5).
5. **Yükleniyor / boş / hata üçü de zorunlu proptur** — opsiyonel değil.

### 6.2 Başlıklar ve erişilebilirlik

| Kural | Bugünkü durum |
|---|---|
| `<th scope="col">` her başlıkta | ✅ 46 adet, 7/7 tabloda |
| `<caption>` her tabloda | 🔴 **0/7.** Ekran okuyucu tablonun ne olduğunu bilmiyor. Görsel olarak gizlenebilir (`sr-only`) ama **var olmalıdır** |
| Sıralanabilir sütunda `aria-sort` | 🔴 Yok (sıralama da yok) |
| Satır eylemleri hover'a bağlanmaz | `frontend-contract.md §2.3`: dokunmatikte hover yoktur |

### 6.3 Sıralama, seçim, sayfalama

**Ölçüm: 10 ekranın hiçbirinde toplu işlem, satır seçimi, sütun sıralaması veya dışa
aktarma yok.** Denetim kaydı ve bakiye talepleri gibi kuyruklarda tek gezinme aracı
"Önceki / Sonraki".

| Yetenek | Kural |
|---|---|
| **Sıralama** | Başlık bir `<button>`'dır (44px), `aria-sort` taşır. Sıralama **sunucuda** yapılır — istemcide yeniden sıralamak sayfalanmış veriyi yanlış gösterir |
| **Seçim** | Toplu işlem varsa: satır checkbox'ı + başlıkta "tümünü seç" + seçim sayısını gösteren bir çubuk. Seçim **sayfa değişince temizlenir** (görünmeyen bir seçimle işlem yapılmaz) |
| **Sayfalama** | Tek bileşen, tek `limit`. Bugün `PAGE` sabiti **25 / 20** olmak üzere iki farklı değerde. Sayfa değişimi `aria-live="polite"` ile duyurulur |
| **Boş durum** | Süzgeçten dolayı boşsa "sonuç yok + süzgeci temizle" eylemi; gerçekten boşsa "ilk kaydı oluştur" eylemi. İkisi **farklı** metinlerdir |

### 6.4 Modal yerine satır içi

`operate.md:52`: *"Modal as first thought. Modals are usually laziness.
**Exhaust inline / progressive alternatives first.**"*
`craft-floor.md:30`: kesintiye de korunmuş odağa da ihtiyaç duymayan bir görev için modal reddedilir.

> **Modal yalnız iki durumda:** (a) yıkıcı/geri alınamaz işlem onayı, (b) korunmuş odak
> gerektiren çok adımlı form. Satır düzenleme, durum değiştirme, hızlı not → **satır içi**.

---

## 7. Erişilebilirlik tabanı

`docs/frontend-contract.md §6`'nın üstüne ölçülebilir eşikler ekler; onunla çelişmez.

### 7.1 Kontrast

| Ne | Eşik | Kaynak |
|---|---|---|
| Gövde metni **ve placeholder** | **≥ 4.5:1** | `craft-floor.md:9` |
| Büyük metin (≥18.66px bold / ≥24px) | ≥ 3:1 | `craft-floor.md:9` |
| Kontrol kenarlığı, ikon, **odak halkası** | ≥ 3:1 | `craft-floor.md:9` |
| Devre dışı içerik | Ölç ve kaydet | — |

- **Placeholder en sık atlanan yerdir** ve gövde metniyle aynı eşiktedir.
- **Renkli zeminde ikincil metin gri yapılmaz**; o hue'dan veya foreground'dan türetilir.
- **Anlam yalnız renkle taşınmaz.** ✅ Bugün uyuluyor: her rozet metin de taşıyor.

### 7.2 Odak halkası

`globals.css:201-205` bugün doğru: `outline: 2px solid var(--color-brand-400)`,
`outline-offset: 2px`, `border-radius: 4px`.

> **`outline: none` tek başına hiçbir yerde yazılmaz.** Kaldırılan her odak halkasının
> yerine görünür bir alternatif konur. `Field` bunu doğru yapıyor (`outline-none` +
> `focus:border-brand-400`) — ama kenarlık rengi tek başına 3:1 eşiğini karşılamalıdır.

### 7.3 Dokunma ve klavye

| Kural | Değer | Bugünkü durum |
|---|---|---|
| Dokunma hedefi | ≥ 44×44px | ✅ `Button` `min-h-11`, select `min-h-12`, modal kapat `size-11`, nav sekmesi 44px |
| Hedefler arası boşluk | ≥ 8px | `frontend-contract.md §2.3` |
| Girdi font boyutu | ≥ 16px | ✅ `globals.css:193` global kural |
| Klavye ile tam akış | Zorunlu | ✅ `Modal` odak tuzağı + `Esc` + panel dışı odağı geri çekme |
| Atlama bağlantısı | Her sayfada ilk odak | ✅ `layout.tsx:81` — ama z-index çakışması var (§2.5) |
| `<select>` yüksekliği | `appearance: none` **zorunlu** | 🔴 WebKit yerel `menulist` `min-h-12`'yi **yok sayar** — ölçümde 25px. `globals.css:236-249` bunu belgeliyor; **6 çağrı yerinden 3'ü `.select-ok` kullanmıyor** |

### 7.4 Ekran okuyucu — iki ölçülmüş hata

🔴 **`role="alert"` aşırı kullanımı.** `ui.tsx:122` `Alert`'e `role="alert"` **sabit** yazmış.
Ölçüm: `/yonetim` altında **27 bilgi/uyarı kutusu** (`tone="info"` 11 + `tone="warn"` 16) bu
rolü taşıyor ve açılışta render ediliyor. Ekran okuyucu, sayfa yüklenir yüklenmez statik
açıklamaları **kesintili uyarı** olarak okuyor.
> **Kural:** `role="alert"` yalnız **yeni beliren** hata/başarı içindir. Statik bilgi kutusu
> `role` almaz. Bu, `Alert`'e bir `duyur?: boolean` prop'u ile çözülür.

🔴 **`aria-live` hiç yok.** Ölçüm: `/yonetim` altında **0 kullanım**. Liste süzgeçten sonra
yenilendiğinde, sayfa değiştiğinde veya bir mutasyon başarıldığında hiçbir şey duyurulmuyor.
> **Kural:** Liste sonucu değişimi ve mutasyon sonucu `aria-live="polite"` ile duyurulur.
> `frontend-contract.md §6` bunu SSE için zaten zorunlu kılıyor; yönetim listelerinde de
> karşılığı olmalıdır.

### 7.5 Yıkıcı onay — odak tutarlılığı

🔴 **Bugün çelişkili.** `talepler:588,668` ve `odeme-yontemleri:686` odağı **"Vazgeç"e**
koyuyor ve gerekçesini yazıyor (*"Enter basılı kalırsa tek tuş parayı yazar"*).
Ama `kullanicilar:392`, `fiyatlar:721`, `fiyatlar:758` odağı **yıkıcı düğmeye** koyuyor.
Aynı risk, üç ekranda tersi uygulanmış — hem de para ekranlarında.

> **Kural: Yıkıcı veya para yazan bir onay diyaloğunda ilk odak her zaman "Vazgeç"tedir.**
> Buton düzeni de tek olmalıdır (bugün 5 ekran `sm:flex-row-reverse`, 2 ekran düz `flex-col`).

### 7.6 `prefers-reduced-motion`

§4.5'e bakınız. Özet: **az ve yumuşak, sıfır değil.** Konum hareketi gider, geri bildirim kalır.

---

## 8. Tarayıcı yüzeyleri

> *"the parts you did not draw still carry the design… **This is the cheapest signal that a
> page was built rather than assembled, and the one models skip most reliably.**"*
> — `craft-floor.md:15`

Altı yüzey, altı ölçülebilir test. Bugünkü durum **kod okunarak doğrulandı**:

| # | Yüzey | Bugün | Kural |
|---|---|---|---|
| 1 | **Seçim rengi** | ✅ Var — `globals.css:207` `::selection { background: var(--color-brand-500); color: #fff }` | Korunur. Seçili metin kontrastı iki temada da ≥4.5:1 olmalı |
| 2 | **Caret (imleç)** | 🔴 **Yok** — `caret-color` 0 kullanım | Her `input`/`textarea` için `caret-color` tanımlanır; varsayılan siyah caret koyu temada görünmez |
| 3 | **Kaydırma çubuğu** | ⚠️ Kısmi — `.thin-scroll` (`globals.css:230`) var ama **opt-in** ve yalnız 2 yerde kullanılıyor | Tablo gövdesi, modal gövdesi ve kenar çubuğu gibi **her iç kaydırma alanında** uygulanır |
| 4 | **Odak halkası** | ✅ Var — `globals.css:201` | §7.2 |
| 5 | **Alt çizgi ofseti** | ⚠️ Elle — `underline-offset-4` bileşenlerde tek tek yazılıyor | Metin içi bağlantı için tek bir varsayılan; her `<Link>`'e elle yazılmaz |
| 6 | **Tabular rakamlar** | 🔴 **`/yonetim` altında 0 kullanım** | §3.5 — **bir para panelinde en görünür craft hatasıdır** |

🔴 **Temalandırmak ≠ yeniden icat etmek.** Impeccable kendi içinde çelişiyor:
`craft-floor.md:15` kaydırma çubuğunu "paletten temalandır" derken `operate.md:50`
"özel kaydırma çubuğu" yasaklıyor. Ayrım şudur:

| İZİN VAR (temalandırma) | YASAK (yeniden icat) |
|---|---|
| `scrollbar-color`, `scrollbar-width: thin` | JS tabanlı özel scrollbar kütüphanesi |
| `::-webkit-scrollbar-thumb` rengi | Scrollbar'ı gizleyip kendi sürgününü çizmek |
| `caret-color`, `::selection` | Özel imleç / özel seçim katmanı çizmek |
| — | Kaydırma davranışını (momentum, tıklama) yeniden yazmak |
| — | `scrollbar: display:none` ile hiç göstermemek |

**Kural cümlesi:** Kaydırma çubuğunun **renkleri** paletten gelir; **genişliği, geometrisi ve
davranışı** işletim sisteminden gelir.

---

## 9. YASAKLAR

`CLAUDE.md`'nin "Asla yapma" listesinin tasarım karşılığıdır. Aynı bağlayıcılıktadır.

### 9.1 Projenin kendi kısıtları (CLAUDE.md · frontend-contract.md)

- ❌ `max-md:` / `max-sm:` ile masaüstünden mobile geri alma zinciri *(✅ bugün 0 kullanım — bozmayın)*
- ❌ Mobilde yatay kaydırılan tablo *(✅ bugün `/yonetim`'de `overflow-x` 0 kullanım)*
- ❌ 16px'ten küçük fontlu girdi alanı (iOS yakınlaşır)
- ❌ `100vh` *(→ `min-h-screen-safe` / `h-screen-safe`)*
- ❌ 44px'ten küçük dokunma hedefi
- ❌ Bileşende doğrudan `fetch` *(→ `web/src/lib/api.ts`)*
- ❌ İstemcide para aritmetiği · para/tarih biçimlemesini `lib/format.ts` dışında yapmak
- ❌ Yerel geri sayım sayacı *(→ sunucudaki `expiresAt`)*
- ❌ Yükleniyor/boş/hata üçlüsünden birini atlamak; hatada `requestId`'yi gizlemek
- ❌ `appearance: none` olmadan `<select>` *(→ `.select-ok`)*

### 9.2 Impeccable "Refuse" listesinden bu projeye uyanlar

| Yasak | Kaynak | Bu projede karşılığı |
|---|---|---|
| ❌ **Kart ızgarasını sayfa iskeleti yapmak.** "Cards are the lazy container; **nested cards are always wrong**" | `craft-floor.md:25` | `/yonetim` ana sayfası bugün tam olarak budur. **İç içe kart mutlak yasak** — tablo kartının içinde ikinci kart olamaz |
| ❌ **Kicker / eyebrow** — başlığın üstündeki küçük tracked etiket. *"This one is a **ban**, not a default: no brief earns it back"* | `craft-floor.md:27` | Sayfa başlığının üstüne "YÖNETİM" tarzı etiket **konulmaz**. *(Tablo sütun başlığı, form etiketi, durum rozeti bunun dışındadır — onlar veri etiketidir)* |
| ❌ **Gradyan metin.** "Emphasis comes from weight or size" | `craft-floor.md:33` | `/yonetim`'de vurgu = ağırlık veya boyut |
| ❌ **Süs amaçlı glass/blur** | `craft-floor.md:34` | `backdrop-filter` yalnız gerçekten yüzen katman için (yapışkan başlık, modal örtüsü) |
| ❌ **1px üstü renkli `border-left`** (kart, liste öğesi, uyarı) | `craft-floor.md:35` | `globals.css:359` `.servis-listesi` 3px kullanıyor — pazarlamada kalır, **yönetime taşınmaz** |
| ❌ **Sert offset gölge** (`4px 4px 0`) | `craft-floor.md:36` | — |
| ❌ **İçerik yerine dekoratif sparkline / progress ring** | `craft-floor.md:37` | Yönetim panelinin en sık tuzağı. Bir mini grafik ancak **okunabilir ve tıklanabilir** bir veriyse kalır |
| ❌ **"Teknik dursun" diye monospace** | `craft-floor.md:38` | Monospace yalnız: `public_id` (UUID), `requestId`, API anahtarı, IBAN. Başlıklarda değil |
| ❌ **Emoji / unicode karakteri ikon yerine** | `craft-floor.md:40` | ✅ ⚠️ 🔴 ✓ × → durum göstergesi olarak **kullanılamaz**. Tek kütüphane, tek stroke genişliği. *(`denetim/page.tsx:352` ham `▾` karakteri kullanıyor — diğer ekranların SVG `Chevron`'undan görsel olarak ayrışıyor)* |
| ❌ **Kesintiye ihtiyaç duymayan görev için modal** | `craft-floor.md:30` · `operate.md:52` | §6.4 |
| ❌ **Ekranlar arası tutarsız bileşen dili.** *"If the 'save' button looks different in two places, one is wrong"* | `operate.md:48` | Bugün: 2 farklı select stili, 6 durum haritası, 5 farklı içerik genişliği, 2 onay-butonu düzeni |
| ❌ **Etiket, buton ve veride display fontu** | `operate.md:49` | Tek aile: `var(--font-sans)` |
| ❌ **Standart affordance'ı "tat" için yeniden icat etmek** | `operate.md:50` | §8'deki ayrım tablosu |
| ❌ **Pasif durumlarda tam doygunlukta renk** | `operate.md:51` | Devre dışı öğe `--muted` ve azaltılmış opaklık kullanır |

### 9.3 Hareket yasakları (Emil "Never Ship", `animate/SKILL.md:171-185`)

- ❌ `transition: all` → özellikleri tek tek yaz
- ❌ `scale(0)`'dan giriş → `scale(0.95–0.97)` + `opacity: 0`
- ❌ Arayüzde `ease-in` → `--ease-out`
- ❌ Klavye tetikli veya günde 100+ eylemde animasyon → **animasyon yok**
- ❌ Gerekçesiz 250ms üstü geçiş (Operate) → §4.1
- ❌ Tetikleyiciye bağlı popover'da `transform-origin: center` → `var(--transform-origin)` *(modal muaftır, merkezde kalır)*
- ❌ Hızlı tetiklenen öğede `@keyframes` → `transition` *(keyframes sıfırdan başlar; transition mevcut değerden yeniden hedeflenir, yani **kesintiye uğrayabilir**)*
- ❌ `width`/`height`/`margin`/`padding`/`top`/`left` animasyonu → `transform`/`opacity`
- ❌ Kapısız `:hover` hareketi → `@media (hover: hover) and (pointer: fine)`
  *(🔴 `globals.css:302,324` bugün yalnız `(hover: hover)` kullanıyor — `pointer: fine` eksik)*
- ❌ `prefers-reduced-motion`'ı sıfıra indirmek → §4.5
- ❌ Ebeveyndeki CSS değişkeniyle çocuk transform'u sürmek *(tüm çocuklarda stil yeniden hesabı)*

---

## 10. Uygulama kontrol listesi

Yeni bir `/yonetim` veya `/panel` ekranı **bitti** sayılmadan önce, tek tek işaretlenir.
`frontend-contract.md §9` listesinin üstüne gelir, onun yerine geçmez.

### Yerleşim ve responsive
- [ ] Mobil taban stiller yazıldı, `md:`/`lg:` **sonra** eklendi — `max-*` zinciri yok
- [ ] 320 / 390 / 430 / 768 / 1440 px'te yatay kaydırma yok
- [ ] Tablo `< md` altında karta dönüşüyor; `overflow-x` yok
- [ ] Tüm dokunma hedefleri ≥ 44×44px, aralarında ≥ 8px
- [ ] `100vh` yok (`min-h-screen-safe`), alt sabit öğelerde `pb-safe` var
- [ ] İçerik genişliği panelin geri kalanıyla aynı *(bugün 672px–1152px arası zıplıyor)*

### Tipografi ve boşluk
- [ ] Veri taşıyan hiçbir metin 14px altında değil (`text-xs` yalnız rozet/dipnot)
- [ ] Para / sayı / tarih sütunları `tabular-nums` taşıyor ve sağa hizalı
- [ ] Paragraf satır uzunluğu 65–75ch
- [ ] Başlık üstü boşluk, başlık altı boşluktan **büyük** (DevTools'tan okundu, tahmin değil)
- [ ] Yarım boşluk basamağı (`gap-1.5`, `px-3.5`…) kullanılmadı
- [ ] `h2` her yerde aynı boyutta (`text-lg font-semibold`)

### Durumlar
- [ ] Yükleniyor **iskeletle** tasarlandı (içerik ortasında spinner yok)
- [ ] Boş durum arayüzü **öğretiyor** ve bir eylem sunuyor
- [ ] Hata durumu `requestId` gösteriyor
- [ ] Her etkileşimli bileşende 7 durum: default/hover/focus/active/disabled/loading/error
- [ ] Süzgeçten dolayı boş ile gerçekten boş **farklı** metinler

### Hareket
- [ ] Hiçbir geçiş 250ms'i aşmıyor
- [ ] Yalnız `transform` / `opacity` (+ akordeonda `height`) animasyonlanıyor
- [ ] Eğri üç onaylı jetondan biri — yaklaştırılmış `cubic-bezier` yok
- [ ] `ease-in` yok, `transition: all` yok, `scale(0)` yok
- [ ] Sayfa girişi / stagger / route geçiş animasyonu yok
- [ ] `:active` basma geri bildirimi var (`scale(0.97)`, 160ms)
- [ ] Hover hareketi `@media (hover: hover) and (pointer: fine)` içinde
- [ ] `prefers-reduced-motion`'da konum hareketi gitti, geri bildirim kaldı

### Erişilebilirlik
- [ ] Tabloda `<caption>` var, `<th scope="col">` var
- [ ] `role="alert"` yalnız yeni beliren hata/başarı için; statik bilgi kutusunda **yok**
- [ ] Liste yenilenmesi ve mutasyon sonucu `aria-live="polite"` ile duyuruluyor
- [ ] Yıkıcı onay diyaloğunda ilk odak **"Vazgeç"te**
- [ ] `<select>` `.select-ok` kullanıyor (WebKit 25px tuzağı)
- [ ] Kontrast ölçüldü: gövde ve **placeholder** ≥4.5:1, kontrol/odak ≥3:1
- [ ] Klavye ile tüm akış tamamlanabiliyor; odak görünür
- [ ] Anlam yalnız renkle taşınmıyor

### Tarayıcı yüzeyleri (§8)
- [ ] `caret-color` tanımlı
- [ ] İç kaydırma alanlarında `.thin-scroll`
- [ ] `tabular-nums` sayısal sütunlarda

### Tutarlılık
- [ ] `ErrorBox` / `Pagination` / `statusTone` / `Chevron` **kopyalanmadı** — ortak bileşen kullanıldı
- [ ] Emoji ikon yerine SVG kullanıldı
- [ ] Aynı işi yapan buton diğer ekranlardakiyle aynı görünüyor
- [ ] Mobil kart ile masaüstü tablo **aynı etiketleri** kullanıyor

### Test
- [ ] `webkit-mobile` (iPhone 14) Playwright projesinde geçiyor
- [ ] `web/scripts/responsive-check.mjs` `PRIVATE_PAGES` listesinde bu yol var
      *(🔴 bugün `/yonetim/destek` ve `/yonetim/yorumlar` listede **yok** — ikisi de modal içinde form barındırıyor)*
- [ ] Gerçek bir telefonda elle açıldı

---

## 11. 🔴 AÇIK KARARLAR — kullanıcı onayı gerekiyor

Bu maddeler bir tasarım tercihi değil, **kapsamı bu dokümanı aşan** kararlardır. Karar
verilmeden ilgili kodda değişiklik yapılmamalıdır. Karar verildiğinde §12'deki yordamla
buraya ve `docs/memory.md` §1'e yazılır.

### 11.1 `web/messages/tr.json` yok — Değişmez #14 fiilen uygulanmıyor
**Ölçüm:** `web/messages/` dizini **mevcut değil**, `next-intl` `package.json`'da **kurulu
değil**. `/yonetim` altında Türkçe metinler bileşenlere gömülü.
CLAUDE.md Değişmez #14 ve `frontend-contract.md §9` ikisi de sözlüğü zorunlu kılıyor.
**Karar gerekiyor:** (a) `next-intl` kurulup metinler çıkarılsın mı, (b) yoksa #14 gerçeğe
göre revize mi edilsin? Bu, 10 ekranı yeniden yazan her tasarım işini doğrudan etkiler.

### 11.2 `ui.tsx` üç yüzeyi birden besliyor — değiştirilebilir mi?
**Ölçüm:** `Button` 139 kullanım (77 yönetim / 35 panel / 27 pazarlama+kimlik),
`Card` 63 kullanım (**31'i pazarlama**, 16 yönetim).
Yönetimi yoğunlaştırmak için `Button`'ı küçültmek pazarlama sayfasındaki 27 düğmeyi de
küçültür. **Karar gerekiyor:** `ui.tsx` ortak kalıp yalnız **katkı yapan** (mevcut davranışı
değiştirmeyen) proplar mı eklensin, yoksa `components/yonetim/` altında ayrı bir katman mı
kurulsun?

### 11.3 `prefers-reduced-motion` global kill'i — kapsamı ne olsun?
§4.5'te gerekçesi yazılı: `globals.css:390-395` her iki referansın da bulgu saydığı desen.
Ama düzeltme pazarlama yüzeyini de etkiler. **Karar gerekiyor:** global mi düzeltilsin,
yoksa yalnız `/yonetim` + `/panel` kapsamında hedefli kural mı yazılsın?

### 11.4 `text-xs` → `text-sm` göçü — görsel yoğunluk değişir
§3.2'deki kural **100 kullanımı** etkiler ve panelin görsel yoğunluğunu değiştirir
(satırlar uzar, sayfa uzar). Referanslar ve Operate modu bunu destekliyor, ama sonuç
kullanıcının kendi ekranında görülmeden onaylanmamalı. **Karar gerekiyor:** göç yapılsın mı,
yapılacaksa tek seferde mi ekran ekran mı?

### 11.5 Gezinti: 10 sekmelik kaydırma şeridi mi, yan menü mi?
`admin-shell.tsx:82-83`'ün **kendi yorumu** *"2-3 sekme için kabul edilebilir"* diyor ama
`NAV` dizisinde **10 sekme** var. Aynı depoda `panel-shell.tsx` daha az bölüm için
`md:` yan menü + mobilde alt gezinti kullanıyor — yani yönetim paneli **daha fazla bölümle
daha zayıf desene** sahip. `operate.md:56` "top bar + side nav"a açıkça izin veriyor.
**Karar gerekiyor:** `admin-shell.tsx` yan menüye geçsin mi? Bu yapısal bir değişikliktir.

### 11.6 `Badge tone="brand"` beş anlam taşıyor — nasıl bölünsün?
§5.4'te ölçüm var: durum / rol / yetenek etiketi / önizleme kaynağı / sayaç.
**Karar gerekiyor:** kaç ayrı ton olsun ve adları ne olsun? (Öneri: `state` · `tag` · `count`
— ama isimlendirme ürün diline aittir, tek başıma karar vermedim.)

### 11.7 Koyu tema varsayılanı — kullanım sahnesinden mi geliyor?
`frontend-contract.md §2.7` koyu temayı varsayılan yapıyor. `craft-floor.md:42` ise
*"Light or dark picked by category"*'yi reddediyor: **kullanım sahnesinden seçilmeli** —
kim, nerede, hangi ortam ışığında? Yönetim paneli operatörü gün içinde ofiste mi, gece mi
çalışıyor? Bu bilgi bende yok. **Karar gerekiyor** — ya da mevcut karar bu gerekçeyle
yazılı olarak onaylanmalı.

### 11.8 `toMinor` üç farklı davranışta — hangisi doğru?
**Ölçüm:** 5 kopya (`bakiye:20`, `fiyatlar:64`, `odeme-yontemleri:58`, `talepler:65`,
`panel/bakiye-yukle:54`), **üç farklı doğrulama**: boş girdide biri hata verir,
`fiyatlar` sessizce `{minor: 0}` döner; negatifi biri kabul eder, diğerleri reddeder.
Aynı görünen "Tutar (TL)" kutusu ekrana göre farklı doğruluyor.
🔴 **Bu bir tasarım kararı değil, bir para sistemi kararıdır** (CLAUDE.md: "Şüphede kaldığında
dur ve sor"). Doğru davranış belirlenip `lib/format.ts`'e taşınmalıdır.

### 11.9 CLAUDE.md'de iki yol yanlış
- Değişmez #15 `web/src/lib/api/client.ts` diyor; gerçek dosya **`web/src/lib/api.ts`**.
- "Para/tarih biçimleme yalnız `web/src/lib/format` içinde" — bu bir klasör değil,
  **`web/src/lib/format.ts`** dosyası.
İçerikleri sözleşmeye **uygun** (`formatMoney` sunucunun `formatted` alanını önceliyor,
`formatDateTime` `timeZone: 'Europe/Istanbul'` veriyor). Yalnız doküman yolu yanlış.
**Karar gerekiyor:** CLAUDE.md düzeltilsin mi, dosyalar mı taşınsın?

---

## 12. Bu doküman nasıl güncellenir

### 12.1 Doğruluk kaynağı zinciri

```
web/src/app/globals.css   ←  DOĞRULUK KAYNAĞI (gerçek kod, ölçülebilir)
        ↓
tasarim-sistemi.md (frontmatter)  ←  taşınabilir dışa aktarım
        ↓
tasarim-sistemi.md (§1-§10)       ←  nerede ve NEDEN kullanılacağı
```

**Frontmatter normatiftir; düzyazı bağlam verir.** Aynı sayı iki yerde farklı yazılamaz.
Bir jeton `globals.css`'te değişirse frontmatter **aynı commit'te** güncellenir.

### 12.2 Ne zaman güncellenir

| Olay | Yapılacak |
|---|---|
| `globals.css`'e jeton eklendi/değişti | Frontmatter'ı güncelle, `durum:` alanını `mevcut` yap |
| Yeni bir ortak bileşen yazıldı | §5.2 tablosuna satır ekle, §5.3'ten çıkar |
| §11'deki bir açık karar verildi | Kararı ilgili bölüme **kural olarak** yaz, §11'den sil, `docs/memory.md` §1'e karar günlüğü girdisi ekle |
| Bir kural ihlali üretimde yakalandı | Kuralı sertleştir — **dar bir düzeltme**, evrensel yeni kural değil |
| Referans skill'leri güncellendi | Alıntılanan satır numaralarını doğrula |

### 12.3 Yeni kural yazma kuralları

1. **Ölçülebilir olacak.** "Daha ferah dursun" değil, "başlık üstü boşluk başlık altından büyük".
2. **Kaynağı olacak.** Ya mevcut koddan ölçülmüş (dosya:satır) ya referanstan alıntı (dosya:satır).
   **Uydurulmuş sayı yazılmaz** — bilinmiyorsa "belirtilmemiş" diye işaretlenir.
3. **Belirginlik riske eşlenecek.** Renk seçiminde model serbest bırakılabilir;
   `tabular-nums`, `44px`, `16px girdi`, `250ms` gibi maddelerde **mutlak dil** kullanılır.
4. **Bölüm başlıkları değiştirilmez.** Başlıklar bu dosyanın API'sidir; başka dokümanlar
   ve kontrol listeleri onlara referans verir.
5. **Var olmayan bileşen anlatılmaz.** §5.2 bugün kodda olan 9 ilkeli anlatır;
   yazılacaklar §5.3'te ayrı durur. İkisi karıştırılmaz.
6. **Genel tavsiye eklenmez.** Bir cümle bir sonraki ajanın kararını değiştirmiyorsa silinir.

### 12.4 Doküman gerçekten işe yarıyor mu — ileri test

Doğrulama, kelime eşleştirmesiyle değil **davranışla** yapılır: bağımsız bir ajana bu
doküman verilir, gerçekçi bir istek yapılır ve **beklenen cevap söylenmez**.

> Örnek test: *"Yönetim panelinde sipariş iptal onayı ekle."*
> Ajan modal mı seçiyor yoksa satır içi mi? İlk odağı "Vazgeç"e mi koyuyor?
> Geçiş 250ms'i aşıyor mu? Tutar sütununa `tabular-nums` koyuyor mu?
> Metni `text-xs` mi yazıyor?

Doküman bunları **zorlamıyorsa doküman eksiktir** — ajan değil.

---

**Bu dokümanı yazarken hiçbir kod dosyası değiştirilmedi.** Tüm tespitler `grep`/`wc` ile
ölçüldü; `/yonetim` ekranları giriş + reCAPTCHA arkasında olduğu için tarayıcıda
görüntülenemedi ve analiz **kod okunarak** yapıldı.
