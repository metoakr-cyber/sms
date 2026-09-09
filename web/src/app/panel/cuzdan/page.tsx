'use client';

/**
 * Cüzdan — bakiye ve hesap ekstresi.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * NE DEĞİŞTİ: ORTAK KATMANA TAŞINDI — DAVRANIŞ AYNI
 * ══════════════════════════════════════════════════════════════════════════
 * Ekran mobil kart listesi ve masaüstü tabloyu İKİ KEZ elle yazıyordu (~50
 * satır ikiz kod) ve etiketleri şimdiden AYRIŞMIŞTI: aynı sütun kartta
 * "Sonraki bakiye", tabloda "Bakiye". `VeriTablosu` bunu yapısal olarak
 * imkânsız kılar — `baslik` tek bir dizedir, hem `<th>` hem kart `<dt>` onu
 * okur. Seçilen tek metin "Sonraki bakiye"dir: alan `balanceAfter`'dır,
 * yani satırdan SONRAKİ bakiyedir; çıplak "Bakiye" hangi an olduğunu söylemez
 * ve bir para ekranında bu belirsizlik pahalıdır.
 *
 * Sayfalama, hata kutusu ve sayfa başlığı da ortak katmandan gelir. Kazanç
 * yalnız satır sayısı değil: hata durumu artık `requestId` GÖSTERİYOR (eski
 * kod düz bir `Alert` yazıyordu ve istek numarası yoktu — destek ekibinin
 * elinde hiçbir iz kalmıyordu).
 *
 * ══════════════════════════════════════════════════════════════════════════
 * SAYFA BOYUTU: 20 → `SAYFA_BOYUTU` (25)
 * ══════════════════════════════════════════════════════════════════════════
 * Sunucu sınırı: `StatementFilter.Normalize` limiti 1–100 arasında kabul eder
 * (api/internal/service/wallet/filter.go:20), varsayılanı 20'dir. 25 sınır
 * içindedir. Panelde tek bir sayfa boyutu olması, "kaç kayıt kaldı" sorusunun
 * ekrandan ekrana aynı cevaplanması demektir (§6.3).
 *
 * HAREKET YOK: bu ekranda tek geçiş `VeriTablosu`nun satır hover RENGİdir
 * (120ms) ve `Button`ın basma geri bildirimidir. İkisi de jetondan gelir.
 */

import * as React from 'react';
import Link from 'next/link';
import { keepPreviousData, useQuery } from '@tanstack/react-query';
import { apiFetch } from '@/lib/api';
import { formatDateTime, formatMoney } from '@/lib/format';
import { useSession } from '@/hooks/useSession';
import { Button, Card, Empty, cx } from '@/components/ui';
import {
  KayitSayaci,
  SAYFA_BOYUTU,
  SayfaBasligi,
  Sayfalama,
  VeriTablosu,
  apiHatasi,
  type Sutun,
} from '@/components/yonetim';
import type { LedgerEntry, Statement } from '@/lib/types';

/**
 * Sütun tanımı — bileşen DIŞINDA, modül düzeyinde.
 *
 * Satırlar bu ekranın hiçbir state'ine bakmaz (eylem yok, seçim yok), bu
 * yüzden dizi her render'da yeniden kurulmaz. `VeriTablosu` sütunları
 * `map`'lediği için değişmeyen bir referans gereksiz yeniden çizimi de önler.
 */
const SUTUNLAR: ReadonlyArray<Sutun<LedgerEntry>> = [
  {
    anahtar: 'tarih',
    baslik: 'Tarih',
    // `sayisal`: tarih sütunu da `tabular-nums` taşır (§3.5) — orantılı
    // rakamlarda "1" dar olduğu için alt alta gelen tarihler hizasız kayar.
    sayisal: true,
    hucre: (e) => <span className="text-muted">{formatDateTime(e.createdAt)}</span>,
  },
  {
    anahtar: 'islem',
    baslik: 'İşlem',
    // Kartın üst satırı, solda: satırın kimliği "ne oldu" sorusudur.
    mobilRol: 'baslik',
    // Etiket SUNUCUDAN gelir (`typeLabel`). İstemcide ikinci bir Türkçe
    // sözlük kurulmaz — kurulursa iki yerde iki farklı ad doğar.
    hucre: (e) => <span className="font-medium">{e.typeLabel}</span>,
  },
  {
    anahtar: 'aciklama',
    baslik: 'Açıklama',
    hucre: (e) =>
      e.note ? (
        <span className="break-anywhere">{e.note}</span>
      ) : (
        <span className="text-muted">—</span>
      ),
  },
  {
    anahtar: 'tutar',
    baslik: 'Tutar',
    hizala: 'sag',
    sayisal: true,
    // Kartın üst satırı, sağda — eski kartın da tam olarak yaptığı yerleşim.
    mobilRol: 'rozet',
    /*
      ANLAM YALNIZ RENKLE TAŞINMAZ (§7.1): işaret METNİN parçasıdır.
      `formatMoney` eksi tutarı "-123,45 ₺" olarak yazar; artıya "+" biz
      ekleriz. Renk yalnız pekiştirir. Kırmızı-yeşil ayırt edemeyen kullanıcı
      işareti okur.
    */
    hucre: (e) => (
      <span
        className={cx(
          'font-semibold',
          e.amount.minor < 0 ? 'text-[var(--color-bad)]' : 'text-[var(--color-ok)]',
        )}
      >
        {e.amount.minor > 0 ? '+' : ''}
        {formatMoney(e.amount)}
      </span>
    ),
  },
  {
    anahtar: 'bakiye',
    // 🔴 TEK METİN. Eski kod kartta "Sonraki bakiye", tabloda "Bakiye"
    // yazıyordu — ölçülmüş etiket ayrışması (§6).
    baslik: 'Sonraki bakiye',
    hizala: 'sag',
    sayisal: true,
    hucre: (e) => formatMoney(e.balanceAfter),
  },
];

export default function CuzdanSayfasi() {
  const { user } = useSession();
  const [offset, setOffset] = React.useState(0);

  const q = useQuery({
    queryKey: ['statement', { limit: SAYFA_BOYUTU, offset }],
    queryFn: () =>
      apiFetch<Statement>(`/wallet/entries?limit=${SAYFA_BOYUTU}&offset=${offset}`),
    // Sayfa değişince liste boşalıp zıplamasın; eski veri yenisi gelene dek kalır.
    placeholderData: keepPreviousData,
  });

  const toplam = q.data?.total ?? 0;
  // Sayfalı listede `VeriTablosu`nun "N kayıt listelendi" duyurusu YANILTICIDIR:
  // N sayfadaki satır sayısıdır, toplam değil. O durumda duyuruyu `Sayfalama`
  // yapar ve ekran okuyucu tek bir doğru cümle duyar.
  const sayfali = toplam > SAYFA_BOYUTU;

  return (
    <div className="mx-auto flex max-w-5xl flex-col gap-6">
      <SayfaBasligi
        baslik="Cüzdan"
        aciklama="Kullanılabilir bakiyeniz ve hesabınızdaki tüm para hareketleri."
      />

      {/*
        BAKİYE VE YÜKLEME TEK KARTTA. "Param ne kadar" ile "nasıl artırırım"
        arasına başka içerik girmesi, en sık yapılan işi aşağı iter. Düğme
        açıklamanın ÜSTÜNDEDİR: açıklama okunmadan da yükleme başlatılabilmeli.
      */}
      <Card>
        {/*
          `text-sm`, `text-xs` DEĞİL (§3.2) ve `uppercase` YOK.
          🔴 CSS `text-transform: uppercase` Türkçede `i → I` üretir, `İ` değil:
          "Kullanılabilir bakiye" → "KULLANILABILIR BAKIYE". Etiketi değerden
          ayıran şey ağırlık ve renktir (§3.3), harf biçimi değil.
        */}
        <p className="text-sm font-medium text-muted">Kullanılabilir bakiye</p>

        {/*
          `text-3xl` tavandır — `text-4xl` ve üstü YALNIZ pazarlamadır (§3.1).
          `tabular-nums`: bakiye her istekte yeniden çizilir; orantılı
          rakamlarda sayı genişliği değişir ve değer ZIPLAR (§3.5).
        */}
        <p className="mt-2 text-3xl font-bold tabular-nums">{formatMoney(user?.balance)}</p>

        <Link href="/panel/bakiye-yukle" className="mt-4 block sm:inline-block">
          <Button fullWidth className="sm:w-auto">
            Bakiye yükle
          </Button>
        </Link>

        {/* 70ch: düz metin satır uzunluğu 65–75ch bandında kalır (§3.4). */}
        <p className="mt-4 max-w-[70ch] text-sm leading-relaxed text-muted">
          Banka havalesi/EFT veya USDT ile yükleme yapabilirsiniz. Ödemeniz
          kontrol edildikten sonra bakiyeniz hesabınıza tanımlanır.
        </p>
      </Card>

      <Card>
        <div className="flex flex-wrap items-center justify-between gap-4">
          {/* `text-lg font-semibold`: h2 her ekranda aynı boyuttadır (§3.3). */}
          <h2 className="text-lg font-semibold">Hesap ekstresi</h2>
          {/* Sayaç `toplam <= 0` iken kendini çizmez; koşul burada tekrarlanmaz. */}
          <KayitSayaci toplam={toplam} />
        </div>

        <VeriTablosu
          className="mt-6"
          baslik="Hesap ekstresi"
          sutunlar={SUTUNLAR}
          satirlar={q.data?.items}
          // Kararlı kimlik: `public_id`. Dizin KULLANILMAZ — sayfa değişince
          // React aynı dizindeki farklı kaydı "aynı satır" sanar.
          satirAnahtari={(e) => e.id}
          yukleniyor={q.isLoading}
          // `HataDurumu` içeride çizilir ve `requestId`'yi GÖSTERİR.
          hata={apiHatasi(q.error)}
          duyuru={!sayfali}
          bos={
            /*
              §6.3 · `operate.md:35`: boş durum ARAYÜZÜ ÖĞRETİR, "burada bir şey
              yok" demez. Bu listede süzgeç olmadığı için tek bir boş hâl vardır
              ve doğru eylem bellidir: ilk hareketi kullanıcı bakiye yükleyerek
              üretir. `Empty`'nin eylem yuvası olmadığı için düğme dışarıda
              durur (ui.tsx bu dalgada değiştirilmez).
            */
            <div className="flex flex-col items-center gap-4">
              <Empty
                title="Ekstre boş"
                hint="Bakiye yüklediğinizde ya da numara satın aldığınızda her hareket tarihi ve tutarıyla buraya yazılır."
              />
              <Link href="/panel/bakiye-yukle">
                <Button variant="outline">Bakiye yükle</Button>
              </Link>
            </div>
          }
        />

        <Sayfalama
          className="mt-6"
          offset={offset}
          limit={SAYFA_BOYUTU}
          toplam={toplam}
          onDegis={setOffset}
        />
      </Card>
    </div>
  );
}
