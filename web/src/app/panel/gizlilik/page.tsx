import type { Metadata } from 'next';
import { SayfaBasligi } from '@/components/yonetim';
import { GizlilikMetni } from '@/components/yasal/gizlilik';

export const metadata: Metadata = { title: 'Gizlilik Politikası' };

/**
 * `/panel/gizlilik` — metnin PANEL yüzeyi.
 *
 * NEDEN VAR: kullanıcı panelde çalışırken hukuki metni okumak için paneli
 * terk etmesin (kullanıcı isteği). Kabuk `panel/layout.tsx` → `PanelShell`
 * üzerinden kendiliğinden gelir.
 *
 * 🔴 METİN BURADA DEĞİL. `@/components/yasal/gizlilik` genel sayfayla
 * (`/gizlilik`) PAYLAŞILAN tek kaynaktır.
 *
 * Kap YOK: panel içeriği tam genişliktir; satır uzunluğunu paylaşılan
 * bileşenin `max-w-[70ch]`i sınırlar.
 */
export default function PanelGizlilikPage() {
  return (
    <div className="flex flex-col gap-6">
      <SayfaBasligi baslik="Gizlilik Politikası" />
      <GizlilikMetni yuzey="panel" />
    </div>
  );
}
