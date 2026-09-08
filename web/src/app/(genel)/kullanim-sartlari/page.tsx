import type { Metadata } from 'next';
import { Card, Alert } from '@/components/ui';

export const metadata: Metadata = {
  title: 'Kullanım Şartları',
  alternates: { canonical: '/kullanim-sartlari' },
};

export default function Page() {
  return (
    <div className="px-4 py-10 md:px-6 md:py-14">
      <div className="mx-auto max-w-3xl">
        <h1 className="text-3xl font-bold tracking-tight md:text-4xl">Kullanım Şartları</h1>
        <Card className="mt-6">
          {/* Metin bilerek boş: yasal metin uydurulmaz. Şirket kuruluşu ve
              hukuki inceleme sonrası doldurulacak (intent.md — açık risk). */}
          <Alert tone="warn">
            Bu metin henüz yayımlanmadı. Hizmet kullanıma açılmadan önce
            hukuki inceleme sonrası burada yayımlanacaktır.
          </Alert>
        </Card>
      </div>
    </div>
  );
}
