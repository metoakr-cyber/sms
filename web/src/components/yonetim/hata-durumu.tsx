/**
 * HataDurumu — tipli API hatasını kullanıcıya gösteren TEK yer.
 *
 * KAPATTIĞI TEKRAR: `ErrorBox` 9 ekranda kopyalanmıştı (8'i birebir, 13'er
 * satır) — 8 `/yonetim` + `/panel/bakiye-yukle`. `requestId` biçimini
 * değiştirmek 9 dosyaya dokunmak demekti; destek ekibinin bir istek numarasını
 * nasıl isteyeceği ekranların insafına kalmıştı.
 *
 * SÖZLEŞME (CLAUDE.md #12): kullanıcıya HAM hata gösterilmez. `ApiError.message`
 * sunucunun ürettiği Türkçe, gösterilebilir metindir; `code`/`status` gibi
 * teknik alanlar ekrana yazılmaz — destek için gereken tek şey `requestId`'dir.
 *
 * `role="alert"` DOĞRUDUR ve `Alert`'ten gelir: bu bileşen yalnız hata
 * OLUŞTUĞUNDA render edilir, yani "yeni beliren hata" tanımına girer
 * (tasarim-sistemi.md §7.4). Açılışta duran statik bilgi kutuları için
 * `Alert tone="info"` kullanılır, bu bileşen değil.
 */
import * as React from 'react';
import { ApiError } from '@/lib/api';
import { Alert, cx } from '@/components/ui';

export function HataDurumu({ hata, className }: { hata: ApiError; className?: string }) {
  return (
    <Alert tone="bad" className={cx('flex flex-col gap-2', className)}>
      <p>{hata.message}</p>

      {hata.fields?.length ? (
        <ul className="list-inside list-disc">
          {hata.fields.map((f) => (
            <li key={f.field}>{f.message}</li>
          ))}
        </ul>
      ) : null}

      {hata.requestId && (
        /*
         * İSTEK NUMARASI 14px VE TAM OPAKLIKTA.
         *
         * Özgün kopyalar bunu `text-xs opacity-60` yazıyordu. İkisi de hata:
         *  · 12px, tasarim-sistemi.md §3.2'nin "veri taşıyan metin 14px altına
         *    inmez" kuralını çiğniyor — ve bu metin telefonda okunup destek
         *    ekibine YAZILIYOR, yani en çok okunması gereken şey.
         *  · %60 opaklık, `Alert`'in zaten renkli olan metnini kontrast
         *    eşiğinin (§7.1, 4.5:1) altına düşürüyor.
         * Monospace burada MEŞRUDUR (§9.2): kimlik dizgileri "teknik dursun"
         * diye değil, karakter karakter okunabilsin diye monospace yazılır.
         */
        <p className="break-anywhere font-mono text-sm">
          <span className="font-sans text-muted">İstek no: </span>
          {hata.requestId}
        </p>
      )}
    </Alert>
  );
}

/**
 * TanStack Query'nin `unknown` hatasını `ApiError`'a daraltır.
 *
 * KAPATTIĞI TEKRAR: `q.error instanceof ApiError ? q.error : null` satırı
 * ekranlarda 20'den fazla kez elle yazılıyordu. `apiFetch` her hatayı
 * `ApiError`'a normalize eder (lib/api.ts §10), ama React Query imzası
 * `unknown` verdiği için daraltma her çağrı yerinde tekrar ediyordu.
 */
export function apiHatasi(err: unknown): ApiError | null {
  return err instanceof ApiError ? err : null;
}
