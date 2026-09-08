'use client';

import { useQuery, useQueryClient } from '@tanstack/react-query';
import { ApiError, apiFetch } from '@/lib/api';
import type { User } from '@/lib/types';

export const meKey = ['me'] as const;

/**
 * Oturumdaki kullanıcı.
 *
 * Oturum çerezi httpOnly'dir; JavaScript OKUYAMAZ. Bu yüzden "giriş yapılmış mı"
 * sorusunun tek doğru cevabı sunucuya sormaktır. İstemcide bir "isLoggedIn"
 * bayrağı tutmak, sunucuda süresi dolmuş bir oturumu arayüzde canlı gösterir.
 */
export function useSession() {
  const q = useQuery({
    queryKey: meKey,
    queryFn: () => apiFetch<User>('/me'),
    // 401 beklenen bir cevaptır (giriş yapılmamış), hata değil — tekrar deneme.
    retry: (count, err) => {
      if (err instanceof ApiError && err.status === 401) return false;
      return count < 2;
    },
    staleTime: 60_000,
  });

  const unauthenticated = q.error instanceof ApiError && q.error.status === 401;

  return {
    user: unauthenticated ? null : q.data ?? null,
    isLoading: q.isLoading,
    isAuthenticated: !!q.data && !unauthenticated,
    unauthenticated,
    error: unauthenticated ? null : q.error,
    refetch: q.refetch,
  };
}

export function useClearSession() {
  const qc = useQueryClient();
  // Çıkışta TÜM önbellek atılır. Yalnız 'me' anahtarını silmek, bir sonraki
  // kullanıcıya önceki kullanıcının bakiyesini/ekstresini gösterebilir.
  return () => qc.clear();
}
