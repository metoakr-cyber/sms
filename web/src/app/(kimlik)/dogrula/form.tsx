'use client';

import * as React from 'react';
import Link from 'next/link';
import { useRouter, useSearchParams } from 'next/navigation';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { ApiError, apiFetch } from '@/lib/api';
import { Button, Card, Alert, Spinner } from '@/components/ui';
import { meKey, useSession } from '@/hooks/useSession';

/**
 * E-posta doğrulama — kayıt e-postasındaki bağlantının indiği sayfa.
 *
 * Bağlantıyı `api/internal/service/auth/emails.go:16` üretir:
 * `{PUBLIC_BASE_URL}/dogrula?token=...`. Buradaki tek iş, token'ı
 * `POST /auth/verify-email` ucuna taşımaktır.
 *
 * Token bir SIRDIR. Bu dosya onu yalnız iki yere koyar: istek gövdesi ve bir
 * ref. İstek bittiği anda adres çubuğundan silinir (`stripToken`), çünkü sorgu
 * dizesi tarayıcı geçmişine yazılır ve sayfadan çıkan bağlantıların Referer
 * başlığında taşınabilir. Sayfada log veya analytics çağrısı bulunmuyor.
 */

/** Adres çubuğundaki token'ı sayfayı yeniden yüklemeden kaldırır. */
function stripToken() {
  if (typeof window === 'undefined') return;
  const url = new URL(window.location.href);
  if (!url.searchParams.has('token')) return;
  url.searchParams.delete('token');
  window.history.replaceState(null, '', `${url.pathname}${url.search}${url.hash}`);
}

export default function VerifyEmailForm() {
  const router = useRouter();
  const params = useSearchParams();
  const qc = useQueryClient();
  const { isAuthenticated } = useSession();

  // Token ilk render'da yakalanır: aşağıdaki temizlik onu sorgu dizesinden
  // siler ve sonraki render'larda params.get('token') boş döner.
  const tokenRef = React.useRef<string>(params.get('token') ?? '');
  const started = React.useRef(false);

  const verify = useMutation({
    retry: false,
    mutationFn: (token: string) =>
      apiFetch<{ message: string }>('/auth/verify-email', {
        method: 'POST',
        body: { token },
      }),
    onSuccess: () => {
      // Oturum açıkken doğruladıysa `emailVerified` değişti; panel bandı
      // ve satın alma kilidi buna bakıyor.
      qc.invalidateQueries({ queryKey: meKey });
    },
    // Başarıda da hatada da temizlenir: sunucu token'ı `ConsumeAuthToken` ile
    // tek seferde tüketir (service/auth/service.go:172), yani her iki durumda
    // da adresteki değer artık ölüdür — geçmişte tutmanın faydası yok.
    onSettled: stripToken,
  });

  React.useEffect(() => {
    // TEK ATIŞ.
    //
    // Token tek kullanımlıktır: ikinci POST TOKEN_INVALID döner ve kullanıcı
    // doğrulama başarılıyken "bağlantı geçersiz" görür. Effect'i iki kez
    // koşturan iki bilinen durum var — StrictMode'un geliştirmedeki çift
    // çağrısı ve Fast Refresh sonrası yeniden bağlanma. Ref, bileşen örneğine
    // bağlı olduğu için ikisinde de ayakta kalır.
    //
    // ÖLÇÜM NOTU: bu depodaki `next dev` yapılandırmasında effect bugün TEK
    // KEZ koşuyor (ölçüldü), yani muhafız şu an sessiz duruyor. Ucuz bir
    // sigorta olarak duruyor; kaldırıldığında bugün hiçbir test düşmez.
    if (started.current) return;
    started.current = true;
    if (tokenRef.current) verify.mutate(tokenRef.current);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Başarıdan sonra yönlendirme.
  //
  // Hedef oturuma göre seçilir: oturumu olmayan kullanıcıyı /panel'e atmak,
  // PanelShell'in "Oturumunuzun süresi doldu" uyarısıyla geri fırlatmasına
  // yol açardı — hesabını yeni doğrulamış birine söylenecek en yanlış cümle.
  React.useEffect(() => {
    if (!verify.isSuccess) return;
    const t = window.setTimeout(
      () => router.replace(isAuthenticated ? '/panel' : '/giris'),
      1800,
    );
    return () => window.clearTimeout(t);
  }, [verify.isSuccess, isAuthenticated, router]);

  const err = verify.error instanceof ApiError ? verify.error : null;

  return (
    <Card className="p-6 md:p-8">
      <h1 className="text-2xl font-bold tracking-tight">E-posta doğrulama</h1>

      {!tokenRef.current ? (
        <>
          <Alert className="mt-5">
            Bağlantı eksik görünüyor. E-postadaki adresi tarayıcıya elle
            yazdıysanız bir karakter eksilmiş olabilir; bağlantıya doğrudan
            tıklayarak tekrar deneyin.
          </Alert>
          <BackLinks />
        </>
      ) : verify.isPending ? (
        <div className="mt-6 flex flex-col items-center gap-3 py-6 text-center">
          <Spinner className="size-7 text-brand-400" />
          <p className="text-sm text-muted">Hesabınız doğrulanıyor…</p>
        </div>
      ) : verify.isSuccess ? (
        <>
          <Alert tone="ok" className="mt-5">
            E-posta adresiniz doğrulandı. Hesabınız artık numara satın alabilir.
          </Alert>
          <p className="mt-4 text-sm text-muted">
            Birkaç saniye içinde yönlendirileceksiniz.
          </p>
          <Link href={isAuthenticated ? '/panel' : '/giris'} className="mt-5 block">
            <Button fullWidth>{isAuthenticated ? 'Panele git' : 'Giriş yap'}</Button>
          </Link>
        </>
      ) : (
        <>
          <Alert className="mt-5">
            {err?.message ?? 'Beklenmeyen bir hata oluştu.'}
            {err?.requestId && (
              <span className="mt-2 block text-xs opacity-70">İstek no: {err.requestId}</span>
            )}
          </Alert>
          <p className="mt-4 text-sm leading-relaxed text-muted">
            Doğrulama bağlantısı 24 saat geçerlidir ve yalnız bir kez
            kullanılabilir. Hesabınızı daha önce doğrulamış olabilirsiniz —
            giriş yapıp deneyin. Sorun sürerse destek ekibiyle iletişime geçin.
          </p>
          {/* Yeniden gönderme düğmesi BİLEREK YOK: `POST /auth/verify-email/resend`
              diye bir uç nokta bugün router'da tanımlı değil (router.go:96-102).
              Hiçbir şey yapmayan bir düğme, kullanıcıya olmayan bir çıkış yolu
              gösterir ve destek talebini gecikmeli olarak geri getirir. */}
          <BackLinks />
        </>
      )}
    </Card>
  );
}

function BackLinks() {
  return (
    <div className="mt-6 flex flex-col gap-3">
      <Link href="/giris" className="block">
        <Button variant="outline" fullWidth>Giriş ekranına dön</Button>
      </Link>
      <p className="text-center text-sm text-muted">
        Hesabınız yok mu?{' '}
        <Link href="/kayit" className="text-brand-400 underline-offset-4 hover:underline">
          Kayıt olun
        </Link>
      </p>
    </div>
  );
}
