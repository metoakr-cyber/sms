'use client';

import * as React from 'react';
import Link from 'next/link';
import { useQuery, useMutation, useQueryClient, keepPreviousData } from '@tanstack/react-query';
import { ApiError, apiFetch } from '@/lib/api';
import { formatMoney, formatDateTime } from '@/lib/format';
import { useSession } from '@/hooks/useSession';
import { Card, Button, Field, Alert, Badge, Skeleton, Empty } from '@/components/ui';
import { Modal } from '@/components/modal';
import type { AdminUser } from '@/lib/types';

const PAGE = 25;

type UserStatus = AdminUser['status'];

/** GET /admin/users — sayfalı zarf (dto.AdminUserListResponse). */
interface AdminUserList {
  items: AdminUser[];
  total: number;
  limit: number;
  offset: number;
}

/** PATCH /admin/users/:id/status YALNIZ {id,status} döner — tam kullanıcı değil. */
interface SetStatusResult {
  id: string;
  status: UserStatus;
}

const STATUS_META: Record<UserStatus, { label: string; tone: 'ok' | 'bad' | 'warn' }> = {
  ACTIVE: { label: 'Aktif', tone: 'ok' },
  SUSPENDED: { label: 'Askıda', tone: 'bad' },
  PENDING_VERIFICATION: { label: 'Doğrulama bekliyor', tone: 'warn' },
};

const STATUS_FILTERS: Array<{ value: '' | UserStatus; label: string }> = [
  { value: '', label: 'Tüm durumlar' },
  { value: 'ACTIVE', label: 'Aktif' },
  { value: 'PENDING_VERIFICATION', label: 'Doğrulama bekliyor' },
  { value: 'SUSPENDED', label: 'Askıda' },
];

/** Hata gösterimi: mesaj + alan hataları + destek için istek numarası. */
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
 * Kullanıcı kimliğini panoya kopyalar.
 *
 * Bakiye düzeltme ekranı kullanıcıyı UUID ile ister ve sorgu parametresi
 * KABUL ETMEZ; yönetici kimliği elle yazmak zorunda kalıyordu. 36 karakterlik
 * bir UUID'yi elle yazmak, yanlış hesaba para yazmanın en kısa yoludur.
 */
function CopyIdButton({ id }: { id: string }) {
  const [copied, setCopied] = React.useState(false);

  React.useEffect(() => {
    if (!copied) return;
    const t = setTimeout(() => setCopied(false), 2000);
    return () => clearTimeout(t);
  }, [copied]);

  return (
    <Button
      variant="outline"
      size="sm"
      onClick={async () => {
        try {
          // Pano API'si güvensiz bağlamda ve izin verilmediğinde reddeder;
          // kopyalanamaması ekranı bozmamalı.
          await navigator.clipboard.writeText(id);
          setCopied(true);
        } catch {
          setCopied(false);
        }
      }}
    >
      {copied ? 'Kopyalandı' : 'Kimliği kopyala'}
    </Button>
  );
}

export default function AdminUsersPage() {
  const qc = useQueryClient();
  const { user: me } = useSession();

  const [searchInput, setSearchInput] = React.useState('');
  const [search, setSearch] = React.useState('');
  const [status, setStatus] = React.useState<'' | UserStatus>('');
  const [offset, setOffset] = React.useState(0);

  // Her tuşa basışta istek atmak yönetim uçlarının dakikalık sınırını
  // (60 istek) tek bir aramada tüketir; arama 350 ms sonra sabitlenir.
  React.useEffect(() => {
    const t = setTimeout(() => {
      setSearch(searchInput.trim());
      setOffset(0);
    }, 350);
    return () => clearTimeout(t);
  }, [searchInput]);

  const q = useQuery({
    queryKey: ['admin-users', { search, status, offset }],
    queryFn: () => {
      const p = new URLSearchParams({ limit: String(PAGE), offset: String(offset) });
      if (search) p.set('q', search);
      if (status) p.set('status', status);
      return apiFetch<AdminUserList>(`/admin/users?${p.toString()}`);
    },
    // Sayfa/arama değişince liste boşalıp zıplamasın.
    placeholderData: keepPreviousData,
  });

  const [confirm, setConfirm] = React.useState<{ user: AdminUser; next: UserStatus } | null>(null);

  const setUserStatus = useMutation({
    mutationFn: (v: { id: string; next: UserStatus }) =>
      apiFetch<SetStatusResult>(`/admin/users/${encodeURIComponent(v.id)}/status`, {
        method: 'PATCH',
        body: { status: v.next },
      }),
    // Yönetsel bir durum değişikliği asla kendiliğinden tekrarlanmaz.
    retry: false,
    onSuccess: () => {
      // Yanıt yalnız {id,status} taşır; satırı yamalamak yerine listeyi tazele.
      qc.invalidateQueries({ queryKey: ['admin-users'] });
      setConfirm(null);
    },
  });

  function askChange(user: AdminUser, next: UserStatus) {
    setUserStatus.reset();
    setConfirm({ user, next });
  }

  const listErr = q.error instanceof ApiError ? q.error : null;
  const mutErr = setUserStatus.error instanceof ApiError ? setUserStatus.error : null;

  const total = q.data?.total ?? 0;
  const hasPrev = offset > 0;
  const hasNext = offset + PAGE < total;
  const items = q.data?.items ?? [];

  return (
    <div className="mx-auto flex max-w-6xl flex-col gap-5">
      <div>
        <h1 className="text-2xl font-bold tracking-tight md:text-3xl">Kullanıcılar</h1>
        <p className="mt-1 text-sm text-muted">
          Hesapları arayın, durumlarını görün ve gerektiğinde erişimi kapatın.
        </p>
      </div>

      <Card>
        <div className="flex flex-col gap-4 md:flex-row md:items-end md:gap-3">
          <div className="md:flex-1">
            <Field
              label="Ara"
              type="search"
              value={searchInput}
              onChange={(e) => setSearchInput(e.target.value)}
              placeholder="E-posta veya kullanıcı adı"
              autoCapitalize="none"
              autoCorrect="off"
              autoComplete="off"
              spellCheck={false}
              hint="Yazmayı bıraktığınızda arama kendiliğinden yapılır."
            />
          </div>

          <label className="flex flex-col gap-1.5 md:w-56">
            <span className="text-sm font-medium">Durum</span>
            <select
              value={status}
              onChange={(e) => {
                setStatus(e.target.value as '' | UserStatus);
                setOffset(0);
              }}
              className="raised min-h-12 w-full rounded-xl border px-3 text-base outline-none
                         focus:border-brand-400 disabled:opacity-60"
            >
              {STATUS_FILTERS.map((s) => (
                <option key={s.value || 'all'} value={s.value}>{s.label}</option>
              ))}
            </select>
          </label>
        </div>
      </Card>

      <Card>
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h2 className="text-lg font-semibold">Hesap listesi</h2>
          {total > 0 && <Badge tone="neutral">{total} kayıt</Badge>}
        </div>

        {q.isLoading ? (
          <div className="mt-4 flex flex-col gap-2">
            {[0, 1, 2, 3, 4].map((i) => <Skeleton key={i} className="h-20" />)}
          </div>
        ) : listErr ? (
          <ErrorBox err={listErr} className="mt-4" />
        ) : !items.length ? (
          <Empty
            title="Kullanıcı bulunamadı"
            hint={search || status
              ? 'Arama veya durum süzgecini değiştirip tekrar deneyin.'
              : 'Henüz kayıtlı kullanıcı yok.'}
          />
        ) : (
          <>
            {/* MOBİL: kart listesi. Yatay kaydırılan tablo kabul edilmez (§2.5). */}
            <ul className="mt-4 flex flex-col gap-2 md:hidden">
              {items.map((u) => {
                const meta = STATUS_META[u.status];
                const isSelf = me?.id === u.id;
                return (
                  <li key={u.id} className="raised rounded-xl border p-3">
                    <div className="flex items-start justify-between gap-3">
                      <div className="min-w-0">
                        <p className="truncate text-sm font-medium">{u.username}</p>
                        <p className="mt-0.5 text-xs text-muted break-anywhere">{u.email}</p>
                      </div>
                      <span className="shrink-0"><Badge tone={meta.tone}>{meta.label}</Badge></span>
                    </div>

                    <dl className="mt-3 grid grid-cols-2 gap-x-3 gap-y-2 border-t
                                   border-[var(--border)] pt-2 text-xs">
                      <div>
                        <dt className="text-muted">Bakiye</dt>
                        <dd className="mt-0.5 font-semibold">{formatMoney(u.balance)}</dd>
                      </div>
                      <div>
                        <dt className="text-muted">Sipariş</dt>
                        <dd className="mt-0.5 font-semibold">{u.orderCount}</dd>
                      </div>
                      <div>
                        <dt className="text-muted">Kayıt</dt>
                        <dd className="mt-0.5">{formatDateTime(u.createdAt)}</dd>
                      </div>
                      <div>
                        <dt className="text-muted">E-posta doğrulama</dt>
                        <dd className="mt-0.5">{u.emailVerified ? 'Yapıldı' : 'Yapılmadı'}</dd>
                      </div>
                    </dl>

                    <div className="mt-2 flex flex-wrap gap-1.5">
                      {u.roles.length
                        ? u.roles.map((r) => <Badge key={r} tone="brand">{r}</Badge>)
                        : <span className="text-xs text-muted">Rol atanmamış</span>}
                    </div>

                    <div className="mt-3 flex flex-wrap gap-2">
                      <StatusActions user={u} isSelf={isSelf} onAsk={askChange} />
                      <CopyIdButton id={u.id} />
                      <Link href="/yonetim/bakiye">
                        <Button variant="outline" size="sm">Bakiye düzelt</Button>
                      </Link>
                    </div>
                    {isSelf && (
                      <p className="mt-2 text-xs text-muted">
                        Bu sizin hesabınız — kendi durumunuzu değiştiremezsiniz.
                      </p>
                    )}
                  </li>
                );
              })}
            </ul>

            {/* MASAÜSTÜ: gerçek tablo */}
            <div className="mt-4 hidden md:block">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-[var(--border)] text-left text-xs
                                 uppercase tracking-wide text-muted">
                    <th scope="col" className="py-2 pr-3 font-medium">Kullanıcı</th>
                    <th scope="col" className="py-2 pr-3 font-medium">Durum</th>
                    <th scope="col" className="py-2 pr-3 text-right font-medium">Bakiye</th>
                    <th scope="col" className="py-2 pr-3 text-right font-medium">Sipariş</th>
                    <th scope="col" className="py-2 pr-3 font-medium">Roller</th>
                    <th scope="col" className="py-2 pr-3 font-medium">Kayıt</th>
                    <th scope="col" className="py-2 text-right font-medium">İşlem</th>
                  </tr>
                </thead>
                <tbody>
                  {items.map((u) => {
                    const meta = STATUS_META[u.status];
                    const isSelf = me?.id === u.id;
                    return (
                      <tr key={u.id} className="border-b border-[var(--border)] last:border-0 align-top">
                        <td className="py-3 pr-3">
                          <p className="font-medium">{u.username}</p>
                          <p className="text-xs text-muted break-anywhere">{u.email}</p>
                          {!u.emailVerified && (
                            <p className="mt-0.5 text-xs text-[var(--color-warn)]">E-posta doğrulanmamış</p>
                          )}
                        </td>
                        <td className="py-3 pr-3"><Badge tone={meta.tone}>{meta.label}</Badge></td>
                        <td className="py-3 pr-3 text-right font-semibold whitespace-nowrap">
                          {formatMoney(u.balance)}
                        </td>
                        <td className="py-3 pr-3 text-right whitespace-nowrap">{u.orderCount}</td>
                        <td className="py-3 pr-3">
                          <div className="flex flex-wrap gap-1">
                            {u.roles.length
                              ? u.roles.map((r) => <Badge key={r} tone="brand">{r}</Badge>)
                              : <span className="text-xs text-muted">—</span>}
                          </div>
                        </td>
                        <td className="py-3 pr-3 whitespace-nowrap text-muted">
                          {formatDateTime(u.createdAt)}
                        </td>
                        <td className="py-3">
                          <div className="flex flex-wrap justify-end gap-2">
                            <StatusActions user={u} isSelf={isSelf} onAsk={askChange} />
                            <CopyIdButton id={u.id} />
                            <Link href="/yonetim/bakiye">
                              <Button variant="outline" size="sm">Bakiye düzelt</Button>
                            </Link>
                          </div>
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
        ONAY DİYALOĞU — tek tıkla durum değiştirilmez.
        Askıya alma kullanıcının açık TÜM oturumlarını anında düşürür ve
        sipariş akışını keser; geri alınabilir ama kullanıcı tarafında
        anında görünür bir kesintidir.
      */}
      <Modal
        open={confirm !== null}
        onClose={() => { setUserStatus.reset(); setConfirm(null); }}
        title={confirm?.next === 'SUSPENDED' ? 'Hesabı askıya al' : 'Hesabı aktifleştir'}
      >
        {confirm && (
          <div className="flex flex-col gap-4">
            <div className="raised rounded-xl border p-3">
              <p className="text-sm font-medium">{confirm.user.username}</p>
              <p className="mt-0.5 text-xs text-muted break-anywhere">{confirm.user.email}</p>
            </div>

            {confirm.next === 'SUSPENDED' ? (
              <Alert tone="warn">
                Askıya alınan hesabın <strong>tüm oturumları anında düşer</strong>. Kullanıcı
                giriş yapamaz, numara alamaz. Bakiyesi silinmez; hesabı yeniden
                aktifleştirdiğinizde kaldığı yerden devam eder.
              </Alert>
            ) : (
              <Alert tone="info">
                Hesap yeniden aktifleştirilecek; kullanıcı giriş yapıp numara alabilecek.
              </Alert>
            )}

            {mutErr && <ErrorBox err={mutErr} />}

            <div className="flex flex-col gap-2 sm:flex-row-reverse">
              <Button
                data-autofocus
                variant={confirm.next === 'SUSPENDED' ? 'danger' : 'primary'}
                loading={setUserStatus.isPending}
                fullWidth
                onClick={() => setUserStatus.mutate({ id: confirm.user.id, next: confirm.next })}
              >
                {confirm.next === 'SUSPENDED' ? 'Evet, askıya al' : 'Evet, aktifleştir'}
              </Button>
              <Button
                variant="outline"
                fullWidth
                disabled={setUserStatus.isPending}
                onClick={() => { setUserStatus.reset(); setConfirm(null); }}
              >
                Vazgeç
              </Button>
            </div>
          </div>
        )}
      </Modal>
    </div>
  );
}

/**
 * Satır eylemleri.
 *
 * Yönetici KENDİ hesabını askıya alamaz (sunucu 422 ile reddeder). Düğmeyi
 * baştan kapatmak, kullanıcıyı yapamayacağı bir işlemin hatasıyla
 * karşılaştırmaktan iyidir.
 */
function StatusActions({
  user, isSelf, onAsk,
}: {
  user: AdminUser;
  isSelf: boolean;
  onAsk: (user: AdminUser, next: UserStatus) => void;
}) {
  if (user.status === 'SUSPENDED') {
    return (
      <Button size="sm" onClick={() => onAsk(user, 'ACTIVE')}>
        Aktifleştir
      </Button>
    );
  }
  return (
    <Button
      variant="danger"
      size="sm"
      disabled={isSelf}
      title={isSelf ? 'Kendi hesabınızın durumunu değiştiremezsiniz.' : undefined}
      onClick={() => onAsk(user, 'SUSPENDED')}
    >
      Askıya al
    </Button>
  );
}
