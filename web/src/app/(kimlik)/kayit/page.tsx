import type { Metadata } from 'next';
import RegisterForm from './form';

export const metadata: Metadata = {
  title: 'Ücretsiz Hesap Aç',
  description: 'SMS Onay hesabı açın. Abonelik yok, yalnız kullandığınız kadar ödersiniz.',
  alternates: { canonical: '/kayit' },
};

export default function RegisterPage() { return <RegisterForm />; }
