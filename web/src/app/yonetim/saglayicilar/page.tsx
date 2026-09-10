'use client';

/**
 * /yonetim/saglayicilar — üst sağlayıcılar, ayarları ve sağlayıcıdaki bakiyemiz.
 *
 * DAVRANIŞ DEĞİŞMEDİ. Bu dosyada değişen şey sunum ve yapıdır: çift render
 * (mobil kart + masaüstü tablo) tek bir sütun tanımına indirildi, yerel
 * `ErrorBox`/`SELECT_CLASS` kopyaları katman bileşenleriyle değiştirildi.
 * İki istisna aşağıda 🔴 ile işaretli ve raporda gerekçelendirildi.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * ETİKET AYRIŞMASI KAPANDI (tasarim-sistemi.md §6)
 * ══════════════════════════════════════════════════════════════════════════
 * Ölçülen iki ayrışma bu ekrandaydı:
 *   kart "Anahtar kurulu değil" ↔ tablo "Kurulu değil"
 *   kart "Katalogu senkronla"   ↔ tablo "Senkronla"
 * `Sutun.baslik` tek dize olduğu için artık yapısal olarak imkânsız. Hangi
 * dizenin kaldığı keyfi değil, bir kurala bağlandı:
 *   · Bir sütun başlığı (tabloda `<th>`, kartta `<dt>`) ismi zaten taşıyorsa
 *     hücrede KISA değer kalır: "Anahtar" başlığı + "Kurulu değil" değeri;
 *     "Senkron" başlığı + "Başarısız" rozeti.
 *   · Aynı gerekçe "Senkronla" düğmesi için de geçerli: hemen üstünde/yanında
 *     "Senkron" sütunu duruyor, yani neyin senkronlandığı iki sunumda da
 *     görünür. Ölçüldü: uzun etiket ("Katalogu senkronla") İşlem sütununu üç
 *     satıra bölüp masaüstü satırını 181px'e çıkarıyordu.
 */

import * as React from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { apiFetch } from '@/lib/api';
import { formatDateTime, formatDecimal, formatMoney, trimDecimal } from '@/lib/format';
import { Alert, Badge, Button, Card, Empty, Field, cx } from '@/components/ui';
import { Modal } from '@/components/modal';
import {
  DurumRozeti,
  HataDurumu,
  KayitSayaci,
  SayfaBasligi,
  Secim,
  VeriTablosu,
  apiHatasi,
  ikiliTon,
  type Sutun,
  SAYFA_BOYUTU,
  Sayfalama,
  SuzgecCubugu,
  useIstemciSuzgec,
} from '@/components/yonetim';
import type { AdminProvider } from '@/lib/types';

/** GET /admin/providers — sayfalama YOK, yalnız {items}. */
interface AdminProviderList {
  items: AdminProvider[];
}

/** POST /admin/providers/:id/sync yanıtı. */
interface ProviderSyncResult {
  providerId: string;
  started: boolean;
  sync: { running: boolean; startedAt?: string; finishedAt?: string; failed?: boolean };
}

/**
 * PUT /admin/providers/:id/api-key yanıtı.
 *
 * 🔴 `masked` alanı GÖSTERİLMEZ. Sunucu maskeli bir önizleme döndürüyor ama
 * onu ekrana yazmak anahtarın bir parçasını ekran görüntüsüne, ekran
 * paylaşımına ve tarayıcı geçmişine taşır. Başarı bilgisi için maskeye gerek
 * yok: liste rozetinin "Kurulu"ya dönmesi ve açık başarı mesajı yeter.
 * Tip burada duruyor ki alanın var olduğu ve BİLEREK okunmadığı belli olsun.
 */
interface SetApiKeyResult {
  masked: string;
}

/** Sunucu tarafındaki provider_protocol enum'u (dto.ProviderProtocols). */
const PROTOCOLS = ['FAKE', 'HEROSMS_V1', 'FIVE_SIM'] as const;

const CAPABILITIES: Array<{ value: string; label: string }> = [
  { value: 'SMS_ACTIVATION', label: 'Tek kullanımlık numara (aktivasyon)' },
  { value: 'SMS_RENTAL', label: 'Kiralık numara' },
];

const CAP_LABELS: Record<string, string> = {
  SMS_ACTIVATION: 'Aktivasyon',
  SMS_RENTAL: 'Kiralama',
};

const QUERY_KEY = ['admin-providers'] as const;

/** Onay kutusu satırı — dokunma hedefi 44px (§7.3). Üç yerde tekrar ediyordu. */
function OnayKutusu({
  isaretli, onDegis, children,
}: { isaretli: boolean; onDegis: (v: boolean) => void; children: React.ReactNode }) {
  return (
    <label className="flex min-h-11 items-start gap-3 py-2">
      <input
        type="checkbox"
        checked={isaretli}
        onChange={(e) => onDegis(e.target.checked)}
        className="size-5 shrink-0 rounded accent-[var(--color-brand-500)]"
      />
      <span className="text-sm">{children}</span>
    </label>
  );
}

export default function AdminProvidersPage() {
  const qc = useQueryClient();
  const [arama, setArama] = React.useState('');

  const q = useQuery({
    queryKey: QUERY_KEY,
    queryFn: () => apiFetch<AdminProviderList>('/admin/providers'),
    /*
     * Senkron durumu AYRI BİR UÇTAN OKUNMAZ: tetikleme ucunu yoklamak her
     * seferinde yeni bir tur başlatır. Durum bu listenin `sync` alanındadır,
     * bu yüzden yalnız bir senkron KOŞARKEN liste tazelenir. Boşta yoklama
     * yapılmaz — yönetim uçları dakikada 60 istekle sınırlıdır.
     */
    refetchInterval: (query) =>
      query.state.data?.items.some((p) => p.sync?.running) ? 5_000 : false,
  });

  const [editing, setEditing] = React.useState<AdminProvider | null>(null);
  const [keying, setKeying] = React.useState<AdminProvider | null>(null);
  const [creating, setCreating] = React.useState(false);

  const sync = useMutation({
    mutationFn: (id: string) =>
      apiFetch<ProviderSyncResult>(`/admin/providers/${encodeURIComponent(id)}/sync`, {
        method: 'POST',
      }),
    // Senkron tetikleme idempotent değildir; otomatik tekrar yeni tur başlatır.
    retry: false,
    onSuccess: () => qc.invalidateQueries({ queryKey: QUERY_KEY }),
  });

  const syncErr = apiHatasi(sync.error);
  const items = q.data?.items ?? [];

  /*
   * Arama ve sayfalama İSTEMCİDE: `GET /admin/providers` sayfasızdır, yanıt
   * `{items}` — tam liste elimizdedir. Ayrımın tamamı `useIstemciSuzgec`
   * başında yazılı; sayfalı bir uçta bu kanca YANLIŞTIR.
   */
  const { sayfadakiler, toplam, offset, setOffset } = useIstemciSuzgec(
    q.data?.items,
    arama,
    (p) => [p.name, p.protocol, p.baseUrl, ...p.capabilities],
  );

  /* ── Sütunlar: TEK veri tanımı; mobil kart bundan türer (§6.1 kural 2) ── */
  const sutunlar: Array<Sutun<AdminProvider>> = [
    {
      anahtar: 'saglayici',
      baslik: 'Sağlayıcı',
      mobilRol: 'baslik',
      hucre: (p) => (
        <div className="min-w-0">
          <p className="font-medium">{p.name}</p>
          {/* `text-sm`, `text-xs` değil: protokol ve adres VERİDİR (§3.2). */}
          <p className="text-sm text-muted">{p.protocol}</p>
          <p className="break-anywhere text-sm text-muted">{p.baseUrl || '—'}</p>
        </div>
      ),
    },
    {
      anahtar: 'durum',
      baslik: 'Durum',
      /*
        🔴 `rozet` YUVASINA YALNIZ ROZET KONUR — ölçüldü. `VeriTablosu` bu
        yuvayı `shrink-0` ile sarar; içine "Son senkron: 09.09.2026 11:20"
        gibi uzun bir metin koymak yuvayı ~200px'te sabitler, 320px'lik kartta
        başlık sütununa ~8px bırakır ve `break-anywhere` taşıyan baseUrl
        KARAKTER KARAKTER kırılır: kart 435px'ten 945px'e çıkıyordu.
        Senkron bilgisi bu yüzden kendi sütununda ve kartın `<dl>` gövdesinde.
      */
      mobilRol: 'rozet',
      hucre: (p) => (
        <DurumRozeti
          durum={p.isActive ? 'ACTIVE' : 'INACTIVE'}
          etiket={p.isActive ? 'Aktif' : 'Pasif'}
          ton={ikiliTon(p.isActive)}
        />
      ),
    },
    {
      anahtar: 'senkron',
      baslik: 'Senkron',
      oncelik: 3,
      hucre: (p) => <SenkronDurumu sync={p.sync} />,
    },
    {
      anahtar: 'anahtar',
      baslik: 'Anahtar',
      hucre: (p) => (
        // §5.4: anahtar eksikse `bad` — sağlayıcı aktif olsa bile çağrı patlar.
        <DurumRozeti
          durum={p.hasApiKey ? 'ACTIVE' : 'FAILED'}
          etiket={p.hasApiKey ? 'Kurulu' : 'Kurulu değil'}
          ton={p.hasApiKey ? 'ok' : 'bad'}
        />
      ),
    },
    {
      anahtar: 'bakiye',
      baslik: 'Bakiyemiz',
      hizala: 'sag',
      sayisal: true,
      hucre: (p) => <span className="font-medium">{formatMoney(p.balance)}</span>,
    },
    {
      anahtar: 'oncelik',
      baslik: 'Öncelik',
      oncelik: 3,
      hizala: 'sag',
      sayisal: true,
      hucre: (p) => p.priority,
    },
    {
      anahtar: 'carpan',
      baslik: 'Çarpan',
      oncelik: 3,
      hizala: 'sag',
      sayisal: true,
      /*
        Sunucudan STRING gelir ve `Number()`a ÇEVRİLMEZ: çarpan doğrudan satış
        fiyatı zincirine giriyor, kayan noktaya uğratmak sessiz bir yuvarlama
        olurdu. `formatDecimal` dönüşümü tamamen metinsel yapar.

        🔴 SUNUCUDAN GELDİĞİ GİBİ BASILMIYOR ARTIK. Eski hâl `4.0000` yazıyordu;
        Türkçede `.` BİNLİK ayırıcı olduğu için kullanıcı bunu "4000" diye
        okudu ve çarpanını yanlış sandı. Değer doğruydu, yazım yanlıştı.
      */
      /*
        1 DIŞINDAKİ DEĞER İŞARETLENİR. Çarpan normalde 1'dir ve öyle kaldığı
        sürece görünmez bir alandır; 1 olmadığı anda satış fiyatını doğrudan
        değiştirir. Listeyi tarayan yöneticinin bunu fark etmesi için rozet
        gerekiyor — ölçüm: çarpan aylarca 4'te kaldı ve kimse görmedi, çünkü
        sütunda yalnız sayı vardı ve "4" tek başına yanlış görünmüyor.
      */
      hucre: (p) => {
        const sade = trimDecimal(p.costMultiplier);
        return (
          <span className="inline-flex items-center gap-2">
            {formatDecimal(p.costMultiplier)}
            {sade !== '1' && <Badge tone="warn">maliyet ×{sade}</Badge>}
          </span>
        );
      },
    },
    {
      anahtar: 'yetenekler',
      baslik: 'Yetenekler',
      oncelik: 3,
      hucre: (p) =>
        p.capabilities.length ? (
          <div className="flex flex-wrap justify-end gap-2 md:justify-start">
            {p.capabilities.map((c) => (
              <Badge key={c} tone="brand">
                {CAP_LABELS[c] ?? c}
              </Badge>
            ))}
          </div>
        ) : (
          <span className="text-muted">Tanımsız</span>
        ),
    },
    {
      anahtar: 'islem',
      baslik: 'İşlem',
      basligiGizle: true,
      hizala: 'sag',
      mobilRol: 'eylem',
      hucre: (p, sunum) => {
        const senkronBekliyor = sync.isPending && sync.variables === p.id;
        return (
          /*
            ÜÇÜ DE EŞİT AĞIRLIKTA ikincil eylem: `fullWidth` ile alt alta
            dizmek birini birincil gibi gösterir ve ÖLÇÜLDÜ — 390px'te kartı
            52px uzatıyordu (487px → 435px). Sarmalayan satır her iki sunumda
            da aynı; `Button` min-h-11 taşıdığı için dokunma hedefi 44px kalır.
          */
          <div className={cx('flex flex-wrap gap-2', sunum === 'tablo' && 'justify-end')}>
            <Button variant="outline" size="sm" onClick={() => setEditing(p)}>
              Ayarlar
            </Button>
            <Button variant="outline" size="sm" onClick={() => setKeying(p)}>
              API anahtarı
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={p.sync?.running || senkronBekliyor}
              loading={senkronBekliyor}
              onClick={() => sync.mutate(p.id)}
            >
              Senkronla
            </Button>
          </div>
        );
      },
    },
  ];

  return (
    <div className="flex flex-col gap-6">
      <SayfaBasligi
        baslik="Sağlayıcılar"
        aciklama="Numara aldığımız üst sağlayıcılar, ayarları ve sağlayıcıdaki bakiyemiz."
      >
        <Button onClick={() => setCreating(true)}>Sağlayıcı ekle</Button>
      </SayfaBasligi>

      {/* `duyur={false}`: sayfa açılışında koşulsuz çizilen statik açıklama;
          kesecek bir eylem yok (§7.4). */}
      <Alert tone="info" duyur={false}>
        Bir sağlayıcının ayarlarını kaydetmek API anahtarını değiştirmez; anahtar
        ayrı bir formdan yazılır ve hiçbir ekranda geri gösterilmez.
      </Alert>

      {syncErr && <HataDurumu hata={syncErr} />}

      <Card>
        <div className="flex flex-wrap items-center justify-between gap-3">
          <h2 className="text-lg font-semibold">Tanımlı sağlayıcılar</h2>
          <KayitSayaci toplam={toplam} />
        </div>

        <SuzgecCubugu
          className="mt-4"
          etkinSayisi={arama ? 1 : 0}
          onTemizle={() => {
            setArama('');
            setOffset(0);
          }}
        >
          <div className="w-full sm:w-72 sm:self-start">
            <Field
              label="Ara"
              type="search"
              value={arama}
              onChange={(e) => {
                setArama(e.target.value);
                setOffset(0);
              }}
              placeholder="Ad, protokol veya yetenek"
              autoCapitalize="none"
              autoCorrect="off"
              autoComplete="off"
              spellCheck={false}
            />
          </div>
        </SuzgecCubugu>

        <VeriTablosu<AdminProvider>
          className="mt-4"
          baslik="Tanımlı sağlayıcılar"
          sutunlar={sutunlar}
          satirlar={sayfadakiler}
          satirAnahtari={(p) => p.id}
          yukleniyor={q.isLoading}
          hata={apiHatasi(q.error)}
          iskeletSatir={3}
          bos={
            // §6.3: SÜZGEÇTEN DOLAYI BOŞ ≠ GERÇEKTEN BOŞ. Sağlayıcısı olan ama
            // aramasıyla eşleşme bulamayan yöneticiye "henüz sağlayıcı yok"
            // demek yanlış olduğu gibi yanlış eyleme de iter.
            arama ? (
              <Empty
                title="Bu aramaya uyan sağlayıcı yok"
                hint="Sağlayıcı adının, protokolün ya da bir yeteneğin parçasını yazmayı deneyin."
              />
            ) : (
              <div className="flex flex-col items-center pb-12">
                <Empty
                  title="Henüz sağlayıcı yok"
                  hint="Sağlayıcı ekleyip API anahtarını tanımladıktan sonra etkinleştirebilirsiniz."
                />
                <Button onClick={() => setCreating(true)}>Sağlayıcı ekle</Button>
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

      {/* Modal içerikleri, kapanınca durumları da gitsin diye koşullu monte edilir. */}
      <Modal open={editing !== null} onClose={() => setEditing(null)} title="Sağlayıcı ayarları">
        {editing && <ProviderSettingsForm provider={editing} onDone={() => setEditing(null)} />}
      </Modal>

      <Modal open={keying !== null} onClose={() => setKeying(null)} title="API anahtarı">
        {keying && <ApiKeyForm provider={keying} onDone={() => setKeying(null)} />}
      </Modal>

      <Modal open={creating} onClose={() => setCreating(false)} title="Sağlayıcı ekle">
        {creating && <CreateProviderForm onDone={() => setCreating(false)} />}
      </Modal>
    </div>
  );
}

/**
 * Senkron durumu — listedeki `sync` alanından okunur, ayrı uç yoktur.
 *
 * Kartta sağa, tabloda sola yaslanır (`items-end md:items-start`): kart yalnız
 * `< md`, tablo yalnız `md:` üstünde çizilir, yani tek sınıf dizisi ikisini de
 * doğru kurar — `max-*` ile geri alma zinciri yok.
 */
function SenkronDurumu({ sync }: { sync: AdminProvider['sync'] }) {
  const govde = () => {
    if (!sync) return <span className="text-muted">—</span>;
    /*
      Rozet metinleri "Senkron" sözcüğünü TAŞIMAZ: onu sütun başlığı ve kart
      `<dt>`si zaten söylüyor. Ölçüldü — "Son senkron başarısız" yazmak bu
      sütunu 173px'e çıkarıyor, İşlem sütununu 187px'e sıkıştırıyor ve üç
      düğme ÜÇ satıra iniyor: satır 181px. Kısaltınca düğmeler iki satıra
      düşüyor. Rozet bir etikettir, bir cümle değildir (§5.2).
    */
    if (sync.running) {
      return (
        <>
          <Badge tone="warn">Çalışıyor</Badge>
          {sync.startedAt && (
            <span className="text-sm text-muted">{formatDateTime(sync.startedAt)}</span>
          )}
        </>
      );
    }
    if (sync.failed) {
      return (
        <>
          <Badge tone="bad">Başarısız</Badge>
          {sync.finishedAt && (
            <span className="text-sm text-muted">{formatDateTime(sync.finishedAt)}</span>
          )}
        </>
      );
    }
    if (sync.finishedAt) {
      return <span className="text-sm text-muted">{formatDateTime(sync.finishedAt)}</span>;
    }
    return <span className="text-muted">Hiç çalışmadı</span>;
  };

  return <div className="flex flex-col items-end gap-1 md:items-start">{govde()}</div>;
}

/* ═══════════ Ayarlar ve Ekle formlarının ORTAK üç alanı ═══════════ */

/**
 * `baseUrl` · `priority` · `costMultiplier` — iki formda da aynı alanlar, aynı
 * kurallar. Eskiden İKİ KEZ elle yazılıyordu ve ZATEN AYRIŞMIŞTI: çarpan hata
 * mesajı bir formda "(örn. 1.25)", diğerinde "(örn. 1.00)"; adres ipucu yalnız
 * Ekle formunda, çarpan ipucu yalnız Ayarlar formunda vardı — oysa dördü de her
 * iki formda doğru. Tek kaynağa indirildi; ipuçları birleştirildi.
 */
interface OrtakAlanDegerleri {
  baseUrl: string;
  priority: string;
  costMultiplier: string;
}

/** Doğrulama + kırpılmış değerler. Gövdeye kırpılmışlar gönderilir. */
function dogrulaOrtakAlanlar(
  v: OrtakAlanDegerleri,
): { errs: Record<string, string>; url: string; prio: number; mult: string } {
  const errs: Record<string, string> = {};

  const url = v.baseUrl.trim();
  if (url && !/^https?:\/\//i.test(url)) {
    errs.baseUrl = 'Adres http:// veya https:// ile başlamalıdır.';
  }
  const prio = Number(v.priority.trim());
  if (!/^\d+$/.test(v.priority.trim()) || !Number.isInteger(prio) || prio < 0 || prio > 10000) {
    errs.priority = 'Öncelik 0-10000 arasında bir tam sayı olmalıdır.';
  }
  const mult = v.costMultiplier.trim();
  if (!/^\d+(\.\d+)?$/.test(mult)) {
    errs.costMultiplier = 'Çarpan ondalık bir sayı olmalıdır (örn. 1.25).';
  }

  return { errs, url, prio, mult };
}

function OrtakAlanlar({
  deger, degistir, hatalar, adresOtoOdak,
}: {
  deger: OrtakAlanDegerleri;
  degistir: (alan: keyof OrtakAlanDegerleri, v: string) => void;
  hatalar: Record<string, string>;
  /** Ayarlar formunda ilk odak adrestedir; Ekle formunda "Ad" alanındadır. */
  adresOtoOdak?: boolean;
}) {
  return (
    <>
      <Field
        data-autofocus={adresOtoOdak || undefined}
        label="Adres (baseUrl)"
        value={deger.baseUrl}
        onChange={(e) => degistir('baseUrl', e.target.value)}
        placeholder="https://api.ornek.com"
        inputMode="url"
        autoCapitalize="none"
        autoCorrect="off"
        autoComplete="off"
        spellCheck={false}
        error={hatalar.baseUrl}
        hint="Boş bırakılırsa protokolün varsayılan adresi kullanılır."
      />

      <Field
        label="Öncelik"
        value={deger.priority}
        onChange={(e) => degistir('priority', e.target.value)}
        inputMode="numeric"
        autoComplete="off"
        error={hatalar.priority}
        hint="0-10000. Küçük değer önce denenir."
      />

      <Field
        label="Maliyet çarpanı"
        value={deger.costMultiplier}
        onChange={(e) => degistir('costMultiplier', e.target.value)}
        inputMode="decimal"
        autoComplete="off"
        error={hatalar.costMultiplier}
        /*
          İPUCU NE OLMADIĞINI DA SÖYLER. Eski metin ("Sağlayıcı maliyeti bu
          çarpanla düzeltilir") doğruydu ama eksikti: alanın kâr marjı
          OLMADIĞINI ve marjın nerede olduğunu söylemiyordu. Bu ipucu
          `aria-describedby` ile alana bağlı, yani ekran okuyucu da duyar.
        */
        hint="Sağlayıcının bildirdiği maliyeti düzeltir; normalde 1 kalır. Kâr marjı DEĞİLDİR — marj, Fiyat kuralları ekranındadır."
      />
      <CarpanUyarisi ham={deger.costMultiplier} />
    </>
  );
}

/**
 * Çarpan 1 değilken beliren uyarı.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * NEDEN VAR
 * ══════════════════════════════════════════════════════════════════════════
 * Bu alan kâr marjı sanıldı ve 4 yazıldı. Sonuç: sağlayıcı maliyeti dörde
 * katlandı, üstüne fiyat kuralının %40 marjı bindi ve müşteri 83 ₺'lik
 * numarayı 332 ₺ gördü. Hiçbir yerde hata yoktu — sistem tam da söylendiği
 * gibi çalışıyordu. Yanlış olan tek şey, alanın ne yaptığını söylememesiydi.
 *
 * Mimari bu ikisini bilerek ayırmıştır (design.md §487, §601): çarpan
 * *maliyet düzeltmesi*, marj *iş kararı*. Eski sistemde ikisi tek alanda
 * karışıktı ve yeniden yazımda ayrılmalarının sebebi tam olarak buydu.
 *
 * 🔴 `duyur={false}` — kutu, kullanıcı yazarken her tuş vuruşunda yeniden
 * çizilir. `role="alert"` olsaydı ekran okuyucu her harfte sözü keserdi.
 * Metnin kendisi ipucunda da var ve o ipucu alana `aria-describedby` ile
 * bağlı; yani duyuru kaybolmuyor, yalnız kesintili olmuyor.
 */
function CarpanUyarisi({ ham }: { ham: string }) {
  const v = ham.trim();
  // Geçersiz giriş için susulur: alan doğrulaması zaten konuşuyor, iki mesaj
  // aynı anda görünürse hangisinin engellediği belirsizleşir.
  if (!/^\d+(\.\d+)?$/.test(v)) return null;
  const sade = trimDecimal(v);
  if (sade === '1') return null;

  return (
    <Alert tone="warn" duyur={false}>
      <strong>Bu alan kâr marjı değildir.</strong> Sağlayıcının bildirdiği
      maliyet {sade} ile çarpılır ve satış fiyatı aynı oranda değişir. Kârı
      buradan ayarlamayın — marj <strong>Fiyat kuralları</strong> ekranındadır.
      Maliyet düzeltmesine ihtiyaç yoksa <strong>1</strong> yazın.
    </Alert>
  );
}

/**
 * Sağlayıcı ayarları.
 *
 * 🔴 PATCH /admin/providers/:id KISMİ DEĞİLDİR: gövdedeki dört alan
 * (baseUrl, isActive, priority, costMultiplier) koşulsuz yazılır. Yalnız
 * önceliği değiştirmek için `{priority}` göndermek baseUrl'ü boşaltır ve
 * isActive'i false yapar — sağlayıcı sessizce ölür, sipariş akışı durur.
 * Bu yüzden form GET'ten gelen MEVCUT DEĞERLERLE doldurulur ve her kaydetmede
 * dördü birden gönderilir.
 */
function ProviderSettingsForm({
  provider, onDone,
}: { provider: AdminProvider; onDone: () => void }) {
  const qc = useQueryClient();
  const [ortak, setOrtak] = React.useState<OrtakAlanDegerleri>({
    baseUrl: provider.baseUrl,
    priority: String(provider.priority),
    // Düzenleme kutusuna SADE hâli konur (`4.0000` → `4`): ayırıcı nokta
    // kalır, çünkü bu değer sunucuya aynen geri gidiyor ve Postgres `numeric`
    // virgülü ayrıştıramaz.
    costMultiplier: trimDecimal(provider.costMultiplier),
  });
  const [isActive, setIsActive] = React.useState(provider.isActive);
  const [errors, setErrors] = React.useState<Record<string, string>>({});
  const degistir = (alan: keyof OrtakAlanDegerleri, v: string) =>
    setOrtak((o) => ({ ...o, [alan]: v }));

  const save = useMutation({
    mutationFn: (body: {
      baseUrl: string; isActive: boolean; priority: number; costMultiplier: string;
    }) =>
      apiFetch<{ id: string; name: string; isActive: boolean }>(
        `/admin/providers/${encodeURIComponent(provider.id)}`,
        { method: 'PATCH', body },
      ),
    retry: false,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEY });
      onDone();
    },
  });

  function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    const { errs, url, prio, mult } = dogrulaOrtakAlanlar(ortak);
    setErrors(errs);
    if (Object.keys(errs).length) return;

    // TAM GÖVDE: dördü birden gönderilir, eksik alan sunucuda sıfırlanır.
    save.mutate({ baseUrl: url, isActive, priority: prio, costMultiplier: mult });
  }

  const err = apiHatasi(save.error);
  const fieldErrs = { ...errors, ...(err?.fieldMap() ?? {}) };

  return (
    <form onSubmit={onSubmit} className="flex flex-col gap-4" noValidate>
      <SaglayiciKimligi ad={provider.name} alt={provider.protocol} />

      {/* `duyur={false}`: formun giriş metni, diyaloğun İLK çiziminde var —
          "Sağlayıcı ayarları" diyalog duyurusunu kesecek ikinci bir duyuru
          olmamalı (§7.4). Aşağıdaki anahtarsız-etkinleştirme uyarısı ise
          `role="alert"` TAŞIMAYA DEVAM EDER: o, kutucuk işaretlenince belirir. */}
      <Alert tone="warn" duyur={false}>
        Bu form sağlayıcının <strong>tüm ayarlarını</strong> birlikte kaydeder.
        Alanları olduğu gibi bırakırsanız değişmez; boşaltırsanız o değer silinir.
      </Alert>

      <OrtakAlanlar adresOtoOdak deger={ortak} degistir={degistir} hatalar={fieldErrs} />

      <OnayKutusu isaretli={isActive} onDegis={setIsActive}>
        Sağlayıcı aktif
        <span className="mt-1 block text-sm text-muted">
          Pasif sağlayıcı teklif ve satın alma yolunda hiç denenmez.
        </span>
      </OnayKutusu>

      {/*
        🔴 `role="alert"` BURADA KALIYOR (varsayılan `duyur`). Bu kutu statik
        değil: `isActive` yukarıdaki kutucuğun yerel durumudur ve kutu tam da
        yönetici "Sağlayıcı aktif"i İŞARETLEDİĞİ anda belirir. Yani eylemin
        sonucunda beliren bir uyarıdır — §7.4'ün `role="alert"`i tanımladığı
        durum birebir budur. Susturulursa, anahtarsız bir sağlayıcıyı
        etkinleştiren ekran okuyucu kullanıcısı uyarıyı hiç duymaz.
      */}
      {!provider.hasApiKey && isActive && (
        <Alert tone="warn">
          Bu sağlayıcının API anahtarı tanımlı değil. Anahtarsız etkinleştirirseniz
          çağrılar başarısız olur.
        </Alert>
      )}

      {err && <HataDurumu hata={err} />}

      <FormDugmeleri
        onayMetni="Ayarları kaydet"
        bekliyor={save.isPending}
        iptalMetni="Vazgeç"
        onIptal={onDone}
      />
    </form>
  );
}

/**
 * API anahtarı — AYRI form, ayrı uç.
 *
 * 🔴 Anahtar hiçbir GET yanıtında dönmez; ekranda gösterilmez ve React
 * durumunda tutulmaz. Alan kontrolsüzdür (ref ile okunur), gönderimden hemen
 * sonra temizlenir.
 *
 * 🔴 MASKELİ ÖNİZLEME DE GÖSTERİLMEZ. Sunucu `masked` döndürüyor; eski sürüm
 * bunu ekrana yazıyordu. Bir maskede bile anahtarın baş/son karakterleri
 * bulunur ve bu ekran görüntüsüne, ekran paylaşımına, destek biletine düşer.
 * Kaydın başarılı olduğunu listedeki "Kurulu" rozeti zaten söylüyor.
 *
 * Boş gönderim sunucuda 422 ile reddedilir (anahtar silme yolu yoktur;
 * sağlayıcı pasifleştirilir), bu yüzden boş formu hiç göndermeyiz.
 */
function ApiKeyForm({ provider, onDone }: { provider: AdminProvider; onDone: () => void }) {
  const qc = useQueryClient();
  const inputRef = React.useRef<HTMLInputElement>(null);
  const [error, setError] = React.useState<string>();

  const save = useMutation({
    mutationFn: (apiKey: string) =>
      apiFetch<SetApiKeyResult>(`/admin/providers/${encodeURIComponent(provider.id)}/api-key`, {
        method: 'PUT',
        body: { apiKey },
      }),
    retry: false,
    onSuccess: () => {
      // Anahtar bellekte kalmasın: alan başarıdan hemen sonra boşaltılır.
      if (inputRef.current) inputRef.current.value = '';
      qc.invalidateQueries({ queryKey: QUERY_KEY });
    },
  });

  function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    const key = inputRef.current?.value.trim() ?? '';
    if (!key) {
      setError('Anahtar boş olamaz. Silmek için sağlayıcıyı pasifleştirin.');
      return;
    }
    setError(undefined);
    save.mutate(key);
  }

  const err = apiHatasi(save.error);

  return (
    <form onSubmit={onSubmit} className="flex flex-col gap-4" noValidate>
      <SaglayiciKimligi
        ad={provider.name}
        alt={provider.hasApiKey ? 'Şu an bir anahtar tanımlı.' : 'Şu an anahtar tanımlı değil.'}
      />

      {/* `duyur={false}`: formun giriş metni, "API anahtarı" diyaloğunun ilk
          çiziminde var. Aşağıdaki `save.isSuccess` kutusu ise `role="alert"`
          taşımaya devam eder — o, kaydetmenin SONUCUdur (§7.4). */}
      <Alert tone="info" duyur={false}>
        Anahtar kaydedildikten sonra <strong>hiçbir ekranda geri gösterilmez</strong>.
        Yeni bir anahtar yazmak eskisinin yerine geçer.
      </Alert>

      <Field
        ref={inputRef}
        data-autofocus
        label="Yeni API anahtarı"
        type="password"
        autoComplete="off"
        autoCapitalize="none"
        autoCorrect="off"
        spellCheck={false}
        error={error ?? err?.fieldMap().apiKey}
        hint="Sağlayıcı panelinden aldığınız anahtarı yapıştırın."
      />

      {err && <HataDurumu hata={err} />}

      {save.isSuccess && (
        <Alert tone="ok">Anahtar kaydedildi. Sağlayıcı listesinde &laquo;Kurulu&raquo; görünecek.</Alert>
      )}

      <FormDugmeleri
        onayMetni="Anahtarı kaydet"
        bekliyor={save.isPending}
        iptalMetni="Kapat"
        onIptal={onDone}
      />
    </form>
  );
}

/**
 * Yeni sağlayıcı.
 *
 * 🔴 Bu formda API anahtarı alanı YOKTUR: ekleme ucu anahtar almaz. Aynı
 * gövdede taşınsaydı anahtar hata ayıklama çıktısına ve tarayıcı ağ sekmesine
 * düşerdi. Yeni sağlayıcı PASİF doğar; anahtar konup boyut eşleştirmeleri
 * senkronlandıktan sonra ayarlardan etkinleştirilir.
 */
function CreateProviderForm({ onDone }: { onDone: () => void }) {
  const qc = useQueryClient();
  const [name, setName] = React.useState('');
  const [protocol, setProtocol] = React.useState<string>(PROTOCOLS[0]);
  const [ortak, setOrtak] = React.useState<OrtakAlanDegerleri>({
    baseUrl: '',
    priority: '100',
    costMultiplier: '1.00',
  });
  const [capabilities, setCapabilities] = React.useState<string[]>(['SMS_ACTIVATION']);
  const [errors, setErrors] = React.useState<Record<string, string>>({});
  const degistir = (alan: keyof OrtakAlanDegerleri, v: string) =>
    setOrtak((o) => ({ ...o, [alan]: v }));

  const create = useMutation({
    mutationFn: (body: {
      name: string; protocol: string; baseUrl: string;
      priority: number; costMultiplier: string; capabilities: string[];
    }) => apiFetch<AdminProvider>('/admin/providers', { method: 'POST', body }),
    retry: false,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEY });
      onDone();
    },
  });

  function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    const { errs, url, prio, mult } = dogrulaOrtakAlanlar(ortak);

    const nm = name.trim();
    if (!nm) errs.name = 'Ad zorunludur.';
    else if (nm.length > 60) errs.name = 'Ad en fazla 60 karakter olabilir.';
    if (!capabilities.length) errs.capabilities = 'En az bir yetenek seçin.';

    setErrors(errs);
    if (Object.keys(errs).length) return;

    create.mutate({ name: nm, protocol, baseUrl: url, priority: prio, costMultiplier: mult, capabilities });
  }

  const err = apiHatasi(create.error);
  const fieldErrs = { ...errors, ...(err?.fieldMap() ?? {}) };

  return (
    <form onSubmit={onSubmit} className="flex flex-col gap-4" noValidate>
      {/* `duyur={false}`: "Sağlayıcı ekle" diyaloğunun ilk çiziminde duran
          giriş metni; diyaloğun kendi duyurusunu kesmemeli (§7.4). */}
      <Alert tone="info" duyur={false}>
        Yeni sağlayıcı <strong>pasif</strong> olarak eklenir ve burada API anahtarı
        sorulmaz. Ekledikten sonra anahtarı tanımlayın, katalogu senkronlayın ve
        ancak ondan sonra ayarlardan etkinleştirin.
      </Alert>

      <Field
        data-autofocus
        label="Ad"
        value={name}
        onChange={(e) => setName(e.target.value)}
        placeholder="Örn. HeroSMS"
        maxLength={60}
        autoComplete="off"
        error={fieldErrs.name}
      />

      <Secim
        etiket="Protokol"
        value={protocol}
        onChange={(e) => setProtocol(e.target.value)}
        hata={fieldErrs.protocol}
        ipucu="Sağlayıcının konuştuğu API biçimi."
      >
        {PROTOCOLS.map((p) => (
          <option key={p} value={p}>
            {p}
          </option>
        ))}
      </Secim>

      <OrtakAlanlar deger={ortak} degistir={degistir} hatalar={fieldErrs} />

      <fieldset className="flex flex-col gap-2">
        <legend className="text-sm font-medium">Yetenekler</legend>
        {CAPABILITIES.map((c) => (
          <OnayKutusu
            key={c.value}
            isaretli={capabilities.includes(c.value)}
            onDegis={(v) =>
              setCapabilities((prev) => (v ? [...prev, c.value] : prev.filter((x) => x !== c.value)))
            }
          >
            {c.label}
          </OnayKutusu>
        ))}
        {fieldErrs.capabilities && (
          // `text-sm`: doğrulama hatası ikincil dipnot değildir (§3.2).
          <span role="alert" className="text-sm text-[var(--color-bad)]">
            {fieldErrs.capabilities}
          </span>
        )}
      </fieldset>

      {err && <HataDurumu hata={err} />}

      <FormDugmeleri
        onayMetni="Sağlayıcıyı ekle"
        bekliyor={create.isPending}
        iptalMetni="Vazgeç"
        onIptal={onDone}
      />
    </form>
  );
}

/* ═══════════════════════ Üç formun ortak parçaları ═══════════════════════ */

/** Formun hangi sağlayıcı üzerinde çalıştığını söyleyen kimlik bloğu. */
function SaglayiciKimligi({ ad, alt }: { ad: string; alt: string }) {
  return (
    <div className="raised rounded-xl border p-4">
      <p className="font-medium">{ad}</p>
      <p className="mt-1 text-sm text-muted">{alt}</p>
    </div>
  );
}

/**
 * Form alt düğmeleri — `OnayDiyalogu` DEĞİLDİR.
 *
 * `OnayDiyalogu` yıkıcı bir işlemin ONAY adımıdır ve ilk odağı bilerek
 * "Vazgeç"e koyar. Burası çok alanlı bir FORM: ilk odak `data-autofocus` ile
 * ilk alandadır, çünkü kullanıcı buraya yazmaya gelir, onaylamaya değil.
 * Düzen yine tektir: mobilde alt alta (onay üstte), `sm:` üstünde onay sağda.
 */
function FormDugmeleri({
  onayMetni, iptalMetni, bekliyor, onIptal,
}: { onayMetni: string; iptalMetni: string; bekliyor: boolean; onIptal: () => void }) {
  return (
    <div className="flex flex-col gap-2 sm:flex-row-reverse sm:justify-start">
      <Button type="submit" loading={bekliyor} fullWidth className="sm:w-auto">
        {onayMetni}
      </Button>
      <Button
        type="button"
        variant="outline"
        fullWidth
        className="sm:w-auto"
        disabled={bekliyor}
        onClick={onIptal}
      >
        {iptalMetni}
      </Button>
    </div>
  );
}
