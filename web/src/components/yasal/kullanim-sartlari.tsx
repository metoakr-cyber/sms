import Link from 'next/link';
import { Card } from '@/components/ui';
import { yasalYol, type YasalYuzey } from './yuzey';

/**
 * Kullanım Şartları — YÜRÜRLÜKTEKİ METNİN TEK KAYNAĞI.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * NEDEN ORTAK BİR DOSYA
 * ══════════════════════════════════════════════════════════════════════════
 * Bu metin iki yerde görünür: genel sitede (`/kullanim-sartlari`) ve panelin
 * içinde (`/panel/kullanim-sartlari`). Aynı hukuki metin iki dosyada dursaydı
 * biri güncellenip diğeri unutulurdu ve kullanıcının okuduğu metin ile
 * yürürlükteki metin ayrışırdı. Bu, bu depoda ÖLÇÜLMÜŞ bir hata sınıfıdır:
 * `veri-tablosu.tsx` başlığı, aynı verinin iki yerde farklı etiketlendiği üç
 * somut vakayı sayıyor. Hukuki metinde bedeli daha ağırdır — sözleşmenin eski
 * sürümünü okutmuş oluruz.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * NEDEN "SADECE VERİ" DEĞİL, SUNUMLA BİRLİKTE PAYLAŞILDI
 * ══════════════════════════════════════════════════════════════════════════
 * İlk akla gelen ayrıştırma şuydu: `BOLUMLER` dizisi ortak bir modülde dursun,
 * her sayfa kendi sunumunu yazsın. Bu YETMEZ — çünkü metnin içinde bölüm
 * NUMARASIYLA atıf var: "6. bölümdeki kurallar", "7. bölümdedir", "2.
 * bölümdeki uyarı". O numaralar sunum sırasında dizinin indisinden üretilir.
 * Sunum iki dosyada dursaydı, birinde sıra veya numaralandırma değiştiği anda
 * atıflar sessizce başka bir bölüme işaret ederdi; ne tip denetimi ne de bir
 * test bunu görür. İçindekiler listesi ile başlık numaralarının eşleşmesi de
 * aynı riski taşır.
 *
 * Bu yüzden paylaşılan birim `h1` HARİÇ gövdenin tamamıdır — bu bileşenin
 * içinde `h1` yoktur. Başlığı iki yüzey bilerek farklı çiziyor: genel sayfa
 * pazarlama ölçeğinde kendi `h1`ini, panel `SayfaBasligi`yi kullanıyor.
 * Amaç sayfa başına tek `h1` (frontend-contract §10.2 · S2); ölçen araç
 * `web/scripts/seo-check.mjs`, elle çalıştırılır.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * GENİŞLİK
 * ══════════════════════════════════════════════════════════════════════════
 * Bu bileşen KENDİ yerleşim kabını açmaz (`mx-auto max-w-*` yok). Genel sayfa
 * kendi kabını verir; panelde içerik tam genişliktir (`panel-shell.tsx`).
 * Düz metin satırları burada `max-w-[70ch]` ile sınırlanır — o bir kap değil,
 * okunabilir satır uzunluğu sınırıdır (frontend-contract §3.4) ve iki yüzeyde
 * de geçerlidir.
 */

/**
 * 🔴 BU METİN YÜRÜRLÜKTEDİR. Taslak değildir.
 *
 * Buradaki her madde SİSTEMİN BUGÜNKÜ DAVRANIŞIDIR; koda bakılarak
 * doğrulanabilir. Karşılığı olmayan hiçbir taahhüt yazılmadı. Bilerek
 * yazılmayan konular `KAPSAM_DISI` dizisinde sayfanın kendisinde de
 * listelenir — okuyucu neyin kapsam dışı olduğunu görmelidir.
 *
 * ⚠️ BİR MADDEYİ DEĞİŞTİRMEDEN ÖNCE: metin yürürlükte olduğu için buradaki
 * her cümle bir taahhüttür. Sistemin davranışı değişirse ÖNCE bu dosya
 * güncellenir; sistem sözleşmenin gerisinde kalamaz.
 *
 * Maddelerin dayanağı (örnekler):
 *   3.2 e-posta kapısı     → transport/http/router.go RequireVerifiedEmail
 *   4.3 elle onay          → api/internal/service/deposit/service.go (paket başlığı)
 *   4.4 10 ₺ – 50.000 ₺    → api/internal/service/deposit/service.go tableMin/MaxAmountMinor
 *   4.9 nakde çevrilmez    → router.go'da para çekme ucu YOK (yalnız GET /wallet/*)
 *   5.1 teklif 120 sn      → api/internal/service/pricing/quote.go QuoteTTL
 *   5.4 sipariş oluşmazsa  → api/internal/service/order/service.go (T1/T2 deseni)
 *   6.1 otomatik tam iade  → api/internal/service/order/lifecycle.go refundOrder
 *   6.2 en az 120 sn       → api/internal/service/order/service.go defaultCancelGrace
 *   6.6 geç gelen kod      → order_integration_test.go#TestRefundedOrderDoesNotLeakCode
 *   7.2 sabit süre kademesi→ port/provider.go RentalProvider.AllowedDurations
 *   7.4 bitiş zamanı       → order/service.go persist (expires_at = sağlayıcı expiredAt)
 *   7.5 uzatma yok         → adapter'da Extend var ama HİÇBİR uç nokta çağırmıyor
 *   7.7 önbellekten fiyat  → pricing/quote.go cheapestOffer(live=false) + worker rental-sync
 *
 * 🔴 SATICI KİMLİK BİLGİSİ (ticaret unvanı, adres, vergi no, MERSİS) ELİMİZDE
 * YOK ve UYDURULMAZ. Yer tutucu da yazılmaz: boş köşeli parantezlerle dolu bir
 * sözleşme, bilgi vermemekten daha kötüdür. Bu eksiklik `KAPSAM_DISI`
 * listesinde okuyucuya açıkça söylenir. Bilgiler edinildiğinde İLETISIM
 * bloğuna satır eklenir.
 */
const ILETISIM: Array<[string, React.ReactNode]> = [
  ['İşletme adı', 'Onay360'],
  [
    'E-posta',
    <a
      key="eposta"
      href="mailto:metoakr@gmail.com"
      className="text-brand-300 underline underline-offset-4"
    >
      metoakr@gmail.com
    </a>,
  ],
  [
    'Destek',
    <>
      Hesabınızdan destek talebi açabilirsiniz;{' '}
      <Link href="/iletisim" className="text-brand-300 underline underline-offset-4">
        iletişim sayfası
      </Link>{' '}
      üzerinden de yazabilirsiniz.
    </>,
  ],
];

type Bolum = { kimlik: string; baslik: string; maddeler: string[] };

/**
 * Bölümler VERİ olarak durur: numaralandırma ve içindekiler listesi tek
 * kaynaktan üretilir. Elle yazılsaydı araya bir madde eklendiğinde
 * numaralar ile içindekiler sessizce ayrışırdı.
 *
 * `kimlik` bağlantı çıpasıdır ve İNGİLİZCE'dir: Türkçe karakterli bir id
 * URL'de yüzde kodlamasına dönüşür ve paylaşılan bağlantı okunmaz olur.
 *
 * 🔴 SIRA DEĞİŞTİRİLMEZ. Metin içinde "6. bölüm" gibi ATIFLAR var; bir bölümü
 * araya sokmak bu atıfları sessizce yanlış hedefe çevirir. Kiralama bölümü bu
 * yüzden iadeden SONRA eklendi — iade 6 olarak kaldı.
 */
const BOLUMLER: Bolum[] = [
  {
    kimlik: 'taraflar',
    baslik: 'Taraflar ve bu metnin kapsamı',
    maddeler: [
      'Bu şartlar, Onay360 adıyla sunulan sanal numara hizmetini kullanan kişi ' +
        'ile hizmeti sunan işletme arasındaki kullanım koşullarını düzenler. ' +
        'Hesap açan ve hizmeti kullanan herkes bu şartları okumuş ve kabul ' +
        'etmiş sayılır.',
      'İşletmeye e-posta ile ulaşılır: metoakr@gmail.com. Bu metne ilişkin ' +
        'soru, itiraz ve talepler bu adrese yapılır; hesabı olan kullanıcılar ' +
        'aynı talepleri panelden destek talebi açarak da iletebilir. İletişim ' +
        'bilgileri sayfanın sonundaki “İletişim” bölümünde de yer alır.',
      'Bu metin yürürlüktedir ve güncel hâli her zaman bu sayfada bulunur. ' +
        'Şartlar değişirse yeni metin burada yayımlanır; değişiklikten sonra ' +
        'hizmeti kullanmaya devam etmek güncel metnin kabulü anlamına gelir.',
      'Metnin kapsamı dışında bıraktığımız konular sayfanın sonunda ayrıca ' +
        'listelenmiştir. Orada sayılan bir konuda bu metin taahhüt içermez.',
    ],
  },
  {
    kimlik: 'hizmet',
    baslik: 'Hizmetin tanımı',
    maddeler: [
      'Onay360, bir servise kayıt olurken veya giriş yaparken gereken SMS onay ' +
        'kodunu alabilmeniz için geçici bir telefon numarası sağlar. Bu numara ' +
        'size tahsis edilmiş bir hat değildir; yalnızca belirli bir süre için ' +
        'ayrılır.',
      'İki ayrı ürün vardır: tek kullanımlık numara ve kiralık numara. Tek ' +
        'kullanımlık numara bir doğrulama içindir. Kiralık numaraya ilişkin ' +
        'kurallar 7. bölümdedir.',
      'Tek kullanımlık siparişte onay kodu ekranınıza düştüğünde sipariş ' +
        'tamamlanır ve numara kapatılır. Kapanmadan önce numaraya ulaşan ' +
        'mesajların tamamı sipariş ekranınızda görünür.',
      'Numara kalıcı değildir. Sipariş kapandıktan sonra numara üzerinde ' +
        'hiçbir hakkınız kalmaz ve numara ileride başka bir kullanıcıya ' +
        'verilebilir.',
      '🔴 Bu nedenle sanal numarayı hesabınızın kurtarma numarası, iki adımlı ' +
        'doğrulama numarası veya kalıcı iletişim numarası olarak KULLANMAYIN. ' +
        'Numara elinizden çıktıktan sonra o hesaba erişiminizi kaybedebilirsiniz; ' +
        'bu risk size aittir.',
      'Onay360 aracı bir hizmettir. Numarayı kullandığınız servisin (örneğin bir ' +
        'mesajlaşma uygulamasının) kendi kurallarının yerine geçmez ve o servisin ' +
        'sanal numaraları kabul edip etmemesi bizim denetimimizde değildir.',
    ],
  },
  {
    kimlik: 'uyelik',
    baslik: 'Üyelik ve hesap',
    maddeler: [
      'Hizmet 18 yaşından küçükler tarafından kullanılamaz.',
      'Kayıt sırasında e-posta doğrulaması zorunludur. Hesabınız doğrulanana ' +
        'kadar fiyat teklifi alamaz, numara satın alamaz ve bakiye yükleyemezsiniz.',
      'Doğrulama e-postası size ulaşmadıysa destek talebi açabilirsiniz; destek ' +
        'talebi açmak için e-posta doğrulaması gerekmez.',
      'Şifreniz sistemde geri döndürülemez biçimde (özet olarak) saklanır; ' +
        'personelimiz dâhil kimse şifrenizi göremez. Şifrenizin içinde e-posta ' +
        'adresiniz veya kullanıcı adınız geçemez.',
      'Hesabınızın güvenliğinden siz sorumlusunuz. Hesabınızı paylaşmayın; ' +
        'hesabınızdan yapılan işlemler size ait sayılır.',
      'Kayıt ve giriş adımlarında otomatik istekleri ayırt etmek için robot ' +
        'denetimi (reCAPTCHA) uygulanabilir.',
      'Bu şartlara aykırı kullanım tespit edilirse hesap askıya alınabilir. ' +
        'Askıya alınan hesabın açık oturumları anında sona erer ve yeniden giriş ' +
        'yapılamaz.',
      'Hesabınızı kapatmak isterseniz talebinizi destek üzerinden veya ' +
        'metoakr@gmail.com adresine iletin. Hesap kapatma kendi kendine işleyen ' +
        'bir akış değildir; talep elle ele alınır. Kapatma sonrasında hangi ' +
        'kayıtların ne kadar süreyle saklandığı Gizlilik Politikası’nda açıklanır.',
    ],
  },
  {
    kimlik: 'bakiye',
    baslik: 'Bakiye yükleme ve ödeme',
    maddeler: [
      'Hizmet ön ödemelidir. Harcamalar yalnızca hesabınızdaki TL bakiyeden ' +
        'düşer. Bakiyeniz yetmiyorsa işlem başlamaz; borçlanma, fatura veya ' +
        'sonradan tahsilat diye bir durum yoktur.',
      'Bakiye yükleme yöntemleri banka havalesi / EFT ve USDT (TRC-20) ile ' +
        'sınırlıdır. Kredi kartıyla ödeme, otomatik ödeme veya abonelik yoktur; ' +
        'sizden kart bilgisi istenmez ve saklanmaz.',
      'Yükleme anında gerçekleşmez. Talebiniz önce “beklemede” olarak açılır; ' +
        'ödeme kontrol edildikten sonra bakiyeniz elle tanımlanır.',
      'Tek seferde en az 10,00 ₺, en çok 50.000,00 ₺ yükleyebilirsiniz. Seçtiğiniz ' +
        'yöntem için daha dar sınırlar uygulanabilir; geçerli sınırlar yükleme ' +
        'ekranında gösterilir.',
      'Banka havalesi / EFT ile yüklemede dekont yüklemesi akışın parçasıdır. ' +
        'Dekont dosyaları site üzerinden doğrudan erişilemeyecek biçimde saklanır ' +
        've yalnızca talebi inceleyen yetkili görüntüleyebilir.',
      'Bildirdiğiniz tutar ile bakiyenize yazılan tutar farklı olabilir: özellikle ' +
        'kripto yüklemelerinde ağ ücreti düşüldükten sonra hesaba GEÇEN tutar ' +
        'esas alınır. Bakiyenize yazılan tutar cüzdan hareketlerinizde görünür.',
      'Talebiniz reddedilirse gerekçesi hesabınızda gösterilir ve bakiyeniz ' +
        'değişmez.',
      'Yükleme yöntemleri geçici olarak kapatılabilir. Kapalı bir yöntem yükleme ' +
        'ekranında listelenmez.',
      'Bakiye yalnızca Onay360 üzerinden hizmet almak için kullanılır. Bakiye ' +
        'nakde çevrilmez, banka hesabına veya kripto cüzdanına aktarılmaz ve ' +
        'başka bir hesaba devredilemez.',
    ],
  },
  {
    kimlik: 'siparis',
    baslik: 'Fiyat, sipariş ve süre',
    maddeler: [
      'Ödeyeceğiniz tutarı numarayı almadan ÖNCE görürsünüz. Size gösterilen ' +
        'fiyat teklifi tek kullanımlıktır ve 120 saniye geçerlidir; süre dolarsa ' +
        'güncel fiyatla yeni bir teklif alınır.',
      'Satın alma, teklifte gösterilen tutarla yapılır. Sipariş sonrasında ek ' +
        'ücret, komisyon veya sürpriz tahsilat çıkmaz.',
      'Satın alma anında tutar bakiyenizden düşülür ve bu düşüş cüzdan ' +
        'hareketlerinizde ayrı bir kayıt olarak görünür.',
      'Numara ayrılamazsa (örneğin stok o an tükenmişse) sipariş hiç oluşmaz ve ' +
        'düşülen tutar aynı işlem içinde bakiyenize geri yüklenir.',
      'Numaranın geçerlilik süresi sabit değildir; her sipariş için ekranda ' +
        'gösterilen kalan süre geçerlidir. Süre sunucu saatine göre hesaplanır, ' +
        'cihazınızın saatine göre değil.',
      'Kiralık numarada fiyat teklifi alabilmek için süreyi de seçmeniz gerekir. ' +
        'Gösterilen tutar, seçtiğiniz sürenin tamamı içindir.',
    ],
  },
  {
    kimlik: 'iade',
    baslik: 'İptal ve iade',
    maddeler: [
      'Numaranın süresi onay kodu gelmeden dolarsa ödediğiniz tutar bakiyenize ' +
        'TAM olarak iade edilir. Bunun için talep açmanız gerekmez; iade sistem ' +
        'tarafından yapılır.',
      'Bekleyen bir siparişi satın almadan en az 120 saniye sonra kendiniz iptal ' +
        'edebilirsiniz. İlk iki dakika içinde iptal edilemez; bu sınır numara ' +
        'sağlayıcısından kaynaklanır. İptal ettiğinizde tutar bakiyenize tam iade ' +
        'edilir.',
      'İadeler her zaman ödenen tutarın tamamıdır. Kısmi iade uygulanmaz ve iade ' +
        'tutarı güncel kurla yeniden hesaplanmaz.',
      'İadeler hesabınızdaki bakiyeye yapılır. Banka hesabına veya kripto ' +
        'cüzdanına geri ödeme yapılmaz.',
      'Onay kodu size iletildikten sonra hizmet ifa edilmiş sayılır; bu siparişler ' +
        'için iade yapılmaz.',
      'İadesi yapılmış bir siparişe sonradan kod ulaşırsa bu kod size ' +
        'gösterilmez. Aksi hâlde hem ücret iade edilmiş hem de doğrulama ' +
        'yapılabilmiş olurdu.',
      'Her yükleme, her satın alma ve her iade cüzdan hareketlerinizde ayrı ayrı ' +
        'görünür; bakiyenizin nasıl oluştuğunu adım adım izleyebilirsiniz.',
    ],
  },
  {
    kimlik: 'kiralama',
    baslik: 'Kiralık numara',
    maddeler: [
      'Kiralık numara, seçtiğiniz süre boyunca hesabınıza ayrılan numaradır. ' +
        'Tek kullanımlık numaradan ayrı bir üründür ve ayrı fiyatlandırılır.',
      'Kiralama süresi serbestçe belirlenmez: numara sağlayıcısının kabul ettiği ' +
        'sabit süre kademelerinden birini seçersiniz. Seçilebilir süreler ' +
        'sağlayıcıdan alınır ve satın alma ekranında listelenir.',
      'Ücret sürenin tamamı için peşin alınır ve satın alma anında bakiyenizden ' +
        'düşer.',
      'Kiralamanın bitiş zamanı numara sağlayıcısının bildirdiği süreye göre ' +
        'belirlenir. Kalan süre sipariş ekranınızda sunucu saatine göre gösterilir.',
      'Kiralama süresi uzatılamaz. Süre dolduktan sonra aynı numarayı kullanmaya ' +
        'devam edemezsiniz; yeni bir kiralama siparişi vermeniz gerekir ve size ' +
        'aynı numaranın verileceği garanti edilmez.',
      'Süre dolduğunda numara kapanır ve numara üzerinde hiçbir hakkınız kalmaz. ' +
        'Kullanılmayan süre için kısmi iade yapılmaz.',
      'Kiralık numaraların fiyat ve stok bilgisi, sağlayıcıdan düzenli aralıklarla ' +
        'alınan bir kopyadan gösterilir; anlık değildir. Listede görünen bir ' +
        'seçenek satın alma anında tükenmiş olabilir. Bu durumda sipariş oluşmaz ' +
        've ücret tahsil edilmez.',
      'Bir servis veya ülke için kiralık numara sunulmuyorsa satın alma ekranında ' +
        'kiralama seçeneği görünmez.',
      'İptal ve iade konusunda 6. bölümdeki kurallar kiralık numaralar için de ' +
        'aynen geçerlidir. 2. bölümdeki uyarı da geçerlidir: kiralık numara da ' +
        'kalıcı değildir ve kurtarma numarası olarak kullanılmamalıdır.',
    ],
  },
  {
    kimlik: 'tedarik',
    baslik: 'Tedarik, stok ve kesinti',
    maddeler: [
      'Numaralar bize ait bir şebekeden değil, numara sağlayıcılarından temin ' +
        'edilir. Bu nedenle bir servis–ülke eşleşmesinin stoğu bizim denetimimizde ' +
        'değildir ve önceden haber verilmeden tükenebilir.',
      'Stok ve fiyatlar gün içinde sürekli değişir. Listede görünen bir seçenek ' +
        'satın alma anında tükenmiş olabilir; bu durumda ücret tahsil edilmez.',
      'Belirli bir ülkenin veya servisin stokta bulunacağı taahhüt edilmez. Bir ' +
        'ülke ya da servis uzun süre hiç stoksuz kalabilir; stoğu olmayan seçenek ' +
        'satın alınamaz.',
      'Belirli bir servis ya da ülke için numara bulunacağı, onay kodunun ' +
        'ulaşacağı veya hedef servisin numarayı kabul edeceği garanti edilmez. ' +
        'Kod gelmediğinde uygulanacak tek çözüm 6. bölümdeki ücret iadesidir.',
      'Kesintisiz çalışma taahhüdü verilmez. Bakım, sağlayıcı arızası veya teknik ' +
        'nedenlerle hizmet geçici olarak durabilir; belirli bir çalışma süresi ' +
        'oranı (SLA) taahhüt edilmemektedir.',
    ],
  },
  {
    kimlik: 'yasak',
    baslik: 'Yasak kullanımlar',
    maddeler: [
      'Hizmetin yürürlükteki mevzuata aykırı biçimde kullanılması yasaktır.',
      'Özellikle şunlar yasaktır: dolandırıcılık; başka bir kişi, kurum ya da ' +
        'markanın kimliğine bürünmek; başkasına ait hesaba izinsiz erişmek veya ' +
        'erişmeye çalışmak; istenmeyen toplu mesaj göndermek; taciz, tehdit ve ' +
        'yasa dışı içerik ya da ürün ticareti.',
      'Başka bir platformun güvenlik önlemlerini, kullanım şartlarını veya hesap ' +
        'sınırlarını dolanmak amacıyla toplu hesap üretmek yasaktır.',
      'Hizmete otomatik araçlarla aşırı istek göndermek, sistemi yavaşlatmaya ya ' +
        'da işlevsiz bırakmaya çalışmak, güvenlik açığı aramak ve bulduğunuz bir ' +
        'açığı bildirmek yerine kullanmak yasaktır.',
      'Aykırılık hâlinde sipariş reddedilebilir, hesap askıya alınabilir ve ' +
        'mevzuatın gerektirdiği hâllerde yetkili makamlara bilgi verilir. Askıya ' +
        'alınan hesabın devam eden siparişleri için 6. bölümdeki iade kuralları ' +
        'aynen uygulanır.',
    ],
  },
  {
    kimlik: 'sorumluluk',
    baslik: 'Sorumluluğun sınırı',
    maddeler: [
      'Numarayı kullandığınız servislerin adları ve markaları o servislere aittir; ' +
        'Onay360 bu servislerle ortak, bayi ya da yetkili değildir. Marka adları ' +
        'yalnızca hangi servis için numara alındığını göstermek amacıyla kullanılır.',
      'Sanal numarayla açılan bir hesabın hedef servis tarafından kapatılmasından, ' +
        'doğrulamanın geçersiz sayılmasından veya numaranın sonradan başka birine ' +
        'verilmesinden doğan zararlardan sorumlu değiliz; numaranın geçici olduğu ' +
        'satın alma öncesinde bilinmektedir.',
      'Sağlayıcı kesintisi, şebeke ve altyapı arızaları ile denetimimiz dışındaki ' +
        'benzeri sebeplerden kaynaklanan gecikme ve aksaklıklardan sorumlu ' +
        'değiliz.',
      'Sorumluluğumuz her hâlükârda ilgili siparişe ödediğiniz tutarla sınırlıdır. ' +
        'Tüketici mevzuatından doğan haklarınız saklıdır.',
    ],
  },
  {
    kimlik: 'veri',
    baslik: 'Kişisel veriler',
    maddeler: [
      'Hesap açmak için ad-soyad, adres, T.C. kimlik numarası, doğum tarihi veya ' +
        'kendi telefon numaranız istenmez. E-posta adresi ve kullanıcı adı ' +
        'yeterlidir.',
      'Satın aldığınız numara ve o numaraya gelen mesajların içeriği sipariş ' +
        'kaydınızda saklanır ve sipariş geçmişinizde görünür.',
      'Hesap güvenliği için oturum kayıtlarında IP adresi ve tarayıcı bilgisi ' +
        'tutulur.',
      'Kişisel verilerin hangi amaçla işlendiği, kimlere aktarıldığı ve ne kadar ' +
        'süreyle saklandığı Gizlilik Politikası sayfasında açıklanır.',
      'Kişisel verilerinize ilişkin başvurularınızı metoakr@gmail.com adresine ' +
        'iletebilirsiniz.',
    ],
  },
  {
    kimlik: 'hukuk',
    baslik: 'Cayma hakkı, uygulanacak hukuk ve başvuru yolları',
    maddeler: [
      'Bu şartlara Türkiye Cumhuriyeti hukuku uygulanır.',
      'Mesafeli Sözleşmeler Yönetmeliği’nin 15. maddesi, elektronik ortamda anında ' +
        'ifa edilen hizmetleri cayma hakkının istisnaları arasında sayar. Onay ' +
        'kodunun size iletilmesiyle hizmet ifa edilmiş olur; bu nedenle kodu ' +
        'iletilmiş siparişlerde cayma hakkı kullanılamaz.',
      'Kod gelmeyen siparişlerde 6. bölümdeki tam iade, cayma hakkından bağımsız ' +
        'olarak ve sizden talep beklenmeden uygulanır.',
      '6502 sayılı Tüketicinin Korunması Hakkında Kanun kapsamındaki ' +
        'uyuşmazlıklarda, Ticaret Bakanlığı’nca her yıl belirlenen parasal ' +
        'sınırların altındaki başvurular tüketici hakem heyetlerine, bu sınırların ' +
        'üzerindeki uyuşmazlıklar tüketici mahkemelerine yapılır. Tüketici, ' +
        'başvurusunu kendi yerleşim yerindeki veya işlemin yapıldığı yerdeki ' +
        'hakem heyetine ya da mahkemeye yapabilir.',
      'Bu metnin bir maddesinin geçersiz sayılması diğer maddeleri etkilemez; ' +
        'kalan maddeler yürürlükte kalır.',
    ],
  },
];

/**
 * Bu metnin BİLEREK kapsamadığı konular.
 *
 * Sayfada gösterilir: kapsam dışı olduğunu bilmediğiniz bir konu, yanlış
 * yazılmış bir maddeden daha tehlikelidir. Her satır BUGÜNÜN durumunu anlatır —
 * hiçbiri "sonra yazacağız" değildir. Bir satır ancak sistem değiştiğinde ve
 * ilgili madde metne EKLENDİĞİNDE buradan çıkar.
 */
const KAPSAM_DISI: string[] = [
  'Ticaret unvanı, adres, vergi numarası ve MERSİS numarası — bu bilgiler bu ' +
    'sayfada yayımlanmamaktadır. İşletmeye yukarıdaki e-posta adresinden ulaşılır.',
  'Kiralama süresi boyunca alınabilecek mesaj sayısı — bu metin kiralık numara ' +
    'için belirli bir mesaj sayısı taahhüt etmez.',
];

/**
 * Metnin gövdesi — `h1` HARİÇ her şey.
 *
 * `className` yalnız ÜST BOŞLUK içindir: genel sayfada `h1`in altındaki
 * aralığı, panelde sayfa kabının `gap`i verir. Bileşen kendi üst boşluğunu
 * varsaymaz.
 */
export function KullanimSartlariMetni({
  yuzey,
  className,
}: {
  yuzey: YasalYuzey;
  className?: string;
}) {
  return (
    <div className={className}>
      <p className="max-w-[70ch] text-base leading-relaxed text-muted md:text-lg">
        Onay360, bir servise kayıt olurken gereken SMS onay kodunu almanız için
        geçici numara sağlayan ön ödemeli bir hizmettir. Aşağıdaki maddeler
        hizmeti nasıl kullanabileceğinizi, paranızın nasıl işlendiğini ve hangi
        durumlarda iade aldığınızı açıklar.
      </p>

      {/* İçindekiler: metin uzun; hangi maddeyi aradığını bilen kullanıcı
          kaydırmadan gitmeli. Bağlantılar sunucuda üretilir, JS gerekmez. */}
      <nav aria-labelledby="icindekiler-basligi" className="mt-8">
        <h2 id="icindekiler-basligi" className="text-xl font-semibold md:text-2xl">
          İçindekiler
        </h2>
        <Card className="mt-4">
          {/* Kendi başına duran gezinme bağlantıları: satırın TAMAMI
              tıklanabilir ve ≥ 44 px yüksekliğinde, aralarında ≥ 8 px
              boşluk var (frontend-contract §2.3). Numara bağlantının
              İÇİNDE: ayrı bir <span> olsaydı 44 px'lik hedefin dışında
              kalırdı. */}
          <ol className="grid gap-2 md:grid-cols-2">
            {BOLUMLER.map((b, i) => (
              <li key={b.kimlik}>
                <Link
                  href={`#${b.kimlik}`}
                  className="flex min-h-11 items-center gap-2 py-1 text-sm text-brand-300"
                >
                  {/* Altı çizili olan YALNIZ başlık: `text-decoration`
                      satır içi çocuklara geçer ve çocuktaki `no-underline`
                      onu KALDIRMAZ (CSS). Çizgiyi bağlantının tamamına
                      verirsek madde numarası da çizilir. */}
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
          {/* scroll-mt: başlık yapışkan başlığın (sticky header) altında
              kalmasın — çıpaya atlayan kullanıcı başlığı görmeli. Aynı id
              hem çıpa hem erişilebilirlik etiketi: iki ayrı id tutmak
              birinin sessizce ayrışması demektir. */}
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
                  {/* Madde numarası ekran okuyucudan gizli DEĞİL: "6.2'ye
                      bakın" diyen bir destek yanıtı ancak numara
                      okunabiliyorsa işe yarar. */}
                  <span className="w-8 shrink-0 text-sm tabular-nums text-muted">
                    {i + 1}.{j + 1}
                  </span>
                  <p className="max-w-[70ch] text-sm leading-relaxed text-muted">{madde}</p>
                </li>
              ))}
            </ol>
          </Card>
        </section>
      ))}

      <h2 className="mt-12 text-xl font-semibold md:text-2xl">İletişim</h2>
      <p className="mt-2 max-w-[70ch] text-sm leading-relaxed text-muted">
        Bu metne ilişkin bildirim, itiraz ve talepler aşağıdaki kanallardan
        yapılır.
      </p>
      <Card className="mt-4">
        <dl className="flex flex-col gap-3 text-sm">
          {ILETISIM.map(([etiket, deger]) => (
            <div key={etiket} className="flex flex-col gap-0.5 md:flex-row md:gap-4">
              <dt className="text-muted md:w-56 md:shrink-0">{etiket}</dt>
              {/* break-anywhere: uzun bir e-posta adresi 320 px'lik ekranda
                  kutuyu yatay kaydırmaya zorlamasın. */}
              <dd className="max-w-[70ch] break-anywhere">{deger}</dd>
            </div>
          ))}
        </dl>
      </Card>

      <h2 className="mt-12 text-xl font-semibold md:text-2xl">
        Bu metnin kapsamı dışında kalanlar
      </h2>
      <p className="mt-2 max-w-[70ch] text-sm leading-relaxed text-muted">
        Aşağıdakiler bilerek kapsam dışında bırakıldı: bugün sistemde karşılığı
        olmayan bir taahhüdü yazmaktansa, kapsam dışı olduğunu açıkça belirtmeyi
        tercih ediyoruz.
      </p>
      <Card className="mt-4">
        <ul className="flex max-w-[70ch] list-disc flex-col gap-2 pl-5 text-sm leading-relaxed text-muted">
          {KAPSAM_DISI.map((satir, i) => (
            <li key={i}>{satir}</li>
          ))}
        </ul>
      </Card>

      <p className="mt-8 max-w-[70ch] text-sm leading-relaxed text-muted">
        Bir maddeyle ilgili sorunuz varsa{' '}
        <Link href="/iletisim" className="text-brand-300 underline underline-offset-4">
          iletişim
        </Link>{' '}
        sayfasından bize ulaşabilir,{' '}
        <Link
          href={yasalYol(yuzey, 'gizlilik')}
          className="text-brand-300 underline underline-offset-4"
        >
          gizlilik politikası
        </Link>{' '}
        ve{' '}
        <Link href="/sss" className="text-brand-300 underline underline-offset-4">
          sık sorulan sorular
        </Link>{' '}
        sayfalarına da bakabilirsiniz.
      </p>
    </div>
  );
}
