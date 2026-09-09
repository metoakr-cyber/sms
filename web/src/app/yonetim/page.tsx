'use client';

/**
 * Yönetim — Genel bakış.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * BU EKRAN NEDEN YENİDEN YAZILDI
 * ══════════════════════════════════════════════════════════════════════════
 * Ölçülen eski hal: 0 API çağrısı, 10 bölümün yalnız 1'ine bağlantı, hiçbir
 * sayı, ve tek gerçek içerik olarak 13 HAM İNGİLİZCE izin kodu
 * (`deposits:approve`, `pricing:write`…) — CLAUDE.md "kullanıcıya görünen her
 * şey Türkçe" kuralının doğrudan ihlali. Ekran bir yönlendirme tahtasıydı ve
 * yönlendirmeyi de yapamıyordu.
 *
 * Sorulan soru: YÖNETİCİ SABAH İLK NEYE BAKAR? Cevap gezinti değil, KUYRUK:
 * kaç bakiye talebi onay bekliyor, kaç destek talebi yanıt bekliyor, kaç yorum
 * moderasyon bekliyor. Gezintiyi artık kabuk yapıyor (`admin-shell.tsx` →
 * `BolumGezinti`), bu ekran da yalnız İŞ taşıyor.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * 🔴 UYDURMA SAYI YOK — HER KARO GERÇEK BİR UCA DAYANIR
 * ══════════════════════════════════════════════════════════════════════════
 * Üç karonun üçü de `internal/transport/http/router.go`'da DOĞRULANMIŞ
 * uçlardan besleniyor ve sayı sunucunun `total` alanından geliyor:
 *
 *   GET /admin/deposits?status=PENDING   (router.go:417, izin deposits:read)
 *   GET /admin/tickets?pending=true      (router.go:435, izin tickets:read)
 *   GET /admin/reviews?status=PENDING    (router.go:451, izin reviews:read)
 *
 * 🔴 `/admin/orders` DİYE BİR UÇ YOKTUR — sipariş sayısı gösteren bir karo
 * bilerek KONULMADI. Sayı gösteremediğimiz kuyruk için karo yazılmaz;
 * boş bir ekran, çalışıyormuş gibi görünen bozuk bir ekrandan iyidir.
 * (tasarim-sistemi.md §5.3 "StatCard eklemeyin" kararının gerekçesi
 * "veri olmadan kart yapılmaz" idi; burada veri VAR ve doğrulandı.)
 *
 * `limit=1`: bize satır değil `total` lazım; sunucu tarafı sayaç ucu yok, en
 * ucuz doğru sorgu tek satırlık sayfadır.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * İZİNLE KAPILANMA
 * ══════════════════════════════════════════════════════════════════════════
 * Bir karo YALNIZ kullanıcının o izni varsa sorgulanır. İzin yokken sorgu
 * atmak, ekranı açar açmaz üç adet 403 üretir ve operatöre düzeltemeyeceği
 * bir hata gösterir. Yetki listesi (aşağıda) hangi kuyruğun neden görünmediğini
 * açıklar.
 */

import * as React from 'react';
import Link from 'next/link';
import { useQueries } from '@tanstack/react-query';
import { apiFetch } from '@/lib/api';
import { useSession } from '@/hooks/useSession';
import { Card, Empty, Skeleton } from '@/components/ui';
import { HataDurumu, SayfaBasligi, apiHatasi } from '@/components/yonetim';

/** Yönetim liste uçlarının ortak zarfı; bize yalnız `total` lazım. */
interface SayimYaniti {
  total: number;
}

interface Kuyruk {
  anahtar: string;
  /** Sunucunun bu uç için istediği izin kodu (router.go). */
  izin: string;
  baslik: string;
  /** Sayı 0 iken gösterilen satır — "boş" burada iyi haberdir. */
  bosMetin: string;
  /** Sayı > 0 iken gösterilen satır; karonun tamamı zaten bağlantıdır. */
  eylemMetni: string;
  yol: string;
  sorgu: string;
}

const KUYRUKLAR: readonly Kuyruk[] = [
  {
    anahtar: 'bakiye-talepleri',
    izin: 'deposits:read',
    baslik: 'Onay bekleyen bakiye talebi',
    bosMetin: 'Onay bekleyen talep yok.',
    eylemMetni: 'Bakiye taleplerini aç',
    yol: '/yonetim/talepler',
    sorgu: '/admin/deposits?status=PENDING&limit=1',
  },
  {
    anahtar: 'destek-talepleri',
    izin: 'tickets:read',
    // `pending=true` tek bir durum DEĞİLDİR: sunucu OPEN + USER_REPLIED
    // birleşimini döner (ticket.go:145-154). Başlık bu yüzden "açık" değil
    // "yanıt bekleyen" der — ölçüt personel müdahalesidir.
    baslik: 'Yanıt bekleyen destek talebi',
    bosMetin: 'Yanıt bekleyen talep yok.',
    eylemMetni: 'Destek taleplerini aç',
    yol: '/yonetim/destek',
    sorgu: '/admin/tickets?pending=true&limit=1',
  },
  {
    anahtar: 'yorumlar',
    izin: 'reviews:read',
    baslik: 'Onay bekleyen yorum',
    bosMetin: 'Onay bekleyen yorum yok.',
    eylemMetni: 'Yorumları aç',
    yol: '/yonetim/yorumlar',
    sorgu: '/admin/reviews?status=PENDING&limit=1',
  },
];

/**
 * İzin kodu → Türkçe açıklama.
 *
 * Metinler UYDURULMADI: `api/migrations/00002_seed_rbac.sql` ve
 * `00013_reviews.sql` içindeki `permissions.description` sütunundan birebir
 * alındı. Sunucu bu açıklamayı bugün API'de döndürmüyor (`/me` yalnız kod
 * dizisi verir), o yüzden eşleme istemcide duruyor.
 *
 * Sıra kasıtlıdır: para → katalog → destek → sistem, yani yan sütundaki bölüm
 * sırasıyla aynı. Sunucudan gelen sırayla yazsaydık liste her hesapta farklı
 * okunurdu.
 */
const IZIN_ETIKETLERI: Record<string, string> = {
  'deposits:read': 'Bakiye yükleme taleplerini görüntüleme',
  'deposits:approve': 'Bakiye yükleme taleplerini onaylama/reddetme',
  'pricing:read': 'Fiyat kurallarını görüntüleme',
  'pricing:write': 'Fiyat kurallarını değiştirme',
  'providers:read': 'Sağlayıcıları görüntüleme',
  'providers:write': 'Sağlayıcı ve eşleştirme yönetimi',
  'tickets:read': 'Tüm destek taleplerini görüntüleme',
  'tickets:reply': 'Destek taleplerine personel olarak yanıt verme',
  'reviews:read': 'Tüm müşteri yorumlarını görüntüleme',
  'reviews:moderate': 'Müşteri yorumlarını onaylama/reddetme',
  'users:read': 'Kullanıcıları görüntüleme',
  'users:write': 'Kullanıcı bilgilerini ve bakiyesini değiştirme',
  'orders:read_all': 'Tüm siparişleri görüntüleme',
  'audit:read': 'Denetim kaydını görüntüleme',
};

const IZIN_SIRASI = Object.keys(IZIN_ETIKETLERI);

export default function AdminHome() {
  const { user } = useSession();

  /*
   * `user` burada HER ZAMAN doludur: `AdminShell` kullanıcı yüklenene kadar
   * children'ı render etmez. Yine de `?? []` yazılıyor — kabuk değişirse bu
   * ekran çökmek yerine boş kuyrukla açılır.
   */
  const izinler = React.useMemo(() => user?.permissions ?? [], [user]);

  const kuyruklar = React.useMemo(
    () => KUYRUKLAR.filter((k) => izinler.includes(k.izin)),
    [izinler],
  );

  /*
   * `useQueries`: kuyruk sayısı izne göre değiştiği için sabit sayıda
   * `useQuery` çağrısı yazılamaz (koşullu hook kuralı). Yeniden deneme,
   * `staleTime` ve odakta tazeleme `components/providers.tsx`'teki genel
   * varsayılanlardan gelir; burada tekrar edilmez.
   */
  const sonuclar = useQueries({
    queries: kuyruklar.map((k) => ({
      queryKey: ['yonetim', 'ozet', k.anahtar] as const,
      queryFn: () => apiFetch<SayimYaniti>(k.sorgu),
    })),
  });

  // `noUncheckedIndexedAccess` açık: eşleştirme flatMap ile yapılır, `!` ile değil.
  const satirlar = kuyruklar.flatMap((kuyruk, i) => {
    const sonuc = sonuclar[i];
    return sonuc ? [{ kuyruk, sonuc }] : [];
  });

  const hatalar = satirlar.flatMap(({ kuyruk, sonuc }) => {
    const hata = apiHatasi(sonuc.error);
    return hata ? [{ kuyruk, hata }] : [];
  });

  const sahipOlunanIzinler = [
    ...IZIN_SIRASI.filter((kod) => izinler.includes(kod)),
    // Bilinmeyen kod GİZLENMEZ: eşlemede olmayan bir izin, hesabın gerçekten
    // sahip olduğu bir yetkidir. Ham kodu göstermek, yetkiyi yok saymaktan iyi.
    ...izinler.filter((kod) => !(kod in IZIN_ETIKETLERI)),
  ];

  return (
    // gap-8 (32px): bloklar arası "cömert ayrım". Blok İÇİ boşluklar 8-16px,
    // yani ayrımın en fazla yarısı (§2.3 ritim kuralı).
    <div className="flex flex-col gap-8">
      <SayfaBasligi
        baslik="Genel bakış"
        aciklama="Bekleyen işler burada toplanır. Bir sayıya dokunun, doğrudan o kuyruğa gidin."
      />

      <section className="flex flex-col gap-4">
        {/* h2 ölçeği her ekranda aynı: text-lg font-semibold (§3.3). */}
        <h2 className="text-lg font-semibold">Bekleyen işler</h2>

        {satirlar.length === 0 ? (
          <Card>
            {/* Boş durum ÖĞRETİR, "burada bir şey yok" demez (§5.1). */}
            <Empty
              title="Bekleyen iş kuyruğu görünmüyor"
              hint="Bakiye talepleri, destek talepleri ve yorumlar için görüntüleme yetkiniz yok. Hesabınızın hangi işleri yapabildiğini aşağıda görebilirsiniz."
            />
          </Card>
        ) : (
          <ul className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {satirlar.map(({ kuyruk, sonuc }) => (
              <li key={kuyruk.anahtar}>
                <KuyrukKarosu
                  kuyruk={kuyruk}
                  sayi={sonuc.data?.total}
                  yukleniyor={sonuc.isPending}
                  hatali={sonuc.isError}
                />
              </li>
            ))}
          </ul>
        )}

        {/* Hata ÜÇÜNCÜ durumdur ve atlanmaz. Karo yine bağlantı kalır (ekrana
            gidilebilir), sayının neden gelmediği ve `requestId` burada durur. */}
        {hatalar.length > 0 && (
          <ul className="flex flex-col gap-4">
            {hatalar.map(({ kuyruk, hata }) => (
              <li key={kuyruk.anahtar} className="flex flex-col gap-2">
                <p className="text-sm font-medium">{kuyruk.baslik}</p>
                <HataDurumu hata={hata} />
              </li>
            ))}
          </ul>
        )}
      </section>

      <section className="flex flex-col gap-4">
        <h2 className="text-lg font-semibold">Yetkileriniz</h2>
        <Card>
          <p className="max-w-[70ch] text-sm text-muted">
            Hesabınızın yönetim panelinde yapabildiği işler. Sunucu her isteği
            ayrıca denetler; burada yazmayan bir işi panelden de yapamazsınız.
          </p>
          {/*
            🔴 ROZET KULLANILMADI. `Badge` `whitespace-nowrap` taşır ve bunlar
            etiket değil CÜMLE; 320px'te yatay taşma üretirdi (§5.2, ölçülmüş
            ihlal). Eski hal tam olarak bunu yapıyordu — üstelik ham kodlarla.
          */}
          <ul className="mt-4 grid gap-2 sm:grid-cols-2">
            {sahipOlunanIzinler.map((kod) => (
              <li key={kod} className="text-sm">
                {IZIN_ETIKETLERI[kod] ?? (
                  // Monospace burada MEŞRUDUR (§9.2): bu bir kimlik dizgisidir,
                  // "teknik dursun" diye seçilmiş bir yazı tipi değil.
                  <span className="break-anywhere font-mono">{kod}</span>
                )}
              </li>
            ))}
          </ul>
        </Card>
      </section>
    </div>
  );
}

/**
 * Tek kuyruk karosu.
 *
 * Bileşen olarak DIŞA VERİLMEDİ ve `components/yonetim/` altına konmadı: bugün
 * tek çağrı yeri var. Bir deseni ikinci kullanıcısı çıkmadan paylaşımlı katmana
 * taşımak, henüz bilinmeyen bir ihtiyacı tahmin etmektir.
 *
 * Karonun TAMAMI bağlantıdır: küçük bir "Aç" düğmesi 44px hedefi zorlar ve
 * sayının kendisi tıklanamaz kalırdı. `<a>` tek odaklanabilir öğedir.
 */
function KuyrukKarosu({
  kuyruk,
  sayi,
  yukleniyor,
  hatali,
}: {
  kuyruk: Kuyruk;
  sayi: number | undefined;
  yukleniyor: boolean;
  hatali: boolean;
}) {
  return (
    <Link
      href={kuyruk.yol}
      className="surface flex h-full flex-col gap-2 rounded-2xl border p-4 md:p-6
                 [transition-property:background-color,border-color]
                 [transition-duration:var(--sure-hizli)]
                 [transition-timing-function:var(--ease-out)]
                 hover:bg-[var(--raised)]"
    >
      {/* 14px: veri taşıyan hiçbir metin `/yonetim`'de 14px altına inmez (§3.2). */}
      <span className="text-sm font-medium text-muted">{kuyruk.baslik}</span>

      {yukleniyor ? (
        <>
          {/* Yükleme İSKELETLE gösterilir, içerik ortasında spinner ile değil
              (§5.1). `Skeleton` `aria-hidden` olduğu için metin karşılığı ayrı. */}
          <Skeleton className="h-8 w-16" />
          <span className="sr-only">Yükleniyor</span>
        </>
      ) : hatali || sayi === undefined ? (
        /*
         * 🔴 KIRMIZI DEĞİL — ÖLÇÜM SONUCU.
         * `text-[var(--color-bad)]` bu yüzeyde AÇIK temada 3,71:1 verdi
         * (eşik 4.5, §7.1); `--color-ok/warn/bad/info` açık tema bloklarında
         * ezilmiyor ve bu bilinen bir jeton açığı (globals.css, bu dalgada
         * dokunulmuyor). Varsayılan metin rengi iki temada da 16,57:1 verir.
         * Anlamı zaten SÖZCÜKLER taşıyor (§7.1 "anlam yalnız renkle taşınmaz");
         * kırmızı ve `requestId` aşağıdaki `HataDurumu` kutusunda duruyor.
         */
        <span className="text-sm font-medium">Sayı alınamadı</span>
      ) : (
        /*
         * `tabular-nums`: bir para panelinde hizasız rakam en görünür craft
         * hatasıdır (§3.5). `text-2xl` (24px) — `text-3xl` h1'in `md:` boyudur,
         * aynı ölçüyü metrik için kullanmak hiyerarşiyi düzler (§3.1).
         *
         * `toLocaleString`: bu bir TAM SAYI, para veya tarih DEĞİL — CLAUDE.md'nin
         * "biçimleme yalnız lib/format" kuralı o ikisini kapsar ve `lib/format.ts`
         * bugün sayaç biçimleyicisi sunmuyor. Binlik ayracı olmadan "12000"
         * telefonda yanlış okunur.
         */
        <span className="text-2xl font-bold tabular-nums">
          {sayi.toLocaleString('tr-TR')}
        </span>
      )}

      <span className="text-sm text-muted">
        {sayi === 0 ? kuyruk.bosMetin : kuyruk.eylemMetni}
      </span>
    </Link>
  );
}
