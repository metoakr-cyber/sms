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
 * İÇERİK TAVANI `max-w-12xl` = 1920px (kullanıcı kararı, 10 Eylül 2026).
 *
 * Eski hâl: `main` `mx-auto max-w-6xl`, sayfalar ayrıca kendi `max-w-4xl/5xl`
 * kaplarını açıyordu. Üst başlık şeridi (`panel-header.tsx`) ise HİÇ
 * sınırlanmamıştı (`px-4 md:px-6`, `max-w` yok). Yani başlık kenardan
 * kenara, içerik ortada dar bir şerit hâlinde duruyordu; 1440 px'te ikisi
 * hizasızdı ve tabloların sütunları gereksiz sıkışıyordu.
 *
 * Yeni hâl: `main` yalnız `px-4 md:px-6` dolgusu ve 1920px'lik bir tavan
 * taşır. İçerik başlıkla AYNI sol/sağ kenardan hizalanır ve tablo, yaygın
 * masaüstü çözünürlüklerinin tamamında ekranın verdiği genişliği kullanır.
 * 🔴 Sayfa içindeki `max-w-[70ch]` KALIR: o bir yerleşim kabı değil, düz metin
 * satır uzunluğu sınırıdır (§3.4) — kaldırılırsa paragraflar okunmaz olur.
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
        <div className="w-full px-4 pt-4 md:px-6">
          <Alert tone="warn">
            E-posta adresiniz doğrulanmamış. Numara satın alabilmek için
            e-postanıza gönderdiğimiz bağlantıya tıklayın.
          </Alert>
        </div>
      )}

        {/*
          GENİŞLİK TAVANI `max-w-12xl` (120rem / 1920px) — kullanıcı kararı,
          10 Eylül 2026. Jeton `globals.css`'te tanımlı; Tailwind'in hazır
          ölçeği `7xl`de biter ve tanımsız bir `max-w-12xl` sessizce hiçbir şey
          yapmazdı.

          TEK YERDE DURUYOR. Sayfaların her biri kendi `mx-auto max-w-*` kabını
          açsaydı (eski hâl buydu: 2xl'den 6xl'e beş farklı değer) içerik
          genişliği ekrandan ekrana zıplardı — §10 bunu bir kusur sayıyor.
          Kabuk tek tavanı verir, sayfalar genişlik kabı AÇMAZ.
        */}
      <main id="icerik"
            className="mx-auto w-full min-w-0 max-w-12xl flex-1 px-4 py-5 pb-28
                       md:px-6 md:py-7 md:pb-10">
        {children}
        <PanelYardimciBaglantilar />
        </main>
      </div>
    </div>
  );
}
