import type { Metadata } from 'next';
import { Card } from '@/components/ui';

export const metadata: Metadata = {
  title: 'Sık Sorulan Sorular',
  description:
    'Sanal numara nedir, kod gelmezse ne olur, iade nasıl yapılır? ' +
    'SMS onay hizmeti hakkında en çok sorulan sorular ve yanıtları.',
  alternates: { canonical: '/sss' },
};

const QA: Array<[string, string]> = [
  ['Sanal numara nedir?',
   'Yalnız SMS almak için kullanılan geçici bir telefon numarasıdır. Bir servise ' +
   'kayıt olurken doğrulama kodu bu numaraya gelir ve kod ekranınızda görünür.'],
  ['Kod gelmezse ne oluyor?',
   'Numaranın süresi kod gelmeden dolarsa ücret otomatik olarak bakiyenize iade ' +
   'edilir. İade için talep açmanıza gerek yoktur; sistem bunu kendisi yapar.'],
  ['Kod ne kadar sürede geliyor?',
   'Çoğu serviste 10–30 saniye içinde gelir. Numara alındıktan sonra bekleme ' +
   'ekranı açık kalır ve kod düştüğü anda ekrana yansır.'],
  ['Aynı numarayı tekrar kullanabilir miyim?',
   'Hayır. Her numara tek bir doğrulama için verilir. Aynı servise tekrar kayıt ' +
   'olmak için yeni bir numara almanız gerekir.'],
  ['Bakiyemi nasıl yüklerim?',
   'Banka havalesi/EFT veya USDT ile yükleme yapabilirsiniz. Ödemeniz onaylandıktan ' +
   'sonra bakiyeniz hesabınıza tanımlanır.'],
  ['Numarayı iptal edebilir miyim?',
   'Evet. Numara alındıktan kısa bir süre sonra iptal düğmesi aktifleşir. Kod ' +
   'gelmemişse iptal ettiğinizde ücret bakiyenize döner.'],
];

export default function FaqPage() {
  return (
    <div className="px-4 py-10 md:px-6 md:py-14">
      <div className="mx-auto max-w-3xl">
        <h1 className="text-3xl font-bold tracking-tight md:text-4xl">Sık sorulan sorular</h1>

        {/* FAQPage yapısal verisi — Google'da açılır soru-cevap görünümü sağlar */}
        <script
          type="application/ld+json"
          dangerouslySetInnerHTML={{
            __html: JSON.stringify({
              '@context': 'https://schema.org',
              '@type': 'FAQPage',
              mainEntity: QA.map(([q, a]) => ({
                '@type': 'Question',
                name: q,
                acceptedAnswer: { '@type': 'Answer', text: a },
              })),
            }),
          }}
        />

        <div className="mt-8 flex flex-col gap-3">
          {QA.map(([q, a]) => (
            /* <details> yerel açılır öğedir: JavaScript kapalıyken de çalışır
               ve arama motoru içeriği görür. */
            <Card key={q} className="p-0">
              <details className="group">
                <summary className="flex min-h-14 cursor-pointer list-none items-center
                                    justify-between gap-3 px-4 py-3 font-medium md:px-6">
                  {q}
                  <svg viewBox="0 0 24 24" className="size-5 shrink-0 text-muted transition-transform
                                                      group-open:rotate-180"
                       fill="none" stroke="currentColor" strokeWidth="2" aria-hidden>
                    <path d="m6 9 6 6 6-6" strokeLinecap="round" strokeLinejoin="round" />
                  </svg>
                </summary>
                <p className="border-t border-[var(--border)] px-4 py-4 text-sm leading-relaxed
                              text-muted md:px-6">{a}</p>
              </details>
            </Card>
          ))}
        </div>
      </div>
    </div>
  );
}
