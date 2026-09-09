import Link from 'next/link';
import type { Metadata } from 'next';
import { Card, Alert } from '@/components/ui';

export const metadata: Metadata = {
  // Sayfaya ÖZEL description şart: yoksa kök layout'un varsayılanı miras
  // alınır ve iki ayrı sayfa birebir aynı açıklamayı taşır (S1 ihlali).
  description:
    'Onay360 kullanım şartları: üyelik, bakiye yükleme, sipariş, iptal ve iade ' +
    'kuralları, tedarik ve stok, yasak kullanımlar ve sorumluluğun sınırı.',
  title: 'Kullanım Şartları',
  alternates: { canonical: '/kullanim-sartlari' },
};

/**
 * 🔴 BU METİN ÖZGÜNDÜR VE UYDURMA MADDE İÇERMEZ.
 *
 * Buradaki her madde SİSTEMİN BUGÜNKÜ DAVRANIŞIDIR; koda bakılarak
 * doğrulanabilir. Karşılığı olmayan hiçbir taahhüt yazılmadı. Özellikle
 * yazılmayanlar ve sebepleri `YOK_SAYILANLAR` dizisinde sayfanın kendisinde
 * de listelenir — okuyucu neyin eksik olduğunu görmelidir.
 *
 * Maddelerin dayanağı (örnekler):
 *   4.3 elle onay          → api/internal/service/deposit/service.go (paket başlığı)
 *   4.4 10 ₺ – 50.000 ₺    → api/internal/service/deposit/service.go tableMin/MaxAmountMinor
 *   5.1 teklif 120 sn      → api/internal/service/pricing/quote.go QuoteTTL
 *   5.4 sipariş oluşmazsa  → api/internal/service/order/service.go (T1/T2 deseni)
 *   6.1 otomatik tam iade  → api/internal/service/order/lifecycle.go refundOrder
 *   6.2 en az 120 sn       → api/internal/service/order/service.go defaultCancelGrace
 *   6.6 geç gelen kod      → order_integration_test.go#TestRefundedOrderDoesNotLeakCode
 *   7.3 Türkiye stoğu      → docs/memory.md (sağlayıcıda 122 serviste physical = 0)
 *
 * ⬇️ SATICI BİLGİSİ YER TUTUCUDUR — şirket kuruluşu tamamlanınca
 * `SATICI_BILGISI` doldurulmalıdır. Doldurulmadan hizmet kullanıma
 * AÇILMAMALIDIR: Mesafeli Sözleşmeler Yönetmeliği satıcı kimlik
 * bilgilerinin yayımlanmasını zorunlu kılar (docs/TESLIM.md §2.4).
 */
const SATICI_BILGISI: Array<[string, string]> = [
  ['Ticaret unvanı', ''],
  ['Adres', ''],
  ['Vergi dairesi / numarası', ''],
  ['MERSİS numarası', ''],
  ['KEP adresi', ''],
];
const saticiHazir = SATICI_BILGISI.every(([, deger]) => deger.length > 0);

type Bolum = { kimlik: string; baslik: string; maddeler: string[] };

/**
 * Bölümler VERİ olarak durur: numaralandırma ve içindekiler listesi tek
 * kaynaktan üretilir. Elle yazılsaydı araya bir madde eklendiğinde
 * numaralar ile içindekiler sessizce ayrışırdı.
 *
 * `kimlik` bağlantı çıpasıdır ve İNGİLİZCE'dir: Türkçe karakterli bir id
 * URL'de yüzde kodlamasına dönüşür ve paylaşılan bağlantı okunmaz olur.
 */
const BOLUMLER: Bolum[] = [
  {
    kimlik: 'taraflar',
    baslik: 'Taraflar ve bu metnin kapsamı',
    maddeler: [
      'Bu şartlar, Onay360 üzerinden hizmet alan kullanıcı ile hizmeti sunan ' +
        'işletme arasındaki kullanım koşullarını düzenler. Hesap açan ve hizmeti ' +
        'kullanan herkes bu şartları okumuş ve kabul etmiş sayılır.',
      'Hizmeti sunan işletmenin ticaret unvanı, adresi ve vergi bilgileri bu ' +
        'sayfanın sonundaki “Satıcı bilgileri” bölümünde yayımlanır.',
      'Şartlar değişirse güncel metin bu sayfada yayımlanır. Değişiklikten sonra ' +
        'hizmeti kullanmaya devam etmek, güncel metnin kabulü anlamına gelir.',
    ],
  },
  {
    kimlik: 'hizmet',
    baslik: 'Hizmetin tanımı',
    maddeler: [
      'Onay360, bir servise kayıt olurken gereken SMS onay kodunu alabilmeniz ' +
        'için geçici bir telefon numarası sağlar. Bu numara size tahsis edilmiş ' +
        'bir hat değildir; yalnızca belirli bir süre için ayrılır.',
      'Numara tek bir doğrulama içindir. Onay kodu ekranınıza düştüğünde sipariş ' +
        'tamamlanır ve numara kapatılır. Kapanmadan önce ulaşan mesajların tamamı ' +
        'sipariş ekranınızda görünür.',
      'Numara kalıcı değildir. Doğrulama tamamlandıktan sonra numara üzerinde ' +
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
    kimlik: 'tedarik',
    baslik: 'Tedarik, stok ve kesinti',
    maddeler: [
      'Numaralar bize ait bir şebekeden değil, numara sağlayıcılarından temin ' +
        'edilir. Bu nedenle bir servis–ülke eşleşmesinin stoğu bizim denetimimizde ' +
        'değildir ve önceden haber verilmeden tükenebilir.',
      'Stok ve fiyatlar gün içinde sürekli değişir. Listede görünen bir seçenek ' +
        'satın alma anında tükenmiş olabilir; bu durumda ücret tahsil edilmez.',
      'Türkiye numarası şu anda satışa sunulmamaktadır. Ülke listede görünse de ' +
        'stok bulunmadığı için seçilemez.',
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
      'Kişisel verilerin işlenmesine ilişkin ayrıntılar Gizlilik Politikası ' +
        'sayfasında yayımlanacaktır.',
    ],
  },
];

/**
 * Bu metinde BİLEREK yer almayan konular.
 *
 * Sayfada gösterilir: eksik olduğunu bilmediğiniz bir madde, yanlış yazılmış
 * bir maddeden daha tehlikelidir. Her satırın sebebi sistemin bugünkü
 * durumudur — hiçbiri "unutuldu" değildir.
 */
const YOK_SAYILANLAR: string[] = [
  'Cayma hakkı ve mesafeli satış bilgilendirmesi — dijital hizmetlerde ' +
    'uygulanacak istisnanın kapsamı hukuki inceleme sonrası yazılacaktır.',
  'Uygulanacak hukuk, yetkili mahkeme ve tüketici hakem heyeti — satıcı ' +
    'bilgileri yayımlanmadan yer belirtilemez.',
  'Kullanılmayan bakiyenin nakde çevrilmesi veya banka hesabına iadesi — ' +
    'bugün sistemde böyle bir işlem yoktur; bakiye yalnızca hizmet alımında ' +
    'kullanılır.',
  'Hesap kapatma ve kişisel verilerin silinmesi talebi — bugün bunun için ' +
    'kendi kendine işleyen bir akış yoktur; talepler destek üzerinden ele ' +
    'alınmaktadır.',
  'Kişisel verilerin saklanma süresi — süre henüz belirlenmemiştir ve ' +
    'Gizlilik Politikası ile birlikte yayımlanacaktır.',
  'Kiralık (uzun süreli) numara — bu ürün bugün satışta değildir; satışa ' +
    'açıldığında kendi maddeleri eklenecektir.',
];

export default function TermsPage() {
  return (
    <div className="px-4 py-10 md:px-6 md:py-14">
      <div className="mx-auto max-w-3xl">
        <h1 className="text-3xl font-bold tracking-tight md:text-4xl">Kullanım Şartları</h1>

        {/* Taslak uyarısı SAYFANIN EN ÜSTÜNDE: aşağı kaydırmadan görülmeli.
            Yasal metin uydurulmaz; bu metin hukuki inceleme bekliyor
            (docs/TESLIM.md §2.4). */}
        <Alert tone="warn" className="mt-6">
          <strong className="font-semibold">Bu metin taslaktır ve henüz yürürlüğe girmemiştir.</strong>{' '}
          Aşağıdaki maddeler hizmetin bugünkü işleyişini anlatır; hukuki inceleme
          tamamlanıp satıcı bilgileri yayımlanana kadar bağlayıcı sözleşme metni
          olarak kabul edilmemelidir. Eksik bırakılan konular sayfanın sonunda
          ayrıca listelenmiştir.
        </Alert>

        <p className="mt-6 text-base leading-relaxed text-muted md:text-lg">
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
                    <p className="text-sm leading-relaxed text-muted">{madde}</p>
                  </li>
                ))}
              </ol>
            </Card>
          </section>
        ))}

        <h2 className="mt-12 text-xl font-semibold md:text-2xl">Satıcı bilgileri</h2>
        {saticiHazir ? (
          <Card className="mt-4">
            <dl className="flex flex-col gap-3 text-sm">
              {SATICI_BILGISI.map(([etiket, deger]) => (
                <div key={etiket} className="flex flex-col gap-0.5 md:flex-row md:gap-4">
                  <dt className="text-muted md:w-56 md:shrink-0">{etiket}</dt>
                  <dd className="break-anywhere">{deger}</dd>
                </div>
              ))}
            </dl>
          </Card>
        ) : (
          <Card className="mt-4">
            {/* Boş bırakmak, uydurmaktan iyidir. Bu uyarı, bilgiler
                doldurulduğu anda kendiliğinden kaybolur. */}
            <Alert tone="warn">
              Ticaret unvanı, adres, vergi ve MERSİS bilgileri henüz yayımlanmadı;
              bu alan şirket kuruluşu tamamlandığında doldurulacaktır.
            </Alert>
          </Card>
        )}

        <h2 className="mt-12 text-xl font-semibold md:text-2xl">
          Bu metinde henüz yer almayan konular
        </h2>
        <p className="mt-2 text-sm leading-relaxed text-muted">
          Aşağıdakiler bilerek boş bırakıldı: bugün sistemde karşılığı olmayan bir
          taahhüdü yazmaktansa, eksik olduğunu açıkça belirtmeyi tercih ediyoruz.
        </p>
        <Card className="mt-4">
          <ul className="flex list-disc flex-col gap-2 pl-5 text-sm leading-relaxed text-muted">
            {YOK_SAYILANLAR.map((satir, i) => (
              <li key={i}>{satir}</li>
            ))}
          </ul>
        </Card>

        <p className="mt-8 text-sm leading-relaxed text-muted">
          Bir maddeyle ilgili sorunuz varsa{' '}
          <Link href="/iletisim" className="text-brand-300 underline underline-offset-4">
            iletişim
          </Link>{' '}
          sayfasından bize ulaşabilir,{' '}
          <Link href="/gizlilik" className="text-brand-300 underline underline-offset-4">
            gizlilik politikası
          </Link>{' '}
          ve{' '}
          <Link href="/sss" className="text-brand-300 underline underline-offset-4">
            sık sorulan sorular
          </Link>{' '}
          sayfalarına da bakabilirsiniz.
        </p>
      </div>
    </div>
  );
}
