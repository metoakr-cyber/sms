'use client';

import * as React from 'react';
import Link from 'next/link';
import { useQuery, keepPreviousData } from '@tanstack/react-query';
import { apiFetch } from '@/lib/api';
import { formatMoney, formatDateTime } from '@/lib/format';
import { useSession } from '@/hooks/useSession';
import { Card, Button, Skeleton, Empty, Alert, Badge } from '@/components/ui';
import type { Statement } from '@/lib/types';

const PAGE = 20;

export default function WalletPage() {
  const { user } = useSession();
  const [offset, setOffset] = React.useState(0);

  const q = useQuery({
    queryKey: ['statement', { limit: PAGE, offset }],
    queryFn: () => apiFetch<Statement>(`/wallet/entries?limit=${PAGE}&offset=${offset}`),
    // Sayfa değişince liste boşalıp zıplamasın; eski veri yenisi gelene dek kalır.
    placeholderData: keepPreviousData,
  });

  const total = q.data?.total ?? 0;
  const hasPrev = offset > 0;
  const hasNext = offset + PAGE < total;

  return (
    <div className="mx-auto flex max-w-5xl flex-col gap-5">
      <h1 className="text-2xl font-bold tracking-tight md:text-3xl">Cüzdan</h1>

      {/* Bakiye ve yükleme TEK KARTTA: "param ne kadar" ile "nasıl artırırım"
          arasına başka içerik girmesi, en sık yapılan işi aşağı iter.
          Alt menüye yedinci bir sekme eklemek yerine yükleme buradan açılır —
          320 px'te altı sekme zaten 44 px dokunma hedefinin sınırında. */}
      <Card>
        <p className="text-xs font-medium uppercase tracking-wide text-muted">Kullanılabilir bakiye</p>
        <p className="mt-1.5 text-3xl font-bold md:text-4xl">{formatMoney(user?.balance)}</p>
        <Link href="/panel/bakiye-yukle" className="mt-4 block sm:inline-block">
          <Button fullWidth className="sm:w-auto">Bakiye yükle</Button>
        </Link>
        <p className="mt-3 text-sm leading-relaxed text-muted">
          Banka havalesi/EFT veya USDT ile yükleme yapabilirsiniz. Ödemeniz
          kontrol edildikten sonra bakiyeniz hesabınıza tanımlanır.
        </p>
      </Card>

      <Card>
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h2 className="text-lg font-semibold">Hesap ekstresi</h2>
          {total > 0 && <Badge tone="neutral">{total} kayıt</Badge>}
        </div>

        {q.isLoading ? (
          <div className="mt-4 flex flex-col gap-2">
            {[0, 1, 2, 3].map((i) => <Skeleton key={i} className="h-16" />)}
          </div>
        ) : q.isError ? (
          <Alert className="mt-4">Ekstre yüklenemedi. Bağlantınızı kontrol edip tekrar deneyin.</Alert>
        ) : !q.data?.items.length ? (
          <Empty title="Ekstre boş" hint="İlk işleminizden sonra burada görünecek." />
        ) : (
          <>
            {/* MOBİL: kart listesi. Yatay kaydırılan tablo kabul edilmez (§2.5). */}
            <ul className="mt-4 flex flex-col gap-2 md:hidden">
              {q.data.items.map((e) => (
                <li key={e.id} className="raised rounded-xl border p-3">
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                      <p className="text-sm font-medium">{e.typeLabel}</p>
                      <p className="mt-0.5 text-xs text-muted">{formatDateTime(e.createdAt)}</p>
                    </div>
                    <span className={`shrink-0 text-sm font-semibold ${
                      e.amount.minor < 0 ? 'text-[var(--color-bad)]' : 'text-[var(--color-ok)]'}`}>
                      {e.amount.minor > 0 ? '+' : ''}{formatMoney(e.amount)}
                    </span>
                  </div>
                  <div className="mt-2 flex items-center justify-between gap-3 border-t
                                  border-[var(--border)] pt-2 text-xs text-muted">
                    <span>Sonraki bakiye</span>
                    <span className="font-medium text-[var(--text)]">{formatMoney(e.balanceAfter)}</span>
                  </div>
                  {e.note && <p className="mt-2 text-xs text-muted break-anywhere">{e.note}</p>}
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
                    <th scope="col" className="py-2 pr-3 font-medium">İşlem</th>
                    <th scope="col" className="py-2 pr-3 font-medium">Açıklama</th>
                    <th scope="col" className="py-2 pr-3 text-right font-medium">Tutar</th>
                    <th scope="col" className="py-2 text-right font-medium">Bakiye</th>
                  </tr>
                </thead>
                <tbody>
                  {q.data.items.map((e) => (
                    <tr key={e.id} className="border-b border-[var(--border)] last:border-0">
                      <td className="py-3 pr-3 whitespace-nowrap text-muted">{formatDateTime(e.createdAt)}</td>
                      <td className="py-3 pr-3 font-medium">{e.typeLabel}</td>
                      <td className="py-3 pr-3 text-muted break-anywhere">{e.note ?? '—'}</td>
                      <td className={`py-3 pr-3 text-right font-semibold whitespace-nowrap ${
                        e.amount.minor < 0 ? 'text-[var(--color-bad)]' : 'text-[var(--color-ok)]'}`}>
                        {e.amount.minor > 0 ? '+' : ''}{formatMoney(e.amount)}
                      </td>
                      <td className="py-3 text-right whitespace-nowrap">{formatMoney(e.balanceAfter)}</td>
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
    </div>
  );
}
