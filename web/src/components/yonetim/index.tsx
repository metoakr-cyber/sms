/**
 * Yönetim bileşen katmanı — tek giriş noktası.
 *
 * NEDEN BARREL: bir yönetim ekranı bu katmandan tipik olarak 5-7 şey kullanır
 * (`SayfaBasligi` + `VeriTablosu` + `HataDurumu` + `Sayfalama` + `DurumRozeti`
 * + `SuzgecCubugu`). Yedi ayrı import satırı, ekranı bağlayan kişiyi bir
 * bileşeni "bulamayıp" yeniden yazmaya iter — kapatmaya çalıştığımız tekrar
 * tam olarak böyle doğar.
 *
 * 🔴 BU KATMAN `ui.tsx`'İN ÜSTÜNE KURULUR — onu YENİDEN YAZMAZ (§11.2).
 * `Button`, `Card`, `Field`, `Alert`, `Badge`, `Skeleton`, `Empty`, `Modal`
 * hâlâ `@/components/ui` ve `@/components/modal`'dan gelir; bu paket yalnız
 * YÖNETİM ekranlarına özgü bileşimleri taşır. Sebebi ölçülmüş: `Button` 139,
 * `Card` 63 kullanımın çoğu pazarlama yüzeyinde — yönetimi yoğunlaştırmak
 * için ortak ilkeli değiştirmek pazarlama sayfalarını da değiştirirdi.
 */

export { VeriTablosu } from './veri-tablosu';
export type {
  Sunum,
  Sutun,
  SutunHizasi,
  SutunOnceligi,
  MobilRol,
  VeriTablosuProps,
} from './veri-tablosu';

export { SayfaBasligi } from './sayfa-basligi';
export { HataDurumu, apiHatasi } from './hata-durumu';
export { Sayfalama, SAYFA_BOYUTU, sayfalamaGorunur } from './sayfalama';
export { useIstemciSuzgec } from './istemci-suzgec';
export type { IstemciSuzgecSonucu } from './istemci-suzgec';
export { DurumRozeti, durumTonu, ikiliTon } from './durum-rozeti';
export type { DurumTonu } from './durum-rozeti';
export { SuzgecCubugu, KayitSayaci } from './suzgec-cubugu';
export { OnayDiyalogu } from './onay-diyalogu';
export { Secim, CokSatir, AcilirOk } from './alanlar';
export { BolumGezinti, BOLUMLER, GENEL_BAKIS } from './bolum-gezinti';
