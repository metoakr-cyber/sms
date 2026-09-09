'use client';

/**
 * Destek talepleri — kullanıcı ekranı (FR-600).
 *
 * Bu ekranda GÖSTERİLEN METNİN TAMAMI KULLANICI GİRDİSİDİR (kendi mesajları ve
 * personelin yanıtları). React metni varsayılan olarak kaçırır;
 * `dangerouslySetInnerHTML` bu dosyada YOKTUR ve olmayacaktır. Kullanıcı
 * mesajları düz metin olarak, `whitespace-pre-wrap` ile satır sonları
 * korunarak basılır — biçimlendirme HTML'e değil CSS'e bırakılır.
 */

import * as React from 'react';
import { useMutation, useQuery, useQueryClient, keepPreviousData } from '@tanstack/react-query';
import { ApiError, apiFetch } from '@/lib/api';
import { formatDateTime } from '@/lib/format';
import { Card, Button, Field, Alert, Badge, Skeleton, Empty } from '@/components/ui';
import { Modal } from '@/components/modal';

const PAGE = 20;

/* ═══════════════════════ Sunucu sözleşmesi ═══════════════════════ */
/*
 * Tipler BU DOSYADA tanımlıdır, `lib/types.ts` içinde değil: o dosya bu turda
 * paylaşılan bir dosyadır ve başka ajanlar da düzenliyor. Talepler kalıcı hâle
 * geldiğinde buradaki dört tip `lib/types.ts`'e taşınmalıdır (rapora yazıldı).
 * Karşılıkları: api/internal/transport/http/dto/ticket.go
 */

type TicketStatus = 'OPEN' | 'ANSWERED' | 'USER_REPLIED' | 'CLOSED';
type TicketPriority = 'LOW' | 'NORMAL' | 'HIGH';

interface TicketMessage {
  id: string;
  body: string;
  /** true ise mesajı destek ekibi yazdı (KK-600). */
  isStaff: boolean;
  authorLabel: string;
  createdAt: string;
}

interface Ticket {
  id: string;
  subject: string;
  priority: TicketPriority;
  priorityLabel: string;
  status: TicketStatus;
  statusLabel: string;
  messageCount: number;
  createdAt: string;
  lastReplyAt: string;
  closedAt?: string;
  messages?: TicketMessage[];
}

interface TicketList {
  items: Ticket[];
  total: number;
  limit: number;
  offset: number;
}

/* ═══════════════════════ Sunucu sınırları ═══════════════════════ */
/* domain/ticket/ticket.go — istemci doğrulaması KOLAYLIKTIR, savunma değil. */
const MIN_SUBJECT = 5;
const MAX_SUBJECT = 120;
const MAX_BODY = 4000;

/** Uzunluk KARAKTER (rune) ile sayılır: "ş" iki bayttır, bir karakterdir. */
const runeLength = (s: string) => Array.from(s).length;

const PRIORITIES: Array<{ value: TicketPriority; label: string }> = [
  { value: 'LOW', label: 'Düşük' },
  { value: 'NORMAL', label: 'Normal' },
  { value: 'HIGH', label: 'Yüksek' },
];

function statusTone(s: TicketStatus): 'ok' | 'warn' | 'brand' | 'neutral' {
  switch (s) {
    case 'ANSWERED': return 'ok';
    case 'OPEN': return 'warn';
    case 'USER_REPLIED': return 'brand';
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

/** Girdi fontu 16px'ten KÜÇÜK OLAMAZ: iOS Safari odakta sayfayı yakınlaştırır. */
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

export default function SupportPage() {
  const [offset, setOffset] = React.useState(0);
  const [creating, setCreating] = React.useState(false);
  const [openId, setOpenId] = React.useState<string | null>(null);

  const q = useQuery({
    queryKey: ['tickets', { limit: PAGE, offset }],
    queryFn: () => apiFetch<TicketList>(`/tickets?limit=${PAGE}&offset=${offset}`),
    // Sayfa değişince liste boşalıp zıplamasın.
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
          <h1 className="text-2xl font-bold tracking-tight md:text-3xl">Destek</h1>
          <p className="mt-1 text-sm text-muted">
            Sorununuzu yazın, destek ekibimiz yanıtlasın. Yanıtlar bu sayfada görünür.
          </p>
        </div>
        <Button className="shrink-0 sm:w-auto" fullWidth onClick={() => setCreating(true)}>
          Yeni talep
        </Button>
      </div>

      <Card>
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h2 className="text-lg font-semibold">Taleplerim</h2>
          {total > 0 && <Badge tone="neutral">{total} talep</Badge>}
        </div>

        {q.isLoading ? (
          <div className="mt-4 flex flex-col gap-2">
            {[0, 1, 2].map((i) => <Skeleton key={i} className="h-20" />)}
          </div>
        ) : listErr ? (
          <ErrorBox err={listErr} className="mt-4" />
        ) : !q.data?.items.length ? (
          <Empty
            title="Henüz talebiniz yok"
            hint="Bir sorunuz veya sorununuz olduğunda 'Yeni talep' ile bize yazabilirsiniz."
          />
        ) : (
          <>
            {/* MOBİL: kart listesi. Yatay kaydırılan tablo kabul edilmez (§2.5). */}
            <ul className="mt-4 flex flex-col gap-2 md:hidden">
              {q.data.items.map((t) => (
                <li key={t.id} className="raised rounded-xl border p-3">
                  <div className="flex items-start justify-between gap-3">
                    {/* break-anywhere: boşluksuz uzun bir konu 320 px'te
                        yatay kaydırma üretirdi. */}
                    <p className="min-w-0 text-sm font-medium break-anywhere">{t.subject}</p>
                    <Badge tone={statusTone(t.status)}>{t.statusLabel}</Badge>
                  </div>
                  <dl className="mt-3 flex flex-col gap-1.5 border-t border-[var(--border)] pt-2 text-xs">
                    <div className="flex items-center justify-between gap-3">
                      <dt className="text-muted">Son hareket</dt>
                      <dd>{formatDateTime(t.lastReplyAt)}</dd>
                    </div>
                    <div className="flex items-center justify-between gap-3">
                      <dt className="text-muted">Öncelik</dt>
                      <dd>{t.priorityLabel}</dd>
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

            {/* MASAÜSTÜ: gerçek tablo. */}
            <div className="mt-4 hidden md:block">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-[var(--border)] text-left text-xs
                                 uppercase tracking-wide text-muted">
                    <th scope="col" className="py-2 pr-3 font-medium">Konu</th>
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
                      <td className="max-w-[16rem] py-3 pr-3">
                        <span className="block truncate font-medium">{t.subject}</span>
                        <span className="block text-xs text-muted">{t.messageCount} mesaj</span>
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

      {creating && <CreateDialog onClose={() => setCreating(false)} onCreated={setOpenId} />}
      {openId && <ThreadDialog id={openId} onClose={() => setOpenId(null)} />}
    </div>
  );
}

/* ═══════════════════════ Yeni talep ═══════════════════════ */

function CreateDialog({
  onClose, onCreated,
}: { onClose: () => void; onCreated: (id: string) => void }) {
  const qc = useQueryClient();
  const [subject, setSubject] = React.useState('');
  const [priority, setPriority] = React.useState<TicketPriority>('NORMAL');
  const [message, setMessage] = React.useState('');
  const [errs, setErrs] = React.useState<Record<string, string>>({});

  const create = useMutation({
    mutationFn: (v: { subject: string; priority: TicketPriority; message: string }) =>
      apiFetch<Ticket>('/tickets', { method: 'POST', body: v }),
    // MUTASYON OTOMATİK TEKRARLANMAZ (değişmez #16): ağ yanıtı yutulduğunda
    // ikinci deneme İKİNCİ BİR TALEP açar ve yönetici aynı soruyu iki kez görür.
    retry: false,
    onSuccess: (t) => {
      qc.invalidateQueries({ queryKey: ['tickets'] });
      onClose();
      onCreated(t.id);
    },
  });

  function submit(e: React.FormEvent) {
    e.preventDefault();
    const next: Record<string, string> = {};
    const s = subject.trim();
    const m = message.trim();
    if (runeLength(s) < MIN_SUBJECT) next.subject = 'Konu en az 5 karakter olmalıdır.';
    else if (runeLength(s) > MAX_SUBJECT) next.subject = 'Konu en fazla 120 karakter olabilir.';
    if (m.length === 0) next.message = 'Mesaj boş olamaz.';
    else if (runeLength(m) > MAX_BODY) next.message = 'Mesaj en fazla 4000 karakter olabilir.';
    setErrs(next);
    if (Object.keys(next).length) return;
    create.mutate({ subject: s, priority, message: m });
  }

  const err = create.error instanceof ApiError ? create.error : null;

  return (
    <Modal open onClose={onClose} title="Yeni destek talebi">
      <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
        <Field
          label="Konu"
          data-autofocus
          value={subject}
          onChange={(e) => setSubject(e.target.value)}
          error={errs.subject}
          hint={`${runeLength(subject)}/${MAX_SUBJECT} karakter`}
          maxLength={MAX_SUBJECT * 2}
          disabled={create.isPending}
        />

        <label className="flex flex-col gap-1.5">
          <span className="text-sm font-medium">Öncelik</span>
          <div className="relative">
            <select
              className={selectClass}
              value={priority}
              disabled={create.isPending}
              onChange={(e) => setPriority(e.target.value as TicketPriority)}
            >
              {PRIORITIES.map((p) => <option key={p.value} value={p.value}>{p.label}</option>)}
            </select>
            <Chevron />
          </div>
        </label>

        <label className="flex flex-col gap-1.5">
          <span className="text-sm font-medium">Mesajınız</span>
          <textarea
            className={textareaClass}
            rows={6}
            value={message}
            disabled={create.isPending}
            onChange={(e) => setMessage(e.target.value)}
            aria-invalid={errs.message ? true : undefined}
            placeholder="Sorununuzu olabildiğince ayrıntılı yazın. Sipariş numarası varsa ekleyin."
          />
          <span className="text-xs text-muted">{runeLength(message)}/{MAX_BODY} karakter</span>
          {errs.message && (
            <span role="alert" className="text-xs text-[var(--color-bad)]">{errs.message}</span>
          )}
        </label>

        {err && <ErrorBox err={err} />}

        <div className="flex flex-col gap-2 sm:flex-row-reverse">
          <Button type="submit" loading={create.isPending} fullWidth className="sm:w-auto">
            Talebi gönder
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

/* ═══════════════════════ Yazışma ═══════════════════════ */

function ThreadDialog({ id, onClose }: { id: string; onClose: () => void }) {
  const qc = useQueryClient();
  const [message, setMessage] = React.useState('');
  const [formErr, setFormErr] = React.useState('');

  const q = useQuery({
    queryKey: ['ticket', id],
    queryFn: () => apiFetch<Ticket>(`/tickets/${encodeURIComponent(id)}`),
  });

  const reply = useMutation({
    mutationFn: (body: string) =>
      apiFetch<Ticket>(`/tickets/${encodeURIComponent(id)}/messages`, {
        method: 'POST', body: { message: body },
      }),
    // Tekrarlanan bir yanıt, yazışmaya aynı mesajı iki kez düşürürdü.
    retry: false,
    onSuccess: () => {
      setMessage('');
      qc.invalidateQueries({ queryKey: ['ticket', id] });
      qc.invalidateQueries({ queryKey: ['tickets'] });
    },
  });

  const t = q.data;
  const loadErr = q.error instanceof ApiError ? q.error : null;
  const replyErr = reply.error instanceof ApiError ? reply.error : null;
  const closed = t?.status === 'CLOSED';

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
            <span className="text-xs text-muted">Açılış: {formatDateTime(t.createdAt)}</span>
          </div>

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
                    <span className="text-sm font-semibold">{m.authorLabel}</span>
                    <span className="text-xs text-muted">{formatDateTime(m.createdAt)}</span>
                  </div>
                  {/*
                    DÜZ METİN. `dangerouslySetInnerHTML` YOKTUR: gövde kullanıcı
                    girdisidir ve HTML olarak basılsaydı bir kullanıcı, talebi
                    okuyan yöneticinin oturumunda betik çalıştırabilirdi.
                    Satır sonları CSS ile korunur, işaretleme ile değil.
                    break-anywhere: boşluksuz uzun bir dize (URL, hash) 320 px'te
                    yatay kaydırma üretirdi.
                  */}
                  <p className="mt-2 text-sm leading-relaxed whitespace-pre-wrap break-anywhere">
                    {m.body}
                  </p>
                </li>
              ))}
            </ul>
          )}

          {closed ? (
            <Alert tone="info">
              Bu talep kapatıldı. Yeni bir sorunuz varsa lütfen yeni bir talep açın —
              önceki yazışmanız burada kalır.
            </Alert>
          ) : (
            <form onSubmit={submit} className="flex flex-col gap-2 border-t
                                               border-[var(--border)] pt-4" noValidate>
              <label className="flex flex-col gap-1.5">
                <span className="text-sm font-medium">Yanıtınız</span>
                <textarea
                  className={textareaClass}
                  rows={4}
                  value={message}
                  disabled={reply.isPending}
                  onChange={(e) => setMessage(e.target.value)}
                  aria-invalid={formErr ? true : undefined}
                />
                <span className="text-xs text-muted">{runeLength(message)}/{MAX_BODY} karakter</span>
                {formErr && (
                  <span role="alert" className="text-xs text-[var(--color-bad)]">{formErr}</span>
                )}
              </label>
              {replyErr && <ErrorBox err={replyErr} />}
              <Button type="submit" loading={reply.isPending} fullWidth className="sm:w-auto sm:self-end">
                Gönder
              </Button>
            </form>
          )}
        </div>
      )}
    </Modal>
  );
}
