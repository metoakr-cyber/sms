import Link from 'next/link';
import type { Metadata } from 'next';
import { Akordeon, Belir } from '@/components/animasyon';
import { Bolum, BolumBasligi, IkonKaro, RenkliKart } from '@/components/pazarlama/parcalar';
import { KapanisCTA } from '@/components/pazarlama/bolumler';
import { Cuzdan, Iade, SagOk, SmsBalon, Takvim } from '@/components/ikonlar';

export const metadata: Metadata = {
  title: 'Sık Sorulan Sorular',
  description:
    'Sanal numara nedir, kod gelmezse ne olur, iade nasıl yapılır? ' +
    'SMS onay hizmeti hakkında en çok sorulan sorular ve yanıtları.',
  alternates: { canonical: '/sss' },
};

/**
 * Sorular KONULARA ayrıldı.
 *
 * Tek bir uzun liste, "iade" sorusunu arayan kullanıcıyı on soru okumaya
 * zorluyordu. JSON-LD `FAQPage` şeması gruplardan BAĞIMSIZ olarak tüm
 * soruları düz bir dizi hâlinde taşır — Google gruplamayı bilmez, soruları
 * bilir; bu yüzden ikisi ayrı üretilir ama AYNI kaynaktan beslenir.
 */
const GRUPLAR: Array<{
  baslik: string;
  ikon: React.ReactNode;
  renk: 'mavi' | 'yesil' | 'mor' | 'turuncu';
  sorular: Array<[string, string]>;
}> = [
  {
    baslik: 'Temeller',
    ikon: <SmsBalon />,
    renk: 'mavi',
    sorular: [
      ['Sanal numara nedir?',
       'Yalnız SMS almak için kullanılan geçici bir telefon numarasıdır. Bir servise ' +
       'kayıt olurken doğrulama kodu bu numaraya gelir ve kod ekranınızda görünür.'],
      ['Kod ne kadar sürede geliyor?',
       'Çoğu serviste 10–30 saniye içinde gelir. Numara alındıktan sonra bekleme ' +
       'ekranı açık kalır ve kod düştüğü anda ekrana yansır.'],
      ['Aynı numarayı tekrar kullanabilir miyim?',
       'Hayır. Her numara tek bir doğrulama için verilir. Aynı servise tekrar kayıt ' +
       'olmak için yeni bir numara almanız gerekir.'],
    ],
  },
  {
    baslik: 'Ücret ve iade',
    ikon: <Iade />,
    renk: 'yesil',
    sorular: [
      ['Kod gelmezse ne oluyor?',
       'Numaranın süresi kod gelmeden dolarsa ücret otomatik olarak bakiyenize iade ' +
       'edilir. İade için talep açmanıza gerek yoktur; sistem bunu kendisi yapar.'],
      ['Numarayı iptal edebilir miyim?',
       'Evet. Numara alındıktan kısa bir süre sonra iptal düğmesi aktifleşir. Kod ' +
       'gelmemişse iptal ettiğinizde ücret bakiyenize döner.'],
      ['Bakiyemi nasıl yüklerim?',
       'Banka havalesi/EFT veya USDT ile yükleme yapabilirsiniz. Ödemeniz onaylandıktan ' +
       'sonra bakiyeniz hesabınıza tanımlanır.'],
    ],
  },
  {
    baslik: 'Kiralama',
    ikon: <Takvim />,
    renk: 'mor',
    sorular: [
      ['Numara kiralama nedir?',
       'Tek kullanımlık numara yalnız bir doğrulama kodu için verilir. Kiralamada ise ' +
       'numara seçtiğiniz süre boyunca (1 gün ile 6 ay arası) size aittir ve o süre ' +
       'içinde gelen tüm SMS mesajlarını görürsünüz.'],
      ['Aylık numara kiralayabilir miyim?',
       'Evet. 30 günlük kiralama en çok tercih edilen modeldir; 60, 90 ve 180 günlük ' +
       'seçenekler de mevcuttur.'],
    ],
  },
];

/** JSON-LD için düz liste — gruplama arayüze aittir, şemaya değil. */
const TUM_SORULAR = GRUPLAR.flatMap((g) => g.sorular);

/** Hızlı erişim kartları — kullanıcının en çok aradığı üç konu. */
const KISAYOLLAR: Array<[string, string, 'yesil' | 'mor' | 'turuncu', React.ReactNode]> = [
  ['Kod gelmezse ücret iade edilir', 'Talep açmanıza gerek yok; sistem kendisi yapar.',
   'yesil', <Iade key="1" />],
  ['1 gün – 6 ay kiralama', 'Numara süre boyunca sizde kalır, tüm mesajları görürsünüz.',
   'mor', <Takvim key="2" />],
  ['Abonelik yok', 'Ön ödemeli TL bakiye; yalnız aldığınız numara kadar ödersiniz.',
   'turuncu', <Cuzdan key="3" />],
];

export default function FaqPage() {
  return (
    <>
      {/* FAQPage yapısal verisi — Google'da açılır soru-cevap görünümü sağlar */}
      <script
        type="application/ld+json"
        dangerouslySetInnerHTML={{
          __html: JSON.stringify({
            '@context': 'https://schema.org',
            '@type': 'FAQPage',
            mainEntity: TUM_SORULAR.map(([q, a]) => ({
              '@type': 'Question',
              name: q,
              acceptedAnswer: { '@type': 'Answer', text: a },
            })),
          }),
        }}
      />

      <Bolum className="kahraman-isik pb-8 pt-12 md:pb-10 md:pt-16">
        <BolumBasligi
          seviye={1}
          hap="Yardım"
          baslik="Sık sorulan"
          vurgu="sorular"
          aciklama="Numara almadan önce en çok merak edilenler. Aradığınızı bulamazsanız
                    iletişim sayfasından yazabilirsiniz."
        />

        <ul className="mt-10 grid gap-4 md:grid-cols-3">
          {KISAYOLLAR.map(([baslik, metin, renk, ikon], i) => (
            <Belir as="li" key={baslik} gecikme={i * 90}>
              <RenkliKart>
                <IkonKaro renk={renk} dolu boy="md">{ikon}</IkonKaro>
                <h2 className="mt-4 font-semibold leading-snug">{baslik}</h2>
                <p className="mt-1.5 text-sm leading-relaxed text-muted">{metin}</p>
              </RenkliKart>
            </Belir>
          ))}
        </ul>
      </Bolum>

      <Bolum className="py-6 md:py-10" icClassName="max-w-3xl">
        {GRUPLAR.map((g, i) => (
          <section key={g.baslik} className={i > 0 ? 'mt-12' : ''}>
            <Belir className="flex items-center gap-3">
              <IkonKaro renk={g.renk} boy="md">{g.ikon}</IkonKaro>
              <h2 className="text-xl font-bold tracking-tight md:text-2xl">{g.baslik}</h2>
            </Belir>
            <Belir className="mt-5">
              {/* Akordeon <details>/<summary> ile kurulu: JavaScript kapalıyken de
                  açılır, klavye desteği tarayıcıdan gelir, içerik HTML'de durur. */}
              <Akordeon sorular={g.sorular} />
            </Belir>
          </section>
        ))}

        <Belir className="mt-10 flex flex-wrap items-center gap-x-6 gap-y-2">
          <Link
            href="/kiralama"
            className="inline-flex min-h-11 items-center gap-2 text-sm font-semibold
                       text-[var(--vurgu)] hover:text-[var(--vurgu-guclu)]"
          >
            Kiralama ayrıntıları <SagOk className="size-4" />
          </Link>
          <Link
            href="/blog"
            className="inline-flex min-h-11 items-center gap-2 text-sm font-semibold
                       text-[var(--vurgu)] hover:text-[var(--vurgu-guclu)]"
          >
            Blog yazıları <SagOk className="size-4" />
          </Link>
        </Belir>
      </Bolum>

      <Bolum className="pt-4 md:pt-6">
        <KapanisCTA
          baslik="Sorunuz burada yoksa"
          metin="İletişim sayfasından yazın; destek ekibi hesabınız üzerinden yanıt versin."
          birincilMetin="Hesap aç"
          ikincilMetin="İletişim"
          ikincilBag="/iletisim"
        />
      </Bolum>
    </>
  );
}
