'use client';

/**
 * Sayfalama — "Önceki / Sonraki" + "N–M / T" sayacı.
 *
 * KAPATTIĞI TEKRAR: 5 yönetim ekranında kopyalanmıştı (4'ü birebir), üstelik
 * sayfa boyutu sabiti İKİ FARKLI DEĞERDEYDİ (`PAGE = 25` ve `PAGE = 20`).
 * Aynı panelde iki farklı sayfa boyutu, "kaç kayıt kaldı" sorusunun ekrandan
 * ekrana farklı cevaplanması demektir.
 *
 * §6.3: "Tek bileşen, tek `limit`. Sayfa değişimi `aria-live="polite"` ile
 * duyurulur." — ikisi de burada.
 *
 * HAREKET YOK (§4.2): sayfalama günde 100+ kez kullanılır, içerik ANINDA
 * değişir. Düğmelerin `:active` basma geri bildirimi `Button`'dan gelir
 * (`scale(0.97)` · `--sure-basma`) ve o KAVRAYIŞA hizmet eder, süs değildir.
 */

import * as React from 'react';
import { Button, cx } from '@/components/ui';

/**
 * TEK sayfa boyutu. Ekranlar kendi `PAGE` sabitini TANIMLAMAZ.
 * 25 seçildi: iki değerden büyüğü, ferah satırda bile tek ekran kaydırmayla
 * taranabilir ve daha az istek üretir.
 */
export const SAYFA_BOYUTU = 25;

export function Sayfalama({
  offset,
  limit,
  toplam,
  onDegis,
  className,
}: {
  offset: number;
  limit: number;
  toplam: number;
  /** Yeni offset. Ekran bunu state'ine yazar ve sorgu yeniden çalışır. */
  onDegis: (yeniOffset: number) => void;
  className?: string;
}) {
  const oncekiVar = offset > 0;
  const sonrakiVar = offset + limit < toplam;

  // Tek sayfaya sığan liste için kontrol GÖSTERİLMEZ: iki devre dışı düğme,
  // hiçbir düğme olmamasından daha çok gürültüdür.
  if (!oncekiVar && !sonrakiVar) return null;

  const ilk = toplam === 0 ? 0 : offset + 1;
  const son = Math.min(offset + limit, toplam);

  return (
    <nav
      aria-label="Sayfalama"
      className={cx('flex items-center justify-between gap-4', className)}
    >
      <Button
        variant="outline"
        size="sm"
        disabled={!oncekiVar}
        onClick={() => onDegis(Math.max(0, offset - limit))}
      >
        Önceki
      </Button>

      {/*
        `aria-live="polite"`: sayfa değiştiğinde ekran okuyucu yeni aralığı
        duyurur. Ölçüm: `/yonetim` altında `aria-live` bugün 0 kullanımda —
        klavye kullanıcısı "Sonraki"ye bastığında hiçbir geri bildirim almıyor.
        `tabular-nums` (§3.5): sayaç sayfadan sayfaya ZIPLAMASIN; orantılı
        rakamlarda "1" dar olduğu için metin her tıklamada kayar.
        `text-sm`, `text-xs` değil — bu bir veri sayacıdır (§3.2).
      */}
      <p aria-live="polite" aria-atomic="true" className="text-sm tabular-nums text-muted">
        {ilk}–{son} / {toplam}
      </p>

      <Button
        variant="outline"
        size="sm"
        disabled={!sonrakiVar}
        onClick={() => onDegis(offset + limit)}
      >
        Sonraki
      </Button>
    </nav>
  );
}
