'use client';

/**
 * useIstemciSuzgec — SAYFASIZ bir uç noktadan gelen listeyi istemcide arar ve
 * sayfalar.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * 🔴 NE ZAMAN KULLANILIR — VE NE ZAMAN KULLANILMAZ
 * ══════════════════════════════════════════════════════════════════════════
 * YALNIZ uç nokta listenin TAMAMINI döndürüyorsa kullanılır:
 *   `GET /admin/providers`         → {items}          ✅
 *   `GET /admin/deposit-methods`   → {items}          ✅
 *   `GET /admin/pricing-rules`     → {items}          ✅
 *   `GET /me/sessions`             → {items}          ✅
 *
 * Uç nokta `{items, total, limit, offset}` döndürüyorsa BU KANCA YANLIŞTIR.
 * Orada istemci yalnız o sayfadaki 25 satırı görür; arama "kaydım yok" diye
 * YANLIŞ cevap verir — oysa kayıt bir sonraki sayfadadır. Sayfalı uçlarda
 * arama sunucuya `?q=` ile gider (`/orders`, `/wallet/entries`, `/tickets`,
 * `/admin/users` böyle çalışır).
 *
 * Bu ayrım kancanın var olma sebebidir: aynı mantık dört ekranda ayrı ayrı
 * yazılırsa, beşincisini yazan kişi uç noktanın sayfalı olup olmadığına
 * bakmadan kopyalar ve sessizce yanlış cevap veren bir arama doğar.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * GECİKMELİ ARAMA (debounce) YOK
 * ══════════════════════════════════════════════════════════════════════════
 * Sunucuya istek gitmiyor; beklenecek bir şey yok. Gecikme eklemek yalnız
 * yazarken sonucun geç görünmesine yol açardı.
 */

import * as React from 'react';
import { SAYFA_BOYUTU } from './sayfalama';

export interface IstemciSuzgecSonucu<T> {
  /** Ekrana çizilecek satırlar. Veri henüz gelmediyse `undefined` KALIR. */
  sayfadakiler: readonly T[] | undefined;
  /** Süzgeçten SONRAKİ kayıt sayısı — `Sayfalama`ya ve `KayitSayaci`ya bu gider. */
  toplam: number;
  /** `Sayfalama`ya verilecek offset (kırpılmış hâli). */
  offset: number;
  setOffset: (yeni: number) => void;
}

export function useIstemciSuzgec<T>(
  /**
   * Tam liste. Yüklenirken `undefined` GELMELİ, boş dizi değil: `VeriTablosu`
   * boş diziyi "sonuç yok" sayar ve iskeletin yerine boş durumu çizer.
   */
  kayitlar: readonly T[] | undefined,
  arama: string,
  /** Satırın aranabilir metin alanları. Boş/`null` değerler atlanır. */
  aranacakAlanlar: (kayit: T) => ReadonlyArray<string | null | undefined>,
): IstemciSuzgecSonucu<T> {
  const [offset, setOffset] = React.useState(0);

  // `aranacakAlanlar` çağrı yerinde satır içi ok işlevi olarak yazılır, yani
  // her render'da yeni bir referanstır. Bağımlılığa konsaydı memo hiç
  // tutmazdı; ref'te tutulur ve süzme yalnız veri ya da arama değişince koşar.
  const alanlarRef = React.useRef(aranacakAlanlar);
  alanlarRef.current = aranacakAlanlar;

  const suzulmus = React.useMemo(() => {
    if (!kayitlar) return undefined;
    // `toLocaleLowerCase('tr')`: varsayılan katlama "I" → "i" yapar, Türkçede
    // karşılığı "ı"dır. Yönetim ekranlarında "IBAN", "IP" gibi metinler var.
    const a = arama.trim().toLocaleLowerCase('tr');
    if (!a) return kayitlar;
    return kayitlar.filter((k) =>
      alanlarRef.current(k).some((alan) => (alan ?? '').toLocaleLowerCase('tr').includes(a)),
    );
  }, [kayitlar, arama]);

  const toplam = suzulmus?.length ?? 0;

  /*
   * OFFSET KIRPILIR. İki yoldan geçersiz kalabilir: arama daraldığında ve
   * kullanıcı son sayfadaki tek kaydı SİLDİĞİNDE. Kırpılmazsa liste boş
   * çizilir ve ekran "kayıt yok" der — oysa vardır, önceki sayfadadır.
   */
  const guvenliOffset = offset >= toplam ? 0 : offset;

  return {
    sayfadakiler: suzulmus?.slice(guvenliOffset, guvenliOffset + SAYFA_BOYUTU),
    toplam,
    offset: guvenliOffset,
    setOffset,
  };
}
