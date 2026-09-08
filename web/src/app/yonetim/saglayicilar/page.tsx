'use client';

import * as React from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { ApiError, apiFetch } from '@/lib/api';
import { formatMoney, formatDateTime } from '@/lib/format';
import { Card, Button, Field, Alert, Badge, Skeleton, Empty } from '@/components/ui';
import { Modal } from '@/components/modal';
import type { AdminProvider } from '@/lib/types';

/** GET /admin/providers — sayfalama YOK, yalnız {items}. */
interface AdminProviderList {
  items: AdminProvider[];
}

/** POST /admin/providers/:id/sync yanıtı. */
interface ProviderSyncResult {
  providerId: string;
  started: boolean;
  sync: { running: boolean; startedAt?: string; finishedAt?: string; failed?: boolean };
}

/** PUT /admin/providers/:id/api-key — anahtarın KENDİSİ değil, maskeli önizleme. */
interface SetApiKeyResult {
  masked: string;
}

/** Sunucu tarafındaki provider_protocol enum'u (dto.ProviderProtocols). */
const PROTOCOLS = ['FAKE', 'HEROSMS_V1', 'FIVE_SIM'] as const;

const CAPABILITIES: Array<{ value: string; label: string }> = [
  { value: 'SMS_ACTIVATION', label: 'Tek kullanımlık numara (aktivasyon)' },
  { value: 'SMS_RENTAL', label: 'Kiralık numara' },
];

const CAP_LABELS: Record<string, string> = {
  SMS_ACTIVATION: 'Aktivasyon',
  SMS_RENTAL: 'Kiralama',
};

const QUERY_KEY = ['admin-providers'] as const;

const SELECT_CLASS =
  'raised min-h-12 w-full rounded-xl border px-3 text-base outline-none ' +
  'focus:border-brand-400 disabled:opacity-60';

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

export default function AdminProvidersPage() {
  const qc = useQueryClient();

  const q = useQuery({
    queryKey: QUERY_KEY,
    queryFn: () => apiFetch<AdminProviderList>('/admin/providers'),
    /*
     * Senkron durumu AYRI BİR UÇTAN OKUNMAZ: tetikleme ucunu yoklamak her
     * seferinde yeni bir tur başlatır. Durum bu listenin `sync` alanındadır,
     * bu yüzden yalnız bir senkron KOŞARKEN liste tazelenir. Boşta yoklama
     * yapılmaz — yönetim uçları dakikada 60 istekle sınırlıdır.
     */
    refetchInterval: (query) =>
      query.state.data?.items.some((p) => p.sync?.running) ? 5_000 : false,
  });

  const [editing, setEditing] = React.useState<AdminProvider | null>(null);
  const [keying, setKeying] = React.useState<AdminProvider | null>(null);
  const [creating, setCreating] = React.useState(false);

  const sync = useMutation({
    mutationFn: (id: string) =>
      apiFetch<ProviderSyncResult>(`/admin/providers/${encodeURIComponent(id)}/sync`, {
        method: 'POST',
      }),
    // Senkron tetikleme idempotent değildir; otomatik tekrar yeni tur başlatır.
    retry: false,
    onSuccess: () => qc.invalidateQueries({ queryKey: QUERY_KEY }),
  });

  const listErr = q.error instanceof ApiError ? q.error : null;
  const syncErr = sync.error instanceof ApiError ? sync.error : null;
  const items = q.data?.items ?? [];

  return (
    <div className="mx-auto flex max-w-6xl flex-col gap-5">
      <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
        <div>
          <h1 className="text-2xl font-bold tracking-tight md:text-3xl">Sağlayıcılar</h1>
          <p className="mt-1 text-sm text-muted">
            Numara aldığımız üst sağlayıcılar, ayarları ve sağlayıcıdaki bakiyemiz.
          </p>
        </div>
        <Button onClick={() => setCreating(true)}>Sağlayıcı ekle</Button>
      </div>

      <Alert tone="info">
        Bir sağlayıcının ayarlarını kaydetmek API anahtarını değiştirmez; anahtar
        ayrı bir formdan yazılır ve hiçbir ekranda geri gösterilmez.
      </Alert>

      {syncErr && <ErrorBox err={syncErr} />}

      <Card>
        <h2 className="text-lg font-semibold">Tanımlı sağlayıcılar</h2>

        {q.isLoading ? (
          <div className="mt-4 flex flex-col gap-2">
            {[0, 1, 2].map((i) => <Skeleton key={i} className="h-24" />)}
          </div>
        ) : listErr ? (
          <ErrorBox err={listErr} className="mt-4" />
        ) : !items.length ? (
          <Empty
            title="Henüz sağlayıcı yok"
            hint="Sağlayıcı ekleyip API anahtarını tanımladıktan sonra etkinleştirebilirsiniz."
          />
        ) : (
          <>
            {/* MOBİL: kart listesi. Yatay kaydırılan tablo kabul edilmez (§2.5). */}
            <ul className="mt-4 flex flex-col gap-2 md:hidden">
              {items.map((p) => (
                <li key={p.id} className="raised rounded-xl border p-3">
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                      <p className="truncate text-sm font-medium">{p.name}</p>
                      <p className="mt-0.5 text-xs text-muted">{p.protocol}</p>
                    </div>
                    <span className="shrink-0">
                      <Badge tone={p.isActive ? 'ok' : 'neutral'}>
                        {p.isActive ? 'Aktif' : 'Pasif'}
                      </Badge>
                    </span>
                  </div>

                  <p className="mt-2 text-xs text-muted break-anywhere">{p.baseUrl || '—'}</p>

                  <dl className="mt-3 grid grid-cols-2 gap-x-3 gap-y-2 border-t
                                 border-[var(--border)] pt-2 text-xs">
                    <div>
                      <dt className="text-muted">Sağlayıcıdaki bakiyemiz</dt>
                      <dd className="mt-0.5 font-semibold">{formatMoney(p.balance)}</dd>
                    </div>
                    <div>
                      <dt className="text-muted">Öncelik</dt>
                      <dd className="mt-0.5 font-semibold">{p.priority}</dd>
                    </div>
                    <div>
                      <dt className="text-muted">Maliyet çarpanı</dt>
                      <dd className="mt-0.5 font-semibold">{p.costMultiplier}</dd>
                    </div>
                    <div>
                      <dt className="text-muted">API anahtarı</dt>
                      <dd className="mt-0.5">
                        <Badge tone={p.hasApiKey ? 'ok' : 'bad'}>
                          {p.hasApiKey ? 'Anahtar kurulu' : 'Anahtar kurulu değil'}
                        </Badge>
                      </dd>
                    </div>
                  </dl>

                  <div className="mt-2 flex flex-wrap gap-1.5">
                    {p.capabilities.length
                      ? p.capabilities.map((c) => (
                          <Badge key={c} tone="brand">{CAP_LABELS[c] ?? c}</Badge>
                        ))
                      : <span className="text-xs text-muted">Yetenek tanımsız</span>}
                  </div>

                  <div className="mt-2"><SyncStatus sync={p.sync} /></div>

                  <div className="mt-3 flex flex-wrap gap-2">
                    <Button variant="outline" size="sm" onClick={() => setEditing(p)}>Ayarlar</Button>
                    <Button variant="outline" size="sm" onClick={() => setKeying(p)}>API anahtarı</Button>
                    <Button
                      variant="outline" size="sm"
                      disabled={p.sync?.running || (sync.isPending && sync.variables === p.id)}
                      loading={sync.isPending && sync.variables === p.id}
                      onClick={() => sync.mutate(p.id)}
                    >
                      Katalogu senkronla
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
                    <th scope="col" className="py-2 pr-3 font-medium">Sağlayıcı</th>
                    <th scope="col" className="py-2 pr-3 font-medium">Durum</th>
                    <th scope="col" className="py-2 pr-3 text-right font-medium">Öncelik</th>
                    <th scope="col" className="py-2 pr-3 text-right font-medium">Çarpan</th>
                    <th scope="col" className="py-2 pr-3 font-medium">Yetenekler</th>
                    <th scope="col" className="py-2 pr-3 text-right font-medium">Bakiyemiz</th>
                    <th scope="col" className="py-2 pr-3 font-medium">Anahtar</th>
                    <th scope="col" className="py-2 text-right font-medium">İşlem</th>
                  </tr>
                </thead>
                <tbody>
                  {items.map((p) => (
                    <tr key={p.id} className="border-b border-[var(--border)] last:border-0 align-top">
                      <td className="py-3 pr-3">
                        <p className="font-medium">{p.name}</p>
                        <p className="text-xs text-muted">{p.protocol}</p>
                        <p className="text-xs text-muted break-anywhere">{p.baseUrl || '—'}</p>
                      </td>
                      <td className="py-3 pr-3">
                        <Badge tone={p.isActive ? 'ok' : 'neutral'}>
                          {p.isActive ? 'Aktif' : 'Pasif'}
                        </Badge>
                        <div className="mt-1"><SyncStatus sync={p.sync} /></div>
                      </td>
                      <td className="py-3 pr-3 text-right whitespace-nowrap">{p.priority}</td>
                      <td className="py-3 pr-3 text-right whitespace-nowrap">{p.costMultiplier}</td>
                      <td className="py-3 pr-3">
                        <div className="flex flex-wrap gap-1">
                          {p.capabilities.length
                            ? p.capabilities.map((c) => (
                                <Badge key={c} tone="brand">{CAP_LABELS[c] ?? c}</Badge>
                              ))
                            : <span className="text-xs text-muted">—</span>}
                        </div>
                      </td>
                      <td className="py-3 pr-3 text-right font-semibold whitespace-nowrap">
                        {formatMoney(p.balance)}
                      </td>
                      <td className="py-3 pr-3">
                        <Badge tone={p.hasApiKey ? 'ok' : 'bad'}>
                          {p.hasApiKey ? 'Kurulu' : 'Kurulu değil'}
                        </Badge>
                      </td>
                      <td className="py-3">
                        <div className="flex flex-wrap justify-end gap-2">
                          <Button variant="outline" size="sm" onClick={() => setEditing(p)}>Ayarlar</Button>
                          <Button variant="outline" size="sm" onClick={() => setKeying(p)}>API anahtarı</Button>
                          <Button
                            variant="outline" size="sm"
                            disabled={p.sync?.running || (sync.isPending && sync.variables === p.id)}
                            loading={sync.isPending && sync.variables === p.id}
                            onClick={() => sync.mutate(p.id)}
                          >
                            Senkronla
                          </Button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </>
        )}
      </Card>

      {/* Modal içerikleri, kapanınca durumları da gitsin diye koşullu monte edilir. */}
      <Modal open={editing !== null} onClose={() => setEditing(null)} title="Sağlayıcı ayarları">
        {editing && (
          <ProviderSettingsForm
            provider={editing}
            onDone={() => setEditing(null)}
          />
        )}
      </Modal>

      <Modal open={keying !== null} onClose={() => setKeying(null)} title="API anahtarı">
        {keying && <ApiKeyForm provider={keying} onDone={() => setKeying(null)} />}
      </Modal>

      <Modal open={creating} onClose={() => setCreating(false)} title="Sağlayıcı ekle">
        {creating && <CreateProviderForm onDone={() => setCreating(false)} />}
      </Modal>
    </div>
  );
}

/** Senkron durumu — listedeki `sync` alanından okunur, ayrı uç yoktur. */
function SyncStatus({ sync }: { sync: AdminProvider['sync'] }) {
  if (!sync) return null;
  if (sync.running) {
    return (
      <span className="inline-flex flex-col gap-0.5">
        <Badge tone="warn">Senkron çalışıyor</Badge>
        {sync.startedAt && (
          <span className="text-xs text-muted">Başlangıç: {formatDateTime(sync.startedAt)}</span>
        )}
      </span>
    );
  }
  if (sync.failed) {
    return (
      <span className="inline-flex flex-col gap-0.5">
        <Badge tone="bad">Son senkron başarısız</Badge>
        {sync.finishedAt && (
          <span className="text-xs text-muted">{formatDateTime(sync.finishedAt)}</span>
        )}
      </span>
    );
  }
  if (sync.finishedAt) {
    return <span className="text-xs text-muted">Son senkron: {formatDateTime(sync.finishedAt)}</span>;
  }
  return null;
}

/**
 * Sağlayıcı ayarları.
 *
 * 🔴 PATCH /admin/providers/:id KISMİ DEĞİLDİR: gövdedeki dört alan
 * (baseUrl, isActive, priority, costMultiplier) koşulsuz yazılır. Yalnız
 * önceliği değiştirmek için `{priority}` göndermek baseUrl'ü boşaltır ve
 * isActive'i false yapar — sağlayıcı sessizce ölür, sipariş akışı durur.
 * Bu yüzden form GET'ten gelen MEVCUT DEĞERLERLE doldurulur ve her kaydetmede
 * dördü birden gönderilir.
 *
 * `costMultiplier` sunucudan STRING gelir ve string gider; Number() ile
 * çevirip geri yazmak çarpanı yuvarlar ve çarpan doğrudan satış fiyatına girer.
 */
function ProviderSettingsForm({
  provider, onDone,
}: { provider: AdminProvider; onDone: () => void }) {
  const qc = useQueryClient();
  const [baseUrl, setBaseUrl] = React.useState(provider.baseUrl);
  const [isActive, setIsActive] = React.useState(provider.isActive);
  const [priority, setPriority] = React.useState(String(provider.priority));
  const [costMultiplier, setCostMultiplier] = React.useState(provider.costMultiplier);
  const [errors, setErrors] = React.useState<Record<string, string>>({});

  const save = useMutation({
    mutationFn: (body: {
      baseUrl: string; isActive: boolean; priority: number; costMultiplier: string;
    }) =>
      apiFetch<{ id: string; name: string; isActive: boolean }>(
        `/admin/providers/${encodeURIComponent(provider.id)}`,
        { method: 'PATCH', body },
      ),
    retry: false,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEY });
      onDone();
    },
  });

  function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    const errs: Record<string, string> = {};

    const url = baseUrl.trim();
    if (url && !/^https?:\/\//i.test(url)) {
      errs.baseUrl = 'Adres http:// veya https:// ile başlamalıdır.';
    }
    const prio = Number(priority.trim());
    if (!/^\d+$/.test(priority.trim()) || !Number.isInteger(prio) || prio < 0 || prio > 10000) {
      errs.priority = 'Öncelik 0-10000 arasında bir tam sayı olmalıdır.';
    }
    const mult = costMultiplier.trim();
    if (!/^\d+(\.\d+)?$/.test(mult)) {
      errs.costMultiplier = 'Çarpan ondalık bir sayı olmalıdır (örn. 1.25).';
    }

    setErrors(errs);
    if (Object.keys(errs).length) return;

    // TAM GÖVDE: dördü birden gönderilir, eksik alan sunucuda sıfırlanır.
    save.mutate({ baseUrl: url, isActive, priority: prio, costMultiplier: mult });
  }

  const err = save.error instanceof ApiError ? save.error : null;
  const fieldErrs = { ...errors, ...(err?.fieldMap() ?? {}) };

  return (
    <form onSubmit={onSubmit} className="flex flex-col gap-4" noValidate>
      <div className="raised rounded-xl border p-3">
        <p className="text-sm font-medium">{provider.name}</p>
        <p className="mt-0.5 text-xs text-muted">{provider.protocol}</p>
      </div>

      <Alert tone="warn">
        Bu form sağlayıcının <strong>tüm ayarlarını</strong> birlikte kaydeder.
        Alanları olduğu gibi bırakırsanız değişmez; boşaltırsanız o değer silinir.
      </Alert>

      <Field
        data-autofocus
        label="Adres (baseUrl)"
        value={baseUrl}
        onChange={(e) => setBaseUrl(e.target.value)}
        placeholder="https://api.ornek.com"
        inputMode="url"
        autoCapitalize="none"
        autoCorrect="off"
        autoComplete="off"
        spellCheck={false}
        error={fieldErrs.baseUrl}
      />

      <Field
        label="Öncelik"
        value={priority}
        onChange={(e) => setPriority(e.target.value)}
        inputMode="numeric"
        autoComplete="off"
        error={fieldErrs.priority}
        hint="0-10000. Küçük değer önce denenir."
      />

      <Field
        label="Maliyet çarpanı"
        value={costMultiplier}
        onChange={(e) => setCostMultiplier(e.target.value)}
        inputMode="decimal"
        autoComplete="off"
        error={fieldErrs.costMultiplier}
        hint="Sağlayıcı maliyeti bu çarpanla düzeltilir (örn. 1.00)."
      />

      <label className="flex items-start gap-3 py-1">
        <input
          type="checkbox"
          checked={isActive}
          onChange={(e) => setIsActive(e.target.checked)}
          className="mt-0.5 size-5 shrink-0 rounded accent-[var(--color-brand-500)]"
        />
        <span className="text-sm">
          Sağlayıcı aktif
          <span className="mt-0.5 block text-xs text-muted">
            Pasif sağlayıcı teklif ve satın alma yolunda hiç denenmez.
          </span>
        </span>
      </label>

      {!provider.hasApiKey && isActive && (
        <Alert tone="warn">
          Bu sağlayıcının API anahtarı tanımlı değil. Anahtarsız etkinleştirirseniz
          çağrılar başarısız olur.
        </Alert>
      )}

      {err && <ErrorBox err={err} />}

      <div className="flex flex-col gap-2 sm:flex-row-reverse">
        <Button type="submit" loading={save.isPending} fullWidth>Ayarları kaydet</Button>
        <Button type="button" variant="outline" fullWidth disabled={save.isPending} onClick={onDone}>
          Vazgeç
        </Button>
      </div>
    </form>
  );
}

/**
 * API anahtarı — AYRI form, ayrı uç.
 *
 * 🔴 Anahtar hiçbir GET yanıtında dönmez; ekranda gösterilmez ve React
 * durumunda tutulmaz. Alan kontrolsüzdür (ref ile okunur), gönderimden hemen
 * sonra temizlenir. Sunucu yalnız maskeli bir önizleme döner — yöneticinin
 * doğru anahtarı yapıştırdığını görmesine yeter, anahtarı ele vermez.
 *
 * Boş gönderim sunucuda 422 ile reddedilir (anahtar silme yolu yoktur;
 * sağlayıcı pasifleştirilir), bu yüzden boş formu hiç göndermeyiz.
 */
function ApiKeyForm({ provider, onDone }: { provider: AdminProvider; onDone: () => void }) {
  const qc = useQueryClient();
  const inputRef = React.useRef<HTMLInputElement>(null);
  const [error, setError] = React.useState<string>();

  const save = useMutation({
    mutationFn: (apiKey: string) =>
      apiFetch<SetApiKeyResult>(`/admin/providers/${encodeURIComponent(provider.id)}/api-key`, {
        method: 'PUT',
        body: { apiKey },
      }),
    retry: false,
    onSuccess: () => {
      // Anahtar bellekte kalmasın: alan başarıdan hemen sonra boşaltılır.
      if (inputRef.current) inputRef.current.value = '';
      qc.invalidateQueries({ queryKey: QUERY_KEY });
    },
  });

  function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    const key = inputRef.current?.value.trim() ?? '';
    if (!key) {
      setError('Anahtar boş olamaz. Silmek için sağlayıcıyı pasifleştirin.');
      return;
    }
    setError(undefined);
    save.mutate(key);
  }

  const err = save.error instanceof ApiError ? save.error : null;

  return (
    <form onSubmit={onSubmit} className="flex flex-col gap-4" noValidate>
      <div className="raised rounded-xl border p-3">
        <p className="text-sm font-medium">{provider.name}</p>
        <p className="mt-0.5 text-xs text-muted">
          {provider.hasApiKey ? 'Şu an bir anahtar tanımlı.' : 'Şu an anahtar tanımlı değil.'}
        </p>
      </div>

      <Alert tone="info">
        Anahtar kaydedildikten sonra <strong>hiçbir ekranda geri gösterilmez</strong>.
        Yeni bir anahtar yazmak eskisinin yerine geçer.
      </Alert>

      <Field
        ref={inputRef}
        data-autofocus
        label="Yeni API anahtarı"
        type="password"
        autoComplete="off"
        autoCapitalize="none"
        autoCorrect="off"
        spellCheck={false}
        error={error ?? err?.fieldMap().apiKey}
        hint="Sağlayıcı panelinden aldığınız anahtarı yapıştırın."
      />

      {err && <ErrorBox err={err} />}

      {save.isSuccess && save.data && (
        <Alert tone="ok">
          Anahtar kaydedildi. Önizleme: <strong>{save.data.masked}</strong>
        </Alert>
      )}

      <div className="flex flex-col gap-2 sm:flex-row-reverse">
        <Button type="submit" loading={save.isPending} fullWidth>Anahtarı kaydet</Button>
        <Button type="button" variant="outline" fullWidth disabled={save.isPending} onClick={onDone}>
          Kapat
        </Button>
      </div>
    </form>
  );
}

/**
 * Yeni sağlayıcı.
 *
 * 🔴 Bu formda API anahtarı alanı YOKTUR: ekleme ucu anahtar almaz. Aynı
 * gövdede taşınsaydı anahtar hata ayıklama çıktısına ve tarayıcı ağ sekmesine
 * düşerdi. Yeni sağlayıcı PASİF doğar; anahtar konup boyut eşleştirmeleri
 * senkronlandıktan sonra ayarlardan etkinleştirilir.
 */
function CreateProviderForm({ onDone }: { onDone: () => void }) {
  const qc = useQueryClient();
  const [name, setName] = React.useState('');
  const [protocol, setProtocol] = React.useState<string>(PROTOCOLS[0]);
  const [baseUrl, setBaseUrl] = React.useState('');
  const [priority, setPriority] = React.useState('100');
  const [costMultiplier, setCostMultiplier] = React.useState('1.00');
  const [capabilities, setCapabilities] = React.useState<string[]>(['SMS_ACTIVATION']);
  const [errors, setErrors] = React.useState<Record<string, string>>({});

  const create = useMutation({
    mutationFn: (body: {
      name: string; protocol: string; baseUrl: string;
      priority: number; costMultiplier: string; capabilities: string[];
    }) => apiFetch<AdminProvider>('/admin/providers', { method: 'POST', body }),
    retry: false,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEY });
      onDone();
    },
  });

  function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    const errs: Record<string, string> = {};

    const nm = name.trim();
    if (!nm) errs.name = 'Ad zorunludur.';
    else if (nm.length > 60) errs.name = 'Ad en fazla 60 karakter olabilir.';

    const url = baseUrl.trim();
    if (url && !/^https?:\/\//i.test(url)) {
      errs.baseUrl = 'Adres http:// veya https:// ile başlamalıdır.';
    }
    const prio = Number(priority.trim());
    if (!/^\d+$/.test(priority.trim()) || !Number.isInteger(prio) || prio < 0 || prio > 10000) {
      errs.priority = 'Öncelik 0-10000 arasında bir tam sayı olmalıdır.';
    }
    const mult = costMultiplier.trim();
    if (!/^\d+(\.\d+)?$/.test(mult)) {
      errs.costMultiplier = 'Çarpan ondalık bir sayı olmalıdır (örn. 1.00).';
    }
    if (!capabilities.length) errs.capabilities = 'En az bir yetenek seçin.';

    setErrors(errs);
    if (Object.keys(errs).length) return;

    create.mutate({
      name: nm, protocol, baseUrl: url,
      priority: prio, costMultiplier: mult, capabilities,
    });
  }

  const err = create.error instanceof ApiError ? create.error : null;
  const fieldErrs = { ...errors, ...(err?.fieldMap() ?? {}) };

  return (
    <form onSubmit={onSubmit} className="flex flex-col gap-4" noValidate>
      <Alert tone="info">
        Yeni sağlayıcı <strong>pasif</strong> olarak eklenir ve burada API anahtarı
        sorulmaz. Ekledikten sonra anahtarı tanımlayın, katalogu senkronlayın ve
        ancak ondan sonra ayarlardan etkinleştirin.
      </Alert>

      <Field
        data-autofocus
        label="Ad"
        value={name}
        onChange={(e) => setName(e.target.value)}
        placeholder="Örn. HeroSMS"
        maxLength={60}
        autoComplete="off"
        error={fieldErrs.name}
      />

      <label className="flex flex-col gap-1.5">
        <span className="text-sm font-medium">Protokol</span>
        <select
          value={protocol}
          onChange={(e) => setProtocol(e.target.value)}
          className={SELECT_CLASS}
        >
          {PROTOCOLS.map((p) => <option key={p} value={p}>{p}</option>)}
        </select>
        {fieldErrs.protocol
          ? <span role="alert" className="text-xs text-[var(--color-bad)]">{fieldErrs.protocol}</span>
          : <span className="text-xs text-muted">Sağlayıcının konuştuğu API biçimi.</span>}
      </label>

      <Field
        label="Adres (baseUrl)"
        value={baseUrl}
        onChange={(e) => setBaseUrl(e.target.value)}
        placeholder="https://api.ornek.com"
        inputMode="url"
        autoCapitalize="none"
        autoCorrect="off"
        autoComplete="off"
        spellCheck={false}
        error={fieldErrs.baseUrl}
        hint="Boş bırakılırsa protokolün varsayılan adresi kullanılır."
      />

      <Field
        label="Öncelik"
        value={priority}
        onChange={(e) => setPriority(e.target.value)}
        inputMode="numeric"
        autoComplete="off"
        error={fieldErrs.priority}
        hint="0-10000. Küçük değer önce denenir."
      />

      <Field
        label="Maliyet çarpanı"
        value={costMultiplier}
        onChange={(e) => setCostMultiplier(e.target.value)}
        inputMode="decimal"
        autoComplete="off"
        error={fieldErrs.costMultiplier}
      />

      <fieldset className="flex flex-col gap-1.5">
        <legend className="text-sm font-medium">Yetenekler</legend>
        {CAPABILITIES.map((c) => (
          <label key={c.value} className="flex items-start gap-3 py-1">
            <input
              type="checkbox"
              checked={capabilities.includes(c.value)}
              onChange={(e) =>
                setCapabilities((prev) =>
                  e.target.checked ? [...prev, c.value] : prev.filter((x) => x !== c.value))
              }
              className="mt-0.5 size-5 shrink-0 rounded accent-[var(--color-brand-500)]"
            />
            <span className="text-sm">{c.label}</span>
          </label>
        ))}
        {fieldErrs.capabilities && (
          <span role="alert" className="text-xs text-[var(--color-bad)]">{fieldErrs.capabilities}</span>
        )}
      </fieldset>

      {err && <ErrorBox err={err} />}

      <div className="flex flex-col gap-2 sm:flex-row-reverse">
        <Button type="submit" loading={create.isPending} fullWidth>Sağlayıcıyı ekle</Button>
        <Button type="button" variant="outline" fullWidth disabled={create.isPending} onClick={onDone}>
          Vazgeç
        </Button>
      </div>
    </form>
  );
}
