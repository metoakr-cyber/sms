import Link from 'next/link';
import type { Metadata } from 'next';
import { fetchPublic } from '@/lib/server-api';
import { Button, Card, Badge } from '@/components/ui';
import { ServiceIcon } from '@/components/service-icon';
import type { Service, Country } from '@/lib/types';

export const metadata: Metadata = {
  title: 'Sanal Numara ile Anında SMS Onay Kodu',
  description:
    'WhatsApp, Telegram, Instagram ve yüzlerce servis için sanal numara kiralayın. ' +
    'Kod saniyeler içinde ekranınıza düşer. Kod gelmezse ücret otomatik iade edilir.',
  alternates: { canonical: '/' },
};

/**
 * Ana sayfa SUNUCU bileşenidir.
 *
 * Servis ve ülke listesi HTML'e gömülü gelir — arama motoru JavaScript
 * çalıştırmadan içeriği görür. Bu liste istemcide çekilseydi sayfa botlara
 * boş görünürdü (frontend-contract.md §10.2).
 */
export default async function HomePage() {
  const [services, countries] = await Promise.all([
    fetchPublic<{ items: Service[] }>('/catalog/services'),
    fetchPublic<{ items: Country[] }>('/catalog/countries'),
  ]);
  const topServices = (services?.items ?? []).slice(0, 12);
  const countryCount = countries?.items.length ?? 0;

  return (
    <>
      {/* Yapısal veri: arama sonuçlarında zengin görünüm sağlar */}
      <script
        type="application/ld+json"
        dangerouslySetInnerHTML={{
          __html: JSON.stringify({
            '@context': 'https://schema.org',
            '@type': 'WebSite',
            name: 'SMS Onay',
            description: 'Sanal numara ile anında SMS onay kodu.',
            inLanguage: 'tr-TR',
          }),
        }}
      />

      {/* ─── Kahraman ─── */}
      <section className="px-4 pt-12 pb-14 md:px-6 md:pt-20 md:pb-20">
        <div className="mx-auto max-w-6xl">
          <div className="max-w-2xl">
            <Badge tone="brand">Kod gelmezse ücret iade</Badge>
            <h1 className="mt-4 text-3xl font-bold leading-tight tracking-tight sm:text-4xl md:text-5xl">
              Sanal numara ile{' '}
              <span className="text-brand-400">anında SMS onayı</span>
            </h1>
            <p className="mt-4 text-base leading-relaxed text-muted md:text-lg">
              WhatsApp, Telegram, Instagram ve yüzlerce servis için numara alın.
              Onay kodu ekranınıza otomatik düşer — kod gelmezse ücretiniz
              hesabınıza geri yüklenir.
            </p>
            <div className="mt-7 flex flex-col gap-3 sm:flex-row">
              <Link href="/kayit" className="sm:w-auto">
                <Button size="md" fullWidth className="sm:w-auto sm:px-7">Ücretsiz hesap aç</Button>
              </Link>
              <Link href="/fiyatlar" className="sm:w-auto">
                <Button variant="outline" fullWidth className="sm:w-auto sm:px-7">Fiyatları gör</Button>
              </Link>
            </div>
          </div>

          {/* İstatistik şeridi — mobilde 2, masaüstünde 4 sütun */}
          <dl className="mt-12 grid grid-cols-2 gap-3 md:mt-16 md:grid-cols-4 md:gap-4">
            {[
              { k: 'Servis', v: services ? `${services.items.length}+` : '—' },
              { k: 'Ülke', v: countryCount ? `${countryCount}` : '—' },
              { k: 'Ortalama kod süresi', v: '~20 sn' },
              { k: 'Kod gelmezse', v: 'İade' },
            ].map((s) => (
              <Card key={s.k} className="p-4 md:p-5">
                <dt className="text-xs font-medium uppercase tracking-wide text-muted">{s.k}</dt>
                <dd className="mt-1.5 text-xl font-bold md:text-2xl">{s.v}</dd>
              </Card>
            ))}
          </dl>
        </div>
      </section>

      {/* ─── Servisler ─── */}
      <section className="px-4 py-12 md:px-6 md:py-16" aria-labelledby="servisler">
        <div className="mx-auto max-w-6xl">
          <h2 id="servisler" className="text-2xl font-bold tracking-tight md:text-3xl">
            Desteklenen servisler
          </h2>
          <p className="mt-2 text-sm text-muted md:text-base">
            En çok kullanılan servisler. Tam liste ve güncel fiyatlar için{' '}
            <Link href="/fiyatlar" className="text-brand-400 underline underline-offset-4">
              fiyat sayfasına
            </Link>{' '}
            bakın.
          </p>

          {topServices.length > 0 ? (
            /* Mobilde 2 sütun, masaüstünde 6 — frontend-contract.md §2.5 */
            <ul className="mt-6 grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6">
              {topServices.map((s) => (
                <li key={s.code}>
                  <Card className="flex min-h-20 flex-col items-center justify-center gap-2 p-3 text-center">
                    <ServiceIcon name={s.name} iconUrl={s.iconUrl} size={36} />
                    <span className="text-sm font-medium break-anywhere">{s.name}</span>
                  </Card>
                </li>
              ))}
            </ul>
          ) : (
            <Card className="mt-6">
              <p className="text-sm text-muted">
                Servis listesi şu anda yüklenemedi. Lütfen birazdan tekrar deneyin.
              </p>
            </Card>
          )}
        </div>
      </section>

      {/* ─── Nasıl çalışır ─── */}
      <section className="px-4 py-12 md:px-6 md:py-16" aria-labelledby="nasil">
        <div className="mx-auto max-w-6xl">
          <h2 id="nasil" className="text-2xl font-bold tracking-tight md:text-3xl">Nasıl çalışır?</h2>
          <ol className="mt-6 grid gap-3 md:grid-cols-4 md:gap-4">
            {[
              ['Servisi seçin', 'WhatsApp, Telegram, Instagram… Hangi servis için numara gerekiyorsa onu seçin.'],
              ['Ülkeyi seçin', 'Stokta olan ülkeler ve anlık fiyatları listelenir.'],
              ['Numarayı alın', 'Numara anında hesabınıza tanımlanır ve ücret bakiyenizden düşer.'],
              ['Kodu bekleyin', 'Kod ekranınıza otomatik düşer. Gelmezse ücret iade edilir.'],
            ].map(([t, d], i) => (
              <li key={t}>
                <Card className="h-full">
                  <span className="grid size-8 place-items-center rounded-lg bg-brand-500/15 text-sm font-bold text-brand-300">
                    {i + 1}
                  </span>
                  <h3 className="mt-3 font-semibold">{t}</h3>
                  <p className="mt-1.5 text-sm leading-relaxed text-muted">{d}</p>
                </Card>
              </li>
            ))}
          </ol>
        </div>
      </section>

      {/* ─── Kapanış ─── */}
      <section className="px-4 py-12 md:px-6 md:py-16">
        <div className="mx-auto max-w-6xl">
          <Card className="flex flex-col items-start gap-5 p-6 md:flex-row md:items-center md:justify-between md:p-8">
            <div>
              <h2 className="text-xl font-bold md:text-2xl">Hesap açmak ücretsiz</h2>
              <p className="mt-1.5 text-sm text-muted">
                Yalnız kullandığınız numara kadar ödersiniz. Abonelik yok.
              </p>
            </div>
            <Link href="/kayit" className="w-full md:w-auto">
              <Button fullWidth className="md:w-auto md:px-7">Hemen başla</Button>
            </Link>
          </Card>
        </div>
      </section>
    </>
  );
}
