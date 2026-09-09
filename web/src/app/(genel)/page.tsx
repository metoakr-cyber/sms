import Link from 'next/link';
import type { Metadata } from 'next';
import { fetchPublic } from '@/lib/server-api';
import { Button } from '@/components/ui';
import { Belir, Akordeon } from '@/components/animasyon';
import { Bolum, BolumBasligi, Hap, IkonKaro } from '@/components/pazarlama/parcalar';
import { Istatistikler } from '@/components/pazarlama/istatistikler';
import {
  Adimlar, HizmetKartlari, KapanisCTA, OzellikIzgarasi, PopulerServisler,
} from '@/components/pazarlama/bolumler';
import { Yorumlar } from '@/components/pazarlama/yorumlar';
import { Iade, Kilit, SagOk, Yildirim } from '@/components/ikonlar';
import { yazilariGetir } from './blog/icerik';
import type { ServiceSummary, Country } from '@/lib/types';

export const metadata: Metadata = {
  title: 'Sanal Numara ile Anında SMS Onay Kodu',
  description:
    'WhatsApp, Telegram, Instagram ve yüzlerce servis için SMS onayı. Tek kullanımlık kod ' +
    'veya 1 gün – 6 ay numara kiralama. Kod gelmezse ücret iade edilir.',
  alternates: { canonical: '/' },
};

/** Ana sayfadaki SSS özeti — tam liste /sss sayfasında. */
const SSS_OZET: Array<[string, string]> = [
  ['Sanal numara nedir?',
   'Yalnız SMS almak için kullanılan geçici bir telefon numarasıdır. Bir servise ' +
   'kayıt olurken doğrulama kodu bu numaraya gelir ve kod ekranınızda görünür.'],
  ['Kod gelmezse ne oluyor?',
   'Numaranın süresi kod gelmeden dolarsa ücret otomatik olarak bakiyenize iade ' +
   'edilir. İade için talep açmanıza gerek yoktur; sistem bunu kendisi yapar.'],
  ['Kod ne kadar sürede geliyor?',
   'Çoğu serviste 10–30 saniye içinde gelir. Numara alındıktan sonra bekleme ' +
   'ekranı açık kalır ve kod düştüğü anda ekrana yansır.'],
  ['Numara kiralama nedir?',
   'Tek kullanımlık numara yalnız bir doğrulama kodu için verilir. Kiralamada ise ' +
   'numara seçtiğiniz süre boyunca (1 gün ile 6 ay arası) size aittir ve o süre ' +
   'içinde gelen tüm SMS mesajlarını görürsünüz.'],
];

/**
 * Ana sayfa SUNUCU bileşenidir.
 *
 * Servis ve ülke listesi HTML'e gömülü gelir — arama motoru JavaScript
 * çalıştırmadan içeriği görür. Bu liste istemcide çekilseydi sayfa botlara
 * boş görünürdü (frontend-contract.md §10.2).
 *
 * 🔴 SAYILAR CANLI VERİDEN GELİR. Sayfada hiçbir "üye sayısı", "işlem sayısı"
 * ya da büyüme yüzdesi SABİT YAZILMAZ; hepsi `/catalog/*` uçlarından okunur.
 * Katalog bugün geliştirme verisiyle küçük; gerçek katalog geldiğinde sayılar
 * kendiliğinden büyür.
 */
export default async function HomePage() {
  const [services, countries] = await Promise.all([
    // STOKLU servisler. /catalog/services stoksuzları da döner ve kullanıcı
    // ana sayfada gördüğü servisi panelde bulamaz.
    fetchPublic<{ items: ServiceSummary[] }>('/catalog/services-in-stock'),
    fetchPublic<{ items: Country[] }>('/catalog/countries'),
  ]);

  // En çok ülkede bulunan servisler öne çıkar: kullanıcının aradığı servisin
  // burada olma ihtimali en yüksek olanlar.
  const tumServisler = [...(services?.items ?? [])]
    .sort((a, b) => b.countryCount - a.countryCount);
  const populer = tumServisler.slice(0, 12);
  const ulkeSayisi = countries?.items.length ?? 0;
  // Stoklu servis × ülke kombinasyonu — türetilmiş ama GERÇEK bir sayı.
  const kombinasyon = tumServisler.reduce((t, s) => t + s.countryCount, 0);

  const yazilar = yazilariGetir().slice(0, 3);

  return (
    <>
      {/* Yapısal veri: arama sonuçlarında zengin görünüm sağlar */}
      <script
        type="application/ld+json"
        dangerouslySetInnerHTML={{
          __html: JSON.stringify({
            '@context': 'https://schema.org',
            '@type': 'WebSite',
            name: 'Onay360',
            description: 'Sanal numara ile anında SMS onay kodu.',
            inLanguage: 'tr-TR',
          }),
        }}
      />

      {/* ═══════════ Kahraman ═══════════ */}
      <section className="kahraman-isik relative overflow-hidden px-4 pb-12 pt-12
                          md:px-6 md:pb-16 md:pt-20">
        {/* İnce nokta dokusu — düz zeminin monotonluğunu kırar, metnin altında kalır. */}
        <span aria-hidden
              className="nokta-doku pointer-events-none absolute inset-0 opacity-60
                         [mask-image:radial-gradient(60%_50%_at_50%_0%,#000,transparent)]" />
        <div className="relative mx-auto max-w-6xl">
          <div className="max-w-3xl">
            <Hap>
              <span className="relative inline-flex size-2 rounded-full
                               bg-[var(--color-ok)] text-[var(--color-ok)] nabiz" />
              Kod gelmezse ücret iade
            </Hap>
            <h1 className="mt-5 text-balance text-4xl font-extrabold leading-[1.08]
                           tracking-tight sm:text-5xl md:text-6xl">
              Sanal numara ile{' '}
              <span className="vurgu-metin">anında SMS onayı</span>
            </h1>
            <p className="mt-5 max-w-2xl text-pretty text-base leading-relaxed text-muted
                          md:text-lg">
              WhatsApp, Telegram, Instagram ve yüzlerce servis için numara alın.
              Onay kodu ekranınıza otomatik düşer — kod gelmezse ücretiniz
              hesabınıza geri yüklenir.
            </p>

            <div className="mt-8 flex flex-col gap-3 sm:flex-row">
              <Link href="/kayit" className="sm:w-auto">
                <Button
                  fullWidth
                  className="cta-marka sm:w-auto sm:px-7"
                >
                  Ücretsiz hesap aç
                  <SagOk className="size-4" />
                </Button>
              </Link>
              <Link href="/fiyatlar" className="sm:w-auto">
                <Button variant="outline" fullWidth className="sm:w-auto sm:px-7">
                  Servisleri gör
                </Button>
              </Link>
            </div>

            {/* Güven şeridi — hepsi ürünün gerçekten yaptığı şeyler. */}
            <ul className="mt-8 flex flex-wrap items-center gap-x-6 gap-y-3">
              {[
                [<Yildirim key="a" />, 'Numara anında tanımlanır', 'mavi'] as const,
                [<Iade key="b" />, 'Kod gelmezse otomatik iade', 'yesil'] as const,
                [<Kilit key="c" />, 'Abonelik yok, ön ödemeli bakiye', 'mor'] as const,
              ].map(([ikon, metin, renk]) => (
                <li key={metin} className="flex items-center gap-2.5 text-sm font-medium">
                  <IkonKaro renk={renk} boy="sm">{ikon}</IkonKaro>
                  {metin}
                </li>
              ))}
            </ul>
          </div>

          {/* ═══════════ Canlı istatistikler ═══════════ */}
          <div className="mt-12 md:mt-16">
            <Istatistikler
              servisSayisi={tumServisler.length}
              ulkeSayisi={ulkeSayisi}
              kombinasyonSayisi={kombinasyon}
            />
            {tumServisler.length > 0 && (
              <p className="mt-4 text-xs text-muted">
                Sayılar canlı katalogdan okunur; stok değiştikçe kendiliğinden değişir.
              </p>
            )}
          </div>
        </div>
      </section>

      {/* ═══════════ Hizmetler ═══════════ */}
      <Bolum aria-labelledby="hizmetler">
        <Belir>
          <BolumBasligi
            id="hizmetler"
            hap="Neler sunuyoruz"
            baslik="Hizmetlerimizi"
            vurgu="keşfedin"
            aciklama="Üç farklı ihtiyaç, üç farklı çözüm. Hepsi aynı bakiyeden ödenir."
          />
        </Belir>
        <div className="mt-10">
          <HizmetKartlari />
        </div>
      </Bolum>

      {/* ═══════════ Nasıl çalışır ═══════════ */}
      <Bolum className="surface border-y px-4 py-14 md:px-6 md:py-20" aria-labelledby="nasil">
        <Belir>
          <BolumBasligi
            id="nasil"
            hap="Dört adım"
            hapRenk="deniz"
            baslik="Nasıl"
            vurgu="çalışır?"
            aciklama="Hesap açtıktan sonra numara almak dört adım sürer."
          />
        </Belir>
        <div className="mt-10">
          <Adimlar />
        </div>
      </Bolum>

      {/* ═══════════ Özellikler ═══════════ */}
      <Bolum aria-labelledby="ozellikler">
        <Belir>
          <BolumBasligi
            id="ozellikler"
            hap="Neden Onay360"
            hapRenk="mor"
            baslik="İşinizi kolaylaştıran"
            vurgu="ayrıntılar"
            aciklama="Numara almaktan kodu görmeye kadar her adım, kullanıcının parasını
                      ve zamanını korumak üzere tasarlandı."
          />
        </Belir>
        <div className="mt-12 md:mt-16">
          <OzellikIzgarasi />
        </div>
      </Bolum>

      {/* ═══════════ Popüler servisler ═══════════ */}
      <Bolum className="surface border-y px-4 py-14 md:px-6 md:py-20" aria-labelledby="servisler">
        <Belir>
          <BolumBasligi
            id="servisler"
            hap="Canlı katalog"
            hapRenk="yesil"
            baslik="Şu anda stokta olan"
            vurgu="servisler"
            aciklama={
              <>
                Stoksuz servis burada gösterilmez. Tam liste için{' '}
                <Link href="/fiyatlar"
                      className="font-medium text-[var(--vurgu)] underline underline-offset-4">
                  servis sayfasına
                </Link>{' '}
                bakın.
              </>
            }
          />
        </Belir>
        <div className="mt-10">
          <PopulerServisler servisler={populer} />
        </div>
      </Bolum>

      {/* ═══════════ Yorumlar — veri boşsa hiç render edilmez ═══════════ */}
      <Yorumlar />

      {/* ═══════════ SSS özeti ═══════════ */}
      <Bolum aria-labelledby="sss">
        <Belir>
          <BolumBasligi
            id="sss"
            hap="Sık sorulanlar"
            hapRenk="turuncu"
            baslik="Aklınıza takılan"
            vurgu="sorular"
          />
        </Belir>
        <div className="mx-auto mt-10 max-w-3xl">
          <Belir>
            <Akordeon sorular={SSS_OZET} />
          </Belir>
          <div className="mt-6 text-center">
            <Link
              href="/sss"
              className="inline-flex min-h-11 items-center gap-2 text-sm font-semibold
                         text-[var(--vurgu)] hover:text-[var(--vurgu-guclu)]"
            >
              Tüm soruları gör <SagOk className="size-4" />
            </Link>
          </div>
        </div>
      </Bolum>

      {/* ═══════════ Blog ═══════════ */}
      {yazilar.length > 0 && (
        <Bolum className="surface border-y px-4 py-14 md:px-6 md:py-20" aria-labelledby="blog">
          <Belir>
            <BolumBasligi
              id="blog"
              hap="Blog"
              baslik="Nasıl çalıştığını"
              vurgu="okuyun"
              aciklama="Sanal numaranın arkasındaki işleyiş, sade anlatımla."
            />
          </Belir>
          <ul className="mt-10 grid gap-4 md:grid-cols-3">
            {yazilar.map((y, i) => (
              <Belir as="li" key={y.slug} gecikme={i * 90}>
                <Link
                  href={`/blog/${y.slug}`}
                  className="raised kart-hover flex h-full flex-col rounded-2xl border p-5
                             golge-1 md:p-6"
                >
                  <Hap renk={y.renk}>{y.etiket}</Hap>
                  <span className="mt-4 text-base font-bold leading-snug">{y.baslik}</span>
                  <span className="mt-2 flex-1 text-sm leading-relaxed text-muted">{y.ozet}</span>
                  <span className="mt-4 inline-flex items-center gap-1.5 text-sm font-semibold
                                   text-[var(--vurgu)]">
                    Oku <SagOk className="size-4" />
                  </span>
                </Link>
              </Belir>
            ))}
          </ul>
        </Bolum>
      )}

      {/* ═══════════ Kapanış ═══════════ */}
      <Bolum>
        <KapanisCTA
          baslik="Hesap açmak ücretsiz"
          metin="Yalnız kullandığınız numara kadar ödersiniz. Abonelik yok, aylık ücret yok."
          ikincilMetin="Kiralamayı incele"
          ikincilBag="/kiralama"
        />
      </Bolum>
    </>
  );
}
