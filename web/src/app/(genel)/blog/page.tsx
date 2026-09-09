import Link from 'next/link';
import type { Metadata } from 'next';
import { Belir } from '@/components/animasyon';
import { Bolum, BolumBasligi, Hap } from '@/components/pazarlama/parcalar';
import { KapanisCTA } from '@/components/pazarlama/bolumler';
import { SagOk, Saat } from '@/components/ikonlar';
import { yazilariGetir, tarihBicimle } from './icerik';

const SITE = process.env.NEXT_PUBLIC_SITE_URL ?? 'http://localhost:3000';

export const metadata: Metadata = {
  title: 'Blog — Sanal Numara ve SMS Onayı Rehberi',
  description:
    'Sanal numara ve SMS doğrulama nasıl çalışır, kod neden gelmez, tek kullanımlık ' +
    'numara ile kiralık numara arasındaki fark nedir? Teknik ve sade anlatım.',
  alternates: { canonical: '/blog' },
  openGraph: {
    type: 'website', locale: 'tr_TR', url: `${SITE}/blog`,
    title: 'Blog — Sanal Numara ve SMS Onayı Rehberi | Onay360',
    description: 'Sanal numara ve SMS doğrulama üzerine teknik ve sade yazılar.',
  },
};

/** Yazılar dosyadan okunur; çalışma anında dosya sistemine gidilmesin diye statik. */
export const dynamic = 'force-static';

export default function BlogPage() {
  const yazilar = yazilariGetir();

  return (
    <>
      {yazilar.length > 0 && (
        <script
          type="application/ld+json"
          dangerouslySetInnerHTML={{
            __html: JSON.stringify({
              '@context': 'https://schema.org',
              '@type': 'Blog',
              name: 'Onay360 Blog',
              url: `${SITE}/blog`,
              inLanguage: 'tr-TR',
              publisher: { '@type': 'Organization', name: 'Onay360' },
              blogPost: yazilar.map((y) => ({
                '@type': 'BlogPosting',
                headline: y.baslik,
                description: y.ozet,
                url: `${SITE}/blog/${y.slug}`,
                datePublished: y.tarih || undefined,
                author: { '@type': 'Organization', name: 'Onay360' },
              })),
            }),
          }}
        />
      )}

      <Bolum className="kahraman-isik pb-8 pt-12 md:pb-10 md:pt-16">
        <BolumBasligi
          seviye={1}
          hap="Onay360 Blog"
          baslik="Sanal numara ve SMS onayı"
          vurgu="rehberi"
          aciklama={
            <>
              Numara nereden geliyor, kod neden bazen gelmiyor, hangi durumda hangi
              modeli seçmeli? Pazarlama cümlesi değil, işin nasıl yürüdüğü.
            </>
          }
        />
      </Bolum>

      <Bolum className="py-6 md:py-8">
        {yazilar.length === 0 ? (
          <div className="surface rounded-2xl border p-8 text-center golge-1">
            <p className="font-medium">Henüz yazı yok.</p>
            <p className="mt-1.5 text-sm text-muted">
              Yeni yazılar eklendiğinde burada listelenecek.
            </p>
          </div>
        ) : (
          <ul className="grid gap-4 md:grid-cols-2 md:gap-5 lg:grid-cols-3">
            {yazilar.map((y, i) => (
              <Belir as="li" key={y.slug} gecikme={i * 90}>
                {/* Bağlantı KARTIN KENDİSİ. Yalnız başlığı bağlayıp kalanı
                    `after:inset-0` ile kaplamak görsel olarak aynı sonucu verir
                    ama denetimde bağlantı 246×21 px ölçülür — 44 px dokunma
                    hedefinin altında (frontend-contract §2.3). Ölçülen şey
                    öğenin kendi kutusudur, kapladığı alan değil. */}
                <Link
                  href={`/blog/${y.slug}`}
                  className="surface kart-hover flex h-full flex-col rounded-2xl border p-5
                             golge-2 md:p-6"
                >
                  <article className="flex h-full flex-col">
                    <span className="flex items-center gap-3">
                      <Hap renk={y.renk}>{y.etiket}</Hap>
                      {y.okuma > 0 && (
                        <span className="inline-flex items-center gap-1.5 text-xs text-muted">
                          <Saat className="size-3.5" />
                          {y.okuma} dk okuma
                        </span>
                      )}
                    </span>

                    <h2 className="mt-4 text-lg font-bold leading-snug tracking-tight">
                      {y.baslik}
                    </h2>
                    <p className="mt-2 flex-1 text-sm leading-relaxed text-muted">{y.ozet}</p>

                    <span className="mt-5 flex items-center justify-between gap-3">
                      {y.tarih ? (
                        <time dateTime={y.tarih} className="text-xs text-muted">
                          {tarihBicimle(y.tarih)}
                        </time>
                      ) : <span />}
                      <span className="inline-flex items-center gap-1.5 text-sm font-semibold
                                       text-[var(--vurgu)]">
                        Oku <SagOk className="size-4" />
                      </span>
                    </span>
                  </article>
                </Link>
              </Belir>
            ))}
          </ul>
        )}
      </Bolum>

      <Bolum className="pt-6 md:pt-8">
        <KapanisCTA
          baslik="Numara almaya hazır mısınız?"
          metin="Hesap açmak ücretsiz. Yalnız aldığınız numara kadar ödersiniz."
          ikincilMetin="Sık sorulanlar"
          ikincilBag="/sss"
        />
      </Bolum>
    </>
  );
}
