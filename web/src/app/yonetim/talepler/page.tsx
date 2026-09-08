'use client';

/**
 * Bakiye talepleri (FR-704).
 *
 * Bu ekran GERÇEK PARA yazar: "Onayla" düğmesi kullanıcının bakiyesini
 * artırır ve kayıt defterine değiştirilemez bir satır yazar. Bu yüzden hem
 * onay hem red iki adımlıdır ve ikinci adım yazılacak tutarı/gerekçeyi
 * AÇIKÇA tekrar gösterir (CLAUDE.md değişmez #4, ekran kuralı #9).
 */

import * as React from 'react';
import { useMutation, useQuery, useQueryClient, keepPreviousData } from '@tanstack/react-query';
import { ApiError, apiBlob, apiFetch } from '@/lib/api';
import { formatMoney, formatDateTime } from '@/lib/format';
import { Card, Button, Field, Alert, Badge, Skeleton, Empty, Spinner } from '@/components/ui';
import { Modal } from '@/components/modal';
import type { AdminDeposit, DepositStatus, Money } from '@/lib/types';

const PAGE = 20;

/** Yanıt zarfları — sunucu DTO'ları (dto/deposit.go). */
interface AdminDepositList {
  items: AdminDeposit[];
  total: number;
  limit: number;
  offset: number;
}
interface DepositReview {
  deposit: AdminDeposit;
  /** TALEP SAHİBİNİN yeni bakiyesi, yöneticinin değil. */
  balance: Money;
  /** true ise bu istek bir TEKRAR'dı; hiçbir şey yazılmadı. */
  alreadyApplied?: boolean;
}

/** Sunucudaki sınırlar (dto/deposit.go: depositMin/MaxAmountMinor). */
const MIN_MINOR = 1000;
const MAX_MINOR = 5_000_000;

const STATUS_FILTERS: Array<{ value: '' | DepositStatus; label: string }> = [
  // Varsayılan ilk sıradadır: yöneticinin işi BEKLEYEN taleplerdir.
  { value: 'PENDING', label: 'Onay bekleyenler' },
  { value: 'COMPLETED', label: 'Onaylananlar' },
  { value: 'REJECTED', label: 'Reddedilenler' },
  { value: 'REFUNDED', label: 'İade edilenler' },
  { value: '', label: 'Tümü' },
];

function statusTone(s: DepositStatus): 'ok' | 'warn' | 'bad' | 'neutral' {
  switch (s) {
    case 'COMPLETED': return 'ok';
    case 'PENDING':   return 'warn';
    case 'REJECTED':  return 'bad';
    default:          return 'neutral';
  }
}

/**
 * TL girişi → kuruş.
 *
 * KAYAN NOKTA KULLANILMAZ: `12.50 * 100` JavaScript'te 1249.9999… verebilir
 * ve bir kuruş kaybolur (yonetim/bakiye/page.tsx ile aynı kalıp).
 */
function toMinor(input: string): { minor: number } | { error: string } {
  const s = input.trim().replace(/\s/g, '').replace(',', '.');
  if (s === '') return { error: 'Tutar giriniz.' };
  if (!/^\d+(\.\d{1,2})?$/.test(s)) {
    return { error: 'Geçerli bir tutar giriniz (en fazla 2 ondalık).' };
  }
  const [whole = '0', frac = ''] = s.split('.');
  const minor = Number(whole) * 100 + Number(frac.padEnd(2, '0'));
  if (!Number.isSafeInteger(minor)) return { error: 'Tutar çok büyük.' };
  return { minor };
}

function tryMoney(minor: number): Money {
  return { minor, currency: 'TRY', formatted: '' };
}

/**
 * Modal içinde odağı `[data-autofocus]` öğesine taşır.
 *
 * İki ayrı sorunu birden kapatır — ikisi de ölçümle bulundu (odak `BODY`'de
 * kalıyordu):
 *  1. modal.tsx'teki `el?.focus() ?? panel.focus()` zinciri HER ZAMAN ikinci
 *     dala da girer (`focus()` `undefined` döner), yani odağı panele geri alır
 *     ve `data-autofocus` işlevsiz kalır.
 *  2. Diyalog adım değiştirdiğinde tıklanan düğme DOM'dan kalkar; odak
 *     `<body>`'ye düşer ve klavye kullanıcısı modalın arkasındaki sayfaya
 *     Tab'lamaya başlar.
 */
function useDialogFocus(ref: React.RefObject<HTMLElement | null>, dep: unknown) {
  React.useEffect(() => {
    // setTimeout(0): modal.tsx'in kendi odak etkisi bu render'dan SONRA
    // çalışır; ondan önce odaklarsak panel odağı geri alır.
    const t = window.setTimeout(() => {
      ref.current?.querySelector<HTMLElement>('[data-autofocus]')?.focus();
    }, 0);
    return () => window.clearTimeout(t);
  }, [ref, dep]);
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
 * kendi hesaplar ve `min-h-12`'yi YOK SAYAR — ölçümde kutu 25 px çıkıyordu,
 * yani 44 px dokunma hedefinin çok altında (§2.3). Görünüm kapatılınca oku
 * kendimiz çizeriz; `Chevron` bunun içindir.
 */
const selectClass =
  'raised min-h-12 w-full appearance-none rounded-xl border px-3 pr-10 text-base ' +
  'outline-none focus:border-brand-400 disabled:opacity-60';

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

const textareaClass =
  'raised w-full rounded-xl border px-3.5 py-2.5 text-base outline-none ' +
  'focus:border-brand-400 disabled:opacity-60';

export default function AdminDepositsPage() {
  const [status, setStatus] = React.useState<'' | DepositStatus>('PENDING');
  const [offset, setOffset] = React.useState(0);
  const [selected, setSelected] = React.useState<AdminDeposit | null>(null);

  const q = useQuery({
    queryKey: ['admin-deposits', { status, limit: PAGE, offset }],
    queryFn: () => {
      const qs = new URLSearchParams({ limit: String(PAGE), offset: String(offset) });
      if (status) qs.set('status', status);
      return apiFetch<AdminDepositList>(`/admin/deposits?${qs.toString()}`);
    },
    // Sayfa/süzgeç değişince liste boşalıp zıplamasın.
    placeholderData: keepPreviousData,
  });

  const total = q.data?.total ?? 0;
  const hasPrev = offset > 0;
  const hasNext = offset + PAGE < total;
  const listErr = q.error instanceof ApiError ? q.error : null;

  return (
    <div className="mx-auto flex max-w-5xl flex-col gap-5">
      <div>
        <h1 className="text-2xl font-bold tracking-tight md:text-3xl">Bakiye talepleri</h1>
        <p className="mt-1 text-sm text-muted">
          Kullanıcıların bildirdiği ödemeleri inceleyip onaylayın veya reddedin.
        </p>
      </div>

      <Card>
        <div className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
          <label className="flex w-full flex-col gap-1.5 sm:max-w-xs">
            <span className="text-sm font-medium">Durum</span>
            <div className="relative">
              <select
                className={selectClass}
                value={status}
                onChange={(e) => {
                  setStatus(e.target.value as '' | DepositStatus);
                  setOffset(0); // süzgeç değişti; eski sayfa numarası anlamsız
                }}
              >
                {STATUS_FILTERS.map((f) => (
                  <option key={f.value || 'all'} value={f.value}>{f.label}</option>
                ))}
              </select>
              <Chevron />
            </div>
          </label>
          {total > 0 && (
            <div className="shrink-0"><Badge tone="neutral">{total} kayıt</Badge></div>
          )}
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
            hint={status === 'PENDING'
              ? 'Onay bekleyen bakiye talebi bulunmuyor.'
              : 'Bu süzgece uyan bir talep bulunmuyor.'}
          />
        ) : (
          <>
            {/* MOBİL: kart listesi. Yatay kaydırılan tablo kabul edilmez (§2.5). */}
            <ul className="mt-4 flex flex-col gap-2 md:hidden">
              {q.data.items.map((d) => (
                <li key={d.id} className="raised rounded-xl border p-3">
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                      <p className="truncate text-sm font-medium">{d.userUsername}</p>
                      <p className="truncate text-xs text-muted break-anywhere">{d.userEmail}</p>
                    </div>
                    <Badge tone={statusTone(d.status)}>{d.statusLabel}</Badge>
                  </div>

                  <dl className="mt-3 flex flex-col gap-1.5 border-t border-[var(--border)] pt-2 text-xs">
                    <div className="flex items-center justify-between gap-3">
                      <dt className="text-muted">Bildirilen tutar</dt>
                      <dd className="font-semibold">{formatMoney(d.amount)}</dd>
                    </div>
                    {d.status === 'COMPLETED' && (
                      <div className="flex items-center justify-between gap-3">
                        <dt className="text-muted">Yazılan tutar</dt>
                        <dd className="font-semibold text-[var(--color-ok)]">{formatMoney(d.credited)}</dd>
                      </div>
                    )}
                    <div className="flex items-center justify-between gap-3">
                      <dt className="text-muted">Yöntem</dt>
                      <dd className="min-w-0 truncate">{d.method}</dd>
                    </div>
                    <div className="flex items-center justify-between gap-3">
                      <dt className="text-muted">Tarih</dt>
                      <dd>{formatDateTime(d.createdAt)}</dd>
                    </div>
                    <div className="flex items-center justify-between gap-3">
                      <dt className="text-muted">Dekont</dt>
                      <dd>{d.hasReceipt ? 'Var' : 'Yok'}</dd>
                    </div>
                  </dl>

                  <Button
                    variant="outline" size="sm" fullWidth className="mt-3"
                    onClick={() => setSelected(d)}
                  >
                    {d.status === 'PENDING' ? 'İncele ve karar ver' : 'Ayrıntılar'}
                  </Button>
                </li>
              ))}
            </ul>

            {/*
              MASAÜSTÜ: gerçek tablo.

              "Yöntem" ve "Dekont" sütunları `lg:` altında GİZLENİR ve bilgi
              kaybolmaz — ikisi de karttaki ve diyalogdaki yerinde durur.
              Yedi sütun 768 px'te sığmıyordu; sığdırmaya çalışmak tabloyu
              yatay kaydırmaya iterdi ki bu kabul edilmez (§2.5).
            */}
            <div className="mt-4 hidden md:block">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-[var(--border)] text-left text-xs
                                 uppercase tracking-wide text-muted">
                    <th scope="col" className="py-2 pr-3 font-medium">Tarih</th>
                    <th scope="col" className="py-2 pr-3 font-medium">Kullanıcı</th>
                    <th scope="col" className="hidden py-2 pr-3 font-medium lg:table-cell">Yöntem</th>
                    <th scope="col" className="py-2 pr-3 text-right font-medium">Tutar</th>
                    <th scope="col" className="hidden py-2 pr-3 font-medium lg:table-cell">Dekont</th>
                    <th scope="col" className="py-2 pr-3 font-medium">Durum</th>
                    <th scope="col" className="py-2 text-right font-medium">
                      <span className="sr-only">İşlem</span>
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {q.data.items.map((d) => (
                    <tr key={d.id} className="border-b border-[var(--border)] align-top last:border-0">
                      <td className="py-3 pr-3 whitespace-nowrap text-muted">
                        {formatDateTime(d.createdAt)}
                      </td>
                      <td className="max-w-[9rem] py-3 pr-3 lg:max-w-[14rem]">
                        <span className="block truncate font-medium">{d.userUsername}</span>
                        <span className="block truncate text-xs text-muted">{d.userEmail}</span>
                      </td>
                      <td className="hidden max-w-[10rem] py-3 pr-3 lg:table-cell">
                        <span className="block truncate">{d.method}</span>
                      </td>
                      <td className="py-3 pr-3 text-right whitespace-nowrap font-semibold">
                        {formatMoney(d.amount)}
                        {d.status === 'COMPLETED' && d.credited.minor !== d.amount.minor && (
                          <span className="block text-xs font-normal text-[var(--color-ok)]">
                            yazılan: {formatMoney(d.credited)}
                          </span>
                        )}
                      </td>
                      <td className="hidden py-3 pr-3 text-muted lg:table-cell">
                        {d.hasReceipt ? 'Var' : '—'}
                      </td>
                      <td className="py-3 pr-3">
                        <Badge tone={statusTone(d.status)}>{d.statusLabel}</Badge>
                      </td>
                      <td className="py-3 text-right">
                        <Button variant="outline" size="sm" onClick={() => setSelected(d)}>
                          {d.status === 'PENDING' ? 'İncele' : 'Ayrıntılar'}
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

      {selected && (
        <ReviewDialog
          key={selected.id}
          deposit={selected}
          onClose={() => setSelected(null)}
        />
      )}
    </div>
  );
}

/* ═══════════════════════ İnceleme diyaloğu ═══════════════════════ */

type Step = 'detail' | 'approve' | 'approveConfirm' | 'reject' | 'rejectConfirm' | 'done';

function ReviewDialog({ deposit, onClose }: { deposit: AdminDeposit; onClose: () => void }) {
  const qc = useQueryClient();
  const [step, setStep] = React.useState<Step>('detail');
  const [credited, setCredited] = React.useState('');
  const [adminNote, setAdminNote] = React.useState('');
  const [reason, setReason] = React.useState('');
  const [formErrors, setFormErrors] = React.useState<Record<string, string>>({});

  const stepRef = React.useRef<HTMLDivElement>(null);
  useDialogFocus(stepRef, step);

  const pending = deposit.status === 'PENDING';

  const approve = useMutation({
    mutationFn: (v: { creditedMinor: number; adminNote: string }) =>
      apiFetch<DepositReview>(`/admin/deposits/${encodeURIComponent(deposit.id)}/approve`, {
        method: 'POST',
        body: { creditedMinor: v.creditedMinor, adminNote: v.adminNote },
      }),
    // Belge amaçlı: para yazan bir çağrı ASLA otomatik tekrarlanmaz.
    // (İdempotency anahtarı sunucuda talebin public_id'sinden türer.)
    retry: false,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['admin-deposits'] });
      setStep('done');
    },
  });

  const reject = useMutation({
    mutationFn: (v: { reason: string; adminNote: string }) =>
      apiFetch<DepositReview>(`/admin/deposits/${encodeURIComponent(deposit.id)}/reject`, {
        method: 'POST',
        body: { reason: v.reason, adminNote: v.adminNote },
      }),
    retry: false,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['admin-deposits'] });
      setStep('done');
    },
  });

  /**
   * Yazılacak tutar.
   *
   * Alan BOŞSA sunucu `creditedMinor: 0` görür ve kullanıcının BİLDİRDİĞİ
   * tutarı yazar. Yönetici farklı bir tutar yazabilir çünkü kriptoda ağ
   * ücreti düşer: kullanıcı 500 ₺ gönderir, hesaba 493,40 ₺ düşer.
   */
  const parsed = credited.trim() === '' ? null : toMinor(credited);
  const creditedMinor = parsed && 'minor' in parsed ? parsed.minor : 0;
  const effective: Money = creditedMinor === 0 ? deposit.amount : tryMoney(creditedMinor);
  const differs = creditedMinor !== 0 && creditedMinor !== deposit.amount.minor;

  function submitApproveForm(e: React.FormEvent) {
    e.preventDefault();
    const errs: Record<string, string> = {};
    if (parsed && 'error' in parsed) {
      errs.credited = parsed.error;
    } else if (creditedMinor !== 0 && (creditedMinor < MIN_MINOR || creditedMinor > MAX_MINOR)) {
      errs.credited = 'Yatan tutar 10,00 ₺ ile 50.000,00 ₺ arasında olmalıdır.';
    }
    if (adminNote.trim().length > 300) errs.adminNote = 'Not en fazla 300 karakter olabilir.';
    setFormErrors(errs);
    if (Object.keys(errs).length) return;
    setStep('approveConfirm');
  }

  function submitRejectForm(e: React.FormEvent) {
    e.preventDefault();
    const errs: Record<string, string> = {};
    const r = reason.trim();
    if (r.length < 5) {
      errs.reason = 'Red nedeni zorunludur (en az 5 karakter) — kullanıcıya gösterilir.';
    } else if (r.length > 300) {
      errs.reason = 'Red nedeni en fazla 300 karakter olabilir.';
    }
    if (adminNote.trim().length > 300) errs.adminNote = 'Not en fazla 300 karakter olabilir.';
    setFormErrors(errs);
    if (Object.keys(errs).length) return;
    setStep('rejectConfirm');
  }

  const mutErr =
    (approve.error instanceof ApiError && approve.error) ||
    (reject.error instanceof ApiError && reject.error) ||
    null;

  const result = approve.data ?? reject.data ?? null;

  const titles: Record<Step, string> = {
    detail: 'Talep ayrıntısı',
    approve: 'Talebi onayla',
    approveConfirm: 'Onayı doğrulayın',
    reject: 'Talebi reddet',
    rejectConfirm: 'Reddi doğrulayın',
    done: 'Sonuç',
  };

  return (
    <Modal open onClose={onClose} title={titles[step]}>
      <div ref={stepRef}>
      {step === 'detail' && (
        <div className="flex flex-col gap-4">
          <DepositFacts deposit={deposit} />
          <ReceiptViewer depositId={deposit.id} hasReceipt={deposit.hasReceipt} />

          {pending ? (
            <div className="flex flex-col gap-2">
              <Button
                data-autofocus
                fullWidth
                onClick={() => { setFormErrors({}); setStep('approve'); }}
              >
                Onayla
              </Button>
              <Button
                variant="danger" fullWidth
                onClick={() => { setFormErrors({}); setStep('reject'); }}
              >
                Reddet
              </Button>
              <Button variant="ghost" fullWidth onClick={onClose}>Kapat</Button>
            </div>
          ) : (
            <Button data-autofocus variant="outline" fullWidth onClick={onClose}>Kapat</Button>
          )}
        </div>
      )}

      {step === 'approve' && (
        <form onSubmit={submitApproveForm} className="flex flex-col gap-4" noValidate>
          <Alert tone="warn">
            Onay, kullanıcının bakiyesini <strong>gerçekten</strong> artırır ve kayıt
            defterine geri alınamaz bir satır yazar. Ödemenin hesabınıza geçtiğini
            doğrulamadan onaylamayın.
          </Alert>

          <div className="raised rounded-xl border p-3 text-sm">
            <div className="flex items-center justify-between gap-3">
              <span className="text-muted">Kullanıcının bildirdiği</span>
              <span className="font-semibold">{formatMoney(deposit.amount)}</span>
            </div>
            <div className="mt-1.5 flex items-center justify-between gap-3">
              <span className="text-muted">Bakiyeye yazılacak</span>
              <span className="font-semibold text-[var(--color-ok)]">{formatMoney(effective)}</span>
            </div>
          </div>

          <Field
            data-autofocus
            label="Yazılacak tutar (TL)"
            value={credited}
            onChange={(e) => setCredited(e.target.value)}
            placeholder={`Boş bırakılırsa ${formatMoney(deposit.amount)}`}
            inputMode="decimal" autoComplete="off"
            error={formErrors.credited}
            hint="Boş bırakırsanız kullanıcının bildirdiği tutar yazılır. Kripto ağ
                  ücreti veya eksik havale nedeniyle hesaba GEÇEN tutar farklıysa,
                  gerçekten geçen tutarı buraya yazın."
          />

          {differs && (
            <Alert tone="warn">
              Bildirilen tutar ile yazılacak tutar <strong>farklı</strong>. Kullanıcı
              ekstresinde yazılan tutarı görecek; farkın nedenini aşağıdaki nota
              yazmanız ileride yapılacak incelemeyi kolaylaştırır.
            </Alert>
          )}

          <label className="flex flex-col gap-1.5">
            <span className="text-sm font-medium">Yönetici notu (isteğe bağlı)</span>
            <textarea
              className={textareaClass} rows={3} maxLength={300}
              value={adminNote} onChange={(e) => setAdminNote(e.target.value)}
              placeholder="Örn. 12.09 tarihli havale, dekont doğrulandı."
            />
            <span className="text-xs text-muted">
              Yalnız yöneticiler görür; kullanıcıya gösterilmez.
            </span>
            {formErrors.adminNote && (
              <span role="alert" className="text-xs text-[var(--color-bad)]">{formErrors.adminNote}</span>
            )}
          </label>

          <div className="flex flex-col gap-2">
            <Button type="submit" fullWidth>Devam et</Button>
            <Button type="button" variant="ghost" fullWidth onClick={() => setStep('detail')}>
              Geri
            </Button>
          </div>
        </form>
      )}

      {step === 'approveConfirm' && (
        <div className="flex flex-col gap-4">
          <Alert tone="warn">
            <p>
              <strong>{deposit.userUsername}</strong> adlı kullanıcının bakiyesine
              {' '}<strong>{formatMoney(effective)}</strong> yazılacak.
            </p>
            <p className="mt-2">Bu işlem geri alınamaz.</p>
          </Alert>

          {differs && (
            <p className="text-sm text-muted">
              Kullanıcı {formatMoney(deposit.amount)} bildirmişti; siz
              {' '}{formatMoney(effective)} onaylıyorsunuz.
            </p>
          )}

          {mutErr && <ErrorBox err={mutErr} />}

          <div className="flex flex-col gap-2">
            {/*
              ODAK "Vazgeç"TEDİR, "Evet"te DEĞİL.

              Buraya klavyeyle gelinir: kullanıcı bir önceki adımda Enter ile
              "Devam et"e basar. Enter tuşu basılı tutulursa `click` keydown'da
              tekrar üretilir; odak "Evet"te olsaydı basılı kalan tek bir tuş
              parayı yazardı. Onay ayrı ve bilinçli bir hareket olmalıdır.
            */}
            <Button
              fullWidth loading={approve.isPending}
              onClick={() => approve.mutate({ creditedMinor, adminNote: adminNote.trim() })}
            >
              Evet, bakiyeye yaz
            </Button>
            <Button
              data-autofocus variant="ghost" fullWidth disabled={approve.isPending}
              onClick={() => setStep('approve')}
            >
              Vazgeç
            </Button>
          </div>
        </div>
      )}

      {step === 'reject' && (
        <form onSubmit={submitRejectForm} className="flex flex-col gap-4" noValidate>
          <Alert tone="info">
            Red bakiyeyi değiştirmez. Yazdığınız neden <strong>kullanıcıya aynen
            gösterilir</strong>; anlaşılır ve nazik bir cümle yazın.
          </Alert>

          <label className="flex flex-col gap-1.5">
            <span className="text-sm font-medium">Red nedeni (kullanıcı görür)</span>
            <textarea
              data-autofocus
              className={textareaClass} rows={3} maxLength={300}
              value={reason} onChange={(e) => setReason(e.target.value)}
              placeholder="Örn. Bildirilen tutarda bir ödeme hesabımıza ulaşmadı."
              aria-invalid={formErrors.reason ? true : undefined}
            />
            {formErrors.reason ? (
              <span role="alert" className="text-xs text-[var(--color-bad)]">{formErrors.reason}</span>
            ) : (
              <span className="text-xs text-muted">En az 5, en fazla 300 karakter.</span>
            )}
          </label>

          <label className="flex flex-col gap-1.5">
            <span className="text-sm font-medium">Yönetici notu (isteğe bağlı)</span>
            <textarea
              className={textareaClass} rows={2} maxLength={300}
              value={adminNote} onChange={(e) => setAdminNote(e.target.value)}
              placeholder="Yalnız yöneticilerin göreceği iç not."
            />
            {formErrors.adminNote && (
              <span role="alert" className="text-xs text-[var(--color-bad)]">{formErrors.adminNote}</span>
            )}
          </label>

          <div className="flex flex-col gap-2">
            <Button type="submit" variant="danger" fullWidth>Devam et</Button>
            <Button type="button" variant="ghost" fullWidth onClick={() => setStep('detail')}>
              Geri
            </Button>
          </div>
        </form>
      )}

      {step === 'rejectConfirm' && (
        <div className="flex flex-col gap-4">
          <Alert tone="warn">
            <p>
              <strong>{deposit.userUsername}</strong> adlı kullanıcının
              {' '}{formatMoney(deposit.amount)} tutarındaki talebi reddedilecek.
            </p>
          </Alert>

          <div className="raised rounded-xl border p-3">
            <p className="text-xs font-medium uppercase tracking-wide text-muted">
              Kullanıcının göreceği metin
            </p>
            <p className="mt-1.5 text-sm break-anywhere">{reason.trim()}</p>
          </div>

          {mutErr && <ErrorBox err={mutErr} />}

          <div className="flex flex-col gap-2">
            {/* Odak "Vazgeç"te — gerekçe için onay adımına bakınız. */}
            <Button
              variant="danger" fullWidth loading={reject.isPending}
              onClick={() => reject.mutate({ reason: reason.trim(), adminNote: adminNote.trim() })}
            >
              Evet, reddet
            </Button>
            <Button
              data-autofocus variant="ghost" fullWidth disabled={reject.isPending}
              onClick={() => setStep('reject')}
            >
              Vazgeç
            </Button>
          </div>
        </div>
      )}

      {step === 'done' && result && (
        <div className="flex flex-col gap-4">
          {/*
            TEKRARLANAN ONAY HATA DEĞİLDİR: sunucu 200 + alreadyApplied döner.
            Kırmızı bir hata göstermek, yöneticiye "olmadı, tekrar dene"
            dedirtir — oysa iş çoktan yapılmıştır.
          */}
          <Alert tone={result.alreadyApplied ? 'info' : 'ok'}>
            {result.alreadyApplied
              ? 'Bu talep daha önce sonuçlandırılmıştı; hiçbir şey yeniden yazılmadı.'
              : `Talep ${result.deposit.statusLabel.toLocaleLowerCase('tr-TR')}.`}
          </Alert>

          <div className="raised rounded-xl border p-3 text-sm">
            <div className="flex items-center justify-between gap-3">
              <span className="text-muted">Durum</span>
              <Badge tone={statusTone(result.deposit.status)}>{result.deposit.statusLabel}</Badge>
            </div>
            <div className="mt-2 flex items-center justify-between gap-3">
              <span className="text-muted">Bakiyeye yazılan</span>
              <span className="font-semibold">{formatMoney(result.deposit.credited)}</span>
            </div>
            <div className="mt-2 flex items-center justify-between gap-3">
              <span className="text-muted">Kullanıcının yeni bakiyesi</span>
              <span className="font-semibold">{formatMoney(result.balance)}</span>
            </div>
          </div>

          <Button data-autofocus fullWidth onClick={onClose}>Kapat</Button>
        </div>
      )}
      </div>
    </Modal>
  );
}

/* ═══════════════════════ Talep bilgileri ═══════════════════════ */

function Row({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex items-start justify-between gap-3 py-1.5">
      <dt className="shrink-0 text-muted">{label}</dt>
      <dd className="min-w-0 text-right font-medium break-anywhere">{value}</dd>
    </div>
  );
}

function DepositFacts({ deposit: d }: { deposit: AdminDeposit }) {
  return (
    <dl className="raised divide-y divide-[var(--border)] rounded-xl border px-3 py-1 text-sm">
      <Row label="Kullanıcı" value={<>{d.userUsername}<span className="block text-xs font-normal text-muted">{d.userEmail}</span></>} />
      <Row label="Durum" value={<Badge tone={statusTone(d.status)}>{d.statusLabel}</Badge>} />
      <Row label="Yöntem" value={d.method} />
      <Row label="Bildirilen tutar" value={formatMoney(d.amount)} />
      {d.status !== 'PENDING' && <Row label="Yazılan tutar" value={formatMoney(d.credited)} />}
      {d.network && <Row label="Ağ" value={d.network} />}
      {d.txHash && <Row label="İşlem numarası" value={<code className="font-mono text-xs">{d.txHash}</code>} />}
      {d.userNote && <Row label="Kullanıcı notu" value={d.userNote} />}
      {d.adminNote && <Row label="Yönetici notu" value={d.adminNote} />}
      {d.rejectionReason && <Row label="Red nedeni" value={d.rejectionReason} />}
      <Row label="Oluşturuldu" value={formatDateTime(d.createdAt)} />
      {d.reviewedAt && <Row label="İncelendi" value={formatDateTime(d.reviewedAt)} />}
    </dl>
  );
}

/* ═══════════════════════ Dekont ═══════════════════════ */

/**
 * Dekont görüntüleyici.
 *
 * Uç `Content-Disposition: attachment` ile döner: doğrudan bir <img src>
 * veya <a href> tarayıcıyı indirmeye zorlar. Dosyayı blob olarak alıp object
 * URL üretiriz — ve BIRAKMAYI unutmayız: her açılışta yeni bir blob bellekte
 * kalırsa, on talebi inceleyen bir yönetici on dosyayı taşır.
 */
function ReceiptViewer({ depositId, hasReceipt }: { depositId: string; hasReceipt: boolean }) {
  const [file, setFile] = React.useState<{ url: string; mime: string } | null>(null);
  const [loading, setLoading] = React.useState(false);
  const [err, setErr] = React.useState<ApiError | null>(null);

  // Blob'u serbest bırak: bileşen ayrıldığında ve dosya değiştiğinde.
  React.useEffect(() => {
    if (!file) return;
    return () => URL.revokeObjectURL(file.url);
  }, [file]);

  async function load() {
    setLoading(true);
    setErr(null);
    try {
      const r = await apiBlob(`/admin/deposits/${encodeURIComponent(depositId)}/receipt`, {
        timeoutMs: 30_000, // dekont birkaç MB olabilir; 15 sn mobil ağda dar
      });
      setFile({ url: r.url, mime: r.mime });
    } catch (e) {
      setErr(e instanceof ApiError ? e : new ApiError({ code: 'UNKNOWN', message: 'Dekont açılamadı.' }));
    } finally {
      setLoading(false);
    }
  }

  if (!hasReceipt) {
    return (
      <Alert tone="info">
        Bu talepte dekont yok. Havale taleplerinde dekont beklenir; onaylamadan
        önce ödemeyi hesap hareketlerinizden doğrulayın.
      </Alert>
    );
  }

  const isImage = file?.mime.startsWith('image/');

  return (
    <div className="flex flex-col gap-2">
      {!file && (
        <Button variant="outline" fullWidth onClick={load} disabled={loading}>
          {loading ? <><Spinner /> Dekont açılıyor…</> : 'Dekontu göster'}
        </Button>
      )}

      {err && <ErrorBox err={err} />}

      {file && (
        <div className="flex flex-col gap-2">
          {isImage ? (
            /* next/image KULLANILMAZ: kaynak bir `blob:` URL'idir, optimize
               edici uzak/yerel bir yol bekler ve bunu işleyemez. */
            // eslint-disable-next-line @next/next/no-img-element
            <img
              src={file.url} alt="Dekont"
              className="max-h-[50dvh] w-full rounded-xl border border-[var(--border)] object-contain"
            />
          ) : (
            <object
              data={file.url} type={file.mime || 'application/pdf'}
              className="h-[50dvh] w-full rounded-xl border border-[var(--border)]"
              aria-label="Dekont"
            >
              <p className="p-3 text-sm text-muted">
                Bu dosya tarayıcıda gösterilemiyor.
              </p>
            </object>
          )}
          <a
            href={file.url} download="dekont"
            className="inline-flex min-h-11 items-center justify-center rounded-xl border
                       border-[var(--border)] px-3.5 text-sm font-medium
                       hover:bg-[var(--raised)]"
          >
            Dekontu indir
          </a>
        </div>
      )}
    </div>
  );
}
