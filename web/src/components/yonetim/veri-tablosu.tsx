'use client';

/**
 * VeriTablosu — masaüstünde tablo, `< md` altında KART. Tek veri tanımı.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * KAPATTIĞI TEKRAR — projenin en büyüğü
 * ══════════════════════════════════════════════════════════════════════════
 * 7 ekran (`denetim` `destek` `fiyatlar` `kullanicilar` `odeme-yontemleri`
 * `saglayicilar` `talepler`) aynı veriyi İKİ KEZ elle yazıyor: bir mobil kart
 * listesi, bir masaüstü tablosu. Ölçüm: ~661 satır ikiz kod.
 *
 * 🔴 VE KOPYALAMA ZATEN BOZULMUŞTU. Aynı veri iki yerde farklı etiketlenmiş:
 *      saglayicilar  kart "Anahtar kurulu değil"  ↔ tablo "Kurulu değil"
 *      saglayicilar  kart "Katalogu senkronla"    ↔ tablo "Senkronla"
 *      talepler      kart "İncele ve karar ver"   ↔ tablo "İncele"
 * Bu bileşen bunu YAPISAL OLARAK İMKÂNSIZ kılar: `baslik` tek bir dizedir ve
 * hem `<th>` hem kart `<dt>` olarak aynı yerden okunur.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * `< md` ALTINDA TABLO YOKTUR — KART VARDIR (CLAUDE.md #17 · §6.1)
 * ══════════════════════════════════════════════════════════════════════════
 * `overflow-x: auto` bir çözüm değil, bir ertelemedir. Bugün 7/7 tablo bu
 * kurala uyuyor ve `/yonetim` altında `overflow-x` sıfır kullanımda — bu
 * panelin en sağlam yanıdır ve bu bileşen onu bozamayacak şekilde kuruldu:
 * tablo `hidden md:block`, kart listesi `md:hidden`. İkisi aynı anda görünmez,
 * hiçbir genişlikte yatay kaydırma üretilemez.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * HAREKET — YOK (tasarim-sistemi.md §4.2)
 * ══════════════════════════════════════════════════════════════════════════
 * Tablo satırı günde 100+ kez görülür: Emil'in sıklık tablosunun ilk satırı,
 * "Animasyon yok. Asla." Satır girişi, stagger, sayfa geçişi — hiçbiri yok.
 * Tek geçiş satır hover'ının RENGİdir ve `--sure-hizli` (120ms) ile sınırlıdır;
 * §4.2'nin izin verdiği ≤150ms tavanının altında. Süre JETONDAN gelir.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * FERAH YOĞUNLUK — kullanıcı kararı (9 Eylül 2026)
 * ══════════════════════════════════════════════════════════════════════════
 * Satır dolgusu `py-4` (16+16px). `text-sm` × `leading-normal` = 21px gövde
 * ile satır ≈ 53px + 1px kenarlık. Gerekçe tercih değil: bu ekranlar gerçek
 * para hareketi yapıyor ve sıkışık bir satırda YANLIŞ SATIRA TIKLAMAK finansal
 * bir hatadır. Yarım boşluk basamağı (`py-3.5`) kullanılmaz (§2.3).
 */

import * as React from 'react';
import type { ApiError } from '@/lib/api';
import { Skeleton, cx } from '@/components/ui';
import { HataDurumu } from './hata-durumu';

/** Hücre hangi sunumda çiziliyor. Bkz. `Sutun.hucre`. */
export type Sunum = 'tablo' | 'kart';

export type SutunHizasi = 'sol' | 'sag';

/**
 * Sütun önceliği — §6.1 kural 3: "sütun önceliği veriden gelir, ekran
 * yazarının insafına bırakılmaz."
 *   1 → her zaman görünür + mobil kartta yer alır
 *   2 → `md:` ve üstü (tablo zaten `md:`'de başladığı için pratikte "her zaman")
 *   3 → `lg:` ve üstü — 768px'te sığmayan sütunlar buraya alınır
 *
 * 🔴 ÖLÇÜLMÜŞ RİSK: `saglayicilar` 8, `fiyatlar` 7, `kullanicilar` 7 sütun
 * taşıyor ve HİÇBİRİNDE `lg:` gizlemesi yok. 768px'te 8 sütun, yatay kaydırma
 * yasağının sınırındadır. Beşten çok sütunu olan her tablo, fazlasına
 * `oncelik: 3` vermek zorundadır.
 */
export type SutunOnceligi = 1 | 2 | 3;

/** Sütunun mobil karttaki rolü. Verilmezse sütun kartın `<dl>` gövdesine iner. */
export type MobilRol =
  /** Kartın üst satırı, solda — kaydın kimliği (kullanıcı adı, servis kodu). */
  | 'baslik'
  /** Kartın üst satırı, sağda — tek bir rozet. */
  | 'rozet'
  /** Kartın en altı, tam genişlik — satırın birincil eylemi. */
  | 'eylem';

export interface Sutun<T> {
  /** React anahtarı ve hata ayıklama adı. Veri alanı adıyla aynı olması iyidir. */
  anahtar: string;
  /**
   * 🔴 TEK METİN KAYNAĞI. Hem `<th>` hem de mobil kart `<dt>` bunu kullanır.
   * Ölçülen etiket ayrışması tam olarak burada iki ayrı dize yazılmasından
   * doğmuştu; bu alan tek olduğu sürece tekrar doğamaz.
   */
  baslik: string;
  /** Sayı/para/tarih sütunları sağa hizalanır (§6.1 kural 4). */
  hizala?: SutunHizasi;
  /**
   * Para / sayı / tarih mi? `tabular-nums` uygular.
   * 🔴 §3.5: bu ZORUNLUDUR ve "modellerin en güvenilir biçimde atladığı şey"
   * olarak işaretlenmiştir. Orantılı rakamlarda `1` diğerlerinden dardır;
   * alt alta gelen tutarlar kayar ve bir bakiye tablosunda bu EN GÖRÜNÜR
   * craft hatasıdır. Ölçüm: `/yonetim` altında bugün 0 kullanım.
   */
  sayisal?: boolean;
  /** Bkz. `SutunOnceligi`. Varsayılan 1. */
  oncelik?: SutunOnceligi;
  /**
   * Sütunun mobil karttaki yeri. Verilmezse kartın `<dl>` gövdesine
   * "etiket: değer" satırı olarak iner — sütunların çoğu böyledir.
   * Her rolden kartta EN FAZLA BİR tane bulunur; birden çok verilirse ilki
   * kullanılır (React `find`). Bu bir kısıt değil, kartın anatomisidir:
   * bir kimlik, bir durum, bir eylem.
   */
  mobilRol?: MobilRol;
  /** Mobil kartta hiç gösterilmez (masaüstünde kalır). */
  mobilGizle?: boolean;
  /** `<th>` metnini görsel olarak gizler ama ekran okuyucuya bırakır (eylem sütunu). */
  basligiGizle?: boolean;
  /**
   * Hücre içeriği.
   *
   * `sunum` YALNIZ SUNUM FARKI İÇİNDİR — masaüstünde dar bir düğme, mobilde
   * `fullWidth` bir düğme gibi. 🔴 `sunum`'a bakıp FARKLI METİN yazmak
   * YASAKTIR: ölçülen üç etiket ayrışmasının tamamı buydu. Metin `baslik`ten
   * ve veriden gelir, sunumdan değil.
   */
  hucre: (satir: T, sunum: Sunum) => React.ReactNode;
}

export interface VeriTablosuProps<T> {
  /**
   * `<caption>` metni — görsel olarak gizli, ama VAR OLMAK ZORUNDA (§6.2).
   * Ölçüm: bugün 0/7 tabloda caption var; ekran okuyucu tablonun ne olduğunu
   * bilmiyor. Bu yüzden zorunlu prop.
   */
  baslik: string;
  sutunlar: ReadonlyArray<Sutun<T>>;
  /** Yüklenirken `undefined` olabilir. */
  satirlar: readonly T[] | undefined;
  /** Satırın kararlı kimliği — `public_id` (CLAUDE.md #10). Dizin KULLANMAYIN. */
  satirAnahtari: (satir: T) => string;

  /* ─── §6.1 kural 5: yükleniyor / boş / hata ÜÇÜ DE ZORUNLU PROP ─── */

  yukleniyor: boolean;
  /** `apiHatasi(q.error)` ile daraltılmış hata; hata yoksa `null`. */
  hata: ApiError | null;
  /**
   * Boş durum. `<Empty>` verilebilir.
   * 🔴 §6.3: süzgeçten dolayı boş ile GERÇEKTEN boş FARKLI metinlerdir —
   * "sonuç yok + süzgeci temizle" ile "ilk kaydı oluştur" aynı ekran değildir.
   * Bu yüzden metin bileşende sabitlenmedi, çağırana bırakıldı.
   */
  bos: React.ReactNode;

  /** İskelet satır sayısı. Varsayılan 5. */
  iskeletSatir?: number;
  /**
   * Liste sonucunu `aria-live="polite"` ile duyur (§7.4). Varsayılan açık.
   * Ölçüm: `/yonetim` altında `aria-live` bugün 0 kullanımda — süzgeç
   * değiştiğinde ekran okuyucuya hiçbir şey söylenmiyor.
   * Aynı ekranda `Sayfalama` da duyuruyorsa `false` verip tek duyuru bırakın.
   */
  duyuru?: boolean;
  className?: string;
}

/* ═══════════════════════ Sınıf yardımcıları ═══════════════════════ */

function hizaSinifi<T>(s: Sutun<T>): string {
  return s.hizala === 'sag' ? 'text-right' : 'text-left';
}

function oncelikSinifi<T>(s: Sutun<T>): string | false {
  if (s.oncelik === 3) return 'hidden lg:table-cell';
  if (s.oncelik === 2) return 'hidden md:table-cell';
  return false;
}

export function VeriTablosu<T>({
  baslik,
  sutunlar,
  satirlar,
  satirAnahtari,
  yukleniyor,
  hata,
  bos,
  iskeletSatir = 5,
  duyuru = true,
  className,
}: VeriTablosuProps<T>) {
  // Kart projeksiyonu: sütun tanımından TÜRETİLİR, elle yazılmaz.
  const kartSutunlari = sutunlar.filter((s) => !s.mobilGizle);
  const kartBaslik = kartSutunlari.find((s) => s.mobilRol === 'baslik');
  const kartRozet = kartSutunlari.find((s) => s.mobilRol === 'rozet');
  const kartEylem = kartSutunlari.find((s) => s.mobilRol === 'eylem');
  const kartVeri = kartSutunlari.filter((s) => s.mobilRol === undefined);

  const duyuruMetni = !duyuru
    ? ''
    : yukleniyor
      ? ''
      : hata
        ? 'Liste yüklenemedi.'
        : !satirlar || satirlar.length === 0
          ? 'Sonuç bulunamadı.'
          : `${satirlar.length} kayıt listelendi.`;

  return (
    <div className={className}>
      {/*
        Canlı bölge DAİMA DOM'da durur — koşullu render edilirse ekran okuyucu
        onu "yeni eklenmiş içerik" saymaz ve HİÇBİR ŞEY duyurulmaz. Bu, canlı
        bölge hatalarının en yaygın biçimidir.
      */}
      <p aria-live="polite" aria-atomic="true" className="sr-only">
        {duyuruMetni}
      </p>

      {yukleniyor ? (
        <Iskelet sutunlar={sutunlar} satirSayisi={iskeletSatir} baslik={baslik} />
      ) : hata ? (
        <HataDurumu hata={hata} />
      ) : !satirlar || satirlar.length === 0 ? (
        <>{bos}</>
      ) : (
        <>
          {/* ─────────── MOBİL: kart listesi (< md) ─────────── */}
          <ul aria-label={baslik} className="flex flex-col gap-3 md:hidden">
            {satirlar.map((satir) => (
              <li
                key={satirAnahtari(satir)}
                /*
                  Yönetim kartı DERİNLİK değil KENARLIK kullanır (§2.4):
                  `--golge-*` kartlara serpilmez. İÇ İÇE KART DA YOKTUR
                  (§9.2, "nested cards are always wrong") — bu `<li>` zaten
                  bir `Card`'ın içinde durur, ikinci bir `Card` açılmaz.
                */
                className="raised rounded-xl border p-4"
              >
                {(kartBaslik || kartRozet) && (
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">{kartBaslik?.hucre(satir, 'kart')}</div>
                    {kartRozet && <div className="shrink-0">{kartRozet.hucre(satir, 'kart')}</div>}
                  </div>
                )}

                {kartVeri.length > 0 && (
                  <dl
                    className={cx(
                      'flex flex-col gap-3 text-sm',
                      (kartBaslik || kartRozet) && 'mt-4 border-t border-[var(--border)] pt-4',
                    )}
                  >
                    {kartVeri.map((s) => (
                      <div key={s.anahtar} className="flex items-start justify-between gap-4">
                        <dt className="shrink-0 text-muted">{s.baslik}</dt>
                        <dd
                          className={cx(
                            'min-w-0 break-anywhere text-right font-medium',
                            s.sayisal && 'tabular-nums',
                          )}
                        >
                          {s.hucre(satir, 'kart')}
                        </dd>
                      </div>
                    ))}
                  </dl>
                )}

                {kartEylem && <div className="mt-4">{kartEylem.hucre(satir, 'kart')}</div>}
              </li>
            ))}
          </ul>

          {/* ─────────── MASAÜSTÜ: gerçek tablo (md: ve üstü) ─────────── */}
          <div className="hidden md:block">
            <table className="w-full text-sm">
              <caption className="sr-only">{baslik}</caption>
              <thead>
                <tr className="border-b border-[var(--border)] text-muted">
                  {sutunlar.map((s) => (
                    <th
                      key={s.anahtar}
                      scope="col"
                      /*
                        `text-sm` — `text-xs` DEĞİL. §3.2: veri taşıyan hiçbir
                        metin 14px altına inmez ve tablo başlığı sütunun ne
                        olduğunu söyleyen metindir.
                        `uppercase` KULLANILMADI: CSS büyük harf dönüşümü
                        Türkçede `i → I` üretir (`İ` değil) ve "İşlem" gibi
                        başlıkları bozar. Başlığı gövdeden ayıran şey ağırlık
                        ve renktir (§3.3), harf biçimi değil.
                      */
                      className={cx(
                        'py-3 pr-4 font-medium last:pr-0',
                        hizaSinifi(s),
                        oncelikSinifi(s),
                      )}
                    >
                      {s.basligiGizle ? <span className="sr-only">{s.baslik}</span> : s.baslik}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {satirlar.map((satir) => (
                  <tr
                    key={satirAnahtari(satir)}
                    /*
                      HOVER RENGİ `--bg`, `--raised` DEĞİL — ölçüldü:
                      açık temada `--raised` (#ffffff) `--surface` (#ffffff) ile
                      AYNIDIR, yani bir `Card` içinde hover GÖRÜNMEZ. `--bg`
                      iki temada da yüzeyden ayrışır (açık #f5f7fb / koyu #0b1120).
                      Yalnız RENK geçer; konum/ölçek hareketi yok (§4.2).
                    */
                    className="border-b border-[var(--border)] last:border-0
                               hover:bg-[var(--bg)]
                               [transition-property:background-color]
                               [transition-duration:var(--sure-hizli)]"
                  >
                    {sutunlar.map((s) => (
                      <td
                        key={s.anahtar}
                        /* py-4 → satır ≈ 53px: FERAH yoğunluk kararı (dosya başı). */
                        className={cx(
                          'py-4 pr-4 align-middle last:pr-0',
                          hizaSinifi(s),
                          oncelikSinifi(s),
                          s.sayisal && 'tabular-nums whitespace-nowrap',
                        )}
                      >
                        {s.hucre(satir, 'tablo')}
                      </td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </>
      )}
    </div>
  );
}

/**
 * Yükleme İSKELETLE gösterilir, içerik ortasında spinner ile değil
 * (`operate.md:34` · §5.1). İskelet tablo ŞEMASINDAN türer: sütun sayısı ve
 * satır yüksekliği gerçek tabloyla aynıdır, böylece veri geldiğinde düzen
 * ZIPLAMAZ. Ölçüm: bugün 10 ekranda 4 farklı iskelet yüksekliği var
 * (`h-12/h-20/h-24/h-28`) ve hiçbiri gerçek satır yüksekliğiyle uyuşmuyor.
 */
function Iskelet<T>({
  sutunlar,
  satirSayisi,
  baslik,
}: {
  sutunlar: ReadonlyArray<Sutun<T>>;
  satirSayisi: number;
  baslik: string;
}) {
  const satirlar = Array.from({ length: satirSayisi }, (_, i) => i);

  return (
    <>
      {/* `aria-hidden` iskeletlerde: ekran okuyucuya "yükleniyor" bilgisini
          canlı bölge verir, boş kutular değil. `Skeleton` zaten aria-hidden. */}
      <ul aria-label={baslik} className="flex flex-col gap-3 md:hidden">
        {satirlar.map((i) => (
          <li key={i} className="raised rounded-xl border p-4">
            <div className="flex items-start justify-between gap-3">
              <Skeleton className="h-5 w-32" />
              <Skeleton className="h-5 w-20" />
            </div>
            <div className="mt-4 flex flex-col gap-3 border-t border-[var(--border)] pt-4">
              <Skeleton className="h-4 w-full" />
              <Skeleton className="h-4 w-4/5" />
              <Skeleton className="h-4 w-3/5" />
            </div>
          </li>
        ))}
      </ul>

      <div className="hidden md:block">
        <table className="w-full text-sm">
          <caption className="sr-only">{baslik} — yükleniyor</caption>
          <thead>
            <tr className="border-b border-[var(--border)] text-muted">
              {sutunlar.map((s) => (
                <th
                  key={s.anahtar}
                  scope="col"
                  className={cx('py-3 pr-4 font-medium last:pr-0', hizaSinifi(s), oncelikSinifi(s))}
                >
                  {s.basligiGizle ? <span className="sr-only">{s.baslik}</span> : s.baslik}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {satirlar.map((i) => (
              <tr key={i} className="border-b border-[var(--border)] last:border-0">
                {sutunlar.map((s) => (
                  <td key={s.anahtar} className={cx('py-4 pr-4 last:pr-0', oncelikSinifi(s))}>
                    <Skeleton className={cx('h-4', s.hizala === 'sag' ? 'ml-auto w-16' : 'w-24')} />
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </>
  );
}
