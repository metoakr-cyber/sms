import Link from 'next/link';
import type { Metadata } from 'next';
import { fetchPublic } from '@/lib/server-api';
import { Card, Badge, Button } from '@/components/ui';
import { ServiceIcon } from '@/components/service-icon';
import type { CatalogItem } from '@/lib/types';

export const metadata: Metadata = {
  title: 'Fiyatlar ve Stok Durumu',
  description:
    'Servis ve ülkeye göre sanal numara stok durumu. WhatsApp, Telegram, ' +
    'Instagram ve daha fazlası için güncel uygunluk listesi.',
  alternates: { canonical: '/fiyatlar' },
};

export default async function PricingPage() {
  const data = await fetchPublic<{ items: CatalogItem[] }>('/catalog/availability', { revalidate: 120 });
  const items = data?.items ?? [];

  // Servise göre grupla — kullanıcı "WhatsApp hangi ülkelerde var" diye bakar,
  // "Türkiye'de hangi servisler var" diye değil.
  const groups = new Map<string, { name: string; iconUrl?: string; rows: CatalogItem[] }>();
  for (const it of items) {
    const g = groups.get(it.serviceCode)
      ?? { name: it.serviceName, iconUrl: it.iconUrl, rows: [] };
    g.rows.push(it);
    groups.set(it.serviceCode, g);
  }

  return (
    <div className="px-4 py-10 md:px-6 md:py-14">
      <div className="mx-auto max-w-6xl">
        <h1 className="text-3xl font-bold tracking-tight md:text-4xl">Fiyatlar ve stok</h1>
        <p className="mt-3 max-w-2xl text-sm leading-relaxed text-muted md:text-base">
          Aşağıda stokta olan servis ve ülke kombinasyonları listelenmiştir.
          Numaranın kesin fiyatı, döviz kuru anlık değiştiği için satın alma
          ekranında <strong className="text-[var(--text)]">size özel bir teklif</strong> olarak
          gösterilir ve teklif süresi boyunca değişmez.
        </p>

        {items.length === 0 ? (
          <Card className="mt-8">
            <p className="text-sm text-muted">
              Stok listesi şu anda yüklenemedi. Lütfen birazdan tekrar deneyin.
            </p>
          </Card>
        ) : (
          <div className="mt-8 flex flex-col gap-4">
            {[...groups.entries()].map(([code, g]) => (
              <Card key={code}>
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <h2 className="flex items-center gap-2.5 text-lg font-semibold">
                    <ServiceIcon name={g.name} iconUrl={g.iconUrl} size={28} />
                    {g.name}
                  </h2>
                  <Badge tone="neutral">{g.rows.length} ülke</Badge>
                </div>
                <ul className="mt-4 grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-4">
                  {g.rows.map((r) => (
                    <li
                      key={`${r.serviceCode}-${r.countryIso2}`}
                      className="raised flex min-h-12 items-center justify-between gap-2
                                 rounded-xl border px-3 py-2"
                    >
                      <span className="min-w-0 truncate text-sm">{r.countryName}</span>
                      {r.inStock
                        ? <Badge tone="ok">Stokta</Badge>
                        : <Badge tone="bad">Tükendi</Badge>}
                    </li>
                  ))}
                </ul>
              </Card>
            ))}
          </div>
        )}

        <Card className="mt-8 flex flex-col items-start gap-4 md:flex-row md:items-center md:justify-between">
          <p className="text-sm text-muted">
            Fiyatı görmek ve numara almak için hesabınıza giriş yapın.
          </p>
          <Link href="/kayit" className="w-full md:w-auto">
            <Button fullWidth className="md:w-auto md:px-6">Hesap aç</Button>
          </Link>
        </Card>
      </div>
    </div>
  );
}
