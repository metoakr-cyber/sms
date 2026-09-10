import type { Metadata } from 'next';
import { GizlilikMetni } from '@/components/yasal/gizlilik';

export const metadata: Metadata = {
  // Sayfaya ÖZEL description şart: yoksa kök layout'un varsayılanı miras
  // alınır ve iki ayrı sayfa birebir aynı açıklamayı taşır (S1 ihlali).
  //
  // Metin, sayfanın GERÇEK içeriğini anlatır: eski açıklama yalnız silme
  // süresinden söz ediyordu, sayfa ise aktarım ve haklar da anlatıyor.
  // ≤160 karakter: seo-check.mjs bunu ÖLÇÜYOR ve uzunu düşürüyor. Uzun
  // açıklama arama sonucunda zaten kırpılır; kırpılan yer cümlenin ortasıdır.
  description:
    'Onay360 gizlilik politikası: hangi verileri neden işliyoruz, ' +
    'kimlerle paylaşıyoruz, ne kadar saklıyoruz ve haklarınızı nasıl kullanırsınız.',
  title: 'Gizlilik Politikası',
  alternates: { canonical: '/gizlilik' },
};

/**
 * `/gizlilik` — metnin GENEL (indekslenen) yüzeyi.
 *
 * 🔴 METİN BURADA DEĞİL: `@/components/yasal/gizlilik` içinde. Aynı metin
 * panelde de görünüyor (`/panel/gizlilik`); iki dosyada dursaydı biri
 * güncellenip diğeri unutulurdu. Ayrıştırmanın gerekçesi
 * `components/yasal/kullanim-sartlari.tsx` başlığında.
 *
 * Bu sayfanın kendine ait olan üç şeyi var: SEO üstverisi, pazarlama
 * ölçeğindeki `h1` ve yerleşim kabı (`max-w-3xl`).
 */
export default function PrivacyPage() {
  return (
    <div className="px-4 py-10 md:px-6 md:py-14">
      <div className="mx-auto max-w-3xl">
        <h1 className="text-3xl font-bold tracking-tight md:text-4xl">Gizlilik Politikası</h1>
        {/* mt-2: "Son güncelleme" satırı başlığa YAPIŞIK durur — o bir alt
            başlıktır, gövdenin ilk paragrafı değil. */}
        <GizlilikMetni yuzey="genel" className="mt-2" />
      </div>
    </div>
  );
}
