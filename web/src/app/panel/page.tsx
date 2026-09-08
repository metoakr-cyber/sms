'use client';

import Link from 'next/link';
import { useQuery } from '@tanstack/react-query';
import { apiFetch } from '@/lib/api';
import { formatMoney, formatDateTime } from '@/lib/format';
import { useSession } from '@/hooks/useSession';
import { Card, Button, Badge, Skeleton, Empty } from '@/components/ui';
import type { Statement } from '@/lib/types';

export default function DashboardPage() {
  const { user } = useSession();
  const stmt = useQuery({
    queryKey: ['statement', { limit: 5 }],
    queryFn: () => apiFetch<Statement>('/wallet/entries?limit=5&offset=0'),
  });

  return (
    <div className="mx-auto flex max-w-5xl flex-col gap-5">
      <div>
        <h1 className="text-2xl font-bold tracking-tight md:text-3xl">
          Merhaba, {user?.username}
        </h1>
        <p className="mt-1 text-sm text-muted">Hesabınızın özeti.</p>
      </div>

      <div className="grid gap-3 sm:grid-cols-2 md:gap-4">
        <Card>
          <p className="text-xs font-medium uppercase tracking-wide text-muted">Bakiye</p>
          <p className="mt-1.5 text-3xl font-bold">{formatMoney(user?.balance)}</p>
          <Link href="/panel/cuzdan" className="mt-4 inline-block w-full sm:w-auto">
            <Button variant="outline" size="sm" fullWidth className="sm:w-auto">Bakiye yükle</Button>
          </Link>
        </Card>

        <Card className="flex flex-col justify-between">
          <div>
            <p className="text-xs font-medium uppercase tracking-wide text-muted">Hesap durumu</p>
            <div className="mt-2 flex flex-wrap items-center gap-2">
              {user?.emailVerified
                ? <Badge tone="ok">E-posta doğrulandı</Badge>
                : <Badge tone="warn">E-posta doğrulanmadı</Badge>}
              {user?.permissions.length ? <Badge tone="brand">Yönetici</Badge> : null}
            </div>
          </div>
          <Link href="/panel/numara-al" className="mt-4 w-full sm:w-auto">
            <Button size="sm" fullWidth className="sm:w-auto">Numara al</Button>
          </Link>
        </Card>
      </div>

      <Card>
        <div className="flex items-center justify-between gap-3">
          <h2 className="text-lg font-semibold">Son hareketler</h2>
          <Link href="/panel/cuzdan"
                className="py-1 text-sm text-brand-400 underline-offset-4 hover:underline">
            Tümü
          </Link>
        </div>

        {stmt.isLoading ? (
          <div className="mt-4 flex flex-col gap-2">
            {[0, 1, 2].map((i) => <Skeleton key={i} className="h-14" />)}
          </div>
        ) : !stmt.data?.items.length ? (
          <Empty title="Henüz hareket yok"
                 hint="Bakiye yüklediğinizde ve numara aldığınızda burada görünür." />
        ) : (
          <ul className="mt-4 flex flex-col gap-2">
            {stmt.data.items.map((e) => (
              <li key={e.id}
                  className="raised flex items-center justify-between gap-3 rounded-xl border px-3 py-3">
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium">{e.typeLabel}</p>
                  <p className="mt-0.5 text-xs text-muted">{formatDateTime(e.createdAt)}</p>
                </div>
                <span className={`shrink-0 text-sm font-semibold ${
                  e.amount.minor < 0 ? 'text-[var(--color-bad)]' : 'text-[var(--color-ok)]'}`}>
                  {e.amount.minor > 0 ? '+' : ''}{formatMoney(e.amount)}
                </span>
              </li>
            ))}
          </ul>
        )}
      </Card>
    </div>
  );
}
