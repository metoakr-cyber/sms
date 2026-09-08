'use client';

import * as React from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { ApiError, apiFetch } from '@/lib/api';
import { Button, Card, Field, Alert } from '@/components/ui';
import { Recaptcha, captchaEnabled, type CaptchaHandle } from '@/components/recaptcha';

/**
 * İstemci tarafı şifre gücü göstergesi.
 *
 * Bu bir GÜVENLİK KONTROLÜ DEĞİLDİR — yalnız geri bildirimdir. Kararı sunucu
 * verir (service/auth: uzunluk + sözlük tabanlı kelime kontrolü). Burada
 * yapılan tek şey kullanıcıyı gönderime basmadan önce uyarmaktır.
 */
function strength(pw: string): { score: 0 | 1 | 2 | 3; label: string; tone: string } {
  if (pw.length < 10) return { score: 0, label: 'Çok kısa — en az 10 karakter', tone: 'var(--color-bad)' };
  let s = 0;
  if (/[a-zçğıöşü]/.test(pw) && /[A-ZÇĞİÖŞÜ]/.test(pw)) s++;
  if (/\d/.test(pw)) s++;
  if (/[^\p{L}\d]/u.test(pw)) s++;
  if (pw.length >= 16) s++;
  if (s >= 3) return { score: 3, label: 'Güçlü', tone: 'var(--color-ok)' };
  if (s === 2) return { score: 2, label: 'Orta', tone: 'var(--color-warn)' };
  return { score: 1, label: 'Zayıf', tone: 'var(--color-bad)' };
}

export default function RegisterForm() {
  const router = useRouter();
  const [busy, setBusy] = React.useState(false);
  const [formError, setFormError] = React.useState('');
  const [fields, setFields] = React.useState<Record<string, string>>({});
  const [password, setPassword] = React.useState('');
  const [captchaToken, setCaptchaToken] = React.useState('');
  const captcha = React.useRef<CaptchaHandle>(null);

  const pwInfo = password ? strength(password) : null;

  async function onSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (busy) return;
    const fd = new FormData(e.currentTarget);
    setBusy(true); setFormError(''); setFields({});

    try {
      await apiFetch('/auth/register', {
        method: 'POST',
        body: {
          email: String(fd.get('email') ?? '').trim(),
          username: String(fd.get('username') ?? '').trim(),
          password: String(fd.get('password') ?? ''),
          acceptTerms: fd.get('acceptTerms') === 'on',
          captchaToken,
        },
      });
      router.replace('/giris?kayit=tamam');
    } catch (err) {
      if (err instanceof ApiError) {
        setFields(err.fieldMap());
        if (!err.fields?.length) setFormError(err.message);
      } else {
        setFormError('Beklenmeyen bir hata oluştu.');
      }
      captcha.current?.reset();
    } finally {
      setBusy(false);
    }
  }

  return (
    <Card className="p-6 md:p-8">
      <h1 className="text-2xl font-bold tracking-tight">Hesap aç</h1>
      <p className="mt-1.5 text-sm text-muted">Ücretsiz. Abonelik yok.</p>

      {formError && <Alert className="mt-5">{formError}</Alert>}

      <form onSubmit={onSubmit} className="mt-6 flex flex-col gap-4" noValidate>
        <Field
          label="E-posta" name="email" type="email" required
          autoComplete="email" inputMode="email" autoCapitalize="none"
          autoCorrect="off" spellCheck={false}
          placeholder="ornek@eposta.com" error={fields.email}
          hint="Hesabınızı doğrulamak için bu adrese bağlantı göndereceğiz."
        />
        <Field
          label="Kullanıcı adı" name="username" required
          autoComplete="username" autoCapitalize="none" autoCorrect="off" spellCheck={false}
          minLength={3} maxLength={32} placeholder="kullanici_adi"
          error={fields.username} hint="3–32 karakter; harf, rakam ve alt çizgi."
        />
        <div className="flex flex-col gap-1.5">
          <Field
            label="Şifre" name="password" type="password" required
            autoComplete="new-password" minLength={10} placeholder="••••••••"
            value={password} onChange={(e) => setPassword(e.target.value)}
            error={fields.password}
          />
          {pwInfo && (
            <div className="flex items-center gap-2" aria-live="polite">
              <div className="h-1 flex-1 overflow-hidden rounded-full bg-[var(--raised)]">
                <div className="h-full rounded-full transition-all"
                     style={{ width: `${(pwInfo.score / 3) * 100}%`, background: pwInfo.tone }} />
              </div>
              <span className="text-xs" style={{ color: pwInfo.tone }}>{pwInfo.label}</span>
            </div>
          )}
        </div>

        <label className="flex items-start gap-3 py-1">
          {/* size-5 + py-1 ile dokunma alanı 44px'e çıkar (§2.3) */}
          <input type="checkbox" name="acceptTerms" required
                 className="mt-0.5 size-5 shrink-0 rounded accent-[var(--color-brand-500)]" />
          <span className="text-sm leading-relaxed text-muted">
            <Link href="/kullanim-sartlari" className="text-brand-400 underline underline-offset-4">
              Kullanım şartlarını
            </Link>{' '}ve{' '}
            <Link href="/gizlilik" className="text-brand-400 underline underline-offset-4">
              gizlilik politikasını
            </Link>{' '}okudum, kabul ediyorum.
          </span>
        </label>
        {fields.acceptTerms && (
          <p role="alert" className="-mt-2 text-xs text-[var(--color-bad)]">{fields.acceptTerms}</p>
        )}

        {captchaEnabled && <Recaptcha ref={captcha} onChange={setCaptchaToken} />}

        <Button type="submit" loading={busy} fullWidth
                disabled={captchaEnabled && !captchaToken}>
          Hesap aç
        </Button>
      </form>

      <p className="mt-6 text-center text-sm text-muted">
        Zaten hesabınız var mı?{' '}
        <Link href="/giris" className="text-brand-400 underline-offset-4 hover:underline">Giriş yapın</Link>
      </p>
    </Card>
  );
}
