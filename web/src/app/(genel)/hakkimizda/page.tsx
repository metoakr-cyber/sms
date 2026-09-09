import Link from 'next/link';
import type { Metadata } from 'next';
import { Button, Card, Alert } from '@/components/ui';

export const metadata: Metadata = {
  title: 'Hakkımızda',
  description:
    'Onay360 nedir, nasıl çalışır ve hangi ilkelerle işler? Ön ödemeli bakiye, ' +
    'tek kullanımlık numara ve kod gelmediğinde otomatik iade.',
  alternates: { canonical: '/hakkimizda' },
};

/**
 * 🔴 BU SAYFADA UYDURMA BİLGİ YOKTUR.
 *
 * Kuruluş yılı, çalışan sayısı, müşteri sayısı, ofis adresi, "en büyük",
 * "%99,9 çalışma süresi" gibi hiçbir iddia yazılmadı — hiçbiri bugün
 * doğrulanabilir değil ve yanlış olanı yazmak hem kullanıcıyı hem de
 * (ticari iletişim mevzuatı açısından) bizi bağlar.
 *
 * Anlatılanların TAMAMI koda bakılarak doğrulanabilir davranışlardır ve
 * sayfadaki iki ana söz gerçek testlere bağlıdır:
 *   "kod gelmezse ücret iade edilir"
 *     test: api/internal/service/order/order_integration_test.go#TestExpiredOrderRefundsWithoutUserAction
 *   "teklifteki fiyattan fazlası tahsil edilmez"
 *     test: api/internal/service/order/order_integration_test.go#TestMaxPriceIsAlwaysSent
 * (CLAUDE.md değişmez #9 ve #21 · docs/trd.md)
 *
 * ⬇️ ŞİRKET BİLGİSİ YER TUTUCUDUR — şirket kuruluşu tamamlanınca
 * `SIRKET_BILGISI` doldurulmalı ve `bilgiHazir` true yapılmalıdır.
 * Doldurulmadan hizmet kullanıma AÇILMAMALIDIR: Mesafeli Sözleşmeler
 * Yönetmeliği satıcı kimlik bilgilerinin yayımlanmasını zorunlu kılar.
 */
const SIRKET_BILGISI: Array<[string, string]> = [
  ['Ticaret unvanı', ''],
  ['Adres', ''],
  ['Vergi dairesi / numarası', ''],
  ['MERSİS numarası', ''],
  ['KEP adresi', ''],
];
const bilgiHazir = SIRKET_BILGISI.every(([, deger]) => deger.length > 0);

// schema.org MUTLAK URL ister; göreli yol veren bir Organization bloğu
// Rich Results testinde sessizce yok sayılır.
const SITE = process.env.NEXT_PUBLIC_SITE_URL ?? 'http://localhost:3000';

const NASIL_CALISIR: Array<[string, string]> = [
  ['Bakiye yüklersiniz',
   'Hesabınıza ön ödemeli TL bakiyesi yüklersiniz. Her işlem bu bakiyeden ' +
   'düşer; kayıtlı kart saklamayız ve abonelik başlatmayız.'],
  ['Servis ve ülke seçersiniz',
   'Numarayı almadan önce ödeyeceğiniz tutarı ve stok durumunu görürsünüz. ' +
   'Fiyat satın alma anında sabitlenir.'],
  ['Numara size özel ayrılır',
   'Numara yalnız size verilir ve tek bir doğrulama içindir. Numaranın ' +
   'ne kadar geçerli olduğunu ekrandaki kalan süre gösterir.'],
  ['Kod ekranınıza düşer',
   'Gelen SMS kodu, siz sayfayı yenilemeden ekrana yansır. Kod gelmezse ' +
   'süre dolduğunda ücret bakiyenize otomatik iade edilir.'],
];

const ILKELER: Array<[string, string]> = [
  ['Kod gelmezse ücret alınmaz',
   'Numaranın süresi kod gelmeden dolarsa ücret bakiyenize geri yüklenir. ' +
   'Bunun için talep açmanız gerekmez; iade sistem tarafından yapılır.'],
  ['Teklifteki fiyattan fazlası tahsil edilmez',
   'Ödeyeceğiniz tutar numarayı almadan önce gösterilir ve satın alma bu ' +
   'tutarla sınırlandırılarak yapılır. Sonradan ek ücret çıkmaz.'],
  ['Her numara tek kullanımlıktır',
   'Bir numara tek bir doğrulama için verilir, başkasıyla paylaşılmaz ve ' +
   'kullanıldıktan sonra size tekrar verilmez.'],
  ['Ön ödemeli bakiye, sürpriz fatura yok',
   'Yalnız yüklediğiniz bakiye kadar harcama yapabilirsiniz. Bakiyeniz ' +
   'yetmiyorsa işlem başlamaz; borçlanma diye bir durum yoktur.'],
  ['Hesap hareketleri kayıtlıdır',
   'Her yükleme, her satın alma ve her iade cüzdan hareketleriniz arasında ' +
   'ayrı ayrı görünür; bakiyenizin nasıl oluştuğunu adım adım izleyebilirsiniz.'],
  ['Panel ve destek tamamen Türkçe',
   'Arayüz, bildirimler ve destek yazışmaları Türkçedir. Yabancı bir panelde ' +
   'çeviriyle iş görmek zorunda kalmazsınız.'],
];

export default function AboutPage() {
  return (
    <div className="px-4 py-10 md:px-6 md:py-14">
      <div className="mx-auto max-w-3xl">
        {/* Organization yapısal verisi: arama sonucunda marka bloğunu besler.
            SADECE doğrulanabilir alanlar var — adres/telefon/kuruluş yılı
            bilinmediği için YAZILMADI, boş string ile de doldurulmadı. */}
        <script
          type="application/ld+json"
          dangerouslySetInnerHTML={{
            __html: JSON.stringify({
              '@context': 'https://schema.org',
              '@type': 'Organization',
              name: 'Onay360',
              url: `${SITE}/`,
              logo: `${SITE}/icon/512`,
              description:
                'Yüzlerce servis için geçici numara ile SMS onay kodu sağlayan ' +
                'ön ödemeli çevrim içi hizmet.',
              inLanguage: 'tr-TR',
            }),
          }}
        />

        <h1 className="text-3xl font-bold tracking-tight md:text-4xl">Hakkımızda</h1>
        <p className="mt-4 text-base leading-relaxed text-muted md:text-lg">
          Onay360, bir servise kayıt olurken gereken SMS onay kodunu almanız için
          geçici telefon numarası sağlayan bir hizmettir. Kendi numaranızı vermek
          istemediğiniz ya da o ülkeye ait bir numaranız olmadığı durumlarda,
          doğrulamayı tek kullanımlık bir numarayla tamamlarsınız.
        </p>

        <h2 className="mt-10 text-xl font-semibold md:text-2xl">Nasıl çalışır?</h2>
        <ol className="mt-4 flex flex-col gap-3">
          {NASIL_CALISIR.map(([baslik, metin], i) => (
            <li key={baslik}>
              <Card className="flex gap-4">
                <span
                  aria-hidden
                  className="grid size-8 shrink-0 place-items-center rounded-full
                             bg-brand-500/12 text-sm font-semibold text-brand-300"
                >
                  {i + 1}
                </span>
                <div>
                  <h3 className="font-medium">{baslik}</h3>
                  <p className="mt-1 text-sm leading-relaxed text-muted">{metin}</p>
                </div>
              </Card>
            </li>
          ))}
        </ol>

        <h2 className="mt-12 text-xl font-semibold md:text-2xl">İlkelerimiz</h2>
        <p className="mt-2 text-sm leading-relaxed text-muted">
          Aşağıdakiler pazarlama cümlesi değil, sistemin çalışma biçimidir.
        </p>
        <div className="mt-4 grid gap-3 md:grid-cols-2">
          {ILKELER.map(([baslik, metin]) => (
            <Card key={baslik}>
              <h3 className="font-medium">{baslik}</h3>
              <p className="mt-1 text-sm leading-relaxed text-muted">{metin}</p>
            </Card>
          ))}
        </div>

        <h2 className="mt-12 text-xl font-semibold md:text-2xl">Ne yapmayız?</h2>
        <Card className="mt-4">
          <ul className="flex list-disc flex-col gap-2 pl-5 text-sm leading-relaxed text-muted">
            <li>
              Aldığınız numarayı başka bir kullanıcıya vermeyiz; numara o
              doğrulama için size aittir.
            </li>
            <li>
              Bakiyenizden, ekranda gördüğünüz tutarın üzerinde bir tahsilat
              yapmayız.
            </li>
            <li>
              Numarayı hangi servis için kullandığınıza dair bir tercih
              dayatmayız; ancak kanuna aykırı kullanım Kullanım Şartları
              kapsamında hesabın kapatılmasına yol açar.
            </li>
          </ul>
        </Card>

        <h2 className="mt-12 text-xl font-semibold md:text-2xl">Kurumsal bilgiler</h2>
        {bilgiHazir ? (
          <Card className="mt-4">
            <dl className="flex flex-col gap-3 text-sm">
              {SIRKET_BILGISI.map(([etiket, deger]) => (
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
              Şirket unvanı, adres ve vergi bilgileri henüz yayımlanmadı. Şirket
              kuruluşu tamamlandığında bu alan doldurulacaktır.
            </Alert>
          </Card>
        )}

        <div className="mt-12 flex flex-col gap-3 sm:flex-row">
          <Link href="/kayit" className="sm:w-auto">
            <Button fullWidth className="sm:w-auto sm:px-7">Ücretsiz hesap aç</Button>
          </Link>
          <Link href="/iletisim" className="sm:w-auto">
            <Button variant="outline" fullWidth className="sm:w-auto sm:px-7">
              Bize ulaşın
            </Button>
          </Link>
        </div>

        <p className="mt-6 text-sm text-muted">
          Merak ettikleriniz için{' '}
          <Link href="/sss" className="text-brand-300 underline underline-offset-4">
            sık sorulan sorular
          </Link>{' '}
          sayfasına da bakabilirsiniz.
        </p>
      </div>
    </div>
  );
}
