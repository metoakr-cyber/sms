import type { Metadata } from 'next';
import { PanelShell } from '@/components/panel-shell';

// Panel ASLA indekslenmez (frontend-contract.md §10.1). Bu, next.config.ts'teki
// X-Robots-Tag başlığına EK katmandır — biri unutulursa diğeri tutar.
export const metadata: Metadata = { robots: { index: false, follow: false } };

export default function PanelLayout({ children }: { children: React.ReactNode }) {
  return <PanelShell>{children}</PanelShell>;
}
