'use client';

import * as React from 'react';
import Link from 'next/link';
import { useRouter, useSearchParams } from 'next/navigation';
import { ApiError, apiFetch } from '@/lib/api';
import { Button, Card, Field, Alert } from '@/components/ui';

/**
 * Şifre sıfırlama — "şifremi unuttum" e-postasındaki bağlantının indiği sayfa.
 *
 * Bağlantıyı `api/internal/service/auth/emails.go:32` üretir:
 * `{PUBLIC_BASE_URL}/sifre-sifirla?token=...`.
 *
 * Token bir SIRDIR. Bu dosya onu yalnız iki yere koyar: istek gövdesi ve bir
 * ref. Sunucu token'ı tek seferde tükettiği için (service/auth/service.go:368)
 * istek bittiği anda adres çubuğundan silinir; sorgu dizesi tarayıcı geçmişine
 * yazılır ve sayfadan çıkan bağlantıların Referer başlığında taşınabilir.
 */

/**
 * İstemci tarafı şifre gücü göstergesi.
 *
 * 🔴 kayit/form.tsx:22 ile AYNI fonksiyon. Kopya olmasının tek sebebi, o
 * dosyanın bu dalgada başka bir ajana ait olması ve ortak bir modüle taşımanın
 * kapsam dışı kalması. İkisi ayrışırsa kullanıcı kayıtta "Güçlü" gördüğü bir
 * şifreyi sıfırlamada "Zayıf" görür — `web/src/lib/` altına alınmalı.
 *
 * Bu bir GÜVENLİK KONTROLÜ DEĞİLDİR — yalnız geri bildirimdir. Kararı sunucu
 * verir: domain/auth.CheckPasswordStrength (uzunluk + yaygın şifre sözlüğü +
 * e-posta/kullanıcı adı benzerliği).
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

/** Adres çubuğundaki token'ı sayfayı yeniden yüklemeden kaldırır. */
function stripToken() {
  if (typeof window === 'undefined') return;
  const url = new URL(window.location.href);
  if (!url.searchParams.has('token')) return;
  url.searchParams.delete('token');
  window.history.replaceState(null, '', `${url.pathname}${url.search}${url.hash}`);
}

export default function ResetPasswordForm() {
  const router = useRouter();
  const params = useSearchParams();

  // Token ilk render'da yakalanır: başarıdan sonraki temizlik onu sorgu
  // dizesinden siler ve params.get('token') boş dönmeye başlar.
  const tokenRef = React.useRef<string>(params.get('token') ?? '');

  const [busy, setBusy] = React.useState(false);
  const [done, setDone] = React.useState(false);
  const [tokenDead, setTokenDead] = React.useState(false);
  const [formError, setFormError] = React.useState('');
  const [requestId, setRequestId] = React.useState<string | undefined>();
  const [fields, setFields] = React.useState<Record<string, string>>({});
  const [password, setPassword] = React.useState('');
  const [confirm, setConfirm] = React.useState('');

  const pwInfo = password ? strength(password) : null;
  const mismatch = confirm.length > 0 && confirm !== password;

  // Başarıdan sonra giriş ekranına. Kısa gecikme, kullanıcının "şifreniz
  // güncellendi" mesajını okumasına vakit bırakır; tüm oturumları kapatan bir
  // işlemin sessizce sayfa değiştirmesi "oldu mu, olmadı mı?" bırakır.
  React.useEffect(() => {
    if (!done) return;
    const t = window.setTimeout(() => router.replace('/giris'), 2200);
    return () => window.clearTimeout(t);
  }, [done, router]);

  async function onSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (busy) return;

    const fd = new FormData(e.currentTarget);
    const pw = String(fd.get('password') ?? '');
    const pw2 = String(fd.get('passwordConfirm') ?? '');

    setFormError(''); setRequestId(undefined); setFields({});

    if (pw !== pw2) {
      setFields({ passwordConfirm: 'Şifreler birbiriyle eşleşmiyor.' });
      return;
    }

    setBusy(true);
    try {
      await apiFetch('/auth/password/reset', {
        method: 'POST',
        body: { token: tokenRef.current, password: pw },
      });
      stripToken();
      setDone(true);
    } catch (err) {
      stripToken();
      if (err instanceof ApiError) {
        // TOKEN_INVALID (410) ayrı ele alınır: form göstermeye devam etmek
        // kullanıcıyı çalışmayacak bir denemeye daha sokar. Doğru çıkış yolu
        // yeni bir bağlantı istemektir.
        if (err.code === 'TOKEN_INVALID') {
          setTokenDead(true);
          setRequestId(err.requestId);
          return;
        }
        setFields(err.fieldMap());
        if (!err.fields?.length) setFormError(err.message);
        setRequestId(err.requestId);
      } else {
        setFormError('Beklenmeyen bir hata oluştu.');
      }
    } finally {
      setBusy(false);
    }
  }

  /* ── Bağlantı hiç token taşımıyor ya da token ölü ── */
  if (!tokenRef.current || tokenDead) {
    return (
      <Card className="p-6 md:p-8">
        <h1 className="text-2xl font-bold tracking-tight">Şifre sıfırlama</h1>
        <Alert className="mt-5">
          Bağlantı geçersiz veya süresi dolmuş.
          {requestId && (
            <span className="mt-2 block text-xs opacity-70">İstek no: {requestId}</span>
          )}
        </Alert>
        <p className="mt-4 text-sm leading-relaxed text-muted">
          Sıfırlama bağlantısı 1 saat geçerlidir ve yalnız bir kez
          kullanılabilir. Yeni bir bağlantı isteyin.
        </p>
        <Link href="/sifremi-unuttum" className="mt-5 block">
          <Button fullWidth>Yeni bağlantı iste</Button>
        </Link>
        <div className="mt-4 flex justify-center">
          <Link href="/giris"
                className="inline-flex min-h-11 items-center px-2 text-sm text-brand-400
                           underline-offset-4 hover:underline">
            Giriş ekranına dön
          </Link>
        </div>
      </Card>
    );
  }

  /* ── Başarı ── */
  if (done) {
    return (
      <Card className="p-6 md:p-8">
        <h1 className="text-2xl font-bold tracking-tight">Şifre sıfırlama</h1>
        <Alert tone="ok" className="mt-5">
          Şifreniz güncellendi. Güvenliğiniz için açık olan tüm oturumlarınız
          kapatıldı — yeni şifrenizle tekrar giriş yapın.
        </Alert>
        <Link href="/giris" className="mt-5 block">
          <Button fullWidth>Giriş yap</Button>
        </Link>
      </Card>
    );
  }

  /* ── Form ── */
  return (
    <Card className="p-6 md:p-8">
      <h1 className="text-2xl font-bold tracking-tight">Yeni şifre belirleyin</h1>
      <p className="mt-1.5 text-sm text-muted">
        Şifreniz değiştiğinde tüm cihazlardaki oturumlarınız kapatılır.
      </p>

      {formError && (
        <Alert className="mt-5">
          {formError}
          {requestId && (
            <span className="mt-2 block text-xs opacity-70">İstek no: {requestId}</span>
          )}
        </Alert>
      )}

      <form onSubmit={onSubmit} className="mt-6 flex flex-col gap-4" noValidate>
        {/* Şifre yöneticileri için gizli kullanıcı adı alanı yoktur: token'dan
            hangi hesap olduğunu istemci bilmiyor. */}
        <div className="flex flex-col gap-1.5">
          <Field
            label="Yeni şifre" name="password" type="password" required
            autoComplete="new-password" minLength={10} placeholder="••••••••"
            value={password} onChange={(e) => setPassword(e.target.value)}
            error={fields.password}
            hint="En az 10 karakter. E-posta veya kullanıcı adınızı içeremez."
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

        <Field
          label="Yeni şifre (tekrar)" name="passwordConfirm" type="password" required
          autoComplete="new-password" minLength={10} placeholder="••••••••"
          value={confirm} onChange={(e) => setConfirm(e.target.value)}
          error={fields.passwordConfirm ?? (mismatch ? 'Şifreler birbiriyle eşleşmiyor.' : undefined)}
        />

        <Button type="submit" loading={busy} fullWidth>Şifreyi güncelle</Button>
      </form>

      <div className="mt-4 flex justify-center">
        <Link href="/giris"
              className="inline-flex min-h-11 items-center px-2 text-sm text-brand-400
                         underline-offset-4 hover:underline">
          Giriş ekranına dön
        </Link>
      </div>
    </Card>
  );
}
