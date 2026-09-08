'use client';

import * as React from 'react';
import Link from 'next/link';
import { useRouter, useSearchParams } from 'next/navigation';
import { useQueryClient } from '@tanstack/react-query';
import { ApiError, apiFetch } from '@/lib/api';
import { Button, Card, Field, Alert } from '@/components/ui';
import { Recaptcha, captchaEnabled, type CaptchaHandle } from '@/components/recaptcha';
import { meKey } from '@/hooks/useSession';

export default function LoginForm() {
  const router = useRouter();
  const params = useSearchParams();
  const qc = useQueryClient();

  const [busy, setBusy] = React.useState(false);
  const [formError, setFormError] = React.useState('');
  const [fields, setFields] = React.useState<Record<string, string>>({});
  const [captchaToken, setCaptchaToken] = React.useState('');
  const captcha = React.useRef<CaptchaHandle>(null);

  const justRegistered = params.get('kayit') === 'tamam';
  const sessionExpired = params.get('sebep') === 'oturum';

  async function onSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (busy) return; // çift gönderimi engelle: giriş hesap kilidini tetikler

    const fd = new FormData(e.currentTarget);
    setBusy(true); setFormError(''); setFields({});

    try {
      await apiFetch('/auth/login', {
        method: 'POST',
        body: {
          email: String(fd.get('email') ?? '').trim(),
          password: String(fd.get('password') ?? ''),
          captchaToken,
        },
      });

      // Oturum çerezi geldi; kullanıcıyı SUNUCUDAN taze çekmeden yönlendirmeyiz,
      // yoksa panel bir an "giriş yapılmamış" görüp geri fırlatır.
      await qc.invalidateQueries({ queryKey: meKey });
      router.replace(params.get('devam') ?? '/panel');
    } catch (err) {
      if (err instanceof ApiError) {
        setFields(err.fieldMap());
        // Alan bazlı hata varsa genel bandı göstermeyiz — aynı bilgi iki kez.
        if (!err.fields?.length) setFormError(err.message);
      } else {
        setFormError('Beklenmeyen bir hata oluştu.');
      }
      // Bir reCAPTCHA token'ı YALNIZ BİR KEZ kullanılabilir. Sıfırlamazsak
      // kullanıcının ikinci denemesi her zaman başarısız olur.
      captcha.current?.reset();
    } finally {
      setBusy(false);
    }
  }

  return (
    <Card className="p-6 md:p-8">
      <h1 className="text-2xl font-bold tracking-tight">Giriş yap</h1>
      <p className="mt-1.5 text-sm text-muted">Hesabınıza erişmek için bilgilerinizi girin.</p>

      {justRegistered && (
        <Alert tone="ok" className="mt-5">
          Kaydınız alındı. E-postanıza gönderilen bağlantı ile hesabınızı doğrulayın.
        </Alert>
      )}
      {sessionExpired && (
        <Alert tone="warn" className="mt-5">Oturumunuzun süresi doldu. Lütfen tekrar giriş yapın.</Alert>
      )}
      {formError && <Alert className="mt-5">{formError}</Alert>}

      <form onSubmit={onSubmit} className="mt-6 flex flex-col gap-4" noValidate>
        <Field
          label="E-posta" name="email" type="email" required
          autoComplete="email" inputMode="email" autoCapitalize="none"
          autoCorrect="off" spellCheck={false}
          placeholder="ornek@eposta.com" error={fields.email}
        />
        <Field
          label="Şifre" name="password" type="password" required
          autoComplete="current-password" placeholder="••••••••"
          error={fields.password}
        />

        <div className="-mt-2 flex justify-end">
          <Link href="/sifremi-unuttum"
                className="inline-flex min-h-11 items-center text-sm text-brand-400
                           underline-offset-4 hover:underline">
            Şifremi unuttum
          </Link>
        </div>

        {captchaEnabled && <Recaptcha ref={captcha} onChange={setCaptchaToken} />}

        <Button type="submit" loading={busy} fullWidth
                disabled={captchaEnabled && !captchaToken}>
          Giriş yap
        </Button>
      </form>

      <p className="mt-6 text-center text-sm text-muted">
        Hesabınız yok mu?{' '}
        <Link href="/kayit" className="text-brand-400 underline-offset-4 hover:underline">Kayıt olun</Link>
      </p>
    </Card>
  );
}
