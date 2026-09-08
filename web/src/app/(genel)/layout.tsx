import { SiteNav, SiteFooter } from '@/components/site-nav';

export default function PublicLayout({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex min-h-screen-safe flex-col">
      <SiteNav />
      <main id="icerik" className="flex-1">{children}</main>
      <SiteFooter />
    </div>
  );
}
