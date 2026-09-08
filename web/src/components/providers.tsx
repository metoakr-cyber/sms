'use client';

import * as React from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ApiError, isRetryable } from '@/lib/api';

/**
 * TanStack Query yapılandırması — docs/frontend-contract.md §3.2.
 *
 * En kritik satır `mutations.retry: false`. Bir POST /orders'ın otomatik
 * tekrarlanması, kullanıcıya İKİ numara alıp iki kez ücret keser.
 */
function makeClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: (count, err) => count < 3 && isRetryable(err),
        // 429'da sunucunun Retry-After'ına uy; yoksa üstel geri çekilme + jitter.
        // Jitter olmadan tüm istemciler aynı anda geri döner (thundering herd).
        retryDelay: (i, err) => {
          const after = err instanceof ApiError ? err.retryAfterMs : undefined;
          if (after !== undefined) return Math.min(after, 30_000);
          return Math.min(300 * 3 ** i, 5_000) + Math.random() * 200;
        },
        staleTime: 30_000,
        refetchOnWindowFocus: true, // mobilde uygulamaya dönünce taze veri
        refetchOnReconnect: true,
      },
      mutations: { retry: false },
    },
  });
}

export function Providers({ children }: { children: React.ReactNode }) {
  // useState ile TEK sefer kurulur: her render'da yeni client, tüm önbelleği atar.
  const [client] = React.useState(makeClient);
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

/**
 * Çevrimdışı bandı.
 *
 * `navigator.onLine` GÜVENİLMEZDİR: internetsiz bir Wi-Fi'ye bağlıyken de
 * `true` döner. Yalnız İPUCU olarak kullanılır — bu yüzden satın alma butonu
 * burada devre dışı bırakılmaz, sadece uyarı gösterilir (§3.3).
 */
export function OfflineBanner() {
  const [offline, setOffline] = React.useState(false);
  React.useEffect(() => {
    const on = () => setOffline(false);
    const off = () => setOffline(true);
    setOffline(!navigator.onLine);
    window.addEventListener('online', on);
    window.addEventListener('offline', off);
    return () => { window.removeEventListener('online', on); window.removeEventListener('offline', off); };
  }, []);
  if (!offline) return null;
  return (
    <div role="status" className="bg-[var(--color-warn)] px-4 py-2 text-center text-sm font-medium text-black">
      İnternet bağlantısı görünmüyor. İşlemler başarısız olabilir.
    </div>
  );
}
