import type { Metadata } from 'next';
import { SayfaBasligi } from '@/components/yonetim';
import { KullanimSartlariMetni } from '@/components/yasal/kullanim-sartlari';

export const metadata: Metadata = { title: 'Kullanım Şartları' };

/**
 * `/panel/kullanim-sartlari` — metnin PANEL yüzeyi.
 *
 * NEDEN VAR: kullanıcı panelde çalışırken hukuki metni okumak için paneli
 * terk etmesin (kullanıcı isteği). Kabuk — üst başlık, sol gezinti, bakiye —
 * `panel/layout.tsx` → `PanelShell` üzerinden kendiliğinden gelir.
 *
 * 🔴 METİN BURADA DEĞİL. `@/components/yasal/kullanim-sartlari` genel sayfayla
 * (`/kullanim-sartlari`) PAYLAŞILAN tek kaynaktır; metni buraya kopyalamak,
 * iki yüzeyden birinin gün gelip eski sözleşmeyi göstermesi demektir.
 *
 * Kap YOK: `panel-shell.tsx`in `main`i tam genişliktir ve panel sayfaları
 * kendi `max-w-*` kabını açmaz (kullanıcı kararı, 10 Eylül 2026). Düz metnin
 * satır uzunluğunu paylaşılan bileşenin `max-w-[70ch]`i sınırlar.
 */
export default function PanelKullanimSartlariPage() {
  return (
    <div className="flex flex-col gap-6">
      <SayfaBasligi baslik="Kullanım Şartları" />
      <KullanimSartlariMetni yuzey="panel" />
    </div>
  );
}
