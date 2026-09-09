import 'server-only';
import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';

/**
 * Blog içerik katmanı — DOSYA TABANLI, veritabanı yok.
 *
 * NEDEN KÜTÜPHANE YOK
 * ───────────────────
 * `gray-matter` + `remark` + eklentileri birlikte ~1,5 MB'lık bir bağımlılık
 * ağacı getirir ve bu ağaç, üç yazılık bir bloğun ihtiyacından kat kat
 * büyüktür. Buradaki ayrıştırıcı BİZİM yazdığımız Markdown'ı okur — keyfi
 * kullanıcı girdisini değil — ve desteklediği söz dizimi bilinçli olarak
 * dardır: başlık, paragraf, liste, tablo, kalın, satır içi kod, bağlantı,
 * alıntı. Desteklenmeyen bir şey yazılırsa düz metin olarak çıkar; sessizce
 * bozulmaz.
 *
 * GÜVENLİK: girdi bize ait olsa da HTML ÖNCE KAÇIŞLANIR. Böylece bir gün
 * içerik başka bir yerden gelmeye başlarsa (CMS, kullanıcı katkısı) burada
 * bir XSS kapısı açılmış olmaz.
 *
 * Sayfalar `generateStaticParams` ile derleme anında üretilir; çalışma anında
 * dosya sistemine gidilmez.
 */

export type Yazi = {
  slug: string;
  baslik: string;
  ozet: string;
  etiket: string;
  renk: 'mavi' | 'mor' | 'yesil' | 'turuncu' | 'pembe' | 'deniz';
  /** RFC 3339 tarih (YYYY-MM-DD). Yazının GERÇEK yazıldığı gün. */
  tarih: string;
  okuma: number;
  govde: string;
};

const KLASOR = join(process.cwd(), 'content', 'blog');

/* ═══════════════════════ Ön bilgi (front matter) ═══════════════════════ */

function onBilgiAyir(ham: string): { alanlar: Record<string, string>; govde: string } {
  const m = /^---\r?\n([\s\S]*?)\r?\n---\r?\n?([\s\S]*)$/.exec(ham);
  if (!m) return { alanlar: {}, govde: ham };
  const alanlar: Record<string, string> = {};
  for (const satir of (m[1] ?? '').split(/\r?\n/)) {
    const i = satir.indexOf(':');
    if (i < 1) continue;
    alanlar[satir.slice(0, i).trim()] = satir.slice(i + 1).trim().replace(/^["']|["']$/g, '');
  }
  return { alanlar, govde: m[2] ?? '' };
}

/* ═══════════════════════ Markdown → HTML ═══════════════════════ */

const kacis = (s: string) =>
  s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');

/**
 * Satır içi biçimleme. Sıra ÖNEMLİ: önce kod (içindeki `*` biçimlenmesin),
 * sonra bağlantı, sonra kalın/italik.
 */
function satirIci(s: string): string {
  let x = kacis(s);
  x = x.replace(/`([^`]+)`/g,
    '<code class="rounded-md bg-[var(--raised)] px-1.5 py-0.5 text-[0.9em]">$1</code>');
  // Yalnız site içi (/ ile başlayan) ve https bağlantılar. `javascript:` gibi
  // bir şema hiçbir zaman href'e giremez.
  x = x.replace(/\[([^\]]+)\]\((\/[^)\s]*|https:\/\/[^)\s]+)\)/g,
    '<a href="$2" class="font-medium text-[var(--vurgu)] underline underline-offset-4 ' +
    'hover:text-[var(--vurgu-guclu)]">$1</a>');
  x = x.replace(/\*\*([^*]+)\*\*/g, '<strong class="font-semibold text-[var(--text)]">$1</strong>');
  x = x.replace(/(^|[^*])\*([^*\n]+)\*/g, '$1<em>$2</em>');
  return x;
}

const P = 'mt-4 text-[15px] leading-[1.75] text-muted';
const H2 = 'mt-10 text-xl font-bold tracking-tight text-[var(--text)] md:text-2xl';
const H3 = 'mt-7 text-lg font-semibold tracking-tight text-[var(--text)]';
const LI = 'text-[15px] leading-[1.75] text-muted';

/** Tablo satırını hücrelere böler; `| a | b |` → ['a','b']. */
const hucreler = (satir: string) =>
  satir.trim().replace(/^\|/, '').replace(/\|$/, '').split('|').map((h) => h.trim());

export function markdownaHtml(kaynak: string): string {
  const satirlar = kaynak.replace(/\r\n/g, '\n').split('\n');
  const cikti: string[] = [];
  let i = 0;
  /* `noUncheckedIndexedAccess` açık: her dizin erişimi `string | undefined`.
     Tek yerde normalize etmek, yirmi yerde `?? ''` yazmaktan okunaklı. */
  const st = (n: number): string => satirlar[n] ?? '';
  /** Listenin son öğesine devam satırı ekler. */
  const ekle = (dizi: string[], parca: string) => {
    if (dizi.length === 0) dizi.push(parca);
    else dizi[dizi.length - 1] = `${dizi[dizi.length - 1]} ${parca}`;
  };

  while (i < satirlar.length) {
    const satir = st(i);

    if (!satir.trim()) { i++; continue; }

    if (satir.startsWith('### ')) { cikti.push(`<h3 class="${H3}">${satirIci(satir.slice(4))}</h3>`); i++; continue; }
    if (satir.startsWith('## '))  { cikti.push(`<h2 class="${H2}">${satirIci(satir.slice(3))}</h2>`); i++; continue; }

    // Tablo: başlık satırı + ayraç satırı + gövde
    if (satir.trim().startsWith('|') && /^\s*\|[\s:|-]+\|\s*$/.test(st(i + 1))) {
      const bas = hucreler(satir);
      i += 2;
      const govde: string[][] = [];
      while (i < satirlar.length && st(i).trim().startsWith('|')) {
        govde.push(hucreler(st(i))); i++;
      }
      // 🔴 Mobilde yatay kaydırılan tablo yasak (frontend-contract §2.5) ama
      // BİR karşılaştırma tablosunu karta çevirmek anlamı bozar. Çözüm:
      // tablo KENDİ kutusunda kaydırılır, sayfa gövdesi kaymaz.
      cikti.push(
        `<div class="mt-6 -mx-4 overflow-x-auto px-4 md:mx-0 md:px-0"><table class="w-full min-w-[26rem] border-collapse text-sm">` +
        `<thead><tr>${bas.map((h) => `<th class="border-b border-[var(--border)] px-3 py-2.5 text-left font-semibold">${satirIci(h)}</th>`).join('')}</tr></thead>` +
        `<tbody>${govde.map((r) => `<tr>${r.map((c) => `<td class="border-b border-[var(--border)] px-3 py-2.5 align-top text-muted">${satirIci(c)}</td>`).join('')}</tr>`).join('')}</tbody>` +
        `</table></div>`,
      );
      continue;
    }

    // Sırasız liste
    if (/^[-*] /.test(satir)) {
      const ogeler: string[] = [];
      while (i < satirlar.length && (/^[-*] /.test(st(i)) || /^\s{2,}\S/.test(st(i)))) {
        if (/^[-*] /.test(st(i))) ogeler.push(st(i).slice(2));
        else ekle(ogeler, st(i).trim());   // devam satırı
        i++;
      }
      cikti.push(
        `<ul class="mt-4 flex list-none flex-col gap-2.5 pl-0">${ogeler.map((o) =>
          `<li class="${LI} relative pl-6 before:absolute before:left-1 before:top-[0.62em] ` +
          `before:size-1.5 before:rounded-full before:bg-[var(--vurgu)] before:content-['']">` +
          `${satirIci(o)}</li>`).join('')}</ul>`,
      );
      continue;
    }

    // Sıralı liste
    if (/^\d+\. /.test(satir)) {
      const ogeler: string[] = [];
      while (i < satirlar.length && (/^\d+\. /.test(st(i)) || /^\s{2,}\S/.test(st(i)))) {
        if (/^\d+\. /.test(st(i))) ogeler.push(st(i).replace(/^\d+\.\s+/, ''));
        else ekle(ogeler, st(i).trim());
        i++;
      }
      cikti.push(
        `<ol class="mt-4 flex list-none flex-col gap-2.5 pl-0">${ogeler.map((o, n) =>
          `<li class="${LI} relative pl-9">` +
          `<span class="absolute left-0 top-0 grid size-6 place-items-center rounded-lg ` +
          `bg-[var(--v-mavi-zemin)] text-xs font-bold text-[var(--v-mavi-metin)]">${n + 1}</span>` +
          `${satirIci(o)}</li>`).join('')}</ol>`,
      );
      continue;
    }

    // Alıntı
    if (satir.startsWith('> ')) {
      const parcalar: string[] = [];
      while (i < satirlar.length && st(i).startsWith('> ')) { parcalar.push(st(i).slice(2)); i++; }
      cikti.push(
        `<blockquote class="mt-6 rounded-r-xl border-l-4 border-[var(--vurgu)] ` +
        `bg-[var(--v-mavi-zemin)] px-4 py-3 text-[15px] leading-relaxed">` +
        `${satirIci(parcalar.join(' '))}</blockquote>`,
      );
      continue;
    }

    // Paragraf — boş satıra kadar
    const parcalar: string[] = [];
    while (i < satirlar.length && st(i).trim() &&
           !/^(#|[-*] |\d+\. |> |\|)/.test(st(i))) {
      parcalar.push(st(i).trim()); i++;
    }
    if (parcalar.length) cikti.push(`<p class="${P}">${satirIci(parcalar.join(' '))}</p>`);
  }

  return cikti.join('\n');
}

/* ═══════════════════════ Okuma ═══════════════════════ */

function oku(dosya: string): Yazi {
  const ham = readFileSync(join(KLASOR, dosya), 'utf8');
  const { alanlar, govde } = onBilgiAyir(ham);
  const slug = dosya.replace(/\.md$/, '');
  return {
    slug,
    baslik: alanlar.baslik ?? slug,
    ozet: alanlar.ozet ?? '',
    etiket: alanlar.etiket ?? 'Yazı',
    renk: (alanlar.renk as Yazi['renk']) ?? 'mavi',
    // Tarih ön bilgiden gelir; UYDURULMAZ. Alan boşsa tarih HİÇ gösterilmez —
    // yanlış bir tarih basmaktansa tarihsiz yazı yayımlamak yeğdir.
    tarih: alanlar.tarih ?? '',
    okuma: Number(alanlar.okuma) || 0,
    govde,
  };
}

/** Tüm yazılar, yeniden eskiye. Derleme anında bir kez okunur. */
export function yazilariGetir(): Yazi[] {
  let dosyalar: string[];
  try {
    dosyalar = readdirSync(KLASOR).filter((d) => d.endsWith('.md'));
  } catch {
    return [];   // klasör yoksa blog boş listelenir, sayfa 500 dönmez
  }
  return dosyalar.map(oku).sort((a, b) => (a.tarih < b.tarih ? 1 : -1));
}

export function yaziGetir(slug: string): Yazi | null {
  // Yol geçişi (`../../etc/passwd`) engellenir: slug yalnız güvenli
  // karakterlerden oluşabilir ve dosya listesiyle eşleşmek zorundadır.
  if (!/^[a-z0-9-]+$/.test(slug)) return null;
  return yazilariGetir().find((y) => y.slug === slug) ?? null;
}

/** Türkçe, açık ve saat dilimi SABİT tarih (frontend-contract §5.2). */
export function tarihBicimle(iso: string): string {
  if (!iso) return '';
  const d = new Date(`${iso}T00:00:00Z`);
  if (Number.isNaN(d.getTime())) return '';
  return new Intl.DateTimeFormat('tr-TR', {
    day: 'numeric', month: 'long', year: 'numeric', timeZone: 'Europe/Istanbul',
  }).format(d);
}
