import Link from 'next/link';
import type { Metadata } from 'next';
import { fetchPublic } from '@/lib/server-api';
import { Card, Button, Badge } from '@/components/ui';
import type { RentalService } from '@/lib/types';

const SITE = process.env.NEXT_PUBLIC_SITE_URL ?? 'http://localhost:3000';

export const metadata: Metadata = {
  title: 'Numara Kiralama — Aylık Sanal Numara',
  description:
    'WhatsApp, Telegram, Instagram ve yüzlerce servis için 1 gün ile 6 ay arası ' +
    'süreyle numara kiralayın. Aylık kiralamada numara size ait kalır, gelen tüm ' +
    'SMS mesajlarını görürsünüz.',
  alternates: { canonical: '/kiralama' },
  openGraph: {
    type: 'website',
    locale: 'tr_TR',
    url: `${SITE}/kiralama`,
    title: 'Numara Kiralama — Aylık Sanal Numara | Onay360',
    description: '1 gün – 6 ay arası numara kiralama. Aylık modelde numara sizde kalır.',
  },
};

/**
 * Kiralama sayfası — SUNUCU bileşeni.
 *
 * Servis listesi HTML'e gömülü gelir: arama motoru JavaScript çalıştırmadan
 * "hangi servisler için numara kiralanabiliyor" sorusunun cevabını görür.
 * İstemcide çekilseydi sayfa botlara boş görünürdü (frontend-contract.md §10.2).
 */
export default async function RentalPage() {
  const data = await fetchPublic<{ items: RentalService[] }>(
    '/catalog/rental/services', { revalidate: 300 });
  const items = [...(data?.items ?? [])].sort((a, b) => b.countryCount - a.countryCount);

  // Süre kademeleri sağlayıcının canlı listesinden geliyor; burada yalnız
  // ANLATIM için sabit — fiyat ve stok her zaman API'den okunur.
  const tiers = [
    { label: '1 gün', note: 'Kısa süreli doğrulama' },
    { label: '3 gün', note: 'Hafta sonu kullanımı' },
    { label: '7 gün', note: 'Bir haftalık test' },
    { label: '14 gün', note: 'İki haftalık' },
    { label: '30 gün', note: 'Aylık — en çok tercih edilen', highlight: true },
    { label: '60 gün', note: 'İki aylık' },
    { label: '90 gün', note: 'Üç aylık' },
    { label: '180 gün', note: 'Altı aylık' },
  ];

  return (
    <div className="px-4 py-10 md:px-6 md:py-14">
      <div className="mx-auto max-w-6xl">
        {/* Yapısal veri: hizmet + SSS. Arama sonuçlarında zengin görünüm sağlar. */}
        <script
          type="application/ld+json"
          dangerouslySetInnerHTML={{
            __html: JSON.stringify([
              {
                '@context': 'https://schema.org',
                '@type': 'Service',
                name: 'Sanal Numara Kiralama',
                serviceType: 'Numara kiralama',
                provider: { '@type': 'Organization', name: 'Onay360' },
                areaServed: 'TR',
                description:
                  '1 gün ile 6 ay arası süreyle sanal numara kiralama. ' +
                  'Kiralama süresince gelen tüm SMS mesajları görüntülenir.',
              },
              {
                '@context': 'https://schema.org',
                '@type': 'FAQPage',
                mainEntity: [
                  {
                    '@type': 'Question',
                    name: 'Numara kiralama ile tek kullanımlık numara arasındaki fark nedir?',
                    acceptedAnswer: {
                      '@type': 'Answer',
                      text:
                        'Tek kullanımlık numara yalnız bir doğrulama kodu için verilir ve ' +
                        'kod geldikten sonra kapanır. Kiralık numara, seçtiğiniz süre boyunca ' +
                        'size aittir ve o süre içinde gelen tüm SMS mesajlarını görürsünüz.',
                    },
                  },
                  {
                    '@type': 'Question',
                    name: 'Aylık numara kiralama nasıl çalışır?',
                    acceptedAnswer: {
                      '@type': 'Answer',
                      text:
                        '30 günlük kiralamada numara bir ay boyunca hesabınıza tanımlı kalır. ' +
                        'Aynı numaraya birden fazla servis için kod alabilir, gelen mesajları ' +
                        'panelinizden takip edebilirsiniz.',
                    },
                  },
                  {
                    '@type': 'Question',
                    name: 'Kiralık numarada kaç mesaj alabilirim?',
                    acceptedAnswer: {
                      '@type': 'Answer',
                      text:
                        'Kiralama süresi boyunca gelen tüm mesajlar panelinizde listelenir; ' +
                        'sayı sınırı yoktur.',
                    },
                  },
                ],
              },
            ]),
          }}
        />

        {/* ─── Başlık ─── */}
        <Badge tone="brand">1 gün – 6 ay</Badge>
        <h1 className="mt-4 text-3xl font-bold leading-tight tracking-tight md:text-4xl">
          Numara kiralama
        </h1>
        <p className="mt-4 max-w-2xl text-base leading-relaxed text-muted md:text-lg">
          Tek bir doğrulama kodu yerine <strong className="text-[var(--text)]">numaranın
          kendisini</strong> kiralayın. Seçtiğiniz süre boyunca numara sizde kalır ve o
          süre içinde gelen <strong className="text-[var(--text)]">tüm SMS mesajlarını</strong> görürsünüz.
        </p>

        {/* ─── Fark ─── */}
        <div className="mt-8 grid gap-3 md:grid-cols-2 md:gap-4">
          <Card>
            <h2 className="font-semibold">Tek kullanımlık</h2>
            <ul className="mt-3 flex flex-col gap-2 text-sm text-muted">
              <li>· Bir doğrulama kodu için</li>
              <li>· Numara ~20 dakika aktif</li>
              <li>· Kod gelmezse ücret iade</li>
              <li>· En düşük maliyet</li>
            </ul>
          </Card>
          <Card className="border-brand-500/40 bg-brand-500/[0.06]">
            <h2 className="font-semibold">Kiralama</h2>
            <ul className="mt-3 flex flex-col gap-2 text-sm text-muted">
              <li>· Numara süre boyunca size ait</li>
              <li>· <strong className="text-[var(--text)]">Sınırsız</strong> gelen mesaj</li>
              <li>· Birden fazla servis için kullanılabilir</li>
              <li>· 1 gün ile 6 ay arası</li>
            </ul>
          </Card>
        </div>

        {/* ─── Süre kademeleri ─── */}
        <h2 className="mt-12 text-2xl font-bold tracking-tight md:text-3xl">
          Kiralama süreleri
        </h2>
        <p className="mt-2 text-sm text-muted">
          Fiyat servise, ülkeye ve süreye göre değişir. Kesin fiyat, döviz kuru anlık
          değiştiği için satın alma ekranında size özel bir teklif olarak gösterilir.
        </p>
        <ul className="mt-5 grid grid-cols-2 gap-3 sm:grid-cols-4">
          {tiers.map((t) => (
            <li key={t.label}>
              <Card className={t.highlight ? 'border-brand-500/50 bg-brand-500/[0.08]' : ''}>
                <p className="text-lg font-bold">{t.label}</p>
                <p className="mt-1 text-xs leading-relaxed text-muted">{t.note}</p>
              </Card>
            </li>
          ))}
        </ul>

        {/* ─── Servisler ─── */}
        <h2 className="mt-12 text-2xl font-bold tracking-tight md:text-3xl">
          Kiralanabilir servisler
        </h2>
        {items.length === 0 ? (
          <Card className="mt-5">
            <p className="text-sm text-muted">
              Kiralık listesi şu anda yüklenemedi. Lütfen birazdan tekrar deneyin.
            </p>
          </Card>
        ) : (
          <>
            <p className="mt-2 text-sm text-muted">
              <strong className="text-[var(--text)]">{items.length}</strong> serviste
              kiralama mevcut.
            </p>
            {/* Sade işaretleme: 495 öğe ağır bileşenle basılırsa sayfa şişer. */}
            <ul className="mt-5 grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-4">
              {items.map((s) => (
                <li key={s.code} className="surface rounded-xl border px-3 py-2.5">
                  <span className="block truncate text-sm font-medium">{s.name}</span>
                  <span className="text-xs text-muted">
                    {s.countryCount} ülke · {s.durationCount} süre
                  </span>
                </li>
              ))}
            </ul>
          </>
        )}

        {/* ─── SSS (görünür metin — yapısal veriyle aynı içerik) ─── */}
        <h2 className="mt-12 text-2xl font-bold tracking-tight md:text-3xl">
          Kiralama hakkında
        </h2>
        <div className="mt-5 flex flex-col gap-3">
          {[
            ['Kiralama ile tek kullanımlık numara arasındaki fark nedir?',
             'Tek kullanımlık numara yalnız bir doğrulama kodu için verilir ve kod geldikten ' +
             'sonra kapanır. Kiralık numara, seçtiğiniz süre boyunca size aittir ve o süre ' +
             'içinde gelen tüm SMS mesajlarını görürsünüz.'],
            ['Aylık numara kiralama nasıl çalışır?',
             '30 günlük kiralamada numara bir ay boyunca hesabınıza tanımlı kalır. Aynı ' +
             'numaraya birden fazla servis için kod alabilir, gelen mesajları panelinizden ' +
             'takip edebilirsiniz.'],
            ['Kiralık numarada kaç mesaj alabilirim?',
             'Kiralama süresi boyunca gelen tüm mesajlar panelinizde listelenir; sayı sınırı yoktur.'],
            ['Süre bitince ne oluyor?',
             'Kiralama süresi dolduğunda numara kapanır ve yeni mesaj almaz. Süre dolmadan ' +
             'önce uzatma yapılabilir.'],
          ].map(([q, a]) => (
            <Card key={q} className="p-0">
              <details className="group">
                <summary className="flex min-h-14 cursor-pointer list-none items-center
                                    justify-between gap-3 px-4 py-3 font-medium md:px-6">
                  {q}
                  <svg viewBox="0 0 24 24" className="size-5 shrink-0 text-muted
                                                      transition-transform group-open:rotate-180"
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

        <Card className="mt-10 flex flex-col items-start gap-4 md:flex-row md:items-center md:justify-between">
          <p className="text-sm text-muted">
            Kiralık numara almak için hesabınıza giriş yapın.
          </p>
          <Link href="/kayit" className="w-full md:w-auto">
            <Button fullWidth className="md:w-auto md:px-6">Hesap aç</Button>
          </Link>
        </Card>
      </div>
    </div>
  );
}
