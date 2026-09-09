import Link from 'next/link';
import type { Metadata } from 'next';
import { Button, Card, Alert } from '@/components/ui';

export const metadata: Metadata = {
  title: 'İletişim',
  description:
    'Onay360 ile iletişime geçin. Hesabınız, siparişleriniz ve bakiyenizle ilgili ' +
    'sorularınız için panel üzerinden destek talebi açabilirsiniz.',
  alternates: { canonical: '/iletisim' },
};

/**
 * 🔴 BU SAYFADA UYDURMA İLETİŞİM BİLGİSİ YOKTUR.
 *
 * Var olmayan bir e-posta adresi ya da telefon numarası yazmak, en iyi
 * ihtimalle ulaşılamayan bir müşteriye, en kötü ihtimalle başkasına ait bir
 * numaraya trafik göndermek demektir. Bir para sisteminde "ulaşamadım"
 * demek "param kayboldu" demektir.
 *
 * ⬇️ İLETİŞİM KANALLARI YER TUTUCUDUR. Kanal gerçekten kurulduğunda
 * `deger` alanı doldurulmalıdır; dolu olan kanallar kendiliğinden listelenir,
 * boş olanlar hiç görünmez.
 *
 * `tur` alanı bağlantı biçimini belirler:
 *   'eposta' → mailto:  ·  'telefon' → tel:  ·  'metin' → düz metin
 */
type Kanal = { etiket: string; deger: string; tur: 'eposta' | 'telefon' | 'metin'; not?: string };

const KANALLAR: Kanal[] = [
  { etiket: 'E-posta', deger: '', tur: 'eposta' },
  { etiket: 'Telefon', deger: '', tur: 'telefon', not: 'Hafta içi mesai saatleri' },
  { etiket: 'Adres', deger: '', tur: 'metin' },
];

const dolu = KANALLAR.filter((k) => k.deger.length > 0);

function kanalBaglantisi(k: Kanal) {
  if (k.tur === 'eposta') return `mailto:${k.deger}`;
  // tel: bağlantısında boşluk ve parantez temizlenir; iOS aksi hâlde
  // numarayı çeviremez.
  if (k.tur === 'telefon') return `tel:${k.deger.replace(/[^\d+]/g, '')}`;
  return null;
}

export default function ContactPage() {
  return (
    <div className="px-4 py-10 md:px-6 md:py-14">
      <div className="mx-auto max-w-3xl">
        <h1 className="text-3xl font-bold tracking-tight md:text-4xl">İletişim</h1>
        <p className="mt-4 text-base leading-relaxed text-muted md:text-lg">
          Hesabınız, bir siparişiniz veya bakiyenizle ilgili bir sorun varsa en
          hızlı yol destek talebi açmaktır: talep hesabınıza bağlı olduğu için
          ilgili sipariş ve cüzdan hareketleri doğrudan görülür, kimlik
          doğrulamayla vakit kaybedilmez.
        </p>

        <h2 className="mt-10 text-xl font-semibold md:text-2xl">Destek talebi</h2>
        <Card className="mt-4">
          <p className="text-sm leading-relaxed text-muted">
            Panelinizdeki Destek bölümünden talep açabilir, yanıtları aynı yerden
            takip edebilirsiniz. Destek talebi açmak için giriş yapmış olmanız
            gerekir; hesabınız yoksa önce ücretsiz hesap açın.
          </p>
          <div className="mt-4 flex flex-col gap-3 sm:flex-row">
            {/* Hedef sayfa /panel/destek — panel kabuğundaki Destek sekmesiyle
                aynı yol. Oturum yoksa /panel/* zaten /giris'e yönlendirir. */}
            <Link href="/panel/destek" className="sm:w-auto">
              <Button fullWidth className="sm:w-auto sm:px-7">Destek talebi aç</Button>
            </Link>
            <Link href="/kayit" className="sm:w-auto">
              <Button variant="outline" fullWidth className="sm:w-auto sm:px-7">
                Ücretsiz hesap aç
              </Button>
            </Link>
          </div>
        </Card>

        <h2 className="mt-12 text-xl font-semibold md:text-2xl">Diğer kanallar</h2>
        {dolu.length > 0 ? (
          <Card className="mt-4">
            <dl className="flex flex-col gap-4 text-sm">
              {dolu.map((k) => {
                const href = kanalBaglantisi(k);
                return (
                  <div key={k.etiket} className="flex flex-col gap-0.5 md:flex-row md:gap-4">
                    <dt className="text-muted md:w-40 md:shrink-0">{k.etiket}</dt>
                    <dd className="break-anywhere">
                      {href ? (
                        <a
                          href={href}
                          className="inline-flex min-h-11 items-center text-brand-300
                                     underline underline-offset-4"
                        >
                          {k.deger}
                        </a>
                      ) : (
                        k.deger
                      )}
                      {k.not && <span className="ml-2 text-muted">({k.not})</span>}
                    </dd>
                  </div>
                );
              })}
            </dl>
          </Card>
        ) : (
          <Card className="mt-4">
            {/* Yayımlanmamış bir kanalı yayımlanmış gibi göstermek yerine
                durumu açıkça söylüyoruz — gizlilik/kullanim-sartlari
                sayfalarındaki desenin aynısı. */}
            <Alert tone="warn">
              E-posta, telefon ve adres bilgileri henüz yayımlanmadı. Şirket
              kuruluşu tamamlandığında burada yayımlanacaktır. O zamana kadar
              tüm başvurular panel üzerindeki destek talebi ile alınmaktadır.
            </Alert>
          </Card>
        )}

        <h2 className="mt-12 text-xl font-semibold md:text-2xl">Yazmadan önce</h2>
        <Card className="mt-4">
          <ul className="flex list-disc flex-col gap-2 pl-5 text-sm leading-relaxed text-muted">
            <li>
              Kod gelmediyse beklemeye devam edin: süre dolduğunda ücret
              bakiyenize otomatik iade edilir, talep açmanız gerekmez.
            </li>
            <li>
              Sorunuz bir siparişle ilgiliyse sipariş numarasını yazın; ekranda
              hata gördüyseniz yanındaki istek numarasını (requestId) da ekleyin.
            </li>
            <li>
              Sık karşılaşılan durumların yanıtları{' '}
              <Link href="/sss" className="text-brand-300 underline underline-offset-4">
                sık sorulan sorular
              </Link>{' '}
              sayfasında.
            </li>
            <li>
              Onay kodunuzu, şifrenizi veya SMS içeriğini kimseyle paylaşmayın;
              destek ekibi bunları sizden istemez.
            </li>
          </ul>
        </Card>
      </div>
    </div>
  );
}
