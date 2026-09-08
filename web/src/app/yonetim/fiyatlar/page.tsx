'use client';

import * as React from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ApiError, apiFetch } from '@/lib/api';
import { formatDateTime, formatMoney } from '@/lib/format';
import { Alert, Badge, Button, Card, Empty, Field, Skeleton, cx } from '@/components/ui';
import { Modal } from '@/components/modal';
import type { Country, PricingPreview, PricingRule, PricingScope, Service } from '@/lib/types';

/* ═════════════════════════ Sabitler ve yardımcılar ═════════════════════════ */

const rulesKey = ['admin', 'pricing-rules'] as const;

/**
 * Kapsam tanımı.
 *
 * `service`/`country`/`duration` alanları sunucudaki `resolve()` ile BİREBİR
 * aynıdır (api/internal/service/pricing/rules.go): kapsamın gerektirmediği bir
 * alanı DOLU göndermek de hatadır ("GLOBAL kapsamında servis seçilemez"), boş
 * bırakmak da. Bu yüzden kapsam değişince ilgisiz alanlar TEMİZLENİR.
 */
const SCOPES: Array<{
  value: PricingScope;
  label: string;
  service: boolean;
  country: boolean;
  duration: boolean;
  hint: string;
}> = [
  { value: 'GLOBAL', label: 'Tüm ürünler (GLOBAL)', service: false, country: false, duration: false,
    hint: 'Başka hiçbir kural eşleşmezse bu kural uygulanır. Sistemde her zaman bir tane bulunmalıdır.' },
  { value: 'COUNTRY', label: 'Ülke', service: false, country: true, duration: false,
    hint: 'Seçilen ülkenin tüm servisleri.' },
  { value: 'SERVICE', label: 'Servis', service: true, country: false, duration: false,
    hint: 'Seçilen servisin tüm ülkeleri.' },
  { value: 'SERVICE_COUNTRY', label: 'Servis + Ülke', service: true, country: true, duration: false,
    hint: 'Yalnız bu servis ve ülke ikilisi. GLOBAL ve tekil kuralları ezer.' },
  { value: 'PRODUCT', label: 'Ürün (servis + ülke + süre)', service: true, country: true, duration: true,
    hint: 'Kiralık ürünler için: belirli bir süreye özel kural. En dar kapsam, hepsini ezer.' },
];

function scopeDef(s: PricingScope) {
  return SCOPES.find((x) => x.value === s) ?? SCOPES[0]!;
}

/** Kapsam hedefini okunur yazar: "wa × TR". Kayıtlı kural da form durumu da geçer. */
function scopeText(r: { serviceCode?: string; countryIso?: string; durationMinutes?: number }): string {
  const parts: string[] = [];
  if (r.serviceCode) parts.push(r.serviceCode);
  if (r.countryIso) parts.push(r.countryIso);
  if (r.durationMinutes) parts.push(`${r.durationMinutes} dk`);
  return parts.length ? parts.join(' × ') : 'tüm ürünler';
}

/**
 * TL girişi → kuruş.
 *
 * `12.50 * 100` JavaScript'te 1249.9999… verir ve bir kuruş kaybolur; çevrim
 * metin üzerinden, tam sayı aritmetiğiyle yapılır (yonetim/bakiye ekranındaki
 * `toMinor` ile aynı kalıp — ortak bir dosyaya taşınmadı, çünkü lib/format
 * biçimleme dosyasıdır, ayrıştırma değil).
 */
function toMinor(input: string): { minor: number } | { error: string } {
  const s = input.trim().replace(/\s/g, '').replace(',', '.');
  if (s === '') return { minor: 0 };
  if (!/^\d+(\.\d{1,2})?$/.test(s)) {
    return { error: 'Geçerli bir tutar giriniz (en fazla 2 ondalık, negatif olamaz).' };
  }
  const [whole = '0', frac = ''] = s.split('.');
  const minor = Number(whole) * 100 + Number(frac.padEnd(2, '0'));
  if (!Number.isSafeInteger(minor)) return { error: 'Tutar çok büyük.' };
  // Sunucudaki sınır (maxRuleAmountMinor): asıl risk büyük değer değil,
  // FAZLADAN İKİ SIFIR — 5,00 ₺ yerine 500,00 ₺ taban her ürünü satılamaz yapar.
  if (minor > 100_000_000) return { error: 'Tutar en fazla 1.000.000,00 ₺ olabilir.' };
  return { minor };
}

/** Marj metni sunucudaki `marginPattern` ile aynı kurala uyar: 0–1000, en çok 2 ondalık. */
function normalizeMargin(input: string): { value: string } | { error: string } {
  const s = input.trim().replace(/\s/g, '').replace(',', '.');
  if (s === '') return { error: 'Marj yüzdesi zorunludur.' };
  if (!/^\d{1,4}(\.\d{1,2})?$/.test(s)) {
    return { error: 'Marj bir sayı olmalıdır (örn. 40 veya 40,50).' };
  }
  if (Number(s) > 1000) return { error: 'Marj yüzdesi en fazla 1000 olabilir.' };
  return { value: s };
}

const selectClass =
  'raised min-h-12 w-full rounded-xl border px-3 text-base outline-none ' +
  'focus:border-brand-400 disabled:opacity-60';

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

/** Değeri gecikmeli yankılar — her tuş vuruşunda önizleme isteği atılmasın. */
function useDebounced<T>(value: T, ms: number): T {
  const [out, setOut] = React.useState(value);
  React.useEffect(() => {
    const t = window.setTimeout(() => setOut(value), ms);
    return () => window.clearTimeout(t);
  }, [value, ms]);
  return out;
}

/* ═════════════════════════════════ Ekran ═════════════════════════════════ */

export default function AdminPricingPage() {
  const qc = useQueryClient();

  const rules = useQuery({
    queryKey: rulesKey,
    queryFn: () => apiFetch<{ items: PricingRule[] }>('/admin/pricing-rules'),
  });

  const services = useQuery({
    queryKey: ['catalog', 'services'],
    queryFn: () => apiFetch<{ items: Service[] }>('/catalog/services'),
    staleTime: 5 * 60_000,
  });

  const countries = useQuery({
    queryKey: ['catalog', 'countries'],
    queryFn: () => apiFetch<{ items: Country[] }>('/catalog/countries'),
    staleTime: 5 * 60_000,
  });

  /* ── Form durumu ── */
  const [scope, setScope] = React.useState<PricingScope>('GLOBAL');
  const [serviceCode, setServiceCode] = React.useState('');
  const [countryIso, setCountryIso] = React.useState('');
  const [duration, setDuration] = React.useState('');
  const [margin, setMargin] = React.useState('');
  const [fixedFee, setFixedFee] = React.useState('');
  const [minPrice, setMinPrice] = React.useState('');
  const [note, setNote] = React.useState('');
  const [errors, setErrors] = React.useState<Record<string, string>>({});
  const [confirmOpen, setConfirmOpen] = React.useState(false);

  const def = scopeDef(scope);

  function changeScope(next: PricingScope) {
    const d = scopeDef(next);
    setScope(next);
    // Kapsamın kullanmadığı alan DOLU kalırsa sunucu "bu kapsamda servis
    // seçilemez" der; kullanıcı formda görünmeyen bir alan yüzünden hata alır.
    if (!d.service) setServiceCode('');
    if (!d.country) setCountryIso('');
    if (!d.duration) setDuration('');
    setErrors({});
  }

  /* ── Önizleme hedefi ──
   * Önizleme HER ZAMAN somut bir servis+ülke ister (sunucu doğrulaması).
   * Kapsam bunları vermiyorsa (GLOBAL, COUNTRY, SERVICE) yönetici kuralı
   * hangi ürün üzerinde denemek istediğini ayrıca seçer. */
  const [testService, setTestService] = React.useState('');
  const [testCountry, setTestCountry] = React.useState('');
  const previewService = def.service ? serviceCode : testService;
  const previewCountry = def.country ? countryIso : testCountry;

  const marginParsed = normalizeMargin(margin);
  const feeParsed = toMinor(fixedFee);
  const minParsed = toMinor(minPrice);

  // Memo bağımlılıkları İLKEL DEĞER olmalı: nesne verilseydi her render'da yeni
  // referans üretilir, önizleme sorgusunun anahtarı sürekli değişir ve
  // gecikmeye rağmen her tuş vuruşunda istek atılırdı.
  const marginValue = 'value' in marginParsed ? marginParsed.value : null;
  const feeMinor = 'minor' in feeParsed ? feeParsed.minor : null;
  const minMinor = 'minor' in minParsed ? minParsed.minor : null;
  const durationMinutes = def.duration ? Number(duration || 0) : 0;

  const candidate =
    marginValue !== null && feeMinor !== null && minMinor !== null
      ? { marginPercent: marginValue, fixedFeeMinor: feeMinor, minPriceMinor: minMinor }
      : null;

  const previewBody = React.useMemo(
    () => ({
      serviceCode: previewService,
      countryIso: previewCountry,
      durationMinutes,
      candidate:
        marginValue !== null && feeMinor !== null && minMinor !== null
          ? { marginPercent: marginValue, fixedFeeMinor: feeMinor, minPriceMinor: minMinor }
          : null,
    }),
    [previewService, previewCountry, durationMinutes, marginValue, feeMinor, minMinor],
  );

  // Yazarken her tuşta istek atmayız: yönetim uçları dakikada 60 istekle sınırlı.
  const debouncedBody = useDebounced(previewBody, 500);
  const previewReady = !!debouncedBody.serviceCode && !!debouncedBody.countryIso;

  const preview = useQuery({
    queryKey: ['admin', 'pricing-preview', debouncedBody],
    // POST, ama DURUM DEĞİŞTİRMEZ (izin: pricing:read) — bu yüzden bir
    // mutasyon değil, sorgudur; gövdesi aday kural taşıdığı için POST'tur.
    queryFn: () => apiFetch<PricingPreview>('/admin/pricing-rules/preview', {
      method: 'POST', body: debouncedBody,
    }),
    enabled: previewReady,
    staleTime: 15_000,
  });

  /* ── Kural yazma ── */
  const create = useMutation({
    mutationFn: (v: {
      scope: PricingScope; serviceCode: string; countryIso: string; durationMinutes: number;
      marginPercent: string; fixedFeeMinor: number; minPriceMinor: number; note: string;
    }) => apiFetch<PricingRule>('/admin/pricing-rules', { method: 'POST', body: v }),
    // Fiyat kuralı yazmak İDEMPOTENT DEĞİLDİR: her çağrı yeni bir kural
    // oluşturur ve öncekini devreden çıkarır. Otomatik tekrar, geçmişte
    // birbirinin kopyası iki kural bırakır.
    retry: false,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: rulesKey });
      setConfirmOpen(false);
      setNote('');
    },
  });

  const [toDeactivate, setToDeactivate] = React.useState<PricingRule | null>(null);
  const deactivate = useMutation({
    mutationFn: (id: number) => apiFetch<void>(`/admin/pricing-rules/${id}`, { method: 'DELETE' }),
    retry: false,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: rulesKey });
      setToDeactivate(null);
    },
  });

  function validate(): boolean {
    const errs: Record<string, string> = {};
    if (def.service && !serviceCode) errs.serviceCode = 'Bu kapsam için servis seçilmelidir.';
    if (def.country && !countryIso) errs.countryIso = 'Bu kapsam için ülke seçilmelidir.';
    if (def.duration) {
      if (!/^\d{1,6}$/.test(duration.trim())) errs.durationMinutes = 'Süreyi dakika olarak giriniz.';
      else if (Number(duration) <= 0) errs.durationMinutes = 'Süre sıfırdan büyük olmalıdır.';
    }
    if ('error' in marginParsed) errs.marginPercent = marginParsed.error;
    if ('error' in feeParsed) errs.fixedFeeMinor = feeParsed.error;
    if ('error' in minParsed) errs.minPriceMinor = minParsed.error;
    if (note.trim().length > 500) errs.note = 'Not en fazla 500 karakter olabilir.';
    setErrors(errs);
    return Object.keys(errs).length === 0;
  }

  function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    create.reset();
    if (validate()) setConfirmOpen(true);
  }

  function doCreate() {
    if (!candidate) return;
    create.mutate({
      scope,
      serviceCode: def.service ? serviceCode : '',
      countryIso: def.country ? countryIso : '',
      durationMinutes: def.duration ? Number(duration) : 0,
      marginPercent: candidate.marginPercent,
      fixedFeeMinor: candidate.fixedFeeMinor,
      minPriceMinor: candidate.minPriceMinor,
      note: note.trim(),
    });
  }

  /** Bu kapsamda şu an etkin olan kural — "Kaydet" onun yerine geçecek. */
  const willReplace = (rules.data?.items ?? []).find(
    (r) =>
      r.scope === scope &&
      (r.serviceCode ?? '') === (def.service ? serviceCode : '') &&
      (r.countryIso ?? '') === (def.country ? countryIso : '') &&
      (r.durationMinutes ?? 0) === (def.duration ? Number(duration || 0) : 0),
  );

  const globalCount = (rules.data?.items ?? []).filter((r) => r.scope === 'GLOBAL').length;

  const createErr = create.error instanceof ApiError ? create.error : null;
  const deactivateErr = deactivate.error instanceof ApiError ? deactivate.error : null;
  const listErr = rules.error instanceof ApiError ? rules.error : null;
  const previewErr = preview.error instanceof ApiError ? preview.error : null;

  const serviceOptions = services.data?.items ?? [];
  const countryOptions = countries.data?.items ?? [];

  return (
    <div className="mx-auto flex max-w-5xl flex-col gap-5">
      <div>
        <h1 className="text-2xl font-bold tracking-tight md:text-3xl">Fiyat kuralları</h1>
        <p className="mt-1 text-sm text-muted">
          Satış fiyatı = sağlayıcı maliyeti × (1 + marj) + sabit bedel; sonuç taban
          fiyatın altına düşemez.
        </p>
      </div>

      {/* Bu ekranın EN ÖNEMLİ cümlesi: "Kaydet" bir güncelleme değildir. */}
      <Alert tone="warn">
        <strong>Kural güncelleme diye bir işlem yoktur.</strong> Bir kapsama yeni kural
        yazdığınızda o kapsamdaki eski kural aynı anda devreden çıkar ve geçmişte kalır —
        eski siparişlerin hangi kuralla fiyatlandığı izlenebilir kalsın diye silinmez.
        Yani &laquo;Kaydet&raquo; bir düzenleme gibi görünür, aslında yeni bir kayıt yazar.
      </Alert>

      {/* ═══════════ Kural formu ═══════════ */}
      <Card>
        <h2 className="text-lg font-semibold">Yeni kural</h2>

        <form onSubmit={onSubmit} className="mt-4 flex flex-col gap-4" noValidate>
          <label className="flex flex-col gap-1.5">
            <span className="text-sm font-medium">Kapsam</span>
            <select
              value={scope}
              onChange={(e) => changeScope(e.target.value as PricingScope)}
              className={selectClass}
            >
              {SCOPES.map((s) => <option key={s.value} value={s.value}>{s.label}</option>)}
            </select>
            <span className="text-xs text-muted">{def.hint}</span>
          </label>

          {def.service && (
            <label className="flex flex-col gap-1.5">
              <span className="text-sm font-medium">Servis</span>
              <select
                value={serviceCode}
                onChange={(e) => setServiceCode(e.target.value)}
                disabled={services.isLoading || services.isError}
                aria-invalid={errors.serviceCode ? true : undefined}
                className={cx(selectClass, errors.serviceCode && 'border-[var(--color-bad)]')}
              >
                <option value="">
                  {services.isLoading ? 'Servisler yükleniyor…'
                    : services.isError ? 'Servisler yüklenemedi'
                    : 'Servis seçiniz…'}
                </option>
                {serviceOptions.map((s) => (
                  <option key={s.code} value={s.code}>{s.name} ({s.code})</option>
                ))}
              </select>
              {errors.serviceCode && (
                <span role="alert" className="text-xs text-[var(--color-bad)]">{errors.serviceCode}</span>
              )}
            </label>
          )}

          {def.country && (
            <label className="flex flex-col gap-1.5">
              <span className="text-sm font-medium">Ülke</span>
              <select
                value={countryIso}
                onChange={(e) => setCountryIso(e.target.value)}
                disabled={countries.isLoading || countries.isError}
                aria-invalid={errors.countryIso ? true : undefined}
                className={cx(selectClass, errors.countryIso && 'border-[var(--color-bad)]')}
              >
                <option value="">
                  {countries.isLoading ? 'Ülkeler yükleniyor…'
                    : countries.isError ? 'Ülkeler yüklenemedi'
                    : 'Ülke seçiniz…'}
                </option>
                {countryOptions.map((c) => (
                  <option key={c.iso2} value={c.iso2}>{c.name} ({c.iso2})</option>
                ))}
              </select>
              {errors.countryIso && (
                <span role="alert" className="text-xs text-[var(--color-bad)]">{errors.countryIso}</span>
              )}
            </label>
          )}

          {def.duration && (
            <Field
              label="Süre (dakika)"
              value={duration}
              onChange={(e) => setDuration(e.target.value)}
              inputMode="numeric" autoComplete="off"
              placeholder="Örn. 60"
              error={errors.durationMinutes}
              hint="Kiralık ürünün süresi. Yalnız ürün kapsamında kullanılır."
            />
          )}

          <div className="grid gap-4 md:grid-cols-3">
            <Field
              label="Marj (%)"
              value={margin}
              onChange={(e) => setMargin(e.target.value)}
              inputMode="decimal" autoComplete="off"
              placeholder="Örn. 40"
              error={errors.marginPercent}
              hint="Maliyetin üstüne eklenecek yüzde."
            />
            <Field
              label="Sabit bedel (TL)"
              value={fixedFee}
              onChange={(e) => setFixedFee(e.target.value)}
              inputMode="decimal" autoComplete="off"
              placeholder="Örn. 2,50"
              error={errors.fixedFeeMinor}
              hint="Marjdan sonra eklenir. Boş = 0."
            />
            <Field
              label="Taban fiyat (TL)"
              value={minPrice}
              onChange={(e) => setMinPrice(e.target.value)}
              inputMode="decimal" autoComplete="off"
              placeholder="Örn. 5,00"
              error={errors.minPriceMinor}
              hint="Fiyat bunun altına düşmez. Boş = 0."
            />
          </div>

          <label className="flex flex-col gap-1.5">
            <span className="text-sm font-medium">Not</span>
            <textarea
              value={note}
              onChange={(e) => setNote(e.target.value)}
              rows={2}
              maxLength={500}
              placeholder="Bu kuralı neden yazdınız? Denetim kaydında görünür."
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

          <Button type="submit" fullWidth loading={create.isPending}>
            Kuralı kaydet
          </Button>
        </form>

        {createErr && <ErrorBox err={createErr} className="mt-4" />}

        {create.isSuccess && create.data && (
          <Alert tone="ok" className="mt-4">
            <p>
              Kural yazıldı: <strong>%{create.data.marginPercent}</strong> marj,{' '}
              {scopeDef(create.data.scope).label.toLowerCase()} kapsamı ({scopeText(create.data)}).
            </p>
            {create.data.replaced && (
              <p className="mt-1">
                Bu kapsamdaki önceki kural <strong>devreden çıkarıldı</strong> ve
                geçmişte kaldı.
              </p>
            )}
          </Alert>
        )}
      </Card>

      {/* ═══════════ Canlı önizleme ═══════════ */}
      <Card>
        <h2 className="text-lg font-semibold">Canlı önizleme</h2>
        <p className="mt-1 text-sm text-muted">
          Formdaki değerler kaydedilmeden önce somut bir ürün üzerinde denenir.
        </p>

        {(!def.service || !def.country) && (
          <div className="mt-4 grid gap-4 md:grid-cols-2">
            {!def.service && (
              <label className="flex flex-col gap-1.5">
                <span className="text-sm font-medium">Deneme servisi</span>
                <select
                  value={testService}
                  onChange={(e) => setTestService(e.target.value)}
                  disabled={services.isLoading || services.isError}
                  className={selectClass}
                >
                  <option value="">Servis seçiniz…</option>
                  {serviceOptions.map((s) => (
                    <option key={s.code} value={s.code}>{s.name} ({s.code})</option>
                  ))}
                </select>
              </label>
            )}
            {!def.country && (
              <label className="flex flex-col gap-1.5">
                <span className="text-sm font-medium">Deneme ülkesi</span>
                <select
                  value={testCountry}
                  onChange={(e) => setTestCountry(e.target.value)}
                  disabled={countries.isLoading || countries.isError}
                  className={selectClass}
                >
                  <option value="">Ülke seçiniz…</option>
                  {countryOptions.map((c) => (
                    <option key={c.iso2} value={c.iso2}>{c.name} ({c.iso2})</option>
                  ))}
                </select>
              </label>
            )}
          </div>
        )}

        {!previewReady ? (
          <Empty
            title="Önizleme için ürün seçin"
            hint="Fiyat her zaman somut bir servis ve ülke üzerinde hesaplanır."
          />
        ) : preview.isLoading ? (
          <div className="mt-4 flex flex-col gap-2">
            {[0, 1, 2].map((i) => <Skeleton key={i} className="h-12" />)}
          </div>
        ) : previewErr ? (
          <ErrorBox err={previewErr} className="mt-4" />
        ) : preview.data ? (
          <>
            <div className="mt-4 rounded-xl border border-[var(--border)] p-4">
              <p className="text-xs font-medium uppercase tracking-wide text-muted">Satış fiyatı</p>
              <p className="mt-1 text-3xl font-bold md:text-4xl">
                {formatMoney(preview.data.sellPrice)}
              </p>
              <div className="mt-3 flex flex-wrap gap-2">
                <Badge tone={preview.data.ruleSource === 'CANDIDATE' ? 'brand' : 'neutral'}>
                  {preview.data.ruleSource === 'CANDIDATE'
                    ? 'Formdaki kural (kaydedilmedi)'
                    : `Kayıtlı kural${preview.data.ruleScope ? ` · ${preview.data.ruleScope}` : ''}`}
                </Badge>
                <Badge tone="neutral">Marj %{preview.data.marginPercent}</Badge>
                {preview.data.hitMinimum && <Badge tone="warn">Taban fiyat uygulandı</Badge>}
                <Badge tone={preview.data.inStock ? 'ok' : 'bad'}>
                  {preview.data.inStock ? `Stok: ${preview.data.stock}` : 'Stok yok'}
                </Badge>
              </div>
            </div>

            {/* Yöneticiye ara değerler: bunlar kullanıcı teklifinde ASLA görünmez. */}
            <dl className="mt-4 flex flex-col gap-2 text-sm">
              <div className="flex justify-between gap-3 border-b border-[var(--border)] pb-2">
                <dt className="text-muted">Sağlayıcı</dt>
                <dd className="min-w-0 truncate font-medium">{preview.data.providerName || '—'}</dd>
              </div>
              <div className="flex justify-between gap-3 border-b border-[var(--border)] pb-2">
                <dt className="text-muted">Maliyet</dt>
                <dd className="font-medium">{formatMoney(preview.data.cost)}</dd>
              </div>
              <div className="flex justify-between gap-3 border-b border-[var(--border)] pb-2">
                <dt className="text-muted">Maliyet (TL)</dt>
                <dd className="font-medium">{formatMoney(preview.data.costInTry)}</dd>
              </div>
              <div className="flex justify-between gap-3">
                <dt className="text-muted">Kur</dt>
                <dd className="break-anywhere text-right font-medium">
                  {preview.data.fxRate || '—'}
                  {preview.data.fxFetchedAt && (
                    <span className="block text-xs font-normal text-muted">
                      {formatDateTime(preview.data.fxFetchedAt)}
                    </span>
                  )}
                </dd>
              </div>
            </dl>

            {/* Sessiz kalmak yöneticiyi yanıltır: önizleme canlı maliyet sormaz. */}
            {preview.data.costSource === 'CACHE' && (
              <Alert tone="warn" className="mt-4">
                Bu fiyat <strong>önbellekteki maliyete</strong> dayanıyor. Numara satın
                alınırken maliyet sağlayıcıdan canlı sorulur; gerçek satış fiyatı bu
                önizlemeden farklı çıkabilir.
              </Alert>
            )}
          </>
        ) : null}
      </Card>

      {/* ═══════════ Etkin kurallar ═══════════ */}
      <Card>
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h2 className="text-lg font-semibold">Etkin kurallar</h2>
          {!!rules.data?.items.length && (
            <Badge tone="neutral">{rules.data.items.length} kural</Badge>
          )}
        </div>

        {deactivateErr && <ErrorBox err={deactivateErr} className="mt-4" />}

        {rules.isLoading ? (
          <div className="mt-4 flex flex-col gap-2">
            {[0, 1, 2].map((i) => <Skeleton key={i} className="h-20" />)}
          </div>
        ) : listErr ? (
          <ErrorBox err={listErr} className="mt-4" />
        ) : rules.isError ? (
          <Alert className="mt-4">Kurallar yüklenemedi. Bağlantınızı kontrol edip tekrar deneyin.</Alert>
        ) : !rules.data?.items.length ? (
          <Empty
            title="Hiç kural yok"
            hint="En az bir GLOBAL kural olmadan hiçbir ürün fiyatlanamaz."
          />
        ) : (
          <>
            {/* MOBİL: kart listesi */}
            <ul className="mt-4 flex flex-col gap-2 md:hidden">
              {rules.data.items.map((r) => (
                <li key={r.id} className="raised rounded-xl border p-3">
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                      <p className="text-sm font-medium">{scopeDef(r.scope).label}</p>
                      <p className="mt-0.5 break-anywhere text-xs text-muted">{scopeText(r)}</p>
                    </div>
                    <span className="shrink-0 text-sm font-semibold">%{r.marginPercent}</span>
                  </div>
                  <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 border-t border-[var(--border)]
                                  pt-2 text-xs text-muted">
                    <span>Sabit bedel: {formatMoney(r.fixedFee)}</span>
                    <span>Taban: {formatMoney(r.minPrice)}</span>
                    <span>{formatDateTime(r.createdAt)}</span>
                  </div>
                  {r.note && <p className="mt-2 break-anywhere text-xs text-muted">{r.note}</p>}
                  <div className="mt-3">
                    <Button
                      variant="danger" size="sm" fullWidth
                      onClick={() => { deactivate.reset(); setToDeactivate(r); }}
                    >
                      Pasifleştir
                    </Button>
                  </div>
                </li>
              ))}
            </ul>

            {/* MASAÜSTÜ: gerçek tablo */}
            <div className="mt-4 hidden md:block">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-[var(--border)] text-left text-xs
                                 uppercase tracking-wide text-muted">
                    <th scope="col" className="py-2 pr-3 font-medium">Kapsam</th>
                    <th scope="col" className="py-2 pr-3 font-medium">Hedef</th>
                    <th scope="col" className="py-2 pr-3 text-right font-medium">Marj</th>
                    <th scope="col" className="py-2 pr-3 text-right font-medium">Sabit bedel</th>
                    <th scope="col" className="py-2 pr-3 text-right font-medium">Taban</th>
                    <th scope="col" className="py-2 pr-3 font-medium">Oluşturma</th>
                    <th scope="col" className="py-2 text-right font-medium">İşlem</th>
                  </tr>
                </thead>
                <tbody>
                  {rules.data.items.map((r) => (
                    <tr key={r.id} className="border-b border-[var(--border)] last:border-0">
                      <td className="py-3 pr-3 font-medium">{scopeDef(r.scope).label}</td>
                      <td className="py-3 pr-3 break-anywhere text-muted">
                        {scopeText(r)}
                        {r.note && <span className="block text-xs opacity-70">{r.note}</span>}
                      </td>
                      <td className="py-3 pr-3 text-right font-semibold whitespace-nowrap">%{r.marginPercent}</td>
                      <td className="py-3 pr-3 text-right whitespace-nowrap">{formatMoney(r.fixedFee)}</td>
                      <td className="py-3 pr-3 text-right whitespace-nowrap">{formatMoney(r.minPrice)}</td>
                      <td className="py-3 pr-3 whitespace-nowrap text-muted">{formatDateTime(r.createdAt)}</td>
                      <td className="py-3 text-right">
                        <Button
                          variant="danger" size="sm"
                          onClick={() => { deactivate.reset(); setToDeactivate(r); }}
                        >
                          Pasifleştir
                        </Button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </>
        )}
      </Card>

      {/* ═══════════ Onay: kural yaz ═══════════ */}
      <Modal open={confirmOpen} onClose={() => setConfirmOpen(false)} title="Kuralı kaydet">
        <div className="flex flex-col gap-4">
          <p className="text-sm">
            <strong>{scopeDef(scope).label}</strong> kapsamına ({scopeText({
              serviceCode: def.service ? serviceCode : undefined,
              countryIso: def.country ? countryIso : undefined,
              durationMinutes: def.duration ? Number(duration || 0) : undefined,
            })}) <strong>%{'value' in marginParsed ? marginParsed.value : ''}</strong> marjlı
            yeni bir kural yazılacak.
          </p>

          {willReplace ? (
            <Alert tone="warn">
              Bu kapsamda şu an <strong>%{willReplace.marginPercent}</strong> marjlı bir kural
              etkin. Kaydettiğinizde o kural devreden çıkar ve geçmişte kalır — bu bir
              güncelleme değil, yeni bir kayıttır.
            </Alert>
          ) : (
            <Alert tone="info">Bu kapsamda ilk kez kural yazılıyor.</Alert>
          )}

          {preview.data && (
            <p className="text-sm text-muted">
              Önizlemedeki satış fiyatı: <strong className="text-[var(--text)]">
                {formatMoney(preview.data.sellPrice)}
              </strong>
              {preview.data.costSource === 'CACHE' && ' (önbellek maliyetine göre)'}
            </p>
          )}

          {createErr && <ErrorBox err={createErr} />}

          <div className="flex flex-col gap-2 sm:flex-row-reverse">
            <Button data-autofocus onClick={doCreate} loading={create.isPending} fullWidth>
              Evet, kuralı yaz
            </Button>
            <Button variant="outline" onClick={() => setConfirmOpen(false)} fullWidth>
              Vazgeç
            </Button>
          </div>
        </div>
      </Modal>

      {/* ═══════════ Onay: kuralı pasifleştir ═══════════ */}
      <Modal
        open={!!toDeactivate}
        onClose={() => setToDeactivate(null)}
        title="Kuralı pasifleştir"
      >
        {toDeactivate && (
          <div className="flex flex-col gap-4">
            <p className="text-sm">
              <strong>{scopeDef(toDeactivate.scope).label}</strong> ({scopeText(toDeactivate)})
              kapsamındaki <strong>%{toDeactivate.marginPercent}</strong> marjlı kural
              devreden çıkarılacak. Kayıt silinmez, geçmişte kalır.
            </p>

            {toDeactivate.scope === 'GLOBAL' && globalCount <= 1 && (
              <Alert tone="warn">
                Bu <strong>son GLOBAL kural</strong>. Kaldırılırsa hiçbir ürün
                fiyatlanamaz: site açık kalır ama tek bir satış yapılamaz. Sunucu bu
                işlemi reddedecektir — marjı değiştirmek için yeni bir GLOBAL kural yazın,
                eskisi kendiliğinden devreden çıkar.
              </Alert>
            )}

            {deactivateErr && <ErrorBox err={deactivateErr} />}

            <div className="flex flex-col gap-2 sm:flex-row-reverse">
              <Button
                data-autofocus variant="danger" fullWidth
                loading={deactivate.isPending}
                onClick={() => deactivate.mutate(toDeactivate.id)}
              >
                Evet, pasifleştir
              </Button>
              <Button variant="outline" onClick={() => setToDeactivate(null)} fullWidth>
                Vazgeç
              </Button>
            </div>
          </div>
        )}
      </Modal>
    </div>
  );
}
