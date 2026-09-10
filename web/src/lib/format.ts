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

/* ═══════════════════════ Ondalık sayı (para DEĞİL) ═══════════════════════ */

/**
 * Sunucudan METİN olarak gelen ondalık sayıyı parçalar.
 *
 * 🔴 `Number()` KULLANILMAZ. Bu değerler (sağlayıcı çarpanı, marj yüzdesi)
 * doğrudan satış fiyatı zincirine giriyor; kayan noktaya çevirip geri yazmak
 * sessiz bir yuvarlama demektir. Ayrıştırma tamamen metin üzerinde yapılır.
 */
function ondalikParcala(ham: string): { isaret: string; tam: string; kesir: string } {
  const s = ham.trim();
  const isaret = s.startsWith('-') ? '-' : '';
  const govde = isaret ? s.slice(1) : s;
  const [tamHam = '', kesirHam = ''] = govde.split('.');
  return {
    isaret,
    // Baştaki sıfırlar atılır ama "0,95"in sıfırı KORUNUR (`|| '0'`).
    tam: tamHam.replace(/^0+(?=\d)/, '') || '0',
    // Sondaki sıfırlar anlamsızdır: 4.0000 → 4, 1.2500 → 1.25
    kesir: kesirHam.replace(/0+$/, ''),
  };
}

/**
 * Ondalık sayıyı TÜRKÇE gösterir: ayırıcı NOKTA değil VİRGÜLdür.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * 🔴 ÖLÇÜLMÜŞ KARIŞIKLIK — bu fonksiyonun var olma sebebi
 * ══════════════════════════════════════════════════════════════════════════
 * Yönetim ekranı çarpanı sunucudan geldiği gibi basıyordu: `4.0000`. Türkçede
 * `.` BİNLİK ayırıcıdır, yani kullanıcı ekranda "4" değil "kırk bin" gördü ve
 * "ben 4 yazmıştım, orada 4000 yazıyor" diye bildirdi. Değer doğruydu, YAZIM
 * yanlıştı — ve bu, fiyatı dört katına çıkaran bir alanda en pahalı yazım
 * hatasıdır.
 *
 * `Intl.NumberFormat` KULLANILMADI: girdiyi önce `Number`a çevirmek gerekirdi
 * (yukarıdaki gerekçe). Dönüşüm tamamen metinseldir ve değeri değiştirmez.
 *
 *   "4.0000" → "4"      "1.2500" → "1,25"      "0.9500" → "0,95"
 */
export function formatDecimal(ham: string | null | undefined): string {
  if (ham == null || ham.trim() === '') return '—';
  const { isaret, tam, kesir } = ondalikParcala(ham);
  return kesir ? `${isaret}${tam},${kesir}` : `${isaret}${tam}`;
}

/**
 * Form GİRDİSİ için sadeleştirme — ayırıcı NOKTA kalır.
 *
 * `formatDecimal`in aksine virgüle çevirmez: bu değer düzenleme kutusuna
 * konur ve oradan sunucuya AYNEN geri gider; sunucu Postgres `numeric`
 * bekliyor ve virgülü ayrıştıramaz. Yapılan tek şey anlamsız sondaki
 * sıfırları atmaktır — kullanıcı `4` yazıp kaydettikten sonra kutuyu tekrar
 * açtığında yine `4` görsün, `4.0000` değil.
 */
export function trimDecimal(ham: string | null | undefined): string {
  if (ham == null || ham.trim() === '') return '';
  const { isaret, tam, kesir } = ondalikParcala(ham);
  return kesir ? `${isaret}${tam}.${kesir}` : `${isaret}${tam}`;
}

/* ═══════════════════════ Telefon numarası ═══════════════════════ */

/**
 * Numarayı okunabilir gruplara böler: `+905347949267` → `+90 534 794 92 67`.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * NEDEN
 * ══════════════════════════════════════════════════════════════════════════
 * Kesintisiz 12 haneli bir dizi ekrandan okunamaz; kullanıcı numarayı başka
 * bir uygulamaya elle girerken hane atlar. Bu ekranda numara YANLIŞ girilirse
 * kod hiç gelmez ve kullanıcı bunu "sistem bozuk" diye okur.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * 🔴 KOPYALANAN DEĞER BU DEĞİLDİR
 * ══════════════════════════════════════════════════════════════════════════
 * Bu fonksiyon YALNIZ GÖSTERİM içindir. Kopyala düğmesi ham `phoneNumber`i
 * verir (boşluksuz E.164) — hedef uygulamaların tamamı boşluklu biçimi kabul
 * etmez ve panoya boşluk koymak sessiz bir hata kaynağıdır.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * ÜLKE KODU TAHMİN EDİLMEZ
 * ══════════════════════════════════════════════════════════════════════════
 * Ülke kodları 1-4 hane arasında değişir ve önekleri çakışır (+1, +7, +90,
 * +996…). Baştan kaç hane koparılacağını tahmin etmek yanlış bölme üretir.
 * Bu yüzden kod SUNUCUDAN gelen `phoneCode` ile eşleştirilir; gelmezse ya da
 * numara onunla başlamıyorsa numara OLDUĞU GİBİ döner — yanlış bölmektense
 * hiç bölmemek yeğdir.
 *
 * Ülkeye özel biçimler (libphonenumber) KULLANILMADI: 201 ülkenin kuralını
 * taşımak için ~150 KB'lık bir bağımlılık gerekir ve bu ekranın ihtiyacı
 * okunabilirlik, tipografik doğruluk değil. Gruplama soldan üçerli, son dört
 * hane ikişerli — Türkiye örneğiyle (3-3-2-2) birebir örtüşür.
 */
export function formatPhone(
  numara: string | null | undefined,
  ulkeKodu?: string | null,
): string {
  const ham = numara?.trim();
  if (!ham) return '—';

  const haneler = ham.replace(/\D/g, '');
  const kod = (ulkeKodu ?? '').replace(/\D/g, '');
  if (!kod || !haneler.startsWith(kod) || haneler.length <= kod.length) return ham;

  const ulusal = haneler.slice(kod.length);
  const gruplar: string[] = [];
  let kalan = ulusal;
  // Dörtten fazla hane kaldıkça üçerli al; kalan tam dört ise 2+2 yap.
  while (kalan.length > 4) {
    gruplar.push(kalan.slice(0, 3));
    kalan = kalan.slice(3);
  }
  if (kalan.length === 4) gruplar.push(kalan.slice(0, 2), kalan.slice(2));
  else if (kalan.length > 0) gruplar.push(kalan);

  return `+${kod} ${gruplar.join(' ')}`;
}

/**
 * Numarayı ülke kodu ve ULUSAL parçaya ayırır.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * NEDEN — ölçülmüş kullanıcı hatası
 * ══════════════════════════════════════════════════════════════════════════
 * WhatsApp/Telegram gibi uygulamalarda numara İKİ ALANA girilir: ülke ayrı bir
 * açılır listeden seçilir, kutuya YALNIZ ulusal kısım yazılır. Kullanıcı tam
 * E.164 numarayı (`+491523456789`) kopyalayıp o kutuya yapıştırdığında hedef
 * uygulama `+49` + `+491523456789` görür ve "numara hatalı" der.
 *
 * Hata kullanıcının değil arayüzün: ekran tek bir kopyalanabilir değer
 * sunuyorsa, yanlış yere yapıştırılması an meselesidir. Çözüm iki parçayı da
 * ayrı ayrı kopyalanabilir yapmaktır.
 *
 * Ülke kodu TAHMİN EDİLMEZ; sunucudan gelen `phoneCode` ile eşleştirilir
 * (gerekçe `formatPhone` başında). Eşleşmezse ulusal parça boş döner ve
 * çağıran yalnız tam numarayı gösterir — yanlış bölmektense hiç bölmemek yeğ.
 */
export function phoneParts(
  numara: string | null | undefined,
  ulkeKodu?: string | null,
): { ulkeKodu: string; ulusal: string } {
  const haneler = (numara ?? '').replace(/\D/g, '');
  const kod = (ulkeKodu ?? '').replace(/\D/g, '');
  if (!kod || !haneler.startsWith(kod) || haneler.length <= kod.length) {
    return { ulkeKodu: '', ulusal: '' };
  }
  return { ulkeKodu: `+${kod}`, ulusal: haneler.slice(kod.length) };
}
