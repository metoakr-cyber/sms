// Package ticket destek talebi kullanım senaryolarını yürütür (FR-600).
//
// İki akış vardır ve ikisi de bu pakettedir:
//
//	kullanıcı → talep aç (konu + öncelik + ilk mesaj), yanıt yaz, oku
//	personel  → listele, yanıtla, kapat / yeniden aç
//
// Bu paket dış HTTP çağrısı YAPMAZ ve para hareketi üretmez. Mesaj yazma TEK
// TRANSACTION'dır: satır kilidi → durum kararı (domain/ticket) → mesaj →
// durum yazımı. Kilitsiz yazmak, aynı talebe iki mesaj aynı anda geldiğinde
// durumun yalnız birine göre hesaplanmasına yol açardı.
package ticket

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/ikmetrik/sms-platform/api/internal/db"
	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
	ticketdom "github.com/ikmetrik/sms-platform/api/internal/domain/ticket"
	"github.com/ikmetrik/sms-platform/api/internal/port"
)

type txRunner interface {
	Queries() *db.Queries
	InTx(ctx context.Context, fn func(*db.Queries) error) error
}

// Deps servis bağımlılıkları.
type Deps struct {
	TxRunner txRunner
	Clock    port.Clock
}

// Service destek talebi servisi.
type Service struct {
	tx    txRunner
	clock port.Clock
}

func New(d Deps) *Service { return &Service{tx: d.TxRunner, clock: d.Clock} }

/* ═══════════════════════ Görünüm yapıları ═══════════════════════ */

// Detail bir talebin tam görünümü: başlık satırı + yazışma.
//
// Mesajlar İKİ AYRI SORGUDAN gelebilir (kullanıcı / yönetim) ve alanları
// farklıdır; ortak yapı burada birleştirilir. AuthorUsername YALNIZ yönetim
// yolunda dolar — kullanıcı sorgusu o sütunu hiç seçmez.
type Message struct {
	PublicID       uuid.UUID
	IsStaff        bool
	Body           string
	CreatedAt      time.Time // biçimleme (RFC 3339) DTO katmanında yapılır
	AuthorUsername string
}

/* ═══════════════════════ Kullanıcı akışı ═══════════════════════ */

// CreateInput yeni talep.
type CreateInput struct {
	UserID   int64
	Subject  string
	Priority ticketdom.Priority
	Body     string
}

// Create talebi ve İLK MESAJI tek transaction'da yazar.
//
// İkisi ayrı yazılsaydı, mesaj yazımı düştüğünde ortada konusu olan ama
// içeriği olmayan bir talep kalırdı; yönetici neyin sorulduğunu göremezdi.
func (s *Service) Create(ctx context.Context, in CreateInput) (db.Ticket, error) {
	subject := strings.TrimSpace(in.Subject)
	body := strings.TrimSpace(in.Body)
	priority := in.Priority
	if !priority.Valid() {
		priority = ticketdom.PriorityNormal
	}

	var out db.Ticket
	err := s.tx.InTx(ctx, func(q *db.Queries) error {
		t, err := q.CreateTicket(ctx, db.CreateTicketParams{
			UserID:   in.UserID,
			Subject:  subject,
			Priority: db.TicketPriority(priority),
		})
		if err != nil {
			return err
		}
		userID := in.UserID
		if _, err := q.CreateTicketMessage(ctx, db.CreateTicketMessageParams{
			TicketID: t.ID,
			UserID:   &userID,
			// İlk mesajı her zaman kullanıcı yazar.
			IsStaff: false,
			Body:    body,
		}); err != nil {
			return err
		}
		out = t
		return nil
	})
	if err != nil {
		return db.Ticket{}, apperr.Internal(err)
	}
	return out, nil
}

// List kullanıcının KENDİ taleplerini döner.
// `status` NIL ise durum süzgeci UYGULANMAZ. Sayım listeyle AYNI süzgeci alır.
//
// 🔴 `userID` süzgeçten AYRI parametredir ve sorguya her zaman girer
// (değişmez #7); hiçbir `status` değeri başkasının talebini döndüremez.
func (s *Service) List(
	ctx context.Context, userID int64, status *ticketdom.Status, q *string, limit, offset int32,
) (
	[]db.ListUserTicketsRow, int64, error,
) {
	// Alan adı dönüşümü `AdminList` ile aynı: domain tipi veritabanı enum'una
	// SERVİSTE çevrilir, transport katmanı `db` tipini hiç tanımaz.
	var filter *db.TicketStatus
	if status != nil {
		v := db.TicketStatus(*status)
		filter = &v
	}
	qs := s.tx.Queries()
	rows, err := qs.ListUserTickets(ctx, db.ListUserTicketsParams{
		UserID: userID, Status: filter, Q: q, Lim: limit, Off: offset,
	})
	if err != nil {
		return nil, 0, apperr.Internal(err)
	}
	total, err := qs.CountUserTickets(ctx, db.CountUserTicketsParams{
		UserID: userID, Status: filter, Q: q,
	})
	if err != nil {
		return nil, 0, apperr.Internal(err)
	}
	return rows, total, nil
}

// Get kullanıcının KENDİ talebini ve yazışmasını döner.
//
// Sahiplik sorgunun parçasıdır (değişmez #7); burada ayrı bir if yoktur.
// Başkasının talebi ile var olmayan talep AYNI yanıtı (404) alır.
//
// test: internal/transport/http/handler/ticket_integration_test.go#TestUserCannotSeeOthersTicket
func (s *Service) Get(ctx context.Context, userID int64, publicID uuid.UUID) (
	db.Ticket, []Message, error,
) {
	q := s.tx.Queries()
	t, err := q.GetTicketForUser(ctx, db.GetTicketForUserParams{
		PublicID: publicID, UserID: userID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.Ticket{}, nil, ErrNotFound
		}
		return db.Ticket{}, nil, apperr.Internal(err)
	}
	rows, err := q.ListTicketMessagesForUser(ctx, t.ID)
	if err != nil {
		return db.Ticket{}, nil, apperr.Internal(err)
	}
	msgs := make([]Message, 0, len(rows))
	for _, m := range rows {
		msgs = append(msgs, Message{
			PublicID: m.PublicID, IsStaff: m.IsStaff, Body: m.Body,
			CreatedAt: m.CreatedAt,
			// AuthorUsername BİLEREK BOŞ: personelin kimliği kullanıcıya
			// gösterilmez (sorgu zaten o sütunu seçmiyor).
		})
	}
	return t, msgs, nil
}

// AddMessage kullanıcı yanıtı ekler.
//
// Durum kararı domain/ticket'a aittir: ANSWERED → USER_REPLIED, kapalı talep
// reddedilir. Karar burada tekrar YAZILMAZ, çağrılır (değişmez #13).
//
// test: internal/service/ticket/ticket_integration_test.go#TestUserReplyReopensAnsweredTicket
func (s *Service) AddMessage(ctx context.Context, userID int64, publicID uuid.UUID, body string) (
	db.Ticket, error,
) {
	return s.appendMessage(ctx, appendInput{
		PublicID: publicID,
		AuthorID: userID,
		IsStaff:  false,
		Body:     body,
		lock: func(ctx context.Context, q *db.Queries) (db.Ticket, error) {
			// SAHİPLİK SORGUNUN PARÇASI: başkasının talebini kilitlemek bile
			// mümkün değil.
			return q.LockTicketForUser(ctx, db.LockTicketForUserParams{
				PublicID: publicID, UserID: userID,
			})
		},
		next: ticketdom.AfterUserMessage,
	})
}

/* ═══════════════════════ Yönetim akışı ═══════════════════════ */

// AdminFilter yönetim listesinin süzgeci.
//
// Status ve OnlyPending AYRI alanlardır: "bekleyen" TEK BİR DURUM DEĞİLDİR
// (OPEN + USER_REPLIED). Tek alana sıkıştırmak, kullanıcının yanıt yazdığı
// talepleri varsayılan ekranda görünmez yapardı.
type AdminFilter struct {
	Status *ticketdom.Status
	// OnlyPending true ise yalnız personel müdahalesi bekleyen talepler.
	OnlyPending bool
	// Q serbest metin: talep konusu + kullanıcı e-postası/adı. NIL ise
	// aranmaz; boş dize "hiçbir şeyle eşleşme" değil, "arama yok" demektir.
	Q *string
}

// AdminList süzgeçle tüm talepleri döner.
//
// Sahiplik kısıtı YOKTUR; koruma izin ara katmanındadır (tickets:read).
func (s *Service) AdminList(ctx context.Context, f AdminFilter, limit, offset int32) (
	[]db.ListTicketsForAdminRow, int64, error,
) {
	var filter *db.TicketStatus
	if f.Status != nil {
		v := db.TicketStatus(*f.Status)
		filter = &v
	}
	q := s.tx.Queries()
	rows, err := q.ListTicketsForAdmin(ctx, db.ListTicketsForAdminParams{
		Status: filter, OnlyPending: f.OnlyPending, Q: f.Q, Lim: limit, Off: offset,
	})
	if err != nil {
		return nil, 0, apperr.Internal(err)
	}
	// Sayım listeyle AYNI süzgeci alır (queries/tickets.sql notu).
	total, err := q.CountTicketsForAdmin(ctx, db.CountTicketsForAdminParams{
		Status: filter, OnlyPending: f.OnlyPending, Q: f.Q,
	})
	if err != nil {
		return nil, 0, apperr.Internal(err)
	}
	return rows, total, nil
}

// AdminGet talebi, sahibini ve yazışmayı döner.
func (s *Service) AdminGet(ctx context.Context, publicID uuid.UUID) (
	db.GetTicketForAdminRow, []Message, error,
) {
	q := s.tx.Queries()
	t, err := q.GetTicketForAdmin(ctx, publicID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.GetTicketForAdminRow{}, nil, ErrNotFound
		}
		return db.GetTicketForAdminRow{}, nil, apperr.Internal(err)
	}
	rows, err := q.ListTicketMessagesForAdmin(ctx, t.ID)
	if err != nil {
		return db.GetTicketForAdminRow{}, nil, apperr.Internal(err)
	}
	msgs := make([]Message, 0, len(rows))
	for _, m := range rows {
		msgs = append(msgs, Message{
			PublicID: m.PublicID, IsStaff: m.IsStaff, Body: m.Body,
			CreatedAt: m.CreatedAt, AuthorUsername: m.AuthorUsername,
		})
	}
	return t, msgs, nil
}

// AdminAddMessage personel yanıtı ekler.
//
// 🔴 is_staff = true BURADA verilir. KK-600'ün tek uygulayıcısı bu satır ve
// sorgu parametresidir; sütunun varsayılanı yoktur ki unutulan bir değer
// sessizce "kullanıcı yazdı" olmasın.
//
// test: internal/service/ticket/ticket_integration_test.go#TestStaffFlagIsRecorded
func (s *Service) AdminAddMessage(ctx context.Context, staffUserID int64, publicID uuid.UUID, body string) (
	db.Ticket, error,
) {
	return s.appendMessage(ctx, appendInput{
		PublicID: publicID,
		AuthorID: staffUserID,
		IsStaff:  true,
		Body:     body,
		lock: func(ctx context.Context, q *db.Queries) (db.Ticket, error) {
			return q.LockTicket(ctx, publicID)
		},
		next: ticketdom.AfterStaffMessage,
	})
}

// AdminSetStatus talebi kapatır ya da yeniden açar.
//
// Yalnız domain'in izin verdiği hedefler yazılabilir (OPEN / CLOSED):
// ANSWERED ve USER_REPLIED bir mesajın sonucudur, elle atanamaz.
//
// test: internal/transport/http/handler/ticket_integration_test.go#TestAdminCannotSetAnsweredByHand
func (s *Service) AdminSetStatus(ctx context.Context, publicID uuid.UUID, target ticketdom.Status) (
	db.Ticket, error,
) {
	if !ticketdom.AdminCanSet(target) {
		return db.Ticket{}, ErrInvalidStatus
	}

	var out db.Ticket
	err := s.tx.InTx(ctx, func(q *db.Queries) error {
		cur, err := q.LockTicket(ctx, publicID)
		if err != nil {
			return err
		}
		if err := ticketdom.Transition(ticketdom.Status(cur.Status), target); err != nil {
			return ErrInvalidStatus
		}

		params := db.SetTicketStatusParams{
			PublicID: publicID,
			Status:   db.TicketStatus(target),
		}
		if target == ticketdom.StatusClosed {
			// closed_at, status ile AYNI ifadede yazılır: ticket_closed_has_time
			// CHECK'i iki adımlı yazımda düşer.
			now := s.clock.Now()
			params.ClosedAt = &now
		}
		// Yeniden açmada ClosedAt nil kalır ve sütun NULL'a döner.

		out, err = q.SetTicketStatus(ctx, params)
		return err
	})
	if err != nil {
		return db.Ticket{}, mapTxErr(err)
	}
	return out, nil
}

/* ═══════════════════════ Ortak yazma yolu ═══════════════════════ */

type appendInput struct {
	PublicID uuid.UUID
	AuthorID int64
	IsStaff  bool
	Body     string
	// lock talebi kilitler. Kullanıcı yolunda sahiplik SORGUNUN parçasıdır.
	lock func(context.Context, *db.Queries) (db.Ticket, error)
	// next yeni durumu hesaplayan domain fonksiyonu.
	next func(ticketdom.Status) (ticketdom.Status, error)
}

// appendMessage kullanıcı ve personel yollarının ORTAK gövdesidir.
//
// İki ayrı kopya yazmak, birinde kilidi ya da durum hesabını unutmayı bir
// kopyala-yapıştır kadar kolaylaştırırdı; fark yalnız üç parametrede.
func (s *Service) appendMessage(ctx context.Context, in appendInput) (db.Ticket, error) {
	body := strings.TrimSpace(in.Body)

	var out db.Ticket
	err := s.tx.InTx(ctx, func(q *db.Queries) error {
		cur, err := in.lock(ctx, q)
		if err != nil {
			return err
		}

		next, err := in.next(ticketdom.Status(cur.Status))
		if err != nil {
			return err
		}

		author := in.AuthorID
		if _, err := q.CreateTicketMessage(ctx, db.CreateTicketMessageParams{
			TicketID: cur.ID,
			UserID:   &author,
			IsStaff:  in.IsStaff,
			Body:     body,
		}); err != nil {
			return err
		}

		out, err = q.TouchTicketAfterMessage(ctx, db.TouchTicketAfterMessageParams{
			ID:          cur.ID,
			Status:      db.TicketStatus(next),
			LastReplyAt: s.clock.Now(),
		})
		return err
	})
	if err != nil {
		return db.Ticket{}, mapTxErr(err)
	}
	return out, nil
}

// mapTxErr transaction içinden çıkan hataları kullanıcıya gösterilebilir
// hâle çevirir. Tipli hatalar (ErrNotFound, ErrClosed…) olduğu gibi geçer.
func mapTxErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, pgx.ErrNoRows):
		// Kilit sıfır satır döndüyse: talep yok VEYA başkasının.
		return ErrNotFound
	case errors.Is(err, ticketdom.ErrClosed):
		return ErrClosed
	}
	if _, ok := apperr.As(err); ok {
		return err
	}
	return apperr.Internal(err)
}
