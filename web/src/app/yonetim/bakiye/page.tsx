'use client';

import * as React from 'react';
import { useMutation } from '@tanstack/react-query';
import { ApiError, apiFetch } from '@/lib/api';
import { formatMoney } from '@/lib/format';
import { Card, Button, Field, Alert, Badge } from '@/components/ui';
import type { Money } from '@/lib/types';

interface AdjustResult { balance: Money; alreadyApplied?: boolean }

/**
 * Kuruş girişi.
 *
 * Kullanıcı "12,50" ya da "12.50" yazar; sunucu int64 KURUŞ bekler ve çıplak
 * ondalık sayıyı REDDEDER (trd.md §9). Çevrimi burada, tek bir yerde ve
 * KAYAN NOKTA KULLANMADAN yaparız: `12.50 * 100` JavaScript'te 1249.9999...
 * verebilir ve bir kuruş kaybolur.
 */
function toMinor(input: string): { minor: number } | { error: string } {
  const s = input.trim().replace(/\s/g, '').replace(',', '.');
  if (s === '' || s === '-') return { error: 'Tutar giriniz.' };
  if (!/^-?\d+(\.\d{1,2})?$/.test(s)) {
    return { error: 'Geçerli bir tutar giriniz (en fazla 2 ondalık).' };
  }
  const neg = s.startsWith('-');
  const [whole = '0', frac = ''] = s.replace('-', '').split('.');
  const minor = Number(whole) * 100 + Number(frac.padEnd(2, '0'));
  if (!Number.isSafeInteger(minor)) return { error: 'Tutar çok büyük.' };
  return { minor: neg ? -minor : minor };
}

export default function AdminBalancePage() {
  const [userId, setUserId] = React.useState('');
  const [amount, setAmount] = React.useState('');
  const [note, setNote] = React.useState('');
  const [errors, setErrors] = React.useState<Record<string, string>>({});

  /*
   * İdempotency anahtarı FORM DOLDURULURKEN üretilir, gönderim anında değil.
   *
   * Gönderim anında üretilseydi: kullanıcı "Uygula"ya basar, ağ kopar, tekrar
   * basar → yeni anahtar → aynı düzeltme İKİ KEZ uygulanır. Anahtar mantıksal
   * işlemi tanımlar; işlem değişmediği sürece anahtar da değişmez.
   * Başarılı yazımdan sonra yeni bir anahtar üretilir.
   */
  const [idemKey, setIdemKey] = React.useState(() => crypto.randomUUID());

  const adjust = useMutation({
    mutationFn: (v: { id: string; amountMinor: number; note: string; key: string }) =>
      apiFetch<AdjustResult>(`/admin/users/${encodeURIComponent(v.id)}/balance`, {
        method: 'POST',
        body: { amountMinor: v.amountMinor, note: v.note, idempotencyKey: v.key },
      }),
    onSuccess: () => {
      setIdemKey(crypto.randomUUID()); // sıradaki düzeltme AYRI bir işlemdir
      setAmount(''); setNote('');
    },
  });

  function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    const errs: Record<string, string> = {};

    if (!/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(userId.trim())) {
      errs.userId = 'Kullanıcı kimliği bir UUID olmalıdır.';
    }
    const parsed = toMinor(amount);
    if ('error' in parsed) errs.amount = parsed.error;
    else if (parsed.minor === 0) errs.amount = 'Tutar sıfır olamaz.';
    if (!note.trim()) errs.note = 'Gerekçe zorunludur — kayıt defterine yazılır.';

    setErrors(errs);
    if (Object.keys(errs).length) return;

    adjust.mutate({
      id: userId.trim(),
      amountMinor: (parsed as { minor: number }).minor,
      note: note.trim(),
      key: idemKey,
    });
  }

  const preview = React.useMemo(() => {
    const p = toMinor(amount);
    if ('error' in p) return null;
    return formatMoney({ minor: p.minor, currency: 'TRY', formatted: '' });
  }, [amount]);

  const err = adjust.error instanceof ApiError ? adjust.error : null;

  return (
    <div className="mx-auto flex max-w-2xl flex-col gap-5">
      <div>
        <h1 className="text-2xl font-bold tracking-tight md:text-3xl">Bakiye düzeltme</h1>
        <p className="mt-1 text-sm text-muted">
          Pozitif tutar bakiyeyi artırır, negatif tutar azaltır.
        </p>
      </div>

      <Alert tone="warn">
        Her düzeltme kayıt defterine <strong>değiştirilemez</strong> bir satır olarak
        yazılır ve sizin adınıza kaydedilir. Silinemez, düzeltilemez — yalnız
        ters yönde yeni bir düzeltme ile dengelenebilir.
      </Alert>

      <Card>
        <form onSubmit={onSubmit} className="flex flex-col gap-4" noValidate>
          <Field
            label="Kullanıcı kimliği (UUID)"
            value={userId} onChange={(e) => setUserId(e.target.value)}
            placeholder="00000000-0000-0000-0000-000000000000"
            autoCapitalize="none" autoCorrect="off" spellCheck={false}
            error={errors.userId}
            hint="Kullanıcının genel kimliği (sayısal id değil)."
          />

          <Field
            label="Tutar (TL)"
            value={amount} onChange={(e) => setAmount(e.target.value)}
            placeholder="Örn. 150,00 veya -25,50"
            inputMode="decimal" autoComplete="off"
            error={errors.amount}
            hint={preview ? `Uygulanacak: ${preview}` : 'Virgül veya nokta kullanabilirsiniz.'}
          />

          <Field
            label="Gerekçe"
            value={note} onChange={(e) => setNote(e.target.value)}
            placeholder="Örn. Havale ile yükleme — dekont #12345"
            maxLength={200} error={errors.note}
          />

          <div className="flex items-center justify-between gap-2 text-xs text-muted">
            <span>İşlem anahtarı</span>
            <code className="break-anywhere font-mono">{idemKey}</code>
          </div>

          <Button type="submit" loading={adjust.isPending} fullWidth>
            Düzeltmeyi uygula
          </Button>
        </form>

        {err && (
          <Alert className="mt-4">
            <p>{err.message}</p>
            {err.fields?.length ? (
              <ul className="mt-1 list-inside list-disc">
                {err.fields.map((f) => <li key={f.field}>{f.message}</li>)}
              </ul>
            ) : null}
            {err.requestId && <p className="mt-2 text-xs opacity-60">İstek no: {err.requestId}</p>}
          </Alert>
        )}

        {adjust.isSuccess && adjust.data && (
          <Alert tone="ok" className="mt-4">
            <div className="flex flex-wrap items-center gap-2">
              <span>Yeni bakiye: <strong>{formatMoney(adjust.data.balance)}</strong></span>
              {adjust.data.alreadyApplied && (
                <Badge tone="warn">Bu işlem zaten uygulanmıştı — tekrar yazılmadı</Badge>
              )}
            </div>
          </Alert>
        )}
      </Card>
    </div>
  );
}
