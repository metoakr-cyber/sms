/**
 * DurumRozeti — durum kodu → ton + biçim. TEK harita.
 *
 * KAPATTIĞI TEKRAR: `statusTone` 4 yönetim + 3 panel ekranında kopyalanmıştı
 * (`talepler:50` `destek:89` `yorumlar:66` `panel/destek:78` `panel/yorumlarim:63`
 * `panel/bakiye-yukle:40`), üstelik ALTISINDAN BİRİ fonksiyon değil `Record`'du
 * ve dönüş tipleri farklıydı. İki ekran haritayı hiç kurmayıp satır içi
 * `p.isActive ? 'ok' : 'neutral'` yazıyordu.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * 🔴 ANLAM YALNIZ RENKLE TAŞINMAZ (tasarim-sistemi.md §7.1)
 * ══════════════════════════════════════════════════════════════════════════
 * Rozet ÜÇ kanal taşır:
 *   1. METİN  — sunucunun `statusLabel` alanı ("Onaylandı", "Bekliyor"). Asıl
 *               kanal budur ve her zaman görünür.
 *   2. BİÇİM  — tona göre farklı bir SVG (onay / saat / çarpı / çizgi).
 *               Kırmızı-yeşil ayırt edemeyen kullanıcı biçimi ayırt eder.
 *   3. RENK   — yalnız pekiştirir; tek başına hiçbir şey söylemez.
 * Emoji/unicode karakter (✓ ⚠ ×) İKON YERİNE KULLANILAMAZ (§9.2) — hepsi
 * 24 ızgarada, tek stroke genişliğinde SVG'dir.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * 🔴 `brand` TONU DURUM İÇİN KULLANILMAZ
 * ══════════════════════════════════════════════════════════════════════════
 * `Badge tone="brand"` bugün BEŞ ayrı anlam taşıyor: durum, rol, yetenek
 * etiketi, önizleme kaynağı ve sayaç (§5.4). Aynı renk beş şey söylüyorsa
 * hiçbir şey söylemiyordur. §5.4 durum için YALNIZ dört ton tanımlar:
 * ok · warn · bad · neutral. Bu yüzden `destek` ekranındaki
 * `USER_REPLIED → 'brand'` eşlemesi burada `warn`'a taşındı: "kullanıcı
 * yanıtladı" bir dikkat durumudur, bir marka vurgusu değil. Ayrımı metin
 * yapar ("Açık" ≠ "Kullanıcı yanıtladı"), renk değil.
 */
import * as React from 'react';
import { Badge } from '@/components/ui';

export type DurumTonu = 'ok' | 'warn' | 'bad' | 'neutral';

/**
 * Durum kodu → ton. §5.4'ün TEK eşleme tablosu.
 *
 * Kodlar sunucudan gelir ve tümü büyük harflidir. Bilinmeyen bir kod
 * `neutral` döner — YANLIŞ RENK GÖSTERMEKTENSE renksiz göstermek yeğdir;
 * yeni bir durum eklendiğinde yeşil görünen bir hata durumu, sessiz ve
 * pahalı bir yanlış anlamadır.
 */
export function durumTonu(durum: string): DurumTonu {
  switch (durum) {
    // Tamamlandı, onaylandı, aktif
    case 'COMPLETED':
    case 'APPROVED':
    case 'ANSWERED':
    case 'ACTIVE':
    case 'RECEIVED':
      return 'ok';

    // Bekliyor, dikkat gerektiriyor
    case 'PENDING':
    case 'OPEN':
    case 'USER_REPLIED':
    case 'WAITING':
      return 'warn';

    // Reddedildi, başarısız, eksik
    case 'REJECTED':
    case 'FAILED':
    case 'CANCELLED':
    case 'CANCELED':
    case 'EXPIRED':
      return 'bad';

    // Nötr, kapalı, arşiv (CLOSED, REFUNDED, INACTIVE, bilinmeyen)
    default:
      return 'neutral';
  }
}

/** Etkin/pasif gibi ikili alanlar için — `isActive ? 'ok' : 'neutral'` satır içi yazılmasın. */
export function ikiliTon(etkin: boolean): DurumTonu {
  return etkin ? 'ok' : 'neutral';
}

export function DurumRozeti({
  durum,
  etiket,
  ton,
}: {
  /** Sunucudan gelen ham kod ("COMPLETED"). Ton bundan türer. */
  durum: string;
  /**
   * Sunucudan gelen Türkçe etiket (`statusLabel`). ZORUNLUDUR: rozetin asıl
   * anlam kanalı budur. Kodu Türkçeleştirmeyi istemciye bırakmak, iki yerde
   * iki farklı sözlük demektir — sunucu zaten doğrusunu gönderiyor.
   */
  etiket: string;
  /** Nadiren: türetilen tonu ezmek gerekirse (örn. ikili alanlar). */
  ton?: DurumTonu;
}) {
  const t = ton ?? durumTonu(durum);
  return (
    <Badge tone={t}>
      <DurumIkonu ton={t} />
      {etiket}
    </Badge>
  );
}

/**
 * Tona göre BİÇİM. `Badge` `inline-flex items-center` olduğu için ikon metinle
 * hizalanır; `me-1` (4px) ikon–metin aralığıdır (§2.3 boşluk ölçeği).
 * `size-3` (12px) rozetin 12px metnine oranlıdır ve rozeti büyütmez.
 */
function DurumIkonu({ ton }: { ton: DurumTonu }) {
  return (
    <svg
      viewBox="0 0 24 24"
      className="me-1 size-3 shrink-0"
      fill="none"
      stroke="currentColor"
      strokeWidth="2.5"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
    >
      {ton === 'ok' && <path d="M4 12.5l5.5 5.5L20 6" />}
      {ton === 'warn' && (
        <>
          <circle cx="12" cy="12" r="9" />
          <path d="M12 7v5.5l3.5 2" />
        </>
      )}
      {ton === 'bad' && <path d="M6 6l12 12M18 6L6 18" />}
      {ton === 'neutral' && <path d="M5 12h14" />}
    </svg>
  );
}
