---
baslik: Sanal numara nasıl çalışır?
ozet: Sanal numaranın arkasında hangi donanım var, mesaj panele nasıl düşüyor ve numaranın süresi neden sınırlı.
etiket: Teknik
renk: mor
tarih: 2026-09-09
okuma: 5
---

**Sanal numara**, bir SIM karta ya da operatör hattına bağlı olan ama fiziksel
bir telefonun içinde durmayan numaradır. Gelen mesaj, bir cihazın ekranı
yerine bir yazılıma düşer; siz de o yazılımın arayüzünden okursunuz.

## Arkasında ne var?

Yaygın olarak iki kurulum görülür:

- **SIM havuzu (SIM bank).** Yüzlerce gerçek SIM kart, çok yuvalı bir donanıma
  takılıdır. Kart bir GSM ağına normal telefon gibi bağlanır; gelen SMS
  donanım tarafından okunup bir sunucuya aktarılır.
- **Operatör/toplu SMS anlaşması.** Numara aralığı doğrudan bir operatörden
  ya da bir mesajlaşma altyapısından kiralanır. Gelen mesaj bir arayüz
  (webhook) çağrısıyla sunucuya iletilir.

İki kurulumda da numara **gerçektir**. "Sanal" olan, numaranın size ulaşma
biçimidir — bir cihaz değil, bir hesap üzerinden.

## Mesaj panele nasıl düşüyor?

Numara size tahsis edildiği anda, o numaraya gelen mesajlar sizin siparişinize
bağlanır. Mesaj geldiğinde:

1. Sağlayıcı bir bildirim gönderir: "şu numaraya mesaj geldi".
2. Sunucu bu bildirimi bir **tetikleyici** olarak kabul eder, içindeki koda
   doğrudan güvenmez; kodu sağlayıcıdan ayrıca sorup teyit eder.
3. Teyit edilen kod siparişe yazılır ve açık olan bekleme ekranına iletilir.

İkinci adım önemsiz görünür ama değildir: bildirim uç noktasının adresini
bilen herkes sahte bir "kod geldi" mesajı gönderebilir. Kodun kaynağı her
zaman sağlayıcının kendi kaydıdır.

## Numaranın süresi neden sınırlı?

Tek kullanımlık numarada süre, numaranın havuzdaki dolaşımını sürdürmek
içindir. Numara size süresiz kalsaydı havuz kısa sürede tükenir, kimse yeni
numara alamazdı. Süre dolduğunda numara havuza döner ve bir süre bekletildikten
sonra başka bir doğrulama için kullanılabilir hale gelir.

Bu, sanal numaranın en çok yanlış anlaşılan yanıdır: numara **size ait**
değildir, o doğrulama boyunca **size tahsis edilmiştir**.

## Nelere uygun değildir?

Dürüst olmak gerekirse sanal numaranın uygun olmadığı yerler var:

- **Kalıcı hesap kurtarma.** Numara sizde kalmadığı için, ileride şifrenizi
  unuttuğunuzda o numaraya erişemezsiniz. Uzun ömürlü bir hesabı tek
  kullanımlık numaraya bağlamak riskli bir tercihtir; bunun için
  [numara kiralama](/kiralama) daha uygundur.
- **Bankacılık ve resmî işlemler.** Bu kurumlar kimliğe bağlı hattı ister ve
  çoğu zaten sanal numara aralıklarını reddeder.
- **Kullanım şartlarını yasaklayan servisler.** Bazı servislerin şartları
  sanal numarayı açıkça yasaklar; bu durumda hesap kapatılabilir.
