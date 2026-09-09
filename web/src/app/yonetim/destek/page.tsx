'use client';

/**
 * Destek talepleri — yönetim ekranı (FR-600).
 *
 * VARSAYILAN GÖRÜNÜM "bekleyenler"dir ve bu TEK BİR DURUM DEĞİLDİR: hem yeni
 * açılan (OPEN) hem de kullanıcının yanıt yazdığı (USER_REPLIED) talepler
 * personelin işidir. Yalnız OPEN süzmek, yanıt bekleyen müşteriyi varsayılan
 * ekranda görünmez yapardı — bu yüzden sunucuda ayrı bir `?pending=true`
 * süzgeci var.
 *
 * Ekrandaki gövde metinlerinin tamamı KULLANICI GİRDİSİDİR. React kaçırır;
 * `dangerouslySetInnerHTML` bu dosyada YOKTUR.
 */

import * as React from 'react';
import { useMutation, useQuery, useQueryClient, keepPreviousData } from '@tanstack/react-query';
import { ApiError, apiFetch } from '@/lib/api';
import { formatDateTime } from '@/lib/format';
import { Card, Button, Alert, Badge, Skeleton, Empty } from '@/components/ui';
import { Modal } from '@/components/modal';

const PAGE = 20;

/* ═══════════════════════ Sunucu sözleşmesi ═══════════════════════ */
/* Karşılıkları: api/internal/transport/http/dto/ticket.go
 * Tipler burada tanımlıdır çünkü `lib/types.ts` bu turda paylaşılan bir
 * dosyadır; kalıcı hâle geldiğinde oraya taşınmalıdır (rapora yazıldı). */

type TicketStatus = 'OPEN' | 'ANSWERED' | 'USER_REPLIED' | 'CLOSED';

interface TicketMessage {
  id: string;
  body: string;
  isStaff: boolean;
  authorLabel: string;
  /** Yönetim görünümünde yazarın kullanıcı adı gelir; kullanıcı görünümünde GELMEZ. */
  authorUsername?: string;
  createdAt: string;
}

interface AdminTicket {
  id: string;
  userId: string;
  userEmail: string;
  userUsername: string;
  subject: string;
  priority: 'LOW' | 'NORMAL' | 'HIGH';
  priorityLabel: string;
  status: TicketStatus;
  statusLabel: string;
  messageCount: number;
  createdAt: string;
  lastReplyAt: string;
  closedAt?: string;
  messages?: TicketMessage[];
}

interface AdminTicketList {
  items: AdminTicket[];
  total: number;
  limit: number;
  offset: number;
}

const MAX_BODY = 4000;
const runeLength = (s: string) => Array.from(s).length;

/** Süzgeç değerleri. `pending` bir durum değil, bir GÖRÜNÜMDÜR. */
type Filter = 'pending' | TicketStatus | 'all';

const FILTERS: Array<{ value: Filter; label: string }> = [
  // Varsayılan ilk sıradadır: yöneticinin işi bekleyen taleplerdir.
  { value: 'pending', label: 'Bekleyenler' },
  { value: 'OPEN', label: 'Yeni açılanlar' },
  { value: 'USER_REPLIED', label: 'Kullanıcı yanıtladı' },
  { value: 'ANSWERED', label: 'Yanıtlananlar' },
  { value: 'CLOSED', label: 'Kapatılanlar' },
  { value: 'all', label: 'Tümü' },
];

function queryFor(f: Filter, offset: number): string {
  const qs = new URLSearchParams({ limit: String(PAGE), offset: String(offset) });
  if (f === 'pending') qs.set('pending', 'true');
  else if (f !== 'all') qs.set('status', f);
  return qs.toString();
}

function statusTone(s: TicketStatus): 'ok' | 'warn' | 'brand' | 'neutral' {
  switch (s) {
    case 'ANSWERED': return 'ok';
    case 'OPEN': return 'warn';
    case 'USER_REPLIED': return 'brand';
    default: return 'neutral';
  }
}

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

/** `appearance-none` ZORUNLU: WebKit'te yerel select 44 px hedefin altına düşer. */
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

/* ═══════════════════════ Sayfa ═══════════════════════ */

export default function AdminTicketsPage() {
  const [filter, setFilter] = React.useState<Filter>('pending');
  const [offset, setOffset] = React.useState(0);
  const [openId, setOpenId] = React.useState<string | null>(null);

  const q = useQuery({
    queryKey: ['admin-tickets', { filter, offset }],
    queryFn: () => apiFetch<AdminTicketList>(`/admin/tickets?${queryFor(filter, offset)}`),
    placeholderData: keepPreviousData,
  });

  const total = q.data?.total ?? 0;
  const hasPrev = offset > 0;
  const hasNext = offset + PAGE < total;
  const listErr = q.error instanceof ApiError ? q.error : null;

  return (
    <div className="mx-auto flex max-w-5xl flex-col gap-5">
      <div>
        <h1 className="text-2xl font-bold tracking-tight md:text-3xl">Destek talepleri</h1>
        <p className="mt-1 text-sm text-muted">
          Kullanıcı taleplerini okuyun, yanıtlayın ve sonuçlananları kapatın.
        </p>
      </div>

      <Card>
        <div className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
          <label className="flex w-full flex-col gap-1.5 sm:max-w-xs">
            <span className="text-sm font-medium">Görünüm</span>
            <div className="relative">
              <select
                className={selectClass}
                value={filter}
                onChange={(e) => {
                  setFilter(e.target.value as Filter);
                  setOffset(0); // süzgeç değişti; eski sayfa numarası anlamsız
                }}
              >
                {FILTERS.map((f) => (
                  <option key={f.value} value={f.value}>{f.label}</option>
                ))}
              </select>
              <Chevron />
            </div>
          </label>
          {total > 0 && <div className="shrink-0"><Badge tone="neutral">{total} kayıt</Badge></div>}
        </div>

        {q.isLoading ? (
          <div className="mt-4 flex flex-col gap-2">
            {[0, 1, 2, 3].map((i) => <Skeleton key={i} className="h-24" />)}
          </div>
        ) : listErr ? (
          <ErrorBox err={listErr} className="mt-4" />
        ) : !q.data?.items.length ? (
          <Empty
            title="Talep yok"
            hint={filter === 'pending'
              ? 'Yanıt bekleyen destek talebi bulunmuyor.'
              : 'Bu görünüme uyan bir talep bulunmuyor.'}
          />
        ) : (
          <>
            {/* MOBİL: kart listesi. Yatay kaydırılan tablo kabul edilmez (§2.5). */}
            <ul className="mt-4 flex flex-col gap-2 md:hidden">
              {q.data.items.map((t) => (
                <li key={t.id} className="raised rounded-xl border p-3">
                  <div className="flex items-start justify-between gap-3">
                    <p className="min-w-0 text-sm font-medium break-anywhere">{t.subject}</p>
                    <Badge tone={statusTone(t.status)}>{t.statusLabel}</Badge>
                  </div>
                  <dl className="mt-3 flex flex-col gap-1.5 border-t border-[var(--border)] pt-2 text-xs">
                    <div className="flex items-center justify-between gap-3">
                      <dt className="text-muted">Kullanıcı</dt>
                      <dd className="min-w-0 truncate">{t.userUsername}</dd>
                    </div>
                    <div className="flex items-center justify-between gap-3">
                      <dt className="text-muted">Öncelik</dt>
                      <dd>{t.priorityLabel}</dd>
                    </div>
                    <div className="flex items-center justify-between gap-3">
                      <dt className="text-muted">Son hareket</dt>
                      <dd>{formatDateTime(t.lastReplyAt)}</dd>
                    </div>
                    <div className="flex items-center justify-between gap-3">
                      <dt className="text-muted">Mesaj</dt>
                      <dd>{t.messageCount}</dd>
                    </div>
                  </dl>
                  <Button variant="outline" size="sm" fullWidth className="mt-3"
                          onClick={() => setOpenId(t.id)}>
                    Yazışmayı aç
                  </Button>
                </li>
              ))}
            </ul>

            {/* MASAÜSTÜ: gerçek tablo. "Öncelik" lg: altında gizlenir ve bilgi
                kaybolmaz — karttaki ve diyalogdaki yerinde durur. */}
            <div className="mt-4 hidden md:block">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-[var(--border)] text-left text-xs
                                 uppercase tracking-wide text-muted">
                    <th scope="col" className="py-2 pr-3 font-medium">Konu</th>
                    <th scope="col" className="py-2 pr-3 font-medium">Kullanıcı</th>
                    <th scope="col" className="hidden py-2 pr-3 font-medium lg:table-cell">Öncelik</th>
                    <th scope="col" className="py-2 pr-3 font-medium">Durum</th>
                    <th scope="col" className="py-2 pr-3 font-medium">Son hareket</th>
                    <th scope="col" className="py-2 text-right font-medium">
                      <span className="sr-only">İşlem</span>
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {q.data.items.map((t) => (
                    <tr key={t.id} className="border-b border-[var(--border)] align-top last:border-0">
                      <td className="max-w-[14rem] py-3 pr-3">
                        <span className="block truncate font-medium">{t.subject}</span>
                        <span className="block text-xs text-muted">{t.messageCount} mesaj</span>
                      </td>
                      <td className="max-w-[10rem] py-3 pr-3">
                        <span className="block truncate">{t.userUsername}</span>
                        <span className="block truncate text-xs text-muted">{t.userEmail}</span>
                      </td>
                      <td className="hidden py-3 pr-3 lg:table-cell">{t.priorityLabel}</td>
                      <td className="py-3 pr-3">
                        <Badge tone={statusTone(t.status)}>{t.statusLabel}</Badge>
                      </td>
                      <td className="py-3 pr-3 whitespace-nowrap text-muted">
                        {formatDateTime(t.lastReplyAt)}
                      </td>
                      <td className="py-3 text-right">
                        <Button variant="outline" size="sm" onClick={() => setOpenId(t.id)}>
                          Aç
                        </Button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

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

      {openId && <AdminThreadDialog id={openId} onClose={() => setOpenId(null)} />}
    </div>
  );
}

/* ═══════════════════════ Yazışma diyaloğu ═══════════════════════ */

function AdminThreadDialog({ id, onClose }: { id: string; onClose: () => void }) {
  const qc = useQueryClient();
  const [message, setMessage] = React.useState('');
  const [formErr, setFormErr] = React.useState('');

  const q = useQuery({
    queryKey: ['admin-ticket', id],
    queryFn: () => apiFetch<AdminTicket>(`/admin/tickets/${encodeURIComponent(id)}`),
  });

  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ['admin-ticket', id] });
    qc.invalidateQueries({ queryKey: ['admin-tickets'] });
  };

  const reply = useMutation({
    mutationFn: (body: string) =>
      apiFetch<AdminTicket>(`/admin/tickets/${encodeURIComponent(id)}/messages`, {
        method: 'POST', body: { message: body },
      }),
    // MUTASYON OTOMATİK TEKRARLANMAZ (değişmez #16): tekrar, kullanıcıya aynı
    // yanıtı iki kez göndermek demektir.
    retry: false,
    onSuccess: () => { setMessage(''); invalidate(); },
  });

  const setStatus = useMutation({
    mutationFn: (status: 'OPEN' | 'CLOSED') =>
      apiFetch<AdminTicket>(`/admin/tickets/${encodeURIComponent(id)}/status`, {
        // PATCH: durum DEĞİŞTİRİR, GET olamaz (değişmez #8).
        method: 'PATCH', body: { status },
      }),
    retry: false,
    onSuccess: invalidate,
  });

  const t = q.data;
  const loadErr = q.error instanceof ApiError ? q.error : null;
  const mutErr =
    (reply.error instanceof ApiError && reply.error) ||
    (setStatus.error instanceof ApiError && setStatus.error) ||
    null;
  const closed = t?.status === 'CLOSED';
  const busy = reply.isPending || setStatus.isPending;

  function submit(e: React.FormEvent) {
    e.preventDefault();
    const m = message.trim();
    if (m.length === 0) { setFormErr('Mesaj boş olamaz.'); return; }
    if (runeLength(m) > MAX_BODY) { setFormErr('Mesaj en fazla 4000 karakter olabilir.'); return; }
    setFormErr('');
    reply.mutate(m);
  }

  return (
    <Modal open onClose={onClose} title={t?.subject ?? 'Destek talebi'}>
      {q.isLoading ? (
        <div className="flex flex-col gap-2">
          {[0, 1].map((i) => <Skeleton key={i} className="h-20" />)}
        </div>
      ) : loadErr ? (
        <ErrorBox err={loadErr} />
      ) : !t ? (
        <Empty title="Talep bulunamadı" />
      ) : (
        <div className="flex flex-col gap-4">
          <div className="flex flex-wrap items-center gap-2">
            <Badge tone={statusTone(t.status)}>{t.statusLabel}</Badge>
            <Badge tone="neutral">Öncelik: {t.priorityLabel}</Badge>
          </div>

          <dl className="raised rounded-xl border p-3 text-xs">
            <div className="flex items-center justify-between gap-3">
              <dt className="text-muted">Kullanıcı</dt>
              <dd className="min-w-0 truncate font-medium">{t.userUsername}</dd>
            </div>
            <div className="mt-1.5 flex items-center justify-between gap-3">
              <dt className="text-muted">E-posta</dt>
              <dd className="min-w-0 truncate break-anywhere">{t.userEmail}</dd>
            </div>
            <div className="mt-1.5 flex items-center justify-between gap-3">
              <dt className="text-muted">Açılış</dt>
              <dd>{formatDateTime(t.createdAt)}</dd>
            </div>
            {t.closedAt && (
              <div className="mt-1.5 flex items-center justify-between gap-3">
                <dt className="text-muted">Kapanış</dt>
                <dd>{formatDateTime(t.closedAt)}</dd>
              </div>
            )}
          </dl>

          {!t.messages?.length ? (
            <Empty title="Bu talepte mesaj yok" />
          ) : (
            <ul className="flex flex-col gap-3">
              {t.messages.map((m) => (
                <li
                  key={m.id}
                  className={
                    m.isStaff
                      ? 'rounded-xl border border-brand-500/30 bg-brand-500/10 p-3'
                      : 'raised rounded-xl border p-3'
                  }
                >
                  <div className="flex flex-wrap items-baseline justify-between gap-2">
                    <span className="text-sm font-semibold">
                      {/* Yönetim görünümünde YAZARIN ADI görünür: "kim yanıtladı"
                          destek ekibinin iç sorusudur. Kullanıcı görünümünde bu
                          alan sunucudan HİÇ gelmez. */}
                      {m.isStaff ? `${m.authorLabel}${m.authorUsername ? ` · ${m.authorUsername}` : ''}`
                                 : t.userUsername}
                    </span>
                    <span className="text-xs text-muted">{formatDateTime(m.createdAt)}</span>
                  </div>
                  {/* DÜZ METİN — `dangerouslySetInnerHTML` YOKTUR. Talebi okuyan
                      yöneticinin oturumunda kullanıcı betiği çalışamaz. */}
                  <p className="mt-2 text-sm leading-relaxed whitespace-pre-wrap break-anywhere">
                    {m.body}
                  </p>
                </li>
              ))}
            </ul>
          )}

          {mutErr && <ErrorBox err={mutErr} />}

          {closed ? (
            <div className="flex flex-col gap-2 border-t border-[var(--border)] pt-4">
              <Alert tone="info">
                Bu talep kapalı. Kullanıcı kapalı bir talebe yazamaz; yazışmaya devam
                edilmesi gerekiyorsa talebi yeniden açın.
              </Alert>
              <Button variant="outline" fullWidth className="sm:w-auto sm:self-end"
                      loading={setStatus.isPending} onClick={() => setStatus.mutate('OPEN')}>
                Talebi yeniden aç
              </Button>
            </div>
          ) : (
            <form onSubmit={submit} className="flex flex-col gap-2 border-t
                                               border-[var(--border)] pt-4" noValidate>
              <label className="flex flex-col gap-1.5">
                <span className="text-sm font-medium">Yanıtınız</span>
                <textarea
                  data-autofocus
                  className={textareaClass}
                  rows={4}
                  value={message}
                  disabled={busy}
                  onChange={(e) => setMessage(e.target.value)}
                  aria-invalid={formErr ? true : undefined}
                />
                <span className="text-xs text-muted">{runeLength(message)}/{MAX_BODY} karakter</span>
                {formErr && (
                  <span role="alert" className="text-xs text-[var(--color-bad)]">{formErr}</span>
                )}
              </label>
              <div className="flex flex-col gap-2 sm:flex-row-reverse">
                <Button type="submit" loading={reply.isPending} disabled={busy}
                        fullWidth className="sm:w-auto">
                  Yanıtla
                </Button>
                <Button type="button" variant="outline" fullWidth className="sm:w-auto"
                        loading={setStatus.isPending} disabled={busy}
                        onClick={() => setStatus.mutate('CLOSED')}>
                  Talebi kapat
                </Button>
              </div>
            </form>
          )}
        </div>
      )}
    </Modal>
  );
}
