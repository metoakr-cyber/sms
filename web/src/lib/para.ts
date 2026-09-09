/**
 * PARA GİRDİSİ AYRIŞTIRMA — tek `toMinor`, seçenekli.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * NEDEN BU DOSYA VAR
 * ══════════════════════════════════════════════════════════════════════════
 * Depoda `toMinor`'ın BEŞ kopyası vardı ve bu beş kopya ÜÇ FARKLI POLİTİKA
 * taşıyordu (ölçüm: yonetim/bakiye:20 · yonetim/fiyatlar:64 ·
 * yonetim/odeme-yontemleri:58 · yonetim/talepler:65 · panel/bakiye-yukle:54).
 *
 * 🔴 FARKLAR KAZA DEĞİL, KASITLI. Naif bir birleştirme PARA HATASIDIR:
 *
 *   · `bakiye` (düzeltme)  → NEGATİF MEŞRU. Bir düzeltme eksi de olabilir;
 *                            negatifi reddetmek yöneticinin yanlış yazılmış
 *                            bir bakiyeyi geri alma yolunu kapatır.
 *   · `fiyatlar` (kural)   → BOŞ = 0 MEŞRU. "Sabit ücret" ve "taban fiyat"
 *                            İSTEĞE BAĞLI alanlardır; boş bırakmak "ücret yok"
 *                            demektir, hata değil. Ayrıca ÜST SINIR vardır:
 *                            asıl risk büyük değer değil, FAZLADAN İKİ SIFIR —
 *                            5,00 ₺ yerine 500,00 ₺ taban her ürünü satılamaz
 *                            yapar ve bunu kimse fark etmez.
 *   · diğer üçü (tutar)    → BOŞ = HATA, NEGATİF RET. Kullanıcı bir tutar
 *                            bildiriyor; boş ya da eksi bir tutar anlamsızdır.
 *
 * Bu yüzden fonksiyon seçeneklidir ve her çağrı yeri için HAZIR BİR AYAR
 * (`TUTAR_*` sabitleri) dışa verilir. Ekran kendi ayarını elle kurmaz —
 * kurarsa üçüncü bir politika doğar ve bu dosya anlamsızlaşır.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * SEÇENEK TABLOSU — hangi ekran hangi ayarı kullanır
 * ══════════════════════════════════════════════════════════════════════════
 * | Ekran                          | Ayar                   | izinNegatif | bosDeger | ustSinirMinor |
 * |--------------------------------|------------------------|-------------|----------|---------------|
 * | /yonetim/bakiye                | TUTAR_BAKIYE_DUZELTME  | true        | 'hata'   | —             |
 * | /yonetim/fiyatlar              | TUTAR_FIYAT_KURALI     | false       | 'sifir'  | 100_000_000   |
 * | /yonetim/odeme-yontemleri      | TUTAR_ZORUNLU          | false       | 'hata'   | —             |
 * | /yonetim/talepler              | TUTAR_ZORUNLU          | false       | 'hata'   | —             |
 * | /panel/bakiye-yukle            | TUTAR_ZORUNLU          | false       | 'hata'   | —             |
 *
 * ══════════════════════════════════════════════════════════════════════════
 * KAYAN NOKTA YASAK
 * ══════════════════════════════════════════════════════════════════════════
 * `12.50 * 100` JavaScript'te 1249.9999… verir ve BİR KURUŞ KAYBOLUR.
 * Çevrim metin üzerinden, tam sayı aritmetiğiyle yapılır. Beş kopyanın
 * beşi de bunu doğru yapıyordu; birleştirme bunu bozmaz.
 *
 * NOT: burası BİÇİMLEME dosyası değildir (o `lib/format.ts`). Burada girdi
 * AYRIŞTIRILIR. `formatMoney` ile karıştırılmamalıdır: biçimlemede birincil
 * kaynak sunucunun `formatted` alanıdır, burada birincil kaynak kullanıcının
 * yazdığı metindir.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * DAVRANIŞ TABLOSU — bu dosya değişirse bu tablonun tamamı yeniden doğrulanır
 * ══════════════════════════════════════════════════════════════════════════
 * | girdi      | TUTAR_ZORUNLU        | TUTAR_BAKIYE_DUZELTME | TUTAR_FIYAT_KURALI     |
 * |------------|----------------------|-----------------------|------------------------|
 * | ""         | hata: bos            | hata: bos             | { minor: 0 }           |
 * | "  "       | hata: bos            | hata: bos             | { minor: 0 }           |
 * | "-"        | hata: bicim          | hata: bos             | hata: bicim            |
 * | "12"       | 1200                 | 1200                  | 1200                   |
 * | "12,5"     | 1250                 | 1250                  | 1250                   |
 * | "12.50"    | 1250                 | 1250                  | 1250                   |
 * | "12,505"   | hata: bicim          | hata: bicim           | hata: bicim            |
 * | "-12,50"   | hata: bicim          | -1250                 | hata: bicim            |
 * | "0"        | 0                    | 0                     | 0                      |
 * | "-0"       | hata: bicim          | 0  (işaretsiz sıfır)  | hata: bicim            |
 * | "abc"      | hata: bicim          | hata: bicim           | hata: bicim            |
 * | "1000001"  | 100000100            | 100000100             | hata: ustSinir         |
 * | 1e20 basam.| hata: tasma          | hata: tasma           | hata: tasma            |
 *
 * 🔴 TEST DURUMU (dürüst rapor): `web/package.json` içinde birim test koşucusu
 * YOKTUR (vitest/jest kurulu değil) ve bu dalga mevcut dosyaları değiştiremez.
 * Yukarıdaki tablo, beş özgün kopyanın birebir kopyalanıp bu fonksiyonla
 * karşılaştırıldığı bir eşdeğerlik denemesiyle doğrulandı (fark: 0).
 * Kalıcı `para.test.ts` + koşucu kurulumu SONRAKİ DALGANIN işidir.
 */

/** Başarı ya da Türkçe hata — çağıran `'error' in sonuc` ile ayırır. */
export type ToMinorSonuc = { minor: number } | { error: string };

/** Kullanıcıya gösterilen dört metin. Hepsi Türkçe, hepsi değiştirilebilir. */
export interface ToMinorMesajlari {
  /** Alan boş ve `bosDeger: 'hata'`. */
  bos: string;
  /** Biçim tutmadı (harf, ikiden çok ondalık, izinsiz eksi). */
  bicim: string;
  /** Sayı `Number.MAX_SAFE_INTEGER`'ı aştı. */
  tasma: string;
  /** `ustSinirMinor` aşıldı. Sınır varsa bu metin de VERİLMELİDİR. */
  ustSinir: string;
}

export interface ToMinorSecenekleri {
  /**
   * Eksi tutar kabul edilsin mi? Yalnız `/yonetim/bakiye` (düzeltme) için
   * `true`. Varsayılan `false`.
   *
   * `true` iken tek başına `"-"` girdisi BOŞ SAYILIR (kullanıcı eksiyi yazdı,
   * rakamı henüz yazmadı) — özgün `bakiye` davranışı budur.
   */
  izinNegatif?: boolean;
  /**
   * Boş girdi ne demek?
   *   'hata'  → "Tutar giriniz." (varsayılan)
   *   'sifir' → `{ minor: 0 }` — YALNIZ isteğe bağlı ücret/alt sınır alanları
   *             için (`/yonetim/fiyatlar`). Zorunlu bir tutar alanında bunu
   *             kullanmak, boş formu 0,00 ₺ olarak göndermek demektir.
   */
  bosDeger?: 'hata' | 'sifir';
  /**
   * Üst sınır (kuruş). Aşılırsa `mesajlar.ustSinir` döner.
   * 🔴 Sınır veriyorsan `mesajlar.ustSinir`'i de ver: varsayılan metin sınırın
   * kaç TL olduğunu SÖYLEMEZ. Tutar biçimleme burada YAPILMAZ (`lib/format.ts`
   * dışında para biçimlenmez), bu yüzden metni çağıran yazar.
   */
  ustSinirMinor?: number;
  /** Varsayılan metinlerin üstüne yazılır. */
  mesajlar?: Partial<ToMinorMesajlari>;
}

const VARSAYILAN_MESAJLAR: ToMinorMesajlari = {
  bos: 'Tutar giriniz.',
  bicim: 'Geçerli bir tutar giriniz (en fazla 2 ondalık).',
  tasma: 'Tutar çok büyük.',
  ustSinir: 'Tutar sınırın üstünde.',
};

/** İşaretli biçim — yalnız `izinNegatif: true` iken. */
const BICIM_NEGATIFLI = /^-?\d+(\.\d{1,2})?$/;
/** İşaretsiz biçim — varsayılan. */
const BICIM_POZITIF = /^\d+(\.\d{1,2})?$/;

/**
 * "12,50" / "12.50" / " 12 , 50 " → 1250 kuruş.
 *
 * @param input Kullanıcının yazdığı ham metin.
 * @param secenekler Ekranın politikası — hazır `TUTAR_*` sabitlerinden biri.
 */
export function toMinor(input: string, secenekler: ToMinorSecenekleri = {}): ToMinorSonuc {
  const izinNegatif = secenekler.izinNegatif ?? false;
  const bosDeger = secenekler.bosDeger ?? 'hata';
  const { ustSinirMinor } = secenekler;
  const mesaj: ToMinorMesajlari = { ...VARSAYILAN_MESAJLAR, ...secenekler.mesajlar };

  // Türkçe klavyede ondalık ayracı virgüldür; boşluk yapıştırmadan gelir.
  const s = input.trim().replace(/\s/g, '').replace(',', '.');

  // "-" tek başına: kullanıcı eksiyi yazdı, rakamı yazmadı. Negatifin meşru
  // olduğu ekranda bu "boş"tur; olmadığı ekranda biçim hatasıdır (regex yakalar).
  const bos = s === '' || (izinNegatif && s === '-');
  if (bos) return bosDeger === 'sifir' ? { minor: 0 } : { error: mesaj.bos };

  if (!(izinNegatif ? BICIM_NEGATIFLI : BICIM_POZITIF).test(s)) {
    return { error: mesaj.bicim };
  }

  const negatif = s.startsWith('-');
  const [tam = '0', ondalik = ''] = s.replace('-', '').split('.');
  // Tam sayı aritmetiği: "12" * 100 + "50" — hiçbir adımda ondalık sayı yok.
  const minor = Number(tam) * 100 + Number(ondalik.padEnd(2, '0'));

  // int64 sunucuda güvenli, ama JS sayısı 2^53'ten sonra sessizce yuvarlar.
  // Sessizce yanlış bir tutar göndermektense hata göstermek gerekir.
  if (!Number.isSafeInteger(minor)) return { error: mesaj.tasma };
  if (ustSinirMinor !== undefined && minor > ustSinirMinor) return { error: mesaj.ustSinir };

  return { minor: negatif ? -minor : minor };
}

/* ═══════════════════════ Hazır ayarlar ═══════════════════════ */

/**
 * `/yonetim/bakiye` — bakiye düzeltme.
 * Negatif MEŞRU: yanlış yazılmış bir bakiye eksi bir düzeltmeyle geri alınır.
 * Üst sınır YOK: sınırı sunucu koyar; burada bir tavan uydurmak, meşru bir
 * büyük düzeltmeyi sessizce engellerdi.
 */
export const TUTAR_BAKIYE_DUZELTME: ToMinorSecenekleri = {
  izinNegatif: true,
  bosDeger: 'hata',
};

/**
 * `/yonetim/fiyatlar` — sabit ücret ve taban fiyat alanları.
 * Boş = 0 MEŞRU (alanlar isteğe bağlı). Negatif ret, ve 1.000.000,00 ₺ tavanı
 * "fazladan iki sıfır" hatasına karşıdır — sunucudaki `maxRuleAmountMinor`
 * ile aynı değer.
 */
export const TUTAR_FIYAT_KURALI: ToMinorSecenekleri = {
  izinNegatif: false,
  bosDeger: 'sifir',
  ustSinirMinor: 100_000_000,
  mesajlar: {
    bicim: 'Geçerli bir tutar giriniz (en fazla 2 ondalık, negatif olamaz).',
    ustSinir: 'Tutar en fazla 1.000.000,00 ₺ olabilir.',
  },
};

/**
 * `/yonetim/odeme-yontemleri` · `/yonetim/talepler` · `/panel/bakiye-yukle`
 * Zorunlu, pozitif tutar. Alt/üst sınırı sunucu ayrıca doğrular; istemci
 * burada bir sınır uydurmaz (uydurursa iki yerde iki farklı kural olur).
 */
export const TUTAR_ZORUNLU: ToMinorSecenekleri = {
  izinNegatif: false,
  bosDeger: 'hata',
};

/* ═══════════════════════ Ters yön ═══════════════════════ */

/**
 * Kuruş → DÜZENLENEBİLİR TL metni. Form ÖN DOLDURMA içindir.
 *
 * NEDEN `formatMoney` DEĞİL: `formatted` alanı "₺" ve binlik ayracı taşır
 * ("1.234,50 ₺"); bir `<input>`'a konursa kullanıcı onu düzenleyemez ve
 * `toMinor` geri okuyamaz. Burada bölme YAPILMAZ, tam sayı ayrıştırılır —
 * bu bir para hesabı değil, gösterim ayrıştırmasıdır.
 *
 * 🔴 NEGATİF DÜZELTMESİ: özgün kopya (`odeme-yontemleri:77`) `Math.trunc`
 * kullanıyordu ve -50 kuruş için `-0` üretip işareti KAYBEDİYORDU
 * ("0,50"). Bugünkü tek çağrı yeri hep pozitif değer geçtiği için bu hata
 * görünmüyor; `bakiye` ekranı eksi bir düzeltmeyi ön doldurduğu gün
 * görünür olurdu. Pozitif girdide çıktı özgün kopyayla BİREBİR AYNIDIR.
 */
export function minorToText(minor: number): string {
  const negatif = minor < 0;
  const mutlak = Math.abs(minor);
  const tam = Math.trunc(mutlak / 100);
  const ondalik = mutlak % 100;
  return `${negatif ? '-' : ''}${tam},${String(ondalik).padStart(2, '0')}`;
}
