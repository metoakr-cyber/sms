'use client';

import * as React from 'react';
import Link from 'next/link';
import { useQuery, useMutation } from '@tanstack/react-query';
import { ApiError, apiFetch } from '@/lib/api';
import { formatMoney, formatDuration } from '@/lib/format';
import { useCountdown } from '@/hooks/useCountdown';
import { useSession } from '@/hooks/useSession';
import { Card, Button, Badge, Alert, Skeleton, Empty, Field, cx } from '@/components/ui';
import { ServiceIcon } from '@/components/service-icon';
import type { CatalogItem, Quote } from '@/lib/types';

export default function BuyPage() {
  const { user } = useSession();
  const [service, setService] = React.useState<string | null>(null);
  const [country, setCountry] = React.useState<string | null>(null);
  const [search, setSearch] = React.useState('');

  const catalog = useQuery({
    queryKey: ['availability'],
    queryFn: () => apiFetch<{ items: CatalogItem[] }>('/catalog/availability'),
  });

  const quote = useMutation({
    mutationFn: (v: { service: string; country: string }) =>
      apiFetch<Quote>(
        `/catalog/quote?serviceCode=${encodeURIComponent(v.service)}` +
        `&countryIso=${encodeURIComponent(v.country)}`,
      ),
  });

  const items = catalog.data?.items ?? [];

  // Servis listesi — her servis bir kez, stoklu ülke sayısıyla.
  const services = React.useMemo(() => {
    const m = new Map<string, { code: string; name: string; iconUrl?: string; count: number }>();
    for (const it of items) {
      if (!it.inStock) continue;
      const e = m.get(it.serviceCode)
        ?? { code: it.serviceCode, name: it.serviceName, iconUrl: it.iconUrl, count: 0 };
      e.count++;
      m.set(it.serviceCode, e);
    }
    return [...m.values()].sort((a, b) => a.name.localeCompare(b.name, 'tr'));
  }, [items]);

  const countries = React.useMemo(() => {
    if (!service) return [];
    return items
      .filter((i) => i.serviceCode === service && i.inStock)
      .sort((a, b) => a.countryName.localeCompare(b.countryName, 'tr'));
  }, [items, service]);

  const filteredServices = React.useMemo(() => {
    const s = search.trim().toLocaleLowerCase('tr');
    return s ? services.filter((x) => x.name.toLocaleLowerCase('tr').includes(s)) : services;
  }, [services, search]);

  function pickService(code: string) {
    setService(code); setCountry(null); setSearch(''); quote.reset();
  }
  function pickCountry(iso: string) {
    if (!service) return;
    setCountry(iso);
    quote.mutate({ service, country: iso });
  }

  return (
    <div className="mx-auto flex max-w-5xl flex-col gap-5">
      <div>
        <h1 className="text-2xl font-bold tracking-tight md:text-3xl">Numara al</h1>
        <p className="mt-1 text-sm text-muted">Servisi ve ülkeyi seçin, fiyatı görün.</p>
      </div>

      {!user?.emailVerified && (
        <Alert tone="warn">
          Numara alabilmek için önce e-posta adresinizi doğrulamanız gerekiyor.
        </Alert>
      )}

      {/* ─── 1. Servis ─── */}
      <Card>
        <div className="flex items-center gap-2">
          <StepBadge n={1} done={!!service} />
          <h2 className="text-lg font-semibold">Servis seçin</h2>
        </div>

        {catalog.isLoading ? (
          <div className="mt-4 grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-4">
            {Array.from({ length: 8 }, (_, i) => <Skeleton key={i} className="h-16" />)}
          </div>
        ) : catalog.isError ? (
          <Alert className="mt-4">Servis listesi yüklenemedi. Bağlantınızı kontrol edin.</Alert>
        ) : services.length === 0 ? (
          <Empty title="Stokta servis yok" hint="Sağlayıcı stoğu şu anda boş görünüyor." />
        ) : (
          <>
            {services.length > 8 && (
              <div className="mt-4">
                <Field label="Servis ara" value={search} onChange={(e) => setSearch(e.target.value)}
                       placeholder="Örn. WhatsApp" type="search" autoCapitalize="none" />
              </div>
            )}
            <ul className="mt-4 grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-4">
              {filteredServices.map((s) => (
                <li key={s.code}>
                  <button type="button" onClick={() => pickService(s.code)}
                          aria-pressed={service === s.code}
                          className={cx(
                            'flex min-h-16 w-full items-center gap-3',
                            'rounded-xl border px-3 py-2 text-left transition-colors',
                            service === s.code
                              ? 'border-brand-500 bg-brand-500/12'
                              : 'raised hover:border-[var(--color-ink-500)]')}>
                    <ServiceIcon name={s.name} iconUrl={s.iconUrl} size={32} />
                    <span className="flex min-w-0 flex-col">
                      <span className="truncate text-sm font-medium">{s.name}</span>
                      <span className="text-xs text-muted">{s.count} ülke</span>
                    </span>
                  </button>
                </li>
              ))}
            </ul>
            {filteredServices.length === 0 && (
              <p className="mt-4 text-sm text-muted">“{search}” için sonuç bulunamadı.</p>
            )}
          </>
        )}
      </Card>

      {/* ─── 2. Ülke ─── */}
      {service && (
        <Card>
          <div className="flex items-center gap-2">
            <StepBadge n={2} done={!!country} />
            <h2 className="text-lg font-semibold">Ülke seçin</h2>
          </div>
          <ul className="mt-4 grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-3">
            {countries.map((c) => (
              <li key={c.countryIso2}>
                <button type="button" onClick={() => pickCountry(c.countryIso2)}
                        aria-pressed={country === c.countryIso2}
                        className={cx(
                          'flex min-h-12 w-full items-center justify-between gap-2',
                          'rounded-xl border px-3 py-2 text-left transition-colors',
                          country === c.countryIso2
                            ? 'border-brand-500 bg-brand-500/12'
                            : 'raised hover:border-[var(--color-ink-500)]')}>
                  <span className="min-w-0 truncate text-sm font-medium">{c.countryName}</span>
                  <span className="shrink-0 text-xs text-muted">+{c.phoneCode}</span>
                </button>
              </li>
            ))}
          </ul>
        </Card>
      )}

      {/* ─── 3. Fiyat ─── */}
      {country && (
        <Card>
          <div className="flex items-center gap-2">
            <StepBadge n={3} done={quote.isSuccess} />
            <h2 className="text-lg font-semibold">Fiyat</h2>
          </div>
          <QuotePanel
            quote={quote.data ?? null}
            error={quote.error}
            pending={quote.isPending}
            onRefresh={() => service && country && quote.mutate({ service, country })}
          />
        </Card>
      )}
    </div>
  );
}

function StepBadge({ n, done }: { n: number; done: boolean }) {
  return (
    <span className={cx('grid size-6 shrink-0 place-items-center rounded-lg text-xs font-bold',
                        done ? 'bg-[var(--color-ok)]/15 text-[var(--color-ok)]'
                             : 'bg-brand-500/15 text-brand-300')}>
      {done ? '✓' : n}
    </span>
  );
}

function QuotePanel({
  quote, error, pending, onRefresh,
}: { quote: Quote | null; error: unknown; pending: boolean; onRefresh: () => void }) {
  const left = useCountdown(quote?.expiresAt);
  const expired = !!quote && left <= 0;

  if (pending) return <Skeleton className="mt-4 h-28" />;

  if (error) {
    const e = error instanceof ApiError ? error : null;
    // Kullanıcıya ne yapması gerektiğini söyleriz; "hata oluştu" demek yetmez.
    const hint =
      e?.code === 'EMAIL_NOT_VERIFIED' ? 'E-posta adresinizi doğruladıktan sonra tekrar deneyin.'
      : e?.code === 'OUT_OF_STOCK'     ? 'Bu kombinasyon az önce tükendi. Başka bir ülke seçin.'
      : e?.code === 'NO_PRICING_RULE'  ? 'Bu servis için fiyatlandırma tanımlı değil. Destekle iletişime geçin.'
      : null;
    return (
      <Alert className="mt-4">
        <p>{e?.message ?? 'Fiyat alınamadı.'}</p>
        {hint && <p className="mt-1 opacity-80">{hint}</p>}
        {e?.requestId && <p className="mt-2 text-xs opacity-60">İstek no: {e.requestId}</p>}
      </Alert>
    );
  }

  if (!quote) return null;

  return (
    <div className="mt-4 flex flex-col gap-4">
      <div className="raised flex flex-wrap items-end justify-between gap-3 rounded-xl border p-4">
        <div>
          <p className="text-xs font-medium uppercase tracking-wide text-muted">Ödenecek tutar</p>
          <p className="mt-1 text-3xl font-bold">{formatMoney(quote.price)}</p>
        </div>
        <div className="text-right">
          <Badge tone={quote.stock > 0 ? 'ok' : 'bad'}>
            {quote.stock > 0 ? `${quote.stock} numara stokta` : 'Tükendi'}
          </Badge>
          <p className={cx('mt-2 text-sm font-medium',
                           expired ? 'text-[var(--color-bad)]' : 'text-muted')}>
            {expired ? 'Teklif süresi doldu' : `Geçerlilik: ${formatDuration(left)}`}
          </p>
        </div>
      </div>

      <p className="text-xs leading-relaxed text-muted">
        Bu fiyat size özeldir ve teklif süresi boyunca değişmez. Süre dolarsa
        güncel kur ile yeni bir teklif alınır. Kod gelmezse ücret iade edilir.
      </p>

      {expired ? (
        <Button onClick={onRefresh} fullWidth>Yeni fiyat al</Button>
      ) : (
        <>
          {/*
            Sipariş ucu (POST /orders) HENÜZ YOK — M5 kalemidir.
            Buton bilerek devre dışıdır: basılabilir bırakıp 404 aldırmak,
            kullanıcıya parasının gittiğini mi gitmediğini mi bilmediği bir an
            yaşatır. Bilinmeyen durum, açık bir "yakında"dan daha kötüdür.
          */}
          <Button fullWidth disabled>Numarayı al</Button>
          <Alert tone="info">
            Satın alma adımı şu anda geliştirilmektedir. Servis seçimi, ülke
            seçimi ve fiyatlandırma canlı çalışıyor.
          </Alert>
        </>
      )}

      <Link href="/panel/cuzdan"
            className="text-center text-sm text-brand-400 underline-offset-4 hover:underline">
        Bakiyemi görüntüle
      </Link>
    </div>
  );
}
