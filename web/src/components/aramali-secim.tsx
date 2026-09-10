'use client';

/**
 * AramaliSecim — arama kutulu seçim alanı ("select2" davranışı).
 *
 * ══════════════════════════════════════════════════════════════════════════
 * NEDEN VAR
 * ══════════════════════════════════════════════════════════════════════════
 * Ülke seçimi 201 maddelik bir yerel `<select>`ti. Yerel seçicide arama yoktur;
 * kullanıcı ilk harfe atlayabilir ama "Birleşik Krallık"ı bulmak için listede
 * elle gezmek zorundadır. Mobilde bu, ekranı doldurmuş bir tekerlek demektir.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * 🔴 LİSTE YERİNDE AÇILIR — MUTLAK KONUMLU AÇILIR PENCERE DEĞİL
 * ══════════════════════════════════════════════════════════════════════════
 * Bu alan bir `Modal` içinde duruyor ve modal kabı `overflow-hidden`, gövdesi
 * `overflow-y-auto` (`modal.tsx`). Mutlak konumlu bir açılır liste kabın
 * dışına taşamaz: uzun listede alt kısmı KIRPILIR. Portal açmak da modalın
 * odak tuzağını delerdi.
 *
 * Yerinde açılan liste bu iki sorunu da doğurmaz ve mobilde daha iyidir:
 * arama kutusu klavyenin hemen üstünde kalır, sonuçlar modalın kendi
 * kaydırmasıyla gezilir. Etkileşim select2 ile aynı — tıkla, yaz, seç.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * ERİŞİLEBİLİRLİK — yerel `<select>`ten VAZGEÇİLDİĞİ İÇİN ELLE KURULUR
 * ══════════════════════════════════════════════════════════════════════════
 * Yerel seçici klavye gezintisini, ekran okuyucu duyurusunu ve odak yönetimini
 * bedava veriyordu. Onu bırakan her bileşen üçünü de yeniden kurmak
 * ZORUNDADIR, yoksa klavye kullanıcısı alanı hiç kullanamaz:
 *   · `role="combobox"` + `aria-expanded` + `aria-controls`
 *   · `role="listbox"` / `role="option"` + `aria-selected`
 *   · `aria-activedescendant` — odak arama kutusunda kalır, vurgu listede gezer
 *   · ↑/↓ gezinir · Enter seçer · Esc kapatır ve odağı tetikleyiciye döndürür
 *
 * HAREKET YOK (§4.2): liste açılıp kapanırken geçiş yoktur. Tek geçiş satır
 * vurgusunun RENGİdir ve `--sure-hizli` ile sınırlıdır.
 */

import * as React from 'react';
import { aramaAnahtari } from '@/lib/arama';
import { cx } from './ui';

export interface SecimSecenegi {
  deger: string;
  etiket: string;
  /** Etiketin yanında görünen ikincil bilgi (ör. "+90"). */
  ipucu?: string;
  devreDisi?: boolean;
  /** Devre dışıysa sebebi — satırda görünür, renkle değil METİNLE anlatılır (§7.1). */
  devreDisiNotu?: string;
  /** Etiket dışında aranacak anahtarlar (ör. ISO kodu, telefon kodu). */
  ara?: readonly string[];
}

export function AramaliSecim({
  etiket,
  deger,
  onDegis,
  secenekler,
  yerTutucu = 'Seçiniz…',
  araYerTutucu = 'Aramak için yazın',
  yukleniyor,
  hata,
  bosMetni = 'Aramanıza uyan sonuç yok.',
  otoOdak,
  id,
}: {
  etiket: string;
  deger: string;
  onDegis: (deger: string) => void;
  secenekler: readonly SecimSecenegi[];
  yerTutucu?: string;
  araYerTutucu?: string;
  yukleniyor?: boolean;
  hata?: string;
  bosMetni?: string;
  /**
   * `Modal` açıldığında odağı bu alana verir (`modal.tsx` `[data-autofocus]`
   * arar). Yerel `<select>`ten geçerken KAYBOLMASI kolay bir davranış: odak
   * verilmezse modal panelin kendisine düşer ve klavye kullanıcısı forma
   * ulaşmak için baştan Tab'lar.
   */
  otoOdak?: boolean;
  id?: string;
}) {
  const otoId = React.useId();
  const alanId = id ?? otoId;
  const listeId = `${alanId}-liste`;
  const etiketId = `${alanId}-etiket`;

  const [acik, setAcik] = React.useState(false);
  const [arama, setArama] = React.useState('');
  const [vurgulu, setVurgulu] = React.useState(0);

  const kabRef = React.useRef<HTMLDivElement>(null);
  const tetikRef = React.useRef<HTMLButtonElement>(null);
  const aramaRef = React.useRef<HTMLInputElement>(null);

  const secili = secenekler.find((s) => s.deger === deger);

  const suzulmus = React.useMemo(() => {
    const q = aramaAnahtari(arama.trim());
    if (!q) return secenekler;
    const rakam = q.replace(/\D/g, '');
    return secenekler.filter(
      (s) =>
        aramaAnahtari(s.etiket).includes(q) ||
        (s.ara ?? []).some((a) => {
          const anahtar = aramaAnahtari(a);
          // Rakam yazıldıysa telefon kodu gibi sayısal anahtarlarda da ara.
          return anahtar.includes(q) || (rakam !== '' && anahtar.replace(/\D/g, '').includes(rakam));
        }),
    );
  }, [secenekler, arama]);

  // Süzgeç daraldığında vurgu listenin dışında kalabilir; başa çekilir.
  React.useEffect(() => {
    setVurgulu(0);
  }, [arama]);

  // Açılınca odak arama kutusuna gider — select2'nin tek önemli davranışı budur.
  React.useEffect(() => {
    if (acik) aramaRef.current?.focus();
  }, [acik]);

  // Dışarı tıklama kapatır. `mousedown` kullanılır, `click` DEĞİL: `click`
  // seçim satırının kendi `onClick`inden SONRA gelir ve seçimi yutardı.
  React.useEffect(() => {
    if (!acik) return;
    const kapat = (e: MouseEvent) => {
      if (!kabRef.current?.contains(e.target as Node)) setAcik(false);
    };
    document.addEventListener('mousedown', kapat);
    return () => document.removeEventListener('mousedown', kapat);
  }, [acik]);

  function kapatVeOdakla() {
    setAcik(false);
    setArama('');
    tetikRef.current?.focus();
  }

  function sec(s: SecimSecenegi) {
    if (s.devreDisi) return;
    onDegis(s.deger);
    setAcik(false);
    setArama('');
    tetikRef.current?.focus();
  }

  function vurguyuGoster(i: number) {
    const oge = document.getElementById(`${listeId}-${i}`);
    // `nearest`: liste kabı içinde en az hareketle görünür kılar; `center`
    // her ok tuşunda listeyi zıplatırdı.
    oge?.scrollIntoView({ block: 'nearest' });
  }

  function listeKlavyesi(e: React.KeyboardEvent) {
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault();
      if (suzulmus.length === 0) return;
      const yon = e.key === 'ArrowDown' ? 1 : -1;
      const yeni = (vurgulu + yon + suzulmus.length) % suzulmus.length;
      setVurgulu(yeni);
      vurguyuGoster(yeni);
      return;
    }
    if (e.key === 'Enter') {
      e.preventDefault();
      const s = suzulmus[vurgulu];
      if (s) sec(s);
      return;
    }
    if (e.key === 'Escape') {
      e.preventDefault();
      kapatVeOdakla();
    }
  }

  const devreDisiKutu = Boolean(yukleniyor || hata);

  return (
    <div ref={kabRef} className="flex flex-col gap-1.5">
      <span id={etiketId} className="text-sm font-medium">
        {etiket}
      </span>

      <button
        ref={tetikRef}
        type="button"
        id={alanId}
        role="combobox"
        aria-expanded={acik}
        aria-controls={listeId}
        aria-haspopup="listbox"
        aria-labelledby={`${etiketId} ${alanId}`}
        data-autofocus={otoOdak ? '' : undefined}
        disabled={devreDisiKutu}
        onClick={() => setAcik((v) => !v)}
        onKeyDown={(e) => {
          if (e.key === 'ArrowDown' && !acik) {
            e.preventDefault();
            setAcik(true);
          }
        }}
        className={cx(
          // `min-h-12` = 48px ≥ 44px dokunma hedefi. `text-base` = 16px:
          // altına inilirse iOS sayfayı YAKINLAŞTIRIR (§7.3).
          'raised flex min-h-12 w-full items-center gap-2 rounded-xl border px-3 text-left text-base',
          'outline-none focus:border-brand-400 disabled:cursor-not-allowed disabled:opacity-60',
          '[transition-property:border-color] [transition-duration:var(--sure-hizli)]',
        )}
      >
        <span className={cx('min-w-0 flex-1 truncate', !secili && 'text-muted')}>
          {yukleniyor
            ? 'Yükleniyor…'
            : hata
              ? hata
              : secili
                ? `${secili.etiket}${secili.ipucu ? ` (${secili.ipucu})` : ''}`
                : yerTutucu}
        </span>
        {/* Emoji/unicode DEĞİL, SVG (§9.2). */}
        <svg
          className="size-4 shrink-0 text-[var(--muted)]"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
          aria-hidden
        >
          <path d="m6 9 6 6 6-6" />
        </svg>
      </button>

      {acik && (
        <div className="raised overflow-hidden rounded-xl border">
          <div className="border-b border-[var(--border)] p-2">
            <input
              ref={aramaRef}
              type="search"
              value={arama}
              onChange={(e) => setArama(e.target.value)}
              onKeyDown={listeKlavyesi}
              placeholder={araYerTutucu}
              aria-label={`${etiket} — ara`}
              aria-controls={listeId}
              aria-activedescendant={suzulmus.length ? `${listeId}-${vurgulu}` : undefined}
              autoCapitalize="none"
              autoCorrect="off"
              autoComplete="off"
              spellCheck={false}
              className="min-h-11 w-full rounded-lg bg-transparent px-2 text-base outline-none"
            />
          </div>

          {/*
            `max-h-64` + kendi kaydırması: liste modalın gövdesini şişirip
            arama kutusunu ekrandan çıkarmasın. `thin-scroll` (§8).
          */}
          <ul
            id={listeId}
            role="listbox"
            aria-labelledby={etiketId}
            className="thin-scroll max-h-64 overflow-y-auto py-1"
          >
            {suzulmus.length === 0 ? (
              <li className="px-3 py-6 text-center text-sm text-muted">{bosMetni}</li>
            ) : (
              suzulmus.map((s, i) => {
                const seciliMi = s.deger === deger;
                return (
                  <li key={s.deger}>
                    <button
                      type="button"
                      id={`${listeId}-${i}`}
                      role="option"
                      aria-selected={seciliMi}
                      disabled={s.devreDisi}
                      // `onMouseEnter`: fare ile gezerken vurgu imleci izler,
                      // böylece Enter'ın neyi seçeceği her an görünür.
                      onMouseEnter={() => setVurgulu(i)}
                      onClick={() => sec(s)}
                      className={cx(
                        'flex min-h-11 w-full items-center gap-2 px-3 text-left text-sm',
                        '[transition-property:background-color] [transition-duration:var(--sure-hizli)]',
                        s.devreDisi
                          ? 'cursor-not-allowed text-muted opacity-60'
                          : i === vurgulu
                            ? 'bg-[var(--bg)]'
                            : '',
                        seciliMi && 'font-semibold',
                      )}
                    >
                      <span className="min-w-0 flex-1 truncate">
                        {s.etiket}
                        {s.ipucu && <span className="ms-1.5 text-muted">{s.ipucu}</span>}
                      </span>
                      {/* Devre dışı olma sebebi METİNLE söylenir; solgunluk
                          tek başına anlam taşımaz (§7.1). */}
                      {s.devreDisi && s.devreDisiNotu && (
                        <span className="shrink-0 text-xs text-muted">{s.devreDisiNotu}</span>
                      )}
                    </button>
                  </li>
                );
              })
            )}
          </ul>

          {/*
            Sonuç sayısı canlı bölgede: süzgeç daraldıkça ekran okuyucu kaç
            sonuç kaldığını duyar. Bölge DAİMA DOM'da durur — koşullu render
            edilirse duyurulmaz.
          */}
          <p aria-live="polite" aria-atomic="true" className="sr-only">
            {arama.trim() ? `${suzulmus.length} sonuç` : ''}
          </p>
        </div>
      )}
    </div>
  );
}
