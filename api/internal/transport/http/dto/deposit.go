package dto

// Bakiye yükleme DTO'ları (FR-500 … FR-503).
//
// Dışa açık her kimlik public_id (UUID) stringidir; sayısal veritabanı
// kimliği hiçbir yanıtta yer almaz (değişmez #10).

import "strings"

// Tutar sınırları — deposits tablosundaki deposit_amount_range CHECK'i ile
// aynıdır. DTO'da KOŞULSUZ uygulanır: yönetici bir yöntemin üst sınırını
// tablonun ötesine çekerse, kullanıcı ham bir 500 değil alan hatası görür.
const (
	depositMinAmountMinor int64 = 1000    //     10,00 ₺
	depositMaxAmountMinor int64 = 5000000 // 50.000,00 ₺
)

/* ═══════════════════════ Kullanıcı: yöntemler ═══════════════════════ */

// DepositMethodPublicResponse kullanıcıya gösterilen ödeme yöntemi.
//
// Yönetim DTO'sundan (DepositMethodResponse) AYRI tutulur: burada `isActive`
// ve `missingFields` YOKTUR — kullanıcıya yalnız aktif yöntemler döner,
// dolayısıyla bu alanlar hem gereksiz hem de iç durumu sızdırır.
type DepositMethodPublicResponse struct {
	ID           string            `json:"id"`
	Code         string            `json:"code"`
	Kind         string            `json:"kind"` // BANK_TRANSFER | CRYPTO
	Name         string            `json:"name"`
	Instructions string            `json:"instructions"`
	Config       map[string]string `json:"config"`
	MinAmount    Money             `json:"minAmount"`
	// MaxAmount.Minor == 0 ise ÜST SINIR YOKTUR (0,00 ₺ tavan DEĞİL).
	MaxAmount Money `json:"maxAmount"`
	// ReferenceLabel kullanıcıya "referans alanına ne yazayım?" sorusunu
	// cevaplar; yöntem tipine göre değişir.
	ReferenceLabel string `json:"referenceLabel"`
	// ReceiptRequired dekont yüklemesi bekleniyor mu (havalede evet).
	ReceiptRequired bool `json:"receiptRequired"`
}

type DepositMethodListResponse struct {
	Items []DepositMethodPublicResponse `json:"items"`
}

/* ═══════════════════════ Kullanıcı: talepler ═══════════════════════ */

type CreateDepositRequest struct {
	MethodID string `json:"methodId"`
	// AmountMinor kuruş. Çıplak ondalık para KABUL EDİLMEZ (değişmez #1).
	AmountMinor int64  `json:"amountMinor"`
	Reference   string `json:"reference"`
	Note        string `json:"note"`
}

func (r *CreateDepositRequest) Validate() []FieldError {
	var errs []FieldError
	if strings.TrimSpace(r.MethodID) == "" {
		errs = append(errs, FieldError{"methodId", "Ödeme yöntemi seçilmelidir."})
	}
	if r.AmountMinor < depositMinAmountMinor || r.AmountMinor > depositMaxAmountMinor {
		errs = append(errs, FieldError{"amountMinor",
			"Tutar 10,00 ₺ ile 50.000,00 ₺ arasında olmalıdır."})
	}
	ref := strings.TrimSpace(r.Reference)
	if len(ref) < 4 {
		errs = append(errs, FieldError{"reference",
			"Havale açıklaması veya işlem numarası zorunludur (en az 4 karakter)."})
	} else if len(ref) > 120 {
		errs = append(errs, FieldError{"reference", "En fazla 120 karakter olabilir."})
	}
	if len(strings.TrimSpace(r.Note)) > 300 {
		errs = append(errs, FieldError{"note", "Not en fazla 300 karakter olabilir."})
	}
	return errs
}

// DepositResponse kullanıcıya dönen talep.
//
// 🔴 `adminNote` BU YAPIDA YOKTUR ve olmayacaktır: yöneticinin iç
// değerlendirmesi ("şüpheli hesap") kullanıcıya gösterilmez. Kullanıcının
// göreceği gerekçe `rejectionReason`'dır (FR-503).
//
// 🔴 `receiptPath` de YOKTUR: dosya yolu bir sırdır ve dışarıya verilirse
// "doğrudan URL ile servis edilemez" garantisi (KK-500) anlamını yitirir.
//
// test: deposit_integration_test.go#TestReceiptPathNeverLeaves
type DepositResponse struct {
	ID          string `json:"id"`
	Method      string `json:"method"`
	Amount      Money  `json:"amount"`
	Credited    Money  `json:"credited"`
	Status      string `json:"status"`
	StatusLabel string `json:"statusLabel"`
	Network     string `json:"network,omitempty"`
	Note        string `json:"note,omitempty"`
	// RejectionReason yalnız REJECTED taleplerde dolar.
	RejectionReason string `json:"rejectionReason,omitempty"`
	HasReceipt      bool   `json:"hasReceipt"`
	CreatedAt       string `json:"createdAt"`
	ReviewedAt      string `json:"reviewedAt,omitempty"`
}

type DepositListResponse struct {
	Items  []DepositResponse `json:"items"`
	Total  int64             `json:"total"`
	Limit  int32             `json:"limit"`
	Offset int32             `json:"offset"`
}

/* ═══════════════════════ Yönetim ═══════════════════════ */

// AdminDepositResponse yönetim listesindeki talep.
type AdminDepositResponse struct {
	ID     string `json:"id"`
	UserID string `json:"userId"` // users.public_id
	// UserEmail ve UserUsername yalnız yönetim yanıtındadır; log'a yazılmaz.
	UserEmail       string `json:"userEmail"`
	UserUsername    string `json:"userUsername"`
	Method          string `json:"method"`
	Amount          Money  `json:"amount"`
	Credited        Money  `json:"credited"`
	Status          string `json:"status"`
	StatusLabel     string `json:"statusLabel"`
	TxHash          string `json:"txHash,omitempty"`
	Network         string `json:"network,omitempty"`
	UserNote        string `json:"userNote,omitempty"`
	AdminNote       string `json:"adminNote,omitempty"`
	RejectionReason string `json:"rejectionReason,omitempty"`
	HasReceipt      bool   `json:"hasReceipt"`
	CreatedAt       string `json:"createdAt"`
	ReviewedAt      string `json:"reviewedAt,omitempty"`
}

type AdminDepositListResponse struct {
	Items  []AdminDepositResponse `json:"items"`
	Total  int64                  `json:"total"`
	Limit  int32                  `json:"limit"`
	Offset int32                  `json:"offset"`
}

// ApproveDepositRequest onay isteği.
//
// İdempotency anahtarı BURADA YOKTUR ve istemciden alınmaz: anahtar sunucuda
// talebin public_id'sinden türer ("deposit:<uuid>"). İstemci anahtarı
// kullanmak, iki yöneticinin aynı talebi farklı anahtarlarla onaylayıp
// bakiyeyi iki kez artırması demektir.
type ApproveDepositRequest struct {
	// CreditedMinor bakiyeye yazılacak tutar. 0 ise kullanıcının bildirdiği
	// tutar kullanılır (kripto ağ ücreti için yönetici farklı yazabilir).
	CreditedMinor int64  `json:"creditedMinor"`
	AdminNote     string `json:"adminNote"`
}

func (r *ApproveDepositRequest) Validate() []FieldError {
	var errs []FieldError
	if r.CreditedMinor != 0 &&
		(r.CreditedMinor < depositMinAmountMinor || r.CreditedMinor > depositMaxAmountMinor) {
		errs = append(errs, FieldError{"creditedMinor",
			"Yatan tutar 10,00 ₺ ile 50.000,00 ₺ arasında olmalıdır."})
	}
	if len(strings.TrimSpace(r.AdminNote)) > 300 {
		errs = append(errs, FieldError{"adminNote", "Not en fazla 300 karakter olabilir."})
	}
	return errs
}

// RejectDepositRequest red isteği.
type RejectDepositRequest struct {
	// Reason ZORUNLUDUR ve kullanıcıya gösterilir (FR-503).
	//
	// Veritabanında rejection_reason NOT NULL DEFAULT '' olduğu için bu
	// zorunluluğun TEK uygulayıcısı burasıdır ve servis katmanıdır.
	//
	// test: deposit_integration_test.go#TestRejectReasonRequired
	Reason    string `json:"reason"`
	AdminNote string `json:"adminNote"`
}

func (r *RejectDepositRequest) Validate() []FieldError {
	var errs []FieldError
	reason := strings.TrimSpace(r.Reason)
	if len(reason) < 5 {
		errs = append(errs, FieldError{"reason",
			"Red nedeni zorunludur (en az 5 karakter) — kullanıcıya gösterilir."})
	} else if len(reason) > 300 {
		errs = append(errs, FieldError{"reason", "Red nedeni en fazla 300 karakter olabilir."})
	}
	if len(strings.TrimSpace(r.AdminNote)) > 300 {
		errs = append(errs, FieldError{"adminNote", "Not en fazla 300 karakter olabilir."})
	}
	return errs
}

// DepositReviewResponse onay/red sonucu.
type DepositReviewResponse struct {
	Deposit AdminDepositResponse `json:"deposit"`
	// Balance TALEP SAHİBİNİN güncel bakiyesidir, yöneticinin değil.
	Balance Money `json:"balance"`
	// AlreadyApplied true ise bu istek bir TEKRAR'dı ve hiçbir şey yazılmadı.
	AlreadyApplied bool `json:"alreadyApplied,omitempty"`
}
