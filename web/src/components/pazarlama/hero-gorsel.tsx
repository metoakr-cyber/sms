/**
 * Ana sayfa kahraman görseli — telefonunda onay kodunu gören kadın.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * NEDEN BİLEŞEN, NEDEN `<img>` DEĞİL
 * ══════════════════════════════════════════════════════════════════════════
 * Görselin zemini ve yüzen kartları TEMAYA GÖRE DEĞİŞMEK ZORUNDA: açık temada
 * beyaz kart + buz mavisi daire, koyu temada kart yüzeyi + koyu lacivert daire.
 * `<img src="...svg">` ile dosya sayfanın CSS'ini göremez; koyu temada açık
 * mavi daire ekranda parlak bir leke olarak kalırdı. Satır içi SVG olarak
 * gömülünce `var(--...)` token'ları normal şekilde çözülür ve görsel tema
 * düğmesini kendiliğinden takip eder.
 *
 * Ek fayda: ayrı bir ağ isteği yok ve ikonlar `currentColor` yerine ölçülmüş
 * vurgu ailelerini kullanabiliyor.
 *
 * ══════════════════════════════════════════════════════════════════════════
 * SABİT KALAN RENKLER
 * ══════════════════════════════════════════════════════════════════════════
 * Figürün kendisi (ten, saç, kıyafet) ve TELEFON EKRANI iki temada da aynıdır.
 * Telefon ekranı gerçek bir cihaz ekranıdır; sayfanın teması onu değiştirmez.
 * Marka mavisi `#1a7fd4` logodan gelir (globals.css `--color-onay-500`).
 *
 * ══════════════════════════════════════════════════════════════════════════
 * ERİŞİLEBİLİRLİK
 * ══════════════════════════════════════════════════════════════════════════
 * `role="img"` + `<title>`/`<desc>`: ekran okuyucu görseli tek bir öğe olarak
 * duyurur, içindeki yüzlerce yolu tek tek gezmez. Kimlikler `hg-` öneklidir —
 * sayfada başka SVG'ler de var ve `id` çakışması gradyanların yanlış
 * tanımı almasına yol açar.
 */

type Props = { className?: string };

export function HeroGorsel({ className }: Props) {
  return (
    <svg
      viewBox="0 0 560 640"
      className={className}
      role="img"
      aria-labelledby="hg-bas hg-ack"
    >
      <title id="hg-bas">Telefonunda SMS onay kodunu gören kadın</title>
      <desc id="hg-ack">
        Mavi bluzlu bir kadın elindeki telefona bakıyor; ekranda gelen SMS
        bildirimi ve dört haneli onay kodu görünüyor.
      </desc>

      <defs>
        <linearGradient id="hg-bluz" x1="0" y1="0" x2="0.35" y2="1">
          <stop offset="0" stopColor="#2f92e0" />
          <stop offset="1" stopColor="#1a7fd4" />
        </linearGradient>
        <linearGradient id="hg-kol" x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stopColor="#2a8ade" />
          <stop offset="1" stopColor="#1673c2" />
        </linearGradient>
        <linearGradient id="hg-etek" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0" stopColor="#2b45b0" />
          <stop offset="1" stopColor="#1f3180" />
        </linearGradient>
        <linearGradient id="hg-sac" x1="0.1" y1="0" x2="0.9" y2="1">
          <stop offset="0" stopColor="#463a56" />
          <stop offset="1" stopColor="#2a2236" />
        </linearGradient>
        <linearGradient id="hg-ten" x1="0" y1="0" x2="0.6" y2="1">
          <stop offset="0" stopColor="#f5c8a4" />
          <stop offset="1" stopColor="#e9b48e" />
        </linearGradient>
        <filter id="hg-golge" x="-40%" y="-40%" width="180%" height="180%">
          <feDropShadow dx="0" dy="6" stdDeviation="9" floodColor="#123a63" floodOpacity="0.13" />
        </filter>
        <filter id="hg-golge-yum" x="-40%" y="-40%" width="180%" height="180%">
          <feDropShadow dx="0" dy="12" stdDeviation="16" floodColor="#0e2c4d" floodOpacity="0.2" />
        </filter>
        <clipPath id="hg-ekran">
          <rect x="223" y="316" width="106" height="188" rx="13" />
        </clipPath>
        {/*
          Pantolon çerçevenin alt kenarında bitiyor. Maskesiz bırakılınca bu,
          sayfanın ortasında yatay bir kesik olarak görünüyor — figür görünmez
          bir rafın üstünde duruyormuş gibi. Alfa geçişi kesiği eritir ve
          sayfa zemininin rengi ne olursa olsun çalışır (maske saydamlık
          üretir, zemin rengi boyamaz).
        */}
        <linearGradient id="hg-solma" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0" stopColor="#ffffff" />
          <stop offset="1" stopColor="#000000" />
        </linearGradient>
        <mask id="hg-alt-solma">
          <rect x="0" y="0" width="560" height="548" fill="#ffffff" />
          <rect x="0" y="548" width="560" height="92" fill="url(#hg-solma)" />
        </mask>
      </defs>

      {/* ═══════════ Zemin ═══════════ */}
      <circle cx="285" cy="312" r="236" fill="var(--v-mavi-zemin)" />
      <g fill="var(--vurgu)" opacity="0.35">
        <circle cx="452" cy="128" r="4" /><circle cx="486" cy="160" r="4" />
        <circle cx="452" cy="192" r="4" /><circle cx="486" cy="224" r="4" />
        <circle cx="418" cy="160" r="4" /><circle cx="486" cy="96" r="4" />
        <circle cx="86" cy="392" r="4" /><circle cx="120" cy="424" r="4" />
        <circle cx="86" cy="456" r="4" /><circle cx="120" cy="360" r="4" />
        <circle cx="54" cy="424" r="4" />
      </g>
      <circle
        cx="285" cy="312" r="236" fill="none"
        stroke="var(--vurgu)" strokeWidth="2" strokeDasharray="3 9" opacity="0.3"
      />

      {/* ═══════════ Yüzen kart: kod doğrulandı ═══════════ */}
      <g filter="url(#hg-golge)">
        <rect x="34" y="152" width="196" height="66" rx="20" fill="var(--surface)" />
        <circle cx="70" cy="185" r="19" fill="var(--v-yesil-zemin)" />
        <path
          d="M62 185.5l6 6 11-12.5" fill="none" stroke="var(--v-yesil-metin)"
          strokeWidth="3.4" strokeLinecap="round" strokeLinejoin="round"
        />
        <rect x="100" y="173" width="96" height="9" rx="4.5" fill="var(--border)" />
        <rect x="100" y="190" width="62" height="8" rx="4" fill="var(--border)" opacity="0.6" />
      </g>

      {/* ═══════════ Yüzen kart: süre ═══════════ */}
      <g filter="url(#hg-golge)">
        <rect x="376" y="214" width="150" height="56" rx="18" fill="var(--surface)" />
        <circle cx="408" cy="242" r="17" fill="var(--v-mavi-zemin)" />
        <circle cx="408" cy="242" r="11" fill="none" stroke="var(--v-mavi-metin)" strokeWidth="2.6" />
        <path
          d="M408 234v9l6 4" fill="none" stroke="var(--v-mavi-metin)"
          strokeWidth="3" strokeLinecap="round" strokeLinejoin="round"
        />
        <rect x="434" y="232" width="72" height="8" rx="4" fill="var(--border)" />
        <rect x="434" y="247" width="46" height="7" rx="3.5" fill="var(--border)" opacity="0.6" />
      </g>

      {/* ═══════════ FİGÜR ═══════════ */}
      {/* Saç · arka */}
      <path
        d="M285 62c-47 0-80 35-80 86v76c0 17-5 30-14 41h49c-7-13-9-28-9-43v-74c0-31 22-52
           54-52s54 21 54 52v74c0 15-2 30-9 43h49c-9-11-14-24-14-41v-76c0-51-33-86-80-86z"
        fill="url(#hg-sac)"
      />

      {/* Boyun */}
      <path d="M262 206h46v42c0 12 6 21 16 26h-78c10-5 16-14 16-26z" fill="url(#hg-ten)" />
      <path d="M262 206h46v20c-9 9-20 14-32 14-5 0-10-1-14-2z" fill="#d79f79" />

      {/* Gövde: bluz */}
      <path
        d="M262 242c-26 4-49 15-59 32-14 22-20 66-23 112-2 42-3 78-2 106h214c1-28 0-64-2-106-3-46-9-90-23-112-10-17-33-28-59-32-4 17-42 17-46 0z"
        fill="url(#hg-bluz)"
      />
      <path
        d="M262 242c5 19 14 34 23 40 9-6 18-21 23-40-8 13-16 19-23 19s-15-6-23-19z"
        fill="url(#hg-ten)"
      />
      <path
        d="M226 302c-7 36-11 82-11 124 0 26 1 46 2 62" fill="none"
        stroke="#1470bd" strokeWidth="3" strokeLinecap="round" opacity="0.5"
      />
      <path
        d="M344 302c7 36 11 82 11 124 0 26-1 46-2 62" fill="none"
        stroke="#1470bd" strokeWidth="3" strokeLinecap="round" opacity="0.5"
      />

      {/* Pantolon — alt kenarda eritilir, bkz. `hg-alt-solma` */}
      <g mask="url(#hg-alt-solma)">
        <path d="M191 528h188c6 36 10 74 12 112H179c2-38 6-76 12-112z" fill="url(#hg-etek)" />
        <path d="M285 528v112" fill="none" stroke="#18276b" strokeWidth="3" opacity="0.55" />
      </g>

      {/* Kollar: sol el telefonu tutar, sağ kol yanda serbest */}
      <path
        d="M356 288c19 26 27 62 26 96" fill="none"
        stroke="url(#hg-kol)" strokeWidth="44" strokeLinecap="round"
      />
      <path
        d="M382 384c1 36-2 70-8 100" fill="none"
        stroke="url(#hg-ten)" strokeWidth="31" strokeLinecap="round"
      />
      <g transform="rotate(9 372 506)">
        <rect x="353" y="482" width="38" height="48" rx="18" fill="#efbd98" />
        <path
          d="M362 496v26M372 494v28M382 496v26" fill="none"
          stroke="#dda57f" strokeWidth="2" strokeLinecap="round" opacity="0.6"
        />
      </g>
      <path
        d="M214 288c-19 26-28 62-26 98" fill="none"
        stroke="url(#hg-kol)" strokeWidth="44" strokeLinecap="round"
      />
      <path
        d="M188 386c3 42 16 78 38 104" fill="none"
        stroke="url(#hg-ten)" strokeWidth="31" strokeLinecap="round"
      />

      {/* ═══════════ Baş ═══════════ */}
      <g transform="rotate(-7 285 175)">
        <circle cx="234" cy="184" r="10" fill="#e9b48e" />
        <circle cx="336" cy="184" r="10" fill="#e9b48e" />
        <ellipse cx="285" cy="172" rx="50" ry="58" fill="url(#hg-ten)" />
        <path
          d="M285 230c-15 0-28-6-38-17 9 17 22 27 38 27s29-10 38-27c-10 11-23 17-38 17z"
          fill="#dfa77f" opacity="0.5"
        />
        <ellipse cx="250" cy="191" rx="12" ry="7.5" fill="#ec9f86" opacity="0.42" />
        <ellipse cx="320" cy="191" rx="12" ry="7.5" fill="#ec9f86" opacity="0.42" />
        <path d="M255 153c7-6 16-6 23-1" fill="none" stroke="#3a2f47" strokeWidth="4.2" strokeLinecap="round" />
        <path d="M292 152c7-5 16-5 23 1" fill="none" stroke="#3a2f47" strokeWidth="4.2" strokeLinecap="round" />
        <path d="M256 173c5 7 15 7 20 0" fill="none" stroke="#2f2a3f" strokeWidth="4.8" strokeLinecap="round" />
        <path d="M294 173c5 7 15 7 20 0" fill="none" stroke="#2f2a3f" strokeWidth="4.8" strokeLinecap="round" />
        <path d="M285 179c3 6 4 11 1 14" fill="none" stroke="#d99a72" strokeWidth="3.2" strokeLinecap="round" />
        <path d="M271 201c9 9 19 9 28 0" fill="none" stroke="#c47a5c" strokeWidth="4.2" strokeLinecap="round" />
        {/* Saç · ön: tepeyi tamamen kapatır, alında boşluk kalmaz */}
        <path
          d="M285 96c-32 0-53 24-53 62 0 6 1 12 2 17 3-16 8-27 16-33 14 10 39 12 60 4 10-4
             17-9 22-15 6 8 10 21 12 44 2-6 3-13 3-21 0-38-30-58-62-58z"
          fill="url(#hg-sac)"
        />
        <circle cx="234" cy="200" r="5.5" fill="#1a7fd4" />
        <circle cx="336" cy="200" r="5.5" fill="#1a7fd4" />
      </g>

      {/* ═══════════ TELEFON — ekran iki temada da aynı ═══════════ */}
      <g transform="rotate(-6 268 402)" filter="url(#hg-golge-yum)">
        <rect x="211" y="304" width="130" height="212" rx="24" fill="#16233b" />
        <rect x="219" y="312" width="114" height="196" rx="17" fill="#ffffff" />
        <rect x="256" y="320" width="40" height="7" rx="3.5" fill="#16233b" />
        <g clipPath="url(#hg-ekran)">
          <rect x="235" y="338" width="34" height="9" rx="4.5" fill="#cfe3f8" />
          <rect x="233" y="352" width="86" height="34" rx="10" fill="#e9f8f0" stroke="#c3e9d4" strokeWidth="1.5" />
          <circle cx="249" cy="369" r="8" fill="#17c666" />
          <path
            d="M245.5 369l2.6 2.6 4.6-5.2" fill="none" stroke="#ffffff"
            strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"
          />
          <rect x="263" y="362" width="46" height="6" rx="3" fill="#a9dcc1" />
          <rect x="263" y="373" width="30" height="5" rx="2.5" fill="#c6e8d6" />
          <rect x="235" y="396" width="58" height="6" rx="3" fill="#dbe6f2" />
          <rect x="234" y="410" width="22" height="30" rx="7" fill="#f2f7fd" stroke="#1a7fd4" strokeWidth="1.8" />
          <rect x="260" y="410" width="22" height="30" rx="7" fill="#f2f7fd" stroke="#1a7fd4" strokeWidth="1.8" />
          <rect x="286" y="410" width="22" height="30" rx="7" fill="#f2f7fd" stroke="#1a7fd4" strokeWidth="1.8" />
          <rect x="312" y="410" width="22" height="30" rx="7" fill="#f2f7fd" stroke="#1a7fd4" strokeWidth="1.8" />
          <g
            fill="#0e5690" fontSize="17" fontWeight="700" textAnchor="middle"
            fontFamily="ui-sans-serif, system-ui, -apple-system, 'Segoe UI', Roboto, Arial, sans-serif"
          >
            <text x="245" y="432">4</text>
            <text x="271" y="432">8</text>
            <text x="297" y="432">2</text>
            <text x="323" y="432">9</text>
          </g>
          <rect x="234" y="456" width="100" height="26" rx="13" fill="#1a7fd4" />
          <rect x="262" y="466" width="44" height="7" rx="3.5" fill="#ffffff" opacity="0.92" />
        </g>
      </g>

      {/* Telefonu saran parmaklar — telefonun ÜSTÜNDE çizilir */}
      <g transform="rotate(-6 268 402)">
        <rect x="200" y="446" width="34" height="16" rx="8" fill="#f2c39e" />
        <rect x="200" y="467" width="31" height="16" rx="8" fill="#eeba94" />
        <rect x="200" y="488" width="28" height="15" rx="7.5" fill="#eab68f" />
        <rect
          x="228" y="468" width="17" height="46" rx="8.5"
          fill="#f5c8a4" transform="rotate(17 236 491)"
        />
      </g>

      {/* ═══════════ Ön rozet ═══════════ */}
      <g filter="url(#hg-golge)">
        <rect x="52" y="452" width="72" height="72" rx="24" fill="var(--surface)" />
        <path
          d="M74 476h28a4 4 0 014 4v16a4 4 0 01-4 4h-16l-9 8v-8h-3a4 4 0 01-4-4v-16a4 4 0 014-4z"
          fill="var(--v-mavi-metin)"
        />
        <circle cx="82" cy="488" r="2.6" fill="var(--v-mavi-zemin)" />
        <circle cx="90" cy="488" r="2.6" fill="var(--v-mavi-zemin)" />
        <circle cx="98" cy="488" r="2.6" fill="var(--v-mavi-zemin)" />
      </g>
    </svg>
  );
}
