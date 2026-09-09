'use client';

/**
 * Secim (`<select>`) ve CokSatir (`<textarea>`) — `Field` ailesinin eksik iki üyesi.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * KAPATTIĞI TEKRAR
 * ══════════════════════════════════════════════════════════════════════════
 * `ui.tsx`'teki `Field` YALNIZ `<input>` sarıyor. Sonuç ölçüldü:
 *   · `selectClass` sabiti     6 dosyada, İKİ FARKLI STİLDE
 *   · `textareaClass` sabiti   5 dosyada
 *   · yerel `Chevron` bileşeni 4 dosyada BİREBİR
 *   · elle `<label>` sarmalayıcı 21 kez — `htmlFor`/`useId`/`aria-describedby`
 *     zinciri her seferinde yeniden kuruluyor, bir yerde unutulursa sessizce
 *     erişilemez bir alan doğuyor.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * 🔴 CHEVRON BİLEŞENİ NEDEN YOK
 * ══════════════════════════════════════════════════════════════════════════
 * Dört ekran `appearance-none` + mutlak konumlu bir `<svg>` çiziyor. Oysa
 * `globals.css:283` içindeki `.select-ok` sınıfı TAM BU TEKRARI ÖNLEMEK İÇİN
 * yazılmış: `appearance: none` + oku CSS `background-image` olarak çiziyor.
 * Ölçüm: 6 çağrı yerinden yalnız 3'ü onu kullanıyor. Doğru kapanış, beşinci
 * bir Chevron kopyası dışa vermek değil, `.select-ok`'u kullanmaktır — bu
 * yüzden burada Chevron YOKTUR.
 *
 * `appearance: none` ZORUNLUDUR: WebKit'te yerel `menulist` görünümü yüksekliği
 * kendi hesaplar ve `min-h-12`'yi YOK SAYAR — ölçümde kutu 25px çıkıyor, 44px
 * dokunma hedefinin çok altında. iOS'ta tüm tarayıcılar WebKit'tir ve bu hata
 * Chromium'da HİÇ GÖRÜNMEZ; göz kararıyla asla yakalanmaz (§7.3).
 *
 * Açılır/kapanır başlıklar (akordeon) için ayrı bir `AcilirOk` dışa verilir:
 * `denetim/page.tsx:352` bugün ham `▾` KARAKTERİ kullanıyor ve §9.2 unicode
 * karakteri ikon yerine kullanmayı yasaklıyor.
 */

import * as React from 'react';
import { cx } from '@/components/ui';

/* ═══════════════════════ Ortak etiket iskeleti ═══════════════════════ */

function AlanKabi({
  alanId,
  etiket,
  etiketiGizle,
  hata,
  ipucu,
  hataId,
  ipucuId,
  children,
}: {
  alanId: string;
  etiket: string;
  etiketiGizle?: boolean;
  hata?: string;
  ipucu?: string;
  hataId: string;
  ipucuId: string;
  children: React.ReactNode;
}) {
  return (
    <div className="flex flex-col gap-2">
      <label
        htmlFor={alanId}
        className={cx('text-sm font-medium', etiketiGizle && 'sr-only')}
      >
        {etiket}
      </label>

      {children}

      {/*
        İPUCU VE HATA 14px (`text-sm`), 12px DEĞİL.
        §3.2: veri taşıyan metin 14px altına inmez. Bir doğrulama hatası
        "ikincil dipnot" değildir — kullanıcının formu gönderebilmek için
        OKUMAK ZORUNDA olduğu metindir.
      */}
      {ipucu && !hata && (
        <p id={ipucuId} className="text-sm text-muted">
          {ipucu}
        </p>
      )}
      {hata && (
        // `role="alert"`: hata YENİ BELİREN bir içeriktir (§7.4). Statik
        // ipucu metni rol ALMAZ — açılışta okunup kullanıcıyı kesmemeli.
        <p id={hataId} role="alert" className="text-sm text-[var(--color-bad)]">
          {hata}
        </p>
      )}
    </div>
  );
}

/* ═══════════════════════ Secim (select) ═══════════════════════ */

type SecimProps = Omit<React.SelectHTMLAttributes<HTMLSelectElement>, 'children'> & {
  etiket: string;
  /** Süzgeç çubuğunda etiket görsel olarak gereksizse gizlenir — ama SİLİNMEZ. */
  etiketiGizle?: boolean;
  hata?: string;
  ipucu?: string;
  children: React.ReactNode;
};

export const Secim = React.forwardRef<HTMLSelectElement, SecimProps>(function Secim(
  { etiket, etiketiGizle, hata, ipucu, id, className, children, ...rest },
  ref,
) {
  const otoId = React.useId();
  const alanId = id ?? otoId;
  const hataId = `${alanId}-hata`;
  const ipucuId = `${alanId}-ipucu`;

  return (
    <AlanKabi
      alanId={alanId}
      etiket={etiket}
      etiketiGizle={etiketiGizle}
      hata={hata}
      ipucu={ipucu}
      hataId={hataId}
      ipucuId={ipucuId}
    >
      <select
        {...rest}
        ref={ref}
        id={alanId}
        aria-invalid={hata ? true : undefined}
        aria-describedby={cx(hata && hataId, ipucu && ipucuId) || undefined}
        className={cx(
          // `select-ok` (globals.css): appearance:none + CSS ile çizilen ok.
          // `min-h-12` = 48px ≥ 44px dokunma hedefi. `text-base` = 16px:
          // altına inilirse iOS sayfayı YAKINLAŞTIRIR (§7.3).
          'raised select-ok min-h-12 w-full rounded-xl border px-4 text-base',
          'outline-none focus:border-brand-400 disabled:cursor-not-allowed disabled:opacity-60',
          // Yalnız RENK geçişi, 120ms. Odak/hover'da konum hareketi yok (§4.3).
          '[transition-property:border-color] [transition-duration:var(--sure-hizli)]',
          hata && 'border-[var(--color-bad)]',
          className,
        )}
      >
        {children}
      </select>
    </AlanKabi>
  );
});

/* ═══════════════════════ CokSatir (textarea) ═══════════════════════ */

type CokSatirProps = React.TextareaHTMLAttributes<HTMLTextAreaElement> & {
  etiket: string;
  etiketiGizle?: boolean;
  hata?: string;
  ipucu?: string;
};

export const CokSatir = React.forwardRef<HTMLTextAreaElement, CokSatirProps>(function CokSatir(
  { etiket, etiketiGizle, hata, ipucu, id, className, rows = 3, ...rest },
  ref,
) {
  const otoId = React.useId();
  const alanId = id ?? otoId;
  const hataId = `${alanId}-hata`;
  const ipucuId = `${alanId}-ipucu`;

  return (
    <AlanKabi
      alanId={alanId}
      etiket={etiket}
      etiketiGizle={etiketiGizle}
      hata={hata}
      ipucu={ipucu}
      hataId={hataId}
      ipucuId={ipucuId}
    >
      <textarea
        {...rest}
        ref={ref}
        id={alanId}
        rows={rows}
        aria-invalid={hata ? true : undefined}
        aria-describedby={cx(hata && hataId, ipucu && ipucuId) || undefined}
        className={cx(
          // px-4/py-3: yarım boşluk basamağı YOK (§2.3 — özgün kopyalar
          // `px-3.5 py-2.5` yazıyordu, ikisi de ızgara dışı).
          'raised w-full rounded-xl border px-4 py-3 text-base',
          // 🔴 `caret-color`: §8 yüzey #2 — ölçüm, depoda 0 kullanım.
          // Tanımlanmazsa koyu temada imleç varsayılan SİYAH kalır ve koyu
          // zeminde GÖRÜNMEZ; kullanıcı nereye yazdığını göremez.
          '[caret-color:var(--text)]',
          'placeholder:text-[var(--muted)] outline-none focus:border-brand-400',
          'disabled:cursor-not-allowed disabled:opacity-60',
          // İç kaydırma alanı → ince kaydırma çubuğu (§8 yüzey #3).
          'thin-scroll',
          '[transition-property:border-color] [transition-duration:var(--sure-hizli)]',
          hata && 'border-[var(--color-bad)]',
          className,
        )}
      />
    </AlanKabi>
  );
});

/* ═══════════════════════ Açılır ok (akordeon) ═══════════════════════ */

/**
 * Açılır/kapanır başlıkların oku. `<select>` için DEĞİLDİR (orada `.select-ok`).
 *
 * `acik` iken 180° döner. Dönen tek şey `transform`'dur (§4.3 kapalı listesi)
 * ve süre `--sure-menu` (200ms, akordeon jetonu) ile eğri `--ease-out`
 * JETONLARDAN gelir — bileşende sabit yazılmaz.
 */
export function AcilirOk({ acik, className }: { acik: boolean; className?: string }) {
  return (
    <svg
      viewBox="0 0 24 24"
      className={cx(
        'size-4 shrink-0',
        '[transition-property:transform] [transition-duration:var(--sure-menu)]',
        '[transition-timing-function:var(--ease-out)]',
        acik && 'rotate-180',
        className,
      )}
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
    >
      <path d="M6 9l6 6 6-6" />
    </svg>
  );
}
