'use client';

/**
 * /yonetim/fiyatlar — fiyat kuralları, canlı önizleme, etkin kural listesi.
 *
 * DAVRANIŞ DEĞİŞMEDİ. Sunum ve yapı değişti:
 *   · yerel `toMinor` → `lib/para.ts` `TUTAR_FIYAT_KURALI` ayarı (aynı politika:
 *     boş = 0, negatif ret, 1.000.000,00 ₺ tavanı, aynı hata metinleri)
 *   · yerel `ErrorBox` / `selectClass` / textarea sınıfı → katman bileşenleri
 *   · mobil kart + masaüstü tablo ikizi → tek `VeriTablosu` sütun tanımı
 *   · iki elle kurulmuş onay modalı → `OnayDiyalogu`
 *
 * 🔴 TEK DAVRANIŞ DEĞİŞİKLİĞİ: her iki onay diyaloğunda İLK ODAK artık
 * "Vazgeç"tedir (eskiden yıkıcı/onay düğmesindeydi — `fiyatlar:721` ve
 * `fiyatlar:758`). Gerekçe tasarim-sistemi.md §7.5: buraya klavyeyle gelinir,
 * Enter basılı kalırsa `click` keydown tekrarıyla yeniden üretilir ve odak
 * onaydaysa TEK TUŞ fiyat kuralını yazar. Bu ekran satış fiyatını belirler.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * 🔴 ÖNİZLEME SORGUSUNUN ANAHTARI — dokunmadan önce oku
 * ══════════════════════════════════════════════════════════════════════════
 * `previewBody` memo'sunun bağımlılıkları İLKEL DEĞER olmak zorundadır. Nesne
 * verilirse her render'da yeni referans üretilir, sorgu anahtarı sürekli
 * değişir ve 500ms gecikmeye RAĞMEN her tuş vuruşunda istek atılır. Yönetim
 * uçları dakikada 60 istekle sınırlıdır.
 */

import * as React from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { apiFetch } from '@/lib/api';
import { formatDateTime, formatMoney } from '@/lib/format';
import { TUTAR_FIYAT_KURALI, toMinor } from '@/lib/para';
import { Alert, Badge, Button, Card, Empty, Field, Skeleton, cx } from '@/components/ui';
import {
  CokSatir,
  HataDurumu,
  KayitSayaci,
  OnayDiyalogu,
  SayfaBasligi,
  Secim,
  VeriTablosu,
  apiHatasi,
  type Sutun,
} from '@/components/yonetim';
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

  // Para AYRIŞTIRMA `lib/para.ts`'te; bu ekranın politikası TUTAR_FIYAT_KURALI:
  // boş = 0 (sabit bedel ve taban İSTEĞE BAĞLI), negatif ret, 1.000.000,00 ₺
  // tavanı — asıl risk büyük değer değil, FAZLADAN İKİ SIFIR.
  const marginParsed = normalizeMargin(margin);
  const feeParsed = toMinor(fixedFee, TUTAR_FIYAT_KURALI);
  const minParsed = toMinor(minPrice, TUTAR_FIYAT_KURALI);

  // 🔴 İLKEL DEĞERLER — dosya başındaki uyarı. Nesne verilirse her tuşta istek gider.
  const marginValue = 'value' in marginParsed ? marginParsed.value : null;
  const feeMinor = 'minor' in feeParsed ? feeParsed.minor : null;
  const minMinor = 'minor' in minParsed ? minParsed.minor : null;
  const durationMinutes = def.duration ? Number(duration || 0) : 0;

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
    // `validate()` bunları zaten geçirdi; burada tekrar bakmak, onay
    // açıkken alanların değişebildiği bir gelecekte sessiz `null` göndermeyi
    // engeller.
    if (marginValue === null || feeMinor === null || minMinor === null) return;
    create.mutate({
      scope,
      serviceCode: def.service ? serviceCode : '',
      countryIso: def.country ? countryIso : '',
      durationMinutes: def.duration ? Number(duration) : 0,
      marginPercent: marginValue,
      fixedFeeMinor: feeMinor,
      minPriceMinor: minMinor,
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

  const createErr = apiHatasi(create.error);
  const deactivateErr = apiHatasi(deactivate.error);
  const previewErr = apiHatasi(preview.error);

  const serviceOptions = services.data?.items ?? [];
  const countryOptions = countries.data?.items ?? [];

  /* ── Kural listesi sütunları: TEK veri tanımı (§6.1 kural 2) ── */
  const sutunlar: Array<Sutun<PricingRule>> = [
    {
      anahtar: 'kapsam',
      baslik: 'Kapsam',
      mobilRol: 'baslik',
      hucre: (r) => <p className="font-medium">{scopeDef(r.scope).label}</p>,
    },
    {
      anahtar: 'hedef',
      baslik: 'Hedef',
      hucre: (r) => (
        <div className="min-w-0">
          <p className="break-anywhere text-muted">{scopeText(r)}</p>
          {/* Not `text-sm text-muted`: eski kopyalar 12px + %70 opaklık
              yazıyordu, ikisi de §3.2 ve §7.1 eşiklerinin altında. */}
          {r.note && <p className="break-anywhere text-sm text-muted">{r.note}</p>}
        </div>
      ),
    },
    {
      anahtar: 'marj',
      baslik: 'Marj',
      mobilRol: 'rozet',
      hizala: 'sag',
      sayisal: true,
      hucre: (r) => <span className="font-semibold tabular-nums">%{r.marginPercent}</span>,
    },
    {
      anahtar: 'sabitBedel',
      baslik: 'Sabit bedel',
      oncelik: 3,
      hizala: 'sag',
      sayisal: true,
      hucre: (r) => formatMoney(r.fixedFee),
    },
    {
      anahtar: 'taban',
      baslik: 'Taban',
      hizala: 'sag',
      sayisal: true,
      hucre: (r) => formatMoney(r.minPrice),
    },
    {
      anahtar: 'olusturma',
      baslik: 'Oluşturma',
      oncelik: 3,
      hizala: 'sag',
      sayisal: true,
      hucre: (r) => <span className="text-muted">{formatDateTime(r.createdAt)}</span>,
    },
    {
      anahtar: 'islem',
      baslik: 'İşlem',
      basligiGizle: true,
      hizala: 'sag',
      mobilRol: 'eylem',
      hucre: (r, sunum) => (
        <Button
          variant="danger"
          size="sm"
          fullWidth={sunum === 'kart'}
          onClick={() => {
            deactivate.reset();
            setToDeactivate(r);
          }}
        >
          Pasifleştir
        </Button>
      ),
    },
  ];

  return (
    <div className="mx-auto flex max-w-5xl flex-col gap-6">
      <SayfaBasligi
        baslik="Fiyat kuralları"
        aciklama="Satış fiyatı = sağlayıcı maliyeti × (1 + marj) + sabit bedel; sonuç taban fiyatın altına düşemez."
      />

      {/* Bu ekranın EN ÖNEMLİ cümlesi: "Kaydet" bir güncelleme değildir.
          `duyur={false}`: önemli olması onu bir UYARI yapmaz — sayfa açılışında
          koşulsuz çizilir, kesecek bir eylem yoktur (§7.4). Aynı bilgi, kaydetme
          anında onay diyaloğunda İKİNCİ KEZ karşıya çıkar. */}
      <Alert tone="warn" duyur={false}>
        <strong>Kural güncelleme diye bir işlem yoktur.</strong> Bir kapsama yeni kural
        yazdığınızda o kapsamdaki eski kural aynı anda devreden çıkar ve geçmişte kalır —
        eski siparişlerin hangi kuralla fiyatlandığı izlenebilir kalsın diye silinmez.
        Yani &laquo;Kaydet&raquo; bir düzenleme gibi görünür, aslında yeni bir kayıt yazar.
      </Alert>

      {/* ═══════════ Kural formu ═══════════ */}
      <Card>
        <h2 className="text-lg font-semibold">Yeni kural</h2>

        <form onSubmit={onSubmit} className="mt-4 flex flex-col gap-4" noValidate>
          <Secim
            etiket="Kapsam"
            value={scope}
            onChange={(e) => changeScope(e.target.value as PricingScope)}
            ipucu={def.hint}
          >
            {SCOPES.map((s) => (
              <option key={s.value} value={s.value}>
                {s.label}
              </option>
            ))}
          </Secim>

          {def.service && (
            <Secim
              etiket="Servis"
              value={serviceCode}
              onChange={(e) => setServiceCode(e.target.value)}
              disabled={services.isLoading || services.isError}
              hata={errors.serviceCode}
            >
              <option value="">
                {services.isLoading ? 'Servisler yükleniyor…'
                  : services.isError ? 'Servisler yüklenemedi'
                  : 'Servis seçiniz…'}
              </option>
              {serviceOptions.map((s) => (
                <option key={s.code} value={s.code}>{s.name} ({s.code})</option>
              ))}
            </Secim>
          )}

          {def.country && (
            <Secim
              etiket="Ülke"
              value={countryIso}
              onChange={(e) => setCountryIso(e.target.value)}
              disabled={countries.isLoading || countries.isError}
              hata={errors.countryIso}
            >
              <option value="">
                {countries.isLoading ? 'Ülkeler yükleniyor…'
                  : countries.isError ? 'Ülkeler yüklenemedi'
                  : 'Ülke seçiniz…'}
              </option>
              {countryOptions.map((c) => (
                <option key={c.iso2} value={c.iso2}>{c.name} ({c.iso2})</option>
              ))}
            </Secim>
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

          <CokSatir
            etiket="Not"
            value={note}
            onChange={(e) => setNote(e.target.value)}
            rows={2}
            maxLength={500}
            placeholder="Bu kuralı neden yazdınız? Denetim kaydında görünür."
            hata={errors.note}
          />

          <Button type="submit" fullWidth loading={create.isPending}>
            Kuralı kaydet
          </Button>
        </form>

        {createErr && <HataDurumu hata={createErr} className="mt-4" />}

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
        <p className="mt-2 max-w-[70ch] text-sm text-muted">
          Formdaki değerler kaydedilmeden önce somut bir ürün üzerinde denenir.
        </p>

        {(!def.service || !def.country) && (
          <div className="mt-4 grid gap-4 md:grid-cols-2">
            {!def.service && (
              <Secim
                etiket="Deneme servisi"
                value={testService}
                onChange={(e) => setTestService(e.target.value)}
                disabled={services.isLoading || services.isError}
              >
                <option value="">Servis seçiniz…</option>
                {serviceOptions.map((s) => (
                  <option key={s.code} value={s.code}>{s.name} ({s.code})</option>
                ))}
              </Secim>
            )}
            {!def.country && (
              <Secim
                etiket="Deneme ülkesi"
                value={testCountry}
                onChange={(e) => setTestCountry(e.target.value)}
                disabled={countries.isLoading || countries.isError}
              >
                <option value="">Ülke seçiniz…</option>
                {countryOptions.map((c) => (
                  <option key={c.iso2} value={c.iso2}>{c.name} ({c.iso2})</option>
                ))}
              </Secim>
            )}
          </div>
        )}

        {!previewReady ? (
          <Empty
            title="Önizleme için ürün seçin"
            hint="Fiyat her zaman somut bir servis ve ülke üzerinde hesaplanır."
          />
        ) : preview.isLoading ? (
          // Yükleme İSKELETLE (§5.1) ve iskelet gerçek yükseklikleri taklit
          // eder ki veri gelince düzen zıplamasın.
          <div className="mt-4 flex flex-col gap-4">
            <Skeleton className="h-28 rounded-xl" />
            <Skeleton className="h-10" />
            <Skeleton className="h-10" />
          </div>
        ) : previewErr ? (
          <HataDurumu hata={previewErr} className="mt-4" />
        ) : preview.data ? (
          <OnizlemeSonucu veri={preview.data} />
        ) : null}
      </Card>

      {/* ═══════════ Etkin kurallar ═══════════ */}
      <Card>
        <div className="flex flex-wrap items-center justify-between gap-3">
          <h2 className="text-lg font-semibold">Etkin kurallar</h2>
          <KayitSayaci toplam={rules.data?.items.length ?? 0} />
        </div>

        {deactivateErr && <HataDurumu hata={deactivateErr} className="mt-4" />}

        <VeriTablosu<PricingRule>
          className="mt-4"
          baslik="Etkin fiyat kuralları"
          sutunlar={sutunlar}
          satirlar={rules.data?.items}
          satirAnahtari={(r) => String(r.id)}
          yukleniyor={rules.isLoading}
          hata={apiHatasi(rules.error)}
          iskeletSatir={3}
          bos={
            <Empty
              title="Hiç kural yok"
              hint="En az bir GLOBAL kural olmadan hiçbir ürün fiyatlanamaz."
            />
          }
        />
      </Card>

      {/* ═══════════ Onay: kural yaz ═══════════ */}
      <OnayDiyalogu
        acik={confirmOpen}
        baslik="Kuralı kaydet"
        onayMetni="Evet, kuralı yaz"
        bekliyor={create.isPending}
        hata={createErr}
        onOnayla={doCreate}
        onIptal={() => setConfirmOpen(false)}
      >
        <p className="text-sm">
          <strong>{scopeDef(scope).label}</strong> kapsamına ({scopeText({
            serviceCode: def.service ? serviceCode : undefined,
            countryIso: def.country ? countryIso : undefined,
            durationMinutes: def.duration ? Number(duration || 0) : undefined,
          })}) <strong>%{marginValue ?? ''}</strong> marjlı yeni bir kural yazılacak.
        </p>

        {/*
          Ton `OnayDiyalogu`nun `uyari` yuvasından DEĞİL buradan geliyor:
          o yuva yıkıcı olmayan diyalogda `info` çizer, oysa "etkin kural
          devreden çıkacak" bir `warn`dır. Ölçülen tonlar korundu.
        */}
        {/*
          `duyur={false}` (ikisinde de): bu kutular onay diyaloğunun GÖVDE
          METNİdir ve diyalog açılırken zaten oradadırlar. Diyalog açılışı
          kendi duyurusunu yapar ("Kuralı kaydet", diyalog); `role="alert"` onu
          kesip önüne geçerdi (§7.4). Üstelik asıl sonuç cümlesi — hangi kapsama
          hangi marjın yazılacağı — yukarıdaki düz `<p>`'dedir; onun sessiz,
          yanındaki kutunun bağıran olması tutarsızdı.
        */}
        {willReplace ? (
          <Alert tone="warn" duyur={false}>
            Bu kapsamda şu an <strong>%{willReplace.marginPercent}</strong> marjlı bir kural
            etkin. Kaydettiğinizde o kural devreden çıkar ve geçmişte kalır — bu bir
            güncelleme değil, yeni bir kayıttır.
          </Alert>
        ) : (
          <Alert tone="info" duyur={false}>Bu kapsamda ilk kez kural yazılıyor.</Alert>
        )}

        {preview.data && (
          <p className="text-sm text-muted">
            Önizlemedeki satış fiyatı:{' '}
            <strong className="tabular-nums text-[var(--text)]">
              {formatMoney(preview.data.sellPrice)}
            </strong>
            {preview.data.costSource === 'CACHE' && ' (önbellek maliyetine göre)'}
          </p>
        )}
      </OnayDiyalogu>

      {/* ═══════════ Onay: kuralı pasifleştir ═══════════ */}
      <OnayDiyalogu
        acik={toDeactivate !== null}
        baslik="Kuralı pasifleştir"
        onayMetni="Evet, pasifleştir"
        yikici
        bekliyor={deactivate.isPending}
        hata={deactivateErr}
        onOnayla={() => toDeactivate && deactivate.mutate(toDeactivate.id)}
        onIptal={() => setToDeactivate(null)}
      >
        {toDeactivate && (
          <>
            <p className="text-sm">
              <strong>{scopeDef(toDeactivate.scope).label}</strong> ({scopeText(toDeactivate)})
              kapsamındaki <strong>%{toDeactivate.marginPercent}</strong> marjlı kural
              devreden çıkarılacak. Kayıt silinmez, geçmişte kalır.
            </p>

            {/* `duyur={false}`: koşul (`globalCount <= 1`) bir eylem değil,
                verinin hâli — kutu "Kuralı pasifleştir" diyaloğu açılırken
                zaten çizilidir. Diyaloğun açılış duyurusunu kesmez (§7.4). */}
            {toDeactivate.scope === 'GLOBAL' && globalCount <= 1 && (
              <Alert tone="warn" duyur={false}>
                Bu <strong>son GLOBAL kural</strong>. Kaldırılırsa hiçbir ürün
                fiyatlanamaz: site açık kalır ama tek bir satış yapılamaz. Sunucu bu
                işlemi reddedecektir — marjı değiştirmek için yeni bir GLOBAL kural yazın,
                eskisi kendiliğinden devreden çıkar.
              </Alert>
            )}
          </>
        )}
      </OnayDiyalogu>
    </div>
  );
}

/* ═══════════════════════ Önizleme sonucu ═══════════════════════ */

/**
 * Önizleme kartı.
 *
 * `text-3xl` — `md:text-4xl` KALDIRILDI: §3.1, `text-4xl` ve üstü yalnız
 * pazarlama yüzeyinindir, Operate modunda yasaktır. Etiket de `text-xs
 * uppercase tracking-wide` değil `text-sm`: 12px veri taşımaz (§3.2) ve CSS
 * büyük harf dönüşümü Türkçede `i → I` üretir.
 */
function OnizlemeSonucu({ veri }: { veri: PricingPreview }) {
  return (
    <>
      <div className="mt-4 rounded-xl border border-[var(--border)] p-4 md:p-6">
        <p className="text-sm font-medium text-muted">Satış fiyatı</p>
        <p className="mt-2 text-3xl font-bold tabular-nums">{formatMoney(veri.sellPrice)}</p>

        <div className="mt-4 flex flex-wrap gap-2">
          {/*
            `brand` tonu burada KAYNAK anlamında (kayıtlı kural ≠ formdaki
            aday). §11.6 "brand beş anlam taşıyor" açık kararı henüz
            verilmediği için ton DEĞİŞTİRİLMEDİ; anlamı taşıyan asıl kanal
            zaten rozetin metnidir.
          */}
          <Badge tone={veri.ruleSource === 'CANDIDATE' ? 'brand' : 'neutral'}>
            {veri.ruleSource === 'CANDIDATE'
              ? 'Formdaki kural (kaydedilmedi)'
              : `Kayıtlı kural${veri.ruleScope ? ` · ${veri.ruleScope}` : ''}`}
          </Badge>
          <Badge tone="neutral">Marj %{veri.marginPercent}</Badge>
          {veri.hitMinimum && <Badge tone="warn">Taban fiyat uygulandı</Badge>}
          <Badge tone={veri.inStock ? 'ok' : 'bad'}>
            {veri.inStock ? `Stok: ${veri.stock}` : 'Stok yok'}
          </Badge>
        </div>
      </div>

      {/* Yöneticiye ara değerler: bunlar kullanıcı teklifinde ASLA görünmez. */}
      <dl className="mt-4 flex flex-col gap-3 text-sm">
        <OnizlemeSatiri etiket="Sağlayıcı">
          <span className="min-w-0 truncate font-medium">{veri.providerName || '—'}</span>
        </OnizlemeSatiri>
        <OnizlemeSatiri etiket="Maliyet">
          <span className="font-medium tabular-nums">{formatMoney(veri.cost)}</span>
        </OnizlemeSatiri>
        <OnizlemeSatiri etiket="Maliyet (TL)">
          <span className="font-medium tabular-nums">{formatMoney(veri.costInTry)}</span>
        </OnizlemeSatiri>
        <OnizlemeSatiri etiket="Kur" sonuncu>
          <span className="break-anywhere text-right font-medium tabular-nums">
            {veri.fxRate || '—'}
            {veri.fxFetchedAt && (
              <span className="block font-normal text-muted">
                {formatDateTime(veri.fxFetchedAt)}
              </span>
            )}
          </span>
        </OnizlemeSatiri>
      </dl>

      {/*
        Sessiz kalmak yöneticiyi yanıltır: önizleme canlı maliyet sormaz.

        🔴 `duyur={false}` — TEK GEREKÇELİ İSTİSNA. Bu kutu bir eylemin sonucu
        gibi görünür (yönetici yazdıkça önizleme yenilenir) ama yaşadığı yer
        SÜREKLİ TAZELENEN bir paneldir: 500ms geciktirilmiş her sorgu sonucunda
        belirip kaybolabilir. `role="alert"` "kullanıcının o an yaptığı şeyi
        BÖL" demektir; buradaki o şey YAZI YAZMAKTIR ve her tuş vuruşunda
        bölünmek §7.4'ün önlemek istediği zararın ta kendisidir. Yanındaki
        satış fiyatı da duyurulmuyor — sayı sessizken uyarının bağırması
        tutarsız olurdu. Aynı bilgi onay diyaloğunda düz metin olarak tekrar
        karşıya çıkar ("(önbellek maliyetine göre)").
      */}
      {veri.costSource === 'CACHE' && (
        <Alert tone="warn" duyur={false} className="mt-4">
          Bu fiyat <strong>önbellekteki maliyete</strong> dayanıyor. Numara satın
          alınırken maliyet sağlayıcıdan canlı sorulur; gerçek satış fiyatı bu
          önizlemeden farklı çıkabilir.
        </Alert>
      )}
    </>
  );
}

function OnizlemeSatiri({
  etiket, sonuncu, children,
}: { etiket: string; sonuncu?: boolean; children: React.ReactNode }) {
  return (
    <div className={cx('flex justify-between gap-4', !sonuncu && 'border-b border-[var(--border)] pb-3')}>
      <dt className="text-muted">{etiket}</dt>
      <dd className="min-w-0">{children}</dd>
    </div>
  );
}
