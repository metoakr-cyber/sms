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
import { Button, Card, Empty, Field, cx } from '@/components/ui';
import {
  KayitSayaci,
  SAYFA_BOYUTU,
  SayfaBasligi,
  Sayfalama,
  Secim,
  SuzgecCubugu,
  VeriTablosu,
  apiHatasi,
  type Sutun,
  sayfalamaGorunur,
} from '@/components/yonetim';
import type { LedgerEntry, Statement } from '@/lib/types';

/**
 * Hareket türü süzgeci — `GET /wallet/entries?type=`.
 *
 * 🔴 BU LİSTE BİR SÖZLÜK DEĞİL, BİR SEÇENEK LİSTESİDİR — ve tek kopyası budur.
 * Satır etiketleri hâlâ SUNUCUDAN gelir (`typeLabel`); bu dosya onları
 * ÜRETMEZ. Burada yazılı olan şey "hangi türler süzülebilir" bilgisidir ve
 * `<option>` metni olmadan bir seçim kutusu kurulamaz.
 *
 * Kaynak: `api/internal/transport/http/handler/wallet.go#ledgerTypeLabel`.
 * Oraya yeni bir tür eklenirse buraya da eklenmelidir; ikisi ayrışırsa
 * kullanıcı süzgeçte göremediği bir türü listede görür. Türler `ledger_type`
 * enum'undan gelir ve nadiren değişir — ölçülmüş tekrar riski, süzgeci hiç
 * vermemenin maliyetinden düşüktür.
 */
const TUR_SUZGECLERI: ReadonlyArray<{ value: string; label: string }> = [
  { value: '', label: 'Tüm hareketler' },
  { value: 'DEPOSIT', label: 'Bakiye yükleme' },
  { value: 'PURCHASE', label: 'Numara satın alma' },
  { value: 'REFUND', label: 'İade' },
  { value: 'ADJUSTMENT', label: 'Düzeltme' },
  { value: 'COMMISSION', label: 'Referans komisyonu' },
  { value: 'CHARGEBACK', label: 'Ödeme itirazı' },
];

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
  const [tur, setTur] = React.useState('');

  // İKİ AYRI DURUM: `aramaGirdisi` kutuya yazılan, `arama` sunucuya giden.
  // Tek durum olsaydı her tuşa basış bir istek olurdu.
  const [aramaGirdisi, setAramaGirdisi] = React.useState('');
  const [arama, setArama] = React.useState('');

  // 350 ms — panelin her yerinde aynı gecikme.
  React.useEffect(() => {
    const t = setTimeout(() => {
      setArama(aramaGirdisi.trim());
      setOffset(0);
    }, 350);
    return () => clearTimeout(t);
  }, [aramaGirdisi]);

  const q = useQuery({
    queryKey: ['statement', { limit: SAYFA_BOYUTU, offset, tur, arama }],
    queryFn: () => {
      // Süzgeç SUNUCUDA uygulanır: istemcide süzmek yalnız o sayfadaki 25
      // satırı görürdü ve "iadem görünmüyor" sonucunu verirdi.
      const p = new URLSearchParams({
        limit: String(SAYFA_BOYUTU),
        offset: String(offset),
      });
      if (tur) p.set('type', tur);
      if (arama) p.set('q', arama);
      return apiFetch<Statement>(`/wallet/entries?${p.toString()}`);
    },
    // Sayfa değişince liste boşalıp zıplamasın; eski veri yenisi gelene dek kalır.
    placeholderData: keepPreviousData,
  });

  const toplam = q.data?.total ?? 0;
  const etkinSuzgec = (tur ? 1 : 0) + (arama ? 1 : 0);
  // Sayfalı listede `VeriTablosu`nun "N kayıt listelendi" duyurusu YANILTICIDIR:
  // N sayfadaki satır sayısıdır, toplam değil. O durumda duyuruyu `Sayfalama`
  // yapar ve ekran okuyucu tek bir doğru cümle duyar.
  const sayfali = sayfalamaGorunur(toplam);

  return (
    <div className="flex flex-col gap-6">
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

        {/*
          SÜZGEÇ ÇUBUĞU EKSTRE KARTININ İÇİNDE — `/panel/siparisler`den farklı
          olarak burada ayrı kart AÇILMAZ: bu sayfada üstte zaten bir bakiye
          kartı var ve üçüncü bir kart, ekranın tek önemli sayısını (bakiye)
          aşağı iterdi. Süzgeç tek bir seçim kutusudur, kendi kartını hak etmez.
        */}
        <SuzgecCubugu
          className="mt-4"
          etkinSayisi={etkinSuzgec}
          onTemizle={() => {
            setTur('');
            setAramaGirdisi('');
            setArama('');
            setOffset(0);
          }}
        >
          <div className="w-full sm:w-72 sm:self-start">
            <Field
              label="Ara"
              type="search"
              value={aramaGirdisi}
              onChange={(e) => setAramaGirdisi(e.target.value)}
              placeholder="Açıklama veya referans"
              autoCapitalize="none"
              autoCorrect="off"
              autoComplete="off"
              spellCheck={false}
              hint="Yazmayı bıraktığınızda arama kendiliğinden yapılır."
            />
          </div>

          <div className="w-full sm:w-64 sm:self-start">
            <Secim
              etiket="Hareket türü"
              value={tur}
              onChange={(e) => {
                setTur(e.target.value);
                // Süzgeç değişince 3. sayfada kalmak BOŞ ekran gösterir.
                setOffset(0);
              }}
            >
              {TUR_SUZGECLERI.map((t) => (
                <option key={t.value || 'tumu'} value={t.value}>
                  {t.label}
                </option>
              ))}
            </Secim>
          </div>
        </SuzgecCubugu>

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
            etkinSuzgec > 0 ? (
              /*
                §6.3: SÜZGEÇTEN DOLAYI BOŞ ≠ GERÇEKTEN BOŞ. Hareketi olan ama
                seçtiği türden hiç kaydı olmayan kullanıcıya "Ekstre boş" demek
                yanlıştır ve yanlış eylemi önerir — yapması gereken bakiye
                yüklemek değil, süzgeci temizlemektir. O düğme `SuzgecCubugu`'nun
                içinde zaten var; burada ikincisi açılmaz (§9.2).
              */
              <Empty
                title="Bu süzgece uyan hareket yok"
                hint="Arama metnini kısaltmayı ya da hareket türünü “Tüm hareketler” yapmayı deneyin."
              />
            ) : (
              <div className="flex flex-col items-center gap-4">
                <Empty
                  title="Ekstre boş"
                  hint="Bakiye yüklediğinizde ya da numara satın aldığınızda her hareket tarihi ve tutarıyla buraya yazılır."
                />
                <Link href="/panel/bakiye-yukle">
                  <Button variant="outline">Bakiye yükle</Button>
                </Link>
              </div>
            )
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
