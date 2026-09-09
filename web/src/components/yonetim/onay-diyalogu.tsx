'use client';

/**
 * OnayDiyalogu — yıkıcı ve para yazan işlemlerin onay adımı.
 *
 * KAPATTIĞI TEKRAR: ~5 ekranda elle kurulmuş onay adımı, İKİ FARKLI buton
 * düzeni (5 ekran `sm:flex-row-reverse`, 2 ekran düz `flex-col`) ve
 * `useDialogFocus` yardımcısı İKİ FARKLI İMZAYLA iki dosyada.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * 🔴 İLK ODAK "VAZGEÇ"TEDİR — bu bileşenin var olma sebebi
 * ══════════════════════════════════════════════════════════════════════════
 * Ölçüm, bugün ÇELİŞKİLİ: `talepler:588,668` ve `odeme-yontemleri:686` odağı
 * "Vazgeç"e koyuyor ve gerekçesini yazıyor; `kullanicilar:392`, `fiyatlar:721`
 * ve `fiyatlar:758` odağı YIKICI DÜĞMEYE koyuyor. Aynı risk, üç ekranda tersi
 * uygulanmış — hem de para ekranlarında.
 *
 * Doğrusu "Vazgeç"tir ve gerekçesi ölçülebilir: buraya KLAVYEYLE gelinir.
 * Kullanıcı bir önceki adımda Enter ile "Devam et"e basar. Enter BASILI
 * KALIRSA `click` olayı keydown tekrarıyla yeniden üretilir; odak "Evet"te
 * olsaydı basılı kalan TEK BİR TUŞ parayı yazardı. Onay ayrı ve bilinçli bir
 * hareket olmak zorundadır.
 *
 * Bu azınlık bir davranıştır (çoğu tasarım sistemi varsayılan eylemi odaklar)
 * ve bilerek seçilmiştir: burası bir para sistemidir (§7.5).
 *
 * ══════════════════════════════════════════════════════════════════════════
 * BUTON DÜZENİ — TEK düzen
 * ══════════════════════════════════════════════════════════════════════════
 * Mobilde alt alta (onay üstte, parmak menzilinde), `sm:` üstünde
 * `flex-row-reverse` ile onay SAĞDA. DOM sırası her iki kırılımda da
 * "onay → vazgeç"tir; odak `data-autofocus` ile Vazgeç'e verilir, DOM sırası
 * değiştirilerek DEĞİL — ekran okuyucu birincil eylemi önce duymalıdır.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * MODAL NE ZAMAN MEŞRU
 * ══════════════════════════════════════════════════════════════════════════
 * §6.4 modalı iki duruma indirir: (a) yıkıcı/geri alınamaz işlem onayı,
 * (b) korunmuş odak gerektiren çok adımlı form. Bu bileşen (a)'dır. Satır
 * düzenleme, durum değiştirme, hızlı not → SATIR İÇİ yapılır, buraya değil.
 */

import * as React from 'react';
import type { ApiError } from '@/lib/api';
import { Modal } from '@/components/modal';
import { Alert, Button } from '@/components/ui';
import { HataDurumu } from './hata-durumu';

export function OnayDiyalogu({
  acik,
  baslik,
  uyari,
  children,
  onayMetni,
  iptalMetni = 'Vazgeç',
  yikici = false,
  bekliyor = false,
  hata,
  onOnayla,
  onIptal,
}: {
  acik: boolean;
  /** Diyalog başlığı — `Modal` bunu `h2` + `aria-labelledby` olarak kullanır. */
  baslik: string;
  /**
   * Tek cümlelik sonuç uyarısı ("Bu işlem geri alınamaz."). Verilirse üstte
   * bir `Alert` olarak çıkar. Yıkıcı işlemde `warn`, diğerlerinde `info`.
   */
  uyari?: React.ReactNode;
  /** Onaylanacak şeyin ÖZETİ: kim, ne kadar, hangi hesap. */
  children?: React.ReactNode;
  /**
   * Onay düğmesinin metni. GENEL OLMAMALI: "Tamam" değil,
   * "Evet, bakiyeye yaz" — kullanıcı neyi onayladığını düğmede okur.
   */
  onayMetni: string;
  iptalMetni?: string;
  /** Yıkıcı/geri alınamaz mı? Onay düğmesini `danger` yapar. */
  yikici?: boolean;
  /** Mutasyon sürüyor. Onay `loading`, iptal `disabled` olur. */
  bekliyor?: boolean;
  /** Mutasyon hatası — diyalog KAPANMAZ, hata burada gösterilir. */
  hata?: ApiError | null;
  onOnayla: () => void;
  onIptal: () => void;
}) {
  return (
    // `Modal` odak tuzağını, Escape'i, iOS kaydırma kilidini ve kaydırma
    // konumunun geri yüklenmesini zaten yapıyor — burada tekrarlanmaz.
    <Modal open={acik} onClose={onIptal} title={baslik}>
      <div className="flex flex-col gap-4">
        {uyari && <Alert tone={yikici ? 'warn' : 'info'}>{uyari}</Alert>}

        {children}

        {hata && <HataDurumu hata={hata} />}

        <div className="flex flex-col gap-2 sm:flex-row-reverse sm:justify-start">
          <Button
            variant={yikici ? 'danger' : 'primary'}
            fullWidth
            loading={bekliyor}
            onClick={onOnayla}
            className="sm:w-auto"
          >
            {onayMetni}
          </Button>

          {/*
            🔴 `data-autofocus` BURADA. `modal.tsx` açılışta
            `[data-autofocus]` öğesini arar ve odaklar; bulamazsa panele
            odaklanır. Bu özniteliği onay düğmesine taşımak, dosya başındaki
            gerekçeyi çiğnemektir.
          */}
          <Button
            data-autofocus
            variant="outline"
            fullWidth
            disabled={bekliyor}
            onClick={onIptal}
            className="sm:w-auto"
          >
            {iptalMetni}
          </Button>
        </div>
      </div>
    </Modal>
  );
}
