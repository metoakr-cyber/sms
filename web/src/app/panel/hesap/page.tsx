'use client';

import * as React from 'react';
import Link from 'next/link';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { apiFetch, ApiError } from '@/lib/api';
import { formatDateTime } from '@/lib/format';
import { useSession } from '@/hooks/useSession';
import { Card, Button, Badge, Alert, Skeleton } from '@/components/ui';
import type { Session } from '@/lib/types';

export default function AccountPage() {
  const { user } = useSession();
  const qc = useQueryClient();
  const [note, setNote] = React.useState<{ tone: 'ok' | 'bad'; text: string } | null>(null);

  const sessions = useQuery({
    queryKey: ['sessions'],
    queryFn: () => apiFetch<{ items: Session[] }>('/me/sessions'),
  });

  const revoke = useMutation({
    mutationFn: (id: string) => apiFetch(`/me/sessions/${encodeURIComponent(id)}`, { method: 'DELETE' }),
    onSuccess: () => {
      setNote({ tone: 'ok', text: 'Oturum kapatıldı.' });
      qc.invalidateQueries({ queryKey: ['sessions'] });
    },
    onError: (e) => setNote({
      tone: 'bad',
      text: e instanceof ApiError ? e.message : 'Oturum kapatılamadı.',
    }),
  });

  return (
    <div className="mx-auto flex max-w-3xl flex-col gap-5">
      <h1 className="text-2xl font-bold tracking-tight md:text-3xl">Hesabım</h1>

      <Card>
        <h2 className="text-lg font-semibold">Bilgiler</h2>
        <dl className="mt-4 flex flex-col gap-3">
          <Row label="Kullanıcı adı" value={user?.username} />
          <Row label="E-posta" value={user?.email} />
          <Row label="Üyelik" value={formatDateTime(user?.createdAt)} />
          <div className="flex flex-wrap items-center justify-between gap-2 border-t
                          border-[var(--border)] pt-3">
            <dt className="text-sm text-muted">Doğrulama</dt>
            <dd>
              {user?.emailVerified
                ? <Badge tone="ok">E-posta doğrulandı</Badge>
                : <Badge tone="warn">Doğrulanmadı</Badge>}
            </dd>
          </div>
        </dl>
      </Card>

      {/* Yorumlarım ALT MENÜYE eklenmedi: orada zaten 6 sekme var ve
          320 px'te yedincisi her sekmeyi 44 px dokunma hedefinin altına
          düşürürdü. Yorum yazmak sık yapılan bir iş değil; doğal yeri
          hesap sayfası. */}
      <Card>
        <h2 className="text-lg font-semibold">Yorumlarım</h2>
        <p className="mt-1.5 text-sm leading-relaxed text-muted">
          Hizmet hakkındaki görüşünüzü yazabilirsiniz. Yorumunuz yönetici
          onayından sonra sitede yayımlanır.
        </p>
        <Link href="/panel/yorumlarim" className="mt-4 block sm:inline-block">
          <Button variant="outline" fullWidth className="sm:w-auto">Yorumlarımı aç</Button>
        </Link>
      </Card>

      <Card>
        <h2 className="text-lg font-semibold">Açık oturumlar</h2>
        <p className="mt-1.5 text-sm text-muted">
          Tanımadığınız bir cihaz görürseniz oturumu kapatın ve şifrenizi değiştirin.
        </p>

        {note && <Alert tone={note.tone} className="mt-4">{note.text}</Alert>}

        {sessions.isLoading ? (
          <div className="mt-4 flex flex-col gap-2">
            {[0, 1].map((i) => <Skeleton key={i} className="h-20" />)}
          </div>
        ) : sessions.isError ? (
          <Alert className="mt-4">Oturum listesi yüklenemedi.</Alert>
        ) : (
          <ul className="mt-4 flex flex-col gap-2">
            {(sessions.data?.items ?? []).map((s) => (
              <li key={s.id} className="raised rounded-xl border p-3">
                <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <p className="text-sm font-medium break-anywhere">
                        {s.userAgent || 'Bilinmeyen cihaz'}
                      </p>
                      {s.current && <Badge tone="brand">Bu cihaz</Badge>}
                    </div>
                    <p className="mt-1 text-xs text-muted">
                      Son görülme: {formatDateTime(s.lastSeenAt)}
                      {s.ip ? ` · ${s.ip}` : ''}
                    </p>
                  </div>
                  {!s.current && (
                    <Button variant="outline" size="sm"
                            loading={revoke.isPending && revoke.variables === s.id}
                            onClick={() => revoke.mutate(s.id)}
                            className="w-full sm:w-auto">
                      Kapat
                    </Button>
                  )}
                </div>
              </li>
            ))}
          </ul>
        )}
      </Card>
    </div>
  );
}

function Row({ label, value }: { label: string; value?: string | null }) {
  return (
    <div className="flex flex-wrap items-center justify-between gap-2 border-b
                    border-[var(--border)] pb-3 last:border-0">
      <dt className="text-sm text-muted">{label}</dt>
      <dd className="text-sm font-medium break-anywhere">{value ?? '—'}</dd>
    </div>
  );
}
