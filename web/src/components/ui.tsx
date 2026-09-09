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
    //
    // 🔴 `transition-colors` DEĞİL — `active:scale` ile birlikte kullanılırsa
    // ölçek geçişi HİÇ tanımlanmaz ve buton basınca ANİ sıçrar. Ölçüldü:
    // eskiden burada yalnız `transition-colors` vardı ve `active:scale-[0.985]`
    // yumuşamıyordu. Basma geri bildiriminin amacı "arayüz seni duydu" demektir;
    // ani sıçrama bunu bozar.
    //
    // `transition-all` de DEĞİL: Emil'in kuralı özelliği açıkça saymaktır.
    // `all` düzen/boya tetikleyen özellikleri de kapsar ve GPU'dan düşürür.
    // Yalnız `transform` + `color`/`background`/`border` geçer.
    //
    // `basma-geri-bildirimi` bir STİL sınıfı değil, bir KANCA: globals.css'in
    // `prefers-reduced-motion` bloğu geçiş listesini daraltırken `transform`ı
    // yalnız bu sınıfta bırakıyor. Kancasız kalırsa hareket duyarlı kullanıcıda
    // basma geri bildirimi ölür — §4.5 onu açıkça korunacaklar arasında sayıyor.
    'inline-flex items-center justify-center gap-2 rounded-xl font-medium ' +
    'basma-geri-bildirimi ' +
    '[transition-property:color,background-color,border-color,transform] ' +
    '[transition-duration:var(--sure-basma)] ' +
    '[transition-timing-function:var(--ease-out)] ' +
    'active:scale-[0.97] disabled:opacity-50 ' +
    'disabled:cursor-not-allowed disabled:active:scale-100 select-none';
  // Her iki boy da min-h-11 (44px): 'sm' YÜKSEKLİKTE değil, yatay dolguda
  // ve ağırlıkta küçüktür. 36px'lik bir buton masaüstünde fareyle sorunsuz
  // ama tablette ıskalanır — ve aynı bileşen her ikisinde de kullanılıyor.
  const sizes = { md: 'min-h-11 px-5 text-sm', sm: 'min-h-11 px-3.5 text-sm' };
  const variants = {
    primary: 'bg-brand-500 text-white hover:bg-brand-600',
    outline: 'border border-[var(--border)] text-[var(--text)] hover:bg-[var(--raised)]',
    ghost:   'text-[var(--muted)] hover:bg-[var(--raised)] hover:text-[var(--text)]',
    // 🔴 Dolgu `--color-bad` DEĞİL `--durum-bad-dolgu`: beyaz metin #ea4d4d
    // üstünde 3,71 veriyordu (ölçüldü) — eşik 4,5. Yeni jeton 5,10 veriyor.
    // Rozet/uyarı zeminleri ve hata kenarlıkları `--color-bad`'da KALIR.
    danger:  'bg-[var(--durum-bad-dolgu)] text-white hover:opacity-90',
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
    // gap-2 (8px) — `gap-1.5` yarım basamaktı (§2.3) ve `Secim`/`CokSatir`
    // gap-2 kullanıyor. Aynı formda iki farklı etiket aralığı vardı.
    <div className="flex flex-col gap-2">
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
          // 🔴 ÖLÇEK `Secim`/`CokSatir` İLE AYNI. Eskiden `min-h-11 px-3.5
          // py-2.5` idi; yeni alanlar `min-h-12 px-4 py-3` kullanıyor ve ikisi
          // AYNI FORMDA yan yana duruyordu — 44px'lik bir girdinin altında
          // 48px'lik bir seçim kutusu. Yarım basamaklar da §2.3'e aykırıydı.
          'raised min-h-12 w-full rounded-xl border px-4 py-3 text-base',
          'placeholder:text-[var(--muted)] outline-none focus:border-brand-400',
          'disabled:cursor-not-allowed disabled:opacity-60',
          // Yalnız RENK geçişi. Odak/hover'da konum hareketi yok (§4.3).
          '[transition-property:border-color] [transition-duration:var(--sure-hizli)]',
          error && 'border-[var(--color-bad)]',
          className,
        )}
      />
      {/*
        İPUCU VE HATA 14px (`text-sm`), 12px DEĞİL — §3.2: veri taşıyan metin
        14px altına inmez. Bir doğrulama hatası "ikincil dipnot" değil,
        kullanıcının formu gönderebilmek için OKUMAK ZORUNDA olduğu metindir.
        `AlanKabi` (alanlar.tsx) zaten böyle; ikisi artık aynı ölçekte.
      */}
      {hint && !error && <p id={hintId} className="text-sm text-muted">{hint}</p>}
      {error && (
        <p id={errId} role="alert" className="text-sm text-[var(--durum-bad-metin)]">{error}</p>
      )}
    </div>
  );
});

/* ─────────────── Uyarı kutusu ─────────────── */

/**
 * Uyarı kutusu.
 *
 * 🔴 `duyur` — `role="alert"` VERİLİP VERİLMEYECEĞİ.
 *
 * `role="alert"` ekran okuyucuya "kullanıcının o an yaptığı şeyi BÖL ve bunu
 * oku" der. Bir işlemin sonucunda beliren hata için doğrudur; sayfa açılışında
 * koşulsuz çizilen açıklama kutusu için YANLIŞTIR — kullanıcı daha hiçbir şey
 * yapmadan sözü kesilir. Ölçüm: `/yonetim` altında 28 `Alert` var ve bir kısmı
 * (bakiye düzeltme ekranının kalıcı uyarısı dâhil) açılışta koşulsuz render
 * ediliyordu; hepsi kesintili duyuru olarak okunuyordu.
 *
 * Varsayılan `true` — bugünkü davranış korunur, sessiz bir gerileme olmaz.
 * Statik açıklama kutuları `duyur={false}` ile bunu KAPATIR.
 */
export function Alert({
  tone = 'bad', duyur = true, children, className,
}: {
  tone?: 'bad' | 'ok' | 'warn' | 'info';
  duyur?: boolean;
  children: React.ReactNode;
  className?: string;
}) {
  // 🔴 ZEMİN `--color-*`, METİN `--durum-*-metin`. Metin de zeminle aynı tam
  // doygun renkti; kendi %10'luk pastelinin üstünde ölçüldü ve AÇIK temada
  // dört tonun DÖRDÜ DE eşiğin altındaydı (1,88–3,27), koyu temada `bad`
  // 3,81'de kalıyordu. `hata-durumu.tsx`'in `requestId` satırı bu kutuda.
  const tones = {
    bad:  'border-[var(--color-bad)]/40  bg-[var(--color-bad)]/10  text-[var(--durum-bad-metin)]',
    ok:   'border-[var(--color-ok)]/40   bg-[var(--color-ok)]/10   text-[var(--durum-ok-metin)]',
    warn: 'border-[var(--color-warn)]/40 bg-[var(--color-warn)]/10 text-[var(--durum-warn-metin)]',
    info: 'border-[var(--color-info)]/40 bg-[var(--color-info)]/10 text-[var(--durum-info-metin)]',
  };
  return (
    <div role={duyur ? 'alert' : undefined}
         className={cx('rounded-xl border px-4 py-3 text-sm', tones[tone], className)}>
      {children}
    </div>
  );
}

/* ─────────────── Rozet ─────────────── */

export function Badge({
  tone = 'neutral', children,
}: { tone?: 'neutral' | 'ok' | 'warn' | 'bad' | 'brand'; children: React.ReactNode }) {
  // Metin rengi `--durum-*-metin`'den gelir (Alert ile aynı gerekçe).
  // `DurumRozeti` panelin TÜM durum etiketlerini buraya yönlendiriyor:
  // "Reddedildi", "Askıda", "Başarısız", "Kurulu değil" — hepsi `bad`.
  const tones = {
    neutral: 'bg-[var(--raised)] text-[var(--muted)] border-[var(--border)]',
    ok:   'bg-[var(--color-ok)]/12   text-[var(--durum-ok-metin)]    border-[var(--color-ok)]/30',
    warn: 'bg-[var(--color-warn)]/12 text-[var(--durum-warn-metin)]  border-[var(--color-warn)]/30',
    bad:  'bg-[var(--color-bad)]/12  text-[var(--durum-bad-metin)]   border-[var(--color-bad)]/30',
    brand:'bg-brand-500/12           text-[var(--durum-marka-metin)] border-brand-500/30',
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

