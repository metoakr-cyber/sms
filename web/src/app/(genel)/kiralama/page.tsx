import Link from 'next/link';
import type { Metadata } from 'next';
import { fetchPublic } from '@/lib/server-api';
import { Akordeon, Belir } from '@/components/animasyon';
import { Bolum, BolumBasligi, Hap, IkonKaro, RenkliKart } from '@/components/pazarlama/parcalar';
import { KapanisCTA } from '@/components/pazarlama/bolumler';
import { Kilit, Onay, SagOk, Saat, SmsBalon, Takvim } from '@/components/ikonlar';
import { cx } from '@/components/ui';
import type { RentalService } from '@/lib/types';

const SITE = process.env.NEXT_PUBLIC_SITE_URL ?? 'http://localhost:3000';

export const metadata: Metadata = {
  title: 'Numara Kiralama — Aylık Sanal Numara',
  description:
    'WhatsApp, Telegram, Instagram için 1 gün ile 6 ay arası süreyle numara ' +
    'kiralayın. Aylık kiralamada numara sizde kalır, gelen tüm SMS mesajlarını ' +
    'görürsünüz.',
  alternates: { canonical: '/kiralama' },
  openGraph: {
    type: 'website',
    locale: 'tr_TR',
    url: `${SITE}/kiralama`,
    title: 'Numara Kiralama — Aylık Sanal Numara | Onay360',
    description: '1 gün – 6 ay arası numara kiralama. Aylık modelde numara sizde kalır.',
    images: ['/opengraph-image'],
  },
};

const SSS: Array<[string, string]> = [
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
];

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
  // Sıra sunucudan gelir (`ListRentalServices` → `services.sort_order`); burada
  // yeniden sıralamak popülerlik sırasını `countryCount` ile ezerdi.
  const items = data?.items ?? [];

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
    <>
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
              mainEntity: SSS.map(([q, a]) => ({
                '@type': 'Question',
                name: q,
                acceptedAnswer: { '@type': 'Answer', text: a },
              })),
            },
          ]),
        }}
      />

      {/* ═══════════ Başlık ═══════════ */}
      <Bolum className="kahraman-isik pb-8 pt-12 md:pb-10 md:pt-16">
        <BolumBasligi
          seviye={1}
          hap="1 gün – 6 ay"
          hapRenk="mor"
          baslik="Numara"
          vurgu="kiralama"
          aciklama={
            <>
              Tek bir doğrulama kodu yerine{' '}
              <strong className="font-semibold text-[var(--text)]">numaranın kendisini</strong>{' '}
              kiralayın. Seçtiğiniz süre boyunca numara sizde kalır ve o süre içinde
              gelen <strong className="font-semibold text-[var(--text)]">tüm SMS
              mesajlarını</strong> görürsünüz.
            </>
          }
        />
      </Bolum>

      {/* ═══════════ İki model ═══════════ */}
      <Bolum className="py-6 md:py-10">
        <div className="grid gap-4 md:grid-cols-2 md:gap-5">
          <Belir>
            <RenkliKart>
              <div className="flex items-center gap-3">
                <IkonKaro renk="deniz" boy="md"><SmsBalon /></IkonKaro>
                <h2 className="text-lg font-bold tracking-tight">Tek kullanımlık</h2>
              </div>
              <ul className="mt-4 flex flex-col gap-2.5">
                {['Bir doğrulama kodu için', 'Numara ~20 dakika aktif',
                  'Kod gelmezse ücret iade', 'En düşük maliyet'].map((m) => (
                  <li key={m} className="flex items-start gap-2.5 text-sm text-muted">
                    <Onay className="mt-0.5 size-4 shrink-0 text-[var(--v-deniz-metin)]" />
                    {m}
                  </li>
                ))}
              </ul>
            </RenkliKart>
          </Belir>

          <Belir gecikme={90}>
            <div className="surface kart-hover relative h-full overflow-hidden rounded-2xl
                            border-2 border-[var(--v-mor-metin)] p-5 golge-3 md:p-6">
              <span className="absolute right-4 top-4">
                <Hap renk="mor">Öne çıkan</Hap>
              </span>
              <div className="flex items-center gap-3">
                <IkonKaro renk="mor" dolu boy="md"><Takvim /></IkonKaro>
                <h2 className="text-lg font-bold tracking-tight">Kiralama</h2>
              </div>
              <ul className="mt-4 flex flex-col gap-2.5">
                {['Numara süre boyunca size ait', 'Sınırsız gelen mesaj',
                  'Birden fazla servis için kullanılabilir', '1 gün ile 6 ay arası'].map((m) => (
                  <li key={m} className="flex items-start gap-2.5 text-sm text-muted">
                    <Onay className="mt-0.5 size-4 shrink-0 text-[var(--v-mor-metin)]" />
                    {m}
                  </li>
                ))}
              </ul>
            </div>
          </Belir>
        </div>
      </Bolum>

      {/* ═══════════ Süre kademeleri ═══════════ */}
      <Bolum className="surface border-y px-4 py-14 md:px-6 md:py-20" aria-labelledby="sureler">
        <Belir>
          <BolumBasligi
            id="sureler"
            hap="Esnek süre"
            hapRenk="deniz"
            baslik="Kiralama"
            vurgu="süreleri"
            aciklama="Fiyat servise, ülkeye ve süreye göre değişir. Kesin fiyat, döviz kuru
                      anlık değiştiği için satın alma ekranında size özel bir teklif olarak
                      gösterilir."
          />
        </Belir>
        <ul className="mt-10 grid grid-cols-2 gap-3 sm:grid-cols-4">
          {tiers.map((t, i) => (
            <Belir as="li" key={t.label} gecikme={Math.min(i, 4) * 70}>
              <div className={cx(
                'kart-hover relative h-full overflow-hidden rounded-2xl border p-4 text-center',
                t.highlight
                  ? 'gradyan-mor border-transparent text-white golge-3'
                  : 'raised golge-1',
              )}>
                <p className={cx('text-xl font-extrabold tracking-tight',
                  !t.highlight && 'vurgu-metin')}>{t.label}</p>
                <p className={cx('mt-1.5 text-xs leading-relaxed',
                  t.highlight ? 'text-white/90' : 'text-muted')}>{t.note}</p>
              </div>
            </Belir>
          ))}
        </ul>
      </Bolum>

      {/* ═══════════ Servisler ═══════════ */}
      <Bolum aria-labelledby="servisler">
        <Belir>
          <BolumBasligi
            id="servisler"
            hap="Canlı katalog"
            hapRenk="yesil"
            baslik="Kiralanabilir"
            vurgu="servisler"
            aciklama={
              items.length > 0
                ? `${items.length} serviste kiralama mevcut. Stoksuz servis listelenmez.`
                : 'Kiralık stok listesi sağlayıcıdan canlı çekilir.'
            }
          />
        </Belir>

        {items.length === 0 ? (
          <div className="surface mt-8 rounded-2xl border p-6 golge-1">
            <p className="text-sm text-muted">
              Şu anda kiralanabilir servis listelenmiyor. Stok sağlayıcı tarafında
              değiştikçe bu liste kendiliğinden güncellenir; birazdan tekrar bakın.
            </p>
            <Link
              href="/fiyatlar"
              className="mt-4 inline-flex min-h-11 items-center gap-2 text-sm font-semibold
                         text-[var(--vurgu)] hover:text-[var(--vurgu-guclu)]"
            >
              Tek kullanımlık numara stoğuna bak <SagOk className="size-4" />
            </Link>
          </div>
        ) : (
          /* Sade işaretleme: 495 öğe ağır bileşenle basılırsa sayfa şişer.
             Renk `.servis-listesi` nth-child kuralından gelir — HTML'e bayt eklemez. */
          <ul className="servis-listesi mt-8 grid grid-cols-2 gap-2 sm:grid-cols-3
                         lg:grid-cols-4">
            {items.map((s) => (
              <li key={s.code} className="surface rounded-xl border px-3 py-2.5 golge-1">
                <span className="block truncate text-sm font-semibold">{s.name}</span>
                <span className="text-xs text-muted">
                  {s.countryCount} ülke · {s.durationCount} süre
                </span>
              </li>
            ))}
          </ul>
        )}
      </Bolum>

      {/* ═══════════ Ne zaman kiralamalı ═══════════ */}
      <Bolum className="surface border-y px-4 py-14 md:px-6 md:py-20" aria-labelledby="nezaman">
        <Belir>
          <BolumBasligi
            id="nezaman"
            hap="Hangi durumda"
            hapRenk="turuncu"
            baslik="Ne zaman kiralamak"
            vurgu="daha doğru?"
          />
        </Belir>
        <ul className="mt-10 grid gap-4 md:grid-cols-3">
          {([
            ['Hesaba tekrar gireceksiniz',
             'Şifre sıfırlama kodu numaraya gider. Tek kullanımlık numarayla o kodu alamazsınız.',
             'mavi', <Kilit key="1" />],
            ['Birden fazla mesaj bekleniyor',
             'Bazı servisler kayıttan sonra da mesaj gönderir: giriş onayı, cihaz doğrulama.',
             'deniz', <SmsBalon key="2" />],
            ['Uzun süre kullanacaksınız',
             'Aynı numarayı haftalarca kullanacaksanız her seferinde yeni numara almaktan ucuzdur.',
             'yesil', <Saat key="3" />],
          ] as const).map(([baslik, metin, renk, ikon], i) => (
            <Belir as="li" key={baslik} gecikme={i * 90}>
              <RenkliKart>
                <IkonKaro renk={renk} dolu boy="md">{ikon}</IkonKaro>
                <h3 className="mt-4 font-semibold leading-snug">{baslik}</h3>
                <p className="mt-1.5 text-sm leading-relaxed text-muted">{metin}</p>
              </RenkliKart>
            </Belir>
          ))}
        </ul>
      </Bolum>

      {/* ═══════════ SSS (görünür metin — yapısal veriyle aynı içerik) ═══════════ */}
      <Bolum aria-labelledby="sss" icClassName="max-w-3xl">
        <Belir>
          <BolumBasligi id="sss" baslik="Kiralama" vurgu="hakkında" />
        </Belir>
        <Belir className="mt-8">
          <Akordeon sorular={SSS} />
        </Belir>
      </Bolum>

      <Bolum className="pt-4 md:pt-6">
        <KapanisCTA
          baslik="Kiralık numara almak için"
          metin="Hesap açmak ücretsiz. Süreyi ve ülkeyi satın alma ekranında seçersiniz."
          ikincilMetin="Sık sorulanlar"
          ikincilBag="/sss"
        />
      </Bolum>
    </>
  );
}
