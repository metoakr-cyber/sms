import Link from 'next/link';
import type { Metadata } from 'next';
import { fetchPublic } from '@/lib/server-api';
import { Card, Button } from '@/components/ui';
import type { ServiceSummary } from '@/lib/types';

export const metadata: Metadata = {
  title: 'Servisler ve Stok Durumu',
  description:
    'WhatsApp, Telegram, Instagram ve yüzlerce servis için geçici numara stok ' +
    'durumu. Hangi serviste kaç ülke mevcut, güncel liste.',
  alternates: { canonical: '/fiyatlar' },
};

/**
 * Servis listesi.
 *
 * Servis × ülke MATRİSİ basılmaz: gerçek katalogda 712 servis ve 9768 stoklu
 * kombinasyon var; hepsini tek sayfada üretmek hem devasa bir HTML hem de
 * kimsenin okumadığı bir duvar demek. Sayfa servisleri gösterir, ülkeler
 * satın alma ekranında seçilir.
 */
export default async function PricingPage() {
  const data = await fetchPublic<{ items: ServiceSummary[] }>(
    '/catalog/services-in-stock', { revalidate: 300 });
  const items = [...(data?.items ?? [])].sort((a, b) => b.countryCount - a.countryCount);

  return (
    <div className="px-4 py-10 md:px-6 md:py-14">
      <div className="mx-auto max-w-6xl">
        <h1 className="text-3xl font-bold tracking-tight md:text-4xl">Servisler ve stok</h1>
        <p className="mt-3 max-w-2xl text-sm leading-relaxed text-muted md:text-base">
          Aşağıdaki servislerde şu anda stok var. Numaranın kesin fiyatı, döviz
          kuru anlık değiştiği için satın alma ekranında{' '}
          <strong className="text-[var(--text)]">size özel bir teklif</strong> olarak
          gösterilir ve teklif süresi boyunca değişmez.
        </p>

        {items.length === 0 ? (
          <Card className="mt-8">
            <p className="text-sm text-muted">
              Stok listesi şu anda yüklenemedi. Lütfen birazdan tekrar deneyin.
            </p>
          </Card>
        ) : (
          <>
            <p className="mt-6 text-sm text-muted">
              <strong className="text-[var(--text)]">{items.length}</strong> serviste stok var.
            </p>
            {/*
              SADE İŞARETLEME, BİLEREK.

              Aynı liste `Card` + `ServiceIcon` bileşenleriyle basıldığında
              sayfa 2,4 MB ediyordu: 712 öğe × ağır sınıf listesi, üstüne bir
              de RSC yükünde ikinci kez. Pazarlama sayfasının tamamı, bir
              uygulama ekranından ağır olamaz (docs/frontend-contract.md §8).

              Servis adları HTML'de kalır — SEO değeri burada.
            */}
            <ul className="mt-4 grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-4">
              {items.map((s) => (
                <li key={s.code} className="surface rounded-xl border px-3 py-2.5">
                  <span className="block truncate text-sm font-medium">{s.name}</span>
                  <span className="text-xs text-muted">{s.countryCount} ülke</span>
                </li>
              ))}
            </ul>
          </>
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
