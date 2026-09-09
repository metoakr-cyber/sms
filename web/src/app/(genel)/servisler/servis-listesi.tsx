'use client';

/**
 * Tüm servislerin aranabilir, SAYFALANMIŞ listesi (/servisler gövdesi).
 *
 * ── NEDEN SAYFALAMA VAR ──
 *
 * Ölçüm (katalog 810 görünür servis, geliştirme veritabanı):
 *
 *     kart sayısı      810
 *     DOM düğümü     5.158   (kart başına ~10 düğüm)
 *     SSR HTML       789 KB
 *
 * Bu, Lighthouse'un "aşırı DOM boyutu" eşiğinin (1.400 düğüm uyarı,
 * 3.000 düğüm hata) üç katıdır. Orta seviye bir telefonda düzen (layout) ve
 * stil hesabı hissedilir şekilde uzuyor, 4G'de 789 KB HTML kabul edilemez
 * (docs/frontend-contract.md §8).
 *
 * Çözüm: ilk `ADIM` kadar kart basılır, gerisi "Daha fazla göster" ile
 * eklenir. ARAMA HER ZAMAN LİSTENİN TAMAMINDA çalışır — sayfalama yalnız
 * ÇİZİLEN kart sayısını sınırlar, veriyi değil. Kullanıcı 700. servisi
 * aradığında düğmeye basmak zorunda kalmaz, yazması yeter.
 *
 * ── NEDEN İSTEMCİ BİLEŞENİ ──
 *
 * Arama kutusu ve "daha fazla" için. Liste SUNUCUDA çekilir ve bu bileşene
 * hazır gelir; ilk `ADIM` kart ilk HTML'dedir, arama motoru en popüler
 * servisleri JavaScript çalıştırmadan görür (docs/frontend-contract.md §10.2).
 * Arama sunucuya gitseydi her tuş vuruşu bir istek olurdu.
 *
 * ── SIRALAMA ──
 *
 * Sıra SUNUCUDAN gelir (`services.sort_order`, bkz. web/scripts/servis-siralama.sql).
 * Burada yeniden sıralanmaz.
 *
 * MOBİL ÖNCE: taban stiller telefon, büyük ekran `sm:`/`lg:` ile eklenir.
 * Tablo YOKTUR — ızgara dar ekranda iki sütuna düşer, yatay kaydırma oluşmaz.
 */

import * as React from 'react';
import { ServiceIcon } from '@/components/service-icon';
import { Badge, Button, Empty, cx } from '@/components/ui';

/** Sunucudan gelen birleşik satır. Karşılığı: dto.ServiceResponse + ServiceSummaryResponse. */
export interface ServisSatiri {
  code: string;
  name: string;
  iconUrl?: string;
  /** Stoklu ülke sayısı. Stoksuz serviste 0'dır. */
  countryCount: number;
  /** En az bir ülkede stok var mı. */
  inStock: boolean;
}

const RENKLER = ['mavi', 'mor', 'deniz', 'yesil', 'turuncu', 'pembe'] as const;

/**
 * Tek seferde çizilecek kart sayısı — hem ilk yük hem "daha fazla" adımı.
 *
 * 60 seçildi çünkü 2, 3, 4 ve 5 sütunun HEPSİNE tam bölünür: hiçbir kırılma
 * noktasında yarım kalan satır oluşmaz. 60 kart ≈ 600 DOM düğümü.
 */
const ADIM = 60;

/**
 * Türkçe duyarsız arama anahtarı.
 *
 * `toLowerCase()` TEK BAŞINA YETMEZ: "İ" harfi İngilizce kurallarla "i̇"
 * (birleşik nokta) olur ve "instagram" araması "İnstagram"ı bulamaz. Türkçe
 * yerel ayarı + açık harf eşlemesi ikisini de kapatır.
 */
function anahtar(s: string): string {
  return s
    .toLocaleLowerCase('tr')
    .replace(/ı/g, 'i')
    .replace(/İ/g, 'i')
    .replace(/ş/g, 's')
    .replace(/ğ/g, 'g')
    .replace(/ü/g, 'u')
    .replace(/ö/g, 'o')
    .replace(/ç/g, 'c');
}

export function ServisListesi({ servisler }: { servisler: ServisSatiri[] }) {
  const [sorgu, setSorgu] = React.useState('');
  const [yalnizStoklu, setYalnizStoklu] = React.useState(false);
  const [limit, setLimit] = React.useState(ADIM);

  // Arama anahtarları BİR KEZ hesaplanır: her tuş vuruşunda 810 satır için
  // yeniden normalize etmek dar cihazlarda hissedilir.
  const indeks = React.useMemo(
    () => servisler.map((s) => ({ s, k: anahtar(`${s.name} ${s.code}`) })),
    [servisler],
  );

  const suzulmus = React.useMemo(() => {
    const q = anahtar(sorgu.trim());
    return indeks
      .filter(({ s, k }) => (!q || k.includes(q)) && (!yalnizStoklu || s.inStock))
      .map(({ s }) => s);
  }, [indeks, sorgu, yalnizStoklu]);

  const stokluSayisi = React.useMemo(
    () => servisler.filter((s) => s.inStock).length,
    [servisler],
  );

  // Süzgeç DEĞİŞİNCE limit başa döner. Aksi halde kullanıcı "daha fazla"ya üç
  // kez basıp sonra arama yaptığında, 3 sonuçlu bir aramanın altında hâlâ
  // "240 gösteriliyor" yazardı. Effect DEĞİL, olay işleyicisinde: effect
  // fazladan bir render turu ve bir kare titreme demek olurdu.
  const araAyarla = (yeni: string) => { setSorgu(yeni); setLimit(ADIM); };
  const stokAyarla = () => { setYalnizStoklu((v) => !v); setLimit(ADIM); };

  const gorunen = suzulmus.slice(0, limit);
  const kalan = suzulmus.length - gorunen.length;
  const suzuluyor = sorgu.trim() !== '' || yalnizStoklu;

  return (
    <div className="flex flex-col gap-5">
      {/* ── Arama ve süzgeç ── */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
        <label className="relative flex-1">
          <span className="sr-only">Servis ara</span>
          {/* 🔴 FONT 16px'TEN KÜÇÜK OLAMAZ (text-base): iOS Safari daha küçük
              bir girdiye odaklandığında sayfayı yakınlaştırır ve kullanıcı
              geri uzaklaştıramaz (frontend-contract.md §2.4). */}
          <input
            type="search"
            inputMode="search"
            value={sorgu}
            onChange={(e) => araAyarla(e.target.value)}
            placeholder="Servis ara — WhatsApp, Telegram, Steam…"
            className="raised min-h-12 w-full rounded-xl border px-4 text-base outline-none
                       focus:border-brand-400"
            aria-describedby="servis-sayaci"
          />
        </label>

        {/* Dokunma hedefi en az 44 px (min-h-12 = 48 px). */}
        <button
          type="button"
          onClick={stokAyarla}
          aria-pressed={yalnizStoklu}
          className={cx(
            'min-h-12 shrink-0 rounded-xl border px-4 text-sm font-medium transition-colors',
            'active:scale-[0.99]',
            yalnizStoklu
              ? 'border-brand-500/40 bg-brand-500/12 text-brand-300'
              : 'raised text-muted',
          )}
        >
          Yalnız stokta olanlar
        </button>
      </div>

      <p id="servis-sayaci" className="text-sm text-muted" aria-live="polite">
        {suzuluyor ? (
          <>
            <strong className="font-semibold text-[var(--text)]">{suzulmus.length}</strong> servis
            bulundu{kalan > 0 && <>, ilk {gorunen.length} tanesi gösteriliyor</>}.
          </>
        ) : (
          <>
            Toplam <strong className="font-semibold text-[var(--text)]">{servisler.length}</strong>{' '}
            servis, <strong className="font-semibold text-[var(--text)]">{stokluSayisi}</strong>{' '}
            tanesinde şu anda stok var
            {kalan > 0 && <> — ilk {gorunen.length} tanesi gösteriliyor</>}.
          </>
        )}
      </p>

      {suzulmus.length === 0 ? (
        <Empty
          title="Aramanıza uyan servis yok"
          hint="Farklı bir yazım deneyin ya da süzgeci kaldırın. Aradığınız servisi bulamazsanız destek talebi açabilirsiniz."
        />
      ) : (
        <>
          <ul className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5">
            {gorunen.map((s, i) => (
              <li key={s.code}>
                <article
                  className={cx(
                    'surface kart-hover flex h-full flex-col items-center gap-2.5 rounded-2xl',
                    'border p-4 text-center golge-1',
                    // Stoksuz servis SİLİKTİR ama gizlenmez: kullanıcı aradığı
                    // servisin var olduğunu ama şu an stok olmadığını görmeli.
                    // Listeden tümüyle çıkarmak "bu site WhatsApp satmıyor"
                    // dedirtir (queries/catalog.sql'deki TR istisnasıyla aynı
                    // gerekçe).
                    !s.inStock && 'opacity-70',
                  )}
                >
                  <ServiceIcon
                    name={s.name}
                    iconUrl={s.iconUrl}
                    size={44}
                    renk={RENKLER[i % RENKLER.length]}
                  />
                  <h3 className="break-anywhere text-sm font-semibold leading-snug">{s.name}</h3>
                  {s.inStock ? (
                    <Badge tone="ok">{s.countryCount} ülkede stokta</Badge>
                  ) : (
                    <Badge tone="neutral">Şu an stok yok</Badge>
                  )}
                </article>
              </li>
            ))}
          </ul>

          {kalan > 0 && (
            // Mobilde tam genişlik: tek elle ulaşılabilir bir hedef.
            // Sonsuz kaydırma DEĞİL — kullanıcı sayfanın sonuna (alt bilgi,
            // "kiralama" bağlantısı) ulaşamaz hâle gelirdi.
            <Button
              variant="outline"
              fullWidth
              className="sm:w-auto sm:self-center"
              onClick={() => setLimit((n) => n + ADIM)}
            >
              Daha fazla göster ({kalan} servis daha)
            </Button>
          )}
        </>
      )}
    </div>
  );
}
