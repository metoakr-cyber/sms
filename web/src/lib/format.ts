/** Biçimlendirme — docs/frontend-contract.md §5 (tarayıcı tuzakları). */

import type { Money } from './types';

/**
 * Para.
 *
 * BİRİNCİL KAYNAK SUNUCUDUR: `formatted` alanı sunucuda üretilir ve olduğu gibi
 * gösterilir. İstemcide yeniden hesaplamak, iki yerde iki farklı yuvarlama
 * demektir — kullanıcı bakiyesini sepette başka, ekstrede başka görür.
 *
 * Yerel biçimlendirme yalnızca YEDEKTİR (sunucu alanı boş gelirse).
 */
export function formatMoney(m: Money | null | undefined): string {
  if (!m) return '—';
  if (m.formatted) return m.formatted;

  // Yedek: minor birimden çevir. TRY ölçeği 2 (kuruş), USD ölçeği 6 (design.md §5).
  const scale = m.currency === 'USD' ? 6 : 2;
  const major = m.minor / 10 ** scale;
  try {
    return new Intl.NumberFormat('tr-TR', {
      style: 'currency', currency: m.currency,
      minimumFractionDigits: 2, maximumFractionDigits: 2,
    }).format(major);
  } catch {
    // Bilinmeyen para birimi kodunda Intl RangeError atar — sayfa çökmemeli.
    return `${major.toFixed(2)} ${m.currency}`;
  }
}

/** Sunucu sayıyı JSON'da güvenle taşır ama minor birim yine de int64'tür;
 *  2^53'ü aşan bir değeri sessizce bozmak yerine görünür kılarız. */
export function moneyIsSafe(m: Money): boolean {
  return Number.isSafeInteger(m.minor);
}

const TZ = 'Europe/Istanbul';

/**
 * Tarih.
 *
 * Girdi YALNIZ RFC 3339 olabilir: `new Date("2026-09-08 10:00")` iOS Safari'de
 * `Invalid Date` döner (boşluklu biçim ECMA-262'de tanımsızdır, Chrome hoşgörülü
 * davranır, Safari davranmaz). Sunucu her zaman RFC 3339 üretir.
 *
 * Saat dilimi AÇIKÇA verilir: kullanıcının cihazı yanlış dilimdeyse sipariş
 * saatleri kayar ve destek talebi olarak geri döner.
 */
export function parseServerDate(s: string | null | undefined): Date | null {
  if (!s) return null;
  const d = new Date(s);
  return Number.isNaN(d.getTime()) ? null : d;
}

export function formatDateTime(s: string | null | undefined): string {
  const d = parseServerDate(s);
  if (!d) return '—';
  return new Intl.DateTimeFormat('tr-TR', {
    dateStyle: 'medium', timeStyle: 'short', timeZone: TZ,
  }).format(d);
}

export function formatDate(s: string | null | undefined): string {
  const d = parseServerDate(s);
  if (!d) return '—';
  return new Intl.DateTimeFormat('tr-TR', { dateStyle: 'medium', timeZone: TZ }).format(d);
}

/** "3 dk 05 sn" — geri sayım için. Negatif değer 0 gösterir. */
export function formatDuration(totalSeconds: number): string {
  const s = Math.max(0, Math.floor(totalSeconds));
  const m = Math.floor(s / 60);
  const r = s % 60;
  return m > 0 ? `${m} dk ${String(r).padStart(2, '0')} sn` : `${r} sn`;
}
