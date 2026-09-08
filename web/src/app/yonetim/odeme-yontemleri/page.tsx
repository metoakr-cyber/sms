'use client';

/**
 * Ödeme yöntemleri.
 *
 * Buradaki IBAN/cüzdan adresi, kullanıcının parayı GÖNDERECEĞİ yerdir. Yanlış
 * bir karakter, paranın başkasına gitmesi demektir — bu yüzden:
 *   · yeni yöntem PASİF doğar (sunucu),
 *   · eksik alanlı yöntem aktifleştirilemez (sunucu 422 + burada kilitli düğme),
 *   · `config` kısmi gönderilmez; sunucuda MERGE DEĞİL, YERİNE GEÇER.
 */

import * as React from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ApiError, apiFetch } from '@/lib/api';
import { formatMoney } from '@/lib/format';
import { Card, Button, Field, Alert, Badge, Skeleton, Empty } from '@/components/ui';
import { Modal } from '@/components/modal';
import type { DepositMethod } from '@/lib/types';

interface DepositMethodList { items: DepositMethod[] }

type Kind = DepositMethod['kind'];

const KIND_LABEL: Record<Kind, string> = {
  BANK_TRANSFER: 'Banka havalesi / EFT',
  CRYPTO: 'Kripto',
};

/**
 * Yönteme göre yapılandırma alanları.
 *
 * Anahtar adları SUNUCUYLA AYNI olmak zorundadır: aktifleştirme ön koşulu
 * `iban`/`hesapAdi` ve `cuzdanAdresi`/`ag` anahtarlarına bakar
 * (handler/admin.go: missingConfigFields). Bir harf farkı, dolu bir formun
 * "eksik alan" hatası almasıdır.
 */
const CONFIG_FIELDS: Record<Kind, Array<{ key: string; label: string; required: boolean; placeholder?: string }>> = {
  BANK_TRANSFER: [
    { key: 'banka', label: 'Banka', required: false, placeholder: 'Örn. Ziraat Bankası' },
    { key: 'hesapAdi', label: 'Hesap adı', required: true, placeholder: 'Hesabın açık adı' },
    { key: 'iban', label: 'IBAN', required: true, placeholder: 'TR00 0000 0000 0000 0000 0000 00' },
  ],
  CRYPTO: [
    { key: 'ag', label: 'Ağ', required: true, placeholder: 'Örn. TRC20' },
    { key: 'cuzdanAdresi', label: 'Cüzdan adresi', required: true, placeholder: 'Cüzdan adresi' },
  ],
};

/** Sunucunun tutar sınırları (dto/deposit.go). Bilgi amaçlı gösterilir. */
const HARD_MIN_MINOR = 1000;
const HARD_MAX_MINOR = 5_000_000;

/**
 * TL girişi → kuruş. KAYAN NOKTA YOK: `12.50 * 100` bir kuruş kaybettirebilir
 * (yonetim/bakiye/page.tsx ile aynı kalıp).
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

/**
 * Kuruş → düzenlenebilir TL metni (form ÖN DOLDURMA için).
 *
 * `formatted` alanı "₺" ve binlik ayracı taşır; girdi kutusuna konursa
 * kullanıcı onu düzenleyemez. Burada bölme YAPILMAZ; tam sayı ayrıştırması
 * kullanılır — para aritmetiği değil, gösterim ayrıştırmasıdır.
 */
function fromMinor(minor: number): string {
  const whole = Math.trunc(minor / 100);
  const frac = Math.abs(minor % 100);
  return `${whole},${String(frac).padStart(2, '0')}`;
}

/**
 * Modal açılınca odağı `[data-autofocus]` öğesine taşır.
 *
 * modal.tsx'teki `el?.focus() ?? panel.focus()` zinciri HER ZAMAN ikinci dala
 * da girer (`focus()` `undefined` döner) ve odağı panele geri alır — yani
 * `data-autofocus` tek başına işlevsizdir. Ölçümde odak diyalog panelinde
 * kalıyordu; klavye kullanıcısı forma ulaşmak için fazladan Tab basıyordu.
 */
function useDialogFocus(ref: React.RefObject<HTMLElement | null>) {
  React.useEffect(() => {
    // setTimeout(0): modal.tsx'in kendi odak etkisinden SONRA çalışsın.
    const t = window.setTimeout(() => {
      ref.current?.querySelector<HTMLElement>('[data-autofocus]')?.focus();
    }, 0);
    return () => window.clearTimeout(t);
  }, [ref]);
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
 * Ham `<select>` stili.
 *
 * `appearance-none` ZORUNLUDUR: WebKit'te yerel `menulist` görünümü yüksekliği
 * kendi hesaplar ve `min-h-12`'yi YOK SAYAR — ölçümde kutu 25 px çıkıyordu,
 * 44 px dokunma hedefinin çok altında (§2.3). Ok bu yüzden elle çizilir.
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

type Dialog =
  | { kind: 'create' }
  | { kind: 'edit'; method: DepositMethod }
  | { kind: 'delete'; method: DepositMethod }
  | null;

export default function DepositMethodsPage() {
  const qc = useQueryClient();
  const [dialog, setDialog] = React.useState<Dialog>(null);

  const q = useQuery({
    queryKey: ['admin-deposit-methods'],
    // 🔴 Yanıt {items:[...]} — total/limit/offset YOKTUR, sayfalama da yok.
    queryFn: () => apiFetch<DepositMethodList>('/admin/deposit-methods'),
  });

  const setActive = useMutation({
    mutationFn: (v: { id: string; isActive: boolean }) =>
      apiFetch<DepositMethod>(`/admin/deposit-methods/${encodeURIComponent(v.id)}/active`, {
        method: 'PATCH',
        body: { isActive: v.isActive },
      }),
    retry: false,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['admin-deposit-methods'] }),
  });

  const listErr = q.error instanceof ApiError ? q.error : null;
  // Aktifleştirme reddi `fields` DEĞİL, düz bir mesajdır (handler/admin.go).
  const activeErr = setActive.error instanceof ApiError ? setActive.error : null;
  const items = q.data?.items ?? [];

  return (
    <div className="mx-auto flex max-w-5xl flex-col gap-5">
      <div>
        <h1 className="text-2xl font-bold tracking-tight md:text-3xl">Ödeme yöntemleri</h1>
        <p className="mt-1 text-sm text-muted">
          Kullanıcıların bakiye yüklerken göreceği hesap bilgileri. Yalnız
          <strong> aktif </strong> yöntemler kullanıcıya gösterilir.
        </p>
      </div>

      <Alert tone="warn">
        Buradaki IBAN ve cüzdan adresi kullanıcının parayı göndereceği yerdir.
        Kaydetmeden önce karakter karakter doğrulayın; yanlış bir adres, paranın
        geri getirilemeyeceği bir yere gitmesi demektir.
      </Alert>

      {activeErr && <ErrorBox err={activeErr} />}

      <Card>
        <div className="flex flex-wrap items-center justify-between gap-3">
          <h2 className="text-lg font-semibold">Tanımlı yöntemler</h2>
          <Button size="sm" onClick={() => setDialog({ kind: 'create' })}>Yeni yöntem</Button>
        </div>

        {q.isLoading ? (
          <div className="mt-4 flex flex-col gap-2">
            {[0, 1, 2].map((i) => <Skeleton key={i} className="h-28" />)}
          </div>
        ) : listErr ? (
          <ErrorBox err={listErr} className="mt-4" />
        ) : !items.length ? (
          <Empty
            title="Henüz yöntem yok"
            hint="Kullanıcılar bakiye yükleyemez. En az bir yöntem ekleyip bilgilerini doldurun."
          />
        ) : (
          <>
            {/* MOBİL: kart listesi. Yatay kaydırılan tablo kabul edilmez (§2.5). */}
            <ul className="mt-4 flex flex-col gap-2 md:hidden">
              {items.map((m) => (
                <li key={m.id} className="raised rounded-xl border p-3">
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                      <p className="truncate text-sm font-medium">{m.name}</p>
                      <p className="truncate text-xs text-muted">
                        {KIND_LABEL[m.kind]} · <code className="font-mono">{m.code}</code>
                      </p>
                    </div>
                    <Badge tone={m.isActive ? 'ok' : 'neutral'}>
                      {m.isActive ? 'Aktif' : 'Pasif'}
                    </Badge>
                  </div>

                  <dl className="mt-3 flex flex-col gap-1.5 border-t border-[var(--border)] pt-2 text-xs">
                    <div className="flex items-center justify-between gap-3">
                      <dt className="text-muted">Tutar aralığı</dt>
                      <dd>{amountRange(m)}</dd>
                    </div>
                    <div className="flex items-center justify-between gap-3">
                      <dt className="text-muted">Sıra</dt>
                      <dd>{m.sortOrder}</dd>
                    </div>
                  </dl>

                  <ConfigSummary method={m} />
                  <MissingNotice method={m} />

                  <div className="mt-3 flex flex-col gap-2">
                    <Button variant="outline" size="sm" fullWidth
                            onClick={() => setDialog({ kind: 'edit', method: m })}>
                      Düzenle
                    </Button>
                    <ActiveButton m={m} pending={setActive.isPending}
                                  onToggle={() => setActive.mutate({ id: m.id, isActive: !m.isActive })} />
                    <Button variant="danger" size="sm" fullWidth
                            onClick={() => setDialog({ kind: 'delete', method: m })}>
                      Sil
                    </Button>
                  </div>
                </li>
              ))}
            </ul>

            {/*
              MASAÜSTÜ: gerçek tablo.

              "Tip" ve "Bilgiler" sütunları `lg:` altında GİZLENİR; ikisi de
              mobil kartta ve düzenleme formunda görünmeye devam eder. Altı
              sütun + üç düğme 768 px'e sığmıyordu ve zorlamak tabloyu yatay
              kaydırmaya iterdi (§2.5).
            */}
            <div className="mt-4 hidden md:block">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-[var(--border)] text-left text-xs
                                 uppercase tracking-wide text-muted">
                    <th scope="col" className="py-2 pr-3 font-medium">Yöntem</th>
                    <th scope="col" className="hidden py-2 pr-3 font-medium lg:table-cell">Tip</th>
                    <th scope="col" className="hidden py-2 pr-3 font-medium lg:table-cell">Bilgiler</th>
                    <th scope="col" className="py-2 pr-3 font-medium">Tutar aralığı</th>
                    <th scope="col" className="py-2 pr-3 font-medium">Durum</th>
                    <th scope="col" className="py-2 text-right font-medium">
                      <span className="sr-only">İşlemler</span>
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {items.map((m) => (
                    <tr key={m.id} className="border-b border-[var(--border)] align-top last:border-0">
                      <td className="max-w-[9rem] py-3 pr-3 lg:max-w-[12rem]">
                        <span className="block truncate font-medium">{m.name}</span>
                        <code className="block truncate font-mono text-xs text-muted">{m.code}</code>
                      </td>
                      <td className="hidden py-3 pr-3 whitespace-nowrap lg:table-cell">
                        {KIND_LABEL[m.kind]}
                      </td>
                      <td className="hidden max-w-[16rem] py-3 pr-3 lg:table-cell">
                        <ConfigSummary method={m} dense />
                      </td>
                      <td className="py-3 pr-3">{amountRange(m)}</td>
                      <td className="py-3 pr-3">
                        <Badge tone={m.isActive ? 'ok' : 'neutral'}>
                          {m.isActive ? 'Aktif' : 'Pasif'}
                        </Badge>
                        <MissingNotice method={m} dense />
                      </td>
                      <td className="py-3">
                        <div className="flex flex-wrap justify-end gap-2">
                          <Button variant="outline" size="sm"
                                  onClick={() => setDialog({ kind: 'edit', method: m })}>
                            Düzenle
                          </Button>
                          <ActiveButton m={m} pending={setActive.isPending}
                                        onToggle={() => setActive.mutate({ id: m.id, isActive: !m.isActive })} />
                          <Button variant="danger" size="sm"
                                  onClick={() => setDialog({ kind: 'delete', method: m })}>
                            Sil
                          </Button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </>
        )}
      </Card>

      {dialog?.kind === 'create' && (
        <MethodForm onClose={() => setDialog(null)} />
      )}
      {dialog?.kind === 'edit' && (
        <MethodForm key={dialog.method.id} method={dialog.method} onClose={() => setDialog(null)} />
      )}
      {dialog?.kind === 'delete' && (
        <DeleteDialog key={dialog.method.id} method={dialog.method} onClose={() => setDialog(null)} />
      )}
    </div>
  );
}

/* ═══════════════════════ Liste parçaları ═══════════════════════ */

/** `maxAmount.minor === 0` ÜST SINIR YOK demektir; 0,00 ₺ tavan DEĞİL. */
function amountRange(m: DepositMethod): string {
  const min = formatMoney(m.minAmount);
  return m.maxAmount.minor === 0 ? `${min} ve üzeri` : `${min} – ${formatMoney(m.maxAmount)}`;
}

function ConfigSummary({ method, dense }: { method: DepositMethod; dense?: boolean }) {
  const fields = CONFIG_FIELDS[method.kind];
  const filled = fields.filter((f) => (method.config[f.key] ?? '').trim() !== '');
  if (!filled.length) {
    return <p className={dense ? 'text-xs text-muted' : 'mt-2 text-xs text-muted'}>Bilgi girilmemiş</p>;
  }
  return (
    <dl className={dense ? 'flex flex-col gap-0.5 text-xs' : 'mt-2 flex flex-col gap-0.5 text-xs'}>
      {filled.map((f) => (
        <div key={f.key} className="flex gap-2">
          <dt className="shrink-0 text-muted">{f.label}:</dt>
          <dd className="min-w-0 break-anywhere">{method.config[f.key]}</dd>
        </div>
      ))}
    </dl>
  );
}

function MissingNotice({ method, dense }: { method: DepositMethod; dense?: boolean }) {
  const missing = method.missingFields ?? [];
  if (!missing.length) return null;
  return (
    <p className={`text-xs text-[var(--color-warn)] ${dense ? 'mt-1.5' : 'mt-2'}`}>
      Aktifleştirilemez — eksik: {missing.join(', ')}
    </p>
  );
}

/**
 * Aktifleştirme düğmesi.
 *
 * Eksik alan varsa düğme KİLİTLİDİR ve nedeni yanında yazar. Sunucu da
 * reddeder (422); buradaki kilit, yöneticiyi anlamsız bir hataya çarptırmamak
 * içindir. Pasifleştirme her zaman serbesttir — bir yöntemi hızla kapatmak
 * için hiçbir ön koşul aranmaz.
 */
function ActiveButton({
  m, pending, onToggle,
}: { m: DepositMethod; pending: boolean; onToggle: () => void }) {
  const blocked = !m.isActive && (m.missingFields?.length ?? 0) > 0;
  return (
    <Button
      variant="outline" size="sm"
      // Mobil kartta tam genişlik, masaüstü tablo hücresinde içerik kadar.
      className="w-full md:w-auto"
      disabled={pending || blocked}
      title={blocked ? `Önce doldurun: ${m.missingFields?.join(', ')}` : undefined}
      onClick={onToggle}
    >
      {m.isActive ? 'Pasifleştir' : 'Aktifleştir'}
    </Button>
  );
}

/* ═══════════════════════ Ekle / düzenle ═══════════════════════ */

function MethodForm({ method, onClose }: { method?: DepositMethod; onClose: () => void }) {
  const qc = useQueryClient();
  const isEdit = !!method;

  const [code, setCode] = React.useState(method?.code ?? '');
  const [kind, setKind] = React.useState<Kind>(method?.kind ?? 'BANK_TRANSFER');
  const [name, setName] = React.useState(method?.name ?? '');
  const [instructions, setInstructions] = React.useState(method?.instructions ?? '');
  const [minAmount, setMinAmount] = React.useState(
    method ? fromMinor(method.minAmount.minor) : '',
  );
  const [maxAmount, setMaxAmount] = React.useState(
    method && method.maxAmount.minor > 0 ? fromMinor(method.maxAmount.minor) : '',
  );
  const [sortOrder, setSortOrder] = React.useState(String(method?.sortOrder ?? 0));

  /*
   * config BÜTÜN OLARAK tutulur.
   *
   * Sunucu gelen config'i mevcutla BİRLEŞTİRMEZ, üzerine yazar: yalnız
   * "hesapAdi" gönderen bir istek IBAN'ı SİLER. Bu yüzden formun durumu
   * mevcut config'in TAM kopyasıdır — formda göstermediğimiz (ileride
   * eklenmiş) anahtarlar da burada durur ve aynen geri gider.
   */
  const [config, setConfig] = React.useState<Record<string, string>>({ ...(method?.config ?? {}) });
  const [errors, setErrors] = React.useState<Record<string, string>>({});

  const bodyRef = React.useRef<HTMLFormElement>(null);
  useDialogFocus(bodyRef);

  const save = useMutation({
    mutationFn: (body: Record<string, unknown>) =>
      isEdit
        ? apiFetch<DepositMethod>(`/admin/deposit-methods/${encodeURIComponent(method.id)}`, {
            method: 'PATCH', body,
          })
        : apiFetch<DepositMethod>('/admin/deposit-methods', { method: 'POST', body }),
    retry: false,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['admin-deposit-methods'] });
      onClose();
    },
  });

  const fields = CONFIG_FIELDS[kind];

  function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    const errs: Record<string, string> = {};

    if (!isEdit) {
      const c = code.trim();
      if (!c) errs.code = 'Kod zorunludur.';
      else if (c.length > 40) errs.code = 'Kod en fazla 40 karakter olabilir.';
    }
    if (!name.trim()) errs.name = 'Ad zorunludur.';

    const min = toMinor(minAmount);
    if ('error' in min) errs.minAmount = min.error;
    else if (min.minor < HARD_MIN_MINOR) {
      errs.minAmount = 'En az tutar 10,00 ₺ altına inemez (sistem alt sınırı).';
    }

    // Boş = üst sınır yok → 0 gönderilir.
    const maxRaw = maxAmount.trim();
    let maxMinor = 0;
    if (maxRaw !== '') {
      const max = toMinor(maxRaw);
      if ('error' in max) errs.maxAmount = max.error;
      else {
        maxMinor = max.minor;
        if (maxMinor > HARD_MAX_MINOR) {
          errs.maxAmount = 'En çok tutar 50.000,00 ₺ üstüne çıkamaz (sistem üst sınırı).';
        } else if ('minor' in min && maxMinor < min.minor) {
          errs.maxAmount = 'En çok tutar, en az tutardan küçük olamaz.';
        }
      }
    }

    const order = Number(sortOrder.trim());
    if (!Number.isInteger(order) || order < 0) errs.sortOrder = 'Sıra 0 veya pozitif tam sayı olmalıdır.';

    for (const f of fields) {
      if (f.required && (config[f.key] ?? '').trim() === '') {
        // Sunucu bu alanları AKTİFLEŞTİRMEDE zorunlu tutar; kaydetmede değil.
        // Yine de burada uyarırız: kaydedip aktifleştiremeyen yönetici,
        // hatayı iki ekran sonra görür.
        errs[`config.${f.key}`] = `${f.label} olmadan yöntem aktifleştirilemez.`;
      }
    }

    setErrors(errs);
    if (Object.keys(errs).length) return;

    const cleanConfig: Record<string, string> = {};
    for (const [k, v] of Object.entries(config)) {
      const t = v.trim();
      if (t !== '') cleanConfig[k] = t;
    }

    const body: Record<string, unknown> = {
      name: name.trim(),
      instructions: instructions.trim(),
      config: cleanConfig, // TAM config — kısmi gönderim mevcut bilgileri siler
      minAmountMinor: 'minor' in min ? min.minor : 0,
      maxAmountMinor: maxMinor,
      sortOrder: order,
    };
    if (!isEdit) {
      body.code = code.trim();
      body.kind = kind;
    }
    save.mutate(body);
  }

  const err = save.error instanceof ApiError ? save.error : null;

  return (
    <Modal open onClose={onClose} title={isEdit ? 'Yöntemi düzenle' : 'Yeni ödeme yöntemi'}>
      <form ref={bodyRef} onSubmit={onSubmit} className="flex flex-col gap-4" noValidate>
        {isEdit ? (
          <Alert tone="info">
            Kod ve tip değiştirilemez. Bilgiler kaydedildiğinde eski değerlerin
            <strong> yerine geçer</strong>; boş bıraktığınız bir alan silinir.
          </Alert>
        ) : (
          <Alert tone="info">
            Yeni yöntem <strong>pasif</strong> olarak eklenir. Bilgilerini
            doldurup listeden aktifleştirene kadar kullanıcıya görünmez.
          </Alert>
        )}

        {!isEdit && (
          <>
            <Field
              data-autofocus
              label="Kod"
              value={code} onChange={(e) => setCode(e.target.value)}
              placeholder="Örn. ziraat-tl"
              autoCapitalize="none" autoCorrect="off" spellCheck={false}
              maxLength={40} error={errors.code}
              hint="Benzersiz, değiştirilemez teknik ad."
            />
            <label className="flex flex-col gap-1.5">
              <span className="text-sm font-medium">Tip</span>
              <div className="relative">
                <select
                  className={selectClass}
                  value={kind}
                  onChange={(e) => setKind(e.target.value as Kind)}
                >
                  <option value="BANK_TRANSFER">{KIND_LABEL.BANK_TRANSFER}</option>
                  <option value="CRYPTO">{KIND_LABEL.CRYPTO}</option>
                </select>
                <Chevron />
              </div>
              <span className="text-xs text-muted">
                İstenen bilgiler tipe göre değişir ve sonradan değiştirilemez.
              </span>
            </label>
          </>
        )}

        <Field
          data-autofocus={isEdit ? true : undefined}
          label="Kullanıcıya görünen ad"
          value={name} onChange={(e) => setName(e.target.value)}
          placeholder="Örn. Ziraat Bankası (TL)"
          maxLength={120} error={errors.name}
        />

        <div className="flex flex-col gap-3 rounded-xl border border-[var(--border)] p-3">
          <p className="text-sm font-medium">{KIND_LABEL[kind]} bilgileri</p>
          {fields.map((f) => (
            <Field
              key={f.key}
              label={f.required ? `${f.label} (zorunlu)` : `${f.label} (isteğe bağlı)`}
              value={config[f.key] ?? ''}
              onChange={(e) => setConfig((c) => ({ ...c, [f.key]: e.target.value }))}
              placeholder={f.placeholder}
              autoCapitalize="none" autoCorrect="off" spellCheck={false}
              error={errors[`config.${f.key}`]}
            />
          ))}
        </div>

        <label className="flex flex-col gap-1.5">
          <span className="text-sm font-medium">Kullanıcıya gösterilecek açıklama</span>
          <textarea
            className={textareaClass} rows={4}
            value={instructions} onChange={(e) => setInstructions(e.target.value)}
            placeholder="Örn. Açıklama alanına kullanıcı adınızı yazınız. Havale hafta içi 1 saat içinde onaylanır."
          />
          <span className="text-xs text-muted">Yükleme ekranında bu yöntemin altında görünür.</span>
        </label>

        <Field
          label="En az tutar (TL)"
          value={minAmount} onChange={(e) => setMinAmount(e.target.value)}
          placeholder="Örn. 100,00"
          inputMode="decimal" autoComplete="off"
          error={errors.minAmount}
          hint="Sistem alt sınırı 10,00 ₺."
        />

        <Field
          label="En çok tutar (TL)"
          value={maxAmount} onChange={(e) => setMaxAmount(e.target.value)}
          placeholder="Boş bırakın: üst sınır yok"
          inputMode="decimal" autoComplete="off"
          error={errors.maxAmount}
          hint="Boş bırakılırsa üst sınır uygulanmaz. Sistem üst sınırı 50.000,00 ₺."
        />

        <Field
          label="Sıra"
          value={sortOrder} onChange={(e) => setSortOrder(e.target.value)}
          inputMode="numeric" autoComplete="off"
          error={errors.sortOrder}
          hint="Küçük sayı önce gösterilir."
        />

        {err && <ErrorBox err={err} />}

        <div className="flex flex-col gap-2">
          <Button type="submit" fullWidth loading={save.isPending}>
            {isEdit ? 'Değişiklikleri kaydet' : 'Yöntemi ekle'}
          </Button>
          <Button type="button" variant="ghost" fullWidth disabled={save.isPending} onClick={onClose}>
            Vazgeç
          </Button>
        </div>
      </form>
    </Modal>
  );
}

/* ═══════════════════════ Silme onayı ═══════════════════════ */

function DeleteDialog({ method, onClose }: { method: DepositMethod; onClose: () => void }) {
  const qc = useQueryClient();
  const bodyRef = React.useRef<HTMLDivElement>(null);
  useDialogFocus(bodyRef);

  const del = useMutation({
    // 🔴 DELETE 204 döner: apiFetch `undefined` verir, dönüş değeri OKUNMAZ.
    mutationFn: () =>
      apiFetch<void>(`/admin/deposit-methods/${encodeURIComponent(method.id)}`, { method: 'DELETE' }),
    retry: false,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['admin-deposit-methods'] });
      onClose();
    },
  });

  const err = del.error instanceof ApiError ? del.error : null;

  return (
    <Modal open onClose={onClose} title="Yöntemi sil">
      <div ref={bodyRef} className="flex flex-col gap-4">
        <Alert tone="warn">
          <p><strong>{method.name}</strong> kalıcı olarak silinecek.</p>
          <p className="mt-2">Bu işlem geri alınamaz.</p>
        </Alert>

        {method.isActive && (
          <Alert>
            Bu yöntem şu anda <strong>aktif</strong>. Silindiği anda kullanıcılar
            bu yolla yükleme yapamaz. Geçici olarak durdurmak istiyorsanız silmek
            yerine <strong>pasifleştirin</strong>.
          </Alert>
        )}

        <p className="text-sm text-muted">
          Geçmiş yükleme talepleri etkilenmez: her talep, oluşturulduğu andaki
          yöntem adını kendi içinde saklar. Kullanıcı bir yıl sonra baktığında
          hangi yolla yatırdığını görmeye devam eder.
        </p>

        {err && <ErrorBox err={err} />}

        <div className="flex flex-col gap-2">
          {/* Odak "Vazgeç"tedir: silme, bilinçli ve ayrı bir hareket olmalı. */}
          <Button variant="danger" fullWidth loading={del.isPending}
                  onClick={() => del.mutate()}>
            Evet, sil
          </Button>
          <Button data-autofocus variant="ghost" fullWidth disabled={del.isPending}
                  onClick={onClose}>
            Vazgeç
          </Button>
        </div>
      </div>
    </Modal>
  );
}
