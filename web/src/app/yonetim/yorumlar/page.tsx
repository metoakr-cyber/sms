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

/**
 * Ham `<select>` stili.
 *
 * `appearance-none` ZORUNLUDUR: WebKit'te yerel `menulist` görünümü yüksekliği
 * kendi hesaplar ve `min-h-12`'yi yok sayar — kutu 44 px dokunma hedefinin
 * altına düşer (yonetim/talepler/page.tsx ile aynı kalıp).
 */
const selectClass =
  'raised min-h-12 w-full appearance-none rounded-xl border px-3 pr-10 text-base ' +
  'outline-none focus:border-brand-400 disabled:opacity-60';

const textareaClass =
  'raised w-full rounded-xl border px-3.5 py-2.5 text-base outline-none ' +
  'focus:border-brand-400 disabled:opacity-60';

function Chevron() {
  return (
    <svg
      className="pointer-events-none absolute right-3 top-1/2 size-4 -translate-y-1/2 text-[var(--muted)]"
      viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"
      strokeLinecap="round" strokeLinejoin="round" aria-hidden
    >
      <path d="M6 9l6 6 6-6" />
    </svg>
  );
}

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

/* ═══════════════════════ Sayfa ═══════════════════════ */

export default function YonetimYorumlarPage() {
  const [status, setStatus] = React.useState<'' | ReviewStatus>('PENDING');
  const [offset, setOffset] = React.useState(0);
  const [karar, setKarar] = React.useState<{ review: AdminReview; tur: 'onay' | 'red' } | null>(null);

  const query = new URLSearchParams({ limit: String(PAGE), offset: String(offset) });
  if (status) query.set('status', status);

  const q = useQuery({
    queryKey: ['admin', 'reviews', { status, offset }],
    queryFn: () => apiFetch<AdminReviewList>(`/admin/reviews?${query.toString()}`),
    placeholderData: keepPreviousData,
  });

  const total = q.data?.total ?? 0;
  const hasPrev = offset > 0;
  const hasNext = offset + PAGE < total;
  const listErr = q.error instanceof ApiError ? q.error : null;

  return (
    <div className="mx-auto flex max-w-4xl flex-col gap-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-2xl font-bold tracking-tight md:text-3xl">Müşteri yorumları</h1>
          <p className="mt-1 text-sm text-muted">
            Onayladığınız yorumlar sitede kullanıcı adıyla yayımlanır. E-posta hiçbir
            zaman gösterilmez.
          </p>
        </div>
        {(q.data?.pendingTotal ?? 0) > 0 && (
          <Badge tone="warn">{q.data?.pendingTotal} yorum karar bekliyor</Badge>
        )}
      </div>

      <Card>
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <label className="relative w-full sm:max-w-xs">
            <span className="sr-only">Durum süzgeci</span>
            <select
              className={selectClass}
              value={status}
              onChange={(e) => {
                setStatus(e.target.value as '' | ReviewStatus);
                setOffset(0);
              }}
            >
              {STATUS_FILTERS.map((f) => (
                <option key={f.value || 'all'} value={f.value}>{f.label}</option>
              ))}
            </select>
            <Chevron />
          </label>
          {total > 0 && <Badge tone="neutral">{total} kayıt</Badge>}
        </div>

        {q.isLoading ? (
          <div className="mt-4 flex flex-col gap-2">
            {[0, 1, 2].map((i) => <Skeleton key={i} className="h-28" />)}
          </div>
        ) : listErr ? (
          <ErrorBox err={listErr} className="mt-4" />
        ) : !q.data?.items.length ? (
          <Empty
            title="Bu süzgeçte yorum yok"
            hint={status === 'PENDING'
              ? 'Karar bekleyen yorum yok. Kuyruk temiz.'
              : 'Başka bir durum seçerek listeyi genişletebilirsiniz.'}
          />
        ) : (
          <>
            <ul className="mt-4 flex flex-col gap-3">
              {q.data.items.map((r) => (
                <li key={r.id} className="raised rounded-xl border p-4">
                  <div className="flex flex-wrap items-start justify-between gap-2">
                    <div className="min-w-0">
                      <p className="truncate text-sm font-semibold">{r.userUsername}</p>
                      {/* E-posta YÖNETİM ekranında görünür (moderasyon kararı
                          kimin yazdığını bilmeden verilemez) ama SİTEDE ASLA. */}
                      <p className="truncate text-xs text-muted">{r.userEmail}</p>
                    </div>
                    <div className="flex items-center gap-2">
                      <Yildizlar puan={r.rating} />
                      <Badge tone={statusTone(r.status)}>{r.statusLabel}</Badge>
                    </div>
                  </div>

                  <p className="mt-3 whitespace-pre-wrap break-anywhere text-sm leading-relaxed">
                    {r.body}
                  </p>

                  <p className="mt-3 text-xs text-muted">
                    Gönderildi: {formatDateTime(r.createdAt)}
                    {r.reviewedAt && <> · Karar: {formatDateTime(r.reviewedAt)}</>}
                  </p>

                  {r.status === 'REJECTED' && r.rejectionReason && (
                    <p className="mt-2 whitespace-pre-wrap break-anywhere rounded-lg
                                  bg-[var(--raised)] p-2 text-xs text-muted">
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
                    <div className="mt-3 flex flex-col gap-2 sm:flex-row">
                      {r.status === 'PENDING' && (
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

/* ═══════════════════════ Karar diyaloğu ═══════════════════════ */

/**
 * Onay ve red TEK diyalogdadır: ikisi de aynı metni yeniden gösterip
 * "bunu mu yayımlıyorsunuz / kaldırıyorsunuz" diye sorar. İki ayrı diyalog
 * yazmak, birinde eklenen bir onay adımının diğerinde unutulmasını
 * kolaylaştırırdı.
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

  function submit(e: React.FormEvent) {
    e.preventDefault();
    if (tur === 'red') {
      const r = reason.trim();
      if (r.length === 0) {
        // Gerekçe kullanıcıya gösterilir; boş bırakmak "reddedildi" deyip
        // sebebini söylememektir. Sunucu da reddeder (422).
        setFormErr('Red gerekçesi zorunludur; kullanıcıya gösterilecek.');
        return;
      }
      if (runeLength(r) > MAX_REASON) {
        setFormErr('Gerekçe en fazla 500 karakter olabilir.');
        return;
      }
    }
    setFormErr('');
    mut.mutate();
  }

  const err = mut.error instanceof ApiError ? mut.error : null;
  const kaldiriliyor = tur === 'red' && review.status === 'APPROVED';

  return (
    <Modal
      open
      onClose={onClose}
      title={tur === 'onay' ? 'Yorumu yayımla' : kaldiriliyor ? 'Yayından kaldır' : 'Yorumu reddet'}
    >
      <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
        {/* İKİNCİ ADIM YORUMU YENİDEN GÖSTERİR: yönetici listede yanlış
            satıra basmış olabilir. */}
        <div className="raised rounded-xl border p-3">
          <div className="flex items-center justify-between gap-2">
            <span className="truncate text-sm font-semibold">{review.userUsername}</span>
            <Yildizlar puan={review.rating} />
          </div>
          <p className="mt-2 whitespace-pre-wrap break-anywhere text-sm leading-relaxed">
            {review.body}
          </p>
        </div>

        {tur === 'onay' ? (
          <p className="text-sm text-muted">
            Bu yorum sitede <strong className="font-semibold text-[var(--text)]">
            {review.userUsername}</strong> adıyla yayımlanacak. E-posta gösterilmez.
          </p>
        ) : (
          <>
            {kaldiriliyor && (
              <Alert tone="warn">
                Bu yorum şu anda sitede yayında. Kaldırdıktan sonra geri alınamaz;
                kullanıcı isterse yeni bir yorum yazabilir.
              </Alert>
            )}
            <label className="flex flex-col gap-1.5">
              <span className="text-sm font-medium">
                Gerekçe <span className="text-muted">(kullanıcıya gösterilir)</span>
              </span>
              <textarea
                data-autofocus
                className={textareaClass}
                rows={4}
                value={reason}
                disabled={mut.isPending}
                onChange={(e) => setReason(e.target.value)}
                aria-invalid={formErr ? true : undefined}
                placeholder="Örn: Yorum reklam bağlantısı içeriyor."
              />
              <span className="text-xs text-muted">{runeLength(reason)}/{MAX_REASON} karakter</span>
              {formErr && (
                <span role="alert" className="text-xs text-[var(--color-bad)]">{formErr}</span>
              )}
            </label>
          </>
        )}

        {err && <ErrorBox err={err} />}

        <div className="flex flex-col gap-2 sm:flex-row-reverse">
          <Button type="submit" loading={mut.isPending} fullWidth className="sm:w-auto">
            {tur === 'onay' ? 'Onayla ve yayımla' : kaldiriliyor ? 'Yayından kaldır' : 'Reddet'}
          </Button>
          <Button type="button" variant="outline" fullWidth className="sm:w-auto"
                  onClick={onClose} disabled={mut.isPending}>
            Vazgeç
          </Button>
        </div>
      </form>
    </Modal>
  );
}
