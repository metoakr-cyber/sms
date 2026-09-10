package wallet

import (
	"time"

	"github.com/ikmetrik/sms-platform/api/internal/db"
)

// StatementFilter hareket dökümü filtresi.
type StatementFilter struct {
	Type db.LedgerType
	// Q serbest metin araması (not + referans kimliği). NIL ise aranmaz;
	// boş dize "hiçbir şeyle eşleşme" değil, "arama yok" demektir.
	Q      *string
	From   *time.Time
	To     *time.Time
	Limit  int32
	Offset int32
}

// Normalize sınırları güvenli değerlere çeker.
func (f *StatementFilter) Normalize() {
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 20
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
}
