'use client';

/**
 * Ödeme yöntemleri.
 *
 * Buradaki IBAN/cüzdan adresi, kullanıcının parayı GÖNDERECEĞİ yerdir. Yanlış
 * bir karakter, paranın başkasına gitmesi demektir — bu yüzden:
 *   · yeni yöntem PASİF doğar (sunucu),
 *   · eksik alanlı yöntem aktifleştirilemez (sunucu 422 + burada kilitli düğme),
 *   · `config` kısmi gönderilmez; sunucuda MERGE DEĞİL, YERİNE GEÇER.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * BU DALGADA NE DEĞİŞTİ (davranış DEĞİL, yapı)
 * ══════════════════════════════════════════════════════════════════════════
 * Mobil kart listesi ile masaüstü tablosu elle İKİ KEZ yazılıyordu (~110 satır
 * ikiz kod). İkisi de `VeriTablosu`'nun tek sütun tanımından türüyor artık;
 * bir etiketi bir yerde değiştirip diğerini unutmak YAPISAL OLARAK imkânsız.
 * `toMinor` / `fromMinor` / `ErrorBox` / `selectClass` / `Chevron` /
 * `textareaClass` / `useDialogFocus` yerel kopyaları kaldırıldı — hepsinin
 * ortak karşılığı var (`lib/para.ts`, `components/yonetim`).
 */

import * as React from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { apiFetch } from '@/lib/api';
import { formatMoney } from '@/lib/format';
import { TUTAR_ZORUNLU, minorToText, toMinor } from '@/lib/para';
import { Alert, Button, Card, Empty, Field, cx } from '@/components/ui';
import { Modal } from '@/components/modal';
import {
  CokSatir,
  DurumRozeti,
  HataDurumu,
  OnayDiyalogu,
  SayfaBasligi,
  Secim,
  VeriTablosu,
  apiHatasi,
  ikiliTon,
  type Sutun,
} from '@/components/yonetim';
import type { DepositMethod } from '@/lib/types';

interface DepositMethodList { items: DepositMethod[] }

type Kind = DepositMethod['kind'];

const KIND_LABEL: Record<Kind, string> = {
  BANK_TRANSFER: 'Banka havalesi / EFT',
  CRYPTO: 'Kripto',
};

/**
 * Yönteme göre yapılandırma alanları.
 *
 * Anahtar adları SUNUCUYLA AYNI olmak zorundadır: aktifleştirme ön koşulu
 * `iban`/`hesapAdi` ve `cuzdanAdresi`/`ag` anahtarlarına bakar
 * (handler/admin.go: missingConfigFields). Bir harf farkı, dolu bir formun
 * "eksik alan" hatası almasıdır.
 */
const CONFIG_FIELDS: Record<Kind, Array<{ key: string; label: string; required: boolean; placeholder?: string }>> = {
  BANK_TRANSFER: [
    { key: 'banka', label: 'Banka', required: false, placeholder: 'Örn. Ziraat Bankası' },
    { key: 'hesapAdi', label: 'Hesap adı', required: true, placeholder: 'Hesabın açık adı' },
    { key: 'iban', label: 'IBAN', required: true, placeholder: 'TR00 0000 0000 0000 0000 0000 00' },
  ],
  CRYPTO: [
    { key: 'ag', label: 'Ağ', required: true, placeholder: 'Örn. TRC20' },
    { key: 'cuzdanAdresi', label: 'Cüzdan adresi', required: true, placeholder: 'Cüzdan adresi' },
  ],
};

/** Sunucunun tutar sınırları (dto/deposit.go). Bilgi amaçlı gösterilir. */
const HARD_MIN_MINOR = 1000;
const HARD_MAX_MINOR = 5_000_000;

/* ═══════════════════════ Liste parçaları ═══════════════════════ */

/** `maxAmount.minor === 0` ÜST SINIR YOK demektir; 0,00 ₺ tavan DEĞİL. */
function amountRange(m: DepositMethod): string {
  const min = formatMoney(m.minAmount);
  return m.maxAmount.minor === 0 ? `${min} ve üzeri` : `${min} – ${formatMoney(m.maxAmount)}`;
}

function ConfigSummary({ method }: { method: DepositMethod }) {
  const fields = CONFIG_FIELDS[method.kind];
  const filled = fields.filter((f) => (method.config[f.key] ?? '').trim() !== '');
  if (!filled.length) return <span className="text-muted">Bilgi girilmemiş</span>;
  return (
    <div className="flex flex-col gap-1">
      {filled.map((f) => (
        <p key={f.key} className="break-anywhere">
          <span className="text-muted">{f.label}: </span>
          {method.config[f.key]}
        </p>
      ))}
    </div>
  );
}

/**
 * Aktifleştirmeyi engelleyen eksik alanlar — yoksa `null`.
 *
 * Eksik alan varsa "Aktifleştir" düğmesi KİLİTLİDİR ve nedeni ALTINDA YAZAR.
 * Sunucu da reddeder (422); buradaki kilit, yöneticiyi anlamsız bir hataya
 * çarptırmamak içindir. PASİFLEŞTİRME HER ZAMAN SERBESTTİR — bir yöntemi
 * hızla kapatmak için hiçbir ön koşul aranmaz; bu yüzden kontrol
 * `!m.isActive` ile başlar.
 *
 * 🔴 `title` TEK BAŞINA YETMEZ: dokunmatik ekranda hover yoktur, yani
 * "Önce doldurun: iban" ipucu telefonda HİÇ görünmez
 * (frontend-contract.md §2.3). Özgün kodda bu metin masaüstünde durum
 * rozetinin altındaydı, mobilde ayrı bir yerde. Artık tek yerde ve KİLİTLİ
 * DÜĞMENİN yanında: açıkladığı şey durum değil, çalışmayan düğmedir.
 */
function aktiflikEngeli(m: DepositMethod): string[] | null {
  const eksik = m.missingFields ?? [];
  return !m.isActive && eksik.length > 0 ? eksik : null;
}

/* ═══════════════════════ Ekran ═══════════════════════ */

type Dialog =
  | { kind: 'create' }
  | { kind: 'edit'; method: DepositMethod }
  | { kind: 'delete'; method: DepositMethod }
  | null;

export default function DepositMethodsPage() {
  const qc = useQueryClient();
  const [dialog, setDialog] = React.useState<Dialog>(null);

  const q = useQuery({
    queryKey: ['admin-deposit-methods'],
    // 🔴 Yanıt {items:[...]} — total/limit/offset YOKTUR, sayfalama da yok.
    queryFn: () => apiFetch<DepositMethodList>('/admin/deposit-methods'),
  });

  const setActive = useMutation({
    mutationFn: (v: { id: string; isActive: boolean }) =>
      apiFetch<DepositMethod>(`/admin/deposit-methods/${encodeURIComponent(v.id)}/active`, {
        method: 'PATCH',
        body: { isActive: v.isActive },
      }),
    retry: false,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['admin-deposit-methods'] }),
  });

  const listErr = apiHatasi(q.error);
  // Aktifleştirme reddi `fields` DEĞİL, düz bir mesajdır (handler/admin.go).
  const activeErr = apiHatasi(setActive.error);
  const items = q.data?.items;

  /*
   * SÜTUNLAR — tek tanım, iki sunum.
   *
   * `oncelik: 3` verilen dört sütun `lg:` altında gizlenir ve HİÇBİR BİLGİ
   * KAYBOLMAZ: dördü de mobil kartta ve düzenleme formunda durmaya devam
   * eder (§6.1 kural 3). 768px'te görünen dört sütun — Yöntem, Tutar
   * aralığı, Durum, İşlemler — yatay kaydırma üretmeden sığar.
   */
  const sutunlar: ReadonlyArray<Sutun<DepositMethod>> = [
    {
      anahtar: 'yontem',
      baslik: 'Yöntem',
      mobilRol: 'baslik',
      /*
        🔴 ÜST SINIR ŞART: `truncate` = `white-space: nowrap`, ve
        `table-layout: auto` hücrenin max-content genişliğini içerikten
        hesaplar — uzun bir yöntem adı tabloyu ZORLA genişletir, kırpma hiç
        devreye girmez (`talepler`de 768px'te ölçüldü: 806px). Kartta sınır
        yoktur; orada kap `min-w-0` bir flex öğesidir.
      */
      hucre: (m, sunum) => (
        <div className={sunum === 'tablo' ? 'max-w-[9rem] lg:max-w-[12rem]' : 'min-w-0'}>
          <span className="block truncate font-medium">{m.name}</span>
          {/* Monospace MEŞRU (§9.2): `code` bir kimliktir, karakter karakter okunur. */}
          <code className="block truncate font-mono text-sm text-muted">{m.code}</code>
        </div>
      ),
    },
    {
      anahtar: 'tip',
      baslik: 'Tip',
      oncelik: 3,
      hucre: (m) => <span className="whitespace-nowrap">{KIND_LABEL[m.kind]}</span>,
    },
    {
      anahtar: 'bilgiler',
      baslik: 'Bilgiler',
      oncelik: 3,
      hucre: (m, sunum) => (
        <div className={sunum === 'tablo' ? 'max-w-[16rem]' : undefined}>
          <ConfigSummary method={m} />
        </div>
      ),
    },
    {
      anahtar: 'aralik',
      baslik: 'Tutar aralığı',
      /*
        `sayisal: true` VERİLMEDİ, `tabular-nums` ELLE yazıldı — bilerek.
        `VeriTablosu` `sayisal` ile `tabular-nums` + `whitespace-nowrap`'i
        birlikte uygular; ikincisi TEK bir tutar için doğru, bir ARALIK için
        değil: "10.000,00 ₺ – 500.000,00 ₺" kırılamayınca sütun ~190px
        istiyor ve 768px'te eylem sütununu düğmeleri alt alta itecek kadar
        eziyor (ölçüldü). §3.5'in istediği şey `tabular-nums`; `nowrap` onun
        gereği değil, `sayisal` bayrağının paket arkadaşı.
      */
      hucre: (m) => <span className="tabular-nums">{amountRange(m)}</span>,
    },
    {
      anahtar: 'sira',
      baslik: 'Sıra',
      hizala: 'sag',
      sayisal: true,
      oncelik: 3,
      hucre: (m) => m.sortOrder,
    },
    {
      anahtar: 'durum',
      baslik: 'Durum',
      mobilRol: 'rozet',
      // `DurumRozeti` üç kanal taşır: metin + biçim (SVG) + renk. Açık temada
      // durum renkleri kontrast eşiğini geçemiyor (rapora bkz.) — metin ve
      // biçim kanalları tam bu yüzden teorik değil.
      hucre: (m) => (
        <DurumRozeti
          durum={m.isActive ? 'ACTIVE' : 'INACTIVE'}
          ton={ikiliTon(m.isActive)}
          etiket={m.isActive ? 'Aktif' : 'Pasif'}
        />
      ),
    },
    {
      anahtar: 'islemler',
      baslik: 'İşlemler',
      basligiGizle: true,
      hizala: 'sag',
      mobilRol: 'eylem',
      /*
        `sunum` YALNIZ SUNUM FARKI İÇİN kullanılır (§6): mobilde tam genişlik
        alt alta, masaüstünde içerik kadar yan yana. METİNLER İKİSİNDE DE
        AYNIDIR — ölçülen etiket ayrışmalarının tamamı buradan doğmuştu.
      */
      hucre: (m, sunum) => {
        const kart = sunum === 'kart';
        const engel = aktiflikEngeli(m);
        return (
          /*
            `min-w-[13rem]` masaüstünde: eylem sütunu üç düğmeyi ALT ALTA
            itmesin. Ölçüldü — sınırsız bırakıldığında tablo bu sütuna
            WebKit'te 134px veriyor ve satır 249px'e çıkıyor (Chromium 177px;
            iki motor genişliği farklı dağıtıyor, yani "Chromium'da iyi
            görünüyor" bir kanıt değil). 13rem iki düğmeyi yan yana tutar ve
            768px'te tablonun min-content toplamını aşırtmaz.
          */
          <div className={cx('flex flex-col gap-2', !kart && 'min-w-[13rem] items-end')}>
            <div className={kart ? 'flex flex-col gap-2' : 'flex flex-wrap justify-end gap-2'}>
              <Button
                variant="outline" size="sm" fullWidth={kart}
                onClick={() => setDialog({ kind: 'edit', method: m })}
              >
                Düzenle
              </Button>
              <Button
                variant="outline" size="sm" fullWidth={kart}
                disabled={setActive.isPending || Boolean(engel)}
                title={engel ? `Önce doldurun: ${engel.join(', ')}` : undefined}
                onClick={() => setActive.mutate({ id: m.id, isActive: !m.isActive })}
              >
                {m.isActive ? 'Pasifleştir' : 'Aktifleştir'}
              </Button>
              <Button
                variant="danger" size="sm" fullWidth={kart}
                onClick={() => setDialog({ kind: 'delete', method: m })}
              >
                Sil
              </Button>
            </div>
            {engel && (
              // `text-sm`, `text-xs` değil (§3.2): bu metin yöneticinin
              // düğmeyi neden kullanamadığını anlatan asıl bilgidir.
              <p className={cx('text-sm text-[var(--color-warn)]', !kart && 'text-right')}>
                Aktifleştirilemez — eksik: {engel.join(', ')}
              </p>
            )}
          </div>
        );
      },
    },
  ];

  return (
    <div className="mx-auto flex max-w-5xl flex-col gap-6">
      {/*
        "Yeni yöntem" düğmesi kart başlığından SAYFA BAŞLIĞINA taşındı.
        Gerekçe: birincil ekleme eylemi tüm yönetim ekranlarında AYNI YERDE
        durmalı (§9.2 "ekranlar arası tutarsız bileşen dili") ve `SayfaBasligi`
        bu yuvayı tam bunun için taşıyor. Eylem kaldırılmadı, yalnız yeri
        sabitlendi; boş durumda ayrıca ikinci bir kopyası sunulur.
      */}
      <SayfaBasligi
        baslik="Ödeme yöntemleri"
        aciklama="Kullanıcıların bakiye yüklerken göreceği hesap bilgileri. Yalnız aktif yöntemler kullanıcıya gösterilir."
      >
        <Button onClick={() => setDialog({ kind: 'create' })}>Yeni yöntem</Button>
      </SayfaBasligi>

      {/* `duyur={false}`: sayfa açılışında koşulsuz çizilen statik uyarı —
          kesintili duyuru için bir eylem yok (§7.4). Metin ekranda ve okuma
          sırasında yerinde durur; yalnız "sözü kes" bayrağı kalkar. */}
      <Alert tone="warn" duyur={false}>
        Buradaki IBAN ve cüzdan adresi kullanıcının parayı göndereceği yerdir.
        Kaydetmeden önce karakter karakter doğrulayın; yanlış bir adres, paranın
        geri getirilemeyeceği bir yere gitmesi demektir.
      </Alert>

      {activeErr && <HataDurumu hata={activeErr} />}

      <Card>
        <h2 className="text-lg font-semibold">Tanımlı yöntemler</h2>

        <VeriTablosu
          className="mt-5"
          baslik="Tanımlı ödeme yöntemleri"
          sutunlar={sutunlar}
          satirlar={items}
          satirAnahtari={(m) => m.id}
          yukleniyor={q.isLoading}
          hata={listErr}
          iskeletSatir={3}
          bos={
            /*
              BOŞ DURUM ÖĞRETİR VE BİR EYLEM SUNAR (§6.3 · operate.md:35).
              Burada süzgeç yoktur, yani "sonuç yok" değil GERÇEKTEN BOŞ
              durumudur — doğru metin "ilk kaydı oluştur"dur.
              🔴 `Empty` bileşeninin eylem (CTA) yuvası YOK; bu yüzden düğme
              dışarıdan ekleniyor. Katman raporuna yazıldı.
            */
            <div className="flex flex-col items-center gap-4">
              <Empty
                title="Henüz yöntem yok"
                hint="Kullanıcılar bakiye yükleyemez. En az bir yöntem ekleyip bilgilerini doldurun."
              />
              <Button onClick={() => setDialog({ kind: 'create' })}>İlk yöntemi ekle</Button>
            </div>
          }
        />
      </Card>

      {dialog?.kind === 'create' && (
        <MethodForm onClose={() => setDialog(null)} />
      )}
      {dialog?.kind === 'edit' && (
        <MethodForm key={dialog.method.id} method={dialog.method} onClose={() => setDialog(null)} />
      )}
      {dialog?.kind === 'delete' && (
        <DeleteDialog key={dialog.method.id} method={dialog.method} onClose={() => setDialog(null)} />
      )}
    </div>
  );
}

/* ═══════════════════════ Ekle / düzenle ═══════════════════════ */

function MethodForm({ method, onClose }: { method?: DepositMethod; onClose: () => void }) {
  const qc = useQueryClient();
  const isEdit = !!method;

  const [code, setCode] = React.useState(method?.code ?? '');
  const [kind, setKind] = React.useState<Kind>(method?.kind ?? 'BANK_TRANSFER');
  const [name, setName] = React.useState(method?.name ?? '');
  const [instructions, setInstructions] = React.useState(method?.instructions ?? '');
  const [minAmount, setMinAmount] = React.useState(
    method ? minorToText(method.minAmount.minor) : '',
  );
  const [maxAmount, setMaxAmount] = React.useState(
    method && method.maxAmount.minor > 0 ? minorToText(method.maxAmount.minor) : '',
  );
  const [sortOrder, setSortOrder] = React.useState(String(method?.sortOrder ?? 0));

  /*
   * config BÜTÜN OLARAK tutulur.
   *
   * Sunucu gelen config'i mevcutla BİRLEŞTİRMEZ, üzerine yazar: yalnız
   * "hesapAdi" gönderen bir istek IBAN'ı SİLER. Bu yüzden formun durumu
   * mevcut config'in TAM kopyasıdır — formda göstermediğimiz (ileride
   * eklenmiş) anahtarlar da burada durur ve aynen geri gider.
   */
  const [config, setConfig] = React.useState<Record<string, string>>({ ...(method?.config ?? {}) });
  const [errors, setErrors] = React.useState<Record<string, string>>({});

  const save = useMutation({
    mutationFn: (body: Record<string, unknown>) =>
      isEdit
        ? apiFetch<DepositMethod>(`/admin/deposit-methods/${encodeURIComponent(method.id)}`, {
            method: 'PATCH', body,
          })
        : apiFetch<DepositMethod>('/admin/deposit-methods', { method: 'POST', body }),
    retry: false,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['admin-deposit-methods'] });
      onClose();
    },
  });

  const fields = CONFIG_FIELDS[kind];

  function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    const errs: Record<string, string> = {};

    if (!isEdit) {
      const c = code.trim();
      if (!c) errs.code = 'Kod zorunludur.';
      else if (c.length > 40) errs.code = 'Kod en fazla 40 karakter olabilir.';
    }
    if (!name.trim()) errs.name = 'Ad zorunludur.';

    // TUTAR_ZORUNLU: boş = hata, negatif = ret. Bu ekranın özgün politikası
    // budur ve `bakiye` ekranının `izinNegatif` ayarıyla KARIŞTIRILMAZ.
    const min = toMinor(minAmount, TUTAR_ZORUNLU);
    if ('error' in min) errs.minAmount = min.error;
    else if (min.minor < HARD_MIN_MINOR) {
      errs.minAmount = 'En az tutar 10,00 ₺ altına inemez (sistem alt sınırı).';
    }

    // Boş = üst sınır yok → 0 gönderilir.
    const maxRaw = maxAmount.trim();
    let maxMinor = 0;
    if (maxRaw !== '') {
      const max = toMinor(maxRaw, TUTAR_ZORUNLU);
      if ('error' in max) errs.maxAmount = max.error;
      else {
        maxMinor = max.minor;
        if (maxMinor > HARD_MAX_MINOR) {
          errs.maxAmount = 'En çok tutar 50.000,00 ₺ üstüne çıkamaz (sistem üst sınırı).';
        } else if ('minor' in min && maxMinor < min.minor) {
          errs.maxAmount = 'En çok tutar, en az tutardan küçük olamaz.';
        }
      }
    }

    const order = Number(sortOrder.trim());
    if (!Number.isInteger(order) || order < 0) errs.sortOrder = 'Sıra 0 veya pozitif tam sayı olmalıdır.';

    for (const f of fields) {
      if (f.required && (config[f.key] ?? '').trim() === '') {
        // Sunucu bu alanları AKTİFLEŞTİRMEDE zorunlu tutar; kaydetmede değil.
        // Yine de burada uyarırız: kaydedip aktifleştiremeyen yönetici,
        // hatayı iki ekran sonra görür.
        errs[`config.${f.key}`] = `${f.label} olmadan yöntem aktifleştirilemez.`;
      }
    }

    setErrors(errs);
    if (Object.keys(errs).length) return;

    const cleanConfig: Record<string, string> = {};
    for (const [k, v] of Object.entries(config)) {
      const t = v.trim();
      if (t !== '') cleanConfig[k] = t;
    }

    const body: Record<string, unknown> = {
      name: name.trim(),
      instructions: instructions.trim(),
      config: cleanConfig, // TAM config — kısmi gönderim mevcut bilgileri siler
      minAmountMinor: 'minor' in min ? min.minor : 0,
      maxAmountMinor: maxMinor,
      sortOrder: order,
    };
    if (!isEdit) {
      body.code = code.trim();
      body.kind = kind;
    }
    save.mutate(body);
  }

  const err = apiHatasi(save.error);

  return (
    // Odak: `modal.tsx` açılışta `[data-autofocus]` öğesini bulup odaklar.
    // Yerel `useDialogFocus` kopyası KALDIRILDI — `modal.tsx`'teki
    // `(ilk ?? panel)?.focus()` zinciri artık doğru çalışıyor, ikinci bir
    // setTimeout hilesi gerekmiyor.
    <Modal open onClose={onClose} title={isEdit ? 'Yöntemi düzenle' : 'Yeni ödeme yöntemi'}>
      <form onSubmit={onSubmit} className="flex flex-col gap-5" noValidate>
        {/*
          `duyur={false}` — bu kutular DİYALOĞUN İLK ÇİZİMİNDE vardır. Diyalog
          açıldığında ekran okuyucu zaten diyaloğun adını ("Yöntemi düzenle"),
          rolünü ve odaklanan alanı okur; `role="alert"` bu duyurunun ÜSTÜNE
          bindirilen ikinci, kesintili bir duyurudur ve ilkini kırpar. Kutu
          diyaloğun gövde metnidir, bir olayın sonucu değil (§7.4).
        */}
        {isEdit ? (
          <Alert tone="info" duyur={false}>
            Kod ve tip değiştirilemez. Bilgiler kaydedildiğinde eski değerlerin
            <strong> yerine geçer</strong>; boş bıraktığınız bir alan silinir.
          </Alert>
        ) : (
          <Alert tone="info" duyur={false}>
            Yeni yöntem <strong>pasif</strong> olarak eklenir. Bilgilerini
            doldurup listeden aktifleştirene kadar kullanıcıya görünmez.
          </Alert>
        )}

        {!isEdit && (
          <>
            <Field
              data-autofocus
              label="Kod"
              value={code} onChange={(e) => setCode(e.target.value)}
              placeholder="Örn. ziraat-tl"
              autoCapitalize="none" autoCorrect="off" spellCheck={false}
              maxLength={40} error={errors.code}
              hint="Benzersiz, değiştirilemez teknik ad."
            />
            <Secim
              etiket="Tip"
              value={kind}
              onChange={(e) => setKind(e.target.value as Kind)}
              ipucu="İstenen bilgiler tipe göre değişir ve sonradan değiştirilemez."
            >
              <option value="BANK_TRANSFER">{KIND_LABEL.BANK_TRANSFER}</option>
              <option value="CRYPTO">{KIND_LABEL.CRYPTO}</option>
            </Secim>
          </>
        )}

        <Field
          data-autofocus={isEdit ? true : undefined}
          label="Kullanıcıya görünen ad"
          value={name} onChange={(e) => setName(e.target.value)}
          placeholder="Örn. Ziraat Bankası (TL)"
          maxLength={120} error={errors.name}
        />

        <fieldset className="flex flex-col gap-4 rounded-xl border border-[var(--border)] p-4">
          {/* `<legend>` — bu bir alan GRUBUDUR; ekran okuyucu grubun adını
              her alanla birlikte okur, ayrı bir `<p>` bunu yapmaz. */}
          {/* `p-0`: tarayıcı `legend`'e varsayılan yatay dolgu verir ve
              etiket, altındaki alanlarla 2px kayar. */}
          <legend className="p-0 text-sm font-medium">{KIND_LABEL[kind]} bilgileri</legend>
          {fields.map((f) => (
            <Field
              key={f.key}
              label={f.required ? `${f.label} (zorunlu)` : `${f.label} (isteğe bağlı)`}
              value={config[f.key] ?? ''}
              onChange={(e) => setConfig((c) => ({ ...c, [f.key]: e.target.value }))}
              placeholder={f.placeholder}
              autoCapitalize="none" autoCorrect="off" spellCheck={false}
              error={errors[`config.${f.key}`]}
            />
          ))}
        </fieldset>

        <CokSatir
          etiket="Kullanıcıya gösterilecek açıklama"
          rows={4}
          value={instructions} onChange={(e) => setInstructions(e.target.value)}
          placeholder="Örn. Açıklama alanına kullanıcı adınızı yazınız. Havale hafta içi 1 saat içinde onaylanır."
          ipucu="Yükleme ekranında bu yöntemin altında görünür."
        />

        <Field
          label="En az tutar (TL)"
          value={minAmount} onChange={(e) => setMinAmount(e.target.value)}
          placeholder="Örn. 100,00"
          inputMode="decimal" autoComplete="off"
          error={errors.minAmount}
          hint="Sistem alt sınırı 10,00 ₺."
        />

        <Field
          label="En çok tutar (TL)"
          value={maxAmount} onChange={(e) => setMaxAmount(e.target.value)}
          placeholder="Boş bırakın: üst sınır yok"
          inputMode="decimal" autoComplete="off"
          error={errors.maxAmount}
          hint="Boş bırakılırsa üst sınır uygulanmaz. Sistem üst sınırı 50.000,00 ₺."
        />

        <Field
          label="Sıra"
          value={sortOrder} onChange={(e) => setSortOrder(e.target.value)}
          inputMode="numeric" autoComplete="off"
          error={errors.sortOrder}
          hint="Küçük sayı önce gösterilir."
        />

        {err && <HataDurumu hata={err} />}

        {/* Buton düzeni `OnayDiyalogu` ile AYNI: mobilde alt alta (birincil
            üstte, parmak menzilinde), `sm:` üstünde birincil sağda. */}
        <div className="flex flex-col gap-2 sm:flex-row-reverse sm:justify-start">
          <Button type="submit" fullWidth loading={save.isPending} className="sm:w-auto">
            {isEdit ? 'Değişiklikleri kaydet' : 'Yöntemi ekle'}
          </Button>
          <Button
            type="button" variant="outline" fullWidth
            disabled={save.isPending} onClick={onClose} className="sm:w-auto"
          >
            Vazgeç
          </Button>
        </div>
      </form>
    </Modal>
  );
}

/* ═══════════════════════ Silme onayı ═══════════════════════ */

function DeleteDialog({ method, onClose }: { method: DepositMethod; onClose: () => void }) {
  const qc = useQueryClient();

  const del = useMutation({
    // 🔴 DELETE 204 döner: apiFetch `undefined` verir, dönüş değeri OKUNMAZ.
    mutationFn: () =>
      apiFetch<void>(`/admin/deposit-methods/${encodeURIComponent(method.id)}`, { method: 'DELETE' }),
    retry: false,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['admin-deposit-methods'] });
      onClose();
    },
  });

  return (
    /*
      🔴 İLK ODAK "VAZGEÇ"TEDİR — `OnayDiyalogu` bunu garanti eder
      (`data-autofocus` iptal düğmesindedir). Özgün kod da böyleydi ve bu
      davranış KORUNDU: yıkıcı bir işlemi basılı kalan tek bir tuş
      tetikleyemez (§7.5).
    */
    <OnayDiyalogu
      acik
      baslik="Yöntemi sil"
      yikici
      uyari={
        <>
          <p><strong>{method.name}</strong> kalıcı olarak silinecek.</p>
          <p className="mt-2">Bu işlem geri alınamaz.</p>
        </>
      }
      onayMetni="Evet, sil"
      bekliyor={del.isPending}
      hata={apiHatasi(del.error)}
      onOnayla={() => del.mutate()}
      onIptal={onClose}
    >
      {method.isActive && (
        // Ton `bad` → `warn`: bu bir HATA değil, bir DİKKAT uyarısıdır (§5.4).
        // Özgün kod `Alert`'i tonsuz bırakmıştı ve varsayılan ton `bad`'dir.
        //
        // `duyur={false}`: kutu, onay diyaloğu AÇILIRKEN zaten oradadır —
        // koşulu (`method.isActive`) bir eylem değil, kaydın hâli. Diyaloğun
        // kendi açılış duyurusu ("Yöntemi sil", diyalog) bağlamı veriyor;
        // `role="alert"` onu kesip yerine geçerdi (§7.4).
        <Alert tone="warn" duyur={false}>
          Bu yöntem şu anda <strong>aktif</strong>. Silindiği anda kullanıcılar
          bu yolla yükleme yapamaz. Geçici olarak durdurmak istiyorsanız silmek
          yerine <strong>pasifleştirin</strong>.
        </Alert>
      )}

      <p className="text-sm text-muted">
        Geçmiş yükleme talepleri etkilenmez: her talep, oluşturulduğu andaki
        yöntem adını kendi içinde saklar. Kullanıcı bir yıl sonra baktığında
        hangi yolla yatırdığını görmeye devam eder.
      </p>
    </OnayDiyalogu>
  );
}
