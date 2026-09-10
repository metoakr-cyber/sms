'use client';

/**
 * Destek talepleri — MÜŞTERİ ekranı (FR-600).
 *
 * Bu ekranda GÖSTERİLEN METNİN TAMAMI KULLANICI GİRDİSİDİR (kendi mesajları ve
 * personelin yanıtları). React metni varsayılan olarak kaçırır;
 * `dangerouslySetInnerHTML` bu dosyada YOKTUR ve olmayacaktır. Kullanıcı
 * mesajları düz metin olarak, `whitespace-pre-wrap` ile satır sonları
 * korunarak basılır — biçimlendirme HTML'e değil CSS'e bırakılır.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * BU DOSYA ORTAK KATMANA BAĞLANDI (`components/yonetim` · §5.3)
 * ══════════════════════════════════════════════════════════════════════════
 * Paketin adı "yonetim" ama bileşenleri geneldir; `/panel` de aynı katmanı
 * kullanır. Yeniden adlandırma ayrı bir iştir (139 import).
 *
 * Silinen yerel kopyalar — hepsi başka dosyalarda da duran BİREBİR kopyaydı:
 *   · `ErrorBox`        → `HataDurumu`   (11 kopyanın biriydi; `requestId`
 *                          artık 14px ve tam opaklıkta, telefonda okunup
 *                          destek ekibine yazılan metin budur)
 *   · `statusTone`      → `DurumRozeti`  (6 kopyanın biri)
 *   · `selectClass` + `Chevron` → `Secim` (`.select-ok`, WebKit 25px tuzağı)
 *   · `textareaClass`   → `CokSatir`     (+ `caret-color`, `.thin-scroll`)
 *   · elle sayfalama    → `Sayfalama`    (yerel `PAGE = 20` sabiti silindi;
 *                          panelde TEK sayfa boyutu var: `SAYFA_BOYUTU`)
 *   · başlık bloğu      → `SayfaBasligi`
 *   · 🔴 mobil kart listesi ↔ masaüstü tablo İKİZİ → `VeriTablosu`
 *
 * ══════════════════════════════════════════════════════════════════════════
 * 🔴 İKİZ KODUN KAPATTIĞI GERÇEK HATA
 * ══════════════════════════════════════════════════════════════════════════
 * Eski dosyada aynı satır iki kez yazılıydı ve ETİKETLERİ ZATEN AYRIŞMIŞTI:
 * eylem düğmesi mobil kartta "Yazışmayı aç", masaüstü tabloda "Aç" diyordu —
 * aynı düğme, iki isim. `/yonetim/destek` bu ayrışmayı "Yazışmayı aç" lehine
 * kapattı; burada da AYNI metin kullanılır, yani iki yüzey artık aynı dili
 * konuşuyor. `VeriTablosu`da `baslik` tek bir dizedir ve hem `<th>` hem kart
 * `<dt>` olarak oradan okunur — ayrışma yapısal olarak imkânsız.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * DAVRANIŞ DEĞİŞMEDİ — iki bilinçli sunum farkı dışında
 * ══════════════════════════════════════════════════════════════════════════
 *  1. `USER_REPLIED` rozeti `brand` → `warn`. `brand` tonu projede beş ayrı
 *     anlam taşıyordu (§5.4) ve durum için kullanılmaz; "kullanıcı yanıtladı"
 *     bir DİKKAT durumudur. Ayrımı metin yapar ("Açık" ≠ "Kullanıcı
 *     yanıtladı"), renk değil. Karar katmanda verilmiş, burada tekrar
 *     tartışılmaz.
 *  2. Sayfa boyutu 20 → 25 (`SAYFA_BOYUTU`). Aynı panelde iki farklı sayfa
 *     boyutu, "kaç kayıt kaldı" sorusunun ekrandan ekrana farklı
 *     cevaplanmasıydı.
 *
 * HAREKET: yok. Liste ve sayfalama günde tekrar tekrar görülür (§4.2). Tek
 * geçişler `Button`'un `:active` basma geri bildirimi ve tablo satırının
 * hover RENGİdir — ikisi de katmandan gelir, burada yeniden tanımlanmaz.
 */

import * as React from 'react';
import { useMutation, useQuery, useQueryClient, keepPreviousData } from '@tanstack/react-query';
import { apiFetch } from '@/lib/api';
import { formatDateTime } from '@/lib/format';
import { Card, Button, Field, Alert, Badge, Skeleton, Empty } from '@/components/ui';
import { Modal } from '@/components/modal';
import {
  SayfaBasligi,
  KayitSayaci,
  Secim,
  CokSatir,
  VeriTablosu,
  Sayfalama,
  SAYFA_BOYUTU,
  SuzgecCubugu,
  DurumRozeti,
  HataDurumu,
  apiHatasi,
  sayfalamaGorunur,
} from '@/components/yonetim';
import type { Sutun } from '@/components/yonetim';

/**
 * Durum süzgeci seçenekleri — `GET /tickets?status=`.
 *
 * 🔴 SATIR ETİKETLERİ SUNUCUDAN gelir (`statusLabel`); bu liste onları ÜRETMEZ,
 * yalnız "hangi durumlar süzülebilir" sorusunu cevaplar.
 * Kaynak: `api/internal/transport/http/handler/ticket.go#ticketStatusLabel`.
 */
const DURUM_SUZGECLERI: ReadonlyArray<{ value: string; label: string }> = [
  { value: '', label: 'Tüm durumlar' },
  { value: 'OPEN', label: 'Açık' },
  { value: 'ANSWERED', label: 'Yanıtlandı' },
  { value: 'USER_REPLIED', label: 'Yanıtınız iletildi' },
  { value: 'CLOSED', label: 'Kapatıldı' },
];

/* ═══════════════════════ Sunucu sözleşmesi ═══════════════════════ */
/*
 * Tipler BU DOSYADA tanımlıdır, `lib/types.ts` içinde değil: o dosya bu turda
 * paylaşılan bir dosyadır ve başka ajanlar da düzenliyor. Talepler kalıcı hâle
 * geldiğinde buradaki dört tip `lib/types.ts`'e taşınmalıdır (rapora yazıldı).
 * Karşılıkları: api/internal/transport/http/dto/ticket.go
 */

type TicketStatus = 'OPEN' | 'ANSWERED' | 'USER_REPLIED' | 'CLOSED';
type TicketPriority = 'LOW' | 'NORMAL' | 'HIGH';

interface TicketMessage {
  id: string;
  body: string;
  /** true ise mesajı destek ekibi yazdı (KK-600). */
  isStaff: boolean;
  authorLabel: string;
  createdAt: string;
}

interface Ticket {
  id: string;
  subject: string;
  priority: TicketPriority;
  priorityLabel: string;
  status: TicketStatus;
  statusLabel: string;
  messageCount: number;
  createdAt: string;
  lastReplyAt: string;
  closedAt?: string;
  messages?: TicketMessage[];
}

interface TicketList {
  items: Ticket[];
  total: number;
  limit: number;
  offset: number;
}

/* ═══════════════════════ Sunucu sınırları ═══════════════════════ */
/* domain/ticket/ticket.go — istemci doğrulaması KOLAYLIKTIR, savunma değil. */
const MIN_SUBJECT = 5;
const MAX_SUBJECT = 120;
const MAX_BODY = 4000;

/** Uzunluk KARAKTER (rune) ile sayılır: "ş" iki bayttır, bir karakterdir. */
const runeLength = (s: string) => Array.from(s).length;

const PRIORITIES: Array<{ value: TicketPriority; label: string }> = [
  { value: 'LOW', label: 'Düşük' },
  { value: 'NORMAL', label: 'Normal' },
  { value: 'HIGH', label: 'Yüksek' },
];

/* ═══════════════════════ Sayfa ═══════════════════════ */

export default function SupportPage() {
  const [offset, setOffset] = React.useState(0);
  const [creating, setCreating] = React.useState(false);
  const [openId, setOpenId] = React.useState<string | null>(null);

  const [durum, setDurum] = React.useState('');
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
    queryKey: ['tickets', { limit: SAYFA_BOYUTU, offset, durum, arama }],
    queryFn: () => {
      // Süzgeç SUNUCUDA uygulanır; istemcide süzmek yalnız bu sayfadaki 25
      // satırı görürdü ve eski bir talebi "yok" gösterirdi.
      const p = new URLSearchParams({
        limit: String(SAYFA_BOYUTU),
        offset: String(offset),
      });
      if (durum) p.set('status', durum);
      if (arama) p.set('q', arama);
      return apiFetch<TicketList>(`/tickets?${p.toString()}`);
    },
    // Sayfa değişince liste boşalıp zıplamasın.
    placeholderData: keepPreviousData,
  });

  const total = q.data?.total ?? 0;

  /*
   * `Sayfalama` GÖRÜNÜR MÜ? Koşul bileşenin kendisinden okunur
   * (`sayfalamaGorunur`), burada kopyalanmaz — kopya, bileşenin gizlenme
   * kuralı değiştiği gün sessizce yanlış olur ve iki canlı bölge birden
   * konuşmaya başlar.
   */
  const sayfali = sayfalamaGorunur(total);

  /*
   * SÜTUN TANIMI = TEK VERİ KAYNAĞI (§6.1 kural 2). Kart sunumu buradan
   * TÜRETİLİR; ikinci kez elle yazılmaz.
   *
   * ÖNCELİKLER: tablo `md:`'de (768px) başlar ve `/panel` içeriği o genişlikte
   * tam sayfadır (yan sütun yok). Nowrap dayatan üç hücre — rozet, tarih,
   * düğme — yaklaşık 400px taban ister; "Konu" SARMALAR (`truncate` yok, çünkü
   * kısaltma `nowrap` demektir ve dar tabloda taşma üretir), böylece kalan
   * genişliğe oturur. Sığmayan tek sütun "Öncelik": günde okunan bir alan
   * değil, `lg:`ye alındı — mobil kartta ve yazışma diyaloğunda duruyor,
   * kaybolmuyor.
   */
  const sutunlar: ReadonlyArray<Sutun<Ticket>> = [
    {
      anahtar: 'konu',
      baslik: 'Konu',
      mobilRol: 'baslik',
      hucre: (t) => (
        // `max-w-[24rem]` yalnız ÜST sınırdır; hücre daralabilir.
        // `break-anywhere`: boşluksuz uzun bir konu 320px'te yatay kaydırma
        // üretirdi.
        <div className="max-w-[24rem] break-anywhere">
          <span className="block font-medium">{t.subject}</span>
          {/* Mesaj sayısı ayrı sütun DEĞİL, konunun alt satırı — dar tabloda
              beşinci bir nowrap sütunu taşma üretiyordu. Kartta da aynı yerde,
              yani iki sunum aynı yapıyı gösteriyor. `text-sm`: veri taşıyan
              metin 14px altına inmez (§3.2 — eskiden `text-xs` idi). */}
          <span className="block text-sm tabular-nums text-muted">{t.messageCount} mesaj</span>
        </div>
      ),
    },
    {
      anahtar: 'oncelik',
      baslik: 'Öncelik',
      // §6.1 kural 3: 768px'te sığmayan sütun `lg:`ye alınır, SİLİNMEZ.
      oncelik: 3,
      hucre: (t) => t.priorityLabel,
    },
    {
      anahtar: 'durum',
      baslik: 'Durum',
      mobilRol: 'rozet',
      hucre: (t) => <DurumRozeti durum={t.status} etiket={t.statusLabel} />,
    },
    {
      anahtar: 'sonHareket',
      baslik: 'Son hareket',
      hizala: 'sag',
      // Tarih de sayıdır: `tabular-nums` olmadan alt alta gelen saatler kayar
      // (§3.5). Eskiden bu hücre `text-xs` idi ve hizasızdı.
      sayisal: true,
      hucre: (t) => formatDateTime(t.lastReplyAt),
    },
    {
      anahtar: 'islem',
      baslik: 'İşlem',
      basligiGizle: true,
      hizala: 'sag',
      mobilRol: 'eylem',
      // `sunum` YALNIZ SUNUM farkı içindir (dar düğme ↔ tam genişlik düğme).
      // Metin ondan TÜRETİLMEZ — ayrışmanın kaynağı tam olarak buydu.
      hucre: (t, sunum) => (
        <Button
          variant="outline"
          size="sm"
          fullWidth={sunum === 'kart'}
          onClick={() => setOpenId(t.id)}
        >
          Yazışmayı aç
        </Button>
      ),
    },
  ];

  return (
    <div className="flex flex-col gap-6">
      <SayfaBasligi
        baslik="Destek"
        aciklama="Sorununuzu yazın, destek ekibimiz yanıtlasın. Yanıtlar bu sayfada görünür."
      >
        {/* Mobilde tam genişlik, `sm:` üstünde doğal genişlik — bugünkü
            davranışın aynısı. */}
        <Button fullWidth className="sm:w-auto" onClick={() => setCreating(true)}>
          Yeni talep
        </Button>
      </SayfaBasligi>

      <Card className="flex flex-col gap-6">
        <div className="flex flex-wrap items-center justify-between gap-3">
          {/* `h2` her yerde aynı boyutta: `text-lg font-semibold` (§3.3). */}
          <h2 className="text-lg font-semibold">Taleplerim</h2>
          {/* Rozet bir ETİKETTİR, cümle değil (§5.2). Sayı `tabular-nums`. */}
          <KayitSayaci toplam={total} />
        </div>

        {/* Süzgeç liste kartının İÇİNDE: bu sayfada "Yeni talep" düğmesi zaten
            sayfa başlığında duruyor, araya üçüncü bir kart girmez. */}
        <SuzgecCubugu
          etkinSayisi={(durum ? 1 : 0) + (arama ? 1 : 0)}
          onTemizle={() => {
            setDurum('');
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
              placeholder="Talep konusu"
              autoCapitalize="none"
              autoCorrect="off"
              autoComplete="off"
              spellCheck={false}
              hint="Yazmayı bıraktığınızda arama kendiliğinden yapılır."
            />
          </div>

          <div className="w-full sm:w-64 sm:self-start">
            <Secim
              etiket="Durum"
              value={durum}
              onChange={(e) => {
                setDurum(e.target.value);
                // Süzgeç değişince 3. sayfada kalmak BOŞ ekran gösterir.
                setOffset(0);
              }}
            >
              {DURUM_SUZGECLERI.map((d) => (
                <option key={d.value || 'tumu'} value={d.value}>
                  {d.label}
                </option>
              ))}
            </Secim>
          </div>
        </SuzgecCubugu>

        <VeriTablosu
          baslik="Destek taleplerim"
          sutunlar={sutunlar}
          satirlar={q.data?.items}
          satirAnahtari={(t) => t.id}
          yukleniyor={q.isLoading}
          hata={apiHatasi(q.error)}
          duyuru={!sayfali}
          bos={
            /* Boş durum ÖĞRETİR, "burada bir şey yok" demez (§5.1).
               §6.3: SÜZGEÇTEN DOLAYI BOŞ ≠ GERÇEKTEN BOŞ — talebi olan ama
               seçtiği durumda kaydı olmayan kullanıcıya "Henüz talebiniz yok"
               demek yanlıştır ve yeni talep açmaya iter. Gerçekten boşta metin
               "ilk kaydı oluştur" yönündedir. */
            durum || arama ? (
              <Empty
                title="Bu süzgece uyan talep yok"
                hint="Arama metnini kısaltmayı ya da durumu “Tüm durumlar” yapmayı deneyin."
              />
            ) : (
              <Empty
                title="Henüz talebiniz yok"
                hint="Bir sorunuz veya sorununuz olduğunda 'Yeni talep' ile bize yazın; yanıtımız bu sayfada görünür."
              />
            )
          }
        />

        <Sayfalama offset={offset} limit={SAYFA_BOYUTU} toplam={total} onDegis={setOffset} />
      </Card>

      {creating && <CreateDialog onClose={() => setCreating(false)} onCreated={setOpenId} />}
      {openId && <ThreadDialog id={openId} onClose={() => setOpenId(null)} />}
    </div>
  );
}

/* ═══════════════════════ Yeni talep ═══════════════════════ */

/**
 * `OnayDiyalogu` DEĞİL, ham `Modal`.
 *
 * §6.4 modalı iki duruma indirir: (a) yıkıcı işlem onayı, (b) korunmuş odak
 * gerektiren çok adımlı form. Bu (b)'dir: konu + öncelik + mesaj. `OnayDiyalogu`
 * iki düğmeli bir evet/hayır adımıdır ve buraya uymaz.
 */
function CreateDialog({
  onClose, onCreated,
}: { onClose: () => void; onCreated: (id: string) => void }) {
  const qc = useQueryClient();
  const [subject, setSubject] = React.useState('');
  const [priority, setPriority] = React.useState<TicketPriority>('NORMAL');
  const [message, setMessage] = React.useState('');
  const [errs, setErrs] = React.useState<Record<string, string>>({});

  const create = useMutation({
    mutationFn: (v: { subject: string; priority: TicketPriority; message: string }) =>
      apiFetch<Ticket>('/tickets', { method: 'POST', body: v }),
    // MUTASYON OTOMATİK TEKRARLANMAZ (değişmez #16): ağ yanıtı yutulduğunda
    // ikinci deneme İKİNCİ BİR TALEP açar ve yönetici aynı soruyu iki kez görür.
    retry: false,
    onSuccess: (t) => {
      qc.invalidateQueries({ queryKey: ['tickets'] });
      onClose();
      onCreated(t.id);
    },
  });

  function submit(e: React.FormEvent) {
    e.preventDefault();
    const next: Record<string, string> = {};
    const s = subject.trim();
    const m = message.trim();
    if (runeLength(s) < MIN_SUBJECT) next.subject = 'Konu en az 5 karakter olmalıdır.';
    else if (runeLength(s) > MAX_SUBJECT) next.subject = 'Konu en fazla 120 karakter olabilir.';
    if (m.length === 0) next.message = 'Mesaj boş olamaz.';
    else if (runeLength(m) > MAX_BODY) next.message = 'Mesaj en fazla 4000 karakter olabilir.';
    setErrs(next);
    if (Object.keys(next).length) return;
    create.mutate({ subject: s, priority, message: m });
  }

  const err = apiHatasi(create.error);

  return (
    <Modal open onClose={onClose} title="Yeni destek talebi">
      <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
        <Field
          label="Konu"
          data-autofocus
          value={subject}
          onChange={(e) => setSubject(e.target.value)}
          error={errs.subject}
          hint={`${runeLength(subject)}/${MAX_SUBJECT} karakter`}
          maxLength={MAX_SUBJECT * 2}
          disabled={create.isPending}
        />

        {/* `Secim`: `.select-ok` (appearance:none) + `min-h-12`. WebKit yerel
            `menulist` görünümü yüksekliği kendi hesaplar ve kutuyu 44px dokunma
            hedefinin altına düşürür; iOS'ta tüm tarayıcılar WebKit'tir. */}
        <Secim
          etiket="Öncelik"
          value={priority}
          disabled={create.isPending}
          onChange={(e) => setPriority(e.target.value as TicketPriority)}
        >
          {PRIORITIES.map((p) => <option key={p.value} value={p.value}>{p.label}</option>)}
        </Secim>

        <CokSatir
          etiket="Mesajınız"
          rows={6}
          value={message}
          disabled={create.isPending}
          onChange={(e) => setMessage(e.target.value)}
          hata={errs.message}
          ipucu={`${runeLength(message)}/${MAX_BODY} karakter`}
          placeholder="Sorununuzu olabildiğince ayrıntılı yazın. Sipariş numarası varsa ekleyin."
        />

        {err && <HataDurumu hata={err} />}

        {/* Mobilde alt alta (birincil üstte, parmak menzilinde), `sm:` üstünde
            `flex-row-reverse` ile birincil SAĞDA — panelin tek buton düzeni. */}
        <div className="flex flex-col gap-2 sm:flex-row-reverse">
          <Button type="submit" loading={create.isPending} fullWidth className="sm:w-auto">
            Talebi gönder
          </Button>
          <Button type="button" variant="outline" fullWidth className="sm:w-auto"
                  onClick={onClose} disabled={create.isPending}>
            Vazgeç
          </Button>
        </div>
      </form>
    </Modal>
  );
}

/* ═══════════════════════ Yazışma ═══════════════════════ */

function ThreadDialog({ id, onClose }: { id: string; onClose: () => void }) {
  const qc = useQueryClient();
  const [message, setMessage] = React.useState('');
  const [formErr, setFormErr] = React.useState('');

  const q = useQuery({
    queryKey: ['ticket', id],
    queryFn: () => apiFetch<Ticket>(`/tickets/${encodeURIComponent(id)}`),
  });

  const reply = useMutation({
    mutationFn: (body: string) =>
      apiFetch<Ticket>(`/tickets/${encodeURIComponent(id)}/messages`, {
        method: 'POST', body: { message: body },
      }),
    // Tekrarlanan bir yanıt, yazışmaya aynı mesajı iki kez düşürürdü.
    retry: false,
    onSuccess: () => {
      setMessage('');
      qc.invalidateQueries({ queryKey: ['ticket', id] });
      qc.invalidateQueries({ queryKey: ['tickets'] });
    },
  });

  const t = q.data;
  const loadErr = apiHatasi(q.error);
  const replyErr = apiHatasi(reply.error);
  const closed = t?.status === 'CLOSED';

  function submit(e: React.FormEvent) {
    e.preventDefault();
    const m = message.trim();
    if (m.length === 0) { setFormErr('Mesaj boş olamaz.'); return; }
    if (runeLength(m) > MAX_BODY) { setFormErr('Mesaj en fazla 4000 karakter olabilir.'); return; }
    setFormErr('');
    reply.mutate(m);
  }

  return (
    <Modal open onClose={onClose} title={t?.subject ?? 'Destek talebi'}>
      {q.isLoading ? (
        <div className="flex flex-col gap-3">
          {[0, 1].map((i) => <Skeleton key={i} className="h-20" />)}
        </div>
      ) : loadErr ? (
        <HataDurumu hata={loadErr} />
      ) : !t ? (
        <Empty title="Talep bulunamadı" hint="Bu talep silinmiş ya da size ait olmayabilir." />
      ) : (
        <div className="flex flex-col gap-4">
          <div className="flex flex-wrap items-center gap-2">
            <DurumRozeti durum={t.status} etiket={t.statusLabel} />
            <Badge tone="neutral">Öncelik: {t.priorityLabel}</Badge>
            {/* `text-sm` + `tabular-nums`: bu bir tarihtir, dipnot değil. */}
            <span className="text-sm tabular-nums text-muted">
              Açılış: {formatDateTime(t.createdAt)}
            </span>
          </div>

          {!t.messages?.length ? (
            <Empty title="Bu talepte mesaj yok" />
          ) : (
            <ul className="flex flex-col gap-3">
              {t.messages.map((m) => (
                <li
                  key={m.id}
                  /* Personel mesajı zeminle ayrışır. ANLAM YALNIZ RENKLE
                     TAŞINMAZ (§7.1): kimin yazdığını `authorLabel` söyler,
                     renk yalnız pekiştirir. */
                  className={
                    m.isStaff
                      ? 'rounded-xl border border-brand-500/30 bg-brand-500/10 p-4'
                      : 'raised rounded-xl border p-4'
                  }
                >
                  <div className="flex flex-wrap items-baseline justify-between gap-2">
                    <span className="text-sm font-semibold">{m.authorLabel}</span>
                    <span className="text-sm tabular-nums text-muted">
                      {formatDateTime(m.createdAt)}
                    </span>
                  </div>
                  {/*
                    DÜZ METİN. `dangerouslySetInnerHTML` YOKTUR: gövde kullanıcı
                    girdisidir ve HTML olarak basılsaydı bir kullanıcı, talebi
                    okuyan yöneticinin oturumunda betik çalıştırabilirdi.
                    Satır sonları CSS ile korunur, işaretleme ile değil.
                    `max-w-[70ch]`: düz metin satır uzunluğu 65-75ch (§3.4).
                    `break-anywhere`: boşluksuz uzun bir dize (URL, hash)
                    320px'te yatay kaydırma üretirdi.
                  */}
                  <p className="mt-2 max-w-[70ch] whitespace-pre-wrap break-anywhere
                                text-sm leading-relaxed">
                    {m.body}
                  </p>
                </li>
              ))}
            </ul>
          )}

          {closed ? (
            /* Statik bilgi kutusu `role="alert"` ALMAZ (§7.4): kullanıcı daha
               hiçbir şey yapmadan sözü kesilmemeli. */
            <Alert tone="info" duyur={false}>
              Bu talep kapatıldı. Yeni bir sorunuz varsa lütfen yeni bir talep açın —
              önceki yazışmanız burada kalır.
            </Alert>
          ) : (
            <form onSubmit={submit}
                  className="flex flex-col gap-4 border-t border-[var(--border)] pt-4"
                  noValidate>
              <CokSatir
                etiket="Yanıtınız"
                rows={4}
                value={message}
                disabled={reply.isPending}
                onChange={(e) => setMessage(e.target.value)}
                hata={formErr}
                ipucu={`${runeLength(message)}/${MAX_BODY} karakter`}
              />
              {replyErr && <HataDurumu hata={replyErr} />}
              <Button type="submit" loading={reply.isPending} fullWidth
                      className="sm:w-auto sm:self-end">
                Gönder
              </Button>
            </form>
          )}
        </div>
      )}
    </Modal>
  );
}
