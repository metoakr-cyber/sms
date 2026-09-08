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
