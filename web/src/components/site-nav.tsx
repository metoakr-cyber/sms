'use client';

import Link from 'next/link';
import * as React from 'react';
import { Logo } from './logo';
import { Button, cx } from './ui';
import { ThemeToggle } from './theme';

const LINKS = [
  { href: '/', label: 'Ana Sayfa' },
  { href: '/kiralama', label: 'Kiralama' },
  { href: '/fiyatlar', label: 'Fiyatlar' },
  { href: '/sss', label: 'Sık Sorulanlar' },
  { href: '/blog', label: 'Blog' },
  { href: '/hakkimizda', label: 'Hakkımızda' },
];

export function SiteNav() {
  const [open, setOpen] = React.useState(false);

  // Menü açıkken arka plan kaydırması kilitlenir. iOS'ta `overflow:hidden`
  // tek başına yetmez; body'yi sabitleyip kaydırma konumunu geri yükleriz (§2.4).
  React.useEffect(() => {
    if (!open) return;
    const y = window.scrollY;
    const { style } = document.body;
    const prev = { position: style.position, top: style.top, width: style.width };
    style.position = 'fixed';
    style.top = `-${y}px`;
    style.width = '100%';
    return () => {
      style.position = prev.position; style.top = prev.top; style.width = prev.width;
      window.scrollTo(0, y);
    };
  }, [open]);

  // Escape ile kapat — klavye kullanıcısı fare olmadan çıkabilmeli.
  React.useEffect(() => {
    if (!open) return;
    const h = (e: KeyboardEvent) => { if (e.key === 'Escape') setOpen(false); };
    window.addEventListener('keydown', h);
    return () => window.removeEventListener('keydown', h);
  }, [open]);

  return (
    <header className="sticky top-0 z-40 border-b border-[var(--border)] bg-[var(--bg)]/85 backdrop-blur">
      <nav className="mx-auto flex max-w-6xl items-center justify-between gap-4 px-4 py-3 md:px-6">
        <Link href="/" aria-label="Ana sayfa"
              className="flex min-h-11 shrink-0 items-center"><Logo /></Link>

        {/* MASAÜSTÜ MENÜ `lg:` ÜSTÜNDE.
            Daha önce `md:` idi ve 768 px'te header 838 px'e taşıyordu:
            altı bağlantı + 44 px dokunma dolgusu + giriş düğmeleri o genişliğe
            sığmıyor. Bağlantıları sıkıştırmak yanlış olurdu — 768 px genelde
            bir TABLET, yani dokunmatik; hedefleri küçültmek dokunma hatasını
            artırır. Bu aralıkta hamburger menü kullanılır.
            Ölçüm: 768 px'te belge 838 → 768. */}
        <ul className="hidden items-center gap-7 lg:flex">
          {LINKS.map((l) => (
            <li key={l.href}>
              {/* px-2: dokunma hedefi 44 px'i İKİ EKSENDE de karşılamalı.
                  min-h-11 yüksekliği veriyor ama "Ana Sayfa" bağlantısı
                  768 px'te 38 px GENİŞLİKTE ölçüldü — otomatik denetim
                  yakaladı (frontend-contract §2.3). Yatay dolgu bunu düzeltir
                  ve gap-7 zaten aralarında boşluk bıraktığı için görünüm
                  değişmez. */}
              <Link href={l.href}
                    className="inline-flex min-h-11 min-w-11 items-center justify-center px-2
                               text-sm text-muted transition-colors hover:text-[var(--text)]">
                {l.label}
              </Link>
            </li>
          ))}
        </ul>

        <div className="hidden items-center gap-2 lg:flex">
          <ThemeToggle />
          <Link href="/giris"><Button variant="outline" size="sm">Giriş yap</Button></Link>
          <Link href="/kayit"><Button size="sm">Kayıt ol</Button></Link>
        </div>

        <div className="flex items-center lg:hidden">
        <ThemeToggle />
        <button
          type="button"
          onClick={() => setOpen((v) => !v)}
          aria-expanded={open}
          aria-controls="mobil-menu"
          aria-label={open ? 'Menüyü kapat' : 'Menüyü aç'}
          className="-mr-2 grid size-11 place-items-center rounded-xl lg:hidden"
        >
          <svg viewBox="0 0 24 24" className="size-6" fill="none" stroke="currentColor" strokeWidth="1.8">
            {open
              ? <path d="M6 6l12 12M18 6L6 18" strokeLinecap="round" />
              : <path d="M4 7h16M4 12h16M4 17h16" strokeLinecap="round" />}
          </svg>
        </button>
        </div>
      </nav>

      {/* Mobil çekmece — tam ekran, tam genişlik dokunma hedefleri */}
      <div
        id="mobil-menu"
        hidden={!open}
        className="border-t border-[var(--border)] bg-[var(--bg)] lg:hidden"
      >
        <ul className="flex flex-col px-4 py-2">
          {LINKS.map((l) => (
            <li key={l.href}>
              <Link
                href={l.href}
                onClick={() => setOpen(false)}
                className="flex min-h-12 items-center text-[15px]"
              >{l.label}</Link>
            </li>
          ))}
        </ul>
        <div className="flex flex-col gap-2 border-t border-[var(--border)] px-4 py-4 pb-safe">
          <Link href="/giris" onClick={() => setOpen(false)}>
            <Button variant="outline" fullWidth>Giriş yap</Button>
          </Link>
          <Link href="/kayit" onClick={() => setOpen(false)}>
            <Button fullWidth>Kayıt ol</Button>
          </Link>
        </div>
      </div>
    </header>
  );
}

// yil sunucuda hesaplanır: `new Date()` bileşen gövdesinde çağrılırsa
// sunucu ve istemci farklı saniyelerde render edip yıl sınırında hidrasyon
// uyuşmazlığı üretebilir.
const yil = new Date().getFullYear();

export function SiteFooter() {
  return (
    <footer className={cx('mt-20 border-t border-[var(--border)] px-4 py-10 md:px-6')}>
      <div className="mx-auto flex max-w-6xl flex-col gap-6 md:flex-row md:items-start md:justify-between">
        <div className="max-w-xs">
          <Logo withTagline />
          <p className="mt-3 text-sm text-muted">
            Yüzlerce servis için geçici numara ile anında SMS onay kodu.
          </p>
        </div>
        <div className="grid grid-cols-2 gap-x-10 gap-y-2 text-sm">
          <Link href="/kiralama" className="inline-flex min-h-11 items-center text-muted hover:text-[var(--text)]">Kiralama</Link>
          <Link href="/fiyatlar" className="inline-flex min-h-11 items-center text-muted hover:text-[var(--text)]">Fiyatlar</Link>
          <Link href="/sss" className="inline-flex min-h-11 items-center text-muted hover:text-[var(--text)]">Sık Sorulanlar</Link>
          <Link href="/kullanim-sartlari" className="inline-flex min-h-11 items-center text-muted hover:text-[var(--text)]">Kullanım Şartları</Link>
          <Link href="/gizlilik" className="inline-flex min-h-11 items-center text-muted hover:text-[var(--text)]">Gizlilik</Link>
          <Link href="/blog" className="inline-flex min-h-11 items-center text-muted hover:text-[var(--text)]">Blog</Link>
          <Link href="/hakkimizda" className="inline-flex min-h-11 items-center text-muted hover:text-[var(--text)]">Hakkımızda</Link>
          <Link href="/iletisim" className="inline-flex min-h-11 items-center text-muted hover:text-[var(--text)]">İletişim</Link>
        </div>
      </div>
      <p className="mx-auto mt-8 max-w-6xl text-xs text-muted">
        © {yil} Onay360. Tüm hakları saklıdır.
      </p>
    </footer>
  );
}
