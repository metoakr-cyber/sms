/**
 * Onay360 markası.
 *
 * Bu SVG, gerçek logonun YERİNE GEÇEN bir vektör sürümdür — asıl logo dosyası
 * `public/marka/onay360.svg` olarak konduğunda `<BrandMark useFile />` ile
 * kullanılabilir. Buradaki sürüm marka renklerini ve 360° dairesel ok +
 * onay işareti motifini taşır; piksel birebir kopya değildir.
 *
 * Renkler logodan alındı: mavi #1a7fd4, kırmızı #e30613.
 */
export function Logo({
  className = 'h-8', withTagline = false,
}: { className?: string; withTagline?: boolean }) {
  return (
    <span className={`inline-flex items-center gap-2.5 ${className}`}>
      <svg viewBox="0 0 40 40" className="h-full w-auto shrink-0" aria-hidden>
        {/* dairesel ok — "360" döngüsü */}
        <path
          d="M20 5.5a14.5 14.5 0 1 0 13.4 9"
          fill="none" stroke="#1a7fd4" strokeWidth="3.4" strokeLinecap="round"
        />
        <path d="M28.8 4.2l5.6 3.4-4.2 4.6z" fill="#1a7fd4" />
        {/* onay işareti */}
        <path
          d="M12.5 20.5l5.4 5.6L31 10.8"
          fill="none" stroke="#e30613" strokeWidth="4.2"
          strokeLinecap="round" strokeLinejoin="round"
        />
      </svg>

      <span className="flex flex-col leading-none">
        <span className="text-[17px] font-bold tracking-tight">
          <span style={{ color: '#1a7fd4' }}>Onay</span>
          <span style={{ color: '#e30613' }}>360</span>
          <span style={{ color: '#e30613' }} className="align-super text-[10px]">°</span>
        </span>
        {withTagline && (
          <span className="mt-1 text-[9px] font-medium uppercase tracking-[0.08em] text-muted">
            Geçici numara ve onaylama hizmetleri
          </span>
        )}
      </span>
    </span>
  );
}
