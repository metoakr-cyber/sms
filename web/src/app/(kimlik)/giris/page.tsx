import type { Metadata } from 'next';
import { Suspense } from 'react';
import { Card, Skeleton } from '@/components/ui';
import LoginForm from './form';

export const metadata: Metadata = {
  title: 'Giriş Yap',
  description: 'SMS Onay hesabınıza giriş yapın.',
  alternates: { canonical: '/giris' },
  robots: { index: false, follow: true },
};

export default function LoginPage() {
  // useSearchParams() Suspense sınırı GEREKTİRİR: sayfa ön-render edilirken
  // sorgu dizesi henüz bilinmez. Sınır olmadan Next.js tüm sayfayı istemci
  // tarafına düşürür ve derleme hatası verir.
  return (
    <Suspense fallback={<Card className="p-6 md:p-8"><Skeleton className="h-80" /></Card>}>
      <LoginForm />
    </Suspense>
  );
}
