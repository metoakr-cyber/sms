import Link from 'next/link';
import { Belir } from '../animasyon';
import { ServiceIcon } from '../service-icon';
import { Button, cx } from '../ui';
import { IkonKaro, RenkliKart, type Renk } from './parcalar';
import {
  Cuzdan, Iade, Izgara, Kalkan, Kure, SagOk, SmsBalon, Takvim, Telefon, Yildirim,
} from '../ikonlar';
import type { ServiceSummary } from '@/lib/types';

/* ═══════════════════════ Hizmet kartları ═══════════════════════ */

/**
 * Üç hizmet kartı — referans tasarım #2.
 *
 * İçerik ÜRÜNÜN GERÇEKTEN YAPTIĞI ÜÇ ŞEYDİR. "7/24 destek", "API erişimi"
 * gibi bugün var olmayan bir şey yazılmadı; olmayan bir özelliği satmak
 * ilk destek talebinde geri döner.
 */
const HIZMETLER: Array<{
  renk: Renk; ikon: React.ReactNode; baslik: string; metin: string;
  bag: string; bagMetin: string;
}> = [
  {
    renk: 'mavi',
    ikon: <SmsBalon />,
    baslik: 'Tek kullanımlık numara',
    metin:
      'Bir doğrulama kodu için geçici numara alın. Kod ekranınıza düştüğü anda ' +
      'görünür; gelmezse ücret bakiyenize otomatik iade edilir.',
    bag: '/fiyatlar',
    bagMetin: 'Stoklu servisleri gör',
  },
  {
    renk: 'mor',
    ikon: <Takvim />,
    baslik: 'Numara kiralama',
    metin:
      '1 gün ile 6 ay arası süreyle numaranın kendisini kiralayın. Süre boyunca ' +
      'numara sizde kalır ve gelen tüm mesajları görürsünüz.',
    bag: '/kiralama',
    bagMetin: 'Kiralama modelleri',
  },
  {
    renk: 'deniz',
    ikon: <Yildirim />,
    baslik: 'Anlık kod ekranı',
    metin:
      'Numarayı aldıktan sonra bekleme ekranı açık kalır. Kod geldiğinde sayfayı ' +
      'yenilemeniz gerekmez; kalan süre sunucu saatinden gösterilir.',
    bag: '/sss',
    bagMetin: 'Nasıl çalıştığını oku',
  },
];

export function HizmetKartlari() {
  return (
    <ul className="grid gap-4 md:grid-cols-3 md:gap-5">
      {HIZMETLER.map((h, i) => (
        <Belir as="li" key={h.baslik} gecikme={i * 90}>
          <RenkliKart className="flex flex-col">
            <IkonKaro renk={h.renk} dolu boy="lg">{h.ikon}</IkonKaro>
            <h3 className="mt-5 text-lg font-bold tracking-tight">{h.baslik}</h3>
            <p className="mt-2 flex-1 text-sm leading-relaxed text-muted">{h.metin}</p>
            <Link
              href={h.bag}
              className="group mt-5 inline-flex min-h-11 items-center gap-2 text-sm
                         font-semibold text-[var(--vurgu)] transition-colors
                         hover:text-[var(--vurgu-guclu)]"
            >
              {h.bagMetin}
              <SagOk className="size-4 transition-transform group-hover:translate-x-1" />
            </Link>
          </RenkliKart>
        </Belir>
      ))}
    </ul>
  );
}

/* ═══════════════════════ Nasıl çalışır ═══════════════════════ */

const ADIMLAR: Array<[string, string, Renk, React.ReactNode]> = [
  ['Servisi seçin',
   'WhatsApp, Telegram, Instagram… Hangi servis için numara gerekiyorsa onu seçin.',
   'mavi', <Izgara key="a" />],
  ['Ülkeyi seçin',
   'Yalnız stokta olan ülkeler listelenir; fiyat size özel bir teklif olarak gösterilir.',
   'mor', <Kure key="b" />],
  ['Numarayı alın',
   'Numara anında hesabınıza tanımlanır ve tutar bakiyenizden düşer.',
   'deniz', <Telefon key="c" />],
  ['Kodu bekleyin',
   'Kod ekranınıza otomatik düşer. Süre kod gelmeden dolarsa ücret iade edilir.',
   'yesil', <SmsBalon key="d" />],
];

export function Adimlar() {
  return (
    <ol className="relative grid gap-4 md:grid-cols-2 lg:grid-cols-4 lg:gap-5">
      {/* Masaüstünde adımları birleştiren ince çizgi. Dekoratiftir. */}
      <span
        aria-hidden
        className="pointer-events-none absolute inset-x-[12%] top-11 hidden h-px
                   bg-gradient-to-r from-transparent via-[var(--border)] to-transparent lg:block"
      />
      {ADIMLAR.map(([baslik, metin, renk, ikon], i) => (
        <Belir as="li" key={baslik} gecikme={i * 90} className="relative">
          <RenkliKart className="flex flex-col items-start">
            <div className="flex w-full items-center justify-between">
              <IkonKaro renk={renk} dolu boy="md">{ikon}</IkonKaro>
              <span
                aria-hidden
                className="text-3xl font-extrabold leading-none tracking-tight
                           text-[var(--border)]"
              >
                {String(i + 1).padStart(2, '0')}
              </span>
            </div>
            <h3 className="mt-4 font-semibold">{baslik}</h3>
            <p className="mt-1.5 text-sm leading-relaxed text-muted">{metin}</p>
          </RenkliKart>
        </Belir>
      ))}
    </ol>
  );
}

/* ═══════════════════════ Özellik ızgarası ═══════════════════════ */

const SOL: Array<[string, string, Renk, React.ReactNode]> = [
  ['Kod anında düşer',
   'Bekleme ekranı açık kalır; kod geldiğinde sayfayı yenilemeniz gerekmez.',
   'mavi', <Yildirim key="1" />],
  ['Gelmezse iade',
   'Süre kod gelmeden dolarsa ücret bakiyenize otomatik döner. Talep açmanıza gerek yok.',
   'yesil', <Iade key="2" />],
  ['Yüzlerce servis',
   'Katalog sağlayıcıdan canlı çekilir; stoksuz servis listede gösterilmez.',
   'mor', <Izgara key="3" />],
];

const SAG: Array<[string, string, Renk, React.ReactNode]> = [
  ['Çoklu ülke',
   'Aynı servis için farklı ülkelerden numara alabilirsiniz.',
   'deniz', <Kure key="4" />],
  ['Ön ödemeli bakiye',
   'Abonelik yok. Yalnız aldığınız numara kadar ödersiniz, bakiye TL olarak durur.',
   'turuncu', <Cuzdan key="5" />],
  ['Numara size özel',
   'Verilen numara o doğrulama boyunca yalnız sizindir; kod başkasına gitmez.',
   'pembe', <Kalkan key="6" />],
];

function Ozellik({
  baslik, metin, renk, ikon, saga,
}: { baslik: string; metin: string; renk: Renk; ikon: React.ReactNode; saga?: boolean }) {
  return (
    <div className={cx('flex gap-4', saga && 'lg:flex-row-reverse lg:text-right')}>
      <IkonKaro renk={renk} boy="md">{ikon}</IkonKaro>
      <div className="min-w-0">
        <h3 className="font-semibold">{baslik}</h3>
        <p className="mt-1 text-sm leading-relaxed text-muted">{metin}</p>
      </div>
    </div>
  );
}

/**
 * Ortadaki telefon görseli SVG değil, HTML+CSS'tir: tema değişkenlerini
 * doğrudan kullanır, `alt` metni gerektirmez (dekoratif) ve ölçeklenirken
 * yazı tipi hinting'i bozulmaz.
 */
function TelefonGorseli() {
  return (
    <div className="relative mx-auto w-[13.5rem] shrink-0" aria-hidden>
      {/* Marka halkası */}
      <span className="absolute left-1/2 top-1/2 -z-10 size-[19rem] -translate-x-1/2
                       -translate-y-1/2 rounded-full opacity-70 blur-2xl gradyan-marka" />
      {/* Sarı vurgu — referans görseldeki küçük aksan */}
      <span className="absolute -right-5 top-8 size-9 rotate-12 rounded-2xl
                       bg-[var(--color-warn)] golge-2" />
      <span className="absolute -left-6 bottom-24 size-7 -rotate-12 rounded-xl
                       bg-[var(--color-kirmizi-500)] golge-2" />

      <div className="raised overflow-hidden rounded-[2rem] border-4 border-[var(--border)]
                      golge-3">
        <div className="gradyan-marka px-4 pb-6 pt-5 text-white">
          <p className="text-[10px] font-semibold uppercase tracking-[0.14em] opacity-90">
            Onay360
          </p>
          <p className="mt-1 text-sm font-semibold">Kod bekleniyor…</p>
        </div>
        <div className="flex flex-col gap-3 p-4">
          <div className="rounded-xl bg-[var(--v-mavi-zemin)] px-3 py-2.5">
            <p className="text-[10px] font-semibold uppercase tracking-wide
                          text-[var(--v-mavi-metin)]">Numara</p>
            <p className="mt-0.5 font-mono text-sm font-semibold">+90 5•• ••• •• ••</p>
          </div>
          <div className="surface rounded-xl border px-3 py-2.5">
            <p className="text-[10px] font-semibold uppercase tracking-wide text-muted">
              Gelen kod
            </p>
            <p className="mt-1 font-mono text-2xl font-extrabold tracking-[0.18em]
                          vurgu-metin">
              ••••
            </p>
          </div>
          <div className="flex items-center gap-2 text-xs text-muted">
            <span className="relative inline-flex size-2 rounded-full
                             bg-[var(--color-ok)] text-[var(--color-ok)] nabiz" />
            Bağlantı canlı
          </div>
        </div>
      </div>
    </div>
  );
}

export function OzellikIzgarasi() {
  return (
    <div className="grid items-center gap-10 lg:grid-cols-[1fr_auto_1fr] lg:gap-12">
      <Belir className="flex flex-col gap-7 lg:gap-9">
        {SOL.map(([b, m, r, i]) => (
          <Ozellik key={b} baslik={b} metin={m} renk={r} ikon={i} saga />
        ))}
      </Belir>

      <Belir gecikme={80} className="order-first lg:order-none">
        <TelefonGorseli />
      </Belir>

      <Belir gecikme={160} className="flex flex-col gap-7 lg:gap-9">
        {SAG.map(([b, m, r, i]) => (
          <Ozellik key={b} baslik={b} metin={m} renk={r} ikon={i} />
        ))}
      </Belir>
    </div>
  );
}

/* ═══════════════════════ Popüler servisler ═══════════════════════ */

const SERVIS_RENKLERI: Renk[] = ['mavi', 'mor', 'deniz', 'yesil', 'turuncu', 'pembe'];

export function PopulerServisler({ servisler }: { servisler: ServiceSummary[] }) {
  if (servisler.length === 0) {
    return (
      <div className="surface rounded-2xl border p-6 golge-1">
        <p className="text-sm text-muted">
          Servis listesi şu anda yüklenemedi. Lütfen birazdan tekrar deneyin.
        </p>
      </div>
    );
  }

  return (
    <ul className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6">
      {servisler.map((s, i) => (
        <Belir as="li" key={s.code} gecikme={Math.min(i, 6) * 60}>
          <div
            className="surface kart-hover flex h-full min-h-28 flex-col items-center
                       justify-center gap-2.5 rounded-2xl border p-3 text-center golge-1"
          >
            <ServiceIcon
              name={s.name}
              iconUrl={s.iconUrl}
              size={40}
              renk={SERVIS_RENKLERI[i % SERVIS_RENKLERI.length] ?? 'mavi'}
            />
            <span className="break-anywhere text-sm font-semibold">{s.name}</span>
            <span className="text-[11px] text-muted">{s.countryCount} ülke</span>
          </div>
        </Belir>
      ))}
    </ul>
  );
}

/* ═══════════════════════ Kapanış ═══════════════════════ */

export function KapanisCTA({
  baslik, metin, birincilMetin = 'Ücretsiz hesap aç', birincilBag = '/kayit',
  ikincilMetin, ikincilBag,
}: {
  baslik: string; metin: string;
  birincilMetin?: string; birincilBag?: string;
  ikincilMetin?: string; ikincilBag?: string;
}) {
  return (
    <Belir>
      <div className="gradyan-marka relative overflow-hidden rounded-3xl p-7 text-white
                      golge-3 md:p-12">
        {/* Dekoratif halkalar — metnin altında kalır, kontrastı etkilemez. */}
        <span aria-hidden className="pointer-events-none absolute -right-16 -top-20 size-72
                                     rounded-full bg-white/10" />
        <span aria-hidden className="pointer-events-none absolute -bottom-24 left-8 size-56
                                     rounded-full bg-white/[0.07]" />
        <div className="relative flex flex-col items-start gap-6 md:flex-row md:items-center
                        md:justify-between">
          <div className="max-w-xl">
            <h2 className="text-2xl font-bold tracking-tight md:text-3xl">{baslik}</h2>
            {/* Beyaz metin gradyanın en açık ucunda bile 4.79:1 (ölçüldü). */}
            <p className="mt-2 text-sm leading-relaxed text-white/90 md:text-base">{metin}</p>
          </div>
          <div className="flex w-full flex-col gap-3 sm:flex-row md:w-auto md:shrink-0">
            <Link href={birincilBag} className="sm:w-auto">
              <Button
                fullWidth
                className="border border-white/20 !bg-white !text-[#14406b] shadow-lg
                           hover:!bg-white/90 sm:w-auto sm:px-7"
              >
                {birincilMetin}
              </Button>
            </Link>
            {ikincilMetin && ikincilBag && (
              <Link href={ikincilBag} className="sm:w-auto">
                <Button
                  variant="outline"
                  fullWidth
                  className="!border-white/45 !text-white hover:!bg-white/15 sm:w-auto sm:px-7"
                >
                  {ikincilMetin}
                </Button>
              </Link>
            )}
          </div>
        </div>
      </div>
    </Belir>
  );
}
