import Link from 'next/link';
import type { Metadata } from 'next';
import { fetchPublic } from '@/lib/server-api';
import { Belir, Sayac } from '@/components/animasyon';
import { Bolum, BolumBasligi, IkonKaro, RenkliKart } from '@/components/pazarlama/parcalar';
import { KapanisCTA } from '@/components/pazarlama/bolumler';
import { Cuzdan, Izgara, Kure, SagOk, Saat } from '@/components/ikonlar';
import type { ServiceSummary, Country } from '@/lib/types';

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
  const [data, countries] = await Promise.all([
    fetchPublic<{ items: ServiceSummary[] }>('/catalog/services-in-stock', { revalidate: 300 }),
    fetchPublic<{ items: Country[] }>('/catalog/countries', { revalidate: 300 }),
  ]);
  const items = [...(data?.items ?? [])].sort((a, b) => b.countryCount - a.countryCount);
  const ulkeSayisi = countries?.items.length ?? 0;
  const kombinasyon = items.reduce((t, s) => t + s.countryCount, 0);

  return (
    <>
      <Bolum className="kahraman-isik pb-8 pt-12 md:pb-10 md:pt-16">
        <BolumBasligi
          seviye={1}
          hap="Canlı katalog"
          hapRenk="yesil"
          baslik="Servisler ve"
          vurgu="stok"
          aciklama={
            <>
              Aşağıdaki servislerde şu anda stok var. Numaranın kesin fiyatı, döviz
              kuru anlık değiştiği için satın alma ekranında{' '}
              <strong className="font-semibold text-[var(--text)]">size özel bir teklif</strong>{' '}
              olarak gösterilir ve teklif süresi boyunca değişmez.
            </>
          }
        />

        {/* Canlı sayılar — hiçbiri koda gömülü değil. */}
        {items.length > 0 && (
          <ul className="mt-10 grid gap-4 sm:grid-cols-3">
            {([
              ['Stoklu servis', items.length, 'mavi', <Izgara key="1" />],
              ['Ülke', ulkeSayisi, 'mor', <Kure key="2" />],
              ['Servis × ülke', kombinasyon, 'deniz', <Cuzdan key="3" />],
            ] as const).map(([etiket, deger, renk, ikon], i) => (
              <Belir as="li" key={etiket} gecikme={i * 90}>
                <RenkliKart className="flex items-center justify-between gap-4">
                  <span>
                    <span className="block text-xs font-semibold uppercase
                                     tracking-[0.09em] text-muted">{etiket}</span>
                    <span className="mt-1.5 block text-3xl font-extrabold tracking-tight">
                      <Sayac deger={deger} />
                    </span>
                  </span>
                  <IkonKaro renk={renk} boy="lg">{ikon}</IkonKaro>
                </RenkliKart>
              </Belir>
            ))}
          </ul>
        )}
      </Bolum>

      <Bolum className="py-6 md:py-10">
        {items.length === 0 ? (
          <div className="surface rounded-2xl border p-6 golge-1">
            <p className="text-sm text-muted">
              Stok listesi şu anda yüklenemedi. Lütfen birazdan tekrar deneyin.
            </p>
          </div>
        ) : (
          <>
            <h2 className="text-xl font-bold tracking-tight md:text-2xl">
              Stokta olan servisler
            </h2>
            <p className="mt-2 text-sm text-muted">
              Servis adının altında, o serviste numara alınabilen ülke sayısı yazar.
            </p>
            {/*
              SADE İŞARETLEME, BİLEREK.

              Aynı liste `Card` + `ServiceIcon` bileşenleriyle basıldığında
              sayfa 2,4 MB ediyordu: 712 öğe × ağır sınıf listesi, üstüne bir
              de RSC yükünde ikinci kez. Pazarlama sayfasının tamamı, bir
              uygulama ekranından ağır olamaz (docs/frontend-contract.md §8).

              Renk `.servis-listesi` sınıfının nth-child kuralından gelir —
              her `<li>`ye ayrı sınıf yazmak HTML'i şişirirdi (globals.css).

              Servis adları HTML'de kalır — SEO değeri burada.
            */}
            <ul className="servis-listesi mt-5 grid grid-cols-2 gap-2 sm:grid-cols-3
                           lg:grid-cols-4">
              {items.map((s) => (
                <li key={s.code} className="surface rounded-xl border px-3 py-2.5 golge-1">
                  <span className="block truncate text-sm font-semibold">{s.name}</span>
                  <span className="text-xs text-muted">{s.countryCount} ülke</span>
                </li>
              ))}
            </ul>
          </>
        )}

        <div className="mt-10 grid gap-4 md:grid-cols-3">
          {([
            ['Fiyat neden burada yazmıyor?',
             'Maliyet sağlayıcıdan dövizle geliyor ve gün içinde değişiyor. Sabit bir ' +
             'liste basmak, sizi ödeme anında farklı bir rakamla karşılaştırırdı.',
             'mavi', <Cuzdan key="a" />],
            ['Teklif ne kadar geçerli?',
             'Satın alma ekranında size özel bir teklif üretilir; teklif süresi boyunca ' +
             'fiyat değişmez.',
             'turuncu', <Saat key="b" />],
            ['Stoksuz servis görünmüyor',
             'Listede yalnız şu anda numara verilebilen servisler var. Stoksuz bir servisi ' +
             'listeleyip sonra "yok" demek, kullanıcının zamanını çalmaktır.',
             'yesil', <Izgara key="c" />],
          ] as const).map(([baslik, metin, renk, ikon], i) => (
            <Belir key={baslik} gecikme={i * 90}>
              <RenkliKart>
                <IkonKaro renk={renk} boy="md">{ikon}</IkonKaro>
                <h3 className="mt-4 font-semibold">{baslik}</h3>
                <p className="mt-1.5 text-sm leading-relaxed text-muted">{metin}</p>
              </RenkliKart>
            </Belir>
          ))}
        </div>

        <Belir className="mt-8">
          <Link
            href="/kiralama"
            className="inline-flex min-h-11 items-center gap-2 text-sm font-semibold
                       text-[var(--vurgu)] hover:text-[var(--vurgu-guclu)]"
          >
            Numara kiralamak istiyorum <SagOk className="size-4" />
          </Link>
        </Belir>
      </Bolum>

      <Bolum className="pt-4 md:pt-6">
        <KapanisCTA
          baslik="Fiyatı görmek için hesap açın"
          metin="Hesap açmak ücretsiz. Teklif, satın alma ekranında size özel üretilir."
          ikincilMetin="Sık sorulanlar"
          ikincilBag="/sss"
        />
      </Bolum>
    </>
  );
}
