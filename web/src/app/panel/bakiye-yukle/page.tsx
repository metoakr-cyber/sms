'use client';

/**
 * Bakiye yükle — ödeme yöntemi seç, parayı gönder, talebi bildir, dekont yükle.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * 🔴 BU DALGADA DAVRANIŞ DEĞİŞMEDİ — SUNUM VE YAPI DEĞİŞTİ
 * ══════════════════════════════════════════════════════════════════════════
 * Bu ekran gerçek para bildirir. Doğrulama eşikleri, idempotens anahtarı,
 * dekont sınırları ve yükleme akışı BİREBİR korundu. Değişen üç şey var:
 * tekrar ortak katmana taşındı, mobil/masaüstü ikizi tek sütun tanımına indi,
 * ve hata kutuları artık `requestId`'yi 14px + tam opaklıkta gösteriyor.
 *
 * ──────────────────────────────────────────────────────────────────────────
 * 1) `toMinor` — YEREL KOPYA SİLİNDİ, `lib/para.ts` `TUTAR_ZORUNLU` KULLANILIYOR
 * ──────────────────────────────────────────────────────────────────────────
 * Depoda `toMinor`'ın beş kopyası ve ÜÇ FARKLI politikası vardı. Bu ekranın
 * kopyası ile `TUTAR_ZORUNLU` ayarı satır satır karşılaştırıldı; DAVRANIŞ
 * FARKI YOK:
 *
 *   | girdi     | eski yerel kopya                         | TUTAR_ZORUNLU |
 *   |-----------|------------------------------------------|---------------|
 *   | ""        | "Tutar giriniz."                         | AYNI          |
 *   | "   "     | "Tutar giriniz."                         | AYNI          |
 *   | "-12,50"  | "Geçerli bir tutar giriniz (en fazla 2…" | AYNI (regex)  |
 *   | "-"       | biçim hatası                             | AYNI          |
 *   | "12,5"    | 1250                                     | AYNI          |
 *   | "12,505"  | biçim hatası                             | AYNI          |
 *   | 1e20 bas. | "Tutar çok büyük."                       | AYNI          |
 *   | üst sınır | YOK (sunucu doğrular)                    | AYNI (yok)    |
 *
 * Regex de aynı (`^\d+(\.\d{1,2})?$`), mesajlar da `VARSAYILAN_MESAJLAR` ile
 * birebir aynı dizeler. ALT/ÜST SINIR yine BU EKRANDA, yöntemin kendi
 * `minAmount`/`maxAmount` alanlarıyla kontrol edilir — `toMinor`'a taşınmadı,
 * çünkü sınır yöntemden yönteme değişir (`lib/para.ts` dosya başı: ekran kendi
 * ayarını KURMAZ, hazır sabiti kullanır).
 *
 * ──────────────────────────────────────────────────────────────────────────
 * 2) SAYFA BOYUTU: 10 → `SAYFA_BOYUTU` (25). "Dekontlu liste ağır" DOĞRU DEĞİL.
 * ──────────────────────────────────────────────────────────────────────────
 * 10 sabiti gerekçesiz konmuştu (özgün commit e7f1292; sebep ne kodda ne
 * commit mesajında yazılı) ve paneldeki TEK farklı değerdi — diğer dört panel
 * ekranı 20 kullanıyor. Hipotez ölçüldü ve ÇÜRÜDÜ:
 *
 *   · `DepositResponse` (dto/deposit.go:88) 12 SKALER alan taşır; en büyüğü
 *     serbest metin `rejectionReason`. Dekont GÖRÜNTÜSÜ listede YOKTUR —
 *     `hasReceipt` yalnız bir `bool`'dur.
 *   · Dekontun kendisi ayrı bir uçtan, yalnız istendiğinde çekilir
 *     (`GET /wallet/deposits/{id}/receipt`, ikili yanıt).
 *   Yani satır, ekstre satırından ağır değildir; 10'da tutmanın bir bedeli
 *   vardı (aynı listeyi görmek için üç kat istek) ve bir karşılığı yoktu.
 *
 * Sunucu sınırı: `pagination(c, 20, 100)` (handler/deposit.go:109) — 25 kabul
 * edilir. §6.3: "Tek bileşen, tek `limit`."
 *
 * ──────────────────────────────────────────────────────────────────────────
 * 3) DEKONT AKIŞI — DOKUNULMADI
 * ──────────────────────────────────────────────────────────────────────────
 * `ReceiptUploader` iki yerde kullanılır ve ikisinde de aynıdır: talep yeni
 * oluştuğunda satır içi, geçmiş bir talep için modal içinde. 5 MB sınırı,
 * kabul edilen üç MIME türü, 60 sn zaman aşımı ve `retry: false` aynen
 * korundu — bir dekont ikinci kez yüklenirse sunucuda ikinci kez diske yazılır.
 */

import * as React from 'react';
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { apiFetch } from '@/lib/api';
import { formatDateTime, formatMoney } from '@/lib/format';
import { TUTAR_ZORUNLU, toMinor } from '@/lib/para';
import { useSession } from '@/hooks/useSession';
import { Alert, Button, Card, Empty, Field, Skeleton, cx } from '@/components/ui';
import { Modal } from '@/components/modal';
import {
  CokSatir,
  DurumRozeti,
  HataDurumu,
  KayitSayaci,
  SAYFA_BOYUTU,
  SayfaBasligi,
  Sayfalama,
  VeriTablosu,
  apiHatasi,
  type Sutun,
} from '@/components/yonetim';
import type { Deposit, PublicDepositMethod } from '@/lib/types';

const depositsKey = ['wallet', 'deposits'] as const;

/** Dekont sınırları — sunucudaki `storage.MaxReceiptBytes` ve sihirli bayt kontrolü ile aynı. */
const MAX_RECEIPT_BYTES = 5 * 1024 * 1024;
const RECEIPT_TYPES = ['image/jpeg', 'image/png', 'application/pdf'];

/**
 * Yöntem yapılandırma alanlarının Türkçe adları.
 *
 * Anahtarlar sunucudaki zorunlu alan listesiyle aynıdır (handler/admin.go):
 * BANK_TRANSFER → iban, hesapAdi · CRYPTO → cuzdanAdresi, ag.
 */
const CONFIG_LABELS: Record<string, string> = {
  iban: 'IBAN',
  hesapAdi: 'Hesap adı',
  cuzdanAdresi: 'Cüzdan adresi',
  ag: 'Ağ',
  banka: 'Banka',
  aciklama: 'Açıklama',
};

/** Elle yazılması hataya açık olan alanlar — kopyala düğmesi ZORUNLU. */
const COPYABLE = new Set(['iban', 'cuzdanAdresi']);

/**
 * Dekont sütununun TEK metni. Hem `<th>`/`<dt>` etiketi hem de mobil kartın
 * eylemsiz satırındaki etiket buradan okunur (bkz. sütun tanımı).
 */
const DEKONT_BASLIGI = 'Dekont';

/** Yapılandırma alanları bu sırayla gösterilir; listede olmayanlar sona eklenir. */
const CONFIG_ORDER = ['banka', 'hesapAdi', 'iban', 'ag', 'cuzdanAdresi', 'aciklama'];

/* ═══════════════════════ Kopyala düğmesi ═══════════════════════ */

/**
 * `navigator.clipboard` GÜVENLİ OLMAYAN BAĞLAMDA (http://) ve bazı iOS
 * sürümlerinde YOKTUR. Yedek olarak eski `execCommand('copy')` yolu denenir;
 * o da olmazsa kullanıcıya "elle seçin" denir — sessizce hiçbir şey yapmayan
 * bir düğme en kötüsüdür, hele kopyalanan şey bir IBAN ise: elle yazılan bir
 * IBAN yanlış hesaba para göndermek demektir.
 *
 * (Ortak katmanda `CopyButton` HENÜZ YOK — `tasarim-sistemi.md §5.3`'te P1-12
 * olarak sırada duruyor. Üç ayrı uygulaması olan bu bileşeni bu dalgada
 * ortaklaştırmak, kapsamı bu ekranın dışına taşırdı.)
 */
function KopyalaDugmesi({ deger, etiket }: { deger: string; etiket: string }) {
  const [durum, setDurum] = React.useState<'bos' | 'ok' | 'hata'>('bos');

  async function kopyala() {
    let ok = false;
    try {
      if (navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(deger);
        ok = true;
      } else {
        ok = eskiKopyala(deger);
      }
    } catch {
      ok = eskiKopyala(deger);
    }
    setDurum(ok ? 'ok' : 'hata');
    window.setTimeout(() => setDurum('bos'), 2500);
  }

  return (
    <button
      type="button"
      onClick={kopyala}
      aria-label={etiket}
      className={cx(
        'grid size-11 shrink-0 place-items-center rounded-xl border',
        /*
          Basma geri bildirimi §4.2'nin birebir reçetesi: `scale(0.97)`,
          `--sure-basma` (160ms), `--ease-out`. Özgün kod `transition-colors
          active:scale-95` yazıyordu; `transform` geçiş listesinde OLMADIĞI
          için ölçek hem basışta hem bırakışta ANLIK SIÇRIYORDU ve 0.95
          Emil'in 0.95–0.98 bandının kenarındaydı. `transition: all` YOK:
          özellikler tek tek sayılır.
        */
        '[transition-property:color,border-color,transform]',
        '[transition-duration:var(--sure-basma)]',
        '[transition-timing-function:var(--ease-out)]',
        'active:scale-[0.97]',
        durum === 'ok'
          ? 'border-[var(--color-ok)] text-[var(--color-ok)]'
          : durum === 'hata'
            ? 'border-[var(--color-bad)] text-[var(--color-bad)]'
            : 'border-[var(--border)] text-muted',
      )}
    >
      {durum === 'ok' ? (
        <svg viewBox="0 0 24 24" className="size-5" fill="none" stroke="currentColor"
             strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
          <path d="m5 13 4 4L19 7" />
        </svg>
      ) : (
        <svg viewBox="0 0 24 24" className="size-5" fill="none" stroke="currentColor"
             strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
          <rect x="9" y="9" width="12" height="12" rx="2.4" />
          <path d="M5 15V5a2 2 0 0 1 2-2h10" />
        </svg>
      )}
      <span className="sr-only" aria-live="polite">
        {durum === 'ok' ? 'Kopyalandı' : durum === 'hata' ? 'Kopyalanamadı, elle seçin' : ''}
      </span>
    </button>
  );
}

function eskiKopyala(deger: string): boolean {
  try {
    const ta = document.createElement('textarea');
    ta.value = deger;
    // Ekran dışına almak yerine görünmez yapmak iOS'ta seçimi bozar;
    // sabit konumlandırıp opaklığı sıfırlarız.
    ta.style.cssText = 'position:fixed;top:0;left:0;opacity:0;pointer-events:none';
    ta.setAttribute('readonly', '');
    document.body.appendChild(ta);
    ta.select();
    ta.setSelectionRange(0, deger.length);
    const ok = document.execCommand('copy');
    document.body.removeChild(ta);
    return ok;
  } catch {
    return false;
  }
}

/* ═══════════════════════ Dekont yükleme ═══════════════════════ */

function DekontYukleyici({ talepId, onBitti }: { talepId: string; onBitti: () => void }) {
  const [dosya, setDosya] = React.useState<File | null>(null);
  const [yerelHata, setYerelHata] = React.useState('');

  const yukle = useMutation({
    mutationFn: (f: File) => {
      const fd = new FormData();
      // Alan adı sunucudaki `receiptField` sabitiyle AYNI olmalı.
      fd.append('dekont', f);
      return apiFetch<Deposit>(`/wallet/deposits/${encodeURIComponent(talepId)}/receipt`, {
        method: 'POST',
        body: fd,
        // Varsayılan 15 sn, mobil ağda 5 MB'lık bir dekont için yetmez.
        timeoutMs: 60_000,
      });
    },
    // Yükleme tekrarlanmaz: aynı dosya ikinci kez diske yazılır.
    retry: false,
    onSuccess: onBitti,
  });

  function sec(e: React.ChangeEvent<HTMLInputElement>) {
    yukle.reset();
    const f = e.target.files?.[0] ?? null;
    setYerelHata('');
    if (!f) { setDosya(null); return; }
    // Sunucu dosyayı sihirli baytlarına göre ayrıca doğrular; buradaki kontrol
    // yalnız kullanıcıyı 5 MB'lık boşa yüklemeden kurtarmak içindir.
    if (f.size > MAX_RECEIPT_BYTES) {
      setDosya(null);
      setYerelHata('Dekont dosyası en fazla 5 MB olabilir.');
      return;
    }
    if (f.type && !RECEIPT_TYPES.includes(f.type)) {
      setDosya(null);
      setYerelHata('Yalnız JPEG, PNG veya PDF dosyası yükleyebilirsiniz.');
      return;
    }
    setDosya(f);
  }

  const hata = apiHatasi(yukle.error);

  return (
    <div className="flex flex-col gap-4">
      {/*
        Dosya alanı `Field` ailesine GİRMEZ: `<input type="file">` değer
        taşımaz, `placeholder`/`hint` düzeni farklıdır ve `file:` sözde
        elemanı gerektirir. Ortak katmana altıncı bir varyant eklemek yerine
        burada kalır — tek çağrı yeri budur.
      */}
      <label className="flex flex-col gap-2">
        <span className="text-sm font-medium">Dekont dosyası</span>
        <input
          // Modal içinde açıldığında ilk odak buraya gelsin (modal.tsx).
          data-autofocus
          type="file"
          accept="image/jpeg,image/png,application/pdf"
          onChange={sec}
          // px-4/py-3: yarım boşluk basamağı yok (§2.3). `text-base` = 16px:
          // altına inilirse iOS sayfayı yakınlaştırır.
          className="raised min-h-12 w-full rounded-xl border px-4 py-3 text-base
                     outline-none file:mr-3 file:min-h-9 file:rounded-lg file:border-0
                     file:bg-brand-500 file:px-3 file:text-sm file:text-white
                     focus:border-brand-400"
        />
        {/* `text-sm`: kabul edilen tür ve boyut, kullanıcının okumak ZORUNDA
            olduğu kısıttır — ikincil dipnot değil (§3.2). */}
        <span className="text-sm text-muted">JPEG, PNG veya PDF · en fazla 5 MB.</span>
      </label>

      {/* Yerel doğrulama hatası YENİ BELİREN bir hatadır: `duyur` varsayılan
          `true` kalır ve ekran okuyucu kesintiyle duyurur (§7.4). */}
      {yerelHata && <Alert>{yerelHata}</Alert>}
      {hata && <HataDurumu hata={hata} />}

      <Button
        fullWidth
        disabled={!dosya}
        loading={yukle.isPending}
        onClick={() => dosya && yukle.mutate(dosya)}
      >
        Dekontu yükle
      </Button>
    </div>
  );
}

/* ═══════════════════════════════ Ekran ═══════════════════════════════ */

/**
 * yeniAnahtar idempotens anahtarı üretir.
 *
 * `crypto.randomUUID` eski Safari'de ve güvenli olmayan bağlamda YOKTUR;
 * o durumda getRandomValues'a, o da yoksa zaman+rastgele birleşimine düşeriz.
 * Anahtarın gizli olması gerekmez — yalnız aynı kullanıcı içinde tekrarlamaması
 * yeter (sunucudaki tekil indeks kullanıcı kapsamlıdır).
 */
function yeniAnahtar(): string {
  const c = globalThis.crypto;
  if (c?.randomUUID) return c.randomUUID();
  if (c?.getRandomValues) {
    const b = new Uint8Array(16);
    c.getRandomValues(b);
    return Array.from(b, (x) => x.toString(16).padStart(2, '0')).join('');
  }
  return `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 12)}`;
}

interface TalepListesi {
  items: Deposit[];
  total: number;
  limit: number;
  offset: number;
}

export default function BakiyeYukleSayfasi() {
  const qc = useQueryClient();
  const { user } = useSession();

  const yontemler = useQuery({
    queryKey: ['wallet', 'deposit-methods'],
    queryFn: () => apiFetch<{ items: PublicDepositMethod[] }>('/wallet/deposit-methods'),
  });

  const [offset, setOffset] = React.useState(0);
  const liste = useQuery({
    queryKey: [...depositsKey, { limit: SAYFA_BOYUTU, offset }],
    queryFn: () =>
      apiFetch<TalepListesi>(`/wallet/deposits?limit=${SAYFA_BOYUTU}&offset=${offset}`),
    placeholderData: keepPreviousData,
  });

  const [yontemId, setYontemId] = React.useState('');
  const [tutar, setTutar] = React.useState('');
  const [referans, setReferans] = React.useState('');
  const [not, setNot] = React.useState('');
  const [hatalar, setHatalar] = React.useState<Record<string, string>>({});
  const [olusan, setOlusan] = React.useState<Deposit | null>(null);
  const [dekontIcin, setDekontIcin] = React.useState<Deposit | null>(null);

  const yontemListesi = yontemler.data?.items ?? [];
  const yontem = yontemListesi.find((m) => m.id === yontemId) ?? null;

  /*
   * İDEMPOTENS ANAHTARI: form başına BİR kez üretilir ve talep GERÇEKTEN
   * oluşana kadar korunur.
   *
   * 🔴 NEDEN GEREKLİ: kullanıcı havaleyi yapmış, formu göndermiş, mobil ağda
   * yanıt kaybolmuş olabilir (tünel, zaman aşımı, uygulamayı arka plana alma).
   * Sunucu talebi YAZDI ama kullanıcı hata gördü ve tekrar basıyor. Anahtar
   * olmadan aynı havale için iki bekleyen talep oluşur; yönetici ikisini de
   * onaylarsa kullanıcıya iki kat yazılır.
   *
   * Sunucu bu anahtarı `Idempotency-Key` başlığından okur ve tekrarda MEVCUT
   * talebi geri döndürür (api/internal/service/deposit/service.go).
   */
  const [idemAnahtari, setIdemAnahtari] = React.useState(() => yeniAnahtar());

  const olustur = useMutation({
    mutationFn: (v: { methodId: string; amountMinor: number; reference: string; note: string }) =>
      apiFetch<Deposit>('/wallet/deposits', {
        method: 'POST',
        body: v,
        idempotencyKey: idemAnahtari,
      }),
    // Talep oluşturmak sunucuda idempotenttir ama OTOMATİK TEKRAR yine de
    // yapılmaz: kullanıcı ne olduğunu görmeli ve kararı o vermeli.
    retry: false,
    onSuccess: (dep) => {
      // Talep oluştu — sıradaki için YENİ anahtar.
      setIdemAnahtari(yeniAnahtar());
      qc.invalidateQueries({ queryKey: depositsKey });
      setOlusan(dep);
      setTutar(''); setReferans(''); setNot('');
      setHatalar({});
    },
    onError: () => {
      // 🔴 HATA YOLUNDA DA LİSTE TAZELENİR. İstek sunucuya ULAŞMIŞ ama yanıt
      // kaybolmuş olabilir: talep oluştu, kullanıcı hata gördü. Listeyi
      // tazelemezsek kullanıcı oluşmuş talebi göremez ve tekrar dener.
      // (Anahtar aynı kaldığı için sunucu ikinciyi yutar, ama kullanıcının
      //  ekranında ne olduğunu GÖRMESİ ayrı bir mesele.)
      qc.invalidateQueries({ queryKey: depositsKey });
    },
  });

  function gonder(e: React.FormEvent) {
    e.preventDefault();
    olustur.reset();
    const errs: Record<string, string> = {};

    if (!yontem) errs.methodId = 'Bir ödeme yöntemi seçiniz.';

    // 🔴 `TUTAR_ZORUNLU`: boş = HATA, negatif = RET. Ekran kendi ayarını
    // kurmaz (`lib/para.ts`), hazır sabiti kullanır.
    const cozulen = toMinor(tutar, TUTAR_ZORUNLU);
    if ('error' in cozulen) {
      errs.amountMinor = cozulen.error;
    } else if (yontem) {
      if (cozulen.minor < yontem.minAmount.minor) {
        errs.amountMinor = `En az ${formatMoney(yontem.minAmount)} yükleyebilirsiniz.`;
      } else if (yontem.maxAmount.minor > 0 && cozulen.minor > yontem.maxAmount.minor) {
        // 🔴 maxAmount.minor === 0 "üst sınır yok" demektir, 0,00 ₺ tavan DEĞİL.
        errs.amountMinor = `En fazla ${formatMoney(yontem.maxAmount)} yükleyebilirsiniz.`;
      }
    }

    const ref = referans.trim();
    if (ref.length < 4) {
      errs.reference = 'Bu alan zorunludur (en az 4 karakter).';
    } else if (ref.length > 120) {
      errs.reference = 'En fazla 120 karakter olabilir.';
    }
    if (not.trim().length > 300) errs.note = 'Not en fazla 300 karakter olabilir.';

    setHatalar(errs);
    if (Object.keys(errs).length || !yontem) return;

    olustur.mutate({
      methodId: yontem.id,
      amountMinor: (cozulen as { minor: number }).minor,
      reference: ref,
      note: not.trim(),
    });
  }

  const tutarOnizleme = React.useMemo(() => {
    const p = toMinor(tutar, TUTAR_ZORUNLU);
    if ('error' in p) return null;
    return formatMoney({ minor: p.minor, currency: 'TRY', formatted: '' });
  }, [tutar]);

  const olusturHatasi = apiHatasi(olustur.error);
  const yontemHatasi = apiHatasi(yontemler.error);

  const toplam = liste.data?.total ?? 0;
  const sayfali = toplam > SAYFA_BOYUTU;

  const configAnahtarlari = yontem
    ? [...CONFIG_ORDER.filter((k) => yontem.config[k]),
       ...Object.keys(yontem.config).filter((k) => !CONFIG_ORDER.includes(k) && yontem.config[k])]
    : [];

  /*
    ═══════════════════════ Talep listesi sütunları ═══════════════════════
    Sütun sırası TABLONUN sırasıdır; mobil kartın anatomisi `mobilRol` ile
    ayrıca verilir, yani kart için ikinci bir kod yolu YOKTUR.

    Kart yerleşimi (eski elle yazılmış kartla aynı):
      üst sol  → Yöntem (`baslik`)     üst sağ → Tutar (`rozet`)
      gövde    → Tarih · Durum         alt     → Dekont (`eylem`)

    🔴 DURUM `mobilRol: 'rozet'` DEĞİL, gövde satırı. Sebep ölçülebilir: red
    nedeni serbest metindir ve rozet yuvası `shrink-0`'dır — uzun bir gerekçe
    orada 320 px'te taşar. Gerekçeyi ayrı bir sütuna almak da ÇÖZÜM DEĞİLDİ:
    altı sütun 768 px'te sığmaz, `oncelik: 3` ile gizlemek ise 768–1023 px
    bandında red nedenini GÖRÜNMEZ kılardı. Bir para ekranında "paran neden
    yatmadı" hiçbir genişlikte gizlenemez.

    `useMemo` YOK: dizi `setDekontIcin` ve iki durum alanına bağlı; her
    render'da yeniden kurulması `VeriTablosu` için serbest, memolamak ise
    bağımlılık listesini elle güncel tutmayı gerektirirdi.
  */
  const sutunlar: ReadonlyArray<Sutun<Deposit>> = [
    {
      anahtar: 'tarih',
      baslik: 'Tarih',
      sayisal: true,
      hucre: (d) => <span className="text-muted">{formatDateTime(d.createdAt)}</span>,
    },
    {
      anahtar: 'yontem',
      baslik: 'Yöntem',
      mobilRol: 'baslik',
      hucre: (d) => <span className="break-anywhere font-medium">{d.method}</span>,
    },
    {
      anahtar: 'tutar',
      // "Bildirilen tutar": bu, kullanıcının BİLDİRDİĞİ tutardır; bakiyeye
      // yazılan değil. Yönetim tarafındaki `talepler` ekranı da aynı sözcüğü
      // kullanır — iki yüzey, tek sözlük.
      baslik: 'Bildirilen tutar',
      hizala: 'sag',
      sayisal: true,
      mobilRol: 'rozet',
      hucre: (d) => (
        <>
          <span className="font-semibold">{formatMoney(d.amount)}</span>
          {/*
            "Yansıyan" YALNIZ FARKLIYSA çıkar. Eski mobil kart onu HER
            COMPLETED satırda gösteriyordu (masaüstü tablo ise yalnız farklıysa)
            — aynı sayıyı iki kez yazmak "bir fark var" izlenimi verir ve bir
            para ekranında bu yanlış alarmdır. Masaüstünün davranışı doğrudur;
            tek tanım o oldu.
          */}
          {d.status === 'COMPLETED' && d.credited.minor !== d.amount.minor && (
            <span className="block text-sm font-normal text-[var(--color-ok)]">
              yansıyan: {formatMoney(d.credited)}
            </span>
          )}
        </>
      ),
    },
    {
      anahtar: 'durum',
      baslik: 'Durum',
      // Ton TEK haritadan (`durumTonu`) gelir; etiket sunucunun `statusLabel`'ı.
      // Eski yerel `statusTone` haritası ile birebir aynı sonucu verir:
      // PENDING→warn · COMPLETED→ok · REJECTED→bad · REFUNDED→neutral.
      hucre: (d) => (
        <>
          <DurumRozeti durum={d.status} etiket={d.statusLabel} />
          {d.rejectionReason && (
            /*
              🔴 `md:max-w-[34ch]` BİR SÜS DEĞİL, TABLO GENİŞLİK PAZARLIĞIDIR.
              Ölçüldü: sınırsız bırakıldığında bu serbest metin hücrenin
              "tercih edilen genişliğini" şişiriyor, tarayıcı Durum sütununa
              fazladan yer veriyor ve komşu Yöntem sütunu 55 px'e düşüp
              "Banka / Havalesi / EFT" diye ÜÇ satıra kırılıyordu (ekran
              görüntüsüyle görüldü). Sınır konunca metin aynı yerde, aynı
              bütünlükte kalıyor; yalnız sütun pazarlığından çekiliyor.
              Sınır `md:` ile EKLENİR, mobilde geri alınmaz (CLAUDE.md #17):
              tablo zaten `md:`'de başlar, kartta pazarlık diye bir şey yoktur.

              `text-left`: `VeriTablosu`'nun kart `<dd>`'si `text-right`tır ve
              bu doğru bir varsayılandır (tek satırlık değerler sağa hizalanır),
              ama BEŞ SATIRLIK bir paragraf sağa hizalı okunmaz — sol kenar
              tırtıklı olur. Tabloda sütun zaten sola hizalı; sınıf orada etkisiz.
            */
            <span
              className="mt-2 block break-anywhere text-left text-sm
                         text-[var(--color-bad)] md:max-w-[34ch]"
            >
              Red nedeni: {d.rejectionReason}
            </span>
          )}
        </>
      ),
    },
    {
      anahtar: 'dekont',
      baslik: DEKONT_BASLIGI,
      hizala: 'sag',
      mobilRol: 'eylem',
      /*
        METİN `sunum`DAN TÜRETİLMEZ (§6): her iki sunumda da aynı sözcükler
        geçer — "Dekont yükle", "Yüklendi", "—". Değişen tek şey YERLEŞİMDİR:
        kartta tam genişlik düğme, tabloda dar düğme.

        🔴 EYLEMSİZ SATIRDA ETİKET ELLE VERİLİR. Kartın `eylem` yuvasının
        `<dt>`si yoktur; tabloda o işi `<th>` yapar. Etiketsiz bırakıldığında
        kartın en altında ÖKSÜZ BİR "—" kalıyordu (390 px ekran görüntüsüyle
        görüldü): kullanıcı neyin tire olduğunu bilmiyor. Etiket `baslik` ile
        AYNI SABİTTEN okunur (`DEKONT_BASLIGI`) — iki ayrı dize yazılmadığı
        sürece ayrışma doğamaz, ki bu bileşenin var olma sebebi tam olarak budur.
      */
      hucre: (d, sunum) => {
        if (!d.hasReceipt && d.status === 'PENDING') {
          return (
            <Button
              variant="outline"
              size="sm"
              fullWidth={sunum === 'kart'}
              onClick={() => setDekontIcin(d)}
            >
              Dekont yükle
            </Button>
          );
        }
        const metin = d.hasReceipt ? 'Yüklendi' : '—';
        if (sunum === 'tablo') return <span className="text-muted">{metin}</span>;
        // Kart: üstündeki `<dl>` satırlarıyla aynı ritim (etiket sol, değer sağ).
        return (
          <p className="flex items-start justify-between gap-4 text-sm">
            <span className="text-muted">{DEKONT_BASLIGI}</span>
            <span className="font-medium">{metin}</span>
          </p>
        );
      },
    },
  ];

  return (
    <div className="mx-auto flex max-w-5xl flex-col gap-6">
      {/*
        ═══════════════════ İKİ ZON, TEK SOL KENAR ═══════════════════
        Sayfa `max-w-5xl` (1024px) — `/panel/cuzdan`, `/panel/siparisler`,
        `/panel/numara-al` ve `/yonetim/talepler` ile AYNI. Eskiden `max-w-3xl`
        idi ve paneldeki EN DAR ekrandı; §10 kontrol listesi içerik genişliğinin
        ekrandan ekrana zıplamasını bir kusur sayıyor.

        Ama sayfanın tamamı 1024'e açılmaz: giriş akışı (okuma + form + IBAN)
        `max-w-3xl` bir sütunda kalır, YALNIZ tablo tam genişliği kullanır.
        Gerekçe ölçülmüş, ikisi de ekran görüntüsüyle görüldü:
          · 704 px'lik tabloda "Yöntem" sütunu 55 px'e düşüyor ve
            "Banka / Havalesi / EFT" ÜÇ satıra kırılıyordu.
          · 992 px'lik bir formda IBAN satırının kopyala düğmesi değerden
            ~600 px uzağa gidiyor — kopyalanacak şeyle düğme arasındaki bağ
            kopuyor, ki bu bir IBAN ekranında en istenmeyen şeydir.
        İki zon AYNI SOL KENARI paylaşır; tablo yalnız sağa doğru uzar.
      */}
      <div className="flex max-w-3xl flex-col gap-6">
      <SayfaBasligi baslik="Bakiye yükle">
        {/* Başlığın eylem yuvası: mevcut bakiye bir EYLEM değil ama başlıkla
            aynı satırda durması, "ne kadarım var / ne kadar yükleyeyim"
            sorusunu tek bakışta cevaplar. `tabular-nums` (§3.5). */}
        <p className="text-sm text-muted">
          Mevcut bakiyeniz:{' '}
          <strong className="tabular-nums text-[var(--text)]">{formatMoney(user?.balance)}</strong>
        </p>
      </SayfaBasligi>

      {/*
        `duyur={false}`: bu kutu sayfa açılışında KOŞULSUZ çizilir, yani "yeni
        beliren" bir şey değildir. `role="alert"` verilirse ekran okuyucu daha
        kullanıcı hiçbir şey yapmadan sözünü keser (§7.4 — ölçülmüş hata).
      */}
      <Alert tone="info" duyur={false}>
        Önce parayı aşağıdaki hesaba gönderin, sonra bu formu doldurun. Talebiniz
        kontrol edildikten sonra bakiyeniz tanımlanır; onay anında değil, kontrol
        sonrasında yansır.
      </Alert>

      {/* ═══════════ Yöntem seçimi ═══════════ */}
      <Card>
        <h2 className="text-lg font-semibold">Ödeme yöntemi</h2>

        {yontemler.isLoading ? (
          /* Yükleme İSKELETLE gösterilir, spinner ile değil (§5.1). Yükseklik
             gerçek yöntem satırıyla (p-4 + iki satır ≈ 80px) eşleşir ki veri
             geldiğinde düzen ZIPLAMASIN. */
          <div className="mt-6 flex flex-col gap-3">
            {[0, 1].map((i) => <Skeleton key={i} className="h-20" />)}
          </div>
        ) : yontemHatasi ? (
          <HataDurumu hata={yontemHatasi} className="mt-6" />
        ) : !yontemListesi.length ? (
          <Empty
            title="Şu an aktif ödeme yöntemi yok"
            hint="Yükleme geçici olarak kapalı. Kısa süre sonra tekrar deneyin."
          />
        ) : (
          <ul className="mt-6 flex flex-col gap-3">
            {yontemListesi.map((m) => (
              <li key={m.id}>
                <label
                  className={cx(
                    // p-4: ferah yoğunluk. Dokunma hedefi zaten iki satırlık
                    // gövdeyle 44 px'in çok üstünde.
                    'flex items-start gap-3 rounded-xl border p-4',
                    // Yalnız RENK geçer (§4.3): konum/ölçek hareketi yok.
                    '[transition-property:border-color,background-color]',
                    '[transition-duration:var(--sure-hizli)]',
                    m.id === yontemId
                      ? 'border-brand-500 bg-brand-500/5'
                      : 'border-[var(--border)]',
                  )}
                >
                  <input
                    type="radio"
                    name="odeme-yontemi"
                    value={m.id}
                    checked={m.id === yontemId}
                    onChange={() => { setYontemId(m.id); setHatalar({}); }}
                    className="mt-1 size-5 shrink-0 accent-[var(--color-brand-500)]"
                  />
                  <span className="min-w-0">
                    <span className="block text-sm font-medium">{m.name}</span>
                    {/* `text-sm`: alt/üst sınır bir KISITTIR, dipnot değil (§3.2). */}
                    <span className="mt-1 block text-sm text-muted">
                      En az {formatMoney(m.minAmount)}
                      {m.maxAmount.minor > 0 ? ` · En fazla ${formatMoney(m.maxAmount)}` : ''}
                      {m.kind === 'CRYPTO' ? ' · Kripto' : ' · Banka havalesi/EFT'}
                    </span>
                  </span>
                </label>
              </li>
            ))}
          </ul>
        )}

        {hatalar.methodId && (
          <p role="alert" className="mt-3 text-sm text-[var(--color-bad)]">{hatalar.methodId}</p>
        )}
      </Card>

      {/* ═══════════ Hesap bilgileri ═══════════ */}
      {yontem && (
        <Card>
          <h2 className="text-lg font-semibold">Nereye göndereceksiniz</h2>

          {yontem.instructions && (
            <p className="mt-3 max-w-[70ch] whitespace-pre-line text-sm leading-relaxed text-muted">
              {yontem.instructions}
            </p>
          )}

          <dl className="mt-6 flex flex-col gap-3">
            {configAnahtarlari.map((k) => {
              const deger = yontem.config[k] ?? '';
              return (
                <div key={k} className="flex items-center justify-between gap-3
                                        rounded-xl border border-[var(--border)] p-4">
                  <div className="min-w-0">
                    {/*
                      `uppercase` KALDIRILDI. CSS büyük harf dönüşümü Türkçede
                      `i → I` üretir (`İ` değil): "Cüzdan adresi" → "CÜZDAN
                      ADRESI". Bir IBAN ekranında yanlış yazılmış bir etiket
                      güveni doğrudan zedeler. Ayrım ağırlık ve renkle yapılır.
                      `text-sm`, `text-xs` değil (§3.2).
                    */}
                    <dt className="text-sm font-medium text-muted">
                      {CONFIG_LABELS[k] ?? k}
                    </dt>
                    {/*
                      `select-text` ZORUNLU: mobilde uzun basıp kopyalamak en
                      yaygın yol. Monospace burada MEŞRUDUR (§9.2) — "teknik
                      dursun" diye değil, IBAN karakter karakter okunabilsin diye.
                    */}
                    <dd className="mt-1 select-text break-anywhere font-mono text-sm">
                      {deger}
                    </dd>
                  </div>
                  {COPYABLE.has(k) && (
                    <KopyalaDugmesi
                      deger={deger}
                      etiket={`${CONFIG_LABELS[k] ?? k} kopyala`}
                    />
                  )}
                </div>
              );
            })}
          </dl>

          {yontem.kind === 'CRYPTO' && (
            /* Yöntem seçilince beliren ama STATİK bir bilgi kutusu: kullanıcının
               eylemine verilen bir yanıt değil, yöntemin kalıcı kuralı. */
            <Alert tone="warn" duyur={false} className="mt-6">
              Gönderimi yalnız <strong>{yontem.config.ag || 'belirtilen'}</strong> ağı
              üzerinden yapın. Başka bir ağdan gönderilen tutar geri getirilemez.
            </Alert>
          )}
        </Card>
      )}

      {/* ═══════════ Talep formu ═══════════ */}
      {yontem && (
        <Card>
          <h2 className="text-lg font-semibold">Yükleme talebi</h2>

          <form onSubmit={gonder} className="mt-6 flex flex-col gap-4" noValidate>
            <Field
              label="Tutar (TL)"
              value={tutar}
              onChange={(e) => setTutar(e.target.value)}
              inputMode="decimal" autoComplete="off"
              placeholder="Örn. 250,00"
              error={hatalar.amountMinor}
              hint={tutarOnizleme
                ? `Bildirilen tutar: ${tutarOnizleme}`
                : `En az ${formatMoney(yontem.minAmount)}${
                    yontem.maxAmount.minor > 0
                      ? `, en fazla ${formatMoney(yontem.maxAmount)}`
                      : ''}`}
            />

            <Field
              label={yontem.referenceLabel}
              value={referans}
              onChange={(e) => setReferans(e.target.value)}
              autoCapitalize="none" autoCorrect="off" spellCheck={false}
              maxLength={120}
              error={hatalar.reference}
              hint={yontem.kind === 'CRYPTO'
                ? 'Gönderdiğiniz işlemin zincirdeki hash değeri.'
                : 'Havale ekranında yazdığınız açıklama ya da dekont numarası.'}
            />

            {/*
              `CokSatir`: elle kurulan `<label>` + `<textarea>` + `aria-*`
              zinciri ortak katmandan gelir. Kazanç yalnız satır değil —
              `caret-color` (koyu temada görünmez imleç), `thin-scroll` ve
              14px hata metni artık bu ekranda da var.
            */}
            <CokSatir
              etiket="Not (isteğe bağlı)"
              value={not}
              onChange={(e) => setNot(e.target.value)}
              rows={2}
              maxLength={300}
              placeholder="Eklemek istediğiniz bir şey varsa yazın."
              hata={hatalar.note}
            />

            {yontem.receiptRequired && (
              <Alert tone="info" duyur={false}>
                Bu yöntemde <strong>dekont</strong> bekleniyor. Talebi oluşturduktan
                sonra dekontu yükleyebilirsiniz.
              </Alert>
            )}

            <Button type="submit" fullWidth loading={olustur.isPending}>
              Talebi oluştur
            </Button>
          </form>

          {olusturHatasi && <HataDurumu hata={olusturHatasi} className="mt-6" />}
        </Card>
      )}

      {/* ═══════════ Oluşturulan talep + dekont ═══════════ */}
      {olusan && (
        <Card>
          <div className="flex flex-wrap items-center justify-between gap-4">
            <h2 className="text-lg font-semibold">Talebiniz alındı</h2>
            <DurumRozeti durum={olusan.status} etiket={olusan.statusLabel} />
          </div>
          <p className="mt-3 max-w-[70ch] text-sm text-muted">
            <strong className="tabular-nums text-[var(--text)]">{formatMoney(olusan.amount)}</strong>
            {' '}tutarındaki talebiniz {olusan.method} yöntemiyle kaydedildi. Kontrol
            edildikten sonra bakiyenize yansıyacak.
          </p>

          {!olusan.hasReceipt ? (
            <div className="mt-6">
              <DekontYukleyici
                talepId={olusan.id}
                onBitti={() => {
                  qc.invalidateQueries({ queryKey: depositsKey });
                  setOlusan({ ...olusan, hasReceipt: true });
                }}
              />
            </div>
          ) : (
            /* Bu kutu bir EYLEMİN sonucudur — `duyur` açık kalır (§7.4). */
            <Alert tone="ok" className="mt-6">Dekont yüklendi.</Alert>
          )}
        </Card>
      )}
      </div>
      {/* ── giriş sütunu biter; buradan sonrası tam genişlik ── */}

      {/* ═══════════ Geçmiş talepler ═══════════ */}
      <Card>
        <div className="flex flex-wrap items-center justify-between gap-4">
          <h2 className="text-lg font-semibold">Yükleme talepleriniz</h2>
          <KayitSayaci toplam={toplam} />
        </div>

        <VeriTablosu
          className="mt-6"
          baslik="Yükleme talepleriniz"
          sutunlar={sutunlar}
          satirlar={liste.data?.items}
          satirAnahtari={(d) => d.id}
          yukleniyor={liste.isLoading}
          hata={apiHatasi(liste.error)}
          duyuru={!sayfali}
          bos={
            /* Süzgeç YOK, yani tek bir boş hâl var ve öğretmesi gereken şey
               akışın kendisi: önce para gönderilir, sonra talep bildirilir
               (§6.3). Eylem sunulmaz — form zaten bu sayfanın üstünde duruyor. */
            <Empty
              title="Henüz talep yok"
              hint="Yukarıdaki formla ilk yükleme talebinizi oluşturduğunuzda, durumu ve dekontu bu listede takip edersiniz."
            />
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

      {/* ═══════════ Dekont yükleme (geçmiş talep) ═══════════ */}
      <Modal open={!!dekontIcin} onClose={() => setDekontIcin(null)} title="Dekont yükle">
        {dekontIcin && (
          <div className="flex flex-col gap-4">
            <p className="text-sm text-muted">
              <strong className="tabular-nums text-[var(--text)]">
                {formatMoney(dekontIcin.amount)}
              </strong>{' '}
              tutarındaki {formatDateTime(dekontIcin.createdAt)} tarihli talep için.
            </p>
            <DekontYukleyici
              talepId={dekontIcin.id}
              onBitti={() => {
                qc.invalidateQueries({ queryKey: depositsKey });
                setDekontIcin(null);
              }}
            />
            <Button variant="outline" fullWidth onClick={() => setDekontIcin(null)}>
              Vazgeç
            </Button>
          </div>
        )}
      </Modal>
    </div>
  );
}
