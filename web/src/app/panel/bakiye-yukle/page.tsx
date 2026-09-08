'use client';

import * as React from 'react';
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ApiError, apiFetch } from '@/lib/api';
import { formatDateTime, formatMoney } from '@/lib/format';
import { useSession } from '@/hooks/useSession';
import { Alert, Badge, Button, Card, Empty, Field, Skeleton, cx } from '@/components/ui';
import { Modal } from '@/components/modal';
import type { Deposit, DepositStatus, PublicDepositMethod } from '@/lib/types';

const PAGE = 10;
const depositsKey = ['wallet', 'deposits'] as const;

/** Dekont sınırları — sunucudaki `storage.MaxReceiptBytes` ve sihirli bayt kontrolü ile aynı. */
const MAX_RECEIPT_BYTES = 5 * 1024 * 1024;
const RECEIPT_TYPES = ['image/jpeg', 'image/png', 'application/pdf'];

/**
 * Yöntem yapılandırma alanlarının Türkçe adları.
 *
 * Anahtarlar sunucudaki zorunlu alan listesiyle aynıdır (handler/admin.go):
 * BANK_TRANSFER → iban, hesapAdi · CRYPTO → cuzdanAdresi, ag.
 */
const CONFIG_LABELS: Record<string, string> = {
  iban: 'IBAN',
  hesapAdi: 'Hesap adı',
  cuzdanAdresi: 'Cüzdan adresi',
  ag: 'Ağ',
  banka: 'Banka',
  aciklama: 'Açıklama',
};

/** Elle yazılması hataya açık olan alanlar — kopyala düğmesi ZORUNLU. */
const COPYABLE = new Set(['iban', 'cuzdanAdresi']);

/** Yapılandırma alanları bu sırayla gösterilir; listede olmayanlar sona eklenir. */
const CONFIG_ORDER = ['banka', 'hesapAdi', 'iban', 'ag', 'cuzdanAdresi', 'aciklama'];

const statusTone: Record<DepositStatus, 'ok' | 'warn' | 'bad' | 'neutral'> = {
  PENDING: 'warn',
  COMPLETED: 'ok',
  REJECTED: 'bad',
  REFUNDED: 'neutral',
};

/**
 * TL girişi → kuruş.
 *
 * `12.50 * 100` JavaScript'te 1249.9999… verir ve bir kuruş kaybolur. Çevrim
 * metin üzerinden, tam sayı aritmetiğiyle yapılır. Bu bir PARA HESABI değil,
 * girdi ayrıştırmasıdır; alt/üst sınırı sunucu ayrıca doğrular.
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
 * Kopyala düğmesi.
 *
 * `navigator.clipboard` GÜVENLİ OLMAYAN BAĞLAMDA (http://) ve bazı iOS
 * sürümlerinde YOKTUR. Yedek olarak eski `execCommand('copy')` yolu denenir;
 * o da olmazsa kullanıcıya "elle seçin" denir — sessizce hiçbir şey yapmayan
 * bir düğme en kötüsüdür, hele kopyalanan şey bir IBAN ise: elle yazılan bir
 * IBAN yanlış hesaba para göndermek demektir.
 */
function CopyButton({ value, label }: { value: string; label: string }) {
  const [state, setState] = React.useState<'idle' | 'ok' | 'fail'>('idle');

  async function copy() {
    let ok = false;
    try {
      if (navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(value);
        ok = true;
      } else {
        ok = legacyCopy(value);
      }
    } catch {
      ok = legacyCopy(value);
    }
    setState(ok ? 'ok' : 'fail');
    window.setTimeout(() => setState('idle'), 2500);
  }

  return (
    <button
      type="button"
      onClick={copy}
      aria-label={label}
      className={cx(
        'grid size-11 shrink-0 place-items-center rounded-xl border transition-colors active:scale-95',
        state === 'ok' ? 'border-[var(--color-ok)] text-[var(--color-ok)]'
          : state === 'fail' ? 'border-[var(--color-bad)] text-[var(--color-bad)]'
          : 'border-[var(--border)] text-muted',
      )}
    >
      {state === 'ok' ? (
        <svg viewBox="0 0 24 24" className="size-5" fill="none" stroke="currentColor"
             strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
          <path d="m5 13 4 4L19 7" />
        </svg>
      ) : (
        <svg viewBox="0 0 24 24" className="size-5" fill="none" stroke="currentColor"
             strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
          <rect x="9" y="9" width="12" height="12" rx="2.4" />
          <path d="M5 15V5a2 2 0 0 1 2-2h10" />
        </svg>
      )}
      <span className="sr-only" aria-live="polite">
        {state === 'ok' ? 'Kopyalandı' : state === 'fail' ? 'Kopyalanamadı, elle seçin' : ''}
      </span>
    </button>
  );
}

function legacyCopy(value: string): boolean {
  try {
    const ta = document.createElement('textarea');
    ta.value = value;
    // Ekran dışına almak yerine görünmez yapmak iOS'ta seçimi bozar;
    // sabit konumlandırıp opaklığı sıfırlarız.
    ta.style.cssText = 'position:fixed;top:0;left:0;opacity:0;pointer-events:none';
    ta.setAttribute('readonly', '');
    document.body.appendChild(ta);
    ta.select();
    ta.setSelectionRange(0, value.length);
    const ok = document.execCommand('copy');
    document.body.removeChild(ta);
    return ok;
  } catch {
    return false;
  }
}

/* ═══════════════════════ Dekont yükleme ═══════════════════════ */

function ReceiptUploader({ depositId, onDone }: { depositId: string; onDone: () => void }) {
  const [file, setFile] = React.useState<File | null>(null);
  const [localError, setLocalError] = React.useState('');

  const upload = useMutation({
    mutationFn: (f: File) => {
      const fd = new FormData();
      // Alan adı sunucudaki `receiptField` sabitiyle AYNI olmalı.
      fd.append('dekont', f);
      return apiFetch<Deposit>(`/wallet/deposits/${encodeURIComponent(depositId)}/receipt`, {
        method: 'POST',
        body: fd,
        // Varsayılan 15 sn, mobil ağda 5 MB'lık bir dekont için yetmez.
        timeoutMs: 60_000,
      });
    },
    // Yükleme tekrarlanmaz: aynı dosya ikinci kez diske yazılır.
    retry: false,
    onSuccess: onDone,
  });

  function pick(e: React.ChangeEvent<HTMLInputElement>) {
    upload.reset();
    const f = e.target.files?.[0] ?? null;
    setLocalError('');
    if (!f) { setFile(null); return; }
    // Sunucu dosyayı sihirli baytlarına göre ayrıca doğrular; buradaki kontrol
    // yalnız kullanıcıyı 5 MB'lık boşa yüklemeden kurtarmak içindir.
    if (f.size > MAX_RECEIPT_BYTES) {
      setFile(null);
      setLocalError('Dekont dosyası en fazla 5 MB olabilir.');
      return;
    }
    if (f.type && !RECEIPT_TYPES.includes(f.type)) {
      setFile(null);
      setLocalError('Yalnız JPEG, PNG veya PDF dosyası yükleyebilirsiniz.');
      return;
    }
    setFile(f);
  }

  const err = upload.error instanceof ApiError ? upload.error : null;

  return (
    <div className="flex flex-col gap-3">
      <label className="flex flex-col gap-1.5">
        <span className="text-sm font-medium">Dekont dosyası</span>
        <input
          // Modal içinde açıldığında ilk odak buraya gelsin (modal.tsx).
          data-autofocus
          type="file"
          accept="image/jpeg,image/png,application/pdf"
          onChange={pick}
          className="raised min-h-12 w-full rounded-xl border px-3 py-2.5 text-base
                     outline-none file:mr-3 file:min-h-9 file:rounded-lg file:border-0
                     file:bg-brand-500 file:px-3 file:text-sm file:text-white
                     focus:border-brand-400"
        />
        <span className="text-xs text-muted">JPEG, PNG veya PDF · en fazla 5 MB.</span>
      </label>

      {localError && <Alert>{localError}</Alert>}
      {err && <ErrorBox err={err} />}

      <Button
        fullWidth
        disabled={!file}
        loading={upload.isPending}
        onClick={() => file && upload.mutate(file)}
      >
        Dekontu yükle
      </Button>
    </div>
  );
}

/* ═══════════════════════════════ Ekran ═══════════════════════════════ */

/**
 * yeniAnahtar idempotens anahtarı üretir.
 *
 * `crypto.randomUUID` eski Safari'de ve güvenli olmayan bağlamda YOKTUR;
 * o durumda getRandomValues'a, o da yoksa zaman+rastgele birleşimine düşeriz.
 * Anahtarın gizli olması gerekmez — yalnız aynı kullanıcı içinde tekrarlamaması
 * yeter (sunucudaki tekil indeks kullanıcı kapsamlıdır).
 */
function yeniAnahtar(): string {
  const c = globalThis.crypto;
  if (c?.randomUUID) return c.randomUUID();
  if (c?.getRandomValues) {
    const b = new Uint8Array(16);
    c.getRandomValues(b);
    return Array.from(b, (x) => x.toString(16).padStart(2, '0')).join('');
  }
  return `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 12)}`;
}

export default function DepositPage() {
  const qc = useQueryClient();
  const { user } = useSession();

  const methods = useQuery({
    queryKey: ['wallet', 'deposit-methods'],
    queryFn: () => apiFetch<{ items: PublicDepositMethod[] }>('/wallet/deposit-methods'),
  });

  const [offset, setOffset] = React.useState(0);
  const list = useQuery({
    queryKey: [...depositsKey, { limit: PAGE, offset }],
    queryFn: () => apiFetch<{ items: Deposit[]; total: number; limit: number; offset: number }>(
      `/wallet/deposits?limit=${PAGE}&offset=${offset}`,
    ),
    placeholderData: keepPreviousData,
  });

  const [methodId, setMethodId] = React.useState('');
  const [amount, setAmount] = React.useState('');
  const [reference, setReference] = React.useState('');
  const [note, setNote] = React.useState('');
  const [errors, setErrors] = React.useState<Record<string, string>>({});
  const [created, setCreated] = React.useState<Deposit | null>(null);
  const [receiptFor, setReceiptFor] = React.useState<Deposit | null>(null);

  const items = methods.data?.items ?? [];
  const method = items.find((m) => m.id === methodId) ?? null;

  /*
   * İDEMPOTENS ANAHTARI: form başına BİR kez üretilir ve talep GERÇEKTEN
   * oluşana kadar korunur.
   *
   * 🔴 NEDEN GEREKLİ: kullanıcı havaleyi yapmış, formu göndermiş, mobil ağda
   * yanıt kaybolmuş olabilir (tünel, zaman aşımı, uygulamayı arka plana alma).
   * Sunucu talebi YAZDI ama kullanıcı hata gördü ve tekrar basıyor. Anahtar
   * olmadan aynı havale için iki bekleyen talep oluşur; yönetici ikisini de
   * onaylarsa kullanıcıya iki kat yazılır.
   *
   * Sunucu bu anahtarı `Idempotency-Key` başlığından okur ve tekrarda MEVCUT
   * talebi geri döndürür (api/internal/service/deposit/service.go).
   */
  const [idemKey, setIdemKey] = React.useState(() => yeniAnahtar());

  const create = useMutation({
    mutationFn: (v: { methodId: string; amountMinor: number; reference: string; note: string }) =>
      apiFetch<Deposit>('/wallet/deposits', { method: 'POST', body: v, idempotencyKey: idemKey }),
    // Talep oluşturmak sunucuda idempotenttir ama OTOMATİK TEKRAR yine de
    // yapılmaz: kullanıcı ne olduğunu görmeli ve kararı o vermeli.
    retry: false,
    onSuccess: (dep) => {
      // Talep oluştu — sıradaki için YENİ anahtar.
      setIdemKey(yeniAnahtar());
      qc.invalidateQueries({ queryKey: depositsKey });
      setCreated(dep);
      setAmount(''); setReference(''); setNote('');
      setErrors({});
    },
    onError: () => {
      // 🔴 HATA YOLUNDA DA LİSTE TAZELENİR. İstek sunucuya ULAŞMIŞ ama yanıt
      // kaybolmuş olabilir: talep oluştu, kullanıcı hata gördü. Listeyi
      // tazelemezsek kullanıcı oluşmuş talebi göremez ve tekrar dener.
      // (Anahtar aynı kaldığı için sunucu ikinciyi yutar, ama kullanıcının
      //  ekranında ne olduğunu GÖRMESİ ayrı bir mesele.)
      qc.invalidateQueries({ queryKey: depositsKey });
    },
  });

  function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    create.reset();
    const errs: Record<string, string> = {};

    if (!method) errs.methodId = 'Bir ödeme yöntemi seçiniz.';

    const parsed = toMinor(amount);
    if ('error' in parsed) {
      errs.amountMinor = parsed.error;
    } else if (method) {
      if (parsed.minor < method.minAmount.minor) {
        errs.amountMinor = `En az ${formatMoney(method.minAmount)} yükleyebilirsiniz.`;
      } else if (method.maxAmount.minor > 0 && parsed.minor > method.maxAmount.minor) {
        // 🔴 maxAmount.minor === 0 "üst sınır yok" demektir, 0,00 ₺ tavan DEĞİL.
        errs.amountMinor = `En fazla ${formatMoney(method.maxAmount)} yükleyebilirsiniz.`;
      }
    }

    const ref = reference.trim();
    if (ref.length < 4) {
      errs.reference = 'Bu alan zorunludur (en az 4 karakter).';
    } else if (ref.length > 120) {
      errs.reference = 'En fazla 120 karakter olabilir.';
    }
    if (note.trim().length > 300) errs.note = 'Not en fazla 300 karakter olabilir.';

    setErrors(errs);
    if (Object.keys(errs).length || !method) return;

    create.mutate({
      methodId: method.id,
      amountMinor: (parsed as { minor: number }).minor,
      reference: ref,
      note: note.trim(),
    });
  }

  const amountPreview = React.useMemo(() => {
    const p = toMinor(amount);
    if ('error' in p) return null;
    return formatMoney({ minor: p.minor, currency: 'TRY', formatted: '' });
  }, [amount]);

  const createErr = create.error instanceof ApiError ? create.error : null;
  const methodsErr = methods.error instanceof ApiError ? methods.error : null;
  const listErr = list.error instanceof ApiError ? list.error : null;

  const total = list.data?.total ?? 0;
  const hasPrev = offset > 0;
  const hasNext = offset + PAGE < total;

  const configKeys = method
    ? [...CONFIG_ORDER.filter((k) => method.config[k]),
       ...Object.keys(method.config).filter((k) => !CONFIG_ORDER.includes(k) && method.config[k])]
    : [];

  return (
    <div className="mx-auto flex max-w-3xl flex-col gap-5">
      <div>
        <h1 className="text-2xl font-bold tracking-tight md:text-3xl">Bakiye yükle</h1>
        <p className="mt-1 text-sm text-muted">
          Mevcut bakiyeniz: <strong className="text-[var(--text)]">{formatMoney(user?.balance)}</strong>
        </p>
      </div>

      <Alert tone="info">
        Önce parayı aşağıdaki hesaba gönderin, sonra bu formu doldurun. Talebiniz
        kontrol edildikten sonra bakiyeniz tanımlanır; onay anında değil, kontrol
        sonrasında yansır.
      </Alert>

      {/* ═══════════ Yöntem seçimi ═══════════ */}
      <Card>
        <h2 className="text-lg font-semibold">Ödeme yöntemi</h2>

        {methods.isLoading ? (
          <div className="mt-4 flex flex-col gap-2">
            {[0, 1].map((i) => <Skeleton key={i} className="h-16" />)}
          </div>
        ) : methodsErr ? (
          <ErrorBox err={methodsErr} className="mt-4" />
        ) : methods.isError ? (
          <Alert className="mt-4">
            Ödeme yöntemleri yüklenemedi. Bağlantınızı kontrol edip tekrar deneyin.
          </Alert>
        ) : !items.length ? (
          <Empty
            title="Şu an aktif ödeme yöntemi yok"
            hint="Yükleme geçici olarak kapalı. Kısa süre sonra tekrar deneyin."
          />
        ) : (
          <ul className="mt-4 flex flex-col gap-2">
            {items.map((m) => (
              <li key={m.id}>
                <label className={cx(
                  'flex items-start gap-3 rounded-xl border p-3',
                  m.id === methodId ? 'border-brand-500 bg-brand-500/5' : 'border-[var(--border)]',
                )}>
                  <input
                    type="radio"
                    name="odeme-yontemi"
                    value={m.id}
                    checked={m.id === methodId}
                    onChange={() => { setMethodId(m.id); setErrors({}); }}
                    className="mt-0.5 size-5 shrink-0 accent-[var(--color-brand-500)]"
                  />
                  <span className="min-w-0">
                    <span className="block text-sm font-medium">{m.name}</span>
                    <span className="mt-0.5 block text-xs text-muted">
                      En az {formatMoney(m.minAmount)}
                      {m.maxAmount.minor > 0 ? ` · En fazla ${formatMoney(m.maxAmount)}` : ''}
                      {m.kind === 'CRYPTO' ? ' · Kripto' : ' · Banka havalesi/EFT'}
                    </span>
                  </span>
                </label>
              </li>
            ))}
          </ul>
        )}
        {errors.methodId && (
          <p role="alert" className="mt-2 text-xs text-[var(--color-bad)]">{errors.methodId}</p>
        )}
      </Card>

      {/* ═══════════ Hesap bilgileri ═══════════ */}
      {method && (
        <Card>
          <h2 className="text-lg font-semibold">Nereye göndereceksiniz</h2>

          {method.instructions && (
            <p className="mt-2 whitespace-pre-line text-sm leading-relaxed text-muted">
              {method.instructions}
            </p>
          )}

          <dl className="mt-4 flex flex-col gap-3">
            {configKeys.map((k) => {
              const value = method.config[k] ?? '';
              return (
                <div key={k} className="flex items-center justify-between gap-3
                                        rounded-xl border border-[var(--border)] p-3">
                  <div className="min-w-0">
                    <dt className="text-xs font-medium uppercase tracking-wide text-muted">
                      {CONFIG_LABELS[k] ?? k}
                    </dt>
                    {/* select-text ZORUNLU: mobilde uzun basıp kopyalamak en yaygın yol. */}
                    <dd className="mt-0.5 select-text break-anywhere font-mono text-sm">
                      {value}
                    </dd>
                  </div>
                  {COPYABLE.has(k) && (
                    <CopyButton value={value} label={`${CONFIG_LABELS[k] ?? k} kopyala`} />
                  )}
                </div>
              );
            })}
          </dl>

          {method.kind === 'CRYPTO' && (
            <Alert tone="warn" className="mt-4">
              Gönderimi yalnız <strong>{method.config.ag || 'belirtilen'}</strong> ağı
              üzerinden yapın. Başka bir ağdan gönderilen tutar geri getirilemez.
            </Alert>
          )}
        </Card>
      )}

      {/* ═══════════ Talep formu ═══════════ */}
      {method && (
        <Card>
          <h2 className="text-lg font-semibold">Yükleme talebi</h2>

          <form onSubmit={onSubmit} className="mt-4 flex flex-col gap-4" noValidate>
            <Field
              label="Tutar (TL)"
              value={amount}
              onChange={(e) => setAmount(e.target.value)}
              inputMode="decimal" autoComplete="off"
              placeholder="Örn. 250,00"
              error={errors.amountMinor}
              hint={amountPreview
                ? `Bildirilen tutar: ${amountPreview}`
                : `En az ${formatMoney(method.minAmount)}${
                    method.maxAmount.minor > 0 ? `, en fazla ${formatMoney(method.maxAmount)}` : ''}`}
            />

            <Field
              label={method.referenceLabel}
              value={reference}
              onChange={(e) => setReference(e.target.value)}
              autoCapitalize="none" autoCorrect="off" spellCheck={false}
              maxLength={120}
              error={errors.reference}
              hint={method.kind === 'CRYPTO'
                ? 'Gönderdiğiniz işlemin zincirdeki hash değeri.'
                : 'Havale ekranında yazdığınız açıklama ya da dekont numarası.'}
            />

            <label className="flex flex-col gap-1.5">
              <span className="text-sm font-medium">Not (isteğe bağlı)</span>
              <textarea
                value={note}
                onChange={(e) => setNote(e.target.value)}
                rows={2}
                maxLength={300}
                placeholder="Eklemek istediğiniz bir şey varsa yazın."
                aria-invalid={errors.note ? true : undefined}
                className={cx(
                  'raised w-full rounded-xl border px-3.5 py-2.5 text-base outline-none',
                  'placeholder:text-[var(--muted)] focus:border-brand-400',
                  errors.note && 'border-[var(--color-bad)]',
                )}
              />
              {errors.note && (
                <span role="alert" className="text-xs text-[var(--color-bad)]">{errors.note}</span>
              )}
            </label>

            {method.receiptRequired && (
              <Alert tone="info">
                Bu yöntemde <strong>dekont</strong> bekleniyor. Talebi oluşturduktan
                sonra dekontu yükleyebilirsiniz.
              </Alert>
            )}

            <Button type="submit" fullWidth loading={create.isPending}>
              Talebi oluştur
            </Button>
          </form>

          {createErr && <ErrorBox err={createErr} className="mt-4" />}
        </Card>
      )}

      {/* ═══════════ Oluşturulan talep + dekont ═══════════ */}
      {created && (
        <Card>
          <div className="flex flex-wrap items-center justify-between gap-2">
            <h2 className="text-lg font-semibold">Talebiniz alındı</h2>
            <Badge tone={statusTone[created.status]}>{created.statusLabel}</Badge>
          </div>
          <p className="mt-2 text-sm text-muted">
            <strong className="text-[var(--text)]">{formatMoney(created.amount)}</strong> tutarındaki
            talebiniz {created.method} yöntemiyle kaydedildi. Kontrol edildikten sonra
            bakiyenize yansıyacak.
          </p>

          {!created.hasReceipt ? (
            <div className="mt-4">
              <ReceiptUploader
                depositId={created.id}
                onDone={() => {
                  qc.invalidateQueries({ queryKey: depositsKey });
                  setCreated({ ...created, hasReceipt: true });
                }}
              />
            </div>
          ) : (
            <Alert tone="ok" className="mt-4">Dekont yüklendi.</Alert>
          )}
        </Card>
      )}

      {/* ═══════════ Geçmiş talepler ═══════════ */}
      <Card>
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h2 className="text-lg font-semibold">Yükleme talepleriniz</h2>
          {total > 0 && <Badge tone="neutral">{total} talep</Badge>}
        </div>

        {list.isLoading ? (
          <div className="mt-4 flex flex-col gap-2">
            {[0, 1, 2].map((i) => <Skeleton key={i} className="h-20" />)}
          </div>
        ) : listErr ? (
          <ErrorBox err={listErr} className="mt-4" />
        ) : list.isError ? (
          <Alert className="mt-4">
            Talepleriniz yüklenemedi. Bağlantınızı kontrol edip tekrar deneyin.
          </Alert>
        ) : !list.data?.items.length ? (
          <Empty title="Henüz talep yok" hint="İlk yükleme talebiniz burada görünecek." />
        ) : (
          <>
            {/* MOBİL: kart listesi. Yatay kaydırılan tablo kabul edilmez. */}
            <ul className="mt-4 flex flex-col gap-2 md:hidden">
              {list.data.items.map((d) => (
                <li key={d.id} className="raised rounded-xl border p-3">
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                      <p className="break-anywhere text-sm font-medium">{d.method}</p>
                      <p className="mt-0.5 text-xs text-muted">{formatDateTime(d.createdAt)}</p>
                    </div>
                    <span className="shrink-0 text-sm font-semibold">{formatMoney(d.amount)}</span>
                  </div>
                  <div className="mt-2 flex flex-wrap items-center gap-2 border-t
                                  border-[var(--border)] pt-2">
                    <Badge tone={statusTone[d.status]}>{d.statusLabel}</Badge>
                    {d.status === 'COMPLETED' && (
                      <span className="text-xs text-muted">
                        Yansıyan: {formatMoney(d.credited)}
                      </span>
                    )}
                    {d.hasReceipt && <Badge tone="neutral">Dekont var</Badge>}
                  </div>
                  {d.rejectionReason && (
                    <Alert className="mt-2">
                      <span className="break-anywhere">Red nedeni: {d.rejectionReason}</span>
                    </Alert>
                  )}
                  {d.status === 'PENDING' && !d.hasReceipt && (
                    <Button variant="outline" size="sm" fullWidth className="mt-2"
                            onClick={() => setReceiptFor(d)}>
                      Dekont yükle
                    </Button>
                  )}
                </li>
              ))}
            </ul>

            {/* MASAÜSTÜ: gerçek tablo */}
            <div className="mt-4 hidden md:block">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-[var(--border)] text-left text-xs
                                 uppercase tracking-wide text-muted">
                    <th scope="col" className="py-2 pr-3 font-medium">Tarih</th>
                    <th scope="col" className="py-2 pr-3 font-medium">Yöntem</th>
                    <th scope="col" className="py-2 pr-3 text-right font-medium">Tutar</th>
                    <th scope="col" className="py-2 pr-3 font-medium">Durum</th>
                    <th scope="col" className="py-2 text-right font-medium">Dekont</th>
                  </tr>
                </thead>
                <tbody>
                  {list.data.items.map((d) => (
                    <tr key={d.id} className="border-b border-[var(--border)] last:border-0">
                      <td className="py-3 pr-3 whitespace-nowrap text-muted">
                        {formatDateTime(d.createdAt)}
                      </td>
                      <td className="py-3 pr-3 break-anywhere font-medium">{d.method}</td>
                      <td className="py-3 pr-3 text-right whitespace-nowrap">
                        {formatMoney(d.amount)}
                        {d.status === 'COMPLETED' && d.credited.minor !== d.amount.minor && (
                          <span className="block text-xs text-muted">
                            yansıyan {formatMoney(d.credited)}
                          </span>
                        )}
                      </td>
                      <td className="py-3 pr-3">
                        <Badge tone={statusTone[d.status]}>{d.statusLabel}</Badge>
                        {d.rejectionReason && (
                          <span className="mt-1 block break-anywhere text-xs text-[var(--color-bad)]">
                            {d.rejectionReason}
                          </span>
                        )}
                      </td>
                      <td className="py-3 text-right">
                        {d.hasReceipt ? (
                          <span className="text-xs text-muted">Yüklendi</span>
                        ) : d.status === 'PENDING' ? (
                          <Button variant="outline" size="sm" onClick={() => setReceiptFor(d)}>
                            Yükle
                          </Button>
                        ) : (
                          <span className="text-xs text-muted">—</span>
                        )}
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

      {/* ═══════════ Dekont yükleme (geçmiş talep) ═══════════ */}
      <Modal open={!!receiptFor} onClose={() => setReceiptFor(null)} title="Dekont yükle">
        {receiptFor && (
          <div className="flex flex-col gap-4">
            <p className="text-sm text-muted">
              <strong className="text-[var(--text)]">{formatMoney(receiptFor.amount)}</strong> tutarındaki
              {' '}{formatDateTime(receiptFor.createdAt)} tarihli talep için.
            </p>
            <ReceiptUploader
              depositId={receiptFor.id}
              onDone={() => {
                qc.invalidateQueries({ queryKey: depositsKey });
                setReceiptFor(null);
              }}
            />
            <Button variant="outline" fullWidth onClick={() => setReceiptFor(null)}>
              Vazgeç
            </Button>
          </div>
        )}
      </Modal>
    </div>
  );
}
