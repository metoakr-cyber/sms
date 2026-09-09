'use client';

/**
 * Bakiye düzeltme.
 *
 * Bu ekran GERÇEK PARA yazar ve yazdığı satır SİLİNEMEZ: kayıt defterine
 * değiştirilemez bir `ADJUSTMENT` girer (CLAUDE.md değişmez #4). Bu yüzden
 * form üç şeyi birden yapar — kimi, ne kadar, neden.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * 🔴 NEGATİF TUTAR BURADA MEŞRUDUR — ve YALNIZ burada
 * ══════════════════════════════════════════════════════════════════════════
 * `lib/para.ts` içindeki `TUTAR_BAKIYE_DUZELTME` ayarı `izinNegatif: true`
 * taşır. Diğer dört çağrı yeri (`talepler`, `odeme-yontemleri`, `fiyatlar`,
 * `panel/bakiye-yukle`) eksiyi REDDEDER. Bu fark kaza değil: yanlış yazılmış
 * bir bakiyeyi geri almanın tek yolu ters yönde bir düzeltmedir; eksiyi
 * reddetmek o yolu kapatır. Ayarı burada elle kurmayın — sabiti kullanın.
 */

import * as React from 'react';
import { useMutation } from '@tanstack/react-query';
import { apiFetch } from '@/lib/api';
import { formatMoney } from '@/lib/format';
import { toMinor, TUTAR_BAKIYE_DUZELTME } from '@/lib/para';
import { Alert, Button, Card, Field } from '@/components/ui';
import { HataDurumu, SayfaBasligi, apiHatasi } from '@/components/yonetim';
import type { Money } from '@/lib/types';

interface AdjustResult { balance: Money; alreadyApplied?: boolean }

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export default function AdminBalancePage() {
  const [userId, setUserId] = React.useState('');
  const [amount, setAmount] = React.useState('');
  const [note, setNote] = React.useState('');
  const [errors, setErrors] = React.useState<Record<string, string>>({});

  /*
   * İdempotency anahtarı FORM DOLDURULURKEN üretilir, gönderim anında değil.
   * Gönderim anında üretilseydi: "Uygula" → ağ kopar → tekrar bas → yeni
   * anahtar → aynı düzeltme İKİ KEZ uygulanır. Anahtar mantıksal işlemi
   * tanımlar; işlem değişmedikçe değişmez.
   */
  const [idemKey, setIdemKey] = React.useState(() => crypto.randomUUID());

  const adjust = useMutation({
    mutationFn: (v: { id: string; amountMinor: number; note: string; key: string }) =>
      apiFetch<AdjustResult>(`/admin/users/${encodeURIComponent(v.id)}/balance`, {
        method: 'POST',
        body: { amountMinor: v.amountMinor, note: v.note, idempotencyKey: v.key },
      }),
    // Para yazan bir çağrı ASLA otomatik tekrarlanmaz (CLAUDE.md #16).
    retry: false,
    onSuccess: () => {
      setIdemKey(crypto.randomUUID()); // sıradaki düzeltme AYRI bir işlemdir
      setAmount(''); setNote('');
    },
  });

  function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    const errs: Record<string, string> = {};

    if (!UUID.test(userId.trim())) {
      errs.userId = 'Kullanıcı kimliği bir UUID olmalıdır.';
    }
    const parsed = toMinor(amount, TUTAR_BAKIYE_DUZELTME);
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

  // Önizleme: yazılacak tutarı kullanıcının yazdığı biçimle DEĞİL, sunucunun
  // göstereceği biçimle yazar — "-25,5" yazan yönetici "-25,50 ₺" görür.
  const onizleme = React.useMemo(() => {
    const p = toMinor(amount, TUTAR_BAKIYE_DUZELTME);
    if ('error' in p) return null;
    return formatMoney({ minor: p.minor, currency: 'TRY', formatted: '' });
  }, [amount]);

  const hata = apiHatasi(adjust.error);
  const sonuc = adjust.isSuccess ? adjust.data : null;

  return (
    <div className="mx-auto flex max-w-2xl flex-col gap-6">
      <SayfaBasligi
        baslik="Bakiye düzeltme"
        aciklama="Pozitif tutar bakiyeyi artırır, negatif tutar azaltır."
      />

      {/*
        `duyur={false}` — bu kutu SAYFA AÇILIŞINDA koşulsuz çizilir; kullanıcı
        henüz hiçbir şey yapmadı. `role="alert"` ekran okuyucuya "o an yapılan
        işi BÖL" der ve bu kutu bölecek bir iş bulamaz: ekrana girer girmez
        statik bir açıklamayı kesintili uyarı olarak okutur (§7.4).
        Metin kaybolmaz — sayfa başlığından sonra sırası gelince okunur.
        Bu ekranın GERÇEK duyurusu aşağıdaki sonuç kutusudur (`role="alert"`
        orada kalıyor): para YAZILDIKTAN sonra beliren tek geri bildirim odur.
      */}
      <Alert tone="warn" duyur={false}>
        Her düzeltme kayıt defterine <strong>değiştirilemez</strong> bir satır olarak
        yazılır ve sizin adınıza kaydedilir. Silinemez, düzeltilemez — yalnız
        ters yönde yeni bir düzeltme ile dengelenebilir.
      </Alert>

      <Card>
        <form onSubmit={onSubmit} className="flex flex-col gap-5" noValidate>
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
            hint={onizleme ? `Uygulanacak: ${onizleme}` : 'Virgül veya nokta kullanabilirsiniz.'}
          />

          <Field
            label="Gerekçe"
            value={note} onChange={(e) => setNote(e.target.value)}
            placeholder="Örn. Havale ile yükleme — dekont #12345"
            maxLength={200} error={errors.note}
          />

          {/* 14px ve tam opaklık (§3.2): bu dize destek ekibine OKUNARAK
              aktarılır — özgün kod `text-xs` yazıyordu. Monospace meşru
              (§9.2): karakter karakter okunabilsin diye. */}
          <div className="flex flex-col gap-1 border-t border-[var(--border)] pt-4 text-sm">
            <span className="text-muted">İşlem anahtarı</span>
            <code className="break-anywhere font-mono">{idemKey}</code>
          </div>

          <Button type="submit" loading={adjust.isPending} fullWidth>
            Düzeltmeyi uygula
          </Button>
        </form>

        {hata && <HataDurumu hata={hata} className="mt-5" />}

        {sonuc && (
          /*
            TEKRARLANAN DÜZELTME HATA DEĞİLDİR: sunucu 200 + alreadyApplied
            döner; `talepler` sonuç adımı da aynı ayrımı aynı renkle yapar.

            🔴 ROZET İÇİNE CÜMLE YAZILMAZ (§5.2, ölçülmüş ihlal): özgün kod bu
            cümleyi bir `Badge`e koymuştu ve `whitespace-nowrap` yüzünden
            320px'te yatay taşma üretiyordu. Rozet etikettir, cümle değil.
          */
          <Alert tone={sonuc.alreadyApplied ? 'info' : 'ok'} className="mt-5">
            {sonuc.alreadyApplied && (
              <p className="mb-2">Bu işlem zaten uygulanmıştı; hiçbir şey yeniden yazılmadı.</p>
            )}
            <p>
              Kullanıcının yeni bakiyesi: <strong>{formatMoney(sonuc.balance)}</strong>
            </p>
          </Alert>
        )}
      </Card>
    </div>
  );
}
