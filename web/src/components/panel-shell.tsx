'use client';

/**
 * PanelShell — `/panel` kabuğu.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * NE DEĞİŞTİ: YAN SÜTUN KALKTI, ÜST HEADER GELDİ (kullanıcı kararı)
 * ══════════════════════════════════════════════════════════════════════════
 * Eski hâl: masaüstünde 240 px'lik sol yan sütun (logo + 6 gezinti + Yönetim +
 * ikincil blok + altta kullanıcı kutusu), mobilde ayrı bir üst logo şeridi ve
 * ayrı bir alt gezinti çubuğu. Yani gezinti listesi DOM'da iki kez, kullanıcı
 * kutusu bir kez, ikincil bağlantılar iki kez yazılıydı.
 *
 * Yeni hâl: her genişlikte tek bir üst başlık (`panel-header.tsx`). Bu dosya
 * artık yalnız KABUK: oturum kapısı, başlık, içerik, yardımcı şerit.
 *
 * 🔴 `/yonetim` bu karardan ETKİLENMEZ — orada yan sütun kullanıcı kararıyla
 * kalıyor (`admin-shell.tsx`, dokunulmadı).
 *
 * ══════════════════════════════════════════════════════════════════════════
 * YERLEŞİM
 * ══════════════════════════════════════════════════════════════════════════
 * Yan sütun gidince içerik alanı 240 px genişledi. Sayfaların kendi
 * `mx-auto max-w-*` kapları var (3xl–5xl arası); `main` bunlara ORTAK bir
 * `max-w-6xl` çerçeve verir ki başlık şeridiyle aynı sol/sağ kenardan
 * hizalansınlar — aksi hâlde 1440 px'te gezinti solda, içerik ortada durur.
 *
 * `pb-28`: mobil alt gezinti çubuğu `fixed`tir, akıştan çıkar. Alt dolgu
 * olmasaydı son kart çubuğun altında kalırdı. Ölçü çubuğun kendi yüksekliğinden
 * gelir: 56 (sekme) + 4 (üst dolgu) + 16+ (güvenli alan) + 1 (kenarlık) ≈ 77 px.
 */

import * as React from 'react';
import { usePathname, useRouter } from 'next/navigation';
import { useSession } from '@/hooks/useSession';
import { Spinner, Alert } from './ui';
import Link from 'next/link';
import { Logo } from './logo';
import { PanelHeader, PanelGezinti, PanelYardimciBaglantilar } from './panel-header';

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

  return (
    <div className="flex min-h-screen-safe flex-col lg:flex-row">
      {/*
        SOL YAN SÜTUN — `≥ lg`. Gezinti ekranın solunda (kullanıcı kararı).
        `< lg`'de bu sütun yoktur; gezinti sabit alt çubuğa döner
        (`PanelGezinti` içindeki tek `<nav>`, kabı CSS ile değişir).

        `sticky top-0 h-screen-safe`: uzun sayfalarda menü kaymaz. Yan sütunun
        kendi içi `overflow-y-auto`, çünkü 7 madde + ikincil bağlantılar kısa
        bir dizüstü ekranında taşabilir.
      */}
      {/*
        SOL YAN SÜTUN — `≥ lg`. Gezinti ekranın solunda (kullanıcı kararı).

        🔴 `contents` (`< lg`): yan sütun kutusu YOK OLUR, çocukları dış düzenin
        doğrudan çocuğu gibi davranır. Bunun sebebi tek: gezinti DOM'da BİR KEZ
        bulunsun. `hidden lg:flex` yapsaydım `< lg`'de nav'ı ikinci kez çizmem
        gerekirdi ve her sayfada İKİ `aria-current` olurdu — bu kabukta ölçülmüş
        eski hatanın ta kendisi. Tek nav, kabı CSS ile değişir.

        `sticky top-0 h-screen-safe`: uzun sayfalarda menü kaymaz. İçi
        `overflow-y-auto`, çünkü 7 madde kısa bir dizüstü ekranında taşabilir.
      */}
      <aside className="contents lg:sticky lg:top-0 lg:flex lg:h-screen-safe lg:w-60
                        lg:shrink-0 lg:flex-col lg:border-r lg:border-[var(--border)]">
        <div className="hidden px-5 py-4 lg:block">
          <Link href="/" aria-label="Ana sayfa" className="inline-flex min-h-11 items-center">
            <Logo className="h-7" />
          </Link>
        </div>
        {/* py-2: odak halkası `outline-offset: 2px` ile kutunun 4 px dışına
            çizilir; dikey dolgu olmadan `overflow-y-auto` onu kırpar. */}
        <div className="lg:min-h-0 lg:flex-1 lg:overflow-y-auto lg:px-3 lg:py-2">
          <PanelGezinti user={user} />
        </div>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <PanelHeader user={user} />


      {!user.emailVerified && (
        <div className="mx-auto w-full max-w-6xl px-4 pt-4 md:px-6">
          <Alert tone="warn">
            E-posta adresiniz doğrulanmamış. Numara satın alabilmek için
            e-postanıza gönderdiğimiz bağlantıya tıklayın.
          </Alert>
        </div>
      )}

      <main id="icerik"
            className="mx-auto w-full min-w-0 max-w-6xl flex-1 px-4 py-5 pb-28 md:px-6 md:py-7 md:pb-10">
        {children}
        <PanelYardimciBaglantilar />
        </main>
      </div>
    </div>
  );
}
