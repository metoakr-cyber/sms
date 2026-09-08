'use client';

import * as React from 'react';

type Theme = 'dark' | 'light' | 'system';
const KEY = 'tema';

/**
 * Sayfa boyanmadan ÖNCE çalışan betik.
 *
 * React devreye girene kadar geçen sürede sayfa varsayılan temayla boyanır ve
 * kullanıcı seçtiği tema koyu iken bir kare BEYAZ patlama görür ("theme flash").
 * Bunu React ile çözmek mümkün değildir; kök öznitelik ilk boyamadan önce
 * yazılmalıdır. `dangerouslySetInnerHTML` burada BİLİNÇLİ bir tercihtir:
 * içerik sabittir, kullanıcı girdisi içermez.
 */
export const themeScript = `(function(){try{
  var t = localStorage.getItem('${KEY}');
  if (t === 'dark' || t === 'light') document.documentElement.setAttribute('data-theme', t);
}catch(e){}})();`;

export function useTheme() {
  const [theme, setThemeState] = React.useState<Theme>('system');

  // localStorage YALNIZ istemcide okunur. Sunucuda okumaya çalışmak hidrasyon
  // uyuşmazlığı üretir (sunucu 'system' der, istemci 'dark' der).
  React.useEffect(() => {
    try {
      const t = localStorage.getItem(KEY);
      if (t === 'dark' || t === 'light') setThemeState(t);
    } catch { /* gizli sekmede localStorage erişimi hata atabilir */ }
  }, []);

  const setTheme = React.useCallback((t: Theme) => {
    setThemeState(t);
    const el = document.documentElement;
    if (t === 'system') {
      el.removeAttribute('data-theme');
      try { localStorage.removeItem(KEY); } catch { /* yok sayılır */ }
    } else {
      el.setAttribute('data-theme', t);
      try { localStorage.setItem(KEY, t); } catch { /* yok sayılır */ }
    }
  }, []);

  return { theme, setTheme };
}

export function ThemeToggle({ className }: { className?: string }) {
  const { theme, setTheme } = useTheme();
  const [mounted, setMounted] = React.useState(false);
  React.useEffect(() => setMounted(true), []);

  // Sunucuda hangi temanın etkin olduğunu BİLEMEYİZ (localStorage istemcide).
  // Bağlanmadan önce ikon çizmek hidrasyon uyuşmazlığı üretir; yer tutarız.
  const next: Theme = theme === 'dark' ? 'light' : 'dark';

  return (
    <button
      type="button"
      onClick={() => setTheme(next)}
      aria-label={next === 'dark' ? 'Koyu temaya geç' : 'Açık temaya geç'}
      title={next === 'dark' ? 'Koyu tema' : 'Açık tema'}
      // shrink-0 ZORUNLU: esnek bir kapsayıcıda size-11 bir ÖNERİDİR, garanti
      // değildir. Kenar çubuğunda yanındaki tam genişlik butonu bu düğmeyi
      // 44px'ten 35px'e sıkıştırıyordu — ölçüm yapılmasa fark edilmezdi.
      className={`grid size-11 shrink-0 place-items-center rounded-xl text-muted
                  transition-colors hover:bg-[var(--raised)] hover:text-[var(--text)] ${className ?? ''}`}
    >
      {!mounted ? (
        <span className="size-5" />
      ) : theme === 'dark' ? (
        <svg viewBox="0 0 24 24" className="size-5" fill="none" stroke="currentColor"
             strokeWidth="1.8" strokeLinecap="round" aria-hidden>
          <circle cx="12" cy="12" r="4" />
          <path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4" />
        </svg>
      ) : (
        <svg viewBox="0 0 24 24" className="size-5" fill="none" stroke="currentColor"
             strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
          <path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8" />
        </svg>
      )}
    </button>
  );
}
