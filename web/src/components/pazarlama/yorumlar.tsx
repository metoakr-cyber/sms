import { Belir } from '../animasyon';
import { Yildiz } from '../ikonlar';
import { Bolum, BolumBasligi, type Renk } from './parcalar';
import { YORUMLAR, type Yorum } from '@/data/yorumlar';
import { cx } from '../ui';

/**
 * Müşteri yorumları.
 *
 * 🔴 VERİ BOŞSA BÖLÜM HİÇ RENDER EDİLMEZ. `web/src/data/yorumlar.ts` bugün
 * bilerek boş; oraya YALNIZ gerçekten alınmış ve yayımlanmasına izin verilmiş
 * yorumlar yazılır. Uydurma yorum yazmak yanıltıcı reklamdır ve para
 * yatırılan bir sitede yakalandığında geri kazanılamaz.
 *
 * Bölümün kendisi HAZIR: gerçek yorumlar dosyaya eklendiği an görünür olur,
 * başka bir kod değişikliği gerekmez.
 *
 * 🔗 SIRADAKİ ADIM: bu oturumda başka bir ajan gerçek bir yorum sistemi
 * yazıyor (`api/internal/service/review`, genel uç: `GET /catalog/reviews`,
 * YALNIZ onaylı yorumları döner). Uç canlıya çıktığında bölümü beslemek için
 * bu bileşeni DEĞİŞTİRMEK GEREKMEZ — sayfa `fetchPublic` ile listeyi çekip
 * `yorumlar` özelliğiyle geçer. Statik dosya o zaman yedek olarak boş kalır.
 */

const AVATAR: Renk[] = ['mavi', 'pembe', 'yesil', 'mor', 'deniz', 'turuncu'];

const AVATAR_SINIF: Record<Renk, string> = {
  mavi:    'bg-[var(--v-mavi-zemin)]    text-[var(--v-mavi-metin)]',
  mor:     'bg-[var(--v-mor-zemin)]     text-[var(--v-mor-metin)]',
  yesil:   'bg-[var(--v-yesil-zemin)]   text-[var(--v-yesil-metin)]',
  turuncu: 'bg-[var(--v-turuncu-zemin)] text-[var(--v-turuncu-metin)]',
  pembe:   'bg-[var(--v-pembe-zemin)]   text-[var(--v-pembe-metin)]',
  deniz:   'bg-[var(--v-deniz-zemin)]   text-[var(--v-deniz-metin)]',
};

function Yildizlar({ puan }: { puan: number }) {
  return (
    <p className="flex items-center gap-0.5" aria-label={`5 üzerinden ${puan} puan`}>
      {[1, 2, 3, 4, 5].map((n) => (
        <Yildiz
          key={n}
          className={cx('size-4', n <= puan
            ? 'text-[var(--color-warn)]'
            : 'text-[var(--border)]')}
        />
      ))}
    </p>
  );
}

function Kart({ yorum, renk }: { yorum: Yorum; renk: Renk }) {
  const bas = yorum.ad.trim().split(/\s+/).map((k) => k[0]).slice(0, 2).join('')
    .toLocaleUpperCase('tr');
  return (
    <figure className="surface kart-hover flex h-full flex-col rounded-2xl border p-5
                       golge-2 md:p-6">
      {yorum.puan && <Yildizlar puan={yorum.puan} />}
      <blockquote className="mt-3 flex-1 text-[15px] italic leading-relaxed text-muted">
        “{yorum.metin}”
      </blockquote>
      <figcaption className="mt-5 flex items-center gap-3">
        <span
          className={cx('grid size-11 shrink-0 place-items-center rounded-full text-sm font-bold',
            AVATAR_SINIF[renk])}
          aria-hidden
        >
          {bas}
        </span>
        <span className="min-w-0">
          <span className="block truncate text-sm font-semibold">{yorum.ad}</span>
          {yorum.unvan && (
            <span className="block truncate text-xs text-muted">{yorum.unvan}</span>
          )}
        </span>
      </figcaption>
    </figure>
  );
}

export function Yorumlar({ yorumlar = YORUMLAR }: { yorumlar?: Yorum[] } = {}) {
  // Boş liste → bölüm HİÇ YOK. Boş bir "Kullanıcılar ne diyor?" başlığı
  // bırakmak, hiç yorum olmadığını duyurmanın en gürültülü yoludur.
  if (yorumlar.length === 0) return null;

  return (
    <Bolum aria-labelledby="yorumlar">
      <BolumBasligi
        id="yorumlar"
        hap="Kullanıcı görüşleri"
        hapRenk="pembe"
        baslik="Kullanıcılar ne"
        vurgu="diyor"
        aciklama="Yayımlanmasına izin verilen kullanıcı yorumları."
      />
      <ul className="mt-10 grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {yorumlar.map((y, i) => (
          <Belir as="li" key={`${y.ad}-${i}`} gecikme={i * 80}>
            <Kart yorum={y} renk={AVATAR[i % AVATAR.length] ?? 'mavi'} />
          </Belir>
        ))}
      </ul>
    </Bolum>
  );
}
