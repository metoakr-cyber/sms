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
