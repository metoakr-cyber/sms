import type { Metadata } from 'next';
import { KullanimSartlariMetni } from '@/components/yasal/kullanim-sartlari';

export const metadata: Metadata = {
  // Sayfaya ÖZEL description şart: yoksa kök layout'un varsayılanı miras
  // alınır ve iki ayrı sayfa birebir aynı açıklamayı taşır (S1 ihlali).
  description:
    'Onay360 kullanım şartları: üyelik, bakiye, sipariş, kiralık numara, ' +
    'iptal ve iade kuralları, yasak kullanımlar ve sorumluluğun sınırı.',
  title: 'Kullanım Şartları',
  alternates: { canonical: '/kullanim-sartlari' },
};

/**
 * `/kullanim-sartlari` — metnin GENEL (indekslenen) yüzeyi.
 *
 * 🔴 METİN BURADA DEĞİL: `@/components/yasal/kullanim-sartlari` içinde. Aynı
 * metin panelde de görünüyor (`/panel/kullanim-sartlari`) ve iki dosyada
 * durursa biri güncellenip diğeri unutulur. Ayrıştırmanın tam gerekçesi o
 * dosyanın başlığında.
 *
 * Bu sayfanın kendine ait olan üç şeyi var: SEO üstverisi, pazarlama
 * ölçeğindeki `h1` ve yerleşim kabı (`max-w-3xl`). Panel yüzeyinde üçü de
 * farklıdır — panelde içerik tam genişliktir ve başlığı `SayfaBasligi` çizer.
 */
export default function TermsPage() {
  return (
    <div className="px-4 py-10 md:px-6 md:py-14">
      <div className="mx-auto max-w-3xl">
        <h1 className="text-3xl font-bold tracking-tight md:text-4xl">Kullanım Şartları</h1>
        <KullanimSartlariMetni yuzey="genel" className="mt-6" />
      </div>
    </div>
  );
}
