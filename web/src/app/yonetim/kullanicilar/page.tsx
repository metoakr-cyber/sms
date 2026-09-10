'use client';

/**
 * Kullanıcılar — hesap arama, durum görüntüleme, erişim kapatma.
 *
 * SUNUM KATMANI: `components/yonetim`. Bu dosya artık tablo/kart ikizi,
 * `ErrorBox`, `PAGE` sabiti, sayfalama şeridi ve `statusTone` haritası
 * TAŞIMAZ — hepsi katmandan gelir (`docs/tasarim-sistemi.md` §5.3).
 *
 * DAVRANIŞ DEĞİŞMEDİ: aynı uç noktalar, aynı sorgu anahtarları, aynı 350 ms
 * arama gecikmesi, aynı `retry: false` mutasyonu, aynı "kendi hesabını
 * askıya alamazsın" kısıtı. Değişenler dosya sonundaki rapora yazıldı.
 */

import * as React from 'react';
import Link from 'next/link';
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { apiFetch } from '@/lib/api';
import { formatDateTime, formatMoney } from '@/lib/format';
import { useSession } from '@/hooks/useSession';
import { Badge, Button, Card, Empty, Field } from '@/components/ui';
import {
  apiHatasi,
  DurumRozeti,
  KayitSayaci,
  OnayDiyalogu,
  Sayfalama,
  SAYFA_BOYUTU,
  SayfaBasligi,
  Secim,
  SuzgecCubugu,
  VeriTablosu,
  sayfalamaGorunur,
} from '@/components/yonetim';
import type { DurumTonu, Sunum, Sutun } from '@/components/yonetim';
import type { AdminUser } from '@/lib/types';

type UserStatus = AdminUser['status'];

/** GET /admin/users — sayfalı zarf (dto.AdminUserListResponse). */
interface AdminUserList {
  items: AdminUser[];
  total: number;
  limit: number;
  offset: number;
}

/** PATCH /admin/users/:id/status YALNIZ {id,status} döner — tam kullanıcı değil. */
interface SetStatusResult {
  id: string;
  status: UserStatus;
}

/**
 * Hesap durumu → Türkçe etiket + ton.
 *
 * TON AÇIKÇA VERİLİR, `durumTonu()`'ndan türetilmez: katmandaki tablo
 * `SUSPENDED` ve `PENDING_VERIFICATION` kodlarını tanımıyor ve ikisi de
 * `neutral` düşüyor. Bugünkü ekran onları `bad`/`warn` gösteriyor; bir hesap
 * kısıtının nötr renge düşmesi sessiz bir gerilemedir. `DurumRozeti` bu
 * durum için `ton` ezmesini kabul ediyor.
 */
const DURUM_BILGISI: Record<UserStatus, { etiket: string; ton: DurumTonu }> = {
  ACTIVE: { etiket: 'Aktif', ton: 'ok' },
  SUSPENDED: { etiket: 'Askıda', ton: 'bad' },
  PENDING_VERIFICATION: { etiket: 'Doğrulama bekliyor', ton: 'warn' },
};

const DURUM_SUZGECLERI: ReadonlyArray<{ value: '' | UserStatus; label: string }> = [
  { value: '', label: 'Tüm durumlar' },
  { value: 'ACTIVE', label: 'Aktif' },
  { value: 'PENDING_VERIFICATION', label: 'Doğrulama bekliyor' },
  { value: 'SUSPENDED', label: 'Askıda' },
];

/**
 * Kullanıcı kimliğini panoya kopyalar.
 *
 * Bakiye düzeltme ekranı kullanıcıyı UUID ile ister ve sorgu parametresi
 * KABUL ETMEZ; yönetici kimliği elle yazmak zorunda kalıyordu. 36 karakterlik
 * bir UUID'yi elle yazmak, yanlış hesaba para yazmanın en kısa yoludur.
 *
 * Katmanda `CopyButton` YOK (§5.3 P1-12, henüz yazılmadı) — bu yüzden yerel
 * kaldı; raporda bildirildi.
 */
function KimlikKopyala({ id }: { id: string }) {
  const [kopyalandi, setKopyalandi] = React.useState(false);

  React.useEffect(() => {
    if (!kopyalandi) return;
    const t = setTimeout(() => setKopyalandi(false), 2000);
    return () => clearTimeout(t);
  }, [kopyalandi]);

  return (
    <Button
      variant="outline"
      size="sm"
      onClick={async () => {
        try {
          // Pano API'si güvensiz bağlamda ve izin verilmediğinde reddeder;
          // kopyalanamaması ekranı bozmamalı.
          await navigator.clipboard.writeText(id);
          setKopyalandi(true);
        } catch {
          setKopyalandi(false);
        }
      }}
    >
      {kopyalandi ? 'Kopyalandı' : 'Kimliği kopyala'}
    </Button>
  );
}

/**
 * Satır eylemleri — her iki sunumda AYNI metinler.
 *
 * Yönetici KENDİ hesabını askıya alamaz (sunucu 422 ile reddeder). Düğmeyi
 * baştan kapatmak, kullanıcıyı yapamayacağı bir işlemin hatasıyla
 * karşılaştırmaktan iyidir — ama devre dışı bir düğmenin `title`'ı
 * dokunmatikte ve klavyede OKUNMAZ, bu yüzden gerekçe metin olarak da yazılır.
 *
 * 🔴 "Bakiye düzelt" BURADAN ÇIKTI, sayfa başlığına taşındı. Gerekçe ölçülmüş:
 * `/yonetim/bakiye` hiçbir sorgu parametresi okumuyor (`bakiye/page.tsx`'te
 * `useSearchParams` yok; kimlik elle yazılıyor) — yani satırdaki bağlantı
 * satırın kullanıcısını HİÇ TAŞIMIYORDU, 25 satırda birbirinin aynı 25
 * bağlantı üretiyordu. Üstelik üçüncü düğme, 1440px'te bile eylem sütununu
 * ikinci satıra sarıyordu: ölçüm 129px satır ↔ iki düğmeyle 76px.
 * Akış aynı kaldı: kimliği kopyala → başlıktaki "Bakiye düzelt" → yapıştır.
 */
function SatirEylemleri({
  kullanici,
  benMi,
  sunum,
  onSor,
}: {
  kullanici: AdminUser;
  benMi: boolean;
  sunum: Sunum;
  onSor: (kullanici: AdminUser, sonraki: UserStatus) => void;
}) {
  const askida = kullanici.status === 'SUSPENDED';

  return (
    <div className={sunum === 'tablo' ? 'flex flex-col items-end gap-2' : 'flex flex-col gap-2'}>
      <div className={sunum === 'tablo' ? 'flex flex-wrap justify-end gap-2' : 'flex flex-wrap gap-2'}>
        {askida ? (
          <Button size="sm" onClick={() => onSor(kullanici, 'ACTIVE')}>
            Aktifleştir
          </Button>
        ) : (
          <Button
            variant="danger"
            size="sm"
            disabled={benMi}
            onClick={() => onSor(kullanici, 'SUSPENDED')}
          >
            Askıya al
          </Button>
        )}

        <KimlikKopyala id={kullanici.id} />
      </div>

      {benMi && (
        <p className="text-sm text-muted">
          Bu sizin hesabınız — kendi durumunuzu değiştiremezsiniz.
        </p>
      )}
    </div>
  );
}

export default function AdminUsersPage() {
  const qc = useQueryClient();
  const { user: ben } = useSession();

  const [aramaGirdisi, setAramaGirdisi] = React.useState('');
  const [arama, setArama] = React.useState('');
  const [durum, setDurum] = React.useState<'' | UserStatus>('');
  const [offset, setOffset] = React.useState(0);

  // Her tuşa basışta istek atmak yönetim uçlarının dakikalık sınırını
  // (60 istek) tek bir aramada tüketir; arama 350 ms sonra sabitlenir.
  React.useEffect(() => {
    const t = setTimeout(() => {
      setArama(aramaGirdisi.trim());
      setOffset(0);
    }, 350);
    return () => clearTimeout(t);
  }, [aramaGirdisi]);

  const q = useQuery({
    queryKey: ['admin-users', { search: arama, status: durum, offset }],
    queryFn: () => {
      const p = new URLSearchParams({ limit: String(SAYFA_BOYUTU), offset: String(offset) });
      if (arama) p.set('q', arama);
      if (durum) p.set('status', durum);
      return apiFetch<AdminUserList>(`/admin/users?${p.toString()}`);
    },
    // Sayfa/arama değişince liste boşalıp zıplamasın.
    placeholderData: keepPreviousData,
  });

  const [onay, setOnay] = React.useState<{ kullanici: AdminUser; sonraki: UserStatus } | null>(null);

  const durumYaz = useMutation({
    mutationFn: (v: { id: string; sonraki: UserStatus }) =>
      apiFetch<SetStatusResult>(`/admin/users/${encodeURIComponent(v.id)}/status`, {
        method: 'PATCH',
        body: { status: v.sonraki },
      }),
    // Yönetsel bir durum değişikliği asla kendiliğinden tekrarlanmaz.
    retry: false,
    onSuccess: () => {
      // Yanıt yalnız {id,status} taşır; satırı yamalamak yerine listeyi tazele.
      qc.invalidateQueries({ queryKey: ['admin-users'] });
      setOnay(null);
    },
  });

  // `reset` React Query'de kararlıdır; `durumYaz` nesnesi her render'da yeniden
  // kurulur. Bağımlılığa nesneyi koymak `sutunlar` memo'sunu işlevsiz bırakırdı.
  const mutasyonuSifirla = durumYaz.reset;

  const sor = React.useCallback(
    (kullanici: AdminUser, sonraki: UserStatus) => {
      mutasyonuSifirla();
      setOnay({ kullanici, sonraki });
    },
    [mutasyonuSifirla],
  );

  const kapat = React.useCallback(() => {
    mutasyonuSifirla();
    setOnay(null);
  }, [mutasyonuSifirla]);

  function temizle() {
    setAramaGirdisi('');
    setArama('');
    setDurum('');
    setOffset(0);
  }

  const toplam = q.data?.total ?? 0;
  const satirlar = q.data?.items;
  const etkinSuzgec = (arama ? 1 : 0) + (durum ? 1 : 0);
  const sayfali = sayfalamaGorunur(toplam);

  /*
   * SÜTUNLAR — tek veri tanımı.
   *
   * Ölçüm: özgün dosyada aynı yedi alan İKİ KEZ yazılmıştı (mobil kart + tablo)
   * ve ikizler zaten ayrışmıştı: roller boşken kart "Rol atanmamış", tablo "—"
   * yazıyordu; "E-posta doğrulanmamış" uyarısı yalnız tabloda, "kendi hesabınız"
   * açıklaması yalnız kartta vardı. `baslik` ve `hucre` tek kaynak olduğu için
   * bu ayrışma bir daha doğamaz.
   *
   * ÖNCELİK (§6.1 kural 3): yedi sütun 768px'e sığmaz. "Sipariş" ve "Kayıt"
   * `oncelik: 3` ile `lg:` üstüne alındı; `md:`'de beş sütun kalır.
   */
  const sutunlar = React.useMemo<ReadonlyArray<Sutun<AdminUser>>>(
    () => [
      {
        anahtar: 'kullanici',
        baslik: 'Kullanıcı',
        mobilRol: 'baslik',
        hucre: (u) => (
          <>
            <p className="font-medium">{u.username}</p>
            <p className="mt-1 break-anywhere text-sm text-muted">{u.email}</p>
            {!u.emailVerified && (
              // Renk tek kanal değil: metin de "doğrulanmamış" diyor (§7.1).
              <p className="mt-1 text-sm text-[var(--color-warn)]">E-posta doğrulanmamış</p>
            )}
          </>
        ),
      },
      {
        anahtar: 'durum',
        baslik: 'Durum',
        mobilRol: 'rozet',
        hucre: (u) => (
          <DurumRozeti
            durum={u.status}
            etiket={DURUM_BILGISI[u.status].etiket}
            ton={DURUM_BILGISI[u.status].ton}
          />
        ),
      },
      {
        anahtar: 'bakiye',
        baslik: 'Bakiye',
        hizala: 'sag',
        sayisal: true,
        hucre: (u) => <span className="font-medium">{formatMoney(u.balance)}</span>,
      },
      {
        anahtar: 'siparis',
        baslik: 'Sipariş',
        hizala: 'sag',
        sayisal: true,
        oncelik: 3,
        hucre: (u) => u.orderCount,
      },
      {
        anahtar: 'roller',
        baslik: 'Roller',
        hucre: (u, sunum) =>
          u.roles.length ? (
            <div className={`flex flex-wrap gap-2 ${sunum === 'kart' ? 'justify-end' : ''}`}>
              {u.roles.map((r) => (
                <Badge key={r} tone="brand">
                  {r}
                </Badge>
              ))}
            </div>
          ) : (
            // Tek metin: kart da tablo da "Rol atanmamış" der (özgün ikizde "—" idi).
            <span className="font-normal text-muted">Rol atanmamış</span>
          ),
      },
      {
        anahtar: 'kayit',
        baslik: 'Kayıt',
        hizala: 'sag',
        sayisal: true,
        oncelik: 3,
        hucre: (u) => <span className="text-muted">{formatDateTime(u.createdAt)}</span>,
      },
      {
        anahtar: 'islem',
        baslik: 'İşlem',
        hizala: 'sag',
        mobilRol: 'eylem',
        hucre: (u, sunum) => (
          <SatirEylemleri kullanici={u} benMi={ben?.id === u.id} sunum={sunum} onSor={sor} />
        ),
      },
    ],
    [ben?.id, sor],
  );

  const askiyaAliniyor = onay?.sonraki === 'SUSPENDED';

  return (
    <div className="flex flex-col gap-6">
      <SayfaBasligi
        baslik="Kullanıcılar"
        aciklama="Hesapları arayın, durumlarını görün ve gerektiğinde erişimi kapatın."
      >
        {/* Satır değil SAYFA eylemi: hedef ekran kullanıcı kimliğini elle ister. */}
        <Link href="/yonetim/bakiye">
          <Button variant="outline">Bakiye düzelt</Button>
        </Link>
      </SayfaBasligi>

      <Card>
        <SuzgecCubugu etkinSayisi={etkinSuzgec} onTemizle={temizle}>
          {/*
            `sm:self-start`: `SuzgecCubugu` varsayılan olarak tabana hizalar
            (`items-end`). `Field` ipucu metnini kutunun ALTINA koyduğu için
            tabana hizalama arama kutusunu seçim kutusundan ~18px yukarı
            kaydırır. Tepeden hizalamak farkı 2px'e indirir (iki bileşenin
            etiket–kutu aralığı farklı: `Field` 6px, `Secim` 8px) — kalan fark
            katman raporunda bildirildi.
          */}
          <div className="w-full sm:w-72 sm:self-start">
            <Field
              label="Ara"
              type="search"
              value={aramaGirdisi}
              onChange={(e) => setAramaGirdisi(e.target.value)}
              placeholder="E-posta veya kullanıcı adı"
              autoCapitalize="none"
              autoCorrect="off"
              autoComplete="off"
              spellCheck={false}
              hint="Yazmayı bıraktığınızda arama kendiliğinden yapılır."
            />
          </div>

          <div className="w-full sm:w-56 sm:self-start">
            <Secim
              etiket="Durum"
              value={durum}
              onChange={(e) => {
                setDurum(e.target.value as '' | UserStatus);
                setOffset(0);
              }}
            >
              {DURUM_SUZGECLERI.map((s) => (
                <option key={s.value || 'tumu'} value={s.value}>
                  {s.label}
                </option>
              ))}
            </Secim>
          </div>
        </SuzgecCubugu>
      </Card>

      <Card>
        <div className="flex flex-wrap items-center justify-between gap-4">
          <h2 className="text-lg font-semibold">Hesap listesi</h2>
          <KayitSayaci toplam={toplam} />
        </div>

        <VeriTablosu
          className="mt-4"
          baslik="Kayıtlı hesaplar"
          sutunlar={sutunlar}
          satirlar={satirlar}
          satirAnahtari={(u) => u.id}
          yukleniyor={q.isLoading}
          hata={apiHatasi(q.error)}
          // Sayfalama kendi aralığını duyuruyor; iki canlı bölge aynı anda
          // konuşmasın (veri-tablosu.tsx `duyuru` notu).
          duyuru={!sayfali}
          bos={
            <Empty
              title="Kullanıcı bulunamadı"
              // §6.3: süzgeçten dolayı boş ≠ gerçekten boş.
              hint={
                etkinSuzgec > 0
                  ? 'Arama veya durum süzgecini değiştirip tekrar deneyin.'
                  : 'Henüz kayıtlı kullanıcı yok.'
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

      {/*
        ONAY DİYALOĞU — tek tıkla durum değiştirilmez.
        Askıya alma kullanıcının açık TÜM oturumlarını anında düşürür ve
        sipariş akışını keser; geri alınabilir ama kullanıcı tarafında anında
        görünür bir kesintidir.

        🔴 İLK ODAK ARTIK "VAZGEÇ"TE (§7.5): özgün dosya `data-autofocus`'u
        yıkıcı düğmeye koyuyordu. Bir önceki adımdan Enter basılı gelirse
        keydown tekrarı `click` üretir ve tek tuş hesabı askıya alırdı.
      */}
      <OnayDiyalogu
        acik={onay !== null}
        baslik={askiyaAliniyor ? 'Hesabı askıya al' : 'Hesabı aktifleştir'}
        yikici={askiyaAliniyor}
        onayMetni={askiyaAliniyor ? 'Evet, askıya al' : 'Evet, aktifleştir'}
        bekliyor={durumYaz.isPending}
        hata={apiHatasi(durumYaz.error)}
        onOnayla={() => onay && durumYaz.mutate({ id: onay.kullanici.id, sonraki: onay.sonraki })}
        onIptal={kapat}
        uyari={
          askiyaAliniyor ? (
            <>
              Askıya alınan hesabın <strong>tüm oturumları anında düşer</strong>. Kullanıcı giriş
              yapamaz, numara alamaz. Bakiyesi silinmez; hesabı yeniden aktifleştirdiğinizde kaldığı
              yerden devam eder.
            </>
          ) : (
            'Hesap yeniden aktifleştirilecek; kullanıcı giriş yapıp numara alabilecek.'
          )
        }
      >
        {onay && (
          // İÇ İÇE KART YOK (§9.2): bu blok `Modal` içindedir, `Card` içinde değil.
          <div className="raised rounded-xl border p-4">
            <p className="font-medium">{onay.kullanici.username}</p>
            <p className="mt-1 break-anywhere text-sm text-muted">{onay.kullanici.email}</p>
          </div>
        )}
      </OnayDiyalogu>
    </div>
  );
}
