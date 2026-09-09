/** Sunucu DTO'larının birebir karşılığı — api/internal/transport/http/dto. */

export interface Money { minor: number; currency: string; formatted: string }

export interface User {
  id: string; email: string; username: string;
  status: string; emailVerified: boolean;
  balance: Money; permissions: string[]; createdAt: string;
}

export interface Session {
  id: string; userAgent: string; ip?: string;
  createdAt: string; lastSeenAt: string; current: boolean;
}

export interface Service { code: string; name: string; iconUrl?: string }

/** Servis ızgarası özeti — /catalog/services-in-stock */
export interface ServiceSummary {
  code: string; name: string; iconUrl?: string; countryCount: number;
}
export interface Country { iso2: string; name: string; phoneCode: string }

export interface CatalogItem {
  serviceCode: string; serviceName: string; iconUrl?: string;
  countryIso2: string; countryName: string; phoneCode: string; inStock: boolean;
}

export interface Quote {
  quoteId: string; price: Money; stock: number;
  expiresAt: string; expiresIn: number;
}

export interface LedgerEntry {
  id: string; type: string; typeLabel: string;
  amount: Money; balanceAfter: Money;
  reference?: string; note?: string; createdAt: string;
}

export interface Statement {
  items: LedgerEntry[]; total: number; limit: number; offset: number;
}

/** Sipariş mesajı (SMS). */
export interface OrderMessage {
  code: string;
  body: string;
  sender?: string;
  receivedAt: string;
}

/** Sipariş — /orders yanıtı. */
export interface Order {
  id: string;
  status: string;
  phoneNumber: string;
  serviceCode: string;
  serviceName: string;
  /** Servis logosu — CANLI katalogdan gelir, sipariş kaydında saklanmaz.
   *  Boşsa `ServiceIcon` harf rozetine düşer (dto/auth.go:334). */
  iconUrl?: string;
  countryIso2: string;
  countryName: string;
  phoneCode?: string;
  price: Money;
  expiresAt: string;
  /** Sunucunun hesapladığı kalan saniye — istemci kendi saatiyle hesaplamaz. */
  expiresIn: number;
  cancellableAt: string;
  cancellableIn: number;
  createdAt: string;
  messages: OrderMessage[];
}

export interface OrderList {
  items: Order[];
  total: number;
  limit: number;
  offset: number;
}

/** Kiralık ızgarası için servis özeti. */
export interface RentalService {
  code: string;
  name: string;
  iconUrl?: string;
  countryCount: number;
  durationCount: number;
}

/** Kiralanabilir süre. Fiyat BURADA YOKTUR — teklif ile verilir. */
export interface RentalDuration {
  minutes: number;
  hours: number;
  days: number;
  label: string;
  inStock: boolean;
}

/* ═══════════════════ Yönetim paneli ═══════════════════ */
/*
 * Bu tipler Go tarafındaki `json:` etiketlerinden BİREBİR türetilmiştir
 * (api/internal/transport/http/dto/). Uyuşmayan bir alan adı TypeScript'te
 * hata vermez — çalışma zamanında sessizce `undefined` gelir. Uç değişirse
 * BURASI da değişmeli.
 */

export interface AdminUser {
  id: string;               // public_id (UUID) — sayısal kimlik DIŞARI VERİLMEZ
  email: string;
  username: string;
  status: 'PENDING_VERIFICATION' | 'ACTIVE' | 'SUSPENDED';
  emailVerified: boolean;
  balance: Money;
  roles: string[];
  orderCount: number;
  createdAt: string;
}

export interface AdminProvider {
  id: string;
  name: string;
  protocol: string;
  baseUrl: string;
  isActive: boolean;
  priority: number;
  /** Anahtarın KENDİSİ değil, VARLIĞI. Şifreli hâli bile dönmez. */
  hasApiKey: boolean;
  costMultiplier: string;
  capabilities: string[];
  balance: Money;
  sync?: { running: boolean; startedAt?: string; finishedAt?: string; failed?: boolean };
}

/** Yönetim listesi: isActive/sortOrder/missingFields taşır. */
export interface DepositMethod {
  id: string;
  code: string;
  kind: 'BANK_TRANSFER' | 'CRYPTO';
  name: string;
  instructions: string;
  config: Record<string, string>;
  minAmount: Money;
  maxAmount: Money;
  isActive: boolean;
  sortOrder: number;
  /** Aktifleştirmeyi engelleyen boş alanlar. */
  missingFields?: string[];
}

/** Kullanıcı tarafı: yalnız AKTİF yöntemler, isActive alanı YOK. */
export interface PublicDepositMethod {
  id: string;
  code: string;
  kind: 'BANK_TRANSFER' | 'CRYPTO';
  name: string;
  instructions: string;
  config: Record<string, string>;
  minAmount: Money;
  maxAmount: Money;
  referenceLabel: string;
  receiptRequired: boolean;
}

export type DepositStatus = 'PENDING' | 'COMPLETED' | 'REJECTED' | 'REFUNDED';

/** Kullanıcının kendi talebi. */
export interface Deposit {
  id: string;
  method: string;
  amount: Money;
  credited: Money;
  status: DepositStatus;
  statusLabel: string;
  network?: string;
  note?: string;
  rejectionReason?: string;
  hasReceipt: boolean;
  createdAt: string;
  reviewedAt?: string;
}

/** Yönetim listesi — kullanıcı bilgisi ve yönetici notu EK olarak gelir. */
export interface AdminDeposit {
  id: string;
  userId: string;
  userEmail: string;
  userUsername: string;
  method: string;
  amount: Money;
  credited: Money;
  status: DepositStatus;
  statusLabel: string;
  txHash?: string;
  network?: string;
  userNote?: string;
  adminNote?: string;
  rejectionReason?: string;
  hasReceipt: boolean;
  createdAt: string;
  reviewedAt?: string;
}

export type PricingScope = 'GLOBAL' | 'COUNTRY' | 'SERVICE' | 'SERVICE_COUNTRY' | 'PRODUCT';

export interface PricingRule {
  id: number;
  scope: PricingScope;
  serviceCode?: string;
  countryIso?: string;
  durationMinutes?: number;
  marginPercent: string;
  fixedFee: Money;
  minPrice: Money;
  note: string;
  createdAt: string;
  /** Yeni kural aynı kapsamdaki eskisini devreden çıkardıysa true. */
  replaced?: boolean;
}

export interface PricingPreview {
  sellPrice: Money;
  cost: Money;
  costInTry: Money;
  fxRate: string;
  fxFetchedAt: string;
  providerName: string;
  stock: number;
  inStock: boolean;
  ruleSource: string;
  ruleScope?: string;
  marginPercent: string;
  hitMinimum: boolean;
  /** CACHE: önbellekteki maliyet. Satın almada canlı maliyet sorulur. */
  costSource: 'CACHE' | 'LIVE';
}

export interface AuditLog {
  /** int64 — JSON'da SAYI gelir (dto/admin.go). string yazmak sessiz hatadır. */
  id: number;
  action: string;
  entityType: string;
  entityId: string;
  actorId?: string;
  actorUsername?: string;
  before?: Record<string, unknown>;
  after?: Record<string, unknown>;
  /** Süzülen (gizlenen) alan adları — denetçi bir şeyin saklandığını görmeli. */
  redactedFields?: string[];
  ip?: string;
  requestId?: string;
  createdAt: string;
}
