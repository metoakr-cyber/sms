'use client';

/**
 * Denetim kaydı — para ve fiyat etkileyen her yönetim işleminin değişmez izi.
 *
 * SUNUM KATMANI: `components/yonetim`. Tablo/kart ikizi, `ErrorBox`, `PAGE`
 * sabiti ve sayfalama şeridi bu dosyadan kalktı.
 *
 * 🔴 AÇ/KAPA KALDIRILDI — ÖNCESİ/SONRASI ARTIK HER ZAMAN GÖRÜNÜR.
 * Özgün ekran her satırda "Göster / Gizle" taşıyordu. Kaldırıldı, çünkü
 * gizlenen şeyin BÜYÜKLÜĞÜ sunucuda sınırlı: `admin.go` içindeki
 * `auditVisibleFields` bir İZİN LİSTESİDİR ve bugün on alan içerir; tanınmayan
 * hiçbir alan dışarı verilmez. Bir kaydın yükü en fazla birkaç satırdır ve
 * satır içine SIĞAR (ölçüm: 1280px'te satır 225px). Gerekçe: §6.4 "önce satır
 * içi alternatifleri tüket" · §4.2 sıklık tablosu (satır başına bir tık,
 * günde onlarca) · hiçbir bilgi erişilemez olmadı — düğmenin açtığı her şey
 * (öncesi, sonrası, gizlenen alanlar, IP, istek no) şimdi tıklamasız görünür.
 * Yan kazanç: özgün satır 352'deki ham `▾` KARAKTERİ de gitti (§9.2).
 *
 * 🔴 HAM JSON DEĞİL, FARK. Özgün ekran `before` ve `after`'ı yan yana iki liste
 * olarak basıyordu; denetçi neyin değiştiğini gözüyle karşılaştırarak buluyordu.
 * Artık alanlar eşleştiriliyor: "öncesi → sonrası", değişenler üstte.
 */
import * as React from 'react';
import { keepPreviousData, useQuery } from '@tanstack/react-query';
import { ApiError, apiFetch } from '@/lib/api';
import { formatDateTime } from '@/lib/format';
import { Badge, Button, Card, Empty, Field } from '@/components/ui';
import {
  apiHatasi,
  KayitSayaci,
  Sayfalama,
  SAYFA_BOYUTU,
  SayfaBasligi,
  VeriTablosu,
  sayfalamaGorunur,
} from '@/components/yonetim';
import type { Sunum, Sutun } from '@/components/yonetim';
import type { AuditLog } from '@/lib/types';

/**
 * Bilinen eylemler ve Türkçe karşılıkları.
 *
 * Bu bir SEÇENEK LİSTESİ değil, ÖNERİ listesidir (`<datalist>`): sunucuya yeni
 * bir denetim eylemi eklendiğinde bu dosya güncellenmemiş olsa bile yönetici
 * adı elle yazıp süzebilir. Kapalı bir `<select>` olsaydı, yeni eylem panelde
 * süzülemez ve pratikte görünmez olurdu.
 *
 * Ham kod ekranda KAYBOLMAZ: Türkçe cümlenin altında ikincil satır olarak
 * durur — süzgeç kutusuna yazılacak değer odur.
 */
const EYLEM_ETIKETLERI: Record<string, string> = {
  'deposit.approve': 'Bakiye talebi onaylandı',
  'deposit.reject': 'Bakiye talebi reddedildi',
  'pricing.rule.create': 'Fiyat kuralı oluşturuldu',
  'pricing.rule.deactivate': 'Fiyat kuralı kapatıldı',
};
const BILINEN_EYLEMLER = Object.keys(EYLEM_ETIKETLERI);
const BILINEN_VARLIKLAR = ['deposit', 'pricing_rule'];

/**
 * Yük alanlarının Türkçe adları — sunucunun izin listesiyle birebir.
 *
 * 🔴 `*Minor` alanları KURUŞTUR ve burada TL'ye ÇEVRİLMEZ. Çevirmek istemcide
 * para aritmetiği olurdu (CLAUDE.md: "İstemcide para aritmetiği yapma") ve
 * denetim kaydı yükünde para birimi alanı YOKTUR — "12500"ü 125,00 ₺ diye
 * yazmak, doğrulanmamış bir varsayımı ekrana basmaktır. Birim etikete yazıldı.
 */
const ALAN_ETIKETLERI: Record<string, string> = {
  status: 'Durum',
  amountMinor: 'Tutar (kuruş)',
  creditedMinor: 'Yüklenen (kuruş)',
  methodName: 'Yöntem',
  hasReceipt: 'Dekont var mı',
  scope: 'Kapsam',
  marginPercent: 'Kâr marjı (%)',
  fixedFeeMinor: 'Sabit ücret (kuruş)',
  minPriceMinor: 'Alt sınır (kuruş)',
  isActive: 'Etkin',
};

/**
 * Değeri gizlenecek alan adları.
 *
 * BİRİNCİL SAVUNMA SUNUCUDADIR: `auditVisibleFields` izin listesi bu adların
 * hiçbirini dışarı vermez, yani bu kontrol bugün HİÇ TETİKLENMEZ. Yine de
 * duruyor: izin listesine ileride eklenecek bir alan (örn. bir sağlayıcı
 * anahtarının parçası) sessizce ekrana düşmesin. Değer gizlenir ama alanın
 * VARLIĞI gizlenmez — denetçi bir şeyin saklandığını görmeli.
 */
const GIZLI_ALAN = /(parola|password|şifre|sifre|hash|secret|token|api[_-]?key|apikey|anahtar)/i;

const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

/**
 * `Ayrıntı` sütununun başlığı — `<th>` ve mobil kart başlığı TEK kaynaktan.
 *
 * 🔴 `const` olduğu için TS'te tipi `string` değil `'Ayrıntı'` LİTERALİdir ve
 * `Ayrinti` bileşeni başlığı tam bu tiple alır. Yani mobil başlığa başka bir
 * dize yazmak DERLEME HATASIdır — aşağıdaki kaçış yolunun notuna bakınız.
 */
const BASLIK_AYRINTI = 'Ayrıntı';

/**
 * `<input type="date">` değerini (YYYY-MM-DD) RFC 3339'a çevirir.
 *
 * Saat dilimi AÇIKÇA +03:00 yazılır, cihazınkiyle DEĞİL: sunucu yalnız RFC 3339
 * kabul eder (Değişmez #19) ve panelin her yerinde tarihler Europe/Istanbul
 * ile gösterilir. Cihazın dilimi kullanılsaydı yurt dışındaki bir yönetici
 * gördüğü günden farklı bir aralığı süzerdi. Türkiye 2016'dan beri kalıcı
 * olarak UTC+03'tür; yaz saati uygulaması yoktur.
 */
function gunBasi(v: string): string | null {
  return /^\d{4}-\d{2}-\d{2}$/.test(v) ? `${v}T00:00:00+03:00` : null;
}
function gunSonu(v: string): string | null {
  return /^\d{4}-\d{2}-\d{2}$/.test(v) ? `${v}T23:59:59+03:00` : null;
}

/** JSON değerini tek satırda okunur yazar; nesne/dizi ise JSON olarak. */
function deger(v: unknown): string {
  if (v === null || v === undefined) return '—';
  if (typeof v === 'boolean') return v ? 'Evet' : 'Hayır';
  if (typeof v === 'string') return v === '' ? '(boş)' : v;
  if (typeof v === 'number') return String(v);
  try {
    return JSON.stringify(v);
  } catch {
    return '(gösterilemedi)';
  }
}

/* ═══════════════════════ Fark çıkarımı ═══════════════════════ */

type FarkTuru = 'degisti' | 'eklendi' | 'silindi' | 'ayni';

interface FarkSatiri {
  anahtar: string;
  etiket: string;
  oncesi: string | null;
  sonrasi: string | null;
  tur: FarkTuru;
}

/** Değişenler üstte, değişmeyenler altta. */
const SIRA: Record<FarkTuru, number> = { degisti: 0, eklendi: 1, silindi: 2, ayni: 3 };

function fark(
  oncesi?: Record<string, unknown>,
  sonrasi?: Record<string, unknown>,
): FarkSatiri[] {
  const anahtarlar = Array.from(
    new Set([...Object.keys(oncesi ?? {}), ...Object.keys(sonrasi ?? {})]),
  );

  const satirlar = anahtarlar.map<FarkSatiri>((k) => {
    const gizli = GIZLI_ALAN.test(k);
    const o = oncesi && k in oncesi ? (gizli ? '••••••' : deger(oncesi[k])) : null;
    const s = sonrasi && k in sonrasi ? (gizli ? '••••••' : deger(sonrasi[k])) : null;
    const tur: FarkTuru =
      o === null ? 'eklendi' : s === null ? 'silindi' : o === s ? 'ayni' : 'degisti';
    return { anahtar: k, etiket: ALAN_ETIKETLERI[k] ?? k, oncesi: o, sonrasi: s, tur };
  });

  // `sort` ECMAScript'te kararlıdır: aynı türdeki alanlar sunucudan geldiği
  // sırayı korur, her yenilemede yer değiştirmez.
  return satirlar.sort((a, b) => SIRA[a.tur] - SIRA[b.tur]);
}

/** "Öncesi → sonrası" okunu çizer. Unicode `→` DEĞİL: §9.2 karakter ikonu yasak. */
function Ok() {
  return (
    <svg
      viewBox="0 0 24 24"
      className="size-4 shrink-0 text-muted"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
    >
      <path d="M4 12h14M13 6l6 6-6 6" />
    </svg>
  );
}

/** Tek bir alanın değişimi. Anlam METİNLE taşınır, okla değil (§7.1). */
function FarkDegeri({ satir }: { satir: FarkSatiri }) {
  if (satir.tur === 'ayni') {
    return <span className="font-medium">{satir.sonrasi}</span>;
  }

  return (
    <span className="flex flex-wrap items-center gap-2">
      <span className="text-muted">
        <span className="sr-only">Öncesi: </span>
        {satir.oncesi ?? '(yoktu)'}
      </span>
      <Ok />
      <span className="font-medium">
        <span className="sr-only">Sonrası: </span>
        {satir.sonrasi ?? '(kaldırıldı)'}
      </span>
    </span>
  );
}

/**
 * Bir kaydın ayrıntısı: fark listesi, gizlenen alanlar, IP ve istek numarası.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * KAÇIŞ YOLU — neden var, neden kapalı
 * ══════════════════════════════════════════════════════════════════════════
 * `VeriTablosu` "kart etiketi = `<th>` etiketi"ni sütun tanımından TÜRETEREK
 * garanti eder — ama yalnız `<dl>` gövdesine inen sütunlar için. Bu sütun
 * `mobilRol: 'eylem'` yuvasındadır ve o yuva bilerek ETİKETSİZDİR: normalde
 * metnini kendi taşıyan bir düğme durur orada. Burada duran şey bir düğme
 * değil, çok satırlı bir İÇERİK bloğu; mobil kartta başlıksız kalırsa
 * kullanıcı neye baktığını bilemez. Yuvayı değiştirmek de çözüm değil:
 * `<dl>` satırı `justify-between` + `text-right`tir, bu blok orada 320px'te
 * ezilir.
 *
 * Denetim raporu bunu haklı olarak "elle atlatılmış garanti" saydı: aynı
 * sabiti kullandığı için BUGÜN tutuyordu, ama tutmaya devam edeceğinin bir
 * güvencesi yoktu. Güvence şimdi TİPTE:
 *
 *     baslik: typeof BASLIK_AYRINTI     // yani `'Ayrıntı'` literali
 *
 * Başlığı prop olarak alıyoruz ve tipi sütunun `baslik` alanına verilen
 * sabitin literal tipi. Buraya başka bir dize geçirmek `tsc` hatasıdır —
 * etiket ayrışması artık derlenmiyor. Prop, sütun tanımında `baslik` ile
 * AYNI SATIRIN yanında geçirilir; ayrışma hem göze hem derleyiciye çarpar.
 */
function Ayrinti({
  log,
  sunum,
  baslik,
}: {
  log: AuditLog;
  sunum: Sunum;
  baslik: typeof BASLIK_AYRINTI;
}) {
  const satirlar = React.useMemo(() => fark(log.before, log.after), [log.before, log.after]);
  const gizlenen = log.redactedFields ?? [];

  return (
    /*
      `text-sm`: tablo `text-sm` taşır ama mobil kartın eylem yuvası taşımaz —
      ölçüldü, aynı blok masaüstünde 14px, kartta 16px çıkıyordu. Aynı bilgi iki
      sunumda iki farklı boyutta yazılmaz (§3.3).
    */
    <div className="flex flex-col gap-3 text-sm">
      {sunum === 'kart' && <p className="font-medium text-muted">{baslik}</p>}

      {satirlar.length === 0 ? (
        <p className="text-muted">Bu işlem bir alan değişikliği kaydetmedi.</p>
      ) : (
        <dl className="flex flex-col gap-2">
          {satirlar.map((s) => (
            <div
              key={s.anahtar}
              /*
                İKİ SÜTUNA `lg:`'DE GEÇER, `sm:`'DE DEĞİL — ölçüldü: bu blok bir
                tablo hücresinde yaşıyor ve hücre genişliği ekranla orantılı
                değil (768px'te sütun 215px, 1280px'te 595px). `sm:` ile 768'de
                160px etiketin yanına 55px kalıyor, satır 617px'e çıkıyordu.
                🔴 `@container` denendi ve GERİ ALINDI: `container-type:inline-size`
                iç boyut hesabını kapatıyor, otomatik yerleşimli tablo hücreye en
                dar genişliği veriyor — sütun 1280px'te 595→87px, satır 661px.
              */
              className="flex flex-col gap-1 lg:flex-row lg:items-baseline lg:gap-4"
            >
              <dt className="text-muted lg:w-40 lg:shrink-0">{s.etiket}</dt>
              <dd className="min-w-0 break-anywhere">
                <FarkDegeri satir={s} />
              </dd>
            </div>
          ))}
        </dl>
      )}

      {gizlenen.length > 0 && (
        /*
          `Alert` KULLANILMADI — gerekçe GÜNCELLENDİ.
          Eski yorum "`ui.tsx:122` `role='alert'`i SABİT yazıyor" diyordu; bu
          artık doğru değil: `Alert` `duyur?: boolean` aldı (`ui.tsx:144`) ve
          `duyur={false}` ile susturulabiliyor. Yani duyuru artık bir engel
          değil. Kutuya geçilmemesinin kalan iki nedeni SUNUM ve YOĞUNLUK:
           · Bu blok bir satırın ayrıntı panelinin İÇİNDEDİR; oraya bir kutu
             koymak İÇ İÇE KAP üretir (§9.2, "nested cards are always wrong").
           · 25 satırlık bir sayfada 25 kutu, ayrıntı panelini tarama yerine
             okuma yüzeyine çevirir (§1.1 taranabilirlik).
          Anlam renkle değil METİNLE taşınıyor ("gizlendi"), renk pekiştiriyor.
          🔴 AÇIK KUSUR: `--color-warn` (#ffa21d) AÇIK temada `--surface`
          (#ffffff) üstünde 2,01:1 — §7.1'in 4,5:1 gövde eşiğinin altında.
          Düzeltme bir jeton gerektiriyor (açık tema için koyulaştırılmış warn)
          ve `globals.css` bu ekranın kapsamı dışında; uydurma renk yazılmadı
          (§12.3 kural 2). Aynı ihlal `kullanicilar:274`, `odeme-yontemleri:284`
          ve `yorumlar:117`'de de duruyor — tek jetonla hepsi kapanır.
        */
        <p className="text-[var(--color-warn)]">
          <span className="font-medium">
            <span className="tabular-nums">{gizlenen.length}</span> alan gizlendi:
          </span>{' '}
          <span className="break-anywhere">{gizlenen.join(', ')}</span>
        </p>
      )}

      {/*
        KÜNYE — kimlikler. Monospace MEŞRU (§9.2): "teknik dursun" diye değil,
        karakter karakter okunup destek ekibine yazılabilsin diye.

        Varlık kimliği AYRI BİR SÜTUN DEĞİL: 36 karakterlik bir UUID sütunu,
        otomatik yerleşimli tabloda "Ayrıntı" sütununu eziyor ve fark satırları
        ("PENDING → APPROVED") iki satıra kırılıyordu (1100px'te ölçüldü).
        Kimlik zaten kaydın künyesine ait bir alan, taranan bir sütun değil.
      */}
      <p className="flex flex-wrap gap-x-4 gap-y-1 text-muted">
        <span className="break-anywhere">
          Varlık kimliği: <span className="font-mono">{log.entityId}</span>
        </span>
        {log.ip && (
          <span>
            IP: <span className="font-mono tabular-nums">{log.ip}</span>
          </span>
        )}
        {log.requestId && (
          <span className="break-anywhere">
            İstek no: <span className="font-mono">{log.requestId}</span>
          </span>
        )}
      </p>
    </div>
  );
}

export default function AdminAuditPage() {
  /* ── Süzgeç (form) durumu ── */
  const [eylem, setEylem] = React.useState('');
  const [varlikTipi, setVarlikTipi] = React.useState('');
  const [aktorId, setAktorId] = React.useState('');
  const [baslangic, setBaslangic] = React.useState('');
  const [bitis, setBitis] = React.useState('');
  const [hatalar, setHatalar] = React.useState<Record<string, string>>({});

  /* ── Uygulanan süzgeç ──
   * Form durumundan AYRIDIR: her tuş vuruşunda sorgu atmak, dakikada 60
   * istekle sınırlı yönetim uçlarında 429 üretir. Süzgeç "Uygula" ile geçer. */
  const [uygulanan, setUygulanan] = React.useState<Record<string, string>>({});
  const [offset, setOffset] = React.useState(0);

  const sorgu = React.useMemo(() => {
    const p = new URLSearchParams(uygulanan);
    p.set('limit', String(SAYFA_BOYUTU));
    p.set('offset', String(offset));
    return p.toString();
  }, [uygulanan, offset]);

  const q = useQuery({
    queryKey: ['admin', 'audit-logs', sorgu],
    queryFn: () =>
      apiFetch<{ items: AuditLog[]; total: number; limit: number; offset: number }>(
        `/admin/audit-logs?${sorgu}`,
      ),
    // Sayfa değişince liste boşalıp zıplamasın.
    placeholderData: keepPreviousData,
  });

  function uygula(e: React.FormEvent) {
    e.preventDefault();
    const errs: Record<string, string> = {};
    const next: Record<string, string> = {};

    if (eylem.trim()) next.action = eylem.trim();
    if (varlikTipi.trim()) next.entityType = varlikTipi.trim();

    const aktor = aktorId.trim();
    if (aktor) {
      if (!UUID_RE.test(aktor)) errs.actorId = 'Aktör kimliği bir UUID olmalıdır.';
      else next.actorId = aktor;
    }

    if (baslangic) {
      const v = gunBasi(baslangic);
      if (!v) errs.from = 'Geçerli bir tarih seçiniz.';
      else next.from = v;
    }
    if (bitis) {
      const v = gunSonu(bitis);
      if (!v) errs.until = 'Geçerli bir tarih seçiniz.';
      else next.until = v;
    }
    if (baslangic && bitis && baslangic > bitis) {
      errs.until = 'Bitiş tarihi başlangıçtan önce olamaz.';
    }

    setHatalar(errs);
    if (Object.keys(errs).length) return;
    setOffset(0);
    setUygulanan(next);
  }

  function temizle() {
    setEylem('');
    setVarlikTipi('');
    setAktorId('');
    setBaslangic('');
    setBitis('');
    setHatalar({});
    setOffset(0);
    setUygulanan({});
  }

  const toplam = q.data?.total ?? 0;
  const satirlar = q.data?.items;
  const suzgecSayisi = Object.keys(uygulanan).length;
  const sayfali = sayfalamaGorunur(toplam);

  /*
   * `apiFetch` her hatayı `ApiError`'a normalize eder; yine de sorgu katmanı
   * `unknown` döndürebilir (örn. render sırasında atılan bir hata). Özgün
   * dosyadaki "Denetim kaydı yüklenemedi" yedeği KORUNDU — aksi hâlde tanınmayan
   * bir hata sessizce BOŞ LİSTE gibi görünürdü, ki bu bir denetim ekranında
   * en kötü yanlış anlamadır.
   */
  const hata =
    apiHatasi(q.error) ??
    (q.isError
      ? new ApiError({
          code: 'UNKNOWN',
          message: 'Denetim kaydı yüklenemedi. Bağlantınızı kontrol edip tekrar deneyin.',
        })
      : null);

  /*
   * SÜTUNLAR — tek veri tanımı (özgün dosyada mobil kart + tablo iki ayrı
   * yerde yazılıydı ve zaten ayrışmıştı: kart yalnız "N alan gizlendi" sayısını,
   * tablo alan ADLARINI gösteriyordu).
   *
   * "Zaman" sütunu `sayisal` (tabular-nums) ama SOLA hizalı: §6.1 kural 4'ün
   * amacı rakamların alt alta hizalanmasıdır ve biçimlenmiş tarihler zaten
   * eşit genişliktedir — sağa hizalamak ilk sütunu gövdeden koparırdı.
   */
  const sutunlar = React.useMemo<ReadonlyArray<Sutun<AuditLog>>>(
    () => [
      {
        anahtar: 'zaman',
        baslik: 'Zaman',
        sayisal: true,
        hucre: (log) => <span className="text-muted">{formatDateTime(log.createdAt)}</span>,
      },
      {
        anahtar: 'aktor',
        baslik: 'Aktör',
        /*
         * `break-anywhere` YOK — ölçüldü: `overflow-wrap: anywhere` bir hücrenin
         * min-content genişliğini TEK KARAKTERE indirir. Otomatik yerleşimli bir
         * tabloda geniş "Ayrıntı" sütunu bu hücreyi ezer ve kullanıcı adı
         * "yonetic / i" diye kelime ortasından bölünür (1100px'te görüldü).
         */
        hucre: (log) => <span>{log.actorUsername || '(sistem)'}</span>,
      },
      {
        anahtar: 'eylem',
        baslik: 'Eylem',
        mobilRol: 'baslik',
        hucre: (log) => (
          <>
            <p className="font-medium">{EYLEM_ETIKETLERI[log.action] ?? log.action}</p>
            {EYLEM_ETIKETLERI[log.action] && (
              // Süzgeç kutusuna yazılacak değer bu — cümlenin arkasında kalmaz.
              <p className="mt-1 text-sm text-muted">{log.action}</p>
            )}
          </>
        ),
      },
      {
        anahtar: 'varlik',
        baslik: 'Varlık',
        mobilRol: 'rozet',
        hucre: (log) => (
          <Badge tone="neutral">
            <span className="sr-only">Varlık: </span>
            {log.entityType}
          </Badge>
        ),
      },
      {
        anahtar: 'ayrinti',
        baslik: BASLIK_AYRINTI,
        // `mobilRol: 'eylem'` yuvası ETİKETSİZDİR; bu sütun bir düğme değil
        // içerik bloğu olduğu için başlığını kendi yazar. Başlık BURADAN,
        // `baslik` ile aynı ifadeden geçer — `Ayrinti`nin prop tipi o sabitin
        // literal tipi olduğu için başka bir dize DERLENMEZ (bkz. `Ayrinti`).
        mobilRol: 'eylem',
        hucre: (log, sunum) => <Ayrinti log={log} sunum={sunum} baslik={BASLIK_AYRINTI} />,
      },
    ],
    [],
  );

  return (
    <div className="flex flex-col gap-6">
      <SayfaBasligi
        baslik="Denetim kaydı"
        aciklama="Para ve fiyat etkileyen her yönetim işlemi burada, değiştirilemez biçimde durur."
      />

      {/* ═══════════ Süzgeçler ═══════════ */}
      <Card>
        <h2 className="text-lg font-semibold">Süzgeçler</h2>

        <form onSubmit={uygula} className="mt-4 flex flex-col gap-6" noValidate>
          <div className="grid gap-6 md:grid-cols-2">
            <div>
              <Field
                label="Eylem"
                list="denetim-eylemler"
                value={eylem}
                onChange={(e) => setEylem(e.target.value)}
                placeholder="Örn. deposit.approve"
                autoCapitalize="none"
                autoCorrect="off"
                spellCheck={false}
                hint="Boş bırakılırsa tüm eylemler."
              />
              <datalist id="denetim-eylemler">
                {BILINEN_EYLEMLER.map((a) => (
                  <option key={a} value={a} />
                ))}
              </datalist>
            </div>

            <div>
              <Field
                label="Varlık tipi"
                list="denetim-varliklar"
                value={varlikTipi}
                onChange={(e) => setVarlikTipi(e.target.value)}
                placeholder="Örn. deposit"
                autoCapitalize="none"
                autoCorrect="off"
                spellCheck={false}
                hint="Boş bırakılırsa tüm varlıklar."
              />
              <datalist id="denetim-varliklar">
                {BILINEN_VARLIKLAR.map((t) => (
                  <option key={t} value={t} />
                ))}
              </datalist>
            </div>
          </div>

          <Field
            label="Aktör (kullanıcı kimliği)"
            value={aktorId}
            onChange={(e) => setAktorId(e.target.value)}
            placeholder="00000000-0000-0000-0000-000000000000"
            autoCapitalize="none"
            autoCorrect="off"
            spellCheck={false}
            error={hatalar.actorId}
            hint="İşlemi yapan yöneticinin genel kimliği (UUID)."
          />

          <div className="grid gap-6 md:grid-cols-2">
            <Field
              label="Başlangıç tarihi"
              type="date"
              value={baslangic}
              onChange={(e) => setBaslangic(e.target.value)}
              error={hatalar.from}
              hint="Gün başından itibaren (Türkiye saati)."
            />
            <Field
              label="Bitiş tarihi"
              type="date"
              value={bitis}
              onChange={(e) => setBitis(e.target.value)}
              error={hatalar.until}
              hint="Gün sonuna kadar (Türkiye saati)."
            />
          </div>

          <div className="flex flex-col gap-2 sm:flex-row">
            <Button type="submit" fullWidth className="sm:w-auto">
              Süzgeci uygula
            </Button>
            <Button
              type="button"
              variant="outline"
              fullWidth
              className="sm:w-auto"
              onClick={temizle}
              disabled={suzgecSayisi === 0}
            >
              Temizle
            </Button>
          </div>
        </form>
      </Card>

      {/* ═══════════ Kayıtlar ═══════════ */}
      <Card>
        <div className="flex flex-wrap items-center justify-between gap-4">
          <h2 className="text-lg font-semibold">Kayıtlar</h2>
          <div className="flex flex-wrap items-center gap-2">
            {suzgecSayisi > 0 && (
              <Badge tone="neutral">
                <span className="tabular-nums">{suzgecSayisi}</span>
                <span className="ms-1">süzgeç etkin</span>
              </Badge>
            )}
            <KayitSayaci toplam={toplam} />
          </div>
        </div>

        {/*
          Gizlenen alan açıklaması SATIR BAŞINA DEĞİL, bir kez burada duruyor:
          25 satırın her birinde tekrarlanan bir cümle bilgi değil gürültüdür.
          Satırda yalnız hangi alanların gizlendiği yazar.
        */}
        <p className="mt-2 max-w-[70ch] text-sm text-muted">
          Denetim kaydı yalnız izin listesindeki alanları dışarı verir; bir kayıtta
          saklanan alan varsa adı o kaydın altında bildirilir.
        </p>

        <VeriTablosu
          className="mt-4"
          baslik="Yönetim işlemlerinin denetim kaydı"
          sutunlar={sutunlar}
          satirlar={satirlar}
          satirAnahtari={(log) => String(log.id)}
          yukleniyor={q.isLoading}
          hata={hata}
          // Sayfalama kendi aralığını duyuruyor; iki canlı bölge çakışmasın.
          duyuru={!sayfali}
          bos={
            <Empty
              title="Kayıt bulunamadı"
              // §6.3: süzgeçten dolayı boş ≠ gerçekten boş.
              hint={
                suzgecSayisi > 0
                  ? 'Süzgeçleri gevşetip tekrar deneyin.'
                  : 'Yönetim işlemleri yapıldıkça burada görünecek.'
              }
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
    </div>
  );
}
