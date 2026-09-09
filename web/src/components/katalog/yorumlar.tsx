/**
 * Sitedeki müşteri yorumları — VERİTABANINDAN.
 *
 * 🔴 UYDURMA YORUM YOKTUR ve olmayacaktır. Bu bileşen yalnız
 * `GET /catalog/reviews` uç noktasını okur; o uç da YALNIZ yöneticinin
 * onayladığı yorumları döner (api/queries/reviews.sql, ListApprovedReviews).
 * Onaylı yorum yoksa bileşen `null` döner ve bölüm HİÇ RENDER EDİLMEZ —
 * boş bir "Kullanıcılar ne diyor?" başlığı, hiç yorum olmadığını duyurmanın
 * en gürültülü yoludur.
 *
 * 🔴 KULLANICI E-POSTASI GÖSTERİLMEZ. Yanıtta böyle bir alan yoktur; sunucu
 * sorgusu e-postayı SEÇMEZ bile.
 * test: api/internal/transport/http/handler/review_integration_test.go#TestPublicReviewsNeverExposeEmail
 *
 * 🔴 `dangerouslySetInnerHTML` BU DOSYADA YOKTUR. Yorum metni kullanıcı
 * girdisidir; React'in varsayılan kaçırması tek savunmadır ve yeterlidir.
 * Satır sonları CSS ile (`whitespace-pre-line`) korunur, HTML ile değil.
 *
 * Sunucu bileşenidir: yorumlar ilk HTML'de gelir, arama motoru görür.
 */

import { fetchPublic } from '@/lib/server-api';
import { Yorumlar } from '@/components/pazarlama/yorumlar';
import type { Yorum } from '@/data/yorumlar';

/** Sunucu sözleşmesi — karşılığı: api/internal/transport/http/dto/review.go */
export interface ApiYorum {
  id: string;
  rating: number;
  body: string;
  /** Kullanıcı adı (harf/rakam/alt çizgi). E-POSTA DEĞİLDİR. */
  authorName: string;
  publishedAt?: string;
}

export interface ApiYorumListesi {
  items: ApiYorum[];
  total: number;
  /**
   * Ortalama puanın ONDA BİRLİK TAM SAYI hâli (4.7 → 47).
   *
   * Sunucu kayan nokta göndermez: `4.699999999999999` gibi bir değer
   * arayüzde iki ayrı yerde iki ayrı yuvarlanırdı. Bölme burada, TEK YERDE
   * yapılır.
   */
  averageX10: number;
}

/** Puanı `Yorum.puan`ın kabul ettiği dar tipe daraltır. */
function puanDarat(n: number): Yorum['puan'] {
  switch (n) {
    case 1: case 2: case 3: case 4: case 5:
      return n;
    default:
      // Sunucu 1–5 dışında bir puan üretemez (CHECK + DTO doğrulaması).
      // Yine de `undefined` döneriz: uydurulmuş bir yıldız göstermektense
      // hiç yıldız göstermemek doğrudur.
      return undefined;
  }
}

/**
 * Onaylı yorumları çeker.
 *
 * API kapalıysa `null` döner ve bölüm görünmez — pazarlama sayfası bir arka
 * uç arızasında 500'e düşmez (server-api.ts ile aynı gerekçe).
 */
export async function onayliYorumlariGetir(): Promise<ApiYorumListesi | null> {
  // 300 sn: yorum listesi saniyelik tazelik gerektirmez, ama yeni onaylanan
  // bir yorumun sitede görünmesi de saatler almamalı.
  return fetchPublic<ApiYorumListesi>('/catalog/reviews', { revalidate: 300 });
}

/** API yanıtını pazarlama bileşeninin beklediği biçime çevirir. */
export function yorumlariEsle(liste: ApiYorumListesi | null): Yorum[] {
  if (!liste?.items?.length) return [];
  return liste.items.map((y) => ({
    ad: y.authorName,
    metin: y.body,
    puan: puanDarat(y.rating),
    // `kaynak` ve `izinTarihi` İÇ KAYITTIR, arayüzde gösterilmez. Elle
    // doldurulan dosyada "izni nereden aldık" sorusunun cevabıydı; burada
    // cevap açık: yorumu kullanıcı kendi panelinden yazdı ve yönetici
    // yayımlanmasını onayladı.
    kaynak: `review:${y.id}`,
    izinTarihi: y.publishedAt ?? '',
  }));
}

/**
 * Sitedeki yorum bölümü — kendi verisini çeker.
 *
 * Ana sayfada `<Yorumlar />` yerine BU bileşen kullanılmalıdır; görünüm aynı
 * (aynı kart bileşenini çağırır), fark verinin nereden geldiğidir.
 */
export async function MusteriYorumlari() {
  const liste = await onayliYorumlariGetir();
  const yorumlar = yorumlariEsle(liste);
  // Boş liste → bölüm YOK. Yer tutucu, iskelet ya da örnek yorum konmaz.
  if (yorumlar.length === 0) return null;
  return <Yorumlar yorumlar={yorumlar} />;
}
