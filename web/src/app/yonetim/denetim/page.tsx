'use client';

import * as React from 'react';
import { keepPreviousData, useQuery } from '@tanstack/react-query';
import { ApiError, apiFetch } from '@/lib/api';
import { formatDateTime } from '@/lib/format';
import { Alert, Badge, Button, Card, Empty, Field, Skeleton, cx } from '@/components/ui';
import type { AuditLog } from '@/lib/types';

const PAGE = 25;

/**
 * Bilinen eylem ve varlık adları.
 *
 * Bunlar bir SEÇENEK LİSTESİ değil, ÖNERİ listesidir (`<datalist>`): sunucuya
 * yeni bir denetim eylemi eklendiğinde bu dosya güncellenmemiş olsa bile
 * yönetici adı elle yazıp süzebilir. Kapalı bir `<select>` olsaydı, yeni eylem
 * panelde süzülemez ve pratikte görünmez olurdu.
 */
const KNOWN_ACTIONS = [
  'deposit.approve',
  'deposit.reject',
  'pricing.rule.create',
  'pricing.rule.deactivate',
];
const KNOWN_ENTITY_TYPES = ['deposit', 'pricing_rule'];

const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

/**
 * `<input type="date">` değerini (YYYY-MM-DD) RFC 3339'a çevirir.
 *
 * Saat dilimi AÇIKÇA +03:00 yazılır, cihazınkiyle DEĞİL: sunucu yalnız RFC 3339
 * kabul eder (Değişmez #19) ve panelin her yerinde tarihler Europe/Istanbul
 * ile gösterilir. Cihazın dilimi kullanılsaydı yurt dışındaki bir yönetici
 * gördüğü günden farklı bir aralığı süzerdi. Türkiye 2016'dan beri kalıcı
 * olarak UTC+03'tür; yaz saati uygulaması yoktur.
 */
function dayStart(v: string): string | null {
  return /^\d{4}-\d{2}-\d{2}$/.test(v) ? `${v}T00:00:00+03:00` : null;
}
function dayEnd(v: string): string | null {
  return /^\d{4}-\d{2}-\d{2}$/.test(v) ? `${v}T23:59:59+03:00` : null;
}

/** JSON değerini tek satırda okunur yazar; nesne/dizi ise JSON olarak. */
function renderValue(v: unknown): string {
  if (v === null || v === undefined) return '—';
  if (typeof v === 'string') return v === '' ? '(boş)' : v;
  if (typeof v === 'number' || typeof v === 'boolean') return String(v);
  try {
    return JSON.stringify(v);
  } catch {
    return '(gösterilemedi)';
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

/** Öncesi/sonrası gövdesi — anahtar-değer listesi. */
function PayloadList({ title, data }: { title: string; data?: Record<string, unknown> }) {
  const keys = data ? Object.keys(data) : [];
  return (
    <div className="min-w-0 flex-1 rounded-xl border border-[var(--border)] p-3">
      <p className="text-xs font-medium uppercase tracking-wide text-muted">{title}</p>
      {keys.length === 0 ? (
        <p className="mt-2 text-sm text-muted">—</p>
      ) : (
        <dl className="mt-2 flex flex-col gap-1">
          {keys.map((k) => (
            <div key={k} className="flex flex-wrap justify-between gap-x-3 text-sm">
              <dt className="text-muted">{k}</dt>
              <dd className="min-w-0 break-anywhere text-right font-medium">
                {renderValue(data?.[k])}
              </dd>
            </div>
          ))}
        </dl>
      )}
    </div>
  );
}

/** Bir denetim kaydının ayrıntısı: kim, ne, öncesi/sonrası, gizlenenler. */
function LogDetail({ log }: { log: AuditLog }) {
  return (
    <>
      <div className="mt-3 flex flex-col gap-3 md:flex-row">
        <PayloadList title="Öncesi" data={log.before} />
        <PayloadList title="Sonrası" data={log.after} />
      </div>

      {/* Sessizce atmak denetimi yanıltır: denetçi bir şeyin saklandığını GÖRMELİ. */}
      {log.redactedFields?.length ? (
        <Alert tone="warn" className="mt-3">
          Şu alanlar gizlendi: <strong className="break-anywhere">
            {log.redactedFields.join(', ')}
          </strong>
          <span className="mt-1 block text-xs opacity-80">
            Denetim kaydı yalnız izin listesindeki alanları dışarı verir; kalanların
            adı burada bildirilir.
          </span>
        </Alert>
      ) : null}

      <div className="mt-3 flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted">
        <span className="break-anywhere">Varlık: {log.entityType} · {log.entityId}</span>
        {log.ip && <span>IP: {log.ip}</span>}
        {log.requestId && <span className="break-anywhere">İstek no: {log.requestId}</span>}
      </div>
    </>
  );
}

export default function AdminAuditPage() {
  /* ── Süzgeç (form) durumu ── */
  const [action, setAction] = React.useState('');
  const [entityType, setEntityType] = React.useState('');
  const [actorId, setActorId] = React.useState('');
  const [from, setFrom] = React.useState('');
  const [until, setUntil] = React.useState('');
  const [errors, setErrors] = React.useState<Record<string, string>>({});

  /* ── Uygulanan süzgeç ──
   * Form durumundan AYRIDIR: her tuş vuruşunda sorgu atmak, dakikada 60
   * istekle sınırlı yönetim uçlarında 429 üretir. Süzgeç "Uygula" ile geçer. */
  const [applied, setApplied] = React.useState<Record<string, string>>({});
  const [offset, setOffset] = React.useState(0);

  /*
   * Açık olan kaydın kimliği.
   *
   * Tip `AuditLog['id']`den TÜRETİLİR, elle yazılmaz: sunucu bu alanı
   * `int64` olarak döner (dto/admin.go `ID int64`), lib/types.ts ise bugün
   * `string` diyor. İkisinden hangisi düzeltilirse düzeltilsin bu ekran
   * derlenmeye devam eder — burada yalnız kimliği KARŞILAŞTIRIRIZ.
   */
  const [openId, setOpenId] = React.useState<AuditLog['id'] | null>(null);

  const query = React.useMemo(() => {
    const p = new URLSearchParams(applied);
    p.set('limit', String(PAGE));
    p.set('offset', String(offset));
    return p.toString();
  }, [applied, offset]);

  const q = useQuery({
    queryKey: ['admin', 'audit-logs', query],
    queryFn: () => apiFetch<{ items: AuditLog[]; total: number; limit: number; offset: number }>(
      `/admin/audit-logs?${query}`,
    ),
    // Sayfa değişince liste boşalıp zıplamasın.
    placeholderData: keepPreviousData,
  });

  function apply(e: React.FormEvent) {
    e.preventDefault();
    const errs: Record<string, string> = {};
    const next: Record<string, string> = {};

    if (action.trim()) next.action = action.trim();
    if (entityType.trim()) next.entityType = entityType.trim();

    const actor = actorId.trim();
    if (actor) {
      if (!UUID_RE.test(actor)) errs.actorId = 'Aktör kimliği bir UUID olmalıdır.';
      else next.actorId = actor;
    }

    if (from) {
      const v = dayStart(from);
      if (!v) errs.from = 'Geçerli bir tarih seçiniz.';
      else next.from = v;
    }
    if (until) {
      const v = dayEnd(until);
      if (!v) errs.until = 'Geçerli bir tarih seçiniz.';
      else next.until = v;
    }
    if (from && until && from > until) {
      errs.until = 'Bitiş tarihi başlangıçtan önce olamaz.';
    }

    setErrors(errs);
    if (Object.keys(errs).length) return;
    setOffset(0);
    setApplied(next);
  }

  function clearFilters() {
    setAction(''); setEntityType(''); setActorId(''); setFrom(''); setUntil('');
    setErrors({}); setOffset(0); setApplied({});
  }

  const total = q.data?.total ?? 0;
  const hasPrev = offset > 0;
  const hasNext = offset + PAGE < total;
  const err = q.error instanceof ApiError ? q.error : null;
  const filterCount = Object.keys(applied).length;

  return (
    <div className="mx-auto flex max-w-5xl flex-col gap-5">
      <div>
        <h1 className="text-2xl font-bold tracking-tight md:text-3xl">Denetim kaydı</h1>
        <p className="mt-1 text-sm text-muted">
          Para ve fiyat etkileyen her yönetim işlemi burada, değiştirilemez biçimde durur.
        </p>
      </div>

      {/* ═══════════ Süzgeçler ═══════════ */}
      <Card>
        <form onSubmit={apply} className="flex flex-col gap-4" noValidate>
          <div className="grid gap-4 md:grid-cols-2">
            <div className="flex flex-col gap-1.5">
              <Field
                label="Eylem"
                list="denetim-eylemler"
                value={action}
                onChange={(e) => setAction(e.target.value)}
                placeholder="Örn. deposit.approve"
                autoCapitalize="none" autoCorrect="off" spellCheck={false}
                hint="Boş bırakılırsa tüm eylemler."
              />
              <datalist id="denetim-eylemler">
                {KNOWN_ACTIONS.map((a) => <option key={a} value={a} />)}
              </datalist>
            </div>

            <div className="flex flex-col gap-1.5">
              <Field
                label="Varlık tipi"
                list="denetim-varliklar"
                value={entityType}
                onChange={(e) => setEntityType(e.target.value)}
                placeholder="Örn. deposit"
                autoCapitalize="none" autoCorrect="off" spellCheck={false}
                hint="Boş bırakılırsa tüm varlıklar."
              />
              <datalist id="denetim-varliklar">
                {KNOWN_ENTITY_TYPES.map((t) => <option key={t} value={t} />)}
              </datalist>
            </div>
          </div>

          <Field
            label="Aktör (kullanıcı kimliği)"
            value={actorId}
            onChange={(e) => setActorId(e.target.value)}
            placeholder="00000000-0000-0000-0000-000000000000"
            autoCapitalize="none" autoCorrect="off" spellCheck={false}
            error={errors.actorId}
            hint="İşlemi yapan yöneticinin genel kimliği (UUID)."
          />

          <div className="grid gap-4 md:grid-cols-2">
            <Field
              label="Başlangıç tarihi" type="date"
              value={from} onChange={(e) => setFrom(e.target.value)}
              error={errors.from}
              hint="Gün başından itibaren (Türkiye saati)."
            />
            <Field
              label="Bitiş tarihi" type="date"
              value={until} onChange={(e) => setUntil(e.target.value)}
              error={errors.until}
              hint="Gün sonuna kadar (Türkiye saati)."
            />
          </div>

          <div className="flex flex-col gap-2 sm:flex-row">
            <Button type="submit" fullWidth>Süzgeci uygula</Button>
            <Button type="button" variant="outline" fullWidth onClick={clearFilters}
                    disabled={filterCount === 0}>
              Temizle
            </Button>
          </div>
        </form>
      </Card>

      {/* ═══════════ Kayıtlar ═══════════ */}
      <Card>
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h2 className="text-lg font-semibold">Kayıtlar</h2>
          <div className="flex flex-wrap items-center gap-2">
            {filterCount > 0 && <Badge tone="brand">{filterCount} süzgeç etkin</Badge>}
            {total > 0 && <Badge tone="neutral">{total} kayıt</Badge>}
          </div>
        </div>

        {q.isLoading ? (
          <div className="mt-4 flex flex-col gap-2">
            {[0, 1, 2, 3].map((i) => <Skeleton key={i} className="h-20" />)}
          </div>
        ) : err ? (
          <ErrorBox err={err} className="mt-4" />
        ) : q.isError ? (
          <Alert className="mt-4">
            Denetim kaydı yüklenemedi. Bağlantınızı kontrol edip tekrar deneyin.
          </Alert>
        ) : !q.data?.items.length ? (
          <Empty
            title="Kayıt bulunamadı"
            hint={filterCount > 0
              ? 'Süzgeçleri gevşetip tekrar deneyin.'
              : 'Yönetim işlemleri yapıldıkça burada görünecek.'}
          />
        ) : (
          <>
            {/* MOBİL: kart listesi. Yatay kaydırılan tablo kabul edilmez. */}
            <ul className="mt-4 flex flex-col gap-2 md:hidden">
              {q.data.items.map((log) => (
                <li key={log.id} className="raised rounded-xl border p-3">
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                      <p className="break-anywhere text-sm font-medium">{log.action}</p>
                      <p className="mt-0.5 text-xs text-muted">{formatDateTime(log.createdAt)}</p>
                    </div>
                    <span className="shrink-0">
                      <Badge tone="neutral">{log.entityType}</Badge>
                    </span>
                  </div>
                  <p className="mt-2 break-anywhere text-xs text-muted">
                    Aktör: {log.actorUsername || '(sistem)'}
                  </p>
                  {log.redactedFields?.length ? (
                    <p className="mt-1 text-xs text-[var(--color-warn)]">
                      {log.redactedFields.length} alan gizlendi
                    </p>
                  ) : null}

                  <button
                    type="button"
                    onClick={() => setOpenId(openId === log.id ? null : log.id)}
                    aria-expanded={openId === log.id}
                    className="mt-2 flex min-h-11 w-full items-center justify-between gap-2
                               rounded-xl border border-[var(--border)] px-3 text-sm"
                  >
                    <span>{openId === log.id ? 'Ayrıntıyı gizle' : 'Öncesi / sonrası'}</span>
                    <span aria-hidden className={cx('transition-transform',
                                                    openId === log.id && 'rotate-180')}>▾</span>
                  </button>

                  {openId === log.id && <LogDetail log={log} />}
                </li>
              ))}
            </ul>

            {/* MASAÜSTÜ: gerçek tablo */}
            <div className="mt-4 hidden md:block">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-[var(--border)] text-left text-xs
                                 uppercase tracking-wide text-muted">
                    <th scope="col" className="py-2 pr-3 font-medium">Zaman</th>
                    <th scope="col" className="py-2 pr-3 font-medium">Aktör</th>
                    <th scope="col" className="py-2 pr-3 font-medium">Eylem</th>
                    <th scope="col" className="py-2 pr-3 font-medium">Varlık</th>
                    <th scope="col" className="py-2 text-right font-medium">Ayrıntı</th>
                  </tr>
                </thead>
                <tbody>
                  {q.data.items.map((log) => (
                    <React.Fragment key={log.id}>
                      <tr className="border-b border-[var(--border)] last:border-0">
                        <td className="py-3 pr-3 whitespace-nowrap text-muted">
                          {formatDateTime(log.createdAt)}
                        </td>
                        <td className="py-3 pr-3 break-anywhere">{log.actorUsername || '(sistem)'}</td>
                        <td className="py-3 pr-3 break-anywhere font-medium">{log.action}</td>
                        <td className="py-3 pr-3 break-anywhere text-muted">
                          {log.entityType}
                          <span className="block text-xs opacity-70">{log.entityId}</span>
                        </td>
                        <td className="py-3 text-right">
                          <Button
                            variant="outline" size="sm"
                            aria-expanded={openId === log.id}
                            onClick={() => setOpenId(openId === log.id ? null : log.id)}
                          >
                            {openId === log.id ? 'Gizle' : 'Göster'}
                          </Button>
                        </td>
                      </tr>
                      {openId === log.id && (
                        <tr className="border-b border-[var(--border)] last:border-0">
                          <td colSpan={5} className="pb-4">
                            <LogDetail log={log} />
                          </td>
                        </tr>
                      )}
                    </React.Fragment>
                  ))}
                </tbody>
              </table>
            </div>

            {(hasPrev || hasNext) && (
              <div className="mt-4 flex items-center justify-between gap-3">
                <Button variant="outline" size="sm" disabled={!hasPrev}
                        onClick={() => { setOpenId(null); setOffset((o) => Math.max(0, o - PAGE)); }}>
                  Önceki
                </Button>
                <span className="text-xs text-muted">
                  {offset + 1}–{Math.min(offset + PAGE, total)} / {total}
                </span>
                <Button variant="outline" size="sm" disabled={!hasNext}
                        onClick={() => { setOpenId(null); setOffset((o) => o + PAGE); }}>
                  Sonraki
                </Button>
              </div>
            )}
          </>
        )}
      </Card>
    </div>
  );
}
