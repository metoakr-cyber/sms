import type { Metadata } from 'next';
import { ForgotForm } from './form';

/*
 * Sayfa SUNUCU bileşeni, form İSTEMCİ bileşeni.
 *
 * 🔴 NEDEN BÖLÜNDÜ: dosya tümüyle 'use client' iken `metadata` dışa
 * AKTARILAMIYORDU ve sayfa sessizce kök layout'un varsayılanlarını miras
 * alıyordu. Sonuç üç ayrı hata:
 *   · canonical "/" gösteriyordu — Google bu sayfayı ana sayfayla birleştirir
 *   · robots "index, follow" idi — kimlik sayfaları noindex olmalı (§10.1)
 *   · title ve description /gizlilik ile BİREBİR aynıydı (S1 ihlali)
 * Otomatik SEO denetimi (scripts/seo-check.mjs) bunu yakaladı.
 * Aynı bölme deseni giris/ ve kayit/ dizinlerinde de var.
 */
export const metadata: Metadata = {
  title: 'Şifremi Unuttum',
  description: 'Onay360 hesabınızın şifresini e-posta ile sıfırlayın.',
  alternates: { canonical: '/sifremi-unuttum' },
  robots: { index: false, follow: true },
};

export default function ForgotPage() {
  return <ForgotForm />;
}
