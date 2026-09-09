import Link from 'next/link';
import type { Metadata } from 'next';

import { fetchPublic } from '@/lib/server-api';
import { Bolum, BolumBasligi, IkonKaro, RenkliKart } from '@/components/pazarlama/parcalar';
import { KapanisCTA } from '@/components/pazarlama/bolumler';
import { Belir } from '@/components/animasyon';
import { Cuzdan, Izgara, Kure, SagOk } from '@/components/ikonlar';
import { ServisListesi, type ServisSatiri } from '@/components/katalog/servis-listesi';
import { MusteriYorumlari } from '@/components/katalog/yorumlar';
import type { ServiceSummary, Country } from '@/lib/types';

export const metadata: Metadata = {
  title: 'Tüm Servisler',
  description:
    'SMS onayı verdiğimiz servislerin tam listesi: WhatsApp, Telegram, Instagram, ' +
    'Google, Steam ve daha fazlası. Hangi serviste kaç ülkede stok var, arayarak bulun.',
  alternates: { canonical: '/servisler' },
};

/**
 * TÜM SERVİSLER.
 *
 * `/fiyatlar` sayfası bir VİTRİNDİR: ilk birkaç servisi logolu gösterir ve
 * buraya bağlanır. Bu sayfa ise TAM listedir — kullanıcı "servisleri gör"
 * dediğinde hepsini görmelidir.
 *
 * ── NEDEN İKİ AYRI UÇTAN VERİ ÇEKİLİYOR ──
 *
 * `/catalog/services-in-stock` yalnız STOKLU servisleri döner. Yalnız onu
 * kullansaydık, stoğu geçici olarak tükenmiş bir servis listeden tümüyle
 * kaybolurdu ve kullanıcı "bu site WhatsApp satmıyor" sonucuna varırdı —
 * oysa doğru cevap "şu an stok yok". `/catalog/services` tüm görünür
 * servisleri döner; ikisi burada birleştirilir ve stoksuz satır SİLİK ama
 * GÖRÜNÜR kalır. Aynı gerekçe queries/catalog.sql'deki Türkiye istisnasında
 * da yazılıdır.
 *
 * ── NEDEN FİYAT YAZMIYOR ──
 *
 * Fiyat kullanıcıya ÖZELDİR ve satın alma ekranında bir TEKLİF olarak
 * üretilir (docs/design.md §8.1): maliyet sağlayıcıdan dövizle gelir, kur
 * gün içinde değişir ve marj kuralları ürün bazında farklıdır. Buraya
 * "şu kadardan başlayan fiyatlarla" yazmak, sunucunun garanti etmediği bir
 * sayıyı garanti gibi göstermek olurdu. Genel (oturumsuz) bir fiyat ucu
 * BİLEREK YOKTUR; eklenmesi ayrı bir karardır ve fiyatlandırma ekibinindir.
 *
 * Sunucu bileşenidir: liste ilk HTML'de gelir, arama motoru servis adlarını
 * JavaScript çalıştırmadan görür (docs/frontend-contract.md §10.2).
 */
export default async function ServislerPage() {
  const [tumu, stoklu, ulkeler] = await Promise.all([
    fetchPublic<{ items: Array<{ code: string; name: string; iconUrl?: string }> }>(
      '/catalog/services',
      { revalidate: 300 },
    ),
    fetchPublic<{ items: ServiceSummary[] }>('/catalog/services-in-stock', { revalidate: 300 }),
    fetchPublic<{ items: Country[] }>('/catalog/countries', { revalidate: 300 }),
  ]);

  const stokHaritasi = new Map((stoklu?.items ?? []).map((s) => [s.code, s]));

  const satirlar: ServisSatiri[] = (tumu?.items ?? []).map((s) => {
    const stok = stokHaritasi.get(s.code);
    return {
      code: s.code,
      name: s.name,
      iconUrl: s.iconUrl,
      countryCount: stok?.countryCount ?? 0,
      inStock: !!stok,
    };
  });

  // Stoklu servisler ÖNCE, sonra en çok ülkede bulunan. Alfabetik sıra
  // kullanıcının aradığı şeyi değil, adı "A" ile başlayanı öne çıkarırdı.
  satirlar.sort((a, b) => {
    if (a.inStock !== b.inStock) return a.inStock ? -1 : 1;
    if (b.countryCount !== a.countryCount) return b.countryCount - a.countryCount;
    return a.name.localeCompare(b.name, 'tr');
  });

  const stokluSayisi = satirlar.filter((s) => s.inStock).length;
  const ulkeSayisi = ulkeler?.items.length ?? 0;
  const kombinasyon = satirlar.reduce((t, s) => t + s.countryCount, 0);

  return (
    <>
      <Bolum className="kahraman-isik pb-8 pt-12 md:pb-10 md:pt-16">
        <BolumBasligi
          seviye={1}
          hap="Tam katalog"
          hapRenk="mavi"
          baslik="Tüm"
          vurgu="servisler"
          aciklama={
            <>
              SMS onayı verdiğimiz servislerin tamamı. Arama kutusuna servis adını yazın;
              stok durumu ve kaç ülkede numara verildiği satırın üzerinde görünür. Kesin
              fiyat, kur anlık değiştiği için satın alma ekranında{' '}
              <strong className="font-semibold text-[var(--text)]">size özel bir teklif</strong>{' '}
              olarak gösterilir ve teklif süresi boyunca değişmez.
            </>
          }
        />

        {/* Canlı sayılar — hiçbiri koda gömülü değil. */}
        {satirlar.length > 0 && (
          <ul className="mt-10 grid gap-4 sm:grid-cols-3">
            {([
              ['Stokta olan servis', stokluSayisi, 'yesil', <Izgara key="1" />],
              ['Ülke', ulkeSayisi, 'mor', <Kure key="2" />],
              ['Servis × ülke', kombinasyon, 'deniz', <Cuzdan key="3" />],
            ] as const).map(([etiket, deger, renk, ikon], i) => (
              <Belir as="li" key={etiket} gecikme={i * 90}>
                <RenkliKart className="flex items-center justify-between gap-4">
                  <span>
                    <span className="block text-xs font-semibold uppercase
                                     tracking-[0.09em] text-muted">{etiket}</span>
                    <span className="mt-1.5 block text-3xl font-extrabold tracking-tight">
                      {deger}
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
        {satirlar.length === 0 ? (
          // HATA DURUMU — sayfa 500'e düşmez, ne olduğunu söyler.
          <div className="surface rounded-2xl border p-6 golge-1">
            <p className="text-sm text-muted">
              Servis listesi şu anda yüklenemedi. Lütfen birazdan tekrar deneyin.
            </p>
          </div>
        ) : (
          <ServisListesi servisler={satirlar} />
        )}

        <Belir className="mt-8">
          <Link
            href="/kiralama"
            className="inline-flex min-h-11 items-center gap-2 text-sm font-semibold
                       text-[var(--vurgu)] hover:text-[var(--vurgu-guclu)]"
          >
            Uzun süreli numara kiralamak istiyorum <SagOk className="size-4" />
          </Link>
        </Belir>
      </Bolum>

      {/* Onaylı yorum yoksa bu bölüm HİÇ RENDER EDİLMEZ. */}
      <MusteriYorumlari />

      <Bolum className="pt-4 md:pt-6">
        <KapanisCTA
          baslik="Aradığınız servis listede mi?"
          metin="Hesap açmak ücretsiz. Numara alma ekranında servisi ve ülkeyi seçin, teklifi görün."
          ikincilMetin="Sık sorulanlar"
          ikincilBag="/sss"
        />
      </Bolum>
    </>
  );
}
