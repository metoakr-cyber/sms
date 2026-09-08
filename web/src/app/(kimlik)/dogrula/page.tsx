import type { Metadata } from 'next';
import { Suspense } from 'react';
import { Card, Skeleton } from '@/components/ui';
import VerifyEmailForm from './form';

export const metadata: Metadata = {
  title: 'E-posta Doğrulama',
  description: 'Onay360 hesabınızın e-posta adresini doğrulayın.',
  alternates: { canonical: '/dogrula' },
  // follow:false — adres çubuğunda tek kullanımlık bir token taşıyan bir sayfa
  // arama motoruna hiçbir şekilde açılmamalı (frontend-contract.md §10.1).
  robots: { index: false, follow: false },
};

export default function VerifyEmailPage() {
  // useSearchParams() Suspense sınırı GEREKTİRİR — giris/page.tsx ile aynı
  // gerekçe: sayfa ön-render edilirken sorgu dizesi henüz bilinmez.
  return (
    <Suspense fallback={<Card className="p-6 md:p-8"><Skeleton className="h-56" /></Card>}>
      <VerifyEmailForm />
    </Suspense>
  );
}
