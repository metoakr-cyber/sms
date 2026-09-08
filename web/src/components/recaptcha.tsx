'use client';

import * as React from 'react';

/**
 * Google reCAPTCHA v2 (onay kutusu).
 *
 * Sunucu tarafı yalnız `success` alanına bakar (adapter/captcha/recaptcha.go),
 * bu yüzden v2 onay kutusu doğru eşleşmedir.
 *
 * SİTE ANAHTARI YOKSA HİÇBİR ŞEY ÇİZİLMEZ ve `onChange('')` çağrılır. Sunucu
 * da o durumda captcha.Disabled kullanır. Böylece geliştirme ortamı anahtarsız
 * çalışır; üretimde config paketi anahtarı zorunlu kılar ve süreç anahtarsız
 * başlamaz — yani "geliştirmede kapalı" durumu üretime SIZAMAZ.
 */

declare global {
  interface Window {
    grecaptcha?: {
      render: (el: HTMLElement, o: Record<string, unknown>) => number;
      reset: (id?: number) => void;
    };
    onRecaptchaLoad?: () => void;
  }
}

export const RECAPTCHA_SITE_KEY = process.env.NEXT_PUBLIC_RECAPTCHA_SITE_KEY ?? '';
export const captchaEnabled = RECAPTCHA_SITE_KEY !== '';

let scriptPromise: Promise<void> | null = null;

/** Betiği SAYFA BAŞINA BİR KEZ yükler; iki form varsa ikinci yükleme çakışır. */
function loadScript(): Promise<void> {
  if (typeof window === 'undefined') return Promise.resolve();
  if (window.grecaptcha?.render) return Promise.resolve();
  if (scriptPromise) return scriptPromise;

  scriptPromise = new Promise<void>((resolve, reject) => {
    window.onRecaptchaLoad = () => resolve();
    const s = document.createElement('script');
    s.src = 'https://www.google.com/recaptcha/api.js?onload=onRecaptchaLoad&render=explicit&hl=tr';
    s.async = true;
    s.defer = true;
    s.onerror = () => reject(new Error('recaptcha yüklenemedi'));
    document.head.appendChild(s);
  });
  return scriptPromise;
}

export interface CaptchaHandle { reset: () => void }

export const Recaptcha = React.forwardRef<CaptchaHandle, {
  onChange: (token: string) => void;
  theme?: 'dark' | 'light';
}>(function Recaptcha({ onChange, theme = 'dark' }, ref) {
  const box = React.useRef<HTMLDivElement>(null);
  const widgetId = React.useRef<number | null>(null);
  const [failed, setFailed] = React.useState(false);

  // onChange'i ref'te tutarız: bağımlılığa koyarsak her render'da widget
  // yeniden çizilir ve kullanıcının işaretlediği kutu sıfırlanır.
  const cb = React.useRef(onChange);
  React.useEffect(() => { cb.current = onChange; }, [onChange]);

  React.useImperativeHandle(ref, () => ({
    reset: () => {
      if (widgetId.current !== null) window.grecaptcha?.reset(widgetId.current);
      cb.current('');
    },
  }), []);

  React.useEffect(() => {
    if (!captchaEnabled) return;
    let cancelled = false;

    loadScript()
      .then(() => {
        if (cancelled || !box.current || widgetId.current !== null) return;
        widgetId.current = window.grecaptcha!.render(box.current, {
          sitekey: RECAPTCHA_SITE_KEY,
          theme,
          callback: (t: string) => cb.current(t),
          // Token ~2 dakikada geçersizleşir. Süre dolduğunda elimizdeki
          // token'ı TEMİZLERİZ; yoksa kullanıcı gönderime basar ve sunucudan
          // anlamsız bir "robot doğrulaması başarısız" alır.
          'expired-callback': () => cb.current(''),
          'error-callback': () => { cb.current(''); setFailed(true); },
        });
      })
      .catch(() => { if (!cancelled) setFailed(true); });

    return () => { cancelled = true; };
  }, [theme]);

  if (!captchaEnabled) return null;

  return (
    <div className="flex flex-col gap-2">
      {/* reCAPTCHA 304px sabit genişliktedir; 320px ekranda taşmaması için
          küçültülür. transform kullanılır çünkü iframe içeriği ölçeklenemez. */}
      <div className="origin-top-left scale-[0.87] sm:scale-100" style={{ height: 78 }}>
        <div ref={box} />
      </div>
      {failed && (
        <p className="text-xs text-[var(--color-warn)]">
          Robot doğrulaması yüklenemedi. Reklam engelleyicinizi kapatıp sayfayı yenileyin.
        </p>
      )}
    </div>
  );
});
