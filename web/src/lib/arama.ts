/**
 * Türkçe duyarsız arama anahtarı.
 *
 * 🔴 `toLowerCase()` TEK BAŞINA YETMEZ ve `toLocaleLowerCase('tr')` TEK BAŞINA
 * DA YETMEZ — ikisi iki AYRI hatayı üretir:
 *
 *   `"İSTANBUL".toLowerCase()`            → `"i̇stanbul"` (i + birleşik nokta)
 *   `"Instagram".toLocaleLowerCase('tr')` → `"ınstagram"` (NOKTASIZ ı)
 *
 * İkincisi bu projede canlı veriyle doğrulandı: kullanıcı "instagram" yazıyor,
 * katalogdaki ad "Instagram+Threads" ve Türkçe yerelde "ınstagram"a dönüyor;
 * `includes` tutmuyor ve EN POPÜLER SERVİS aramada bulunamıyor.
 *
 * Çözüm: her iki tarafı da AYNI fonksiyondan geçirmek ve noktalı/noktasız
 * ayrımını tümüyle KALDIRMAK. Arama eşleştirmesi dilbilgisel doğruluk değil,
 * kullanıcının yazdığını bulmaktır.
 *
 * Diğer Türkçe harfler de katlanıyor (ş→s, ğ→g, ü→u, ö→o, ç→c): Türkçe klavyesi
 * olmayan ya da acele eden kullanıcı "sikayet" yazıp "Şikayet"i bulabilsin.
 *
 * Sorgu ve hedef AYNI fonksiyondan geçmezse hiçbir şey çalışmaz — tek kullanım
 * biçimi budur.
 *
 * ⚠️ BU FONKSİYONUN BİRİM TESTİ YOK, çünkü `web/` içinde bir birim test
 * koşucusu (vitest/jest) kurulu değil — yalnız Playwright uçtan uca testleri
 * var. Ölçülen davranış (2026-09-09, gerçek katalog adlarıyla):
 *
 *   "Instagram+Threads"    ← "instagram"  ✓   (eski davranışta ✗)
 *   "İSTANBUL"             ← "istanbul"   ✓
 *   "Şikayet Var"          ← "sikayet"    ✓
 *   "Google,youtube,Gmail" ← "gmail"      ✓
 *   "TikTok/Douyin"        ← "tiktok"     ✓
 *
 * Koşucu kurulduğu gün bu tablo teste çevrilmelidir.
 */
export function aramaAnahtari(s: string): string {
  return s
    .toLocaleLowerCase('tr')
    .replace(/[ıİI]/g, 'i')
    .replace(/ş/g, 's')
    .replace(/ğ/g, 'g')
    .replace(/ü/g, 'u')
    .replace(/ö/g, 'o')
    .replace(/ç/g, 'c');
}
