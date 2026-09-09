'use client';

import * as React from 'react';
import Link from 'next/link';
import { apiFetch } from '@/lib/api';
import { Card, Button, Field, Alert } from '@/components/ui';

export function ForgotForm() {
  const [busy, setBusy] = React.useState(false);
  const [sent, setSent] = React.useState(false);

  async function onSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (busy) return;
    const fd = new FormData(e.currentTarget);
    setBusy(true);
    try {
      await apiFetch('/auth/password/forgot', {
        method: 'POST',
        body: { email: String(fd.get('email') ?? '').trim() },
      });
    } catch {
      // BİLEREK YUTULUYOR.
      //
      // Hata gösterirsek "bu e-posta kayıtlı değil" bilgisini sızdırırız ve
      // saldırgan hangi adreslerin sistemde olduğunu tek tek öğrenebilir.
      // Sunucu da aynı nedenle her durumda 200 döner; istemci bu sözleşmeyi
      // bozmamalıdır.
    } finally {
      setBusy(false);
      setSent(true);
    }
  }

  return (
    <Card className="p-6 md:p-8">
      <h1 className="text-2xl font-bold tracking-tight">Şifremi unuttum</h1>

      {sent ? (
        <>
          <Alert tone="ok" className="mt-5">
            E-posta adresiniz kayıtlıysa şifre sıfırlama bağlantısı gönderildi.
            Gelen kutunuzu ve spam klasörünü kontrol edin.
          </Alert>
          <Link href="/giris" className="mt-5 block">
            <Button variant="outline" fullWidth>Giriş ekranına dön</Button>
          </Link>
        </>
      ) : (
        <>
          <p className="mt-1.5 text-sm text-muted">
            Hesabınızın e-posta adresini girin, size sıfırlama bağlantısı gönderelim.
          </p>
          <form onSubmit={onSubmit} className="mt-6 flex flex-col gap-4" noValidate>
            <Field label="E-posta" name="email" type="email" required
                   autoComplete="email" inputMode="email" autoCapitalize="none"
                   autoCorrect="off" spellCheck={false} placeholder="ornek@eposta.com" />
            <Button type="submit" loading={busy} fullWidth>Bağlantı gönder</Button>
          </form>
          <div className="mt-4 flex justify-center">
            <Link href="/giris"
                  className="inline-flex min-h-11 items-center px-2 text-sm text-brand-400
                             underline-offset-4 hover:underline">
              Giriş ekranına dön
            </Link>
          </div>
        </>
      )}
    </Card>
  );
}
