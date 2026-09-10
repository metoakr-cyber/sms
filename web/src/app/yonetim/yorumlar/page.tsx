'use client';

/**
 * Müşteri yorumları — yönetim ekranı (moderasyon).
 *
 * Bu ekran SİTEDE NE YAYIMLANACAĞINA karar verir. "Onayla" düğmesine basmak,
 * bir kullanıcının cümlesini ana sayfaya koymaktır; "Reddet" ise onu
 * yayımlamamaktır ve gerekçesi KULLANICIYA GÖSTERİLİR. İkisi de geri
 * alınamayan sonuçlar üretir, bu yüzden ikisi de iki adımlıdır ve ikinci adım
 * yorumun tam metnini yeniden gösterir.
 *
 * 🔴 GÖSTERİLEN YORUM METNİ KULLANICI GİRDİSİDİR. `dangerouslySetInnerHTML`
 * BU DOSYADA YOKTUR; metin düz basılır, satır sonları CSS ile korunur.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * 🔴 NEDEN `VeriTablosu` KULLANILMADI — kasıtlı, ölçülebilir karar
 * ══════════════════════════════════════════════════════════════════════════
 * Diğer yönetim ekranlarının satırı SKALER alanlardan oluşur (tutar, tarih,
 * durum) ve bir tablo hücresine sığar. Bu ekranın BİRİNCİL VERİSİ bir
 * PARAGRAFTIR — kullanıcının yazdığı serbest metin, satır sonlarıyla birlikte.
 *
 * Onu bir sütuna koymanın iki yolu var ve ikisi de yanlış:
 *   1. Kısaltmak (`truncate`) → yönetici, yayına çıkacak metnin YALNIZ İLK
 *      SATIRINI görüp "Onayla ve yayımla"ya basar. Kısaltılmış önizlemeden
 *      verilen bir yayın kararı, bu ekranın var olma sebebini ortadan kaldırır.
 *   2. Kısaltmamak → satır yüksekliği 5-10 kat değişkenlik gösterir; "ferah
 *      52px satır" hedefi de tablo taranabilirliği de kalmaz.
 *
 * Bu yüzden liste HER KIRILIMDA karttır. §6.1'in amacı (mobilde yatay kaydırma
 * yasağı, tek veri kaynağı) zaten sağlanıyor: tek bir `<li>` şablonu var,
 * ikizi yok, `overflow-x` yok. `VeriTablosu`ya zorlamak, kapattığı tekrarı
 * değil YENİ bir riski getirirdi.
 *
 * Katmanın geri kalanı KULLANILDI: `SayfaBasligi` · `SuzgecCubugu` +
 * `KayitSayaci` · `Secim` · `CokSatir` · `DurumRozeti` · `HataDurumu` ·
 * `Sayfalama` · `OnayDiyalogu`. Silinen yerel kopyalar: `ErrorBox`,
 * `statusTone`, `selectClass`, `textareaClass`, `Chevron`, elle sayfalama,
 * elle kurulmuş onay diyaloğu.
 */

import * as React from 'react';
import { useMutation, useQuery, useQueryClient, keepPreviousData } from '@tanstack/react-query';
import { apiFetch } from '@/lib/api';
import { formatDateTime } from '@/lib/format';
import { Card, Button, Badge, Skeleton, Empty, cx, Field } from '@/components/ui';
import { Yildiz } from '@/components/ikonlar';
import {
  SayfaBasligi,
  SuzgecCubugu,
  KayitSayaci,
  Secim,
  CokSatir,
  Sayfalama,
  SAYFA_BOYUTU,
  DurumRozeti,
  HataDurumu,
  OnayDiyalogu,
  apiHatasi,
  sayfalamaGorunur,
} from '@/components/yonetim';

/* ═══════════════════════ Sunucu sözleşmesi ═══════════════════════ */
/* Karşılıkları: api/internal/transport/http/dto/review.go */

type ReviewStatus = 'PENDING' | 'APPROVED' | 'REJECTED';

interface AdminReview {
  id: string;
  userId: string;
  userEmail: string;
  userUsername: string;
  rating: number;
  body: string;
  status: ReviewStatus;
  statusLabel: string;
  rejectionReason?: string;
  createdAt: string;
  reviewedAt?: string;
}

interface AdminReviewList {
  items: AdminReview[];
  total: number;
  /** Süzgeçten BAĞIMSIZ bekleyen sayısı. */
  pendingTotal: number;
  limit: number;
  offset: number;
}

/** Sunucudaki sınır (domain/review: MaxRejectionReasonLen). */
const MAX_REASON = 500;
const runeLength = (s: string) => Array.from(s).length;

const STATUS_FILTERS: Array<{ value: '' | ReviewStatus; label: string }> = [
  // Varsayılan ilk sıradadır: yöneticinin işi BEKLEYEN yorumlardır.
  { value: 'PENDING', label: 'Onay bekleyenler' },
  { value: 'APPROVED', label: 'Yayında olanlar' },
  { value: 'REJECTED', label: 'Yayımlanmayanlar' },
  { value: '', label: 'Tümü' },
];

/**
 * Puan — YILDIZ + SAYI.
 *
 * 🔴 Yıldız tek başına yetmez: dolu ve boş yıldız AYNI BİÇİMDEDİR, yalnız
 * rengi değişir — anlamı yalnız renkle taşımak §7.1'in yasağıdır. Üstelik
 * açık temada `--color-warn` beyaz üzerinde 1,53:1 kontrast veriyor (ölçüldü),
 * yani dolu yıldız neredeyse görünmüyor. Sayı ikinci kanaldır.
 */
function Puan({ puan }: { puan: number }) {
  return (
    <span className="flex items-center gap-2">
      <span className="flex items-center gap-1" aria-hidden>
        {[1, 2, 3, 4, 5].map((n) => (
          <Yildiz
            key={n}
            className={cx('size-4', n <= puan
              ? 'text-[var(--color-warn)]'
              : 'text-[var(--border)]')}
          />
        ))}
      </span>
      <span className="text-sm tabular-nums text-muted">
        <span aria-hidden>{puan}/5</span>
        <span className="sr-only">5 üzerinden {puan} puan</span>
      </span>
    </span>
  );
}

/* ═══════════════════════ Sayfa ═══════════════════════ */

export default function YonetimYorumlarPage() {
  const [status, setStatus] = React.useState<'' | ReviewStatus>('PENDING');
  const [offset, setOffset] = React.useState(0);
  const [karar, setKarar] = React.useState<{ review: AdminReview; tur: 'onay' | 'red' } | null>(null);

  // Sayfa boyutu katmandan gelir: tek panelde tek `limit` (§6.3).

  // İKİ AYRI DURUM: `aramaGirdisi` kutuya yazılan, `arama` sunucuya giden.
  // 350 ms — panelin ve yönetimin her yerinde aynı gecikme.
  const [aramaGirdisi, setAramaGirdisi] = React.useState('');
  const [arama, setArama] = React.useState('');
  React.useEffect(() => {
    const t = setTimeout(() => {
      setArama(aramaGirdisi.trim());
      setOffset(0);
    }, 350);
    return () => clearTimeout(t);
  }, [aramaGirdisi]);

  const query = new URLSearchParams({ limit: String(SAYFA_BOYUTU), offset: String(offset) });
  if (status) query.set('status', status);
  if (arama) query.set('q', arama);

  const q = useQuery({
    queryKey: ['admin', 'reviews', { status, arama, offset }],
    queryFn: () => apiFetch<AdminReviewList>(`/admin/reviews?${query.toString()}`),
    placeholderData: keepPreviousData,
  });

  const total = q.data?.total ?? 0;
  const bekleyen = q.data?.pendingTotal ?? 0;
  const listErr = apiHatasi(q.error);
  const items = q.data?.items;

  /*
   * `Sayfalama` GÖRÜNÜR MÜ? Koşul bileşenin kendisinden okunur
   * (`sayfalamaGorunur`), burada kopyalanmaz — kopya, bileşenin gizlenme
   * kuralı değiştiği gün sessizce yanlış olur ve iki canlı bölge birden
   * konuşmaya başlar.
   */
  const sayfali = sayfalamaGorunur(total);

  /*
   * Canlı bölge — `VeriTablosu`nun yaptığı işin kart listesi karşılığı (§7.4).
   * Bölge DAİMA DOM'da durur: koşullu render edilirse ekran okuyucu onu "yeni
   * içerik" saymaz ve hiçbir şey duyurulmaz. Bu yüzden bölge değil, İÇERİĞİ
   * susturulur.
   *
   * 🔴 SAYFALIYKEN SUSAR — iki ayrı hatayı birden kapatır:
   *  1. ÇİFT DUYURU: `Sayfalama`nın kendi `aria-live`'ı ("1–25 / 87") zaten
   *     konuşuyor; süzgeç değişiminde ekran okuyucu iki polite duyuruyu sıraya
   *     alıyordu. `kullanicilar`/`denetim` bunu `duyuru={!sayfali}` ile çözmüş,
   *     bu ekran kuralın dışında kalmıştı.
   *  2. YANLIŞ SAYI: `items.length` SAYFADAKİ yorum sayısıdır — 87 kayıtlık
   *     listede her sayfada "25 yorum listelendi" derdi. Sayfalı liste tam
   *     olarak bu cümlenin yanlış olduğu durumdur; orada `Sayfalama`nın doğru
   *     ve toplamı içeren metni tek başına kalır. Tek sayfalık listede ise
   *     sayfadaki sayı ZATEN toplamdır — yani duyuru konuştuğu her yerde doğru.
   *
   * 🔴 HATA DURUMU BURADAN ÇIKARILDI — üçüncü çift duyuru buydu.
   * Hata olduğunda aşağıda `HataDurumu` çiziliyor; o bir `Alert tone="bad"`tir
   * ve `Alert`in varsayılan `duyur`u `role="alert"` verir (`ui.tsx:155`), yani
   * ZATEN kesintili olarak duyuruluyor — üstelik sunucunun gerçek Türkçe
   * mesajını ve `requestId`'yi taşıyarak. Buraya ayrıca "Liste yüklenemedi."
   * yazmak, aynı olay için ikinci ve DAHA AZ BİLGİLİ bir duyuru sıraya
   * sokuyordu (§7.4: tek olay, tek duyuru).
   *
   * Eski gerekçe ("hata durumunda `total` 0'a düşer, çakışma doğmaz") ölçünce
   * yanlış çıktı: aynı anahtarla yapılan bir yenileme başarısız olduğunda
   * TanStack Query `data`yı KORUR — `total` 87'de kalır, `Sayfalama` çizilmeye
   * devam eder. Yani çakışma tam da o yolda doğuyordu.
   *
   * Boş durum polite kalır: `Empty` bir canlı bölge değildir, tek duyuru budur.
   */
  const duyuru =
    q.isLoading || listErr
      ? ''
      : !items || items.length === 0
        ? 'Sonuç bulunamadı.'
        : sayfali
          ? ''
          : `${items.length} yorum listelendi.`;

  return (
    // GENİŞLİK KABI YOK: tavanı kabuğun `main`i verir (`max-w-12xl`), böylece
    // içerik genişliği ekrandan ekrana zıplamaz. Eski hâlde her yönetim ekranı
    // kendi kabını açıyordu ve beş farklı değer vardı (2xl…6xl).
    <div className="flex flex-col gap-6">
      <SayfaBasligi
        baslik="Müşteri yorumları"
        aciklama="Onayladığınız yorumlar sitede kullanıcı adıyla yayımlanır. E-posta hiçbir zaman gösterilmez."
      >
        {/* Rozet bir ETİKETTİR, cümle değil (§5.2): "3 yorum karar bekliyor"
            yerine üç kelime. Sayı `tabular-nums`. */}
        {bekleyen > 0 && (
          <Badge tone="warn">
            <span className="tabular-nums">{bekleyen}</span>
            <span className="ms-1">karar bekliyor</span>
          </Badge>
        )}
      </SayfaBasligi>

      <Card className="flex flex-col gap-6">
        <SuzgecCubugu
          sag={<KayitSayaci toplam={total} />}
          etkinSayisi={(status ? 1 : 0) + (arama ? 1 : 0)}
          onTemizle={() => {
            setStatus('');
            setAramaGirdisi('');
            setArama('');
            setOffset(0);
          }}
        >
          <div className="w-full sm:w-72 sm:self-start">
            <Field
              label="Ara"
              type="search"
              value={aramaGirdisi}
              onChange={(e) => setAramaGirdisi(e.target.value)}
              placeholder="Yorum metni, e-posta veya kullanıcı adı"
              autoCapitalize="none"
              autoCorrect="off"
              autoComplete="off"
              spellCheck={false}
              hint="Yazmayı bıraktığınızda arama kendiliğinden yapılır."
            />
          </div>

          {/* Etiket GÖRÜNÜR yapıldı (eskiden `sr-only`): aynı bölümdeki
              `destek` ekranında etiket görünürdü, burada değildi — §9.2'nin
              "ekranlar arası tutarsız bileşen dili" bulgusu. */}
          <Secim
            etiket="Durum"
            className="sm:w-64"
            value={status}
            onChange={(e) => {
              setStatus(e.target.value as '' | ReviewStatus);
              setOffset(0);
            }}
          >
            {STATUS_FILTERS.map((f) => (
              <option key={f.value || 'all'} value={f.value}>{f.label}</option>
            ))}
          </Secim>
        </SuzgecCubugu>

        <p aria-live="polite" aria-atomic="true" className="sr-only">{duyuru}</p>

        {q.isLoading ? (
          <Iskelet />
        ) : listErr ? (
          <HataDurumu hata={listErr} />
        ) : !items?.length ? (
          /* §6.3: süzgeçten dolayı boş ile gerçekten boş FARKLI metinlerdir. */
          <Empty
            title="Bu süzgeçte yorum yok"
            hint={status === 'PENDING'
              ? 'Karar bekleyen yorum yok. Kuyruk temiz.'
              : 'Başka bir durum seçerek listeyi genişletebilirsiniz.'}
          />
        ) : (
          <ul className="flex flex-col gap-4">
            {items.map((r) => (
              /* İÇ İÇE KART YOK (§9.2): bu bir `<li>` satırıdır, ikinci bir
                 `Card` değil; derinlik değil KENARLIK kullanır (§2.4). */
              <li key={r.id} className="raised rounded-xl border p-4 md:p-5">
                <div className="flex flex-wrap items-start justify-between gap-3">
                  <div className="min-w-0">
                    <p className="truncate font-medium">{r.userUsername}</p>
                    {/* E-posta YÖNETİM ekranında görünür (moderasyon kararı
                        kimin yazdığını bilmeden verilemez) ama SİTEDE ASLA. */}
                    <p className="truncate text-sm text-muted">{r.userEmail}</p>
                  </div>
                  {/* `shrink-0` YOK, `flex-wrap` VAR: 320px'te yıldızlar +
                      "4/5" + rozet tek satıra sığmıyor (ölçüldü). Sığmayan
                      blok sarmalıdır; sabitlenirse kart yatay taşar. */}
                  <div className="flex flex-wrap items-center justify-end gap-3">
                    <Puan puan={r.rating} />
                    <DurumRozeti durum={r.status} etiket={r.statusLabel} />
                  </div>
                </div>

                {/* Karara konu olan metin: 65-75ch bandında (§3.4), kısaltma YOK. */}
                <p className="mt-4 max-w-[70ch] whitespace-pre-wrap break-anywhere text-sm leading-relaxed">
                  {r.body}
                </p>

                <p className="mt-4 text-sm tabular-nums text-muted">
                  Gönderildi: {formatDateTime(r.createdAt)}
                  {r.reviewedAt && <> · Karar: {formatDateTime(r.reviewedAt)}</>}
                </p>

                {r.status === 'REJECTED' && r.rejectionReason && (
                  /*
                    🔴 ZEMİN `--bg`, `--raised` DEĞİL. Ölçüldü: bu kutu `raised`
                    bir `<li>` içinde duruyor ve `bg-[var(--raised)]` yazılmıştı
                    — aynı renk üstüne aynı renk, yani kutu HİÇ GÖRÜNMÜYORDU.
                    `--bg` iki temada da yüzeyden ayrışır.
                  */
                  <p className="mt-3 max-w-[70ch] whitespace-pre-wrap break-anywhere rounded-lg
                                bg-[var(--bg)] p-3 text-sm text-muted">
                    <span className="font-medium">Gerekçe: </span>{r.rejectionReason}
                  </p>
                )}

                {/* İŞLEMLER — duruma göre.
                    PENDING: onayla / reddet.
                    APPROVED: yalnız "yayından kaldır" (bu da bir reddir ve
                      gerekçe ister) — yanlışlıkla onaylanmış bir yorumu
                      indirmenin başka yolu yok.
                    REJECTED: TERMİNAL, işlem yok. */}
                {r.status !== 'REJECTED' && (
                  <div className="mt-4 flex flex-col gap-2 sm:flex-row">
                    {r.status === 'PENDING' && (
                      // Onay BİRİNCİL (dolu) düğmedir: yayına çıkaran eylem
                      // budur ve red/kaldırma ondan görsel olarak ayrışmalıdır.
                      <Button size="sm" fullWidth className="sm:w-auto"
                              onClick={() => setKarar({ review: r, tur: 'onay' })}>
                        Onayla ve yayımla
                      </Button>
                    )}
                    <Button size="sm" variant="outline" fullWidth className="sm:w-auto"
                            onClick={() => setKarar({ review: r, tur: 'red' })}>
                      {r.status === 'APPROVED' ? 'Yayından kaldır' : 'Reddet'}
                    </Button>
                  </div>
                )}
              </li>
            ))}
          </ul>
        )}

        <Sayfalama
          offset={offset}
          limit={SAYFA_BOYUTU}
          toplam={total}
          onDegis={setOffset}
        />
      </Card>

      {karar && (
        <KararDialog
          review={karar.review}
          tur={karar.tur}
          onClose={() => setKarar(null)}
        />
      )}
    </div>
  );
}

/** Yükleme İSKELETLE gösterilir (§5.1); iskelet gerçek kartın anatomisini
 *  taklit eder, böylece veri geldiğinde düzen ZIPLAMAZ. */
function Iskelet() {
  return (
    <ul className="flex flex-col gap-4">
      {[0, 1, 2].map((i) => (
        <li key={i} className="raised rounded-xl border p-4 md:p-5">
          <div className="flex items-start justify-between gap-3">
            <Skeleton className="h-5 w-40" />
            <Skeleton className="h-5 w-28" />
          </div>
          <div className="mt-4 flex flex-col gap-3">
            <Skeleton className="h-4 w-full" />
            <Skeleton className="h-4 w-4/5" />
            <Skeleton className="h-4 w-2/5" />
          </div>
        </li>
      ))}
    </ul>
  );
}

/* ═══════════════════════ Karar diyaloğu ═══════════════════════ */

/**
 * Onay ve red TEK diyalogdadır: ikisi de aynı metni yeniden gösterip
 * "bunu mu yayımlıyorsunuz / kaldırıyorsunuz" diye sorar. İki ayrı diyalog
 * yazmak, birinde eklenen bir onay adımının diğerinde unutulmasını
 * kolaylaştırırdı.
 *
 * Kap artık `OnayDiyalogu`: buton düzeni (mobilde alt alta, `sm:` üstünde
 * onay sağda), yıkıcı tonlama ve hata gösterimi tek yerden gelir. Elle
 * kurulmuş `flex-col sm:flex-row-reverse` bloğu ve `ErrorBox` silindi.
 */
function KararDialog({
  review, tur, onClose,
}: { review: AdminReview; tur: 'onay' | 'red'; onClose: () => void }) {
  const qc = useQueryClient();
  const [reason, setReason] = React.useState('');
  const [formErr, setFormErr] = React.useState('');

  const mut = useMutation({
    mutationFn: () =>
      tur === 'onay'
        ? apiFetch<AdminReview>(`/admin/reviews/${encodeURIComponent(review.id)}/approve`, {
            method: 'POST',
          })
        : apiFetch<AdminReview>(`/admin/reviews/${encodeURIComponent(review.id)}/reject`, {
            method: 'POST', body: { reason: reason.trim() },
          }),
    // Tekrarlanan bir karar, ikinci kez 409 döner ve yönetici kararın
    // yazılmadığını sanar (değişmez #16).
    retry: false,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['admin', 'reviews'] });
      onClose();
    },
  });

  const kaldiriliyor = tur === 'red' && review.status === 'APPROVED';

  function onayla() {
    if (tur === 'red') {
      const r = reason.trim();
      if (r.length === 0) {
        // Gerekçe kullanıcıya gösterilir; boş bırakmak "reddedildi" deyip
        // sebebini söylememektir. Sunucu da reddeder (422).
        setFormErr('Red gerekçesi zorunludur; kullanıcıya gösterilecek.');
        return;
      }
      if (runeLength(r) > MAX_REASON) {
        // Sayaç `ipucu` yuvasında ve hata varken gizleniyor; güncel uzunluk
        // bu yüzden hata metnine yazılır.
        setFormErr(`Gerekçe en fazla ${MAX_REASON} karakter olabilir; şu an ${runeLength(r)}.`);
        return;
      }
    }
    setFormErr('');
    mut.mutate();
  }

  return (
    <OnayDiyalogu
      acik
      baslik={tur === 'onay' ? 'Yorumu yayımla' : kaldiriliyor ? 'Yayından kaldır' : 'Yorumu reddet'}
      /* Onay düğmesi listedeki düğmeyle AYNI metni taşır: yönetici neyi
         onayladığını düğmede okur, "Tamam" yazmaz (§ OnayDiyalogu sözleşmesi). */
      onayMetni={tur === 'onay' ? 'Onayla ve yayımla' : kaldiriliyor ? 'Yayından kaldır' : 'Reddet'}
      /* Red ve kaldırma YIKICIDIR (kırmızı); onay değildir — onay bir yayın
         eylemidir, bir yıkım değil. Renk ayrımı, iki kararın karışmasını
         önleyen ikinci kanaldır; birincisi düğme metnidir. */
      yikici={tur === 'red'}
      uyari={kaldiriliyor
        ? 'Bu yorum şu anda sitede yayında. Kaldırdıktan sonra geri alınamaz; kullanıcı isterse yeni bir yorum yazabilir.'
        : undefined}
      bekliyor={mut.isPending}
      hata={apiHatasi(mut.error)}
      onOnayla={onayla}
      onIptal={onClose}
    >
      {/* İKİNCİ ADIM YORUMU YENİDEN GÖSTERİR: yönetici listede yanlış
          satıra basmış olabilir. Kısaltma YOK — karar tam metin üzerinde
          verilir. */}
      <div className="raised rounded-xl border p-4">
        <div className="flex items-center justify-between gap-3">
          <span className="truncate font-medium">{review.userUsername}</span>
          <Puan puan={review.rating} />
        </div>
        <p className="mt-3 whitespace-pre-wrap break-anywhere text-sm leading-relaxed">
          {review.body}
        </p>
      </div>

      {tur === 'onay' ? (
        /* Düz paragraf, `Alert` DEĞİL — ve gerekçe ARTIK BU DEĞİL.
           Eski yorum "`Alert` sabit `role='alert'` taşıyor" diyordu; `Alert`
           bu dalgada `duyur?: boolean` aldı (`ui.tsx:144`), yani susturulabilir
           bir kutu artık mümkün. Kutuya geçilmemesinin GERÇEK nedeni sunum:
           bu cümle bir uyarı değil, onayın SONUCUdur ve `fiyatlar`ın onay
           diyaloğu aynı rolü (§9.2 "tutarlı bileşen dili") düz `<p>` ile
           yazıyor — kutu yalnız yanındaki uyarıya ayrılmış durumda.
           Duyuru açısından fark yok: `OnayDiyalogu` açılışında bu metin zaten
           diyaloğun gövdesidir, kesecek bir eylem yoktur (§7.4). */
        <p className="text-sm text-muted">
          Bu yorum sitede <strong className="font-semibold text-[var(--text)]">
          {review.userUsername}</strong> adıyla yayımlanacak. E-posta gösterilmez.
        </p>
      ) : (
        /*
          `data-autofocus` BURADA ve `OnayDiyalogu`nun "Vazgeç"teki odağını
          bilerek devralır (`modal.tsx` DOM sırasındaki İLK işaretli öğeyi
          odaklar). §7.5'in yasağı odağı ONAY DÜĞMESİNE koymaktır — basılı
          kalan Enter'ın kararı yazması riski oradadır; metin alanında Enter
          satır başı yapar. Gerekçe ZORUNLU alandır, bugünkü davranış da
          budur. Onay dalında böyle bir alan yok; orada odak "Vazgeç"te kalır.
        */
        <CokSatir
          data-autofocus
          etiket="Gerekçe (kullanıcıya gösterilir)"
          rows={4}
          value={reason}
          disabled={mut.isPending}
          onChange={(e) => setReason(e.target.value)}
          placeholder="Örn: Yorum reklam bağlantısı içeriyor."
          ipucu={`${runeLength(reason)}/${MAX_REASON} karakter`}
          hata={formErr || undefined}
        />
      )}
    </OnayDiyalogu>
  );
}
