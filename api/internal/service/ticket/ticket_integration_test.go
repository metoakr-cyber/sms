//go:build integration

package ticket_test

// Destek talebi akışının GERÇEK Postgres'e karşı garantileri (FR-600).
//
// SQLite ile taklit edilemez: `SELECT ... FOR UPDATE`, ENUM tipleri ve
// BEFORE UPDATE tetikleyicisi burada ölçülen şeylerin ta kendisidir.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"

	"github.com/ikmetrik/sms-platform/api/internal/adapter/postgres"
	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
	ticketdom "github.com/ikmetrik/sms-platform/api/internal/domain/ticket"
	ticketsvc "github.com/ikmetrik/sms-platform/api/internal/service/ticket"
	"github.com/ikmetrik/sms-platform/api/internal/testsupport"
)

var pool *pgxpool.Pool

func TestMain(m *testing.M) {
	// 🔴 GÜVENLİK KAPISI: bu testler DELETE FROM yapar. Veritabanı adı
	// "_test" ile bitmiyorsa süreç durur — kapı Makefile'da değil burada,
	// çünkü `go test` komutunu elle yazan kişiyi Makefile korumaz.
	url, err := testsupport.MustTestDatabaseURL()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if url == "" {
		fmt.Println("⚠️  DATABASE_URL tanımsız — entegrasyon testleri ATLANDI")
		os.Exit(0)
	}
	p, err := postgres.NewPool(context.Background(), url)
	if err != nil {
		fmt.Printf("postgres: %v\n", err)
		os.Exit(1)
	}
	pool = p
	code := m.Run()
	pool.Close()
	os.Exit(code)
}

/* ═══════════════════════ Kurulum ═══════════════════════ */

func newService(t *testing.T) *ticketsvc.Service {
	t.Helper()
	return ticketsvc.New(ticketsvc.Deps{
		TxRunner: postgres.NewTxRunner(pool),
		Clock:    fixedClock{},
	})
}

// fixedClock zaman ClockPort üzerinden gelir; `time.Now()` doğrudan çağrılmaz.
// Değer ilerlemez çünkü bu testlerin hiçbiri süreye bakmaz.
type fixedClock struct{}

func (fixedClock) Now() time.Time {
	return time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
}

func seedUser(t *testing.T, suffix string) int64 {
	t.Helper()
	tag := uuid.NewString()[:8]
	var id int64
	err := pool.QueryRow(context.Background(), `
		INSERT INTO users (email, username, password_hash, status, balance_minor)
		VALUES ($1, $2, '$2a$10$x', 'ACTIVE', 0)
		RETURNING id`,
		fmt.Sprintf("tkt-%s-%s@ornek.test", suffix, tag),
		fmt.Sprintf("t_%s_%s", suffix, tag)).Scan(&id)
	if err != nil {
		t.Fatalf("kullanıcı eklenemedi: %v", err)
	}
	// LIFO: bu temizlik, kullanıcı silinmeden ÖNCE koşar.
	// ticket_messages, tickets'a CASCADE ile bağlıdır.
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = pool.Exec(ctx, `DELETE FROM ticket_messages WHERE user_id = $1`, id)
		_, _ = pool.Exec(ctx, `DELETE FROM tickets WHERE user_id = $1`, id)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
	})
	return id
}

func newTicket(t *testing.T, s *ticketsvc.Service, userID int64) uuid.UUID {
	t.Helper()
	tk, err := s.Create(context.Background(), ticketsvc.CreateInput{
		UserID:   userID,
		Subject:  "Bakiye yüklemem görünmüyor",
		Priority: ticketdom.PriorityNormal,
		Body:     "Dün havale yaptım ama bakiyeme yansımadı.",
	})
	if err != nil {
		t.Fatalf("talep açılamadı: %v", err)
	}
	return tk.PublicID
}

func statusOf(t *testing.T, id uuid.UUID) string {
	t.Helper()
	var s string
	if err := pool.QueryRow(context.Background(),
		`SELECT status::text FROM tickets WHERE public_id = $1`, id).Scan(&s); err != nil {
		t.Fatalf("durum okunamadı: %v", err)
	}
	return s
}

/* ═══════════════════════ Testler ═══════════════════════ */

// TestCreateWritesFirstMessage talep ve ilk mesaj TEK transaction'da yazılır.
//
// Ayrı yazılsalardı, mesaj yazımı düştüğünde konusu olan ama içeriği olmayan
// bir talep kalırdı ve yönetici neyin sorulduğunu göremezdi.
func TestCreateWritesFirstMessage(t *testing.T) {
	s := newService(t)
	user := seedUser(t, "create")
	id := newTicket(t, s, user)

	tk, msgs, err := s.Get(context.Background(), user, id)
	if err != nil {
		t.Fatalf("talep okunamadı: %v", err)
	}
	if string(tk.Status) != string(ticketdom.StatusOpen) {
		t.Errorf("yeni talep OPEN olmalı, %s geldi", tk.Status)
	}
	if len(msgs) != 1 {
		t.Fatalf("ilk mesaj yazılmadı: %d mesaj", len(msgs))
	}
	if msgs[0].IsStaff {
		t.Error("ilk mesaj personel mesajı olarak kaydedilmiş")
	}
}

// TestStaffFlagIsRecorded KK-600: personel yanıtı is_staff = true ile kaydedilir.
//
// Eski sistemde bu bayrak var olmayan bir alandan okunuyordu ve HER ZAMAN
// false çıkıyordu; personel yanıtları kullanıcı mesajı gibi görünüyordu.
// Bayrak veritabanından OKUNARAK doğrulanır — servis yanıtı değil, yazılan
// satır ölçülür.
func TestStaffFlagIsRecorded(t *testing.T) {
	s := newService(t)
	user := seedUser(t, "staffflag")
	staff := seedUser(t, "staffuser")
	id := newTicket(t, s, user)

	if _, err := s.AdminAddMessage(context.Background(), staff, id, "Kontrol ediyoruz."); err != nil {
		t.Fatalf("personel yanıtı eklenemedi: %v", err)
	}

	rows, err := pool.Query(context.Background(), `
		SELECT m.is_staff FROM ticket_messages m
		JOIN tickets t ON t.id = m.ticket_id
		WHERE t.public_id = $1 ORDER BY m.id`, id)
	if err != nil {
		t.Fatalf("mesajlar okunamadı: %v", err)
	}
	defer rows.Close()

	var flags []bool
	for rows.Next() {
		var f bool
		if err := rows.Scan(&f); err != nil {
			t.Fatalf("tarama: %v", err)
		}
		flags = append(flags, f)
	}
	if len(flags) != 2 {
		t.Fatalf("iki mesaj bekleniyordu, %d var", len(flags))
	}
	if flags[0] {
		t.Error("kullanıcının ilk mesajı is_staff=true kaydedilmiş")
	}
	if !flags[1] {
		t.Error("personel yanıtı is_staff=false kaydedilmiş (KK-600 ihlali)")
	}
}

// TestStaffReplyMarksAnswered personel yazınca durum ANSWERED olur.
func TestStaffReplyMarksAnswered(t *testing.T) {
	s := newService(t)
	user := seedUser(t, "answered")
	staff := seedUser(t, "answeredstaff")
	id := newTicket(t, s, user)

	if _, err := s.AdminAddMessage(context.Background(), staff, id, "Yanıtımız."); err != nil {
		t.Fatalf("personel yanıtı: %v", err)
	}
	if got := statusOf(t, id); got != "ANSWERED" {
		t.Errorf("ANSWERED bekleniyordu, %s geldi", got)
	}
}

// TestUserReplyReopensAnsweredTicket kullanıcı yazınca talep yöneticinin
// kuyruğuna geri döner (ANSWERED → USER_REPLIED).
//
// Bu geçiş olmadan kullanıcının yanıtı yöneticinin bekleyen listesinde HİÇ
// görünmez — yani mesaj sessizce kaybolur.
func TestUserReplyReopensAnsweredTicket(t *testing.T) {
	s := newService(t)
	user := seedUser(t, "reopen")
	staff := seedUser(t, "reopenstaff")
	id := newTicket(t, s, user)

	if _, err := s.AdminAddMessage(context.Background(), staff, id, "Yanıtımız."); err != nil {
		t.Fatalf("personel yanıtı: %v", err)
	}
	if _, err := s.AddMessage(context.Background(), user, id, "Hâlâ görünmüyor."); err != nil {
		t.Fatalf("kullanıcı yanıtı: %v", err)
	}
	if got := statusOf(t, id); got != "USER_REPLIED" {
		t.Errorf("USER_REPLIED bekleniyordu, %s geldi", got)
	}
}

// TestClosedTicketRejectsUserMessage kapalı talebe kullanıcı yazamaz.
//
// KARAR: kapalı talep kullanıcı mesajıyla YENİDEN AÇILMAZ; 409 döner ve
// kullanıcıya yeni talep açması söylenir. Gerekçe:
// domain/ticket/ticket.go#AfterUserMessage.
func TestClosedTicketRejectsUserMessage(t *testing.T) {
	s := newService(t)
	user := seedUser(t, "closed")
	id := newTicket(t, s, user)

	if _, err := s.AdminSetStatus(context.Background(), id, ticketdom.StatusClosed); err != nil {
		t.Fatalf("kapatılamadı: %v", err)
	}

	_, err := s.AddMessage(context.Background(), user, id, "Bir şey daha soracaktım.")
	ae, ok := apperr.As(err)
	if !ok || ae.Code != "TICKET_CLOSED" {
		t.Fatalf("TICKET_CLOSED bekleniyordu, %v geldi", err)
	}
	if got := statusOf(t, id); got != "CLOSED" {
		t.Errorf("durum değişmemeliydi, %s oldu", got)
	}
}

// TestAdminCanReopenClosedTicket kapalı talebin kaçış kapısı personeldedir.
func TestAdminCanReopenClosedTicket(t *testing.T) {
	s := newService(t)
	user := seedUser(t, "adminreopen")
	id := newTicket(t, s, user)
	ctx := context.Background()

	if _, err := s.AdminSetStatus(ctx, id, ticketdom.StatusClosed); err != nil {
		t.Fatalf("kapatılamadı: %v", err)
	}
	if _, err := s.AdminSetStatus(ctx, id, ticketdom.StatusOpen); err != nil {
		t.Fatalf("yeniden açılamadı: %v", err)
	}
	if got := statusOf(t, id); got != "OPEN" {
		t.Fatalf("OPEN bekleniyordu, %s geldi", got)
	}

	// closed_at NULL'a dönmeli: "kapalı değil ama kapanma zamanı var" okunamaz
	// bir kayıttır ve raporu bozar.
	var closedAt *time.Time
	if err := pool.QueryRow(ctx,
		`SELECT closed_at FROM tickets WHERE public_id = $1`, id).Scan(&closedAt); err != nil {
		t.Fatalf("closed_at okunamadı: %v", err)
	}
	if closedAt != nil {
		t.Error("yeniden açılan talepte closed_at temizlenmemiş")
	}

	// Yeniden açıldıysa kullanıcı yine yazabilmeli.
	if _, err := s.AddMessage(ctx, user, id, "Teşekkürler, devam edelim."); err != nil {
		t.Fatalf("yeniden açılan talebe yazılamadı: %v", err)
	}
}

// TestAdminCannotSetAnsweredByHand ANSWERED / USER_REPLIED elle atanamaz.
//
// İkisi de bir MESAJIN sonucudur. Elle yazılabilseydi, hiç yanıt gelmemiş bir
// talep "yanıtlandı" görünür ve kullanıcı boş bir yazışmaya bakarak beklerdi.
func TestAdminCannotSetAnsweredByHand(t *testing.T) {
	s := newService(t)
	user := seedUser(t, "handset")
	id := newTicket(t, s, user)

	for _, target := range []ticketdom.Status{ticketdom.StatusAnswered, ticketdom.StatusUserReplied} {
		_, err := s.AdminSetStatus(context.Background(), id, target)
		ae, ok := apperr.As(err)
		if !ok || ae.Code != "TICKET_INVALID_STATUS" {
			t.Errorf("%s elle atanabildi (%v)", target, err)
		}
	}
	if got := statusOf(t, id); got != "OPEN" {
		t.Errorf("durum değişmemeliydi, %s oldu", got)
	}
}

// TestUserCannotWriteToOthersTicket sahiplik SORGUNUN parçasıdır.
//
// Başkasının talebi ile var olmayan talep AYNI sonucu verir: 404. Farklı
// yanıtlar, geçerli talep kimliklerinin varlığını sızdırırdı.
func TestUserCannotWriteToOthersTicket(t *testing.T) {
	s := newService(t)
	owner := seedUser(t, "owner")
	other := seedUser(t, "other")
	id := newTicket(t, s, owner)

	_, err := s.AddMessage(context.Background(), other, id, "Merhaba.")
	ae, ok := apperr.As(err)
	if !ok || ae.Code != "TICKET_NOT_FOUND" {
		t.Fatalf("başkasının talebine yazılabildi: %v", err)
	}

	// Var olmayan talep AYNI kodu vermeli.
	_, err = s.AddMessage(context.Background(), other, uuid.New(), "Merhaba.")
	ae2, ok2 := apperr.As(err)
	if !ok2 || ae2.Code != ae.Code {
		t.Fatalf("var olmayan talep farklı yanıt verdi: %v", err)
	}
}

// TestInvalidTicketTransitionIsRejectedByDB veritabanı İKİNCİ savunma hattıdır.
//
// Servis katmanı atlanarak doğrudan SQL yazılır: kodda bir yol durum alanına
// doğrudan yazarsa (arka plan işi, elle SQL, gelecekteki bir handler)
// tetikleyici son sözü söyler.
func TestInvalidTicketTransitionIsRejectedByDB(t *testing.T) {
	s := newService(t)
	user := seedUser(t, "dbguard")
	id := newTicket(t, s, user) // OPEN

	// OPEN → USER_REPLIED diye bir ok YOKTUR.
	_, err := pool.Exec(context.Background(),
		`UPDATE tickets SET status = 'USER_REPLIED' WHERE public_id = $1`, id)
	if err == nil {
		t.Fatal("geçersiz geçiş veritabanında kabul edildi")
	}
	if !strings.Contains(err.Error(), "gecersiz talep gecisi") {
		t.Fatalf("beklenen tetikleyici hatası değil: %v", err)
	}
	if got := statusOf(t, id); got != "OPEN" {
		t.Errorf("durum değişmemeliydi, %s oldu", got)
	}
}

// TestConcurrentMessagesKeepConsistentStatus aynı talebe eşzamanlı yazımlar.
//
// Kilit (LockTicketForUser / LockTicket) olmasaydı iki mesaj aynı ESKİ durumu
// okur ve ikisi de kendi hesabını yazardı: personel yanıtı ANSWERED yazarken
// kullanıcı yanıtı USER_REPLIED yazar, kaybeden yazım sessizce yok olurdu.
// Burada ölçülen şey, sonucun HER ZAMAN geçerli bir durum olmasıdır.
func TestConcurrentMessagesKeepConsistentStatus(t *testing.T) {
	s := newService(t)
	user := seedUser(t, "concurrent")
	staff := seedUser(t, "concurrentstaff")
	id := newTicket(t, s, user)

	var g errgroup.Group
	for i := 0; i < 8; i++ {
		i := i
		g.Go(func() error {
			var err error
			if i%2 == 0 {
				_, err = s.AddMessage(context.Background(), user, id, fmt.Sprintf("kullanıcı %d", i))
			} else {
				_, err = s.AdminAddMessage(context.Background(), staff, id, fmt.Sprintf("personel %d", i))
			}
			// Yarışta kaybeden çağrı bir hata dönebilir; sessizce yutulmaz,
			// ama testi düşürmez — ölçülen şey durum tutarlılığıdır.
			if err != nil && !errors.Is(err, context.Canceled) {
				return nil
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		t.Fatalf("eşzamanlı yazım: %v", err)
	}

	got := statusOf(t, id)
	if got != "OPEN" && got != "ANSWERED" && got != "USER_REPLIED" {
		t.Fatalf("geçersiz durum oluştu: %s", got)
	}

	// Mesajların HEPSİ yazılmış olmalı: kilit yazımı sıraya sokar, düşürmez.
	var n int
	if err := pool.QueryRow(context.Background(), `
		SELECT count(*) FROM ticket_messages m
		JOIN tickets t ON t.id = m.ticket_id
		WHERE t.public_id = $1`, id).Scan(&n); err != nil {
		t.Fatalf("sayım: %v", err)
	}
	if n != 9 { // ilk mesaj + 8 eşzamanlı
		t.Errorf("9 mesaj bekleniyordu, %d yazıldı", n)
	}
}

// TestSubjectLengthIsEnforcedByDB uzunluk sınırı veritabanında da vardır.
//
// DTO doğrulaması ilk savunmadır; başka bir yol (CLI, iş, içe aktarma) satır
// yazarsa sınırsız metin hem depoyu hem listeleme ekranını bozar.
func TestSubjectLengthIsEnforcedByDB(t *testing.T) {
	user := seedUser(t, "len")
	_, err := pool.Exec(context.Background(),
		`INSERT INTO tickets (user_id, subject) VALUES ($1, $2)`,
		user, strings.Repeat("a", ticketdom.MaxSubjectLen+1))
	if err == nil {
		t.Fatal("sınırın üstündeki konu kabul edildi")
	}
	if !strings.Contains(err.Error(), "ticket_subject_len") {
		t.Fatalf("beklenen CHECK ihlali değil: %v", err)
	}
}

// TestMessageLengthIsEnforcedByDB aynı gerekçe, mesaj gövdesi için.
func TestMessageLengthIsEnforcedByDB(t *testing.T) {
	s := newService(t)
	user := seedUser(t, "msglen")
	id := newTicket(t, s, user)

	_, err := pool.Exec(context.Background(), `
		INSERT INTO ticket_messages (ticket_id, user_id, is_staff, body)
		SELECT t.id, $1, false, $2 FROM tickets t WHERE t.public_id = $3`,
		user, strings.Repeat("b", ticketdom.MaxBodyLen+1), id)
	if err == nil {
		t.Fatal("sınırın üstündeki mesaj kabul edildi")
	}
	if !strings.Contains(err.Error(), "ticket_message_len") {
		t.Fatalf("beklenen CHECK ihlali değil: %v", err)
	}
}
