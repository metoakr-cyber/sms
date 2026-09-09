import net from 'node:net';

/**
 * Küçük RESP istemcisi — YALNIZ hız limiti sayaçlarını sıfırlamak için.
 *
 * NEDEN VAR:
 * `/auth` uçları IP başına dakikada 30 istekle sınırlı (router.go `authLimit`).
 * Bu sınır kaba kuvvete karşıdır ve DOĞRUDUR. Ama uçtan uca takım tek bir
 * makineden, saniyeler içinde onlarca kayıt/giriş yapar; sınır o zaman ÜRÜNDE
 * HATA YOKKEN testleri düşürür. Bu, en kötü tür kararsızlıktır: kırmızı
 * doğruyu söylemez ve bir süre sonra kimse bakmaz.
 *
 * NEDEN GÜVENLİ:
 * `scripts/e2e.sh` Redis'te AYRI bir veritabanı (varsayılan 15) kullanır ve
 * yalnız `rl:` önekli anahtarlar silinir. Oturumlar (`sess:`/`user:`) ve
 * webhook kuyruğu aynı veritabanında yaşar — bu yüzden FLUSHDB YAPILMAZ;
 * yapılsaydı testler koşarken kendi oturumlarını düşürürlerdi.
 *
 * Bir bağımlılık (`ioredis`) eklemek yerine 80 satır yazıldı: `web/` paketi
 * ön yüz paketidir, ona test için sunucu tarafı bir istemci eklemek üretim
 * bağımlılık denetimini (npm audit --omit=dev) gereksizce genişletirdi.
 */

type Yanit = string | number | null | Yanit[];

interface Adres {
  host: string;
  port: number;
  db: number;
  password: string;
}

export function redisAdresi(url: string): Adres {
  const u = new URL(url);
  const yol = u.pathname.replace(/^\//, '');
  return {
    host: u.hostname || '127.0.0.1',
    port: Number(u.port || 6379),
    db: yol === '' ? 0 : Number(yol),
    password: decodeURIComponent(u.password || ''),
  };
}

function komut(...args: string[]): Buffer {
  let s = `*${args.length}\r\n`;
  for (const a of args) s += `$${Buffer.byteLength(a, 'utf8')}\r\n${a}\r\n`;
  return Buffer.from(s, 'utf8');
}

/** Tamponun başındaki tek bir RESP değerini çözer; eksikse null döner. */
function coz(buf: Buffer): { deger: Yanit; uzunluk: number } | null {
  const son = buf.indexOf('\r\n');
  if (son < 0) return null;
  const tip = buf[0];
  const govde = buf.subarray(1, son).toString('utf8');
  const basUzunluk = son + 2;

  switch (tip) {
    case 0x2b: // '+' basit dize
      return { deger: govde, uzunluk: basUzunluk };
    case 0x2d: // '-' hata
      throw new Error(`redis: ${govde}`);
    case 0x3a: // ':' tamsayı
      return { deger: Number(govde), uzunluk: basUzunluk };
    case 0x24: {
      // '$' toplu dize
      const n = Number(govde);
      if (n === -1) return { deger: null, uzunluk: basUzunluk };
      if (buf.length < basUzunluk + n + 2) return null;
      return {
        deger: buf.subarray(basUzunluk, basUzunluk + n).toString('utf8'),
        uzunluk: basUzunluk + n + 2,
      };
    }
    case 0x2a: {
      // '*' dizi
      const n = Number(govde);
      if (n === -1) return { deger: null, uzunluk: basUzunluk };
      const ogeler: Yanit[] = [];
      let ofset = basUzunluk;
      for (let i = 0; i < n; i++) {
        const oge = coz(buf.subarray(ofset));
        if (oge === null) return null;
        ogeler.push(oge.deger);
        ofset += oge.uzunluk;
      }
      return { deger: ogeler, uzunluk: ofset };
    }
    default:
      throw new Error(`redis: anlaşılmayan yanıt tipi: ${String.fromCharCode(tip ?? 0)}`);
  }
}

/**
 * `rl:` önekli hız limiti sayaçlarını siler.
 *
 * Anahtar sayısı test koşusunda birkaç düzinedir; `KEYS` bu ölçekte
 * sorunsuzdur ve `SCAN` döngüsünden çok daha az koda mal olur. Üretimde
 * çalışan bir kod değildir.
 */
export async function hizLimitiSifirla(url: string): Promise<void> {
  const adres = redisAdresi(url);

  await new Promise<void>((tamam, hata) => {
    const sock = net.createConnection({ host: adres.host, port: adres.port });
    sock.setTimeout(5000);

    let tampon = Buffer.alloc(0);
    const kuyruk: Array<(y: Yanit) => void> = [];
    let hataOldu: Error | null = null;

    const bitir = (e: Error | null) => {
      if (hataOldu) return;
      hataOldu = e;
      sock.destroy();
      e ? hata(e) : tamam();
    };

    const gonder = (...args: string[]): Promise<Yanit> =>
      new Promise((cozumle) => {
        kuyruk.push(cozumle);
        sock.write(komut(...args));
      });

    sock.on('data', (parca) => {
      tampon = Buffer.concat([tampon, parca]);
      try {
        for (;;) {
          const sonuc = coz(tampon);
          if (sonuc === null) break;
          tampon = tampon.subarray(sonuc.uzunluk);
          kuyruk.shift()?.(sonuc.deger);
        }
      } catch (e) {
        bitir(e instanceof Error ? e : new Error(String(e)));
      }
    });

    sock.on('error', (e) => bitir(e));
    sock.on('timeout', () => bitir(new Error('redis: zaman aşımı')));

    sock.on('connect', () => {
      void (async () => {
        try {
          if (adres.password) await gonder('AUTH', adres.password);
          if (adres.db !== 0) await gonder('SELECT', String(adres.db));
          const anahtarlar = await gonder('KEYS', 'rl:*');
          if (Array.isArray(anahtarlar) && anahtarlar.length > 0) {
            await gonder('DEL', ...anahtarlar.map(String));
          }
          bitir(null);
        } catch (e) {
          bitir(e instanceof Error ? e : new Error(String(e)));
        }
      })();
    });
  });
}
