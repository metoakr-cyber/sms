package ticket

import (
	"net/http"

	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
)

// Destek talebi hata kataloğu (FR-600).
//
// Kullanıcı ASLA ham veritabanı hatası görmez (değişmez #12): her biri Türkçe
// bir metin ve açık bir HTTP kodu taşır.
//
// test: internal/transport/http/handler/ticket_integration_test.go#TestUserCannotSeeOthersTicket
var (
	// ErrNotFound talep yok VEYA başkasına ait.
	//
	// İKİSİ AYNI HATAYA DÜŞER, bilerek: "bu talep başkasının" demek geçerli
	// talep kimliklerinin varlığını sızdırır ve numaralandırma saldırısına
	// kapı açar (ErrQuoteNotFound ile aynı gerekçe).
	ErrNotFound = apperr.NewStatus(apperr.KindDomain, "TICKET_NOT_FOUND",
		"Destek talebi bulunamadı.", http.StatusNotFound)

	// ErrClosed kapalı talebe mesaj yazma denemesi.
	//
	// Kullanıcı için bu bir çıkmaz sokak değildir: metin ne yapması
	// gerektiğini söyler. Kararın gerekçesi domain/ticket/ticket.go
	// AfterUserMessage yorumundadır.
	ErrClosed = apperr.NewStatus(apperr.KindDomain, "TICKET_CLOSED",
		"Bu talep kapatılmış. Lütfen yeni bir destek talebi açın.",
		http.StatusConflict)

	// ErrInvalidStatus yöneticinin yazmaya çalıştığı durum elle yazılamaz.
	ErrInvalidStatus = apperr.NewStatus(apperr.KindDomain, "TICKET_INVALID_STATUS",
		"Bu durum elle atanamaz.", http.StatusUnprocessableEntity)
)
