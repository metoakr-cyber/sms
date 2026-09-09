'use client';

import * as React from 'react';

/**
 * Hareket parçaları — belirme ve sayaç.
 *
 * 🔴 TEMEL KURAL: İÇERİK ÖNCE VAR, HAREKET SONRA EKLENİR.
 *
 * Her iki bileşen de sunucuda NİHAİ hâliyle render edilir: metin ve sayı
 * HTML'in içindedir. JavaScript devreye girdiğinde hareket "sonradan"
 * eklenir. Bunun tersi (CSS'te `opacity: 0`, JS ile açma) üç senaryoda
 * sayfayı BOŞ gösterir: arama motoru botu, JS hatası, hareket azaltma
 * tercihi. Bu proje trafiğini aramadan alacak — o risk alınamaz.
 *
 * Kaydırma kütüphanesi eklenmedi; IntersectionObserver zaten tarayıcıda var
 * (Safari 12.1+) ve bu kadarı için 30 KB'lık bir paket taşımak gereksiz.
 */

/** Sunucuda `useLayoutEffect` uyarı basar; orada `useEffect`e düşülür. */
const useIzomorfikLayoutEffect =
  typeof window !== 'undefined' ? React.useLayoutEffect : React.useEffect;

function hareketKapali(): boolean {
  return (
    typeof window === 'undefined' ||
    typeof window.matchMedia !== 'function' ||
    window.matchMedia('(prefers-reduced-motion: reduce)').matches
  );
}

/* ─────────────── Belirme ─────────────── */

type BelirProps = {
  children: React.ReactNode;
  className?: string;
  /** Sıralı belirme için ms cinsinden gecikme (kart ızgaralarında). */
  gecikme?: number;
  as?: 'div' | 'section' | 'li' | 'article' | 'header';
};

/**
 * Görünür alana girince yumuşakça belirir.
 *
 * Öğe İLK RENDER'DA ZATEN EKRANDAYSA hiç dokunulmaz — yoksa sayfa açılır
 * açılmaz kahraman bölüm bir kare kaybolup geri gelirdi.
 */
export function Belir({ children, className, gecikme = 0, as: Etiket = 'div' }: BelirProps) {
  const ref = React.useRef<HTMLElement | null>(null);
  const [durum, setDurum] = React.useState<'yok' | 'gizli' | 'acik'>('yok');

  useIzomorfikLayoutEffect(() => {
    const el = ref.current;
    if (!el || hareketKapali() || typeof IntersectionObserver === 'undefined') return;

    // Zaten ekrandaysa: animasyon yok. Boyama ÖNCESİ ölçülür (layout effect),
    // böylece kullanıcı hiçbir zaman kaybolan bir içerik görmez.
    const kutu = el.getBoundingClientRect();
    if (kutu.top < window.innerHeight * 0.92) return;

    setDurum('gizli');
    const gozlemci = new IntersectionObserver(
      (girisler) => {
        for (const g of girisler) {
          if (!g.isIntersecting) continue;
          setDurum('acik');
          gozlemci.disconnect();
        }
      },
      { rootMargin: '0px 0px -6% 0px', threshold: 0.05 },
    );
    gozlemci.observe(el);
    return () => gozlemci.disconnect();
  }, []);

  return (
    <Etiket
      // @ts-expect-error — dinamik etiket, ref tipi birleşimi daraltılamıyor
      ref={ref}
      className={className}
      data-belir={durum === 'yok' ? undefined : durum}
      style={durum === 'acik' && gecikme ? { transitionDelay: `${gecikme}ms` } : undefined}
    >
      {children}
    </Etiket>
  );
}

/* ─────────────── Sayaç ─────────────── */

type SayacProps = {
  /** Nihai değer. CANLI VERİDEN gelir — bileşene sabit sayı yazılmaz. */
  deger: number;
  /** Sayının sonuna eklenecek işaret (örn. "+"). */
  sonek?: string;
  sure?: number;
  className?: string;
};

/**
 * Sıfırdan gerçek değere sayar.
 *
 * Sunucu NİHAİ sayıyı basar (SEO ve JS'siz görünüm). İstemcide, boyamadan
 * önce sayaç 0'a çekilir ve yukarı sayar — böylece "önce 6 gördüm, sonra
 * 0'a düştü" gibi bir sıçrama olmaz.
 *
 * Hareket azaltma açıksa hiç oynanmaz: sayı olduğu gibi durur.
 */
export function Sayac({ deger, sonek = '', sure = 1100, className }: SayacProps) {
  const ref = React.useRef<HTMLSpanElement | null>(null);
  const [gosterilen, setGosterilen] = React.useState(deger);

  useIzomorfikLayoutEffect(() => {
    const el = ref.current;
    if (!el || hareketKapali() || deger <= 0) return;
    if (typeof IntersectionObserver === 'undefined') return;

    setGosterilen(0);

    let cerceve = 0;
    let baslangic = 0;
    const adim = (t: number) => {
      if (!baslangic) baslangic = t;
      const o = Math.min(1, (t - baslangic) / sure);
      // easeOutCubic: hızlı başlar, yumuşak biter
      const y = 1 - Math.pow(1 - o, 3);
      setGosterilen(Math.round(deger * y));
      if (o < 1) cerceve = requestAnimationFrame(adim);
    };

    const basla = () => { cerceve = requestAnimationFrame(adim); };

    const kutu = el.getBoundingClientRect();
    if (kutu.top < window.innerHeight) {
      basla();
      return () => cancelAnimationFrame(cerceve);
    }

    const gozlemci = new IntersectionObserver(
      (girisler) => {
        for (const g of girisler) {
          if (!g.isIntersecting) continue;
          gozlemci.disconnect();
          basla();
        }
      },
      { threshold: 0.3 },
    );
    gozlemci.observe(el);
    return () => { gozlemci.disconnect(); cancelAnimationFrame(cerceve); };
  }, [deger, sure]);

  return (
    <span ref={ref} className={className}>
      {/*
        Ekran okuyucu NİHAİ değeri bir kez okur. Sayarken her kareyi
        duyurmak (aria-live) kullanıcıyı otuz kez "1, 2, 3…" dinlemeye
        mahkûm ederdi; görsel sayaç bu yüzden `aria-hidden`.
      */}
      <span className="sr-only">{new Intl.NumberFormat('tr-TR').format(deger)}{sonek}</span>
      <span aria-hidden>
        {new Intl.NumberFormat('tr-TR').format(gosterilen)}{sonek}
      </span>
    </span>
  );
}

/* ─────────────── Akordeon ─────────────── */

/**
 * SSS akordeonu.
 *
 * `<details>/<summary>` KULLANILIR — JavaScript kapalıyken de açılır, arama
 * motoru içeriği görür ve klavye desteği tarayıcıdan gelir (Enter/Space).
 * ARIA ile elle kurulmuş bir akordeon aynı davranışı ancak taklit ederdi.
 * `aria-expanded`/`aria-controls` burada GEREKMEZ: yerel öğe kendi durumunu
 * erişilebilirlik ağacına zaten bildirir.
 */
export function Akordeon({
  sorular, className,
}: { sorular: Array<[string, string]>; className?: string }) {
  return (
    <div className={className}>
      {sorular.map(([soru, cevap]) => (
        <details
          key={soru}
          className="surface group mb-3 overflow-hidden rounded-2xl border golge-1
                     transition-colors last:mb-0 hover:border-[var(--vurgu)]/40"
        >
          <summary
            className="flex min-h-14 cursor-pointer list-none items-center justify-between
                       gap-4 px-4 py-3.5 text-[15px] font-semibold md:px-6"
          >
            {soru}
            {/* + / − geçişi: kapalıyken artı, açıkken eksi. Tek SVG, iki çizgi;
                dikey çizgi açılınca döner ve kaybolur. */}
            <span
              className="grid size-8 shrink-0 place-items-center rounded-lg
                         bg-[var(--v-mavi-zemin)] text-[var(--v-mavi-metin)]"
              aria-hidden
            >
              <svg viewBox="0 0 24 24" className="size-4" fill="none" stroke="currentColor"
                   strokeWidth="2.4" strokeLinecap="round">
                <path d="M5 12h14" />
                <path d="M12 5v14" className="origin-center transition-all duration-300
                                              group-open:rotate-90 group-open:opacity-0" />
              </svg>
            </span>
          </summary>
          <p className="border-t border-[var(--border)] px-4 py-4 text-sm leading-relaxed
                        text-muted md:px-6">
            {cevap}
          </p>
        </details>
      ))}
    </div>
  );
}
