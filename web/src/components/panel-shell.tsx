'use client';

import * as React from 'react';
import Link from 'next/link';
import { usePathname, useRouter } from 'next/navigation';
import { apiFetch } from '@/lib/api';
import { formatMoney } from '@/lib/format';
import { useSession, useClearSession } from '@/hooks/useSession';
import { Logo } from './logo';
import { Spinner, cx, Alert } from './ui';
import { ThemeToggle } from './theme';

type Item = { href: string; label: string; icon: React.ReactNode };

const I = (d: string) => (
  <svg viewBox="0 0 24 24" className="size-5 shrink-0" fill="none" stroke="currentColor"
       strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
    <path d={d} />
  </svg>
);

const NAV: Item[] = [
  { href: '/panel',            label: 'Özet',      icon: I('M4 13h6V4H4zM14 20h6v-9h-6zM4 20h6v-4H4zM14 8h6V4h-6z') },
  { href: '/panel/numara-al',  label: 'Numara al', icon: I('M12 5v14M5 12h14') },
  { href: '/panel/siparisler', label: 'Siparişler', icon: I('M9 5H7a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V7a2 2 0 0 0-2-2h-2M9 5a2 2 0 0 0 2 2h2a2 2 0 0 0 2-2M9 5a2 2 0 0 1 2-2h2a2 2 0 0 1 2 2') },
  { href: '/panel/cuzdan',     label: 'Cüzdan',    icon: I('M3 8h18v10a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2zM3 8V6a2 2 0 0 1 2-2h11M17 13h.01') },
  { href: '/panel/destek',     label: 'Destek',    icon: I('M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z') },
  { href: '/panel/hesap',      label: 'Hesap',     icon: I('M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2M12 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8') },
];

const ADMIN: Item = {
  href: '/yonetim', label: 'Yönetim',
  icon: I('M12 2l8 4v6c0 5-3.4 9.3-8 10-4.6-.7-8-5-8-10V6z'),
};

/**
 * Kenar çubuğunun İKİNCİL bloğu — mobil alt çubuğa GİRMEZ.
 *
 * Neden ayrı bir dizi: alt çubuk `repeat(items.length, 1fr)` ızgarasıyla çiziliyor
 * ve yönetici hesabında zaten 7 madde var. 320 px'lik bir telefonda 9 sütun =
 * sekme başına 35 px; dokunma hedefi için gereken 44 px'in altı. Üstelik 11 px
 * yazıyla "Kullanım Şartları" o genişliğe sığmaz.
 *
 * Bu maddeler mobilde KAYBOLMUYOR: `PanelAltBaglantilar` bileşeni onları içeriğin
 * sonunda çiziyor (aşağıya bakın). Yani masaüstünde kenar çubuğunda, mobilde
 * sayfa sonunda — her iki durumda da ulaşılabilir.
 */
const IKINCIL: Item[] = [
  { href: '/panel/yorumlarim', label: 'Yorumlarım',
    icon: I('M12 3l2.7 5.5 6 .9-4.3 4.2 1 6-5.4-2.8-5.4 2.8 1-6L5.3 9.4l6-.9z') },
  { href: '/kullanim-sartlari', label: 'Kullanım Şartları',
    icon: I('M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8zM14 2v6h6M8 13h8M8 17h5') },
  { href: '/gizlilik', label: 'Gizlilik',
    icon: I('M5 11h14v10H5zM8 11V7a4 4 0 1 1 8 0v4') },
];

export function PanelShell({ children }: { children: React.ReactNode }) {
  const { user, isLoading, unauthenticated } = useSession();
  const router = useRouter();
  const pathname = usePathname();

  // Yönlendirme EFFECT içinde yapılır. Render sırasında router.replace çağırmak
  // React'te uyarı üretir ve çift yönlendirmeye yol açar.
  React.useEffect(() => {
    if (unauthenticated) {
      const devam = encodeURIComponent(pathname);
      router.replace(`/giris?sebep=oturum&devam=${devam}`);
    }
  }, [unauthenticated, pathname, router]);

  if (isLoading || unauthenticated || !user) {
    return (
      <div className="grid min-h-screen-safe place-items-center">
        <Spinner className="size-7 text-brand-400" />
        <span className="sr-only">Yükleniyor</span>
      </div>
    );
  }

  const isAdmin = user.permissions.length > 0;
  const items = isAdmin ? [...NAV, ADMIN] : NAV;

  return (
    <div className="flex min-h-screen-safe flex-col md:flex-row">
      {/* ─── Masaüstü kenar çubuğu ─── */}
      <aside className="hidden w-60 shrink-0 border-r border-[var(--border)] md:flex md:flex-col">
        <div className="px-5 py-5">
          <Link href="/" aria-label="Ana sayfa" className="inline-flex min-h-11 items-center"><Logo /></Link>
        </div>
        <nav className="flex-1 px-3">
          <ul className="flex flex-col gap-1">
            {items.map((it) => <li key={it.href}><NavLink item={it} pathname={pathname} /></li>)}
          </ul>
          {/* Ayırıcı: üstteki blok "işini yaptığın yer", alttaki "ara sıra
              baktığın yer". Aynı listede olsalardı Kullanım Şartları,
              Numara al ile eşit ağırlıkta görünürdü. */}
          <hr className="my-3 border-[var(--border)]" />
          <ul className="flex flex-col gap-1">
            {IKINCIL.map((it) => <li key={it.href}><NavLink item={it} pathname={pathname} /></li>)}
          </ul>
        </nav>
        <UserBox />
      </aside>

      {/* ─── Mobil üst bar ─── */}
      <header className="sticky top-0 z-30 flex items-center justify-between gap-3
                         border-b border-[var(--border)] bg-[var(--bg)]/90 px-4 py-3
                         backdrop-blur md:hidden">
        <Link href="/" aria-label="Ana sayfa" className="inline-flex min-h-11 items-center"><Logo className="h-7" /></Link>
        <div className="flex items-center gap-1">
          <ThemeToggle />
          <Link href="/panel/cuzdan"
                className="raised flex min-h-11 items-center rounded-xl border px-3 text-sm font-semibold">
            {formatMoney(user.balance)}
          </Link>
        </div>
      </header>

      <div className="flex min-w-0 flex-1 flex-col">
        {!user.emailVerified && (
          <div className="px-4 pt-4 md:px-6">
            <Alert tone="warn">
              E-posta adresiniz doğrulanmamış. Numara satın alabilmek için
              e-postanıza gönderdiğimiz bağlantıya tıklayın.
            </Alert>
          </div>
        )}
        {/* pb-24: mobil alt navigasyonun altında içerik kalmasın */}
        <main id="icerik" className="min-w-0 flex-1 px-4 py-5 pb-24 md:px-6 md:py-7 md:pb-7">
          {children}

          {/* İKİNCİL bağlantıların MOBİL karşılığı. Kenar çubuğu `md:` altında
              gizli, alt çubuğa da sığmıyorlar (yukarıdaki gerekçe) — bu blok
              olmasaydı telefondaki kullanıcı Kullanım Şartları'na panelden
              HİÇ ulaşamazdı. Yasal metne ulaşılamaması kabul edilebilir değil. */}
          <nav className="mt-10 border-t border-[var(--border)] pt-4 md:hidden"
               aria-label="Yardımcı bağlantılar">
            <ul className="flex flex-wrap gap-x-5 gap-y-1">
              {IKINCIL.map((it) => (
                <li key={it.href}>
                  <Link href={it.href}
                        className="inline-flex min-h-11 items-center text-sm text-muted underline-offset-4 hover:underline">
                    {it.label}
                  </Link>
                </li>
              ))}
            </ul>
          </nav>
        </main>
      </div>

      {/* ─── Mobil alt navigasyon ─── */}
      <nav className="fixed inset-x-0 bottom-0 z-30 border-t border-[var(--border)]
                      bg-[var(--bg)]/95 backdrop-blur pb-safe md:hidden"
           aria-label="Ana gezinme">
        <ul className="grid" style={{ gridTemplateColumns: `repeat(${items.length}, minmax(0,1fr))` }}>
          {items.map((it) => {
            const active = it.href === '/panel' ? pathname === '/panel' : pathname.startsWith(it.href);
            return (
              <li key={it.href}>
                <Link href={it.href} aria-current={active ? 'page' : undefined}
                      className={cx('flex min-h-14 flex-col items-center justify-center gap-1 text-[11px]',
                                    active ? 'text-brand-400' : 'text-muted')}>
                  {it.icon}
                  <span>{it.label}</span>
                </Link>
              </li>
            );
          })}
        </ul>
      </nav>
    </div>
  );
}

function NavLink({ item, pathname }: { item: Item; pathname: string }) {
  const active = item.href === '/panel' ? pathname === '/panel' : pathname.startsWith(item.href);
  return (
    <Link href={item.href} aria-current={active ? 'page' : undefined}
          className={cx('flex min-h-11 items-center gap-3 rounded-xl px-3 text-sm transition-colors',
                        active ? 'bg-brand-500/12 font-medium text-brand-300'
                               : 'text-muted hover:bg-[var(--raised)] hover:text-[var(--text)]')}>
      {item.icon}{item.label}
    </Link>
  );
}

function UserBox() {
  const { user } = useSession();
  const clear = useClearSession();
  const router = useRouter();
  const [busy, setBusy] = React.useState(false);
  if (!user) return null;

  async function logout() {
    setBusy(true);
    try { await apiFetch('/auth/logout', { method: 'POST' }); }
    catch { /* çerez zaten geçersizse de arayüzü çıkışa götürürüz */ }
    finally { clear(); router.replace('/giris'); }
  }

  return (
    <div className="border-t border-[var(--border)] p-3">
      <div className="raised rounded-xl border px-3 py-2.5">
        <p className="truncate text-sm font-medium">{user.username}</p>
        <p className="mt-0.5 text-xs text-muted">Bakiye</p>
        <p className="text-base font-bold">{formatMoney(user.balance)}</p>
      </div>
      <div className="mt-2 flex items-center gap-1">
      <button type="button" onClick={logout} disabled={busy}
              className="flex min-h-11 w-full items-center gap-3 rounded-xl px-3 text-sm
                         text-muted transition-colors hover:bg-[var(--raised)] hover:text-[var(--text)]
                         disabled:opacity-50">
        {busy ? <Spinner /> : I('M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4M16 17l5-5-5-5M21 12H9')}
        Çıkış yap
      </button>
      <ThemeToggle />
      </div>
    </div>
  );
}
