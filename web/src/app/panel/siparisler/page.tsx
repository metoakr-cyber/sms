'use client';

import * as React from 'react';
import { useQuery, useQueryClient, keepPreviousData } from '@tanstack/react-query';
import { ApiError, apiFetch } from '@/lib/api';
import { formatMoney, formatDateTime } from '@/lib/format';
import { Card, Button, Badge, Skeleton, Empty, Alert } from '@/components/ui';
import { Modal } from '@/components/modal';
import { CodeWaiter } from '@/components/code-waiter';
import { ServiceIcon } from '@/components/service-icon';
import type { Order, OrderList } from '@/lib/types';

/**
 * Sipariş geçmişi — `GET /orders`.
 *
 * Bu ekran olmadan kullanıcı, numara alma modalını kapattığı anda siparişini
 * bir daha bulamıyordu: kod modal kapandıktan sonra geldiyse ulaşılamaz
 * oluyordu. Buradan sipariş yeniden açılır ve canlı akış kaldığı yerden devam
 * eder (CodeWaiter açılışta REST ile senkron olur).
 */

const PAGE = 20;

type Tone = 'neutral' | 'ok' | 'warn' | 'bad' | 'brand';

/**
 * Sunucudaki `db.OrderStatus` değerlerinin kullanıcıya görünen karşılığı
 * (api/internal/db/models.go:194). Sunucu bir gün yeni bir durum eklerse
 * `fallback` devreye girer: bilinmeyen bir kod ham hâliyle ekrana basılmaz.
 */
const STATUS: Record<string, { label: string; tone: Tone }> = {
  PENDING: { label: 'Kod bekleniyor', tone: 'warn' },
  COMPLETED: { label: 'Tamamlandı', tone: 'ok' },
  CANCELLED: { label: 'İptal edildi', tone: 'neutral' },
  REFUNDED: { label: 'İade edildi', tone: 'neutral' },
  FAILED: { label: 'Başarısız', tone: 'bad' },
};

function statusView(code: string): { label: string; tone: Tone } {
  return STATUS[code] ?? { label: 'Bilinmeyen durum', tone: 'neutral' };
}

/**
 * Detayı açılabilen durumlar.
 *
 * FAILED DIŞARIDA. CodeWaiter yalnız COMPLETED / CANCELLED / REFUNDED durumunu
 * "bitmiş" kabul eder (code-waiter.tsx:35); FAILED bir sipariş için hâlâ geri
 * sayım paneli ve "iptal et ve iade al" düğmesi çizerdi — parası zaten iade
 * edilmiş bir sipariş için kullanıcıya ikinci bir iade sözü vermek olurdu.
 * Doğru düzeltme CodeWaiter'ın terminal kümesine FAILED'ı eklemektir; o dosya
 * bu dalgada başka bir ajana ait (raporda bildirildi).
 */
const OPENABLE = new Set(['PENDING', 'COMPLETED', 'CANCELLED', 'REFUNDED']);

function actionLabel(status: string): string {
  if (status === 'PENDING') return 'Kodu bekle';
  if (status === 'COMPLETED') return 'Kodu gör';
  return 'Detay';
}

export default function OrdersPage() {
  const qc = useQueryClient();
  const [offset, setOffset] = React.useState(0);
  const [open, setOpen] = React.useState<Order | null>(null);

  // Modal içinde durum değişmiş olabilir: kod geldi, sipariş iptal edildi,
  // süre doldu. Kapanışta tazelemezsek liste hâlâ "Kod bekleniyor" gösterir.
  const closeDetail = React.useCallback(() => {
    setOpen(null);
    qc.invalidateQueries({ queryKey: ['orders'] });
  }, [qc]);

  const q = useQuery({
    queryKey: ['orders', { limit: PAGE, offset }],
    queryFn: () => apiFetch<OrderList>(`/orders?limit=${PAGE}&offset=${offset}`),
    // Sayfa değişince liste boşalıp zıplamasın; eski veri yenisi gelene dek kalır.
    placeholderData: keepPreviousData,
  });

  const err = q.error instanceof ApiError ? q.error : null;
  const total = q.data?.total ?? 0;
  const hasPrev = offset > 0;
  const hasNext = offset + PAGE < total;

  return (
    <div className="mx-auto flex max-w-5xl flex-col gap-5">
      <div>
        <h1 className="text-2xl font-bold tracking-tight md:text-3xl">Siparişlerim</h1>
        <p className="mt-1 text-sm text-muted">Aldığınız numaralar ve gelen kodlar.</p>
      </div>

      <Card>
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h2 className="text-lg font-semibold">Sipariş geçmişi</h2>
          {total > 0 && <Badge tone="neutral">{total} sipariş</Badge>}
        </div>

        {q.isLoading ? (
          <div className="mt-4 flex flex-col gap-2">
            {[0, 1, 2, 3].map((i) => <Skeleton key={i} className="h-20" />)}
          </div>
        ) : q.isError ? (
          <Alert className="mt-4">
            {err?.message ?? 'Siparişleriniz yüklenemedi. Bağlantınızı kontrol edip tekrar deneyin.'}
            {err?.requestId && (
              <span className="mt-2 block text-xs opacity-70">İstek no: {err.requestId}</span>
            )}
            <span className="mt-3 block">
              <Button variant="outline" size="sm" onClick={() => q.refetch()}>Tekrar dene</Button>
            </span>
          </Alert>
        ) : !q.data?.items.length ? (
          <Empty title="Henüz siparişiniz yok"
                 hint="Numara aldığınızda siparişleriniz burada listelenir." />
        ) : (
          <>
            {/* KART LİSTESİ — lg'ye KADAR.
                Kırılım `md:` DEĞİL `lg:`: 768 px'te panel kenar çubuğu (240 px)
                açılıyor ve tabloya kalan genişlik ~432 px'e düşüyor; altı sütun
                oraya sığmayıp sayfayı yatay kaydırıyordu (ölçüldü: scrollWidth
                918 > 768). Yatay kaydırılan tablo kabul edilmez (§2.5). */}
            <ul className="mt-4 flex flex-col gap-2 lg:hidden">
              {q.data.items.map((o) => {
                const st = statusView(o.status);
                return (
                  <li key={o.id} className="raised rounded-xl border p-3">
                    <div className="flex items-start justify-between gap-3">
                      <div className="flex min-w-0 items-center gap-2.5">
                        <ServiceIcon name={o.serviceName} size={32} />
                        <div className="min-w-0">
                          <p className="truncate text-sm font-medium">{o.serviceName}</p>
                          <p className="mt-0.5 truncate text-xs text-muted">{o.countryName}</p>
                        </div>
                      </div>
                      <Badge tone={st.tone}>{st.label}</Badge>
                    </div>

                    <p className="mt-3 select-text break-anywhere text-base font-semibold">
                      {o.phoneNumber}
                    </p>

                    <div className="mt-2 flex items-center justify-between gap-3 border-t
                                    border-[var(--border)] pt-2 text-xs text-muted">
                      <span>{formatDateTime(o.createdAt)}</span>
                      <span className="font-medium text-[var(--text)]">{formatMoney(o.price)}</span>
                    </div>

                    {OPENABLE.has(o.status) && (
                      <Button
                        variant={o.status === 'PENDING' ? 'primary' : 'outline'}
                        size="sm" fullWidth className="mt-3"
                        onClick={() => setOpen(o)}
                      >
                        {actionLabel(o.status)}
                      </Button>
                    )}
                  </li>
                );
              })}
            </ul>

            {/* MASAÜSTÜ: gerçek tablo */}
            <div className="mt-4 hidden lg:block">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-[var(--border)] text-left text-xs
                                 uppercase tracking-wide text-muted">
                    <th scope="col" className="py-2 pr-3 font-medium">Tarih</th>
                    {/* Ülke ayrı sütun DEĞİL: 768 px'te yedi sütunun en dar
                        hâli bile kart genişliğini aşıyor ve tablo sayfayı
                        yatay kaydırıyordu. Servis hücresinin altına alındı —
                        mobil kart sunumu da aynı şekilde gösteriyor. */}
                    <th scope="col" className="py-2 pr-3 font-medium">Servis</th>
                    <th scope="col" className="py-2 pr-3 font-medium">Numara</th>
                    <th scope="col" className="py-2 pr-3 font-medium">Durum</th>
                    <th scope="col" className="py-2 pr-3 text-right font-medium">Tutar</th>
                    <th scope="col" className="py-2 text-right font-medium">
                      <span className="sr-only">İşlem</span>
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {q.data.items.map((o) => {
                    const st = statusView(o.status);
                    return (
                      <tr key={o.id} className="border-b border-[var(--border)] last:border-0">
                        <td className="py-3 pr-3 whitespace-nowrap text-muted">
                          {formatDateTime(o.createdAt)}
                        </td>
                        <td className="py-3 pr-3">
                          <span className="flex items-center gap-2">
                            <ServiceIcon name={o.serviceName} size={24} />
                            <span className="min-w-0">
                              <span className="block font-medium">{o.serviceName}</span>
                              <span className="block text-xs text-muted">{o.countryName}</span>
                            </span>
                          </span>
                        </td>
                        <td className="py-3 pr-3 select-text whitespace-nowrap font-medium">
                          {o.phoneNumber}
                        </td>
                        <td className="py-3 pr-3"><Badge tone={st.tone}>{st.label}</Badge></td>
                        <td className="py-3 pr-3 text-right whitespace-nowrap">
                          {formatMoney(o.price)}
                        </td>
                        <td className="py-3 text-right">
                          {OPENABLE.has(o.status) && (
                            <Button
                              variant={o.status === 'PENDING' ? 'primary' : 'outline'}
                              size="sm"
                              onClick={() => setOpen(o)}
                            >
                              {actionLabel(o.status)}
                            </Button>
                          )}
                        </td>
                      </tr>
                    );
                  })}
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

      {/*
        Detay, numara alma ekranındaki KOD BEKLEME bileşeninin ta kendisidir.
        Ayrı bir sayfaya yönlendirmek kullanıcıyı tanımadığı ikinci bir ekrana
        atardı; aynı bileşen, aynı geri sayım, aynı iptal düğmesi.

        `key`: modal kapanıp başka bir siparişle açıldığında CodeWaiter'ın
        akışını sıfırlar — aksi hâlde önceki siparişin durumu bir an görünür.
      */}
      <Modal open={!!open} onClose={closeDetail} title={open?.serviceName ?? ''}>
        {open && <CodeWaiter key={open.id} order={open} onClose={closeDetail} />}
      </Modal>
    </div>
  );
}
