export function Logo({ className = 'h-8' }: { className?: string }) {
  return (
    <span className={`inline-flex items-center gap-2 ${className}`}>
      <svg viewBox="0 0 32 32" className="h-full w-auto" aria-hidden>
        <rect width="32" height="32" rx="9" fill="#3454d1" />
        <path
          d="M9 12.5c0-1.4 1.5-2.5 3.6-2.5 1.7 0 3 .5 4 1.4M22.8 19.2c0 1.5-1.6 2.6-3.9 2.6-1.9 0-3.4-.6-4.4-1.6"
          stroke="#fff" strokeWidth="2.1" strokeLinecap="round" fill="none"
        />
        <path
          d="M12.6 15.9h6.6c1.9 0 3.4 1.1 3.4 2.6M19.2 15.9h-6.6c-2 0-3.6-1.1-3.6-2.6"
          stroke="#fff" strokeWidth="2.1" strokeLinecap="round" fill="none" opacity=".55"
        />
      </svg>
      <span className="text-[15px] font-semibold tracking-tight">SMS Onay</span>
    </span>
  );
}
