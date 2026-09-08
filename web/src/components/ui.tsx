/** Ortak arayüz parçaları. Mobil-öncedir; `md:` ile büyük ekran eklenir. */
import * as React from 'react';

export function cx(...parts: Array<string | false | null | undefined>): string {
  return parts.filter(Boolean).join(' ');
}

/* ─────────────── Buton ─────────────── */

type ButtonProps = React.ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: 'primary' | 'ghost' | 'danger' | 'outline';
  size?: 'md' | 'sm';
  loading?: boolean;
  fullWidth?: boolean;
};

export function Button({
  variant = 'primary', size = 'md', loading, fullWidth,
  className, children, disabled, ...rest
}: ButtonProps) {
  const base =
    // min-h-11 = 44px: Apple HIG dokunma hedefi alt sınırı (§2.3).
    'inline-flex items-center justify-center gap-2 rounded-xl font-medium ' +
    'transition-colors active:scale-[0.985] disabled:opacity-50 ' +
    'disabled:cursor-not-allowed disabled:active:scale-100 select-none';
  // Her iki boy da min-h-11 (44px): 'sm' YÜKSEKLİKTE değil, yatay dolguda
  // ve ağırlıkta küçüktür. 36px'lik bir buton masaüstünde fareyle sorunsuz
  // ama tablette ıskalanır — ve aynı bileşen her ikisinde de kullanılıyor.
  const sizes = { md: 'min-h-11 px-5 text-sm', sm: 'min-h-11 px-3.5 text-sm' };
  const variants = {
    primary: 'bg-brand-500 text-white hover:bg-brand-600',
    outline: 'border border-[var(--border)] text-[var(--text)] hover:bg-[var(--raised)]',
    ghost:   'text-[var(--muted)] hover:bg-[var(--raised)] hover:text-[var(--text)]',
    danger:  'bg-[var(--color-bad)] text-white hover:opacity-90',
  };
  return (
    <button
      {...rest}
      disabled={disabled || loading}
      aria-busy={loading || undefined}
      className={cx(base, sizes[size], variants[variant], fullWidth && 'w-full', className)}
    >
      {loading && <Spinner />}
      {children}
    </button>
  );
}

export function Spinner({ className }: { className?: string }) {
  return (
    <svg className={cx('size-4 animate-spin', className)} viewBox="0 0 24 24" fill="none" aria-hidden>
      <circle cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="3" className="opacity-25" />
      <path d="M22 12a10 10 0 0 1-10 10" stroke="currentColor" strokeWidth="3" strokeLinecap="round" />
    </svg>
  );
}

/* ─────────────── Kart ─────────────── */

export function Card({ className, children, ...rest }: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div {...rest} className={cx('surface rounded-2xl border p-4 md:p-6', className)}>
      {children}
    </div>
  );
}

/* ─────────────── Form alanı ─────────────── */

type FieldProps = React.InputHTMLAttributes<HTMLInputElement> & {
  label: string;
  error?: string;
  hint?: string;
};

export const Field = React.forwardRef<HTMLInputElement, FieldProps>(function Field(
  { label, error, hint, id, className, ...rest }, ref,
) {
  const autoId = React.useId();
  const fieldId = id ?? autoId;
  const errId = `${fieldId}-err`;
  const hintId = `${fieldId}-hint`;
  return (
    <div className="flex flex-col gap-1.5">
      <label htmlFor={fieldId} className="text-sm font-medium">{label}</label>
      <input
        {...rest}
        ref={ref}
        id={fieldId}
        aria-invalid={error ? true : undefined}
        // Hata mesajı ekran okuyucuya alanla BİRLİKTE okunur; yalnız kırmızı
        // kenarlık görme engelli kullanıcı için hiçbir şey ifade etmez.
        aria-describedby={cx(error && errId, hint && hintId) || undefined}
        className={cx(
          'raised min-h-11 w-full rounded-xl border px-3.5 py-2.5 text-base',
          'placeholder:text-[var(--muted)] outline-none',
          'focus:border-brand-400',
          error && 'border-[var(--color-bad)]',
          className,
        )}
      />
      {hint && !error && <p id={hintId} className="text-xs text-muted">{hint}</p>}
      {error && (
        <p id={errId} role="alert" className="text-xs text-[var(--color-bad)]">{error}</p>
      )}
    </div>
  );
});

/* ─────────────── Uyarı kutusu ─────────────── */

export function Alert({
  tone = 'bad', children, className,
}: { tone?: 'bad' | 'ok' | 'warn' | 'info'; children: React.ReactNode; className?: string }) {
  const tones = {
    bad:  'border-[var(--color-bad)]/40  bg-[var(--color-bad)]/10  text-[var(--color-bad)]',
    ok:   'border-[var(--color-ok)]/40   bg-[var(--color-ok)]/10   text-[var(--color-ok)]',
    warn: 'border-[var(--color-warn)]/40 bg-[var(--color-warn)]/10 text-[var(--color-warn)]',
    info: 'border-[var(--color-info)]/40 bg-[var(--color-info)]/10 text-[var(--color-info)]',
  };
  return (
    <div role="alert" className={cx('rounded-xl border px-4 py-3 text-sm', tones[tone], className)}>
      {children}
    </div>
  );
}

/* ─────────────── Rozet ─────────────── */

export function Badge({
  tone = 'neutral', children,
}: { tone?: 'neutral' | 'ok' | 'warn' | 'bad' | 'brand'; children: React.ReactNode }) {
  const tones = {
    neutral: 'bg-[var(--raised)] text-[var(--muted)] border-[var(--border)]',
    ok:   'bg-[var(--color-ok)]/12   text-[var(--color-ok)]   border-[var(--color-ok)]/30',
    warn: 'bg-[var(--color-warn)]/12 text-[var(--color-warn)] border-[var(--color-warn)]/30',
    bad:  'bg-[var(--color-bad)]/12  text-[var(--color-bad)]  border-[var(--color-bad)]/30',
    brand:'bg-brand-500/12 text-brand-300 border-brand-500/30',
  };
  return (
    <span className={cx(
      'inline-flex items-center rounded-lg border px-2 py-0.5 text-xs font-medium whitespace-nowrap',
      tones[tone],
    )}>{children}</span>
  );
}

/* ─────────────── İskelet (yükleniyor) ─────────────── */

export function Skeleton({ className }: { className?: string }) {
  return <div className={cx('animate-pulse rounded-lg bg-[var(--raised)]', className)} aria-hidden />;
}

/* ─────────────── Boş durum ─────────────── */

export function Empty({ title, hint }: { title: string; hint?: string }) {
  return (
    <div className="flex flex-col items-center gap-2 px-4 py-12 text-center">
      <p className="font-medium">{title}</p>
      {hint && <p className="max-w-sm text-sm text-muted">{hint}</p>}
    </div>
  );
}

