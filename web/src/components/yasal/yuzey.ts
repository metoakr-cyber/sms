/**
 * Hukuki metinlerin İKİ yüzeyi — aynı metin, iki kap.
 *
 *   `genel` → `/kullanim-sartlari`, `/gizlilik`      (pazarlama sitesi, indekslenir)
 *   `panel` → `/panel/kullanim-sartlari`, `/panel/gizlilik`
 *             (oturum arkası kopya; kullanıcı paneli terk etmeden okur)
 *
 * NEDEN BİR YARDIMCI: metnin İÇİNDE kardeş hukuki sayfaya bağlantı var
 * (şartlar → gizlilik ve tersi). Panelde okuyan kullanıcıyı `/gizlilik`e
 * göndermek onu kabuktan çıkarır — yani "panelde açılsın" isteğinin tam
 * tersini yapar. Bağlantı bu yüzden yüzeye göre burada kurulur; iki metin
 * bileşeni de yolu elle yazmak yerine bu fonksiyonu çağırır.
 *
 * `/sss` ve `/iletisim` bu yardımcıdan GEÇMEZ: panel karşılıkları yok, ikisi
 * de her yüzeyde genel siteye gider.
 */
export type YasalYuzey = 'genel' | 'panel';

/** Kardeş hukuki sayfalar. Yol parçası her iki yüzeyde de aynı ada dayanır. */
export type YasalSayfa = 'gizlilik' | 'kullanim-sartlari';

export function yasalYol(yuzey: YasalYuzey, sayfa: YasalSayfa): string {
  return yuzey === 'panel' ? `/panel/${sayfa}` : `/${sayfa}`;
}
