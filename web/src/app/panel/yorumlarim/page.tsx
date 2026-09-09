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
 */

import * as React from 'react';
import { useMutation, useQuery, useQueryClient, keepPreviousData } from '@tanstack/react-query';
import { ApiError, apiFetch } from '@/lib/api';
import { formatDateTime } from '@/lib/format';
import { Card, Button, Alert, Badge, Skeleton, Empty, cx } from '@/components/ui';
import { Modal } from '@/components/modal';
import { Yildiz } from '@/components/ikonlar';

const PAGE = 20;

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

function statusTone(s: ReviewStatus): 'ok' | 'warn' | 'bad' | 'neutral' {
  switch (s) {
    case 'APPROVED': return 'ok';
    case 'PENDING': return 'warn';
    case 'REJECTED': return 'bad';
    default: return 'neutral';
  }
}

/** Hata gösterimi — mesaj + alan hataları + istek numarası (§9). */
function ErrorBox({ err, className }: { err: ApiError; className?: string }) {
  return (
    <Alert className={className}>
      <p>{err.message}</p>
      {err.fields?.length ? (
        <ul className="mt-1 list-inside list-disc">
          {err.fields.map((f) => <li key={f.field}>{f.message}</li>)}
        </ul>
      ) : null}
      {err.requestId && <p className="mt-2 text-xs opacity-60">İstek no: {err.requestId}</p>}
    </Alert>
  );
}

/** Girdi fontu 16px'ten KÜÇÜK OLAMAZ: iOS Safari odakta sayfayı yakınlaştırır. */
const textareaClass =
  'raised w-full rounded-xl border px-3.5 py-2.5 text-base outline-none ' +
  'focus:border-brand-400 disabled:opacity-60';

/** Salt okunur yıldız gösterimi. */
function Yildizlar({ puan }: { puan: number }) {
  return (
    <span className="flex items-center gap-0.5" aria-label={`5 üzerinden ${puan} puan`}>
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
    <fieldset disabled={disabled} className="flex flex-col gap-1.5">
      <legend className="text-sm font-medium">Puanınız</legend>
      <div className="flex items-center gap-1">
        {[1, 2, 3, 4, 5].map((n) => (
          <label
            key={n}
            /* Dokunma hedefi 44 px: p-2 + size-7 ikon = 44 px kenar. */
            className={cx(
              'cursor-pointer rounded-lg p-2 transition-colors',
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
    queryKey: ['reviews', 'mine', { limit: PAGE, offset }],
    queryFn: () => apiFetch<ReviewList>(`/reviews/mine?limit=${PAGE}&offset=${offset}`),
    // Sayfa değişince liste boşalıp zıplamasın.
    placeholderData: keepPreviousData,
  });

  const total = q.data?.total ?? 0;
  const hasPrev = offset > 0;
  const hasNext = offset + PAGE < total;
  const listErr = q.error instanceof ApiError ? q.error : null;

  // Onay bekleyen bir yorum varken yenisi gönderilemez (sunucu kısıtı:
  // reviews_one_pending_per_user_idx). Düğmeyi kapatmak sunucunun kararını
  // TEKRAR ETMEZ, yalnız kullanıcıyı 409 almadan uyarır.
  const bekleyen = q.data?.items.some((r) => r.status === 'PENDING') ?? false;

  return (
    <div className="mx-auto flex max-w-3xl flex-col gap-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-2xl font-bold tracking-tight md:text-3xl">Yorumlarım</h1>
          <p className="mt-1 text-sm text-muted">
            Deneyiminizi yazın. Yorumunuz ekibimizin onayından sonra sitede yayımlanır.
          </p>
        </div>
        <Button
          className="shrink-0 sm:w-auto"
          fullWidth
          disabled={bekleyen}
          onClick={() => setCreating(true)}
        >
          Yorum yaz
        </Button>
      </div>

      {bekleyen && (
        <Alert tone="info">
          Onay bekleyen bir yorumunuz var. O sonuçlandığında yenisini yazabilirsiniz.
        </Alert>
      )}

      <Card>
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h2 className="text-lg font-semibold">Gönderdiğim yorumlar</h2>
          {total > 0 && <Badge tone="neutral">{total} yorum</Badge>}
        </div>

        {q.isLoading ? (
          <div className="mt-4 flex flex-col gap-2">
            {[0, 1, 2].map((i) => <Skeleton key={i} className="h-24" />)}
          </div>
        ) : listErr ? (
          <ErrorBox err={listErr} className="mt-4" />
        ) : !q.data?.items.length ? (
          <Empty
            title="Henüz yorum yazmadınız"
            hint="Hizmetimizle ilgili düşüncelerinizi paylaşırsanız, onaydan sonra sitede yayımlanır."
          />
        ) : (
          <>
            {/* MOBİL VE MASAÜSTÜ AYNI: kart listesi. Yorum metni uzundur ve
                tabloya sığmaz; yatay kaydırılan tablo kabul edilmez (§2.5). */}
            <ul className="mt-4 flex flex-col gap-3">
              {q.data.items.map((r) => (
                <li key={r.id} className="raised rounded-xl border p-4">
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <Yildizlar puan={r.rating} />
                    <Badge tone={statusTone(r.status)}>{r.statusLabel}</Badge>
                  </div>

                  {/* whitespace-pre-wrap: satır sonları korunur, HTML yorumlanmaz.
                      break-anywhere: boşluksuz uzun bir dize 320 px'te yatay
                      kaydırma üretirdi. */}
                  <p className="mt-3 whitespace-pre-wrap break-anywhere text-sm leading-relaxed">
                    {r.body}
                  </p>

                  <dl className="mt-3 flex flex-col gap-1.5 border-t border-[var(--border)]
                                 pt-2 text-xs">
                    <div className="flex items-center justify-between gap-3">
                      <dt className="text-muted">Gönderildi</dt>
                      <dd>{formatDateTime(r.createdAt)}</dd>
                    </div>
                    {r.reviewedAt && (
                      <div className="flex items-center justify-between gap-3">
                        <dt className="text-muted">
                          {r.status === 'APPROVED' ? 'Yayımlandı' : 'Karar tarihi'}
                        </dt>
                        <dd>{formatDateTime(r.reviewedAt)}</dd>
                      </div>
                    )}
                  </dl>

                  {r.status === 'REJECTED' && r.rejectionReason && (
                    <Alert tone="warn" className="mt-3">
                      <p className="font-medium">Yayımlanmama nedeni</p>
                      <p className="mt-1 whitespace-pre-wrap break-anywhere">
                        {r.rejectionReason}
                      </p>
                      <p className="mt-2 opacity-80">
                        Dilerseniz yeni bir yorum yazabilirsiniz.
                      </p>
                    </Alert>
                  )}

                  {r.status === 'PENDING' && (
                    <p className="mt-3 text-xs text-muted">
                      Bu yorum henüz sitede görünmüyor; onaydan sonra yayımlanacak.
                    </p>
                  )}
                </li>
              ))}
            </ul>

            {(hasPrev || hasNext) && (
              <div className="mt-4 flex items-center justify-between gap-3">
                <Button variant="outline" size="sm" disabled={!hasPrev}
                        onClick={() => setOffset((o) => Math.max(0, o - PAGE))}>
                  Önceki
                </Button>
                <span className="text-xs text-muted">
                  {offset + 1}–{Math.min(offset + PAGE, total)} / {total}
                </span>
                <Button variant="outline" size="sm" disabled={!hasNext}
                        onClick={() => setOffset((o) => o + PAGE)}>
                  Sonraki
                </Button>
              </div>
            )}
          </>
        )}
      </Card>

      {creating && <YorumYazDialog onClose={() => setCreating(false)} />}
    </div>
  );
}

/* ═══════════════════════ Yeni yorum ═══════════════════════ */

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

  const err = create.error instanceof ApiError ? create.error : null;
  const kalan = MAX_BODY - runeLength(body);

  return (
    <Modal open onClose={onClose} title="Yorum yaz">
      <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
        <PuanSecici value={rating} onChange={setRating} disabled={create.isPending} />
        {errs.rating && (
          <span role="alert" className="text-xs text-[var(--color-bad)]">{errs.rating}</span>
        )}

        <label className="flex flex-col gap-1.5">
          <span className="text-sm font-medium">Yorumunuz</span>
          <textarea
            data-autofocus
            className={textareaClass}
            rows={6}
            value={body}
            disabled={create.isPending}
            onChange={(e) => setBody(e.target.value)}
            aria-invalid={errs.body ? true : undefined}
            placeholder="Hangi servis için numara aldınız, kod ne kadar sürede geldi, ne beğendiniz?"
          />
          <span className={cx('text-xs', kalan < 0 ? 'text-[var(--color-bad)]' : 'text-muted')}>
            {runeLength(body)}/{MAX_BODY} karakter
          </span>
          {errs.body && (
            <span role="alert" className="text-xs text-[var(--color-bad)]">{errs.body}</span>
          )}
        </label>

        <p className="text-xs text-muted">
          Yorumunuz ekibimizin onayından sonra sitede kullanıcı adınızla yayımlanır.
          E-posta adresiniz hiçbir zaman gösterilmez.
        </p>

        {err && <ErrorBox err={err} />}

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
