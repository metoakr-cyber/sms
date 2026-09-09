'use client';

/**
 * SuzgecCubugu — süzgeç ve arama alanlarını tutan düzen kabı.
 *
 * KAPATTIĞI TEKRAR: 7 ekran aynı düzeni elle kuruyor
 * (`flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between`), sağ
 * tarafta kayıt sayacı rozeti kimi ekranda var kimi ekranda yok, ve
 * "etkin süzgeç" göstergesi YALNIZ `denetim` ekranında.
 *
 * BU BİR KAP — kendi süzgecini ÜRETMEZ. Süzgeç kontrolleri `Secim`/`Field`
 * olarak dışarıdan verilir. Kabın işi üç şeydir:
 *   1. Mobilde alt alta, `sm:` üstünde yan yana ve TABANA hizalı dizmek
 *      (etiketler farklı uzunlukta olduğu için `items-end` şart; `items-center`
 *      kutuları kaydırır).
 *   2. Sağ tarafta kayıt sayacı / ikincil bilgi için bir yuva açmak.
 *   3. Etkin süzgeç varsa "temizle" yolunu HER ekranda aynı yerde sunmak.
 *
 * §6.3: "Süzgeçten dolayı boş" ile "gerçekten boş" farklı metinlerdir —
 * kullanıcının süzgeci temizleyebilmesi bu ayrımın diğer yarısıdır.
 *
 * HAREKET YOK: süzgeç değişimi günde 100+ kez olur; sonuç ANINDA değişir.
 */

import * as React from 'react';
import { Badge, Button, cx } from '@/components/ui';

export function SuzgecCubugu({
  children,
  sag,
  etkinSayisi = 0,
  onTemizle,
  className,
}: {
  /** Süzgeç kontrolleri — `Secim`, `Field`, arama kutusu. */
  children: React.ReactNode;
  /** Sağ taraf: kayıt sayacı rozeti ya da ikincil bir eylem. */
  sag?: React.ReactNode;
  /** Kaç süzgeç etkin? >0 ise "temizle" düğmesi görünür. */
  etkinSayisi?: number;
  /** Verilmezse temizle düğmesi HİÇ çizilmez (etkin süzgeç olsa bile). */
  onTemizle?: () => void;
  className?: string;
}) {
  const temizlenebilir = etkinSayisi > 0 && Boolean(onTemizle);

  return (
    <div
      className={cx(
        'flex flex-col gap-4 sm:flex-row sm:flex-wrap sm:items-end sm:justify-between',
        className,
      )}
    >
      {/* Süzgeçler: mobilde tam genişlik, `sm:` üstünde yan yana sarar. */}
      <div className="flex flex-col gap-4 sm:flex-row sm:flex-wrap sm:items-end">
        {children}

        {temizlenebilir && (
          <Button
            variant="ghost"
            size="sm"
            onClick={onTemizle}
            /*
              Dokunma hedefleri arası ≥8px (§7.3). `sm:mb-*` ile hizalama
              YAPILMAZ; `items-end` zaten tabana oturtuyor. `Button` min-h-11
              taşıdığı için hedef 44px'in altına düşmez.
            */
          >
            Süzgeçleri temizle
            {/* Sayı da METİN kanalıyla verilir — rozet yalnız renk değil. */}
            <span className="tabular-nums">({etkinSayisi})</span>
          </Button>
        )}
      </div>

      {sag && <div className="shrink-0">{sag}</div>}
    </div>
  );
}

/**
 * Kayıt sayacı — `SuzgecCubugu`'nun `sag` yuvasına konur.
 *
 * `tabular-nums` (§3.5): sayfa değiştikçe sayı genişliği ZIPLAMASIN.
 * Rozet İÇİNE CÜMLE YAZILMAZ (§5.2 ölçülmüş ihlal: `bakiye:161` bir rozetin
 * içine tam cümle koymuş ve 320px'te yatay taşma üretiyor) — burada içerik
 * iki kelimedir ve öyle kalmalıdır.
 */
export function KayitSayaci({ toplam }: { toplam: number }) {
  if (toplam <= 0) return null;
  return (
    <Badge tone="neutral">
      <span className="tabular-nums">{toplam}</span>
      <span className="ms-1">kayıt</span>
    </Badge>
  );
}
