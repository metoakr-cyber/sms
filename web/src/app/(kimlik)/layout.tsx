import Link from 'next/link';
import { Logo } from '@/components/logo';

export default function AuthLayout({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex min-h-screen-safe flex-col">
      <header className="px-4 py-4 md:px-6">
        <Link href="/" aria-label="Ana sayfa" className="inline-flex min-h-11 items-center"><Logo /></Link>
      </header>
      {/* Dikey ortalama YALNIZ ekran yeterince uzunsa; mobilde klavye açılınca
          ortalama içeriği yukarı taşıyıp form alanlarını kırpıyor. */}
      <main id="icerik" className="flex flex-1 items-start justify-center px-4 pb-safe pt-4 sm:items-center sm:py-10">
        <div className="w-full max-w-md">{children}</div>
      </main>
    </div>
  );
}
