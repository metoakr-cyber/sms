'use client';

/**
 * Destek talepleri — yönetim ekranı (FR-600).
 *
 * VARSAYILAN GÖRÜNÜM "bekleyenler"dir ve bu TEK BİR DURUM DEĞİLDİR: hem yeni
 * açılan (OPEN) hem de kullanıcının yanıt yazdığı (USER_REPLIED) talepler
 * personelin işidir. Yalnız OPEN süzmek, yanıt bekleyen müşteriyi varsayılan
 * ekranda görünmez yapardı — bu yüzden sunucuda ayrı bir `?pending=true`
 * süzgeci var.
 *
 * Ekrandaki gövde metinlerinin tamamı KULLANICI GİRDİSİDİR. React kaçırır;
 * `dangerouslySetInnerHTML` bu dosyada YOKTUR.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * BU DOSYA `components/yonetim` KATMANINA BAĞLANDI (tasarim-sistemi.md §5.3)
 * ══════════════════════════════════════════════════════════════════════════
 * Silinen yerel kopyalar: `ErrorBox` (9 ekranda birebir), `statusTone`
 * (6 kopya), `selectClass` + `Chevron` (`.select-ok` zaten globals.css'te),
 * `textareaClass`, elle kurulmuş sayfalama ve mobil kart ↔ masaüstü tablo
 * İKİZİ. Veri artık TEK YERDE (`sutunlar`) tanımlıdır; kart sunumu ondan
 * türetilir, ikinci kez elle yazılmaz.
 *
 * 🔴 ÇÖZÜLEN ETİKET AYRIŞMASI: eylem düğmesi mobil kartta "Yazışmayı aç",
 * masaüstü tabloda "Aç" yazıyordu — aynı düğme, iki isim (§6 ölçülen üç
 * ayrışmadan biri). Tek metin seçildi: "Yazışmayı aç". Kısa olan değil,
 * NE YAPTIĞINI SÖYLEYEN kazandı; masaüstü sütunu zaten yeterince geniş.
 */

import * as React from 'react';
import { useMutation, useQuery, useQueryClient, keepPreviousData } from '@tanstack/react-query';
import { apiFetch } from '@/lib/api';
import { formatDateTime } from '@/lib/format';
import { Card, Button, Alert, Badge, Skeleton, Empty } from '@/components/ui';
import { Modal } from '@/components/modal';
import {
  SayfaBasligi,
  SuzgecCubugu,
  KayitSayaci,
  Secim,
  CokSatir,
  VeriTablosu,
  Sayfalama,
  SAYFA_BOYUTU,
  DurumRozeti,
  HataDurumu,
  apiHatasi,
} from '@/components/yonetim';
import type { Sutun } from '@/components/yonetim';

/* ═══════════════════════ Sunucu sözleşmesi ═══════════════════════ */
/* Karşılıkları: api/internal/transport/http/dto/ticket.go
 * Tipler burada tanımlıdır çünkü `lib/types.ts` bu turda paylaşılan bir
 * dosyadır; kalıcı hâle geldiğinde oraya taşınmalıdır (rapora yazıldı). */

type TicketStatus = 'OPEN' | 'ANSWERED' | 'USER_REPLIED' | 'CLOSED';

interface TicketMessage {
  id: string;
  body: string;
  isStaff: boolean;
  authorLabel: string;
  /** Yönetim görünümünde yazarın kullanıcı adı gelir; kullanıcı görünümünde GELMEZ. */
  authorUsername?: string;
  createdAt: string;
}

interface AdminTicket {
  id: string;
  userId: string;
  userEmail: string;
  userUsername: string;
  subject: string;
  priority: 'LOW' | 'NORMAL' | 'HIGH';
  priorityLabel: string;
  status: TicketStatus;
  statusLabel: string;
  messageCount: number;
  createdAt: string;
  lastReplyAt: string;
  closedAt?: string;
  messages?: TicketMessage[];
}

interface AdminTicketList {
  items: AdminTicket[];
  total: number;
  limit: number;
  offset: number;
}

const MAX_BODY = 4000;
const runeLength = (s: string) => Array.from(s).length;

/** Süzgeç değerleri. `pending` bir durum değil, bir GÖRÜNÜMDÜR. */
type Filter = 'pending' | TicketStatus | 'all';

const FILTERS: Array<{ value: Filter; label: string }> = [
  // Varsayılan ilk sıradadır: yöneticinin işi bekleyen taleplerdir.
  { value: 'pending', label: 'Bekleyenler' },
  { value: 'OPEN', label: 'Yeni açılanlar' },
  { value: 'USER_REPLIED', label: 'Kullanıcı yanıtladı' },
  { value: 'ANSWERED', label: 'Yanıtlananlar' },
  { value: 'CLOSED', label: 'Kapatılanlar' },
  { value: 'all', label: 'Tümü' },
];

function queryFor(f: Filter, offset: number): string {
  // Sayfa boyutu artık ekranın değil, katmanın kararı: tek panelde tek `limit`
  // (§6.3). Yerel `PAGE = 20` sabiti silindi.
  const qs = new URLSearchParams({ limit: String(SAYFA_BOYUTU), offset: String(offset) });
  if (f === 'pending') qs.set('pending', 'true');
  else if (f !== 'all') qs.set('status', f);
  return qs.toString();
}

/* ═══════════════════════ Sayfa ═══════════════════════ */

export default function AdminTicketsPage() {
  const [filter, setFilter] = React.useState<Filter>('pending');
  const [offset, setOffset] = React.useState(0);
  const [openId, setOpenId] = React.useState<string | null>(null);

  const q = useQuery({
    queryKey: ['admin-tickets', { filter, offset }],
    queryFn: () => apiFetch<AdminTicketList>(`/admin/tickets?${queryFor(filter, offset)}`),
    placeholderData: keepPreviousData,
  });

  const total = q.data?.total ?? 0;

  /*
   * `Sayfalama` GÖRÜNÜR MÜ? — `sayfalama.tsx` tek sayfaya sığan listede kendini
   * hiç çizmez, yani bu ifade "sayfalama ekranda var mı" sorusunun aynısıdır.
   * İki yerde birden kullanılıyor ve ikisi de AYNI hatayı kapatıyor:
   *
   *  1. ÇİFT CANLI BÖLGE. `VeriTablosu` ("12 kayıt listelendi.") ve `Sayfalama`
   *     ("1–25 / 87") ikisi de `aria-live="polite"`. Süzgeç değişince ekran
   *     okuyucu iki duyuruyu sıraya alır; ikincisi birincinin üstüne biner.
   *     `kullanicilar` ve `denetim` bunu `duyuru={!sayfali}` ile çözmüştü, bu
   *     ekran kuralın dışında kalmıştı.
   *  2. YANLIŞ SAYI. `VeriTablosu`nun duyurusu SAYFADAKİ satır sayısıdır; 87
   *     kayıtlık listede her sayfada "25 kayıt listelendi" der. Sayfalı liste
   *     tam olarak bu ifadenin yanlış olduğu durumdur — ve orada susuyor.
   *     Kalan durumda (tek sayfa) sayfadaki sayı ZATEN toplama eşittir, yani
   *     duyuru açık kaldığı her yerde doğrudur.
   */
  const sayfali = total > SAYFA_BOYUTU;

  /*
   * SÜTUN TANIMI = TEK VERİ KAYNAĞI (§6.1 kural 2).
   * `baslik` alanı hem `<th>` hem mobil kart `<dt>` olarak okunur; aynı veriyi
   * iki yerde farklı etiketlemek yapısal olarak imkânsız.
   *
   * `sunum` YALNIZ sunum farkı içindir (dar düğme ↔ `fullWidth` düğme, kısaltma
   * ↔ satır kaydırma). Metin ondan TÜRETİLMEZ.
   *
   * 🔴 SÜTUN ÖNCELİKLERİ ÖLÇÜMLE BELİRLENDİ. `admin-shell` `md:` üstünde
   * 240px'lik (`w-60`) bir yan sütun çiziyor; tablonun ilk göründüğü
   * genişlikte (768px) içerik alanı 768 − 240 − 48 = **480px**'dir. İlk
   * taslak 7 sütundu ve tarayıcıda ölçüldüğünde sayfa 951px'e taşıyordu
   * (yatay kaydırma = CLAUDE.md #17 ihlali): `whitespace-nowrap` taşıyan üç
   * hücre (rozet ~150px, tarih ~128px, düğme ~103px) sert bir asgari genişlik
   * dayatıyor, üstüne `truncate` (= nowrap) konmuş hücreler taban ekliyor.
   * Düzeltme: "Mesaj" ayrı sütun olmaktan çıkıp Konu'nun alt satırına indi ·
   * Konu/Kullanıcı kısaltma yerine SARMALIYOR · Kullanıcı ve Son hareket
   * `oncelik: 3`. 768px'te kalan: Konu · Durum · İşlem — bilgi kaybolmaz,
   * üçü de mobil kartta ve yazışma diyaloğunda durur.
   */
  const sutunlar: ReadonlyArray<Sutun<AdminTicket>> = [
    {
      anahtar: 'konu',
      baslik: 'Konu',
      mobilRol: 'baslik',
      hucre: (t) => (
        // `truncate` YOK: kısaltma `nowrap` demektir ve dar tabloda taşma
        // üretir (yukarıdaki ölçüm). `max-w-[22rem]` yalnız ÜST sınırdır;
        // hücre daralabilir.
        <div className="max-w-[22rem] break-anywhere">
          <span className="block font-medium">{t.subject}</span>
          {/* Mesaj sayısı burada: ayrı sütun 768px'te sığmıyordu ve bu alt
              satır zaten bugünkü masaüstü davranışı. Kartta da aynı yerde. */}
          <span className="block text-sm tabular-nums text-muted">{t.messageCount} mesaj</span>
        </div>
      ),
    },
    {
      anahtar: 'kullanici',
      baslik: 'Kullanıcı',
      oncelik: 3,
      hucre: (t) => (
        <div className="max-w-[16rem] break-anywhere">
          <span className="block">{t.userUsername}</span>
          <span className="block text-sm text-muted">{t.userEmail}</span>
        </div>
      ),
    },
    {
      anahtar: 'oncelik',
      baslik: 'Öncelik',
      // §6.1 kural 3: 768px'te sığmayan sütun `lg:`'ye alınır, silinmez.
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
      oncelik: 3,
      hizala: 'sag',
      // Tarih de sayıdır: `tabular-nums` olmadan alt alta gelen saatler kayar
      // (§3.5 — `/yonetim` altında bugün 0 kullanım).
      sayisal: true,
      hucre: (t) => formatDateTime(t.lastReplyAt),
    },
    {
      anahtar: 'islem',
      baslik: 'İşlem',
      basligiGizle: true,
      hizala: 'sag',
      mobilRol: 'eylem',
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
    <div className="mx-auto flex max-w-5xl flex-col gap-6">
      <SayfaBasligi
        baslik="Destek talepleri"
        aciklama="Kullanıcı taleplerini okuyun, yanıtlayın ve sonuçlananları kapatın."
      />

      <Card className="flex flex-col gap-6">
        <SuzgecCubugu sag={<KayitSayaci toplam={total} />}>
          <Secim
            etiket="Görünüm"
            className="sm:w-64"
            value={filter}
            onChange={(e) => {
              setFilter(e.target.value as Filter);
              setOffset(0); // süzgeç değişti; eski sayfa numarası anlamsız
            }}
          >
            {FILTERS.map((f) => (
              <option key={f.value} value={f.value}>
                {f.label}
              </option>
            ))}
          </Secim>
        </SuzgecCubugu>

        <VeriTablosu
          baslik="Destek talepleri"
          sutunlar={sutunlar}
          satirlar={q.data?.items}
          satirAnahtari={(t) => t.id}
          yukleniyor={q.isLoading}
          hata={apiHatasi(q.error)}
          // Sayfalama kendi aralığını duyuruyor; iki canlı bölge aynı anda
          // konuşmasın (yukarıdaki `sayfali` notu).
          duyuru={!sayfali}
          bos={
            /* §6.3: süzgeçten dolayı boş ile gerçekten boş FARKLI metinlerdir. */
            <Empty
              title="Talep yok"
              hint={
                filter === 'pending'
                  ? 'Yanıt bekleyen destek talebi bulunmuyor.'
                  : 'Bu görünüme uyan bir talep bulunmuyor. Görünümü "Tümü" yaparak listeyi genişletebilirsiniz.'
              }
            />
          }
        />

        <Sayfalama
          offset={offset}
          limit={SAYFA_BOYUTU}
          toplam={total}
          onDegis={setOffset}
        />
      </Card>

      {openId && <AdminThreadDialog id={openId} onClose={() => setOpenId(null)} />}
    </div>
  );
}

/* ═══════════════════════ Yazışma diyaloğu ═══════════════════════ */

/**
 * `OnayDiyalogu` DEĞİL, ham `Modal`.
 *
 * §6.4 modalı iki duruma indirir: (a) yıkıcı işlem onayı, (b) korunmuş odak
 * gerektiren çok adımlı form. Bu diyalog (b)'dir: yazışmayı okuyup yanıt
 * yazmak tek bir "evet/hayır" değildir. `OnayDiyalogu` iki düğmeli bir onay
 * adımıdır ve buraya uymaz.
 *
 * "Talebi kapat" için ARA ONAY EKLENMEDİ: bugün tek tıklamayla kapanıyor ve
 * işlem GERİ ALINABİLİR — aynı diyalogdaki "Talebi yeniden aç" düğmesi bunu
 * yapar. Geri alınabilir bir işleme onay adımı eklemek, günde onlarca kez
 * yapılan bir işi yavaşlatmaktan başka bir şey yapmazdı.
 */
function AdminThreadDialog({ id, onClose }: { id: string; onClose: () => void }) {
  const qc = useQueryClient();
  const [message, setMessage] = React.useState('');
  const [formErr, setFormErr] = React.useState('');

  const q = useQuery({
    queryKey: ['admin-ticket', id],
    queryFn: () => apiFetch<AdminTicket>(`/admin/tickets/${encodeURIComponent(id)}`),
  });

  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ['admin-ticket', id] });
    qc.invalidateQueries({ queryKey: ['admin-tickets'] });
  };

  const reply = useMutation({
    mutationFn: (body: string) =>
      apiFetch<AdminTicket>(`/admin/tickets/${encodeURIComponent(id)}/messages`, {
        method: 'POST', body: { message: body },
      }),
    // MUTASYON OTOMATİK TEKRARLANMAZ (değişmez #16): tekrar, kullanıcıya aynı
    // yanıtı iki kez göndermek demektir.
    retry: false,
    onSuccess: () => { setMessage(''); invalidate(); },
  });

  const setStatus = useMutation({
    mutationFn: (status: 'OPEN' | 'CLOSED') =>
      apiFetch<AdminTicket>(`/admin/tickets/${encodeURIComponent(id)}/status`, {
        // PATCH: durum DEĞİŞTİRİR, GET olamaz (değişmez #8).
        method: 'PATCH', body: { status },
      }),
    retry: false,
    onSuccess: invalidate,
  });

  const t = q.data;
  const loadErr = apiHatasi(q.error);
  const mutErr = apiHatasi(reply.error) ?? apiHatasi(setStatus.error);
  const closed = t?.status === 'CLOSED';
  const busy = reply.isPending || setStatus.isPending;

  function submit(e: React.FormEvent) {
    e.preventDefault();
    const m = message.trim();
    if (m.length === 0) { setFormErr('Mesaj boş olamaz.'); return; }
    if (runeLength(m) > MAX_BODY) {
      // Sayaç `ipucu` yuvasındadır ve hata varken gizlenir; bu yüzden güncel
      // uzunluk HATA METNİNİN İÇİNDE tekrar verilir. Sınırı aştığını söyleyip
      // "ne kadar aştın"ı gizlemek, kullanıcıyı saymaya zorlar.
      setFormErr(`Mesaj en fazla ${MAX_BODY} karakter olabilir; şu an ${runeLength(m)}.`);
      return;
    }
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
        <Empty title="Talep bulunamadı" hint="Talep silinmiş ya da bağlantı eskimiş olabilir." />
      ) : (
        <div className="flex flex-col gap-5">
          <div className="flex flex-wrap items-center gap-2">
            <DurumRozeti durum={t.status} etiket={t.statusLabel} />
            <Badge tone="neutral">Öncelik: {t.priorityLabel}</Badge>
          </div>

          {/* Künye. `text-sm` — `text-xs` DEĞİL: burası veri (§3.2). */}
          <dl className="raised flex flex-col gap-3 rounded-xl border p-4 text-sm">
            <div className="flex items-start justify-between gap-4">
              <dt className="shrink-0 text-muted">Kullanıcı</dt>
              <dd className="min-w-0 truncate font-medium">{t.userUsername}</dd>
            </div>
            <div className="flex items-start justify-between gap-4">
              <dt className="shrink-0 text-muted">E-posta</dt>
              <dd className="min-w-0 break-anywhere text-right">{t.userEmail}</dd>
            </div>
            <div className="flex items-start justify-between gap-4">
              <dt className="shrink-0 text-muted">Açılış</dt>
              <dd className="tabular-nums">{formatDateTime(t.createdAt)}</dd>
            </div>
            {t.closedAt && (
              <div className="flex items-start justify-between gap-4">
                <dt className="shrink-0 text-muted">Kapanış</dt>
                <dd className="tabular-nums">{formatDateTime(t.closedAt)}</dd>
              </div>
            )}
          </dl>

          {!t.messages?.length ? (
            <Empty title="Bu talepte mesaj yok" />
          ) : (
            <ul className="flex flex-col gap-3">
              {t.messages.map((m) => (
                <li
                  key={m.id}
                  className={
                    m.isStaff
                      ? 'rounded-xl border border-brand-500/30 bg-brand-500/10 p-4'
                      : 'raised rounded-xl border p-4'
                  }
                >
                  <div className="flex flex-wrap items-baseline justify-between gap-2">
                    <span className="text-sm font-semibold">
                      {/* Yönetim görünümünde YAZARIN ADI görünür: "kim yanıtladı"
                          destek ekibinin iç sorusudur. Kullanıcı görünümünde bu
                          alan sunucudan HİÇ gelmez. */}
                      {m.isStaff ? `${m.authorLabel}${m.authorUsername ? ` · ${m.authorUsername}` : ''}`
                                 : t.userUsername}
                    </span>
                    <span className="text-sm tabular-nums text-muted">
                      {formatDateTime(m.createdAt)}
                    </span>
                  </div>
                  {/* DÜZ METİN — `dangerouslySetInnerHTML` YOKTUR. Talebi okuyan
                      yöneticinin oturumunda kullanıcı betiği çalışamaz. */}
                  <p className="mt-3 max-w-[70ch] text-sm leading-relaxed whitespace-pre-wrap break-anywhere">
                    {m.body}
                  </p>
                </li>
              ))}
            </ul>
          )}

          {mutErr && <HataDurumu hata={mutErr} />}

          {closed ? (
            <div className="flex flex-col gap-3 border-t border-[var(--border)] pt-5">
              {/*
                🔴 `role="alert"` KALIYOR (varsayılan `duyur`). Bu kutu iki
                yoldan gelir ve ikincisi belirleyici: "Talebi kapat" başarıyla
                dönünce `t.status` CLOSED olur, yanıt formu YERİNİ buna bırakır.
                Panelde toast yok (§5.3 P1-11); yani bu kutu, kapatma işleminin
                ekran okuyucuya ulaşan TEK geri bildirimidir — susturulursa
                yönetici düğmeye bastı mı, işledi mi, bilemez.
                Zaten kapalı bir talep açılırken bir kez fazladan okunması bu
                kaybın yanında kabul edilebilir; üstelik okunan şey talebin en
                önemli durum bilgisidir.
              */}
              <Alert tone="info">
                Bu talep kapalı. Kullanıcı kapalı bir talebe yazamaz; yazışmaya devam
                edilmesi gerekiyorsa talebi yeniden açın.
              </Alert>
              <Button variant="outline" fullWidth className="sm:w-auto sm:self-end"
                      loading={setStatus.isPending} onClick={() => setStatus.mutate('OPEN')}>
                Talebi yeniden aç
              </Button>
            </div>
          ) : (
            <form onSubmit={submit} className="flex flex-col gap-4 border-t
                                               border-[var(--border)] pt-5" noValidate>
              {/*
                `data-autofocus` BU ALANDA KALIYOR (bugünkü davranış).
                §7.5'in yasağı odağı ONAY DÜĞMESİNE koymaktır — basılı kalan
                Enter'ın işlemi tetiklemesi riski oradadır. Bir metin alanında
                Enter satır başı yapar, hiçbir şey göndermez; buraya gelen
                yöneticinin ilk işi zaten yazmaktır.
              */}
              <CokSatir
                data-autofocus
                etiket="Yanıtınız"
                rows={5}
                value={message}
                disabled={busy}
                onChange={(e) => setMessage(e.target.value)}
                ipucu={`${runeLength(message)}/${MAX_BODY} karakter`}
                hata={formErr || undefined}
              />
              <div className="flex flex-col gap-2 sm:flex-row-reverse">
                <Button type="submit" loading={reply.isPending} disabled={busy}
                        fullWidth className="sm:w-auto">
                  Yanıtla
                </Button>
                <Button type="button" variant="outline" fullWidth className="sm:w-auto"
                        loading={setStatus.isPending} disabled={busy}
                        onClick={() => setStatus.mutate('CLOSED')}>
                  Talebi kapat
                </Button>
              </div>
            </form>
          )}
        </div>
      )}
    </Modal>
  );
}
