'use client';

/**
 * AdminShell — `/yonetim` kabuğu.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * NE DEĞİŞTİ: 10 SEKMELİK YATAY ŞERİT → BÖLÜMLÜ YAN SÜTUN
 * ══════════════════════════════════════════════════════════════════════════
 * Bu dosyanın ESKİ HALİ kendi yorumunda şunu yazıyordu: "Sekmeler mobilde
 * yatay kaydırılır — 2-3 sekme için kabul edilebilir." `NAV` dizisinde ise
 * 10 SEKME vardı. Kaydırma çubuğunun sağında kalan sekmeler telefonda
 * pratikte görülmez; panelin yarısı keşfedilemez durumdaydı. Aynı depoda
 * `panel-shell.tsx` DAHA AZ bölüm için yan menü kullanıyor — yani yönetim
 * paneli daha fazla bölümle daha zayıf desene sahipti
 * (tasarim-sistemi.md §11.5, `operate.md:56` "top bar + side nav"a izin verir).
 *
 * Gezinti listesi ARTIK BURADA DEĞİL: tek kaynağı `BolumGezinti`
 * (`components/yonetim/bolum-gezinti.tsx`). Bir bölüm eklemek için bu dosyaya
 * dokunulmaz.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * MOBİL KAP SEÇİMİ: ÇEKMECE — alt gezinti DEĞİL
 * ══════════════════════════════════════════════════════════════════════════
 * `panel-shell.tsx` mobilde alt çubuk kullanır ve kendi yorumunda sınırı
 * yazar: `repeat(items.length, 1fr)` ızgarası 320px'te 9 sütunda sekme başına
 * 35px verir — 44px dokunma hedefinin altı. Yönetimde 11 hedef (Genel bakış +
 * 10 bölüm) var; alt çubuk aritmetik olarak imkânsız. Bölüm başlıklarını da
 * (Para · Katalog · Destek · Sistem) taşıyamaz.
 *
 * Çekmece `Modal` ile açılır, elle yazılmaz: odak tuzağı, `Escape`, iOS
 * kaydırma kilidi ve kaydırma konumunun geri yüklenmesi orada ÇÖZÜLMÜŞ
 * durumda. İkinci bir uygulama, dördünden birini unutmakla biterdi.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * HAREKET
 * ══════════════════════════════════════════════════════════════════════════
 * Gezinti geçişi günde 100+ kez yaşanır → animasyon YOK (§4.2, "Asla").
 * Tek geçiş menü düğmesinin hover RENGİdir (`--sure-hizli`, 120ms) ve
 * `Button`'ın kendi basma geri bildirimi. `Modal`'ın kendisi de animasyonsuz
 * açılır; buraya animasyon EKLENMEDİ.
 */

import * as React from 'react';
import Link from 'next/link';
import { usePathname, useRouter } from 'next/navigation';
import { useSession } from '@/hooks/useSession';
import { Logo } from './logo';
import { Spinner, Card, Button, Badge } from './ui';
import { Modal } from './modal';
import { ThemeToggle } from './theme';
/*
 * Barrel (`@/components/yonetim`) yerine DOĞRUDAN import: kabuk her `/yonetim`
 * rotasında yüklenir; barrel üzerinden gelmek `VeriTablosu` gibi yalnız tek
 * ekranın kullandığı bileşenleri de kabuk parçasına bağlar.
 */
import { BolumGezinti } from './yonetim/bolum-gezinti';

export function AdminShell({ children }: { children: React.ReactNode }) {
  const { user, isLoading, unauthenticated } = useSession();
  const router = useRouter();
  const pathname = usePathname();
  const [menuAcik, setMenuAcik] = React.useState(false);

  React.useEffect(() => {
    if (unauthenticated) router.replace('/giris?sebep=oturum&devam=/yonetim');
  }, [unauthenticated, router]);

  // Rota değişince çekmece kapanır. Yoksa bir bölüme dokunan kullanıcı,
  // açtığı sayfanın üstünde duran menüyü elle kapatmak zorunda kalır.
  React.useEffect(() => {
    setMenuAcik(false);
  }, [pathname]);

  /*
   * Çekmece YALNIZ `lg:` altında açılabilir (düğme `lg:hidden`), ama ekran
   * açıkken genişleyebilir: tablet dikeyden yataya döndüğünde 1024px eşiği
   * aşılır ve yan sütun görünür hale gelir.
   * 🔴 EŞİK DÜĞMENİN KIRILMA NOKTASIYLA AYNI OLMAK ZORUNDA: yan sütun `lg`'de
   * açılırken bu etki 768'de kapatsaydı, 768-1023 arasında kullanıcı ne
   * çekmeceyi ne de yan sütunu görürdü — gezinti tamamen kaybolurdu. Kapatılmazsa kullanıcı, arkasında
   * zaten aynı menüyü taşıyan bir örtünün altında kalır.
   */
  React.useEffect(() => {
    if (!menuAcik || typeof window.matchMedia !== 'function') return;
    const mq = window.matchMedia('(min-width: 1024px)');
    if (mq.matches) {
      setMenuAcik(false);
      return;
    }
    const onChange = (e: MediaQueryListEvent) => {
      if (e.matches) setMenuAcik(false);
    };
    mq.addEventListener('change', onChange);
    return () => mq.removeEventListener('change', onChange);
  }, [menuAcik]);

  if (isLoading || unauthenticated || !user) {
    return (
      <div className="grid min-h-screen-safe place-items-center">
        <Spinner className="size-7 text-brand-400" />
        <span className="sr-only">Yükleniyor</span>
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
    <div className="flex min-h-screen-safe flex-col lg:flex-row">
      {/* ─── Masaüstü yan sütunu ───
          Yapışkanlık için ÜÇÜ BİRDEN gerekir: `sticky` + `top-0` + sabit
          yükseklik + `self-start`. `self-start` olmadan esnek öğe kabı doldurur
          ve `sticky` hiçbir zaman tetiklenmez — 800 satırlık bir denetim
          tablosunda gezinti yukarıda kaybolurdu.

          🔴 `h-screen-safe` BİLEREK `md:` ÖNEKSİZ. Ölçüldü: `h-screen-safe`
          globals.css'te düz bir `@layer utilities` bloğunda tanımlı, Tailwind
          v4'ün `@utility` direktifiyle değil — bu yüzden ondan VARYANT
          ÜRETİLMEZ ve `md:h-screen-safe` derlenmiş CSS'te HİÇ OLUŞMAZ
          (tailwind derlemesi çıktısında arandı: 0 eşleşme, tsc bunu yakalamaz).
          Öneke gerek de yok: `< md` altında öğe `hidden`, yüksekliğin etkisi
          zaten sıfırdır. */}
      <aside
        className="hidden h-screen-safe w-60 shrink-0 flex-col border-r border-[var(--border)]
                   lg:sticky lg:top-0 lg:flex lg:self-start"
      >
        <div className="flex items-center gap-3 px-5 py-5">
          <Link href="/panel" aria-label="Panel" className="inline-flex min-h-11 items-center">
            <Logo className="h-7" />
          </Link>
          {/*
            Hangi yüzeyde olduğumuzu söyleyen tek işaret.

            🔴 `tone="brand"` DEĞİL — ÖLÇÜM SONUCU. Eski hal `<Badge tone="brand">`
            kullanıyordu; `Badge`'in `brand` tonu `text-brand-300` taşır ve AÇIK
            temada bu rozet zemininde **1,85:1** ölçüldü (eşik 4.5, §7.1) — yani
            gündüz temasında okunmuyor. `bolum-gezinti.tsx` aynı tuzağı etkin
            gezinti satırı için zaten belgelemiş. `neutral` tonu aynı ölçümde
            iki temada da eşiği geçiyor.

            Bu, §11.6'daki "brand tonu beş anlam taşıyor" açık kararını
            ÇÖZMEZ; yalnız o beş kullanımdan birini eksiltir ve yeni bir ton
            uydurmaz. `Badge` `className` kabul etmediği için (§5.2) elle
            boyanmış bir kopya yazmak tek alternatifti — o da ekranlar arası
            bileşen dilini bozardı (§9.2).
          */}
          <Badge tone="neutral">Yönetim</Badge>
        </div>

        {/* İç kaydırma alanı → `.thin-scroll` (§8, tarayıcı yüzeyleri). */}
        <div className="thin-scroll min-h-0 flex-1 overflow-y-auto px-3 py-2 pb-6">
          <BolumGezinti />
        </div>

        <div className="flex items-center gap-2 border-t border-[var(--border)] p-3">
          <Link href="/panel" className="min-w-0 flex-1">
            <Button variant="outline" size="sm" fullWidth>Panele dön</Button>
          </Link>
          <ThemeToggle />
        </div>
      </aside>

      {/* `min-w-0`: içindeki geniş tablo esnek öğeyi şişirip yatay kaydırma
          üretmesin (mobilde yatay kaydırma yasak, CLAUDE.md #17). */}
      <div className="flex min-w-0 flex-1 flex-col">
        {/*
          ═══════════════════════════════════════════════════════════════════
          ÜST ŞERİT — MÜŞTERİ PANELİYLE AYNI (kullanıcı kararı, 10 Eylül 2026)
          ═══════════════════════════════════════════════════════════════════
          Eski hâl `md:hidden` idi: masaüstünde yönetimde HİÇ üst şerit yoktu,
          müşteri panelinde ise her genişlikte vardı. İki yüzey arasında geçen
          aynı kişi için üst şeridin bir yüzeyde belirip diğerinde kaybolması,
          "hangi paneldeyim" sorusunu her geçişte yeniden sordurur.

          Kutu ölçüleri `panel-header.tsx` ile BİREBİR: `sticky top-0 z-20`,
          `border-b`, düz `bg-[var(--bg)]` ve `px-4 py-2 md:px-6`. Eskiden
          `bg-[var(--bg)]/90 + backdrop-blur + py-3` idi; saydamlık ve bulanıklık
          panelde YOK, iki şerit yan yana konduğunda fark görünüyordu.

          Menü düğmesi `< lg`'de: yan sütun artık `lg`'de açılıyor (panelle aynı
          eşik), yani 768-1023 px arasında da çekmeceye ihtiyaç var.
        */}
        <header className="sticky top-0 z-20 border-b border-[var(--border)] bg-[var(--bg)]">
          <div className="flex items-center gap-2 px-4 py-2 md:px-6">
            <button
              type="button"
              onClick={() => setMenuAcik(true)}
              aria-haspopup="dialog"
              aria-expanded={menuAcik}
              className="flex min-h-11 items-center gap-2 rounded-xl px-3 text-sm font-medium
                         [transition-property:background-color]
                         [transition-duration:var(--sure-hizli)]
                         hover:bg-[var(--raised)] lg:hidden"
            >
              {/* Emoji/unicode DEĞİL, SVG (§9.2). Modal'ın kapatma ikonuyla aynı
                  çizgi kalınlığı (2) — açan ve kapatan denetim aynı dili konuşur. */}
              <svg viewBox="0 0 24 24" className="size-5" fill="none" stroke="currentColor"
                   strokeWidth="2" strokeLinecap="round" aria-hidden>
                <path d="M4 7h16M4 12h16M4 17h16" />
              </svg>
              Bölümler
            </button>

            {/* Logo YALNIZ `< lg`: `≥ lg`'de yan sütun zaten taşıyor ve ikinci
                kopya ekran okuyucuda iki "Panel" bağlantısı demektir. Panel
                başlığındaki `lg:hidden` logosuyla aynı kural. */}
            <Link href="/panel" aria-label="Panel"
                  className="inline-flex min-h-11 shrink-0 items-center lg:hidden">
              <Logo className="h-7" />
            </Link>

            <div className="ml-auto flex shrink-0 items-center gap-2">
              {/* Yan sütundakiyle aynı ton — gerekçe yukarıda. */}
              <Badge tone="neutral">Yönetim</Badge>
              <ThemeToggle />
            </div>
          </div>
        </header>

        {/*
          İÇERİK TAVANI `max-w-12xl` (1920px) — panelle AYNI.
          Panel `main`i 10 Eylül'de `max-w-6xl`den kurtarıldı; yönetim geride
          kalmıştı ve aynı depoda iki farklı içerik genişliği vardı. Yönetim
          tabloları (8 sütuna kadar) genişlikten en çok yararlanan yüzeydir.
          Dolgu panelle birebir: `px-4 md:px-6`.
        */}
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
              className="mx-auto w-full min-w-0 max-w-12xl flex-1 px-4 py-6 pb-safe
                         md:px-6 md:py-8">
          {children}
        </main>
      </div>

      {/* ─── Mobil çekmece ───
          `md:` üstünde hiç açılmaz; yukarıdaki matchMedia etkisi eşiği
          geçildiğinde kapatır. */}
      <Modal open={menuAcik} onClose={() => setMenuAcik(false)} title="Yönetim bölümleri">
        <BolumGezinti />
        <div className="mt-6 border-t border-[var(--border)] pt-4">
          <Link href="/panel" className="block">
            <Button variant="outline" fullWidth>Panele dön</Button>
          </Link>
        </div>
      </Modal>
    </div>
  );
}
