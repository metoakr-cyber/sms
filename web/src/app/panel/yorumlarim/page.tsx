'use client';

/**
 * Yorumlarım — müşteri ekranı.
 *
 * Kullanıcı yorumunu buradan yazar; yönetici onaylayana kadar yorum sitede
 * GÖRÜNMEZ ve bu ekran onu açıkça söyler. "Gönderildi" deyip sonra hiçbir şey
 * olmaması, kullanıcıyı destek talebi açmaya iter.
 *
 * 🔴 BU EKRANDA GÖSTERİLEN METNİN TAMAMI KULLANICI GİRDİSİDİR (kendi yorumu
 * ve yöneticinin red gerekçesi). `dangerouslySetInnerHTML` BU DOSYADA YOKTUR
 * ve olmayacaktır; React metni varsayılan olarak kaçırır. Satır sonları
 * `whitespace-pre-wrap` ile korunur — biçimlendirme HTML'e değil CSS'e
 * bırakılır.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * 🔴 NEDEN `VeriTablosu` KULLANILMADI — kasıtlı, `/yonetim/yorumlar` ile aynı
 * ══════════════════════════════════════════════════════════════════════════
 * Diğer listelerin satırı SKALER alanlardan oluşur (tutar, tarih, durum) ve bir
 * tablo hücresine sığar. Bu ekranın BİRİNCİL VERİSİ bir PARAGRAFTIR —
 * kullanıcının yazdığı serbest metin, satır sonlarıyla birlikte.
 *
 * Onu bir sütuna koymanın iki yolu var ve ikisi de yanlış:
 *   1. Kısaltmak (`truncate`) → kullanıcı yayına çıkacak kendi cümlesinin
 *      yalnız ilk satırını görür; red gerekçesiyle karşılaştıramaz.
 *   2. Kısaltmamak → satır yüksekliği 5-10 kat değişkenlik gösterir; ne ferah
 *      satır hedefi ne de tablo taranabilirliği kalır.
 *
 * §6.1'in amacı (mobilde yatay kaydırma yasağı, tek veri kaynağı) burada zaten
 * sağlanıyor: TEK bir `<li>` şablonu var, mobil/masaüstü ikizi YOK, `overflow-x`
 * YOK. `VeriTablosu`ya zorlamak, kapattığı tekrarı değil YENİ bir riski
 * getirirdi. Aynı karar `/yonetim/yorumlar` için de verilmişti; iki ekran aynı
 * gerekçeyle aynı biçimde duruyor.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * KATMANIN GERİ KALANI KULLANILDI — silinen yerel kopyalar
 * ══════════════════════════════════════════════════════════════════════════
 *   · `ErrorBox`      → `HataDurumu`  (11 kopyanın biriydi; `requestId` artık
 *                        14px ve tam opaklıkta)
 *   · `statusTone`    → `DurumRozeti` (6 kopyanın biri; rozet artık METİN +
 *                        BİÇİM + renk taşıyor — renk körü kullanıcı için
 *                        yalnız renk yetmez, §7.1)
 *   · `textareaClass` → `CokSatir`    (+ `caret-color`: koyu temada varsayılan
 *                        siyah imleç GÖRÜNMÜYORDU)
 *   · elle sayfalama  → `Sayfalama`   (yerel `PAGE = 20` silindi → `SAYFA_BOYUTU`)
 *   · başlık bloğu    → `SayfaBasligi`
 *
 * DAVRANIŞ DEĞİŞMEDİ. Tek sayısal fark sayfa boyutudur (20 → 25): aynı panelde
 * iki farklı `limit`, "kaç kayıt kaldı" sorusunun ekrandan ekrana farklı
 * cevaplanmasıydı.
 *
 * HAREKET: yok. Liste her girişte görülür (§4.2). Tek geçişler `Button`'un
 * `:active` basma geri bildirimi ve `CokSatir`'ın odak KENARLIK rengidir —
 * ikisi de katmandan gelir.
 */

import * as React from 'react';
import { useMutation, useQuery, useQueryClient, keepPreviousData } from '@tanstack/react-query';
import { apiFetch } from '@/lib/api';
import { formatDateTime } from '@/lib/format';
import { Card, Button, Alert, Skeleton, Empty, cx } from '@/components/ui';
import { Modal } from '@/components/modal';
import { Yildiz } from '@/components/ikonlar';
import {
  SayfaBasligi,
  KayitSayaci,
  CokSatir,
  Sayfalama,
  SAYFA_BOYUTU,
  DurumRozeti,
  HataDurumu,
  apiHatasi,
} from '@/components/yonetim';

/* ═══════════════════════ Sunucu sözleşmesi ═══════════════════════ */
/*
 * Tipler BU DOSYADA tanımlıdır, `lib/types.ts` içinde değil: o dosya bu turda
 * paylaşılan bir dosyadır ve başka ajanlar da düzenliyor. Yorumlar kalıcı hâle
 * geldiğinde buradaki tipler `lib/types.ts`'e taşınmalıdır (rapora yazıldı).
 * Karşılıkları: api/internal/transport/http/dto/review.go
 */

type ReviewStatus = 'PENDING' | 'APPROVED' | 'REJECTED';

interface Review {
  id: string;
  rating: number;
  body: string;
  status: ReviewStatus;
  statusLabel: string;
  rejectionReason?: string;
  createdAt: string;
  reviewedAt?: string;
}

interface ReviewList {
  items: Review[];
  total: number;
  limit: number;
  offset: number;
}

/* ═══════════════════════ Sunucu sınırları ═══════════════════════ */
/* domain/review/review.go — istemci doğrulaması KOLAYLIKTIR, savunma değil. */
const MIN_BODY = 10;
const MAX_BODY = 1000;

/** Uzunluk KARAKTER (rune) ile sayılır: "ş" iki bayttır, bir karakterdir. */
const runeLength = (s: string) => Array.from(s).length;

/** Salt okunur yıldız gösterimi. */
function Yildizlar({ puan }: { puan: number }) {
  return (
    <span className="flex items-center gap-1" aria-label={`5 üzerinden ${puan} puan`}>
      {[1, 2, 3, 4, 5].map((n) => (
        <Yildiz
          key={n}
          className={cx('size-4', n <= puan
            ? 'text-[var(--color-warn)]'
            : 'text-[var(--border)]')}
        />
      ))}
    </span>
  );
}

/**
 * Puan seçici.
 *
 * `<input type="radio">` KULLANILIR, tıklanabilir `<span>` değil: klavye
 * kullanıcısı ok tuşlarıyla gezebilir ve ekran okuyucu "5 üzerinden 4"
 * diyebilir. Görsel yıldızlar `aria-hidden`; erişilebilirlik radyo
 * grubundadır.
 */
function PuanSecici({
  value, onChange, disabled,
}: { value: number; onChange: (n: number) => void; disabled?: boolean }) {
  return (
    <fieldset disabled={disabled} className="flex flex-col gap-2">
      <legend className="text-sm font-medium">Puanınız</legend>
      <div className="flex items-center gap-1">
        {[1, 2, 3, 4, 5].map((n) => (
          <label
            key={n}
            /* Dokunma hedefi 44px: p-2 (8+8) + size-7 (28) = 44px kenar. */
            className={cx(
              'cursor-pointer rounded-lg p-2',
              // Yalnız RENK geçer, 120ms. Konum/ölçek hareketi yok (§4.3).
              '[transition-property:background-color] [transition-duration:var(--sure-hizli)]',
              'has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-brand-400',
              disabled && 'cursor-not-allowed opacity-60',
            )}
          >
            <input
              type="radio"
              name="puan"
              className="sr-only"
              value={n}
              checked={value === n}
              onChange={() => onChange(n)}
            />
            <span className="sr-only">{n} yıldız</span>
            <Yildiz
              aria-hidden
              className={cx('size-7', n <= value
                ? 'text-[var(--color-warn)]'
                : 'text-[var(--border)]')}
            />
          </label>
        ))}
      </div>
    </fieldset>
  );
}

/* ═══════════════════════ Sayfa ═══════════════════════ */

export default function YorumlarimPage() {
  const [offset, setOffset] = React.useState(0);
  const [creating, setCreating] = React.useState(false);

  const q = useQuery({
    queryKey: ['reviews', 'mine', { limit: SAYFA_BOYUTU, offset }],
    queryFn: () => apiFetch<ReviewList>(`/reviews/mine?limit=${SAYFA_BOYUTU}&offset=${offset}`),
    // Sayfa değişince liste boşalıp zıplamasın.
    placeholderData: keepPreviousData,
  });

  const total = q.data?.total ?? 0;
  const items = q.data?.items;
  const listErr = apiHatasi(q.error);

  // Onay bekleyen bir yorum varken yenisi gönderilemez (sunucu kısıtı:
  // reviews_one_pending_per_user_idx). Düğmeyi kapatmak sunucunun kararını
  // TEKRAR ETMEZ, yalnız kullanıcıyı 409 almadan uyarır.
  const bekleyen = items?.some((r) => r.status === 'PENDING') ?? false;

  /*
   * `Sayfalama` tek sayfaya sığan listede kendini hiç çizmez; bu ifade
   * "sayfalama ekranda var mı" ile aynı şeydir.
   */
  const sayfali = total > SAYFA_BOYUTU;

  /*
   * Canlı bölge — `VeriTablosu`nun yaptığı işin kart listesi karşılığı (§7.4).
   * Bölge DAİMA DOM'da durur: koşullu render edilirse ekran okuyucu onu "yeni
   * içerik" saymaz ve hiçbir şey duyurulmaz. Bu yüzden bölge değil, İÇERİĞİ
   * susturulur.
   *
   * 🔴 SAYFALIYKEN SUSAR — iki hatayı birden kapatır: (1) `Sayfalama`nın kendi
   * `aria-live`'ı zaten konuşuyor, iki polite duyuru sıraya girerdi; (2)
   * `items.length` SAYFADAKİ sayıdır — 87 kayıtlık listede her sayfada
   * "25 yorum listelendi" derdi. Tek sayfalık listede sayfadaki sayı ZATEN
   * toplamdır, yani duyuru konuştuğu her yerde doğrudur.
   */
  const duyuru = q.isLoading
    ? ''
    : listErr
      ? 'Liste yüklenemedi.'
      : !items || items.length === 0
        ? 'Henüz yorumunuz yok.'
        : sayfali
          ? ''
          : `${items.length} yorum listelendi.`;

  return (
    <div className="mx-auto flex max-w-4xl flex-col gap-6">
      <SayfaBasligi
        baslik="Yorumlarım"
        aciklama="Deneyiminizi yazın. Yorumunuz ekibimizin onayından sonra sitede yayımlanır."
      >
        <Button
          fullWidth
          className="sm:w-auto"
          disabled={bekleyen}
          onClick={() => setCreating(true)}
        >
          Yorum yaz
        </Button>
      </SayfaBasligi>

      {bekleyen && (
        /* `duyur={false}`: bu kutu VERİ YÜKLENİNCE koşullu çizilir, bir işlemin
           sonucunda BELİRMEZ. `role="alert"` olsaydı ekran okuyucu kullanıcının
           sözünü sayfa açılışında keserdi (§7.4). */
        <Alert tone="info" duyur={false}>
          Onay bekleyen bir yorumunuz var. O sonuçlandığında yenisini yazabilirsiniz.
        </Alert>
      )}

      <Card className="flex flex-col gap-6">
        <div className="flex flex-wrap items-center justify-between gap-3">
          {/* `h2` her yerde aynı boyutta: `text-lg font-semibold` (§3.3). */}
          <h2 className="text-lg font-semibold">Gönderdiğim yorumlar</h2>
          <KayitSayaci toplam={total} />
        </div>

        <p aria-live="polite" aria-atomic="true" className="sr-only">{duyuru}</p>

        {q.isLoading ? (
          /* Yükleme İSKELETLE (§5.1). İskelet gerçek kartın şemasından türer:
             üst satır (yıldız + rozet), üç satırlık paragraf, alt bilgi —
             böylece veri geldiğinde düzen ZIPLAMAZ. */
          <ul className="flex flex-col gap-4">
            {[0, 1, 2].map((i) => (
              <li key={i} className="raised rounded-xl border p-4 md:p-5">
                <div className="flex items-center justify-between gap-3">
                  <Skeleton className="h-5 w-28" />
                  <Skeleton className="h-5 w-24" />
                </div>
                <div className="mt-4 flex flex-col gap-2">
                  <Skeleton className="h-4 w-full" />
                  <Skeleton className="h-4 w-4/5" />
                </div>
                <Skeleton className="mt-4 h-4 w-40" />
              </li>
            ))}
          </ul>
        ) : listErr ? (
          <HataDurumu hata={listErr} />
        ) : !items?.length ? (
          /* Boş durum ÖĞRETİR (§5.1). Bu liste süzgeçsizdir: boş olmasının tek
             anlamı GERÇEKTEN boş olmasıdır (§6.3). */
          <Empty
            title="Henüz yorum yazmadınız"
            hint="Hizmetimizle ilgili düşüncelerinizi paylaşırsanız, onaydan sonra sitede kullanıcı adınızla yayımlanır."
          />
        ) : (
          <ul className="flex flex-col gap-4">
            {items.map((r) => (
              /* İÇ İÇE KART YOK (§9.2): bu bir `<li>` satırıdır, ikinci bir
                 `Card` değil; derinlik değil KENARLIK kullanır (§2.4). */
              <li key={r.id} className="raised rounded-xl border p-4 md:p-5">
                {/* `flex-wrap`, `shrink-0` DEĞİL: 320px'te yıldızlar + rozet tek
                    satıra sığmayabilir. Sığmayan blok SARMALIDIR; sabitlenirse
                    kart yatay taşar. */}
                <div className="flex flex-wrap items-center justify-between gap-3">
                  <Yildizlar puan={r.rating} />
                  <DurumRozeti durum={r.status} etiket={r.statusLabel} />
                </div>

                {/* Yorumun kendisi: 65-75ch bandında (§3.4), kısaltma YOK.
                    `whitespace-pre-wrap` satır sonlarını korur, HTML yorumlamaz.
                    `break-anywhere`: boşluksuz uzun bir dize 320px'te yatay
                    kaydırma üretirdi. */}
                <p className="mt-4 max-w-[70ch] whitespace-pre-wrap break-anywhere
                              text-sm leading-relaxed">
                  {r.body}
                </p>

                {/* `text-sm` + `tabular-nums`: tarih veridir, dipnot değil
                    (§3.2/§3.5 — eskiden `text-xs` idi ve rakamlar kayıyordu). */}
                <p className="mt-4 text-sm tabular-nums text-muted">
                  Gönderildi: {formatDateTime(r.createdAt)}
                  {r.reviewedAt && (
                    <> · {r.status === 'APPROVED' ? 'Yayımlandı' : 'Karar'}: {formatDateTime(r.reviewedAt)}</>
                  )}
                </p>

                {r.status === 'REJECTED' && r.rejectionReason && (
                  <Alert tone="warn" duyur={false} className="mt-4">
                    <p className="font-medium">Yayımlanmama nedeni</p>
                    <p className="mt-1 max-w-[70ch] whitespace-pre-wrap break-anywhere">
                      {r.rejectionReason}
                    </p>
                    <p className="mt-2">Dilerseniz yeni bir yorum yazabilirsiniz.</p>
                  </Alert>
                )}

                {r.status === 'PENDING' && (
                  <p className="mt-4 text-sm text-muted">
                    Bu yorum henüz sitede görünmüyor; onaydan sonra yayımlanacak.
                  </p>
                )}
              </li>
            ))}
          </ul>
        )}

        <Sayfalama offset={offset} limit={SAYFA_BOYUTU} toplam={total} onDegis={setOffset} />
      </Card>

      {creating && <YorumYazDialog onClose={() => setCreating(false)} />}
    </div>
  );
}

/* ═══════════════════════ Yeni yorum ═══════════════════════ */

/**
 * `OnayDiyalogu` DEĞİL, ham `Modal`. §6.4: modal (a) yıkıcı işlem onayı ya da
 * (b) korunmuş odak gerektiren çok adımlı form içindir. Bu (b)'dir — puan +
 * metin, tek bir evet/hayır değil.
 */
function YorumYazDialog({ onClose }: { onClose: () => void }) {
  const qc = useQueryClient();
  const [rating, setRating] = React.useState(5);
  const [body, setBody] = React.useState('');
  const [errs, setErrs] = React.useState<Record<string, string>>({});

  const create = useMutation({
    mutationFn: (v: { rating: number; body: string }) =>
      apiFetch<Review>('/reviews', { method: 'POST', body: v }),
    // MUTASYON OTOMATİK TEKRARLANMAZ (değişmez #16): ağ yanıtı yutulduğunda
    // ikinci deneme sunucudan 409 alır ("bekleyen yorumunuz var") ve kullanıcı
    // gönderdiği yorumun kaybolduğunu sanar.
    retry: false,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['reviews', 'mine'] });
      onClose();
    },
  });

  function submit(e: React.FormEvent) {
    e.preventDefault();
    const next: Record<string, string> = {};
    const b = body.trim();
    if (runeLength(b) < MIN_BODY) next.body = 'Yorum en az 10 karakter olmalıdır.';
    else if (runeLength(b) > MAX_BODY) next.body = 'Yorum en fazla 1000 karakter olabilir.';
    if (rating < 1 || rating > 5) next.rating = 'Puan 1 ile 5 arasında olmalıdır.';
    setErrs(next);
    if (Object.keys(next).length) return;
    create.mutate({ rating, body: b });
  }

  const err = apiHatasi(create.error);

  /*
   * SINIR AŞIMI CANLI GÖSTERİLİR. Eski dosya sayacı sınır aşılınca KIRMIZIYA
   * boyuyordu ama metni değiştirmiyordu — yani uyarı yalnız RENKLE taşınıyordu
   * (§7.1 ihlali) ve ekran okuyucuya hiç ulaşmıyordu. Aynı bilgi artık
   * `hata` olarak veriliyor: `CokSatir` onu `aria-describedby` +
   * `role="alert"` ile bağlar, `aria-invalid` işaretler ve kenarlığı
   * kırmızıya çeker. Metin sabit olduğu için sınır aşıldığında BİR KEZ
   * duyurulur, her tuş vuruşunda değil.
   */
  const asim = runeLength(body) > MAX_BODY;

  return (
    <Modal open onClose={onClose} title="Yorum yaz">
      <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
        <PuanSecici value={rating} onChange={setRating} disabled={create.isPending} />
        {errs.rating && (
          // `role="alert"`: hata YENİ BELİREN içeriktir (§7.4).
          <p role="alert" className="text-sm text-[var(--color-bad)]">{errs.rating}</p>
        )}

        <CokSatir
          etiket="Yorumunuz"
          data-autofocus
          rows={6}
          value={body}
          disabled={create.isPending}
          onChange={(e) => setBody(e.target.value)}
          hata={errs.body ?? (asim ? 'Yorum en fazla 1000 karakter olabilir.' : undefined)}
          ipucu={`${runeLength(body)}/${MAX_BODY} karakter`}
          placeholder="Hangi servis için numara aldınız, kod ne kadar sürede geldi, ne beğendiniz?"
        />

        <p className="max-w-[70ch] text-sm text-muted">
          Yorumunuz ekibimizin onayından sonra sitede kullanıcı adınızla yayımlanır.
          E-posta adresiniz hiçbir zaman gösterilmez.
        </p>

        {err && <HataDurumu hata={err} />}

        <div className="flex flex-col gap-2 sm:flex-row-reverse">
          <Button type="submit" loading={create.isPending} fullWidth className="sm:w-auto">
            Yorumu gönder
          </Button>
          <Button type="button" variant="outline" fullWidth className="sm:w-auto"
                  onClick={onClose} disabled={create.isPending}>
            Vazgeç
          </Button>
        </div>
      </form>
    </Modal>
  );
}
