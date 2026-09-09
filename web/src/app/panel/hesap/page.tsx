'use client';

/**
 * Hesabım — müşteri ekranı.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * BU DOSYA ORTAK KATMANA BAĞLANDI (`components/yonetim` · §5.3)
 * ══════════════════════════════════════════════════════════════════════════
 *   · başlık bloğu            → `SayfaBasligi`
 *   · açık oturum listesi     → `VeriTablosu` (`< md` KART, `md:` üstü tablo)
 *   · oturum kapatma hatası   → `HataDurumu`
 *
 * ══════════════════════════════════════════════════════════════════════════
 * 🔴 ÜÇ DURUMDAN İKİSİ EKSİKTİ — kapatıldı
 * ══════════════════════════════════════════════════════════════════════════
 * Eski dosyada:
 *   · BOŞ DURUM YOKTU. `(sessions.data?.items ?? []).map(...)` boş dizide
 *     hiçbir şey çizmiyordu; kullanıcı "Açık oturumlar" başlığının altında
 *     BOŞLUK görüyordu — yükleniyor mu, bozuk mu, gerçekten boş mu belirsiz.
 *   · HATA `requestId` TAŞIMIYORDU: `<Alert>Oturum listesi yüklenemedi.</Alert>`.
 *     Destek ekibinin isteyeceği tek şey istek numarasıdır ve ekranda yoktu
 *     (CLAUDE.md #12 · §10 "hatada requestId"). Oturum KAPATMA hatası da
 *     yalnız `e.message` gösteriyordu.
 * `VeriTablosu` üçünü de ZORUNLU prop yapar; atlanamaz.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * 🔴 "YORUMLARIM" KARTI KALDIRILDI — kabuk değişti, gerekçesi düştü
 * ══════════════════════════════════════════════════════════════════════════
 * Kartın kendi yorumu şunu diyordu: *"Yorumlarım ALT MENÜYE eklenmedi: orada
 * zaten 6 sekme var ve 320px'te yedincisi her sekmeyi 44px dokunma hedefinin
 * altına düşürürdü."* Bu gerekçe artık GEÇERSİZ: yan sütun/alt çubuk kararı
 * değişti ve `/panel/yorumlarim` yeni kabukta İKİ yerden erişilebilir —
 * (1) profil menüsü (ikonlu, her genişlikte), (2) her sayfanın altındaki
 * `nav[aria-label="Yardımcı bağlantılar"]` şeridi, düz `<a>` olarak.
 * Yani bu kart ÜÇÜNCÜ kopyaydı ve aynı bağlantı bu sayfanın kendi altında
 * zaten bir kez daha duruyordu. Tek bağlantı için açılan bir `Card`,
 * `craft-floor.md:25`'in "kart tembel kaptır" bulgusunun ders kitabı örneği.
 * Erişim KAYBOLMADI; tekrar kalktı.
 *
 * HAREKET: yok. Hesap sayfası bir okuma ekranıdır; tek geçiş `Button`'un
 * `:active` basma geri bildirimidir ve katmandan gelir (§4.2).
 */

import * as React from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { apiFetch } from '@/lib/api';
import { formatDateTime } from '@/lib/format';
import { useSession } from '@/hooks/useSession';
import { Card, Button, Badge, Alert, Empty, cx } from '@/components/ui';
import {
  SayfaBasligi,
  VeriTablosu,
  HataDurumu,
  apiHatasi,
} from '@/components/yonetim';
import type { Sutun } from '@/components/yonetim';
import type { Session } from '@/lib/types';

export default function AccountPage() {
  const { user } = useSession();
  const qc = useQueryClient();
  const [basari, setBasari] = React.useState('');

  const sessions = useQuery({
    queryKey: ['sessions'],
    queryFn: () => apiFetch<{ items: Session[] }>('/me/sessions'),
  });

  const revoke = useMutation({
    mutationFn: (id: string) =>
      apiFetch(`/me/sessions/${encodeURIComponent(id)}`, { method: 'DELETE' }),
    // MUTASYON OTOMATİK TEKRARLANMAZ (değişmez #16).
    retry: false,
    onSuccess: () => {
      setBasari('Oturum kapatıldı.');
      qc.invalidateQueries({ queryKey: ['sessions'] });
    },
    onError: () => setBasari(''),
  });

  const kapatmaHatasi = apiHatasi(revoke.error);

  /*
   * SÜTUN TANIMI = TEK VERİ KAYNAĞI (§6.1 kural 2). Mobil kart bundan türer.
   *
   * ÖNCELİK: IP `lg:`ye alındı. 768px'te "Cihaz" sütunu uzun bir user-agent
   * dizesi taşır ve nowrap dayatan iki hücre (tarih, düğme) yanında IP için
   * yer kalmıyor. IP mobil kartta ve `lg:` üstünde duruyor — kaybolmuyor.
   */
  const sutunlar: ReadonlyArray<Sutun<Session>> = [
    {
      anahtar: 'cihaz',
      baslik: 'Cihaz',
      mobilRol: 'baslik',
      hucre: (s) => (
        // `break-anywhere`: user-agent dizesi boşluksuz uzun parçalar içerir
        // ve 320px'te yatay kaydırma üretirdi. `truncate` YOK — kısaltma
        // `nowrap` demektir ve dar tabloda taşma üretir.
        <div className="flex max-w-[26rem] flex-wrap items-center gap-2">
          <span className="break-anywhere font-medium">
            {s.userAgent || 'Bilinmeyen cihaz'}
          </span>
          {/* "Bu cihaz" bir DURUM değil, bir "buradasınız" işaretidir —
              bu yüzden `DurumRozeti` değil düz `Badge`. */}
          {s.current && <Badge tone="brand">Bu cihaz</Badge>}
        </div>
      ),
    },
    {
      anahtar: 'sonGorulme',
      baslik: 'Son görülme',
      hizala: 'sag',
      // Tarih de sayıdır: `tabular-nums` olmadan alt alta gelen saatler kayar
      // (§3.5). Eskiden bu metin `text-xs` idi ve hizasızdı.
      sayisal: true,
      hucre: (s) => formatDateTime(s.lastSeenAt),
    },
    {
      anahtar: 'ip',
      baslik: 'IP',
      oncelik: 3,
      hizala: 'sag',
      // Monospace burada MEŞRU (§9.2): kimlik dizgisi karakter karakter
      // okunabilsin diye, "teknik dursun" diye değil.
      hucre: (s) => (s.ip ? <span className="font-mono text-sm">{s.ip}</span> : '—'),
    },
    {
      anahtar: 'islem',
      baslik: 'İşlem',
      basligiGizle: true,
      hizala: 'sag',
      mobilRol: 'eylem',
      /*
       * ONAY ADIMI EKLENMEDİ. §6.4 modalı yıkıcı/geri alınamaz işleme ayırır;
       * bir oturumu kapatmak GERİ ALINABİLİR (o cihaz yeniden giriş yapar) ve
       * güvenlik gerekçesiyle HIZLI olmalıdır — tanımadığınız bir cihazı
       * görünce araya bir diyalog girmesi yardım etmez. Bugünkü davranış
       * korundu: tek tıkla kapanır.
       *
       * 🔴 AÇIK KUSUR (katmanda, burada değil): `VeriTablosu` kart eylemini
       * `{kartEylem && <div className="mt-4">…</div>}` ile çizer — koşul
       * SÜTUNUN varlığına bakar, hücrenin DEĞERİNE değil. Bu yüzden "bu cihaz"
       * satırının mobil kartında 16px boş bir blok kalıyor (ölçüldü: son çocuk
       * `.mt-4`, yükseklik 0, içerik boş). Tek karttaki 16px için ekrana sahte
       * bir metin uydurmadım; doğru düzeltme `veri-tablosu.tsx` içinde hücre
       * sonucunu `null` ise bloğu hiç çizmemektir. O dosya bu turda
       * DEĞİŞTİRİLMEYECEKLER listesinde, rapora yazıldı.
       */
      hucre: (s, sunum) =>
        s.current ? null : (
          <Button
            variant="outline"
            size="sm"
            fullWidth={sunum === 'kart'}
            loading={revoke.isPending && revoke.variables === s.id}
            onClick={() => revoke.mutate(s.id)}
          >
            Kapat
          </Button>
        ),
    },
  ];

  return (
    <div className="mx-auto flex max-w-4xl flex-col gap-6">
      <SayfaBasligi baslik="Hesabım" />

      <Card className="flex flex-col gap-6">
        {/* `h2` her yerde aynı boyutta: `text-lg font-semibold` (§3.3). */}
        <h2 className="text-lg font-semibold">Bilgiler</h2>
        {/*
          🔴 ÇİFT ÇİZGİ DÜZELTİLDİ. Eski dosyada üç satır `border-b` taşıyordu
          ve dördüncü blok ayrıca `border-t` yazıyordu: üçüncü satırın ALT
          çizgisi ile dördüncünün ÜST çizgisi arasında yalnız `gap` kalıyor,
          yani listenin ortasında ÇİFT AYIRICI oluşuyordu. Dördüncü satır artık
          aynı `Satir` bileşenidir; ayırıcı tek kaynaktan (`border-b` +
          `last:border-0`) gelir ve son satırda hiç çizilmez.
        */}
        <dl className="flex flex-col gap-4">
          <Satir label="Kullanıcı adı">{user?.username ?? '—'}</Satir>
          <Satir label="E-posta">{user?.email ?? '—'}</Satir>
          <Satir label="Üyelik" sayisal>{formatDateTime(user?.createdAt)}</Satir>
          <Satir label="Doğrulama">
            {user?.emailVerified
              ? <Badge tone="ok">E-posta doğrulandı</Badge>
              : <Badge tone="warn">Doğrulanmadı</Badge>}
          </Satir>
        </dl>
      </Card>

      <Card className="flex flex-col gap-6">
        <div>
          <h2 className="text-lg font-semibold">Açık oturumlar</h2>
          {/* 70ch: düz metin satır uzunluğu 65-75ch bandında kalır (§3.4). */}
          <p className="mt-2 max-w-[70ch] text-sm text-muted">
            Tanımadığınız bir cihaz görürseniz oturumu kapatın ve şifrenizi değiştirin.
          </p>
        </div>

        {/* Başarı kutusu BİR İŞLEMİN SONUCUNDA belirir — `role="alert"` burada
            DOĞRUDUR ve `Alert`'in varsayılanıdır (§7.4). */}
        {basari && <Alert tone="ok">{basari}</Alert>}
        {kapatmaHatasi && <HataDurumu hata={kapatmaHatasi} />}

        <VeriTablosu
          baslik="Açık oturumlar"
          sutunlar={sutunlar}
          satirlar={sessions.data?.items}
          satirAnahtari={(s) => s.id}
          yukleniyor={sessions.isLoading}
          hata={apiHatasi(sessions.error)}
          iskeletSatir={2}
          bos={
            /* Bu liste pratikte hiç boş olmaz (bu sayfayı görüyorsanız en az
               bir oturumunuz vardır), ama boş durum ZORUNLUDUR: bir sunucu
               sürümü boş dizi döndürürse ekran "bir şey ters gitti" demek
               yerine sessiz kalmamalı. */
            <Empty
              title="Açık oturum bulunamadı"
              hint="Bu beklenmedik bir durum. Sayfayı yenileyin; sürerse destek talebi açın."
            />
          }
        />
      </Card>
    </div>
  );
}

/**
 * Etiket-değer satırı.
 *
 * Ortak `KeyValueList` bileşeni HENÜZ YOK (`tasarim-sistemi.md §5.3` P1-10,
 * yazılmamış) — bu yüzden yerel kalıyor. Yazıldığında bu yardımcı ve
 * `/panel` altındaki benzerleri oraya taşınır.
 */
function Satir({
  label, children, sayisal,
}: { label: string; children: React.ReactNode; sayisal?: boolean }) {
  return (
    <div className="flex flex-wrap items-center justify-between gap-3 border-b
                    border-[var(--border)] pb-4 last:border-0 last:pb-0">
      <dt className="text-sm text-muted">{label}</dt>
      <dd className={cx('break-anywhere text-sm font-medium', sayisal && 'tabular-nums')}>
        {children}
      </dd>
    </div>
  );
}
