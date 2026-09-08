'use client';

import * as React from 'react';
import Link from 'next/link';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { ApiError, apiFetch } from '@/lib/api';
import { formatMoney, formatDuration } from '@/lib/format';
import { useCountdown } from '@/hooks/useCountdown';
import { useSession } from '@/hooks/useSession';
import { Button, Badge, Alert, Skeleton, Empty, cx } from '@/components/ui';
import { ServiceIcon } from '@/components/service-icon';
import { Modal } from '@/components/modal';
import { CodeWaiter } from '@/components/code-waiter';
import type { CatalogItem, Quote, ServiceSummary, Order,
  RentalService, RentalDuration } from '@/lib/types';



/** Satın alma modu. */
type Mode = 'activation' | 'rental';

export default function BuyPage() {
  const { user } = useSession();
  const [search, setSearch] = React.useState('');
  const [mode, setMode] = React.useState<Mode>('activation');
  const [openService, setOpenService] = React.useState<ServiceSummary | null>(null);

  // Izgara YALNIZ servis özetini çeker (~35 KB).
  //
  // Tüm servis×ülke matrisi 1,09 MB ediyor: gerçek katalogda 712 servis ve
  // 9768 stoklu kombinasyon var. Mobilde 4G'de bunu indirtmek, kullanıcıyı
  // sayfa açılmadan kaybetmektir (docs/frontend-contract.md §8). Ülkeler
  // servis seçilince ayrıca çekilir.
  const catalog = useQuery({
    queryKey: ['services-in-stock'],
    queryFn: () => apiFetch<{ items: ServiceSummary[] }>('/catalog/services-in-stock'),
    enabled: mode === 'activation',
  });

  // Kiralık katalog AYRI çekilir: aktivasyonda stoklu bir servis kiralıkta
  // olmayabilir (495 kiralık / 712 aktivasyon). Tek listeyi ikisine de
  // kullanmak, kullanıcıyı seçtiği servis için boş bir ülke listesine götürür.
  const rentalCatalog = useQuery({
    queryKey: ['rental-services'],
    queryFn: () => apiFetch<{ items: RentalService[] }>('/catalog/rental/services'),
    enabled: mode === 'rental',
  });

  const services: ServiceSummary[] = mode === 'rental'
    ? (rentalCatalog.data?.items ?? []).map((r) => ({
        code: r.code, name: r.name, iconUrl: r.iconUrl, countryCount: r.countryCount,
      }))
    : (catalog.data?.items ?? []);

  const loading = mode === 'rental' ? rentalCatalog.isLoading : catalog.isLoading;
  const failed = mode === 'rental' ? rentalCatalog.isError : catalog.isError;

  const filtered = React.useMemo(() => {
    // localeCompare/toLocaleLowerCase'de 'tr' ZORUNLU: varsayılan yerelde
    // "İSTANBUL".toLowerCase() → "i̇stanbul" olur ve arama tutmaz.
    const q = search.trim().toLocaleLowerCase('tr');
    if (!q) return services;
    return services.filter((s) =>
      s.name.toLocaleLowerCase('tr').includes(q) || s.code.toLowerCase().includes(q));
  }, [services, search]);

  return (
    <div className="mx-auto flex max-w-5xl flex-col gap-5">
      <div className="text-center">
        <h1 className="text-2xl font-bold uppercase tracking-wide md:text-3xl">Numara Al</h1>
        <p className="mt-1.5 text-sm text-muted">
          Servisi seçin, ülke ve fiyatı bir sonraki adımda görün.
        </p>
      </div>

      {!user?.emailVerified && (
        <Alert tone="warn">
          Numara alabilmek için önce e-posta adresinizi doğrulamanız gerekiyor.
        </Alert>
      )}

      {/*
        MOD ANAHTARI — sekme değil, segment.
        İki farklı ÜRÜN var: tek kullanımlık kod ve süreli numara kiralama.
        Aynı ızgarada karıştırmak, kullanıcının 30 günlük bir numarayı tek
        kod sanıp almasına yol açardı.
      */}
      <div role="tablist" aria-label="Ürün türü"
           className="raised flex gap-1 rounded-2xl border p-1">
        {([
          ['activation', 'Tek kullanımlık', 'Bir kod al, kullan'],
          ['rental', 'Numara kirala', '1 gün – 6 ay'],
        ] as const).map(([m, label, hint]) => (
          <button
            key={m}
            type="button"
            role="tab"
            aria-selected={mode === m}
            onClick={() => { setMode(m); setSearch(''); setOpenService(null); }}
            className={cx(
              'flex min-h-14 flex-1 flex-col items-center justify-center rounded-xl',
              'px-3 py-2 text-sm transition-colors',
              mode === m
                ? 'bg-brand-500 font-semibold text-white'
                : 'text-muted hover:bg-[var(--raised)] hover:text-[var(--text)]',
            )}
          >
            <span>{label}</span>
            <span className={cx('text-[11px]', mode === m ? 'text-white/75' : 'text-muted')}>
              {hint}
            </span>
          </button>
        ))}
      </div>

      {/* Arama HER ZAMAN görünür. Yüzlerce servis arasında kaydırarak aramak,
          mobilde kullanıcıyı listeyi terk etmeye iter. */}
      <label className="relative block">
        <span className="sr-only">Servis ara</span>
        <svg viewBox="0 0 24 24" aria-hidden
             className="pointer-events-none absolute left-4 top-1/2 size-5 -translate-y-1/2 text-muted"
             fill="none" stroke="currentColor" strokeWidth="1.8">
          <circle cx="11" cy="11" r="7" /><path d="M20 20l-3.5-3.5" strokeLinecap="round" />
        </svg>
        <input
          type="search" value={search} onChange={(e) => setSearch(e.target.value)}
          placeholder="Aradığınız servisi yazın.."
          autoCapitalize="none" autoCorrect="off" spellCheck={false}
          className="raised min-h-14 w-full rounded-2xl border pl-12 pr-4 text-base
                     placeholder:text-[var(--muted)] outline-none focus:border-brand-400"
        />
      </label>

      {loading ? (
        <div className="grid grid-cols-2 gap-3 lg:grid-cols-3">
          {Array.from({ length: 12 }, (_, i) => <Skeleton key={i} className="h-20" />)}
        </div>
      ) : failed ? (
        <Alert>Servis listesi yüklenemedi. Bağlantınızı kontrol edip tekrar deneyin.</Alert>
      ) : filtered.length === 0 ? (
        <Empty
          title={search ? `“${search}” için sonuç yok` : 'Stokta servis yok'}
          hint={search ? 'Farklı bir yazım deneyin.' : 'Sağlayıcı stoğu şu anda boş görünüyor.'}
        />
      ) : (
        /* Mobilde 2 sütun — gerçek sistemdeki düzen (docs/frontend-contract.md §2.5) */
        <ul className="grid grid-cols-2 gap-3 lg:grid-cols-3">
          {filtered.map((s) => (
            <li key={s.code}>
              <button
                type="button" onClick={() => setOpenService(s)}
                className="raised flex min-h-20 w-full flex-col justify-center gap-1.5
                           rounded-xl border px-3 py-3 text-left transition-colors
                           hover:border-[var(--color-ink-500)] active:scale-[0.99]"
              >
                <span className="flex min-w-0 items-center gap-2.5">
                  <ServiceIcon name={s.name} iconUrl={s.iconUrl} size={28} />
                  <span className="min-w-0 truncate text-[13px] font-bold uppercase">
                    {s.name}
                  </span>
                </span>
                <span className="flex items-baseline gap-1.5">
                  <span className="text-xs font-semibold text-[var(--color-ok)]">
                    {mode === 'rental' ? 'Süre seçin' : 'Fiyat seçin'}
                  </span>
                  <span className="text-[11px] text-muted">· {s.countryCount} ülke</span>
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}

      <BuyModal
        service={openService}
        mode={mode}
        onClose={() => setOpenService(null)}
        emailVerified={!!user?.emailVerified}
      />
    </div>
  );
}

/* ─────────────────────────────────────────────────────────────── */

function BuyModal({
  service, mode, onClose, emailVerified,
}: {
  service: ServiceSummary | null; mode: Mode;
  onClose: () => void; emailVerified: boolean;
}) {
  const [countryIso, setCountryIso] = React.useState('');
  const [durationMinutes, setDurationMinutes] = React.useState(0);
  const [order, setOrder] = React.useState<Order | null>(null);
  const qc = useQueryClient();
  const rental = mode === 'rental';

  // Ülkeler YALNIZ modal açıkken ve YALNIZ seçilen servis için çekilir (~6 KB).
  // `enabled` olmadan, modal kapalıyken de istek giderdi.
  const countries = useQuery({
    queryKey: ['availability', mode, service?.code],
    queryFn: async () => {
      const path = rental
        ? `/catalog/rental/countries?serviceCode=${encodeURIComponent(service!.code)}`
        : `/catalog/availability?serviceCode=${encodeURIComponent(service!.code)}`;
      const d = await apiFetch<{ items: Array<CatalogItem | { iso2: string; name: string; phoneCode: string }> }>(path);
      // İki uç iki farklı şekil döndürüyor; tek biçime indiriyoruz ki
      // aşağıdaki liste her iki modda aynı kodla çizilsin.
      // SUNUCUNUN SIRASI KORUNUR — burada yeniden sıralanmaz.
      //
      // Sunucu Türkiye'yi başa alıyor (kullanıcıların çoğu Türkiye'den).
      // İstemcide alfabetik sıralamak o kararı sessizce geri alırdı ve
      // "neden sıra değişti?" sorusunun cevabı iki dosyada aranırdı.
      // STOKSUZ SATIR ELENMEZ, DEVRE DIŞI GÖSTERİLİR.
      //
      // Sunucu stoksuz kombinasyonları zaten gizliyor; tek istisna kullanıcının
      // kendi ülkesi (Türkiye). Onu da listeden atarsak kullanıcı "bu site Türk
      // numarası satmıyor" diye düşünür. Seçilemez göstermek doğruyu söyler:
      // "satıyoruz, şu an stok yok."
      return d.items.map((x) =>
        'countryIso2' in x
          ? { iso: x.countryIso2, name: x.countryName, phone: x.phoneCode, ok: x.inStock }
          : { iso: x.iso2, name: x.name, phone: x.phoneCode, ok: true },
      );
    },
    enabled: !!service,
  });

  // Kiralık süreler — ülke seçilince çekilir.
  const durations = useQuery({
    queryKey: ['rental-durations', service?.code, countryIso],
    queryFn: () => apiFetch<{ items: RentalDuration[] }>(
      `/catalog/rental/durations?serviceCode=${encodeURIComponent(service!.code)}` +
      `&countryIso=${encodeURIComponent(countryIso)}`),
    enabled: rental && !!service && !!countryIso,
    select: (d) => d.items.filter((x) => x.inStock),
  });

  const quote = useMutation({
    mutationFn: (v: { service: string; country: string; minutes?: number }) =>
      apiFetch<Quote>(
        `/catalog/quote?serviceCode=${encodeURIComponent(v.service)}` +
        `&countryIso=${encodeURIComponent(v.country)}` +
        (v.minutes ? `&durationMinutes=${v.minutes}` : ''),
      ),
  });

  // Modal her açılışta SIFIRDAN başlar. Sıfırlanmazsa kullanıcı bir servisi
  // kapatıp diğerini açtığında ÖNCEKİ servisin fiyatını görür — ve o fiyata
  // güvenerek satın alır.
  React.useEffect(() => {
    setCountryIso('');
    setDurationMinutes(0);
    setOrder(null);
    quote.reset();
    purchase.reset();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [service?.code, mode]);

  /*
    SATIN ALMA — MUTASYON ASLA OTOMATİK TEKRARLANMAZ.

    `retry: false` burada bir kez daha yazılıyor (genel yapılandırmada da var):
    bu çağrının tekrarı İKİ NUMARA alır ve kullanıcıdan iki kez ücret keser.
    Sağlayıcı tarafı da idempotent değildir — spec'te "idempot" kelimesi hiç
    geçmiyor.

    Gövdede YALNIZ quoteId var: fiyat, sağlayıcı ve maliyet sunucuda belirlenir.
  */
  const purchase = useMutation({
    retry: false,
    mutationFn: (quoteId: string) =>
      apiFetch<Order>('/orders', { method: 'POST', body: { quoteId } }),
    onSuccess: (o) => {
      setOrder(o);
      // Bakiye değişti — panelin her yerinde güncel görünsün.
      qc.invalidateQueries({ queryKey: ['me'] });
      qc.invalidateQueries({ queryKey: ['statement'] });
      qc.invalidateQueries({ queryKey: ['orders'] });
    },
  });

  function pickCountry(iso: string) {
    setCountryIso(iso);
    setDurationMinutes(0);
    quote.reset();
    // KİRALIKTA ülke seçmek teklif ALMAZ: önce süre seçilmeli, çünkü fiyat
    // süreye göre değişiyor. Aktivasyonda süre yok, doğrudan teklif alınır.
    if (iso && service && !rental) {
      quote.mutate({ service: service.code, country: iso });
    }
  }

  function pickDuration(minutes: number) {
    setDurationMinutes(minutes);
    if (service && countryIso) {
      quote.mutate({ service: service.code, country: countryIso, minutes });
    }
  }

  // Sipariş verildiyse modal KOD BEKLEME ekranına döner. Ayrı bir sayfaya
  // yönlendirmek, kullanıcıyı parasını verdiği anda tanımadığı bir ekrana atar.
  if (service && order) {
    return (
      <Modal open onClose={onClose} title={service.name}>
        <CodeWaiter order={order} onClose={onClose} iconUrl={service.iconUrl} />
      </Modal>
    );
  }

  return (
    <Modal open={!!service} onClose={onClose} title={service?.name ?? ''}>
      {!service ? null : (
        <div className="flex flex-col gap-4">
          <label className="flex flex-col gap-1.5">
            <span className="text-sm font-medium">Ülke Seçin</span>
            <select
              data-autofocus
              value={countryIso}
              onChange={(e) => pickCountry(e.target.value)}
              disabled={countries.isLoading || countries.isError}
              className="raised min-h-12 w-full rounded-xl border px-3 text-base outline-none
                         focus:border-brand-400 disabled:opacity-60"
            >
              <option value="">
                {countries.isLoading ? 'Ülkeler yükleniyor…'
                  : countries.isError ? 'Ülkeler yüklenemedi'
                  : 'Ülke seçiniz…'}
              </option>
              {(countries.data ?? []).map((c) => (
                <option key={c.iso} value={c.iso} disabled={!c.ok}>
                  {c.name} (+{c.phone}){c.ok ? '' : ' — şu an stok yok'}
                </option>
              ))}
            </select>
          </label>

          {/* ── Kiralama süresi ── */}
          {rental && countryIso && (
            <div className="flex flex-col gap-2">
              <span className="text-sm font-medium">Kiralama Süresi</span>
              {durations.isLoading ? (
                <Skeleton className="h-24" />
              ) : durations.isError ? (
                <Alert>Süreler yüklenemedi.</Alert>
              ) : (durations.data ?? []).length === 0 ? (
                <Alert tone="warn">Bu ülke için kiralanabilir süre kalmadı.</Alert>
              ) : (
                <ul className="grid grid-cols-2 gap-2">
                  {(durations.data ?? []).map((d) => (
                    <li key={d.minutes}>
                      <button
                        type="button"
                        onClick={() => pickDuration(d.minutes)}
                        aria-pressed={durationMinutes === d.minutes}
                        className={cx(
                          'flex min-h-12 w-full items-center justify-center rounded-xl',
                          'border px-3 py-2 text-sm transition-colors',
                          durationMinutes === d.minutes
                            ? 'border-brand-500 bg-brand-500/12 font-semibold'
                            : 'raised hover:border-[var(--color-ink-500)]',
                        )}
                      >
                        {d.label}
                      </button>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          )}

          {/* Teklif: aktivasyonda ülke yeter, kiralıkta süre de gerekir. */}
          {(rental ? !!durationMinutes : !!countryIso) && (
            <QuoteBox
              quote={quote.data ?? null}
              error={quote.error}
              pending={quote.isPending}
              emailVerified={emailVerified}
              buying={purchase.isPending}
              buyError={purchase.error}
              onBuy={() => quote.data && purchase.mutate(quote.data.quoteId)}
              onRefresh={() => quote.mutate({
                service: service.code, country: countryIso,
                minutes: rental ? durationMinutes : undefined,
              })}
            />
          )}
        </div>
      )}
    </Modal>
  );
}

function QuoteBox({
  quote, error, pending, emailVerified, buying, buyError, onBuy, onRefresh,
}: {
  quote: Quote | null; error: unknown; pending: boolean;
  emailVerified: boolean; buying: boolean; buyError: unknown;
  onBuy: () => void; onRefresh: () => void;
}) {
  const left = useCountdown(quote?.expiresAt);
  const expired = !!quote && left <= 0;

  if (pending) return <Skeleton className="h-36" />;

  if (error) {
    const e = error instanceof ApiError ? error : null;
    // Kullanıcıya NE YAPACAĞINI söyleriz. Eski sistemin "Sunucuya
    // bağlanılamadı." kutusu, kullanıcıyı hiçbir yere götürmüyordu.
    const hint =
      e?.code === 'EMAIL_NOT_VERIFIED' ? 'E-posta adresinizi doğruladıktan sonra tekrar deneyin.'
      : e?.code === 'OUT_OF_STOCK'     ? 'Bu ülke az önce tükendi. Başka bir ülke seçin.'
      : e?.code === 'NO_PRICING_RULE'  ? 'Bu servis için fiyatlandırma tanımlı değil. Destekle iletişime geçin.'
      : e?.code === 'NETWORK'          ? 'Bağlantınızı kontrol edip tekrar deneyin.'
      : null;
    return (
      <Alert>
        <p>{e?.message ?? 'Fiyat alınamadı.'}</p>
        {hint && <p className="mt-1 opacity-80">{hint}</p>}
        {e?.requestId && <p className="mt-2 text-xs opacity-60">İstek no: {e.requestId}</p>}
        <Button variant="outline" size="sm" onClick={onRefresh} className="mt-3">
          Tekrar dene
        </Button>
      </Alert>
    );
  }

  if (!quote) return null;

  return (
    <div className="flex flex-col gap-4">
      {/* Kesikli çerçeve: gerçek sistemdeki fiyat kutusu deseni */}
      <div className="rounded-xl border border-dashed border-[var(--border)] p-4">
        <dl className="flex flex-col gap-2.5">
          <div className="flex items-center justify-between gap-3">
            <dt className="text-sm text-muted">Stok Durumu</dt>
            <dd>
              <Badge tone={quote.stock > 0 ? 'ok' : 'bad'}>
                {quote.stock > 0 ? `${quote.stock} adet` : 'Tükendi'}
              </Badge>
            </dd>
          </div>
          <div className="flex items-center justify-between gap-3">
            <dt className="text-sm text-muted">Birim Fiyat</dt>
            <dd className="text-2xl font-bold text-[var(--color-ok)]">
              {formatMoney(quote.price)}
            </dd>
          </div>
          <div className="flex items-center justify-between gap-3 border-t
                          border-[var(--border)] pt-2.5">
            <dt className="text-sm text-muted">Fiyat geçerliliği</dt>
            <dd className={cx('text-sm font-medium',
                              expired ? 'text-[var(--color-bad)]' : 'text-[var(--text)]')}>
              {expired ? 'Süre doldu' : formatDuration(left)}
            </dd>
          </div>
        </dl>

        {expired ? (
          <Button onClick={onRefresh} fullWidth className="mt-4">Yeni fiyat al</Button>
        ) : (
          <Button
            fullWidth
            className="mt-4"
            loading={buying}
            disabled={!emailVerified || quote.stock <= 0}
            onClick={onBuy}
          >
            Satın Al ({formatMoney(quote.price)})
          </Button>
        )}
      </div>

      {buyError instanceof ApiError && (
        <Alert>
          <p>{buyError.message}</p>
          {/* İstek numarası: kullanıcı destek yazarsa olayı log'da bulabilelim. */}
          {buyError.requestId && (
            <p className="mt-2 text-xs opacity-60">İstek no: {buyError.requestId}</p>
          )}
        </Alert>
      )}

      {!emailVerified && (
        <Alert tone="warn">E-posta doğrulaması olmadan satın alma yapılamaz.</Alert>
      )}

      <p className="text-xs leading-relaxed text-muted">
        SMS kodu geldiğinde otomatik olarak ekrana yansıyacaktır. Kod gelmezse
        ücret bakiyenize iade edilir.
      </p>

      <Link href="/panel/cuzdan"
            className="inline-flex min-h-11 items-center justify-center text-sm text-brand-400
                       underline-offset-4 hover:underline">
        Bakiyemi görüntüle
      </Link>
    </div>
  );
}
