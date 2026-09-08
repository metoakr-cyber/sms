import type { Metadata, Viewport } from 'next';
import { Inter } from 'next/font/google';
import { Providers, OfflineBanner } from '@/components/providers';
import { themeScript } from '@/components/theme';
import './globals.css';

// Self-host + Türkçe karakter alt kümesi (§2.7). `latin-ext` olmadan
// ğ ş ı ç ö ü yedek fontla çizilir ve satır yüksekliği zıplar.
const inter = Inter({
  subsets: ['latin', 'latin-ext'],
  display: 'swap',
  variable: '--font-inter',
});

const SITE = process.env.NEXT_PUBLIC_SITE_URL ?? 'http://localhost:3000';

export const metadata: Metadata = {
  metadataBase: new URL(SITE),
  title: {
    default: 'Onay360 — Geçici Numara ile Anında SMS Onayı',
    template: '%s · Onay360',
  },
  description:
    'WhatsApp, Telegram, Instagram ve yüzlerce servis için geçici numara ile ' +
    'anında SMS onay kodu alın. Kod gelmezse ücret iade edilir.',
  applicationName: 'Onay360',
  alternates: { canonical: '/' },
  openGraph: {
    type: 'website',
    locale: 'tr_TR',
    siteName: 'Onay360',
    url: SITE,
    title: 'Onay360 — Geçici Numara ile Anında SMS Onayı',
    description: 'Yüzlerce servis için geçici numara. Kod gelmezse ücret iade edilir.',
  },
  twitter: { card: 'summary_large_image' },
  robots: { index: true, follow: true },
  formatDetection: { telephone: false },
};

export const viewport: Viewport = {
  width: 'device-width',
  initialScale: 1,
  viewportFit: 'cover', // çentik alanı için env(safe-area-inset-*) çalışsın
  // maximumScale ve userScalable BİLEREK AYARLANMADI: yakınlaştırmayı kapatmak
  // az gören kullanıcılar için siteyi kullanılamaz hale getirir (WCAG 1.4.4).
  themeColor: [
    { media: '(prefers-color-scheme: dark)', color: '#0b1120' },
    { media: '(prefers-color-scheme: light)', color: '#f5f7fb' },
  ],
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="tr" className={inter.variable} suppressHydrationWarning>
      <head>
        {/* İlk boyamadan ÖNCE çalışmalı — aşağı taşınırsa beyaz patlama geri gelir. */}
        <script dangerouslySetInnerHTML={{ __html: themeScript }} />
      </head>
      <body className="min-h-screen-safe">
        {/* Klavye kullanıcısı için içeriğe atlama — her sayfada ilk odak */}
        <a
          href="#icerik"
          className="sr-only focus:not-sr-only focus:absolute focus:left-4 focus:top-4 focus:z-50
                     focus:rounded-lg focus:bg-brand-500 focus:px-4 focus:py-2 focus:text-white"
        >
          İçeriğe atla
        </a>
        <Providers>
          <OfflineBanner />
          {children}
        </Providers>
      </body>
    </html>
  );
}
