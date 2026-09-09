'use client';

import * as React from 'react';
import Link from 'next/link';
import { usePathname, useRouter } from 'next/navigation';
import { useSession } from '@/hooks/useSession';
import { Logo } from './logo';
import { Spinner, Card, Button, Badge, cx } from './ui';
import { ThemeToggle } from './theme';

// Sekmeler mobilde yatay kaydırılır; sıra EN SIK KULLANILANDAN başlar,
// çünkü kaydırma çubuğunun sağında kalan sekmeler pratikte görülmez.
const NAV = [
  { href: '/yonetim',                  label: 'Genel bakış' },
  { href: '/yonetim/talepler',         label: 'Bakiye talepleri' },
  { href: '/yonetim/kullanicilar',     label: 'Kullanıcılar' },
  { href: '/yonetim/bakiye',           label: 'Bakiye düzeltme' },
  { href: '/yonetim/fiyatlar',         label: 'Fiyat kuralları' },
  { href: '/yonetim/odeme-yontemleri', label: 'Ödeme yöntemleri' },
  { href: '/yonetim/saglayicilar',     label: 'Sağlayıcılar' },
  { href: '/yonetim/destek',           label: 'Destek' },
  { href: '/yonetim/yorumlar',         label: 'Yorumlar' },
  { href: '/yonetim/denetim',          label: 'Denetim kaydı' },
];

export function AdminShell({ children }: { children: React.ReactNode }) {
  const { user, isLoading, unauthenticated } = useSession();
  const router = useRouter();
  const pathname = usePathname();

  React.useEffect(() => {
    if (unauthenticated) router.replace('/giris?sebep=oturum&devam=/yonetim');
  }, [unauthenticated, router]);

  if (isLoading || unauthenticated || !user) {
    return (
      <div className="grid min-h-screen-safe place-items-center">
        <Spinner className="size-7 text-brand-400" />
      </div>
    );
  }

  /*
   * İstemci tarafı izin kontrolü YALNIZCA ARAYÜZ İÇİNDİR, güvenlik sınırı DEĞİLDİR.
   * Gerçek karar sunucudadır: RequirePermission("users:write") ara katmanı
   * (transport/http/router.go). Bu blok kaldırılsa bile yetkisiz bir kullanıcı
   * hiçbir yönetim ucunu çağıramaz — burada yaptığımız tek şey, kullanamayacağı
   * bir ekranı ona göstermemek.
   */
  if (user.permissions.length === 0) {
    return (
      <div className="grid min-h-screen-safe place-items-center px-4">
        <Card className="max-w-sm text-center">
          <h1 className="text-lg font-semibold">Bu sayfaya erişiminiz yok</h1>
          <p className="mt-2 text-sm text-muted">
            Yönetim paneli yalnız yetkili hesaplara açıktır.
          </p>
          <Link href="/panel" className="mt-5 block">
            <Button variant="outline" fullWidth>Panele dön</Button>
          </Link>
        </Card>
      </div>
    );
  }

  return (
    <div className="flex min-h-screen-safe flex-col">
      <header className="sticky top-0 z-30 border-b border-[var(--border)]
                         bg-[var(--bg)]/90 backdrop-blur">
        <div className="mx-auto flex max-w-6xl items-center justify-between gap-3 px-4 py-3 md:px-6">
          <div className="flex min-w-0 items-center gap-3">
            <Link href="/panel" aria-label="Panel" className="inline-flex min-h-11 items-center"><Logo className="h-7" /></Link>
            <Badge tone="brand">Yönetim</Badge>
          </div>
          <div className="flex shrink-0 items-center gap-1">
            <ThemeToggle />
            <Link href="/panel">
              <Button variant="ghost" size="sm">Panele dön</Button>
            </Link>
          </div>
        </div>
        {/* Sekmeler mobilde yatay kaydırılır — 2-3 sekme için kabul edilebilir,
            tablo değil. Kaydırma çubuğu gizlenmez ki keşfedilebilir kalsın. */}
        <nav className="mx-auto max-w-6xl overflow-x-auto px-4 thin-scroll md:px-6">
          <ul className="flex gap-1">
            {NAV.map((n) => {
              const active = n.href === '/yonetim' ? pathname === '/yonetim' : pathname.startsWith(n.href);
              return (
                <li key={n.href}>
                  <Link href={n.href} aria-current={active ? 'page' : undefined}
                        className={cx('flex min-h-11 items-center whitespace-nowrap border-b-2 px-3 text-sm',
                                      active ? 'border-brand-500 font-medium text-brand-300'
                                             : 'border-transparent text-muted hover:text-[var(--text)]')}>
                    {n.label}
                  </Link>
                </li>
              );
            })}
          </ul>
        </nav>
      </header>

      <main id="icerik" className="mx-auto w-full max-w-6xl flex-1 px-4 py-6 pb-safe md:px-6 md:py-8">
        {children}
      </main>
    </div>
  );
}
