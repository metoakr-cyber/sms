import Link from 'next/link';
import type { Metadata } from 'next';
import { Card, Alert } from '@/components/ui';

export const metadata: Metadata = {
  // Sayfaya ÖZEL description şart: yoksa kök layout'un varsayılanı miras
  // alınır ve iki ayrı sayfa birebir aynı açıklamayı taşır (S1 ihlali).
  //
  // Metin, sayfanın GERÇEK içeriğini anlatır: eski açıklama yalnız silme
  // süresinden söz ediyordu, sayfa ise aktarım ve haklar da anlatıyor.
  // ≤160 karakter: seo-check.mjs bunu ÖLÇÜYOR ve uzunu düşürüyor. Uzun
  // açıklama arama sonucunda zaten kırpılır; kırpılan yer cümlenin ortasıdır.
  description:
    'Onay360 gizlilik politikası: hangi verileri neden işliyoruz, ' +
    'kimlerle paylaşıyoruz, ne kadar saklıyoruz ve haklarınızı nasıl kullanırsınız.',
  title: 'Gizlilik Politikası',
  alternates: { canonical: '/gizlilik' },
};

/**
 * 🔴 BU METİN YÜRÜRLÜKTEKİ BİR BELGEDİR — taslak uyarısı YOKTUR.
 *
 * Bu yüzden buradaki her cümle SİSTEMİN BUGÜNKÜ DAVRANIŞIDIR ve koda
 * bakılarak doğrulanabilir. Karşılığı olmayan hiçbir taahhüt yazılmadı;
 * yazılamayanlar `YOK_SAYILANLAR` içinde sayfanın kendisinde de listelenir.
 *
 * Maddelerin dayanağı (örnekler):
 *   2 · veri kategorileri   → api/migrations/00001_init_identity.sql (users,
 *                             sessions, audit_logs) · 00003_ledger.sql ·
 *                             00008_orders.sql (orders, order_messages,
 *                             deposits) · 00012_tickets.sql · 00013_reviews.sql
 *   2.4 sahiplik            → api/internal/transport/http/router.go (auth grubu),
 *                             handler/order.go Stream (KK-403 → 404)
 *   3.8 giriş kilidi        → api/internal/service/auth/service.go
 *                             loginAttemptLimit=5 / loginAttemptWindow=15dk
 *   4.2 sağlayıcı isteği    → api/internal/adapter/provider/herosms/operations.go
 *                             Purchase → {service, country, amount, operator,
 *                             maxPrice, verificationType}
 *   5.3 reCAPTCHA           → web/src/components/recaptcha.tsx (yalnız
 *                             kayıt/giriş formları) ·
 *                             api/internal/adapter/captcha/recaptcha.go (remoteip)
 *   5.5 hata izleme         → api/internal/adapter/obs/sentry.go
 *                             SendDefaultPII:false + sensitiveKeys maskesi
 *   5.6 harici kaynak yok   → web/next.config.ts CSP (font-src 'self' data:)
 *   6 · çerezler            → api/internal/transport/http/middleware/session.go
 *                             SetSessionCookie (HttpOnly/Secure/SameSite=Lax) ·
 *                             api/internal/config/config.go SESSION_TTL=720h
 *   6 · tema                → web/src/components/theme.tsx (localStorage 'tema')
 *   8.1 argon2id            → api/internal/domain/auth/password.go
 *   8.2 token özeti         → api/migrations/00001_init_identity.sql auth_tokens
 *   8.4 HTTPS/HSTS          → deploy/Caddyfile
 *   8.7 dekont              → api/internal/adapter/storage/storage.go (KK-500) ·
 *                             handler/deposit.go serveReceipt (sahiplik)
 *   8.8 anahtar şifreleme   → api/internal/adapter/crypto/secretbox.go (AES-GCM)
 */

/** KVKK başvuru ve iletişim adresi. Sayfada birden çok yerde geçer. */
const BASVURU_EPOSTA = 'metoakr@gmail.com';

/** Son güncelleme SABİT yazılır: `new Date()` her istekte değişir ve
 *  sunucu/istemci farkı hidrasyon uyuşmazlığı üretir. */
const SON_GUNCELLEME = '9 Eylül 2026';

/** Bir bölümün gövdesinden sonra basılacak yapılandırılmış blok. */
type Ek = 'veriler' | 'cerezler' | 'saklama' | 'haklar';

type Bolum = { kimlik: string; baslik: string; maddeler: string[]; ek?: Ek };

/**
 * Bölümler VERİ olarak durur: numaralandırma ve içindekiler listesi tek
 * kaynaktan üretilir. Elle yazılsaydı araya bir madde eklendiğinde numaralar
 * ile içindekiler sessizce ayrışırdı.
 *
 * `kimlik` bağlantı çıpasıdır ve İNGİLİZCE'dir: Türkçe karakterli bir id
 * URL'de yüzde kodlamasına dönüşür ve paylaşılan bağlantı okunmaz olur.
 */
const BOLUMLER: Bolum[] = [
  {
    kimlik: 'scope',
    baslik: 'Bu metin ne anlatıyor',
    maddeler: [
      'Bu metin, Onay360 adıyla sunulan sanal numara hizmetinde hangi kişisel ' +
        'verilerin işlendiğini, bunları hangi amaçla ve hangi hukuki sebeple ' +
        'işlediğimizi, kimlerle paylaştığımızı, ne kadar sakladığımızı ve ' +
        'haklarınızı nasıl kullanabileceğinizi açıklar.',
      '6698 sayılı Kişisel Verilerin Korunması Kanunu’nun 10. maddesindeki ' +
        'aydınlatma yükümlülüğü kapsamında hazırlanmıştır; sitenin tüm ' +
        'sayfalarını ve hesap panelini kapsar.',
      'Hesap açmak için ad-soyad, T.C. kimlik numarası, adres, doğum tarihi ' +
        'veya kendi telefon numaranız İSTENMEZ. Kayıt için e-posta adresi, ' +
        'kullanıcı adı ve şifre yeterlidir.',
      'Kayıt ekranında bu metni okuduğunuzu onaylamanız istenir. Bu onay kutusu ' +
        'bir “açık rıza” değildir: hizmeti verebilmek için zorunlu olan ' +
        'işlemelerin her birinde dayandığımız hukuki sebep 3. bölümde ayrı ayrı ' +
        'yazılıdır.',
    ],
  },
  {
    kimlik: 'data',
    baslik: 'İşlediğimiz kişisel veriler',
    ek: 'veriler',
    maddeler: [
      'Aşağıdaki liste sistemde gerçekten tutulan alanlardan çıkarılmıştır. ' +
        'Burada sayılmayan bir kişisel veri kategorisi toplanmamaktadır.',
      'Sanal numaraya gelen mesajların TAM METNİ saklanır — yalnız içindeki ' +
        'doğrulama kodu değil. Bu mesajları numarayı kullandığınız servis ' +
        'üretir; içeriğine biz karar vermeyiz. Numaraya, doğrulama kodu dışında ' +
        'bir bilgi taşıyan mesaj da gelebilir.',
      'Yorum yazarsanız ve yorumunuz onaylanırsa KULLANICI ADINIZLA BİRLİKTE ' +
        'herkese açık olarak yayımlanır. Yorumda paylaştığınız her bilgi ' +
        'oturum açmamış ziyaretçiler tarafından da görülebilir. Yayımlanan bir ' +
        'yorumun metni sonradan değiştirilemez; fikriniz değişirse yeni bir ' +
        'yorum yazarsınız.',
      'Panelde gördüğünüz her kayıt yalnız size aittir. Sipariş, cüzdan, dekont, ' +
        'destek ve yorum kayıtları sorgu düzeyinde hesabınıza bağlıdır: başka ' +
        'bir kullanıcının kaydının adresi bilinse bile o kayda erişilemez.',
    ],
  },
  {
    kimlik: 'purpose',
    baslik: 'İşleme amaçlarımız ve hukuki sebepler',
    maddeler: [
      'Hesap açma, giriş, e-posta doğrulama ve şifre sıfırlama: sözleşmenin ' +
        'kurulması ve ifası için zorunludur (KVKK m.5/2-c).',
      'Fiyat teklifi, numara satın alma, gelen mesajların size gösterilmesi, ' +
        'bakiye düşümü ve iade: sözleşmenin ifası (KVKK m.5/2-c).',
      'Bakiye yükleme talebinizin incelenmesi ve dekontun doğrulanması: ' +
        'sözleşmenin ifası ve mali kayıt tutma yükümlülüğü (KVKK m.5/2-c ve ' +
        'm.5/2-ç).',
      'Sipariş, ödeme ve cüzdan kayıtlarının saklanması: vergi ve ticaret ' +
        'mevzuatından doğan hukuki yükümlülük (KVKK m.5/2-ç).',
      'Oturum kayıtları, sunucu istek kayıtları, denetim kayıtları, istek ' +
        'sıklığı sınırları ve robot denetimi: hesabınızın ve sistemin ' +
        'güvenliği ile kötüye kullanımın önlenmesi — meşru menfaat ' +
        '(KVKK m.5/2-f).',
      'Destek yazışmaları: talebinizin çözülmesi ve uyuşmazlık hâlinde kayıt ' +
        'tutulması (KVKK m.5/2-c ve m.5/2-f).',
      'Pazarlama e-postası göndermiyoruz, reklam amacıyla profil çıkarmıyoruz ve ' +
        'ziyaretçi davranışını izleyen bir analitik aracı kullanmıyoruz. Size ' +
        'gönderdiğimiz e-postalar yalnızca ikisidir: e-posta doğrulama ve şifre ' +
        'sıfırlama bağlantısı.',
      'Otomatik sistemlerle hakkınızda bir değerlendirme yapılmaz. Otomatik ' +
        'uygulanan tek kısıt güvenlik kısıtıdır: aynı hesaba art arda beş ' +
        'başarısız giriş denemesi yapılırsa giriş on beş dakika kilitlenir, ' +
        'ayrıca bazı işlemlerde istek sıklığı sınırlanır.',
    ],
  },
  {
    kimlik: 'provider',
    baslik: 'Numara sağlayıcısına ne gönderiliyor',
    maddeler: [
      'Numaralar bize ait bir şebekeden gelmez; yurt dışında yerleşik bir sanal ' +
        'numara altyapı sağlayıcısından temin edilir.',
      'Bir numara alınırken sağlayıcıya gönderilen istek YALNIZ şunları içerir: ' +
        'seçtiğiniz servisin kodu, ülkenin kodu, adet bilgisi (bir), operatör ' +
        'tercihi, kabul ettiğimiz azami maliyet ve doğrulama tipi (SMS ya da ' +
        'çağrı). Kiralık numarada bunlara kiralama süresi eklenir.',
      '🔴 Bu istekte e-posta adresiniz, kullanıcı adınız, IP adresiniz veya ' +
        'hesabınızı gösteren herhangi bir kimlik YER ALMAZ. Sağlayıcı, numarayı ' +
        'hangi kullanıcı için aldığımızı bilmez; siparişi hesabınıza bağlayan ' +
        'kayıt yalnız bizim tarafımızda durur ve sağlayıcıya hiç gitmez.',
      'Siparişi iptal ederken ya da tamamlarken sağlayıcıya yalnız o numaraya ' +
        'ait aktivasyon numarası gönderilir; başka hiçbir bilgi gitmez.',
      'Numaraya bir mesaj ulaştığında sağlayıcı bize haber verir; mesajın ' +
        'metnini biz sağlayıcıdan okuyup sipariş kaydınıza yazarız.',
      'Sağlayıcı, kendi hizmetini verirken numaraya gelen mesajları kendi ' +
        'sistemlerinde de işler. Bu işlemenin ayrıntıları sağlayıcının kendi ' +
        'politikasına tabidir ve bizim denetimimizde değildir. Bu nedenle ' +
        'sanal numarayı kalıcı iletişim ya da hesap kurtarma numarası olarak ' +
        'kullanmayın.',
    ],
  },
  {
    kimlik: 'third-parties',
    baslik: 'Üçüncü taraflar ve yurt dışına aktarım',
    maddeler: [
      'Kişisel verilerinizi satmıyoruz ve reklam amacıyla kimseyle ' +
        'paylaşmıyoruz. Aşağıdakiler, hizmetin çalışması için veri aktarılan ' +
        'tarafların tamamıdır.',
      'Sanal numara altyapı sağlayıcısı (yurt dışında yerleşik): 4. bölümde ' +
        'sayılan sipariş bilgileri aktarılır. Kimliğinizi gösteren bir veri ' +
        'aktarılmaz.',
      'Robot denetimi — Google reCAPTCHA: yalnız KAYIT ve GİRİŞ sayfalarında ' +
        'yüklenir. Bu iki sayfada Google’ın betiği tarayıcınıza iner ve ' +
        'doğrulama sırasında IP adresiniz ile tarayıcınıza ait teknik bilgiler ' +
        'Google’a iletilir; doğrulamayı sunucumuz da Google’a sorar ve bu ' +
        'istekte IP adresiniz yer alır. Google yurt dışında yerleşiktir ve bu ' +
        'sayfalarda kendi çerezlerini yerleştirebilir; o çerezler bizim ' +
        'denetimimizde değildir. Sitenin diğer sayfalarında reCAPTCHA yüklenmez.',
      'E-posta gönderimi: doğrulama ve şifre sıfırlama iletileri bir e-posta ' +
        'gönderim servisi üzerinden yollanır; bu servise e-posta adresiniz ve ' +
        'iletinin metni gider.',
      'Hata izleme: sunucuda beklenmeyen bir hata oluştuğunda teknik hata ' +
        'kaydı bir hata izleme servisine gönderilir. Bu kayıtta IP adresiniz ve ' +
        'hesabınızın numarası yer alabilir. Şifre, oturum bilgisi, doğrulama ' +
        'kodu ve telefon numarası taşıyan alanlar gönderilmeden önce ' +
        'maskelenir; servisin kendiliğinden kişisel veri toplaması kapalıdır.',
      'Site dışarıdan yazı tipi, ikon, reklam ya da izleme betiği YÜKLEMEZ. ' +
        'Yukarıdaki robot denetimi dışında sayfalarımız yalnız kendi ' +
        'sunucumuzdaki dosyaları kullanır; bu sınır tarayıcıya gönderilen bir ' +
        'içerik güvenlik politikasıyla da uygulanır.',
      'Yetkili kamu kurum ve kuruluşlarının mevzuata dayalı ve usulüne uygun ' +
        'talepleri karşılanır.',
    ],
  },
  {
    kimlik: 'cookies',
    baslik: 'Çerezler ve tarayıcıda saklananlar',
    ek: 'cerezler',
    maddeler: [
      'Sitede kullandığımız TEK çerez, giriş yapmanızı mümkün kılan oturum ' +
        'çerezidir. Reklam, ölçümleme veya ziyaretçi takibi amacıyla çerez ' +
        'kullanmıyoruz.',
      'Kullandığımız tek çerez hizmetin çalışması için zorunlu olduğundan ' +
        'sitede çerez izni penceresi göstermiyoruz: kapatabileceğiniz isteğe ' +
        'bağlı bir çerez yok.',
      'Tarayıcınızda çerezleri engellerseniz siteyi gezebilir, fiyatları ve ' +
        'servis listesini görebilirsiniz; ancak giriş yapamazsınız.',
    ],
  },
  {
    kimlik: 'retention',
    baslik: 'Saklama süreleri',
    ek: 'saklama',
    maddeler: [
      'Kayıtları, işleme amacı için gereken süre boyunca ve mevzuatın öngördüğü ' +
        'süreler kadar saklarız.',
      '🔴 Mesajın İÇERİĞİ ile SİPARİŞİN KENDİSİ aynı süre saklanmaz; ikisi ' +
        'bilerek ayrılmıştır. Gelen mesajların metni doksan gün sonra silinir. ' +
        'Siparişin kendisi — hangi servis için, hangi ülkeden, hangi numarayı, ' +
        'ne zaman ve kaç liraya aldığınız — mali kayıt olduğu için on yıl ' +
        'saklanır. Yani doksan gün sonra “hangi siparişi verdiğiniz” görünmeye ' +
        'devam eder, “o numaraya ne yazıldığı” görünmez.',
      'Süresi dolan kayıtlar silinir. Silme canlı sistemde uygulanır; teknik ' +
        'yedeklerde kalan kopyalar da yedeğin kendi saklama süresi dolduğunda ' +
        'ortadan kalkar.',
      'Hesabınızı sildirseniz bile sipariş, ödeme ve cüzdan kayıtları yasal ' +
        'saklama süresi dolana kadar durur; bu kayıtlar mali belge ' +
        'niteliğindedir ve silinmeleri mevzuata aykırı olurdu.',
    ],
  },
  {
    kimlik: 'security',
    baslik: 'Güvenlik tedbirleri',
    maddeler: [
      'Şifreniz sistemde açık hâliyle hiçbir zaman saklanmaz. argon2id ile ' +
        'geri döndürülemez biçimde özetlenir; veritabanına erişen biri bile ' +
        'şifrenizi geri elde edemez. Personelimiz dâhil kimse şifrenizi göremez.',
      'E-posta doğrulama ve şifre sıfırlama bağlantılarının içindeki anahtar da ' +
        'saklanmaz; yalnız SHA-256 özeti tutulur. Doğrulama bağlantısı 24 saat, ' +
        'şifre sıfırlama bağlantısı 1 saat geçerlidir ve tek kullanımlıktır.',
      'Oturum çerezi sayfadaki JavaScript’e kapalıdır (HttpOnly) ve başka bir ' +
        'siteden gelen isteklerde gönderilmez (SameSite=Lax).',
      'Site yalnız HTTPS üzerinden sunulur. Şifresiz bağlantılar HTTPS’e ' +
        'yönlendirilir ve tarayıcıya bir yıl boyunca yalnız HTTPS kullanması ' +
        'söylenir (HSTS).',
      'Tarayıcıya gönderilen güvenlik başlıkları, sayfanın izin verilen adresler ' +
        'dışından betik yüklemesini, başka bir sitede çerçeve içine alınmasını ' +
        've dosya tipinin yanlış yorumlanmasını engeller. Kamera, mikrofon ve ' +
        'konum erişimi kapalıdır.',
      'Aynı hesaba art arda beş başarısız giriş denemesi yapılırsa giriş on beş ' +
        'dakika kilitlenir. Kayıt ve giriş adımlarında robot denetimi ' +
        'uygulanır. Şifre sıfırlama talebi saatte üç ile sınırlıdır.',
      'Yüklediğiniz dekont dosyaları web kökünün DIŞINDA saklanır ve doğrudan ' +
        'bir adresle indirilemez. Dosyanın adı sunucuda üretilir; sizin ' +
        'verdiğiniz ad hiç kullanılmaz. Dosyayı yalnız siz ve talebi inceleyen ' +
        'yetkili görebilir.',
      'Sağlayıcı erişim anahtarları gibi sırlar veritabanında AES-256-GCM ile ' +
        'şifreli saklanır.',
      'Yönetim tarafındaki erişim rol ve izinlere bağlıdır; her yetkili yalnız ' +
        'işini yapmak için gereken kayda erişir. Bakiye ve yükleme talebi ' +
        'işlemleri, işlemi yapanın kimliğiyle birlikte denetim kaydına yazılır.',
      'Uygulama günlüğüne şifre, oturum kimliği, doğrulama kodu ve sağlayıcı ' +
        'anahtarı yazılmaz; adreslerin sorgu kısmı hiç kaydedilmez ve e-posta ' +
        'adresi günlüğe yazılması gereken yerlerde maskelenir.',
      '🔴 Hiçbir sistem mutlak güvenlik sunmaz. Yukarıdakiler bugün uygulanan ' +
        'tedbirlerdir; “verileriniz kesinlikle ele geçirilemez” gibi bir ' +
        'taahhüt vermiyoruz. Hesabınız için başka hiçbir yerde kullanmadığınız ' +
        'bir şifre seçin ve açık oturumlarınızı ara ara kontrol edin.',
    ],
  },
  {
    kimlik: 'rights',
    baslik: 'Haklarınız ve başvuru yolu',
    ek: 'haklar',
    maddeler: [
      'Kanunun 11. maddesi uyarınca sahip olduğunuz dokuz hak, bu bölümün ' +
        'sonunda kanundaki hâliyle sıralanmıştır.',
      'Bu hakların bir kısmını hiç başvuru yapmadan, doğrudan hesabınızdan ' +
        'kullanabilirsiniz: hesap bilgileriniz, açık oturumlarınız, sipariş ' +
        'geçmişiniz ve numaraya gelen mesajlar, cüzdan hareketleriniz, bakiye ' +
        'yükleme talepleriniz ve yüklediğiniz dekontlar, destek yazışmalarınız ' +
        've yorumlarınız panelde görünür. Tanımadığınız bir oturumu hesap ' +
        'sayfanızdan kendiniz kapatabilirsiniz.',
      `Başvurularınızı ${BASVURU_EPOSTA} adresine gönderin. Başvuruyu hesabınıza ` +
        'kayıtlı e-posta adresinden göndermeniz, kimliğinizi doğrulamamızı ve ' +
        'talebin daha hızlı sonuçlanmasını sağlar.',
      `🔴 Hesabınıza giremiyorsanız destek talebi AÇAMAZSINIZ: destek sistemi ` +
        'giriş yapmayı gerektirir. Şifrenizi unuttuysanız, hesabınız askıya ' +
        'alındıysa ya da başka bir sebeple giriş yapamıyorsanız doğrudan ' +
        `${BASVURU_EPOSTA} adresine yazın. Bu adres, hesabına erişemeyen ` +
        'kişiler için de açıktır.',
      'Başvurular en geç otuz gün içinde sonuçlandırılır (KVKK m.13).',
      'Başvurunuz reddedilirse veya süresinde cevap verilmezse Kişisel Verileri ' +
        'Koruma Kurulu’na şikâyette bulunabilirsiniz (KVKK m.14).',
    ],
  },
  {
    kimlik: 'deletion',
    baslik: 'Hesabınızı ve verilerinizi sildirmek',
    maddeler: [
      '🔴 Bugün panelde hesabınızı kendi kendinize silmenizi sağlayan bir düğme ' +
        `YOKTUR. Silme talebinizi ${BASVURU_EPOSTA} adresine iletin; talep elle ` +
        'işlenir. Olmayan bir düğmeyi tarif etmemek için bunu açıkça yazıyoruz.',
      'Silme talebiniz sonuçlandığında, yasal saklama yükümlülüğü bulunmayan ' +
        'kayıtlar silinir. Sipariş, ödeme ve cüzdan kayıtları mali belge ' +
        'niteliğinde olduğu için on yıllık süre dolana kadar saklanmaya devam ' +
        'eder (7. bölüm).',
      'Verilerinizin bir kopyasını istiyorsanız aynı adrese yazın; panelde tek ' +
        'tuşla dışa aktarma bugün yoktur.',
    ],
  },
  {
    kimlik: 'changes',
    baslik: 'Bu metindeki değişiklikler',
    maddeler: [
      'Bu metin değişirse güncel hâli bu sayfada yayımlanır ve sayfanın ' +
        'başındaki “Son güncelleme” tarihi değişir.',
      'Değişiklikten sonra hizmeti kullanmaya devam etmeniz, güncel metnin ' +
        'geçerli olduğu anlamına gelir.',
    ],
  },
];

/* ═══════════════════════ Yapılandırılmış bloklar ═══════════════════════ */

/** Veri kategorileri — her satır sistemde gerçekten var olan alanlardan çıktı. */
const VERI_KATEGORILERI: Array<[string, string]> = [
  [
    'Hesap bilgileri',
    'E-posta adresiniz, kullanıcı adınız, şifrenizin geri döndürülemez özeti, ' +
      'hesabınızın durumu, kayıt tarihi ve e-posta doğrulama tarihi.',
  ],
  [
    'Oturum kayıtları',
    'Giriş yaptığınız cihazın IP adresi ve tarayıcı tanıtım bilgisi, oturumun ' +
      'açılış, son görülme ve bitiş zamanı. Bu listeyi hesap sayfanızdan ' +
      'görebilir, istediğiniz oturumu kapatabilirsiniz.',
  ],
  [
    'Sipariş kayıtları',
    'Size tahsis edilen sanal numara, seçtiğiniz servis ve ülke, ödediğiniz ' +
      'tutar, siparişin durumu ve zaman damgaları. Kiralık numarada ayrıca ' +
      'kiralama süresi ve kiralamanın bitiş zamanı.',
  ],
  [
    'Numaraya gelen mesajlar',
    'Mesajın tam metni, içindeki doğrulama kodu, gönderen bilgisi ve geliş ' +
      'zamanı.',
  ],
  [
    'Cüzdan hareketleri',
    'Her yükleme, satın alma ve iade için tutar, işlem tipi, işlem sonrası ' +
      'bakiye ve varsa açıklama notu.',
  ],
  [
    'Bakiye yükleme talepleri',
    'Bildirdiğiniz tutar, seçtiğiniz yöntem, kripto ödemelerde işlem hash’i ve ' +
      'ağ adı, yazdığınız açıklama ve yüklediğiniz dekont dosyası.',
  ],
  [
    'Destek yazışmaları',
    'Açtığınız talebin konusu, önceliği ve yazdığınız mesajların tamamı.',
  ],
  [
    'Yorumlar',
    'Verdiğiniz puan ve yorum metni. Onaylanırsa kullanıcı adınızla birlikte ' +
      'sitede herkese açık yayımlanır.',
  ],
  [
    'Fiyat teklifleri',
    'Aldığınız her fiyat teklifi için hangi servis ve ülke için sorduğunuz, ' +
      'size gösterilen tutar ve teklifin geçerlilik süresi. Teklif tek ' +
      'kullanımlıktır; kullanılmazsa süresi dolar.',
  ],
  [
    'Yönetim notları',
    'Bakiye yükleme talebiniz incelenirken yetkilinin yazdığı iç değerlendirme ' +
      'notu ve talep reddedilirse size gösterilen red gerekçesi. Yorumunuz ' +
      'reddedilirse gerekçesi de kaydedilir ve size gösterilir.',
  ],
  [
    'Sunucu istek kayıtları',
    'Her istek için istek kimliği, yöntem, adres, sonuç kodu, süre, IP adresi ' +
      've oturum açıksa hesabınızın numarası. Adresin sorgu kısmı bilerek ' +
      'kaydedilmez.',
  ],
  [
    'Denetim kayıtları',
    'Yönetim tarafında yapılan işlemler (bir yükleme talebinin onaylanması ' +
      'gibi): işlemi yapan yetkili, işlemin IP adresi ve tarayıcı bilgisi, ' +
      'etkilenen kaydın kimliği ve değişen alanlar. Bu kayıtlara e-posta ' +
      'adresi ve dosya yolu yazılmaz.',
  ],
];

type Cerez = { ad: string; amac: string; sure: string; ozellik: string };

const CEREZLER: Cerez[] = [
  {
    ad: 'sid',
    amac:
      'Oturum çerezi. Giriş yaptıktan sonra her isteğinizde kim olduğunuzu ' +
      'anlamamızı sağlar; hizmetin çalışması için zorunludur.',
    sure: 'En çok 30 gün; siteyi her kullanışınızda tazelenir. Çıkış yaptığınızda silinir.',
    ozellik:
      'HttpOnly (sayfadaki JavaScript okuyamaz) · SameSite=Lax (başka siteden ' +
      'gelen isteklerde gönderilmez) · yalnız HTTPS üzerinden gönderilir.',
  },
  {
    ad: 'Tema tercihi (“tema”)',
    amac:
      'Açık/koyu görünüm tercihiniz. ÇEREZ DEĞİLDİR: tarayıcınızın kendi ' +
      'belleğinde (localStorage) saklanır ve sunucuya hiç gönderilmez.',
    sure: 'Siz silene ya da tarayıcı verilerini temizleyene kadar cihazınızda kalır.',
    ozellik: 'Yalnız cihazınızda durur; bizim sunucumuza ulaşmaz.',
  },
  {
    ad: 'Google reCAPTCHA çerezleri',
    amac:
      'Robot denetimi. Yalnız kayıt ve giriş sayfalarında, Google tarafından ' +
      'yerleştirilebilir.',
    sure: 'Google tarafından belirlenir.',
    ozellik:
      'Bizim denetimimizde değildir; içerikleri ve süreleri Google’ın ' +
      'politikasına tabidir.',
  },
];

/** Saklama süreleri — kullanıcı kararı (9 Eylül 2026). */
const SAKLAMA: Array<[string, string]> = [
  ['Numaraya gelen mesajların içeriği ve doğrulama kodu', '90 gün'],
  [
    'Sipariş kayıtları, cüzdan hareketleri, bakiye yükleme talepleri ve dekontlar',
    '10 yıl',
  ],
  ['Denetim kayıtları', '2 yıl'],
  [
    'Hesap bilgileriniz',
    'Hesabınız var olduğu sürece; silme talebiniz işlendiğinde silinir',
  ],
  [
    'Oturum kayıtları',
    // 🔴 "Çıkış yapınca silinir" YAZILAMAZ — kodda çıkış SİLMEZ, İPTAL EDER:
    // queries/sessions.sql:14 `UPDATE sessions SET revoked_at = now()`; satır
    // yerinde durur ve IP + tarayıcı bilgisini taşır. Silen tek sorgu
    // queries/retention.sql `WHERE expires_at < ...`, yani girişten 30 gün sonra.
    // expires_at hiçbir yerde uzatılmıyor (TouchSession yalnız last_seen_at yazar).
    'Oturumu kapattığınızda oturum anında geçersiz kılınır; kaydı ise ' +
      'süresi dolduğunda — en geç girişten 30 gün sonra — silinir',
  ],
  [
    'E-posta doğrulama ve şifre sıfırlama bağlantıları',
    'Süresi dolduğunda silinir (doğrulama 24 saat, şifre sıfırlama 1 saat)',
  ],
];

/**
 * KVKK m.11 — kanun metni BİREBİR alınmıştır.
 *
 * Bilerek yeniden yazılmadı: bir hakkın kapsamını “sadeleştirmek” o hakkı
 * daraltır ve okuyucu kanunda olmayan bir sınır varmış gibi anlar.
 */
const HAKLAR: string[] = [
  'Kişisel veri işlenip işlenmediğini öğrenme,',
  'Kişisel verileri işlenmişse buna ilişkin bilgi talep etme,',
  'Kişisel verilerin işlenme amacını ve bunların amacına uygun kullanılıp ' +
    'kullanılmadığını öğrenme,',
  'Yurt içinde veya yurt dışında kişisel verilerin aktarıldığı üçüncü kişileri ' +
    'bilme,',
  'Kişisel verilerin eksik veya yanlış işlenmiş olması hâlinde bunların ' +
    'düzeltilmesini isteme,',
  'Kanunun 7. maddesinde öngörülen şartlar çerçevesinde kişisel verilerin ' +
    'silinmesini veya yok edilmesini isteme,',
  'Düzeltme ile silme veya yok etme işlemlerinin, kişisel verilerin aktarıldığı ' +
    'üçüncü kişilere bildirilmesini isteme,',
  'İşlenen verilerin münhasıran otomatik sistemler vasıtasıyla analiz edilmesi ' +
    'suretiyle kişinin kendisi aleyhine bir sonucun ortaya çıkmasına itiraz etme,',
  'Kişisel verilerin kanuna aykırı olarak işlenmesi sebebiyle zarara uğraması ' +
    'hâlinde zararın giderilmesini talep etme.',
];

/**
 * Bu metinde BİLEREK yer almayan konular.
 *
 * Sayfada gösterilir: eksik olduğunu bilmediğiniz bir madde, yanlış yazılmış
 * bir maddeden daha tehlikelidir. Her satırın sebebi sistemin ya da işletmenin
 * bugünkü durumudur — hiçbiri “unutuldu” değildir.
 */
const YOK_SAYILANLAR: string[] = [
  'Veri sorumlusunun ticaret unvanı, adresi, vergi bilgileri ve VERBİS kaydı — ' +
    'şirket kuruluşu tamamlanmadığı için yayımlanmadı. Bu bilgiler ' +
    'yayımlanana kadar bütün başvurular yukarıdaki e-posta adresinden ' +
    'karşılanır.',
  'Sunucuların ve yedeklerin bulunduğu ülke ile barındırma hizmeti ' +
    'sağlayıcısının kimliği. Bu bilgiyi öğrenmek isterseniz yukarıdaki ' +
    'e-posta adresinden yazılı olarak talep edebilirsiniz.',
  'Sunucu istek kayıtlarının (web sunucusu erişim kayıtları) saklama süresi. ' +
    'Bu süre barındırma yapılandırmasıyla belirlenir ve bu belgede taahhüt ' +
    'edilmez; yukarıda sayılan saklama süreleri uygulamanın kendi ' +
    'veritabanı kayıtları içindir.',
  'Panelden hesap silme ve verilerin dışa aktarımı — bugün böyle bir işlev ' +
    'yoktur; talepler e-posta ile elle karşılanır (10. bölüm).',
  'Yaş doğrulaması — hizmet 18 yaşından küçüklere sunulmaz, ancak sistemde ' +
    'yaşı doğrulayan teknik bir kontrol yoktur.',
];

/* ═══════════════════════ Sunum ═══════════════════════ */

function EkBlok({ ek }: { ek: Ek }) {
  if (ek === 'veriler' || ek === 'saklama') {
    const satirlar = ek === 'veriler' ? VERI_KATEGORILERI : SAKLAMA;
    return (
      <Card className="mt-4">
        {/* Tablo DEĞİL: iki sütunlu bir tablo 320 px'te yatay kaydırma
            üretirdi. Mobilde alt alta, `md:` ile yan yana. */}
        <dl className="flex flex-col gap-4">
          {satirlar.map(([etiket, deger]) => (
            <div key={etiket} className="flex flex-col gap-1 md:flex-row md:gap-4">
              <dt className="text-sm font-medium md:w-64 md:shrink-0">{etiket}</dt>
              <dd className="text-sm leading-relaxed text-muted">{deger}</dd>
            </div>
          ))}
        </dl>
      </Card>
    );
  }

  if (ek === 'cerezler') {
    return (
      <div className="mt-4 flex flex-col gap-3">
        {CEREZLER.map((c) => (
          <Card key={c.ad}>
            <p className="font-mono text-sm font-semibold break-anywhere">{c.ad}</p>
            <dl className="mt-3 flex flex-col gap-2">
              {([
                ['Amaç', c.amac],
                ['Süre', c.sure],
                ['Özellikler', c.ozellik],
              ] as Array<[string, string]>).map(([etiket, deger]) => (
                <div key={etiket} className="flex flex-col gap-0.5 md:flex-row md:gap-4">
                  <dt className="text-xs text-muted md:w-28 md:shrink-0 md:text-sm">
                    {etiket}
                  </dt>
                  <dd className="text-sm leading-relaxed text-muted">{deger}</dd>
                </div>
              ))}
            </dl>
          </Card>
        ))}
      </div>
    );
  }

  // 'haklar' — kanun metni, harf sıralı (a, b, c, ç…) DEĞİL sayısal listelenir:
  // Türkçe alfabe sırası CSS'in `lower-alpha` sayacında yoktur ve "ç" yerine
  // "d" basılırdı.
  return (
    <Card className="mt-4">
      <ol className="flex flex-col gap-2">
        {HAKLAR.map((hak, i) => (
          <li key={i} className="flex gap-3">
            <span className="w-5 shrink-0 text-sm tabular-nums text-muted">{i + 1}.</span>
            <p className="text-sm leading-relaxed text-muted">{hak}</p>
          </li>
        ))}
      </ol>
    </Card>
  );
}

export default function PrivacyPage() {
  return (
    <div className="px-4 py-10 md:px-6 md:py-14">
      <div className="mx-auto max-w-3xl">
        <h1 className="text-3xl font-bold tracking-tight md:text-4xl">Gizlilik Politikası</h1>
        <p className="mt-2 text-sm text-muted">Son güncelleme: {SON_GUNCELLEME}</p>

        <p className="mt-6 text-base leading-relaxed text-muted md:text-lg">
          Onay360, bir servise kayıt olurken gereken SMS onay kodunu almanız için
          geçici numara sağlayan ön ödemeli bir hizmettir. Bu metin, hizmeti
          kullanırken hangi kişisel verilerin işlendiğini, bunların kimlerle
          paylaşıldığını, ne kadar saklandığını ve haklarınızı nasıl
          kullanabileceğinizi anlatır.
        </p>

        {/* Sayfanın en önemli iki bilgisi yukarıda: sağlayıcıya kimlik
            gitmiyor ve hesabına giremeyen kişinin de bir kanalı var. */}
        <Alert tone="info" className="mt-6">
          <strong className="font-semibold">Kısaca:</strong> Numara sağlayıcısına
          kimliğinizi gösteren hiçbir bilgi gönderilmez. Reklam ve analitik çerezi
          kullanmıyoruz. Numaraya gelen mesajların içeriği 90 gün sonra silinir.
          Her türlü başvuru için{' '}
          <a
            href={`mailto:${BASVURU_EPOSTA}`}
            className="underline underline-offset-4 break-anywhere"
          >
            {BASVURU_EPOSTA}
          </a>{' '}
          adresine yazabilirsiniz — hesabınıza giremiyorsanız da bu adres açıktır.
        </Alert>

        {/* İçindekiler: metin uzun; hangi bölümü aradığını bilen kullanıcı
            kaydırmadan gitmeli. Bağlantılar sunucuda üretilir, JS gerekmez. */}
        <nav aria-labelledby="icindekiler-basligi" className="mt-8">
          <h2 id="icindekiler-basligi" className="text-xl font-semibold md:text-2xl">
            İçindekiler
          </h2>
          <Card className="mt-4">
            {/* Satırın TAMAMI tıklanabilir ve ≥ 44 px yüksekliğinde, aralarında
                ≥ 8 px boşluk var (frontend-contract §2.3). Numara bağlantının
                İÇİNDE: ayrı bir <span> olsaydı 44 px'lik hedefin dışında
                kalırdı. */}
            <ol className="grid gap-2 md:grid-cols-2">
              {BOLUMLER.map((b, i) => (
                <li key={b.kimlik}>
                  <Link
                    href={`#${b.kimlik}`}
                    className="flex min-h-11 items-center gap-2 py-1 text-sm text-brand-300"
                  >
                    {/* Altı çizili olan YALNIZ başlık: `text-decoration` satır içi
                        çocuklara geçer ve çocuktaki `no-underline` onu KALDIRMAZ
                        (CSS). Çizgiyi bağlantının tamamına verirsek madde
                        numarası da çizilir. */}
                    <span aria-hidden className="w-5 shrink-0 tabular-nums text-muted">
                      {i + 1}.
                    </span>
                    <span className="underline underline-offset-4">{b.baslik}</span>
                  </Link>
                </li>
              ))}
            </ol>
          </Card>
        </nav>

        {BOLUMLER.map((bolum, i) => (
          <section key={bolum.kimlik} aria-labelledby={bolum.kimlik}>
            {/* scroll-mt: başlık yapışkan başlığın altında kalmasın — çıpaya
                atlayan kullanıcı başlığı görmeli. Aynı id hem çıpa hem
                erişilebilirlik etiketi: iki ayrı id tutmak birinin sessizce
                ayrışması demektir. */}
            <h2
              id={bolum.kimlik}
              className="mt-12 scroll-mt-20 text-xl font-semibold md:text-2xl"
            >
              {i + 1}. {bolum.baslik}
            </h2>
            <Card className="mt-4">
              <ol className="flex flex-col gap-3">
                {bolum.maddeler.map((madde, j) => (
                  <li key={`${bolum.kimlik}-${j}`} className="flex gap-3">
                    {/* Madde numarası ekran okuyucudan gizli DEĞİL: "7.2'ye
                        bakın" diyen bir destek yanıtı ancak numara
                        okunabiliyorsa işe yarar. */}
                    <span className="w-8 shrink-0 text-sm tabular-nums text-muted">
                      {i + 1}.{j + 1}
                    </span>
                    <p className="text-sm leading-relaxed text-muted">{madde}</p>
                  </li>
                ))}
              </ol>
            </Card>
            {bolum.ek && <EkBlok ek={bolum.ek} />}
          </section>
        ))}

        <h2 className="mt-12 text-xl font-semibold md:text-2xl">Başvuru adresi</h2>
        <Card className="mt-4">
          <p className="text-sm leading-relaxed text-muted">
            Kişisel verilerinize ilişkin bilgi, düzeltme, silme ve diğer tüm
            talepleriniz için:
          </p>
          <p className="mt-3">
            <a
              href={`mailto:${BASVURU_EPOSTA}`}
              className="inline-flex min-h-11 items-center text-base font-medium
                         text-brand-300 underline underline-offset-4 break-anywhere"
            >
              {BASVURU_EPOSTA}
            </a>
          </p>
          <p className="mt-1 text-sm leading-relaxed text-muted">
            Hesabınıza giriş yapabiliyorsanız panelden destek talebi açmak daha
            hızlıdır: talep hesabınıza bağlı olduğu için kimlik doğrulamayla vakit
            kaybedilmez. Giriş yapamıyorsanız yukarıdaki adrese yazın.
          </p>
        </Card>

        <h2 className="mt-12 text-xl font-semibold md:text-2xl">
          Bu metinde henüz yer almayan konular
        </h2>
        <p className="mt-2 text-sm leading-relaxed text-muted">
          Aşağıdakiler bilerek boş bırakıldı: bugün karşılığı olmayan bir taahhüdü
          yazmaktansa, eksik olduğunu açıkça belirtmeyi tercih ediyoruz.
        </p>
        <Card className="mt-4">
          <ul className="flex list-disc flex-col gap-2 pl-5 text-sm leading-relaxed text-muted">
            {YOK_SAYILANLAR.map((satir, i) => (
              <li key={i}>{satir}</li>
            ))}
          </ul>
        </Card>

        <p className="mt-8 text-sm leading-relaxed text-muted">
          Hizmetin kuralları için{' '}
          <Link
            href="/kullanim-sartlari"
            className="text-brand-300 underline underline-offset-4"
          >
            kullanım şartları
          </Link>
          , diğer sorularınız için{' '}
          <Link href="/sss" className="text-brand-300 underline underline-offset-4">
            sık sorulan sorular
          </Link>{' '}
          ve{' '}
          <Link href="/iletisim" className="text-brand-300 underline underline-offset-4">
            iletişim
          </Link>{' '}
          sayfalarına bakabilirsiniz.
        </p>
      </div>
    </div>
  );
}
