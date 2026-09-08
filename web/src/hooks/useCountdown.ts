'use client';

import * as React from 'react';

/**
 * Sunucu zaman damgasına dayalı geri sayım.
 *
 * SAYAÇ TUTULMAZ, HER SANİYE YENİDEN HESAPLANIR.
 *
 * Neden: mobil tarayıcı arka plandaki sekmenin zamanlayıcılarını kısar ya da
 * tamamen durdurur. `setInterval` ile "kalan--" yapan bir sayaç, kullanıcı
 * uygulamaya döndüğünde geride kalır ve süresi dolmuş bir teklifi hâlâ geçerli
 * gösterir. `expiresAt - now` ise sekme dondurulsa da doğru kalır.
 * (docs/frontend-contract.md §2.6)
 */
export function useCountdown(expiresAt: string | null | undefined): number {
  const target = React.useMemo(() => {
    if (!expiresAt) return null;
    const t = Date.parse(expiresAt);
    return Number.isNaN(t) ? null : t;
  }, [expiresAt]);

  const [left, setLeft] = React.useState(() =>
    target === null ? 0 : Math.max(0, (target - Date.now()) / 1000));

  React.useEffect(() => {
    if (target === null) { setLeft(0); return; }

    const tick = () => setLeft(Math.max(0, (target - Date.now()) / 1000));
    tick();
    const id = setInterval(tick, 500);

    // Sekme geri geldiğinde HEMEN yeniden hesapla; bir sonraki tick'i bekleme.
    const onVis = () => { if (document.visibilityState === 'visible') tick(); };
    document.addEventListener('visibilitychange', onVis);

    return () => { clearInterval(id); document.removeEventListener('visibilitychange', onVis); };
  }, [target]);

  return left;
}
