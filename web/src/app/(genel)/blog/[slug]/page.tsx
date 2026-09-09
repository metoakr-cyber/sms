import Link from 'next/link';
import { notFound } from 'next/navigation';
import type { Metadata } from 'next';
import { Bolum, Hap } from '@/components/pazarlama/parcalar';
import { KapanisCTA } from '@/components/pazarlama/bolumler';
import { SagOk, Saat } from '@/components/ikonlar';
import { markdownaHtml, tarihBicimle, yaziGetir, yazilariGetir } from '../icerik';

const SITE = process.env.NEXT_PUBLIC_SITE_URL ?? 'http://localhost:3000';

/** Tüm yazılar derleme anında üretilir; listede olmayan slug 404 döner. */
export function generateStaticParams() {
  return yazilariGetir().map((y) => ({ slug: y.slug }));
}
export const dynamicParams = false;

export async function generateMetadata(
  { params }: { params: Promise<{ slug: string }> },
): Promise<Metadata> {
  const { slug } = await params;
  const y = yaziGetir(slug);
  if (!y) return { title: 'Yazı bulunamadı' };
  return {
    title: y.baslik,
    description: y.ozet,
    alternates: { canonical: `/blog/${y.slug}` },
    openGraph: {
      type: 'article',
      locale: 'tr_TR',
      url: `${SITE}/blog/${y.slug}`,
      title: `${y.baslik} | Onay360`,
      description: y.ozet,
      publishedTime: y.tarih || undefined,
    },
  };
}

export default async function BlogYaziPage(
  { params }: { params: Promise<{ slug: string }> },
) {
  const { slug } = await params;
  const yazi = yaziGetir(slug);
  if (!yazi) notFound();

  const govde = markdownaHtml(yazi.govde);
  const digerleri = yazilariGetir().filter((y) => y.slug !== yazi.slug).slice(0, 2);

  return (
    <>
      <script
        type="application/ld+json"
        dangerouslySetInnerHTML={{
          __html: JSON.stringify([
            {
              '@context': 'https://schema.org',
              '@type': 'BlogPosting',
              headline: yazi.baslik,
              description: yazi.ozet,
              inLanguage: 'tr-TR',
              mainEntityOfPage: { '@type': 'WebPage', '@id': `${SITE}/blog/${yazi.slug}` },
              datePublished: yazi.tarih || undefined,
              dateModified: yazi.tarih || undefined,
              // 🔴 Yazar UYDURULMAZ. İçeriği kurum yazdı, kurum sorumlu.
              author: { '@type': 'Organization', name: 'Onay360' },
              publisher: { '@type': 'Organization', name: 'Onay360' },
            },
            {
              '@context': 'https://schema.org',
              '@type': 'BreadcrumbList',
              itemListElement: [
                { '@type': 'ListItem', position: 1, name: 'Ana Sayfa', item: `${SITE}/` },
                { '@type': 'ListItem', position: 2, name: 'Blog', item: `${SITE}/blog` },
                { '@type': 'ListItem', position: 3, name: yazi.baslik,
                  item: `${SITE}/blog/${yazi.slug}` },
              ],
            },
          ]),
        }}
      />

      <article>
        <Bolum className="kahraman-isik pb-6 pt-10 md:pb-8 md:pt-14" icClassName="max-w-3xl">
          {/* Kırıntı yolu — görünür metin, yapısal veriyle aynı. */}
          <nav aria-label="Kırıntı yolu" className="text-sm text-muted">
            <Link href="/blog"
                  className="inline-flex min-h-11 items-center gap-2 font-medium
                             text-[var(--vurgu)] hover:text-[var(--vurgu-guclu)]">
              <SagOk className="size-4 rotate-180" />
              Tüm yazılar
            </Link>
          </nav>

          <div className="mt-2 flex flex-wrap items-center gap-3">
            <Hap renk={yazi.renk}>{yazi.etiket}</Hap>
            {yazi.okuma > 0 && (
              <span className="inline-flex items-center gap-1.5 text-xs text-muted">
                <Saat className="size-3.5" />{yazi.okuma} dk okuma
              </span>
            )}
            {yazi.tarih && (
              <time dateTime={yazi.tarih} className="text-xs text-muted">
                {tarihBicimle(yazi.tarih)}
              </time>
            )}
          </div>

          <h1 className="mt-4 text-balance text-3xl font-bold leading-tight tracking-tight
                         md:text-4xl">
            {yazi.baslik}
          </h1>
          <p className="mt-4 text-pretty text-base leading-relaxed text-muted md:text-lg">
            {yazi.ozet}
          </p>
          <p className="mt-5 border-t border-[var(--border)] pt-4 text-xs text-muted">
            Onay360 tarafından yazıldı.
          </p>
        </Bolum>

        <Bolum className="pb-4 pt-2 md:pb-6 md:pt-3" icClassName="max-w-3xl">
          {/*
            Gövde derleme anında bizim yazdığımız Markdown'dan üretilir ve
            ayrıştırıcı HTML'i ÖNCE kaçışlar (`icerik.ts`). Dışarıdan gelen
            hiçbir dize buraya girmez.
          */}
          <div dangerouslySetInnerHTML={{ __html: govde }} />
        </Bolum>
      </article>

      {digerleri.length > 0 && (
        <Bolum className="py-8 md:py-10" icClassName="max-w-3xl">
          <h2 className="text-lg font-bold tracking-tight">Diğer yazılar</h2>
          <ul className="mt-4 grid gap-3 sm:grid-cols-2">
            {digerleri.map((y) => (
              <li key={y.slug}>
                <Link
                  href={`/blog/${y.slug}`}
                  className="surface kart-hover flex h-full flex-col rounded-2xl border p-4
                             golge-1"
                >
                  <span className="text-xs font-semibold uppercase tracking-wide
                                   text-[var(--vurgu)]">{y.etiket}</span>
                  <span className="mt-2 font-semibold leading-snug">{y.baslik}</span>
                  <span className="mt-1.5 text-sm leading-relaxed text-muted">{y.ozet}</span>
                </Link>
              </li>
            ))}
          </ul>
        </Bolum>
      )}

      <Bolum className="pt-4 md:pt-6" icClassName="max-w-3xl">
        <KapanisCTA
          baslik="Numara alarak deneyin"
          metin="Hesap açmak ücretsiz, abonelik yok. Kod gelmezse ücret iade edilir."
          ikincilMetin="Fiyatlar"
          ikincilBag="/fiyatlar"
        />
      </Bolum>
    </>
  );
}
