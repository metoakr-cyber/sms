'use client';

import * as React from 'react';
import { cx } from './ui';

/**
 * Modal — mobilde alttan yükselen sayfa, masaüstünde ortada diyalog
 * (docs/frontend-contract.md §2.5).
 *
 * Radix yerine elle yazıldı çünkü ihtiyaç duyulan davranışlar sayılı ve her
 * biri burada AÇIKÇA görünüyor: odak tuzağı, Escape, arka plan kaydırma kilidi,
 * iOS'ta kaydırma konumunun geri yüklenmesi. Bir bağımlılık eklemek bu dört
 * satırı gizler, kaldırmaz.
 */
export function Modal({
  open, onClose, title, children,
}: { open: boolean; onClose: () => void; title: string; children: React.ReactNode }) {
  const panel = React.useRef<HTMLDivElement>(null);
  const titleId = React.useId();

  // Arka plan kaydırma kilidi.
  //
  // iOS'ta `overflow:hidden` TEK BAŞINA YETMEZ: sayfa yine kayar. body'yi
  // sabitlemek gerekir, ama sabitlemek kaydırma konumunu sıfırlar — bu yüzden
  // konum saklanır ve kapanışta geri yüklenir. Yapılmazsa kullanıcı modalı
  // kapattığında kendini sayfanın en başında bulur.
  React.useEffect(() => {
    if (!open) return;
    const y = window.scrollY;
    const { style } = document.body;
    const prev = { position: style.position, top: style.top, width: style.width };
    style.position = 'fixed';
    style.top = `-${y}px`;
    style.width = '100%';
    return () => {
      style.position = prev.position; style.top = prev.top; style.width = prev.width;
      window.scrollTo(0, y);
    };
  }, [open]);

  // Escape ile kapat + odağı modal İÇİNDE tut.
  //
  // Odak tuzağı olmadan Tab tuşu kullanıcıyı modalın arkasındaki sayfaya
  // götürür: ekran okuyucu kullanıcısı görünmeyen bir formda gezinir.
  React.useEffect(() => {
    if (!open) return;
    const prevFocus = document.activeElement as HTMLElement | null;
    // 🔴 `?.focus() ?? panel.focus()` YAZILAMAZ: focus() undefined döner,
    // dolayısıyla `??` sağ tarafı HER ZAMAN çalışır ve odağı panele geri
    // alır — `data-autofocus` projenin tamamında sessizce işlevsiz kalır.
    // Ölçümle bulundu (iki bağımsız denetimde), göz kararıyla görünmüyordu.
    //
    // test: modal_test.tsx yok — bu davranış tarayıcı gerektirir; kalıcı
    // doğrulama web/scripts/responsive-check.mjs odak kontrolüne eklenmeli.
    const ilk = panel.current?.querySelector<HTMLElement>('[data-autofocus]');
    (ilk ?? panel.current)?.focus();

    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') { onClose(); return; }
      if (e.key !== 'Tab' || !panel.current) return;
      const items = panel.current.querySelectorAll<HTMLElement>(
        'a[href],button:not([disabled]),input:not([disabled]),select:not([disabled]),textarea,[tabindex]:not([tabindex="-1"])',
      );
      if (items.length === 0) return;
      const first = items[0]!, last = items[items.length - 1]!;
      const aktif = document.activeElement;

      // 🔴 ODAK PANELİN DIŞINDAYSA GERİ ÇEKİLİR.
      //
      // Eski kod yalnız `aktif === first` / `aktif === last` durumlarını
      // yakalıyordu. Odak panelin KENDİSİNDEYSE (ilk açılışta öyle olur) ya da
      // bir adım değişiminde tıklanan düğme DOM'dan kalktığı için <body>'ye
      // düştüyse, hiçbir dal çalışmıyor ve Tab kullanıcıyı diyaloğun
      // ARKASINDAKİ sayfaya taşıyordu. Para onayı gösteren bir diyalogda
      // klavye kullanıcısı böylece arkadaki formu doldurmaya başlıyordu.
      if (!aktif || !panel.current.contains(aktif)) {
        e.preventDefault();
        (e.shiftKey ? last : first).focus();
        return;
      }
      if (e.shiftKey && aktif === first) { e.preventDefault(); last.focus(); }
      else if (!e.shiftKey && aktif === last) { e.preventDefault(); first.focus(); }
    };
    window.addEventListener('keydown', onKey);
    return () => { window.removeEventListener('keydown', onKey); prevFocus?.focus?.(); };
  }, [open, onClose]);

  if (!open) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-end justify-center sm:items-center">
      {/* Arka plan. Tıklanınca kapanır ama ekran okuyucuya sunulmaz. */}
      <button
        type="button" aria-hidden tabIndex={-1} onClick={onClose}
        className="absolute inset-0 bg-black/60 backdrop-blur-[2px]"
      />
      <div
        ref={panel} role="dialog" aria-modal="true" aria-labelledby={titleId} tabIndex={-1}
        className={cx(
          'surface relative z-10 flex max-h-[92dvh] w-full flex-col overflow-hidden border',
          // ASGARİ YÜKSEKLİK: içerik azken (henüz seçim yapılmamışken) sayfa
          // altında ince bir şerit gibi kalıyordu — yarım açılmış, bozuk bir
          // görüntü. Alttan yükselen bir sayfa, "sayfa" gibi durmalı.
          'min-h-[52dvh] sm:min-h-0',
          // mobil: alttan yükselen sayfa, tam genişlik, üstü yuvarlak
          'rounded-t-2xl',
          // masaüstü: ortada diyalog
          'sm:max-w-md sm:rounded-2xl',
        )}
      >
        <div className="flex items-center justify-between gap-3 border-b border-[var(--border)] px-5 py-4">
          <h2 id={titleId} className="min-w-0 truncate text-lg font-bold">{title}</h2>
          <button
            type="button" onClick={onClose} aria-label="Kapat"
            className="-mr-2 grid size-11 shrink-0 place-items-center rounded-xl
                       text-muted hover:bg-[var(--raised)] hover:text-[var(--text)]"
          >
            <svg viewBox="0 0 24 24" className="size-5" fill="none" stroke="currentColor"
                 strokeWidth="2" strokeLinecap="round" aria-hidden>
              <path d="M6 6l12 12M18 6L6 18" />
            </svg>
          </button>
        </div>

        <div className="thin-scroll overflow-y-auto px-5 py-5 pb-safe">{children}</div>
      </div>
    </div>
  );
}
