import { Belir, Sayac } from '../animasyon';
import { IkonKaro, type Renk } from './parcalar';
import { Izgara, Kure, Yildirim } from '../ikonlar';
import { cx } from '../ui';

/**
 * Canlı istatistik kartları.
 *
 * 🔴 BURADA SABİT SAYI YOKTUR. Üç değer de `/catalog/*` uçlarından okunan
 * canlı katalogdan HESAPLANIR ve sunucuda render edilir. Katalog büyüyünce
 * sayılar kendiliğinden büyür; kimse kodu güncellemez.
 *
 * 🔴 ROZETLERDE BÜYÜME İDDİASI YOKTUR. Referans tasarımdaki "+%12,5 bu ay"
 * gibi bir rozet, ölçmediğimiz bir şeyi iddia etmek olurdu. Rozetler yalnız
 * sayının NE OLDUĞUNU söyler.
 *
 * Değer okunamadıysa (API kapalı) bölüm hiç render EDİLMEZ — "0 servis"
 * yazan bir kart, boş bir kartdan daha kötüdür.
 */

type Kart = {
  etiket: string;
  deger: number;
  sonek?: string;
  rozet: string;
  renk: Renk;
  ikon: React.ReactNode;
};

export function Istatistikler({
  servisSayisi, ulkeSayisi, kombinasyonSayisi,
}: { servisSayisi: number; ulkeSayisi: number; kombinasyonSayisi: number }) {
  if (servisSayisi <= 0 && ulkeSayisi <= 0) return null;

  const kartlar: Kart[] = [
    {
      etiket: 'Stoklu servis',
      deger: servisSayisi,
      rozet: 'Şu anda numara verilebilen servisler',
      renk: 'mavi',
      ikon: <Izgara />,
    },
    {
      etiket: 'Ülke',
      deger: ulkeSayisi,
      rozet: 'Numara alınabilen ülkeler',
      renk: 'mor',
      ikon: <Kure />,
    },
    {
      etiket: 'Servis × ülke',
      deger: kombinasyonSayisi,
      rozet: 'Stokta olan kombinasyon sayısı',
      renk: 'deniz',
      ikon: <Yildirim />,
    },
  ];

  return (
    <ul className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
      {kartlar.map((k, i) => (
        <Belir as="li" key={k.etiket} gecikme={i * 90}>
          <div
            className="surface kart-hover relative h-full overflow-hidden rounded-2xl
                       border p-5 golge-2 md:p-6"
          >
            <div className="flex items-start justify-between gap-4">
              <div className="min-w-0">
                <p className="text-xs font-semibold uppercase tracking-[0.09em] text-muted">
                  {k.etiket}
                </p>
                <p className="mt-2 text-4xl font-extrabold leading-none tracking-tight md:text-5xl">
                  <Sayac deger={k.deger} sonek={k.sonek} />
                </p>
              </div>
              <IkonKaro renk={k.renk} boy="lg">{k.ikon}</IkonKaro>
            </div>

            <p className="mt-4 text-xs leading-relaxed text-muted">{k.rozet}</p>

            {/* Referans tasarımdaki kalın alt çizgi — kartın rengiyle. */}
            <span
              aria-hidden
              className={cx(
                'absolute inset-x-0 bottom-0 h-1',
                k.renk === 'mavi' ? 'gradyan-mavi'
                  : k.renk === 'mor' ? 'gradyan-mor' : 'gradyan-deniz',
              )}
            />
          </div>
        </Belir>
      ))}
    </ul>
  );
}
