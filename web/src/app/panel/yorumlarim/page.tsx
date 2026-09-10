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
  Secim,
  SuzgecCubugu,
  DurumRozeti,
  HataDurumu,
  VeriTablosu,
  apiHatasi,
  sayfalamaGorunur,
} from '@/components/yonetim';
import type { Sunum, Sutun } from '@/components/yonetim';

/**
 * Durum süzgeci seçenekleri — `GET /reviews/mine?status=`.
 *
 * 🔴 SATIR ETİKETLERİ SUNUCUDAN gelir (`statusLabel`); bu liste onları ÜRETMEZ.
 * Kaynak: `api/internal/transport/http/handler/review.go#reviewStatusLabel`.
 */
const DURUM_SUZGECLERI: ReadonlyArray<{ value: string; label: string }> = [
  { value: '', label: 'Tüm durumlar' },
  { value: 'PENDING', label: 'Onay bekliyor' },
  { value: 'APPROVED', label: 'Yayında' },
  { value: 'REJECTED', label: 'Yayımlanmadı' },
];

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

/**
 * Yorumun gövdesi + duruma özgü notlar — tablo hücresi ve mobil kart için TEK
 * tanım.
 *
 * 🔴 `sunum` YALNIZ HİZALAMA İÇİN OKUNUR, metin ikisinde de aynıdır. Mobil
 * kartta bu hücre bir `<dd>` içine düşer ve `VeriTablosu` `<dd>`'yi sağa
 * hizalar — kısa değerler için doğru, ÇOK SATIRLI DÜZ METİN için yanlıştır:
 * sağa yaslı bir paragrafın sol kenarı tırtıklı olur ve okuma hızını düşürür.
 * `sunum`'a bakıp farklı METİN yazmak yasaktır; farklı HİZA vermek bileşenin
 * açıkça izin verdiği kullanımdır.
 */
function YorumGovdesi({ yorum, sunum }: { yorum: Review; sunum: Sunum }) {
  return (
    <div className={sunum === 'kart' ? 'text-left font-normal' : undefined}>
      {/* 65-75ch bandı (§3.4), kısaltma YOK. `whitespace-pre-wrap` satır
          sonlarını korur, HTML yorumlamaz. `break-anywhere`: boşluksuz uzun
          bir dize 320px'te yatay kaydırma üretirdi. */}
      <p className="max-w-[70ch] whitespace-pre-wrap break-anywhere leading-relaxed">
        {yorum.body}
      </p>

      {yorum.status === 'REJECTED' && yorum.rejectionReason && (
        <Alert tone="warn" duyur={false} className="mt-4">
          <p className="font-medium">Yayımlanmama nedeni</p>
          <p className="mt-1 max-w-[70ch] whitespace-pre-wrap break-anywhere">
            {yorum.rejectionReason}
          </p>
          <p className="mt-2">Dilerseniz yeni bir yorum yazabilirsiniz.</p>
        </Alert>
      )}

      {yorum.status === 'PENDING' && (
        <p className="mt-3 text-sm text-muted">
          Bu yorum henüz sitede görünmüyor; onaydan sonra yayımlanacak.
        </p>
      )}
    </div>
  );
}

/* ═══════════════════════ Sayfa ═══════════════════════ */

export default function YorumlarimPage() {
  const [offset, setOffset] = React.useState(0);
  const [creating, setCreating] = React.useState(false);

  const [durum, setDurum] = React.useState('');

  const q = useQuery({
    queryKey: ['reviews', 'mine', { limit: SAYFA_BOYUTU, offset, durum }],
    queryFn: () => {
      // Süzgeç SUNUCUDA uygulanır; istemcide süzmek yalnız bu sayfadaki 25
      // satırı görürdü.
      const p = new URLSearchParams({
        limit: String(SAYFA_BOYUTU),
        offset: String(offset),
      });
      if (durum) p.set('status', durum);
      return apiFetch<ReviewList>(`/reviews/mine?${p.toString()}`);
    },
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
   * `Sayfalama` GÖRÜNÜR MÜ? Koşul bileşenin kendisinden okunur
   * (`sayfalamaGorunur`), burada kopyalanmaz — kopya, bileşenin gizlenme
   * kuralı değiştiği gün sessizce yanlış olur ve iki canlı bölge birden
   * konuşmaya başlar.
   */
  const sayfali = sayfalamaGorunur(total);

  /*
   * KENDİ CANLI BÖLGESİ KALKTI. Liste artık `VeriTablosu` ile çiziliyor ve
   * duyuruyu O yapıyor (`duyuru` prop'u). İkisi birden dursaydı ekran okuyucu
   * aynı olayı iki kez duyururdu.
   *
   * `Sayfalama` tek sayfaya sığan listede kendini hiç çizmez; `sayfali`
   * "sayfalama ekranda var mı" ile aynı şeydir ve sayfalıyken duyuruyu ONA
   * bırakırız: `items.length` SAYFADAKİ sayıdır, 87 kayıtlık listede her
   * sayfada "25 yorum listelendi" derdi.
   */

  /*
   * SÜTUNLAR — tek veri tanımı (§6.1 kural 2). Ekran eskiden elle bir kart
   * listesi çiziyordu; `VeriTablosu` masaüstünde gerçek tablo, `< md` altında
   * kart üretir ve ikisi AYNI tanımdan gelir.
   */
  const sutunlar = React.useMemo<ReadonlyArray<Sutun<Review>>>(
    () => [
      {
        anahtar: 'puan',
        baslik: 'Puan',
        mobilRol: 'baslik',
        hucre: (r) => <Yildizlar puan={r.rating} />,
      },
      {
        anahtar: 'durum',
        baslik: 'Durum',
        mobilRol: 'rozet',
        hucre: (r) => <DurumRozeti durum={r.status} etiket={r.statusLabel} />,
      },
      {
        anahtar: 'yorum',
        baslik: 'Yorum',
        hucre: (r, sunum) => <YorumGovdesi yorum={r} sunum={sunum} />,
      },
      {
        anahtar: 'tarih',
        baslik: 'Gönderildi',
        hizala: 'sag',
        // Tarih veridir: `tabular-nums` olmadan alt alta gelen tarihler kayar.
        sayisal: true,
        // Dört sütun 768px'e sığar ama yorum gövdesi genişliğin çoğunu ister;
        // tarih `lg:` üstüne alındı — mobil kartta yine tam metinle var.
        oncelik: 3,
        hucre: (r) => (
          <span className="text-muted">
            {formatDateTime(r.createdAt)}
            {r.reviewedAt && (
              <>
                {' · '}
                {r.status === 'APPROVED' ? 'Yayımlandı' : 'Karar'}:{' '}
                {formatDateTime(r.reviewedAt)}
              </>
            )}
          </span>
        ),
      },
    ],
    [],
  );

  return (
    <div className="flex flex-col gap-6">
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

        {/* Süzgeç liste kartının İÇİNDE: "Yorum yaz" düğmesi zaten sayfa
            başlığında duruyor, araya üçüncü bir kart girmez. */}
        <SuzgecCubugu
          etkinSayisi={durum ? 1 : 0}
          onTemizle={() => {
            setDurum('');
            setOffset(0);
          }}
        >
          <div className="w-full sm:w-60 sm:self-start">
            <Secim
              etiket="Durum"
              value={durum}
              onChange={(e) => {
                setDurum(e.target.value);
                // Süzgeç değişince 3. sayfada kalmak BOŞ ekran gösterir.
                setOffset(0);
              }}
            >
              {DURUM_SUZGECLERI.map((d) => (
                <option key={d.value || 'tumu'} value={d.value}>
                  {d.label}
                </option>
              ))}
            </Secim>
          </div>
        </SuzgecCubugu>

        <VeriTablosu
          baslik="Gönderdiğim yorumlar"
          sutunlar={sutunlar}
          satirlar={items}
          satirAnahtari={(r) => r.id}
          yukleniyor={q.isLoading}
          hata={listErr}
          // İskelet satırı gerçek satır yüksekliğinden türer; üç satır, eski
          // elle yazılmış iskeletle aynı sayı.
          iskeletSatir={3}
          // Sayfalama kendi aralığını duyuruyor; iki canlı bölge aynı anda
          // konuşmasın (veri-tablosu.tsx `duyuru` notu).
          duyuru={!sayfali}
          bos={
            /* Boş durum ÖĞRETİR (§5.1). §6.3: SÜZGEÇTEN DOLAYI BOŞ ≠ GERÇEKTEN
               BOŞ — yorumu olan ama seçtiği durumda kaydı olmayan kullanıcıya
               "Henüz yorum yazmadınız" demek yanlıştır ve yanlış eyleme iter. */
            durum ? (
              <Empty
                title="Bu durumda yorum yok"
                hint="Durumu “Tüm durumlar” yaparak gönderdiğiniz bütün yorumları görebilirsiniz."
              />
            ) : (
              <Empty
                title="Henüz yorum yazmadınız"
                hint="Hizmetimizle ilgili düşüncelerinizi paylaşırsanız, onaydan sonra sitede kullanıcı adınızla yayımlanır."
              />
            )
          }
        />

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
