/**
 * SayfaBasligi — yönetim ekranlarının h1 bloğu.
 *
 * KAPATTIĞI TEKRAR: `text-2xl font-bold tracking-tight md:text-3xl` dizgisi
 * 10 ekranda BİREBİR kopyalanmıştı. Ölçek kararı (24px mobil → 30px `md:`,
 * tasarim-sistemi.md §3.1) tek bir yerde durmalı; bugün bir ekranın başlığını
 * büyütmek isteyen kişi diğer dokuzu unutur.
 *
 * 🔴 KICKER YOK. Başlığın üstüne "YÖNETİM" gibi küçük, harf aralıklı bir
 * etiket KONULMAZ — `craft-floor.md:27` bunu bir yasak (ban) sayar, varsayılan
 * değil. Bu yüzden bileşende öyle bir slot yoktur; olmayan slot doldurulamaz.
 *
 * BOŞLUK RİTMİ (§2.3): başlık ÜSTÜ boşluk başlık ALTI boşluktan büyüktür.
 * Bu bileşen kendi üst boşluğunu VERMEZ — onu sayfa kabının `gap`'i (20–24px)
 * verir; içeride açıklama başlığın 8px altındadır. 8 < 20, ritim korunur.
 * Bileşene `mt-*` eklemeyin, kabın `gap`'ini değiştirin.
 *
 * ANİMASYON YOK: sayfa başlığı her ekran yüklenişinde görülür — Emil'in
 * sıklık tablosunun ilk satırı (§4.2 "Sayfa yükleme / route geçişi → yok").
 */
import * as React from 'react';
import { cx } from '@/components/ui';

export function SayfaBasligi({
  baslik,
  aciklama,
  children,
  className,
}: {
  baslik: string;
  /** Bir cümle: operatör bu ekranda NE yapar. Sözlük yok, metin burada durur. */
  aciklama?: string;
  /** Sağ üst eylem alanı — "Yeni ekle", "Senkronla" gibi düğmeler. */
  children?: React.ReactNode;
  className?: string;
}) {
  return (
    <div
      className={cx(
        // Mobil taban: alt alta. `md:` üstünde eylemler sağa geçer.
        // `max-*` ile geri alma zinciri YOK (CLAUDE.md #17).
        'flex flex-col gap-4 md:flex-row md:items-start md:justify-between',
        className,
      )}
    >
      <div className="min-w-0">
        <h1 className="text-2xl font-bold tracking-tight md:text-3xl">{baslik}</h1>
        {aciklama && (
          // 70ch: düz metin satır uzunluğu 65–75ch bandında kalır (§3.4).
          <p className="mt-2 max-w-[70ch] text-sm text-muted">{aciklama}</p>
        )}
      </div>

      {children && (
        // `shrink-0`: uzun bir başlık eylem düğmelerini ezmesin.
        // `flex-wrap`: iki düğme 390px'te alt alta iner, taşmaz.
        <div className="flex shrink-0 flex-wrap items-center gap-2">{children}</div>
      )}
    </div>
  );
}
