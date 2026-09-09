'use client';

/**
 * Bakiye talepleri (FR-704).
 *
 * Bu ekran GERÇEK PARA yazar: "Onayla" düğmesi kullanıcının bakiyesini
 * artırır ve kayıt defterine değiştirilemez bir satır yazar. Bu yüzden hem
 * onay hem red iki adımlıdır ve ikinci adım yazılacak tutarı/gerekçeyi
 * AÇIKÇA tekrar gösterir (CLAUDE.md değişmez #4, ekran kuralı #9).
 *
 * ══════════════════════════════════════════════════════════════════════════
 * 🔴 DEĞİŞMEYECEK İKİ ŞEY — bu dalganın en kritik kısıtı
 * ══════════════════════════════════════════════════════════════════════════
 * 1. ONAY ADIMLARINDA İLK ODAK "VAZGEÇ"TEDİR. Buraya klavyeyle gelinir:
 *    kullanıcı bir önceki adımda Enter ile "Devam et"e basar. Enter BASILI
 *    KALIRSA `click` olayı keydown tekrarıyla yeniden üretilir; odak "Evet"te
 *    olsaydı basılı kalan TEK BİR TUŞ parayı yazardı. Bu azınlık bir
 *    davranıştır ve bilerek seçilmiştir (§7.5).
 * 2. `toMinor` POLİTİKASI: bu ekran `TUTAR_ZORUNLU` kullanır — negatif RET,
 *    boş HATA. `bakiye` ekranının `izinNegatif: true` ayarıyla karıştırılmaz.
 *
 * Mobil kart + masaüstü tablo tek `VeriTablosu` sütun tanımından türer. Elle
 * yazılan iki kopya üç yerde AYRIŞMIŞTI ("İncele" ↔ "İncele ve karar ver",
 * "Tutar" ↔ "Bildirilen tutar", "—" ↔ "Yok"); artık yapısal olarak imkânsız.
 */

import * as React from 'react';
import { useMutation, useQuery, useQueryClient, keepPreviousData } from '@tanstack/react-query';
import { ApiError, apiBlob, apiFetch } from '@/lib/api';
import { formatMoney, formatDateTime } from '@/lib/format';
import { TUTAR_ZORUNLU, toMinor } from '@/lib/para';
import { Alert, Button, Card, Empty, Field, Spinner, cx } from '@/components/ui';
import { Modal } from '@/components/modal';
import {
  CokSatir,
  DurumRozeti,
  HataDurumu,
  KayitSayaci,
  SAYFA_BOYUTU,
  SayfaBasligi,
  Sayfalama,
  Secim,
  SuzgecCubugu,
  VeriTablosu,
  apiHatasi,
  type Sutun,
} from '@/components/yonetim';
import type { AdminDeposit, DepositStatus, Money } from '@/lib/types';

/** Yanıt zarfları — sunucu DTO'ları (dto/deposit.go). */
interface AdminDepositList {
  items: AdminDeposit[];
  total: number;
  limit: number;
  offset: number;
}
interface DepositReview {
  deposit: AdminDeposit;
  /** TALEP SAHİBİNİN yeni bakiyesi, yöneticinin değil. */
  balance: Money;
  /** true ise bu istek bir TEKRAR'dı; hiçbir şey yazılmadı. */
  alreadyApplied?: boolean;
}

/** Sunucudaki sınırlar (dto/deposit.go: depositMin/MaxAmountMinor). */
const MIN_MINOR = 1000;
const MAX_MINOR = 5_000_000;

const STATUS_FILTERS: Array<{ value: '' | DepositStatus; label: string }> = [
  // Varsayılan ilk sıradadır: yöneticinin işi BEKLEYEN taleplerdir.
  { value: 'PENDING', label: 'Onay bekleyenler' },
  { value: 'COMPLETED', label: 'Onaylananlar' },
  { value: 'REJECTED', label: 'Reddedilenler' },
  { value: 'REFUNDED', label: 'İade edilenler' },
  { value: '', label: 'Tümü' },
];

function tryMoney(minor: number): Money {
  return { minor, currency: 'TRY', formatted: '' };
}

/**
 * Modal içinde odağı `[data-autofocus]` öğesine taşır — ADIM DEĞİŞTİĞİNDE.
 *
 * `modal.tsx` açılıştaki ilk odağı zaten doğru veriyor; bu kanca onun
 * kapatmadığı ikinci sorunu kapatır: diyalog adım değiştirdiğinde tıklanan
 * düğme DOM'dan kalkar, odak `<body>`'ye düşer ve klavye kullanıcısı modalın
 * ARKASINDAKİ sayfaya Tab'lamaya başlar. `modal.tsx`'in odak etkisi
 * `[open, onClose]` bağımlılığıyla çalışır, adım değişimini GÖRMEZ.
 *
 * Bu yüzden tek adımlı diyaloglarda (`odeme-yontemleri`) bu kanca YOKTUR ve
 * ortak katmana da taşınmadı — çok adımlı diyaloğa özgüdür.
 */
function useAdimOdagi(ref: React.RefObject<HTMLElement | null>, adim: unknown) {
  React.useEffect(() => {
    // setTimeout(0): modal.tsx'in kendi odak etkisi bu render'dan SONRA
    // çalışır; ondan önce odaklarsak panel odağı geri alır.
    const t = window.setTimeout(() => {
      ref.current?.querySelector<HTMLElement>('[data-autofocus]')?.focus();
    }, 0);
    return () => window.clearTimeout(t);
  }, [ref, adim]);
}

/**
 * Diyalog adımlarının düğme sırası — `OnayDiyalogu` ile AYNI düzen: mobilde
 * alt alta (birincil üstte), `sm:` üstünde birincil sağda. DOM sırası her iki
 * kırılımda da "birincil → ikincil"dir; odak `data-autofocus` ile verilir,
 * DOM sırası değiştirilerek DEĞİL — ekran okuyucu birincili önce duymalıdır.
 *
 * `OnayDiyalogu` KULLANILAMAZ: o bileşen kendi `Modal`'ını açar; onay burada
 * ayrı bir diyalog değil, açık diyaloğun bir ADIMIdır (girilen tutar ve not
 * korunur). İç içe iki modal yerine yalnız düzen paylaşılır.
 */
function AdimDugmeleri({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex flex-col gap-2 sm:flex-row-reverse sm:justify-start">{children}</div>
  );
}

export default function AdminDepositsPage() {
  const [status, setStatus] = React.useState<'' | DepositStatus>('PENDING');
  const [offset, setOffset] = React.useState(0);
  const [selected, setSelected] = React.useState<AdminDeposit | null>(null);

  const q = useQuery({
    queryKey: ['admin-deposits', { status, limit: SAYFA_BOYUTU, offset }],
    queryFn: () => {
      const qs = new URLSearchParams({ limit: String(SAYFA_BOYUTU), offset: String(offset) });
      if (status) qs.set('status', status);
      return apiFetch<AdminDepositList>(`/admin/deposits?${qs.toString()}`);
    },
    // Sayfa/süzgeç değişince liste boşalıp zıplamasın.
    placeholderData: keepPreviousData,
  });

  const total = q.data?.total ?? 0;
  const listErr = apiHatasi(q.error);

  /*
   * `Sayfalama` GÖRÜNÜR MÜ? — tek sayfaya sığan listede o bileşen kendini hiç
   * çizmez, yani bu ifade "sayfalama ekranda var mı" ile aynı şeydir.
   *
   * Duyuru `false` yerine `!sayfali`: kapalı bırakmak, `Sayfalama`nın da
   * görünmediği kısa listede (varsayılan süzgeç "Onay bekleyenler" çoğu gün
   * 25 kaydın altındadır) ekran okuyucuyu SÜZGEÇ DEĞİŞİMİNDE tamamen sessiz
   * bırakıyordu — çift duyuruyu çözerken tek duyuruyu da kaldırmıştı.
   * `kullanicilar` ve `denetim` zaten bu ifadeyi kullanıyor; beş ekranın beşi
   * artık aynı kuralda (§9.2 "ekranlar arası tutarsız bileşen dili").
   */
  const sayfali = total > SAYFA_BOYUTU;

  /*
   * SÜTUNLAR — tek tanım, iki sunum.
   *
   * "Yöntem" ve "Dekont" `lg:` altında gizlenir (`oncelik: 3`) ve bilgi
   * KAYBOLMAZ: ikisi de mobil kartta ve inceleme diyaloğunda yerinde durur.
   * Yedi sütun 768px'te sığmıyordu; sığdırmaya çalışmak tabloyu yatay
   * kaydırmaya iterdi ki bu kabul edilmez (§6.1 kural 1).
   */
  const sutunlar: ReadonlyArray<Sutun<AdminDeposit>> = [
    {
      anahtar: 'tarih',
      baslik: 'Tarih',
      sayisal: true, // §3.5: tarih sütunu da tabular-nums taşır
      hucre: (d) => <span className="text-muted">{formatDateTime(d.createdAt)}</span>,
    },
    {
      anahtar: 'kullanici',
      baslik: 'Kullanıcı',
      mobilRol: 'baslik',
      /*
        🔴 `truncate` TEK BAŞINA TABLOYU TAŞIRIR — ölçüldü, 768px'te 806px.
        `truncate` `white-space: nowrap` demektir; `table-layout: auto` bir
        hücrenin max-content genişliğini içerikten hesapladığı için uzun bir
        e-posta sütunu ZORLA GENİŞLETİR ve kırpma hiç devreye girmez. Üst
        sınırı veren bir kap şart (özgün kod bunu `<td>`'ye yazıyordu).
        Kartta üst sınır YOKTUR: orada kap `min-w-0` bir flex öğesidir ve
        daralma zaten çalışır — bu, `sunum`un meşru kullanımıdır (genişlik),
        metin farkı değil.
      */
      hucre: (d, sunum) => (
        <div className={sunum === 'tablo' ? 'max-w-[9rem] lg:max-w-[14rem]' : 'min-w-0'}>
          <span className="block truncate font-medium">{d.userUsername}</span>
          {/* `text-sm`, `text-xs` DEĞİL (§3.2): e-posta bir veri alanıdır ve
              destek yazışmasında okunur. */}
          <span className="block truncate text-sm text-muted">{d.userEmail}</span>
        </div>
      ),
    },
    {
      anahtar: 'yontem',
      baslik: 'Yöntem',
      oncelik: 3,
      hucre: (d, sunum) => (
        <span className={cx('block', sunum === 'tablo' ? 'max-w-[10rem] truncate' : 'break-anywhere')}>
          {d.method}
        </span>
      ),
    },
    {
      /*
        BAŞLIK "Bildirilen tutar" — "Tutar" DEĞİL.
        Özgün kod mobilde "Bildirilen tutar", masaüstünde "Tutar" yazıyordu.
        Bir para ekranında bu ayrım önemlidir: bu sütun kullanıcının BİLDİRDİĞİ
        tutardır, bakiyeye yazılan değil. İnceleme diyaloğu da aynı sözcüğü
        kullanır — ekran içinde tek sözlük.
      */
      anahtar: 'tutar',
      baslik: 'Bildirilen tutar',
      hizala: 'sag',
      sayisal: true,
      hucre: (d) => (
        <>
          <span className="font-semibold">{formatMoney(d.amount)}</span>
          {/* "yazılan" YALNIZ FARKLIYSA çıkar. Özgün mobil kart onu her
              COMPLETED satırda gösteriyordu — aynı sayıyı iki kez yazmak fark
              varmış izlenimi verir. Diyalogdaki "Yazılan tutar" hep oradadır. */}
          {d.status === 'COMPLETED' && d.credited.minor !== d.amount.minor && (
            <span className="block text-sm font-normal text-[var(--color-ok)]">
              yazılan: {formatMoney(d.credited)}
            </span>
          )}
        </>
      ),
    },
    {
      anahtar: 'dekont',
      baslik: 'Dekont',
      oncelik: 3,
      // Tek metin: özgün kod mobilde "Yok", masaüstünde "—" yazıyordu.
      hucre: (d) => (d.hasReceipt ? 'Var' : <span className="text-muted">Yok</span>),
    },
    {
      anahtar: 'durum',
      baslik: 'Durum',
      mobilRol: 'rozet',
      // Ton `durumTonu()` ile TEK haritadan gelir; etiket sunucunun
      // `statusLabel`'ıdır — istemcide ikinci bir Türkçe sözlük kurulmaz.
      hucre: (d) => <DurumRozeti durum={d.status} etiket={d.statusLabel} />,
    },
    {
      anahtar: 'islem',
      baslik: 'İşlem',
      basligiGizle: true,
      hizala: 'sag',
      mobilRol: 'eylem',
      // Metin `sunum`dan TÜRETİLMEZ (§6): mobilde "İncele ve karar ver",
      // masaüstünde "İncele" yazan özgün ayrışma tam olarak buydu. Fark
      // yalnız genişliktedir.
      hucre: (d, sunum) => (
        <Button
          variant="outline"
          size="sm"
          fullWidth={sunum === 'kart'}
          onClick={() => setSelected(d)}
        >
          {d.status === 'PENDING' ? 'İncele' : 'Ayrıntılar'}
        </Button>
      ),
    },
  ];

  const suzgecli = status !== 'PENDING';

  return (
    <div className="mx-auto flex max-w-5xl flex-col gap-6">
      <SayfaBasligi
        baslik="Bakiye talepleri"
        aciklama="Kullanıcıların bildirdiği ödemeleri inceleyip onaylayın veya reddedin."
      />

      <Card>
        <SuzgecCubugu sag={<KayitSayaci toplam={total} />}>
          {/* `onTemizle` VERİLMEDİ: tek bir açılır listede varsayılana dönüş
              zaten listenin kendisidir. Süzgeçten boşalan sonuçta "Tümünü
              göster" eylemi ayrıca sunulur (§6.3). */}
          <Secim
            etiket="Durum"
            className="sm:w-64"
            value={status}
            onChange={(e) => {
              setStatus(e.target.value as '' | DepositStatus);
              setOffset(0); // süzgeç değişti; eski sayfa numarası anlamsız
            }}
          >
            {STATUS_FILTERS.map((f) => (
              <option key={f.value || 'all'} value={f.value}>{f.label}</option>
            ))}
          </Secim>
        </SuzgecCubugu>

        <VeriTablosu
          className="mt-6"
          baslik="Bakiye talepleri listesi"
          sutunlar={sutunlar}
          satirlar={q.data?.items}
          satirAnahtari={(d) => d.id}
          yukleniyor={q.isLoading}
          hata={listErr}
          // `Sayfalama` de duyuruyor; sayfalı listede tek duyuru kalsın
          // (yukarıdaki `sayfali` notu). Sayfalı liste ayrıca `VeriTablosu`nun
          // "N kayıt listelendi" metninin YANLIŞ olduğu tek durumdur: N sayfadaki
          // satır sayısıdır, toplam değil.
          duyuru={!sayfali}
          bos={
            /* §6.3: süzgeçten boş ≠ gerçekten boş. Varsayılan süzgeç "Onay
               bekleyenler" olduğu için ikinci hâl İYİ HABERDİR; eylem sunulmaz. */
            suzgecli ? (
              <div className="flex flex-col items-center gap-4">
                <Empty
                  title="Sonuç yok"
                  hint="Seçtiğiniz duruma uyan bir talep bulunmuyor. Süzgeci genişletip tüm talepleri görebilirsiniz."
                />
                <Button
                  variant="outline"
                  onClick={() => { setStatus(''); setOffset(0); }}
                >
                  Tümünü göster
                </Button>
              </div>
            ) : (
              <Empty
                title="Bekleyen talep yok"
                hint="Onay bekleyen bakiye talebi bulunmuyor. Kullanıcı bir ödeme bildirdiğinde talep burada belirir."
              />
            )
          }
        />

        <Sayfalama
          className="mt-6"
          offset={offset}
          limit={SAYFA_BOYUTU}
          toplam={total}
          onDegis={setOffset}
        />
      </Card>

      {selected && (
        <ReviewDialog
          key={selected.id}
          deposit={selected}
          onClose={() => setSelected(null)}
        />
      )}
    </div>
  );
}

/* ═══════════════════════ İnceleme diyaloğu ═══════════════════════ */

type Step = 'detail' | 'approve' | 'approveConfirm' | 'reject' | 'rejectConfirm' | 'done';

function ReviewDialog({ deposit, onClose }: { deposit: AdminDeposit; onClose: () => void }) {
  const qc = useQueryClient();
  const [step, setStep] = React.useState<Step>('detail');
  const [credited, setCredited] = React.useState('');
  const [adminNote, setAdminNote] = React.useState('');
  const [reason, setReason] = React.useState('');
  const [formErrors, setFormErrors] = React.useState<Record<string, string>>({});

  const stepRef = React.useRef<HTMLDivElement>(null);
  useAdimOdagi(stepRef, step);

  const pending = deposit.status === 'PENDING';

  const approve = useMutation({
    mutationFn: (v: { creditedMinor: number; adminNote: string }) =>
      apiFetch<DepositReview>(`/admin/deposits/${encodeURIComponent(deposit.id)}/approve`, {
        method: 'POST',
        body: { creditedMinor: v.creditedMinor, adminNote: v.adminNote },
      }),
    // Belge amaçlı: para yazan bir çağrı ASLA otomatik tekrarlanmaz.
    // (İdempotency anahtarı sunucuda talebin public_id'sinden türer.)
    retry: false,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['admin-deposits'] });
      setStep('done');
    },
  });

  const reject = useMutation({
    mutationFn: (v: { reason: string; adminNote: string }) =>
      apiFetch<DepositReview>(`/admin/deposits/${encodeURIComponent(deposit.id)}/reject`, {
        method: 'POST',
        body: { reason: v.reason, adminNote: v.adminNote },
      }),
    retry: false,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['admin-deposits'] });
      setStep('done');
    },
  });

  /**
   * Yazılacak tutar.
   *
   * Alan BOŞSA sunucu `creditedMinor: 0` görür ve kullanıcının BİLDİRDİĞİ
   * tutarı yazar. Yönetici farklı bir tutar yazabilir çünkü kriptoda ağ
   * ücreti düşer: kullanıcı 500 ₺ gönderir, hesaba 493,40 ₺ düşer.
   *
   * 🔴 BOŞLUK KONTROLÜ BURADA YAPILIR, `toMinor`'da DEĞİL: bu ekranın ayarı
   * `TUTAR_ZORUNLU`'dur ve boş girdiyi HATA sayar. "Boş = bildirilen tutar"
   * kuralı bu alana özgüdür ve `toMinor`'a taşınırsa üçüncü bir para
   * politikası doğar (`lib/para.ts` dosya başı).
   */
  const parsed = credited.trim() === '' ? null : toMinor(credited, TUTAR_ZORUNLU);
  const creditedMinor = parsed && 'minor' in parsed ? parsed.minor : 0;
  const effective: Money = creditedMinor === 0 ? deposit.amount : tryMoney(creditedMinor);
  const differs = creditedMinor !== 0 && creditedMinor !== deposit.amount.minor;

  function submitApproveForm(e: React.FormEvent) {
    e.preventDefault();
    const errs: Record<string, string> = {};
    if (parsed && 'error' in parsed) {
      errs.credited = parsed.error;
    } else if (creditedMinor !== 0 && (creditedMinor < MIN_MINOR || creditedMinor > MAX_MINOR)) {
      errs.credited = 'Yatan tutar 10,00 ₺ ile 50.000,00 ₺ arasında olmalıdır.';
    }
    if (adminNote.trim().length > 300) errs.adminNote = 'Not en fazla 300 karakter olabilir.';
    setFormErrors(errs);
    if (Object.keys(errs).length) return;
    setStep('approveConfirm');
  }

  function submitRejectForm(e: React.FormEvent) {
    e.preventDefault();
    const errs: Record<string, string> = {};
    const r = reason.trim();
    if (r.length < 5) {
      errs.reason = 'Red nedeni zorunludur (en az 5 karakter) — kullanıcıya gösterilir.';
    } else if (r.length > 300) {
      errs.reason = 'Red nedeni en fazla 300 karakter olabilir.';
    }
    if (adminNote.trim().length > 300) errs.adminNote = 'Not en fazla 300 karakter olabilir.';
    setFormErrors(errs);
    if (Object.keys(errs).length) return;
    setStep('rejectConfirm');
  }

  const mutErr = apiHatasi(approve.error) ?? apiHatasi(reject.error);
  const result = approve.data ?? reject.data ?? null;

  const titles: Record<Step, string> = {
    detail: 'Talep ayrıntısı',
    approve: 'Talebi onayla',
    approveConfirm: 'Onayı doğrulayın',
    reject: 'Talebi reddet',
    rejectConfirm: 'Reddi doğrulayın',
    done: 'Sonuç',
  };

  return (
    <Modal open onClose={onClose} title={titles[step]}>
      {/*
        ══════════════════════════════════════════════════════════════════════
        🔴 ADIM KUTULARI `role="alert"` TAŞIMAYA DEVAM EDER — bilinçli karar
        ══════════════════════════════════════════════════════════════════════
        Bir denetim raporu bu kutuları "açılışta koşulsuz çizilen statik uyarı"
        saydı; ölçtüm, DEĞİLLER. Diyaloğun kendisi koşullu monte ediliyor
        (`{selected && <ReviewDialog/>}`) ve adım kutularının tamamı, diyalog
        ZATEN AÇIKKEN basılan bir düğmeyle ("Onayla" / "Reddet" / "Devam et")
        ekrana giriyor. Yani §7.4'ün tarifi birebir karşılanıyor: eylemin
        sonucunda beliren yeni içerik.

        Ve burada susmanın bedeli ölçülebilir: adım değişiminde ekran okuyucuya
        HİÇBİR ŞEY söylenmez.
         · `Modal`ın başlığı (`titles[step]`) adım başına değişiyor ama AÇIK bir
           diyaloğun erişilebilir adının değişmesi duyurulmaz.
         · `useAdimOdagi` odağı yeni adımın `[data-autofocus]` öğesine taşır;
           onay adımlarında bu öğe "Vazgeç" düğmesidir (§7.5) — etiketi
           neyin onaylandığı hakkında tek kelime söylemez.
        Kutular susturulursa ekran okuyucu kullanıcısı "Devam et"e bastıktan
        sonra yalnız "Vazgeç, düğme" duyar ve KİME NE KADAR para yazılacağını
        bilmeden onaylar. Bu bir para sistemidir; kesinti burada doğru davranış.

        Statik olan TEK kutu `ReceiptViewer`ın "dekont yok" bilgisidir — o
        diyaloğun ilk çiziminde durduğu için `duyur={false}` aldı.
      */}
      <div ref={stepRef}>
        {step === 'detail' && (
          <div className="flex flex-col gap-5">
            <DepositFacts deposit={deposit} />
            <ReceiptViewer depositId={deposit.id} hasReceipt={deposit.hasReceipt} />

            {pending ? (
              <div className="flex flex-col gap-2">
                <Button
                  data-autofocus
                  fullWidth
                  onClick={() => { setFormErrors({}); setStep('approve'); }}
                >
                  Onayla
                </Button>
                <Button
                  variant="danger" fullWidth
                  onClick={() => { setFormErrors({}); setStep('reject'); }}
                >
                  Reddet
                </Button>
                <Button variant="ghost" fullWidth onClick={onClose}>Kapat</Button>
              </div>
            ) : (
              <Button data-autofocus variant="outline" fullWidth onClick={onClose}>Kapat</Button>
            )}
          </div>
        )}

        {step === 'approve' && (
          <form onSubmit={submitApproveForm} className="flex flex-col gap-5" noValidate>
            <Alert tone="warn">
              Onay, kullanıcının bakiyesini <strong>gerçekten</strong> artırır ve kayıt
              defterine geri alınamaz bir satır yazar. Ödemenin hesabınıza geçtiğini
              doğrulamadan onaylamayın.
            </Alert>

            {/* İki tutarın yan yana okunduğu tek yer — `tabular-nums` (§3.5)
                olmadan basamaklar hizalanmaz ve fark gözden kaçar. */}
            <dl className="raised flex flex-col gap-3 rounded-xl border p-4 text-sm">
              <div className="flex items-center justify-between gap-4">
                <dt className="text-muted">Kullanıcının bildirdiği</dt>
                <dd className="font-semibold tabular-nums">{formatMoney(deposit.amount)}</dd>
              </div>
              <div className="flex items-center justify-between gap-4">
                <dt className="text-muted">Bakiyeye yazılacak</dt>
                <dd className="font-semibold tabular-nums text-[var(--color-ok)]">
                  {formatMoney(effective)}
                </dd>
              </div>
            </dl>

            <Field
              data-autofocus
              label="Yazılacak tutar (TL)"
              value={credited}
              onChange={(e) => setCredited(e.target.value)}
              placeholder={`Boş bırakılırsa ${formatMoney(deposit.amount)}`}
              inputMode="decimal" autoComplete="off"
              error={formErrors.credited}
              hint="Boş bırakırsanız kullanıcının bildirdiği tutar yazılır. Kripto ağ
                    ücreti veya eksik havale nedeniyle hesaba GEÇEN tutar farklıysa,
                    gerçekten geçen tutarı buraya yazın."
            />

            {differs && (
              <Alert tone="warn">
                Bildirilen tutar ile yazılacak tutar <strong>farklı</strong>. Kullanıcı
                ekstresinde yazılan tutarı görecek; farkın nedenini aşağıdaki nota
                yazmanız ileride yapılacak incelemeyi kolaylaştırır.
              </Alert>
            )}

            <CokSatir
              etiket="Yönetici notu (isteğe bağlı)"
              rows={3} maxLength={300}
              value={adminNote} onChange={(e) => setAdminNote(e.target.value)}
              placeholder="Örn. 12.09 tarihli havale, dekont doğrulandı."
              ipucu="Yalnız yöneticiler görür; kullanıcıya gösterilmez."
              hata={formErrors.adminNote}
            />

            <AdimDugmeleri>
              <Button type="submit" fullWidth className="sm:w-auto">Devam et</Button>
              <Button
                type="button" variant="outline" fullWidth className="sm:w-auto"
                onClick={() => setStep('detail')}
              >
                Geri
              </Button>
            </AdimDugmeleri>
          </form>
        )}

        {step === 'approveConfirm' && (
          <div className="flex flex-col gap-5">
            <Alert tone="warn">
              <p>
                <strong>{deposit.userUsername}</strong> adlı kullanıcının bakiyesine
                {' '}<strong>{formatMoney(effective)}</strong> yazılacak.
              </p>
              <p className="mt-2">Bu işlem geri alınamaz.</p>
            </Alert>

            {differs && (
              <p className="text-sm text-muted">
                Kullanıcı {formatMoney(deposit.amount)} bildirmişti; siz
                {' '}{formatMoney(effective)} onaylıyorsunuz.
              </p>
            )}

            {mutErr && <HataDurumu hata={mutErr} />}

            <AdimDugmeleri>
              <Button
                fullWidth className="sm:w-auto" loading={approve.isPending}
                onClick={() => approve.mutate({ creditedMinor, adminNote: adminNote.trim() })}
              >
                Evet, bakiyeye yaz
              </Button>
              {/*
                🔴 ODAK "VAZGEÇ"TEDİR, "Evet"te DEĞİL — dosya başındaki
                gerekçeye bakınız. Bu özniteliği onay düğmesine taşımak,
                basılı kalan tek bir tuşun para yazmasına yol açar.
              */}
              <Button
                data-autofocus variant="outline" fullWidth className="sm:w-auto"
                disabled={approve.isPending}
                onClick={() => setStep('approve')}
              >
                Vazgeç
              </Button>
            </AdimDugmeleri>
          </div>
        )}

        {step === 'reject' && (
          <form onSubmit={submitRejectForm} className="flex flex-col gap-5" noValidate>
            <Alert tone="info">
              Red bakiyeyi değiştirmez. Yazdığınız neden <strong>kullanıcıya aynen
              gösterilir</strong>; anlaşılır ve nazik bir cümle yazın.
            </Alert>

            <CokSatir
              data-autofocus
              etiket="Red nedeni (kullanıcı görür)"
              rows={3} maxLength={300}
              value={reason} onChange={(e) => setReason(e.target.value)}
              placeholder="Örn. Bildirilen tutarda bir ödeme hesabımıza ulaşmadı."
              ipucu="En az 5, en fazla 300 karakter."
              hata={formErrors.reason}
            />

            <CokSatir
              etiket="Yönetici notu (isteğe bağlı)"
              rows={2} maxLength={300}
              value={adminNote} onChange={(e) => setAdminNote(e.target.value)}
              placeholder="Yalnız yöneticilerin göreceği iç not."
              hata={formErrors.adminNote}
            />

            <AdimDugmeleri>
              <Button type="submit" variant="danger" fullWidth className="sm:w-auto">
                Devam et
              </Button>
              <Button
                type="button" variant="outline" fullWidth className="sm:w-auto"
                onClick={() => setStep('detail')}
              >
                Geri
              </Button>
            </AdimDugmeleri>
          </form>
        )}

        {step === 'rejectConfirm' && (
          <div className="flex flex-col gap-5">
            <Alert tone="warn">
              <p>
                <strong>{deposit.userUsername}</strong> adlı kullanıcının
                {' '}{formatMoney(deposit.amount)} tutarındaki talebi reddedilecek.
              </p>
            </Alert>

            <div className="raised rounded-xl border p-4">
              {/* `uppercase` kaldırıldı: CSS büyük harf dönüşümü Türkçede
                  `i → I` üretir (`İ` değil). `text-xs` de (§3.2). */}
              <p className="text-sm font-medium text-muted">Kullanıcının göreceği metin</p>
              <p className="mt-2 break-anywhere text-sm">{reason.trim()}</p>
            </div>

            {mutErr && <HataDurumu hata={mutErr} />}

            <AdimDugmeleri>
              <Button
                variant="danger" fullWidth className="sm:w-auto" loading={reject.isPending}
                onClick={() => reject.mutate({ reason: reason.trim(), adminNote: adminNote.trim() })}
              >
                Evet, reddet
              </Button>
              {/* Odak "Vazgeç"te — gerekçe için onay adımına bakınız. */}
              <Button
                data-autofocus variant="outline" fullWidth className="sm:w-auto"
                disabled={reject.isPending}
                onClick={() => setStep('reject')}
              >
                Vazgeç
              </Button>
            </AdimDugmeleri>
          </div>
        )}

        {step === 'done' && result && (
          <div className="flex flex-col gap-5">
            {/* TEKRARLANAN ONAY HATA DEĞİLDİR: sunucu 200 + alreadyApplied
                döner. Kırmızı kutu yöneticiye "tekrar dene" dedirtir — oysa
                iş çoktan yapılmıştır. */}
            <Alert tone={result.alreadyApplied ? 'info' : 'ok'}>
              {result.alreadyApplied
                ? 'Bu talep daha önce sonuçlandırılmıştı; hiçbir şey yeniden yazılmadı.'
                : `Talep ${result.deposit.statusLabel.toLocaleLowerCase('tr-TR')}.`}
            </Alert>

            <dl className="raised flex flex-col gap-3 rounded-xl border p-4 text-sm">
              <div className="flex items-center justify-between gap-4">
                <dt className="text-muted">Durum</dt>
                <dd>
                  <DurumRozeti durum={result.deposit.status} etiket={result.deposit.statusLabel} />
                </dd>
              </div>
              <div className="flex items-center justify-between gap-4">
                <dt className="text-muted">Bakiyeye yazılan</dt>
                <dd className="font-semibold tabular-nums">{formatMoney(result.deposit.credited)}</dd>
              </div>
              <div className="flex items-center justify-between gap-4">
                <dt className="text-muted">Kullanıcının yeni bakiyesi</dt>
                <dd className="font-semibold tabular-nums">{formatMoney(result.balance)}</dd>
              </div>
            </dl>

            <Button data-autofocus fullWidth onClick={onClose}>Kapat</Button>
          </div>
        )}
      </div>
    </Modal>
  );
}

/* ═══════════════════════ Talep bilgileri ═══════════════════════ */

function Satir({
  etiket, deger, sayisal,
}: { etiket: string; deger: React.ReactNode; sayisal?: boolean }) {
  return (
    <div className="flex items-start justify-between gap-4 py-3">
      <dt className="shrink-0 text-muted">{etiket}</dt>
      <dd className={cx('min-w-0 break-anywhere text-right font-medium', sayisal && 'tabular-nums')}>
        {deger}
      </dd>
    </div>
  );
}

function DepositFacts({ deposit: d }: { deposit: AdminDeposit }) {
  return (
    <dl className="raised divide-y divide-[var(--border)] rounded-xl border px-4 text-sm">
      <Satir
        etiket="Kullanıcı"
        deger={
          <>
            {d.userUsername}
            <span className="block font-normal text-muted">{d.userEmail}</span>
          </>
        }
      />
      <Satir etiket="Durum" deger={<DurumRozeti durum={d.status} etiket={d.statusLabel} />} />
      <Satir etiket="Yöntem" deger={d.method} />
      <Satir etiket="Bildirilen tutar" deger={formatMoney(d.amount)} sayisal />
      {d.status !== 'PENDING' && (
        <Satir etiket="Yazılan tutar" deger={formatMoney(d.credited)} sayisal />
      )}
      {d.network && <Satir etiket="Ağ" deger={d.network} />}
      {d.txHash && (
        // Monospace MEŞRU (§9.2): işlem numarası karakter karakter okunur.
        <Satir etiket="İşlem numarası" deger={<code className="font-mono">{d.txHash}</code>} />
      )}
      {d.userNote && <Satir etiket="Kullanıcı notu" deger={d.userNote} />}
      {d.adminNote && <Satir etiket="Yönetici notu" deger={d.adminNote} />}
      {d.rejectionReason && <Satir etiket="Red nedeni" deger={d.rejectionReason} />}
      <Satir etiket="Oluşturuldu" deger={formatDateTime(d.createdAt)} sayisal />
      {d.reviewedAt && <Satir etiket="İncelendi" deger={formatDateTime(d.reviewedAt)} sayisal />}
    </dl>
  );
}

/* ═══════════════════════ Dekont ═══════════════════════ */

/**
 * Dekont görüntüleyici.
 *
 * Uç `Content-Disposition: attachment` ile döner: doğrudan bir <img src>
 * veya <a href> tarayıcıyı indirmeye zorlar. Dosyayı blob olarak alıp object
 * URL üretiriz — ve BIRAKMAYI unutmayız: her açılışta yeni bir blob bellekte
 * kalırsa, on talebi inceleyen bir yönetici on dosyayı taşır.
 *
 * 🔴 BU BLOK BU DALGADA DEĞİŞTİRİLMEDİ (yalnız `ErrorBox` → `HataDurumu`).
 * Sızıntı ve iptal mantığı ölçümle kurulmuştur; sunum düzeltmesi uğruna
 * dokunulmaz.
 */
function ReceiptViewer({ depositId, hasReceipt }: { depositId: string; hasReceipt: boolean }) {
  const [file, setFile] = React.useState<{ url: string; mime: string } | null>(null);
  const [loading, setLoading] = React.useState(false);
  const [err, setErr] = React.useState<ApiError | null>(null);

  // Blob'u serbest bırak: bileşen ayrıldığında ve dosya değiştiğinde.
  React.useEffect(() => {
    if (!file) return;
    return () => URL.revokeObjectURL(file.url);
  }, [file]);

  /*
   * 🔴 YÜKLEME SÜRERKEN DİYALOG KAPANIRSA: yukarıdaki temizlik `file` state'i
   * hiç dolmadığı için ÇALIŞMAZ ve blob sekme kapanana kadar bellekte kalır.
   * Bu bayrak isteğin sonucunu "artık kimse beklemiyor" diye işaretler;
   * gelen blob o zaman state'e yazılmadan doğrudan serbest bırakılır.
   * Ayrıca istek de iptal edilir — kimsenin okumayacağı bir indirmeyi
   * sürdürmenin anlamı yok.
   */
  const canli = React.useRef(true);
  const iptal = React.useRef<AbortController | null>(null);
  React.useEffect(() => {
    canli.current = true;
    return () => {
      canli.current = false;
      iptal.current?.abort();
    };
  }, []);

  async function load() {
    setLoading(true);
    setErr(null);
    const ac = new AbortController();
    iptal.current = ac;
    try {
      const r = await apiBlob(`/admin/deposits/${encodeURIComponent(depositId)}/receipt`, {
        timeoutMs: 30_000, // dekont birkaç MB olabilir; 15 sn mobil ağda dar
        signal: ac.signal,
      });
      if (!canli.current) {
        URL.revokeObjectURL(r.url); // kimse beklemiyor — sızdırma
        return;
      }
      setFile({ url: r.url, mime: r.mime });
    } catch (e) {
      if (!canli.current) return;
      setErr(e instanceof ApiError ? e : new ApiError({ code: 'UNKNOWN', message: 'Dekont açılamadı.' }));
    } finally {
      if (canli.current) setLoading(false);
    }
  }

  if (!hasReceipt) {
    return (
      // `duyur={false}`: bu kutu inceleme diyaloğunun İLK adımında, diyalog
      // açılırken çizilir — koşulu bir eylem değil, talebin hâli. Diyaloğun
      // kendi açılış duyurusunu kesmemeli (§7.4).
      <Alert tone="info" duyur={false}>
        Bu talepte dekont yok. Havale taleplerinde dekont beklenir; onaylamadan
        önce ödemeyi hesap hareketlerinizden doğrulayın.
      </Alert>
    );
  }

  const isImage = file?.mime.startsWith('image/');

  return (
    <div className="flex flex-col gap-3">
      {!file && (
        <Button variant="outline" fullWidth onClick={load} disabled={loading}>
          {loading ? <><Spinner /> Dekont açılıyor…</> : 'Dekontu göster'}
        </Button>
      )}

      {err && <HataDurumu hata={err} />}

      {file && (
        <div className="flex flex-col gap-3">
          {isImage ? (
            /* next/image KULLANILMAZ: kaynak bir `blob:` URL'idir, optimize
               edici uzak/yerel bir yol bekler ve bunu işleyemez. */
            // eslint-disable-next-line @next/next/no-img-element
            <img
              src={file.url} alt="Dekont"
              className="max-h-[50dvh] w-full rounded-xl border border-[var(--border)] object-contain"
            />
          ) : (
            <object
              data={file.url} type={file.mime || 'application/pdf'}
              className="h-[50dvh] w-full rounded-xl border border-[var(--border)]"
              aria-label="Dekont"
            >
              <p className="p-4 text-sm text-muted">
                Bu dosya tarayıcıda gösterilemiyor.
              </p>
            </object>
          )}
          <a
            href={file.url} download="dekont"
            className="inline-flex min-h-11 items-center justify-center rounded-xl border
                       border-[var(--border)] px-4 text-sm font-medium
                       hover:bg-[var(--raised)]
                       [transition-property:background-color]
                       [transition-duration:var(--sure-hizli)]"
          >
            Dekontu indir
          </a>
        </div>
      )}
    </div>
  );
}
