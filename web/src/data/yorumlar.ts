/**
 * MÜŞTERİ YORUMLARI — GERÇEK VERİ DOSYASI.
 *
 * 🔴 BU DOSYA BUGÜN BİLEREK BOŞTUR.
 *
 * Buraya YALNIZ gerçekten alınmış, kaynağı belli yorumlar yazılır. Uydurma
 * bir isim ya da "harika hizmet" cümlesi eklemek üç ayrı sorundur:
 *
 *   1. TÜKETİCİ HUKUKU — Türkiye'de Ticari Reklam ve Haksız Ticari
 *      Uygulamalar Yönetmeliği uyarınca doğrulanamayan tüketici beyanı
 *      yayımlamak yanıltıcı reklamdır; idari para cezası konusudur.
 *   2. KİŞİSEL VERİ — gerçek bir müşterinin adını, işini ya da fotoğrafını
 *      AÇIK RIZASI OLMADAN yayımlamak KVKK ihlalidir. Rıza yazılı alınır ve
 *      `kaynak` alanına nereden alındığı yazılır.
 *   3. GÜVEN — para yatırılan bir sitede uydurma yorum yakalandığı an,
 *      kullanıcının aklına gelen ilk soru "başka ne uydurdular" olur.
 *
 * Dizi boşken ana sayfadaki yorum bölümü HİÇ RENDER EDİLMEZ — boş bir
 * "Müşterilerimiz ne diyor?" başlığı bırakılmaz.
 *
 * 🔗 KALICI ÇÖZÜM: `api/internal/service/review` (bu oturumda yazılıyor)
 * gerçek yorumları veritabanında tutuyor ve `GET /catalog/reviews` YALNIZ
 * ONAYLI olanları döndürüyor. O uç canlıya çıktığında ana sayfa listeyi
 * `fetchPublic` ile çekip `<Yorumlar yorumlar={...} />` şeklinde geçmeli;
 * bu dosya elle doldurulacak bir yer OLMAKTAN ÇIKAR. Uç hazır olana kadar
 * burası bilerek boştur — geçici diye uydurma yorum yazılmaz.
 *
 * Doldururken:
 *   · `ad` — kişinin yayımlanmasına izin verdiği ad (kısaltma olabilir: "M. Yılmaz")
 *   · `unvan` — isteğe bağlı, izin verilmişse
 *   · `metin` — kişinin kendi cümlesi; düzeltilmez, kısaltılacaksa […] ile
 *   · `puan` — 1–5, kişinin verdiği puan; yoksa alanı hiç yazmayın
 *   · `kaynak` — yorumun nereden geldiği (destek talebi no, e-posta tarihi…)
 *   · `izinTarihi` — yayın izninin alındığı tarih (RFC 3339)
 */

export type Yorum = {
  ad: string;
  unvan?: string;
  metin: string;
  puan?: 1 | 2 | 3 | 4 | 5;
  /** İç kayıt — arayüzde GÖSTERİLMEZ, denetim izi içindir. */
  kaynak: string;
  /** İç kayıt — arayüzde GÖSTERİLMEZ. */
  izinTarihi: string;
};

export const YORUMLAR: Yorum[] = [];
