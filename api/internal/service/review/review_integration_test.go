//go:build integration

package review_test

// Müşteri yorumu akışının GERÇEK Postgres'e karşı garantileri.
//
// SQLite ile taklit edilemez: kısmi BENZERSİZ İNDEKS (bekleyen yorum sınırı),
// ENUM tipi, BEFORE UPDATE tetikleyicileri ve `SELECT ... FOR UPDATE`
// burada ölçülen şeylerin ta kendisidir.

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
	reviewdom "github.com/ikmetrik/sms-platform/api/internal/domain/review"
	reviewsvc "github.com/ikmetrik/sms-platform/api/internal/service/review"
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

func newService(t *testing.T) *reviewsvc.Service {
	t.Helper()
	return reviewsvc.New(reviewsvc.Deps{
		TxRunner: postgres.NewTxRunner(pool),
		Clock:    fixedClock{},
	})
}

// fixedClock zaman ClockPort üzerinden gelir; `time.Now()` doğrudan çağrılmaz.
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
		fmt.Sprintf("rev-%s-%s@ornek.test", suffix, tag),
		fmt.Sprintf("r_%s_%s", suffix, tag)).Scan(&id)
	if err != nil {
		t.Fatalf("kullanıcı eklenemedi: %v", err)
	}
	// LIFO: bu temizlik, kullanıcı silinmeden ÖNCE koşar.
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = pool.Exec(ctx, `DELETE FROM reviews WHERE user_id = $1`, id)
		_, _ = pool.Exec(ctx, `UPDATE reviews SET reviewed_by_user_id = NULL
		                       WHERE reviewed_by_user_id = $1`, id)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
	})
	return id
}

func newReview(t *testing.T, s *reviewsvc.Service, userID int64) uuid.UUID {
	t.Helper()
	r, err := s.Submit(context.Background(), reviewsvc.SubmitInput{
		UserID: userID, Rating: 5,
		Body: "Numara saniyeler içinde geldi, kod anında düştü. Teşekkürler.",
	})
	if err != nil {
		t.Fatalf("yorum gönderilemedi: %v", err)
	}
	return r.PublicID
}

func statusOf(t *testing.T, id uuid.UUID) string {
	t.Helper()
	var s string
	if err := pool.QueryRow(context.Background(),
		`SELECT status::text FROM reviews WHERE public_id = $1`, id).Scan(&s); err != nil {
		t.Fatalf("durum okunamadı: %v", err)
	}
	return s
}

/* ═══════════════════════ Testler ═══════════════════════ */

// TestNewReviewStartsPending yeni yorumun HİÇBİR KOŞULDA yayında başlamadığını
// doğrular.
//
// Durum istemciden alınmaz (dto.CreateReviewRequest'te `status` alanı yoktur)
// ve sütunun varsayılanı PENDING'dir. Bu testin düşmesi, moderasyonun
// tümüyle atlandığı anlamına gelir.
func TestNewReviewStartsPending(t *testing.T) {
	s := newService(t)
	user := seedUser(t, "pending")
	id := newReview(t, s, user)

	if got := statusOf(t, id); got != string(reviewdom.StatusPending) {
		t.Fatalf("yeni yorum %s durumunda, PENDING olmalıydı", got)
	}

	// Ve sitede GÖRÜNMEZ.
	items, sum, err := s.PublicList(context.Background())
	if err != nil {
		t.Fatalf("site listesi: %v", err)
	}
	for _, it := range items {
		if it.PublicID == id {
			t.Fatal("onaylanmamış yorum sitede görünüyor")
		}
	}
	_ = sum
}

// TestConcurrentSubmitsLeaveOnePending KARAR: kullanıcı başına aynı anda EN
// FAZLA BİR bekleyen yorum.
//
// Eşzamanlılık testi ZORUNLUDUR: uygulama katmanında "önce SELECT sonra
// INSERT" yapan bir kontrol, aynı anda gelen N isteğin hepsi tarafından
// geçilirdi. Kısıt kısmi benzersiz indekstedir ve burada ölçülen odur.
//
// Test AYRICA hatanın kullanıcıya gösterilebilir olduğunu doğrular: ham
// "duplicate key value violates unique constraint" metni bir kullanıcı
// mesajı değildir (değişmez #12).
func TestConcurrentSubmitsLeaveOnePending(t *testing.T) {
	s := newService(t)
	user := seedUser(t, "concurrent")

	const N = 8
	var g errgroup.Group
	ok := make(chan struct{}, N)
	conflicts := make(chan error, N)

	for i := 0; i < N; i++ {
		i := i
		g.Go(func() error {
			_, err := s.Submit(context.Background(), reviewsvc.SubmitInput{
				UserID: user, Rating: 4,
				Body: fmt.Sprintf("Eşzamanlı gönderim denemesi numara %d, yeterince uzun metin.", i),
			})
			switch {
			case err == nil:
				ok <- struct{}{}
			case errors.Is(err, reviewsvc.ErrPendingExists):
				conflicts <- err
			default:
				return fmt.Errorf("beklenmeyen hata: %w", err)
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		t.Fatal(err)
	}
	close(ok)
	close(conflicts)

	if n := len(ok); n != 1 {
		t.Fatalf("%d gönderim başarılı oldu, tam olarak 1 olmalıydı", n)
	}
	if n := len(conflicts); n != N-1 {
		t.Fatalf("%d çakışma bildirildi, %d olmalıydı", n, N-1)
	}

	// Veritabanı da aynı şeyi söylemeli.
	var pending int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM reviews WHERE user_id = $1 AND status = 'PENDING'`,
		user).Scan(&pending); err != nil {
		t.Fatalf("sayım: %v", err)
	}
	if pending != 1 {
		t.Fatalf("kuyrukta %d bekleyen yorum var, 1 olmalıydı", pending)
	}

	// Hata metni kullanıcıya gösterilebilir olmalı.
	e, isApp := apperr.As(reviewsvc.ErrPendingExists)
	if !isApp {
		t.Fatal("ErrPendingExists tipli bir uygulama hatası değil")
	}
	if strings.Contains(strings.ToLower(e.Message), "constraint") ||
		strings.Contains(strings.ToLower(e.Message), "duplicate") {
		t.Fatalf("kullanıcıya ham veritabanı hatası gösteriliyor: %q", e.Message)
	}
}

// TestDecidedReviewFreesTheQueueSlot karar verilen yorumdan sonra kullanıcının
// yeniden yazabildiğini doğrular.
//
// Sınır "ömür boyu bir yorum" DEĞİLDİR: hizmet değişir, görüş de değişir.
// Sınır yalnız AYNI ANDA bekleyen yorum sayısınadır.
func TestDecidedReviewFreesTheQueueSlot(t *testing.T) {
	ctx := context.Background()
	s := newService(t)
	user := seedUser(t, "freeslot")
	staff := seedUser(t, "freeslotstaff")
	id := newReview(t, s, user)

	if _, err := s.Decide(ctx, staff, id, reviewdom.StatusApproved, ""); err != nil {
		t.Fatalf("onay: %v", err)
	}
	if _, err := s.Submit(ctx, reviewsvc.SubmitInput{
		UserID: user, Rating: 3,
		Body: "Fikrim değişti, ikinci yorumumu yazıyorum ve bu da yeterince uzun.",
	}); err != nil {
		t.Fatalf("karar verildikten sonra yeni yorum reddedildi: %v", err)
	}
}

// TestApprovedReviewCanBeUnpublishedButNotReQueued durum makinesinin iki
// kritik okunu doğrular.
//
// APPROVED → REJECTED VAR: yanlışlıkla onaylanmış bir yorumu siteden indirmek
// mümkün olmalı. APPROVED → PENDING YOK: onay bir karardır, kuyruğa geri
// dönmez.
func TestApprovedReviewCanBeUnpublishedButNotReQueued(t *testing.T) {
	ctx := context.Background()
	s := newService(t)
	user := seedUser(t, "unpublish")
	staff := seedUser(t, "unpublishstaff")
	id := newReview(t, s, user)

	if _, err := s.Decide(ctx, staff, id, reviewdom.StatusApproved, ""); err != nil {
		t.Fatalf("onay: %v", err)
	}
	if got := statusOf(t, id); got != "APPROVED" {
		t.Fatalf("onay sonrası durum %s", got)
	}

	// Yayından kaldırma — gerekçe zorunlu.
	if _, err := s.Decide(ctx, staff, id, reviewdom.StatusRejected, ""); err == nil {
		t.Fatal("gerekçesiz yayından kaldırma kabul edildi")
	} else if !errors.Is(err, reviewsvc.ErrReasonRequired) {
		t.Fatalf("beklenen ErrReasonRequired, gelen: %v", err)
	}

	if _, err := s.Decide(ctx, staff, id, reviewdom.StatusRejected,
		"Sonradan hakaret içerdiği görüldü."); err != nil {
		t.Fatalf("yayından kaldırma: %v", err)
	}
	if got := statusOf(t, id); got != "REJECTED" {
		t.Fatalf("kaldırma sonrası durum %s", got)
	}

	// Reddedilen geri getirilemez.
	if _, err := s.Decide(ctx, staff, id, reviewdom.StatusApproved, ""); err == nil {
		t.Fatal("REJECTED → APPROVED geçişi kabul edildi")
	} else if !errors.Is(err, reviewsvc.ErrAlreadyDecided) {
		t.Fatalf("beklenen ErrAlreadyDecided, gelen: %v", err)
	}
}

// TestInvalidReviewTransitionIsRejectedByDB veritabanı tetikleyicisinin İKİNCİ
// SAVUNMA HATTI olduğunu doğrular.
//
// Servis katmanı atlanarak (doğrudan SQL) geçersiz bir geçiş denendiğinde
// tetikleyici düşmelidir. Aksi hâlde bir veri düzeltme betiği reddedilmiş bir
// yorumu sessizce yayına alabilirdi.
func TestInvalidReviewTransitionIsRejectedByDB(t *testing.T) {
	ctx := context.Background()
	s := newService(t)
	user := seedUser(t, "dbguard")
	staff := seedUser(t, "dbguardstaff")
	id := newReview(t, s, user)

	if _, err := s.Decide(ctx, staff, id, reviewdom.StatusRejected, "Reklam."); err != nil {
		t.Fatalf("red: %v", err)
	}

	_, err := pool.Exec(ctx, `
		UPDATE reviews SET status = 'APPROVED', rejection_reason = ''
		WHERE public_id = $1`, id)
	if err == nil {
		t.Fatal("veritabanı REJECTED → APPROVED geçişine izin verdi")
	}
	if !strings.Contains(err.Error(), "gecersiz yorum gecisi") {
		t.Fatalf("beklenen tetikleyici hatası değil: %v", err)
	}
}

// TestReviewBodyIsImmutable onaylanmış metnin sonradan değiştirilemediğini
// doğrular.
//
// KARAR: "onaylandı" bir yöneticinin O METNE verdiği karardır. Metin sonradan
// değişebilseydi, kullanıcı nazik bir yorum onaylattıktan sonra içeriği
// reklama çevirebilirdi ve hiçbir yerde iz kalmazdı.
func TestReviewBodyIsImmutable(t *testing.T) {
	ctx := context.Background()
	s := newService(t)
	user := seedUser(t, "immutable")
	staff := seedUser(t, "immutablestaff")
	id := newReview(t, s, user)

	if _, err := s.Decide(ctx, staff, id, reviewdom.StatusApproved, ""); err != nil {
		t.Fatalf("onay: %v", err)
	}

	_, err := pool.Exec(ctx,
		`UPDATE reviews SET body = 'Ucuz numara icin tikla: ornek.test' WHERE public_id = $1`, id)
	if err == nil {
		t.Fatal("onaylı yorumun metni değiştirilebildi")
	}
	if !strings.Contains(err.Error(), "degistirilemez") {
		t.Fatalf("beklenen tetikleyici hatası değil: %v", err)
	}

	_, err = pool.Exec(ctx, `UPDATE reviews SET rating = 1 WHERE public_id = $1`, id)
	if err == nil {
		t.Fatal("onaylı yorumun puanı değiştirilebildi")
	}
}

// TestRejectionReasonOnlyOnRejected veritabanı kısıtının çelişkili satırı
// reddettiğini doğrular.
//
// "Onaylandı ama gerekçesi 'küfür içeriyor'" gibi bir satır, moderasyon
// geçmişini okunamaz yapar.
func TestRejectionReasonOnlyOnRejected(t *testing.T) {
	ctx := context.Background()
	s := newService(t)
	user := seedUser(t, "reasoncheck")
	staff := seedUser(t, "reasoncheckstaff")
	id := newReview(t, s, user)

	if _, err := s.Decide(ctx, staff, id, reviewdom.StatusApproved, "gereksiz gerekçe"); err != nil {
		t.Fatalf("onay: %v", err)
	}
	var reason string
	if err := pool.QueryRow(ctx,
		`SELECT rejection_reason FROM reviews WHERE public_id = $1`, id).Scan(&reason); err != nil {
		t.Fatalf("okuma: %v", err)
	}
	if reason != "" {
		t.Fatalf("onayda gerekçe yazılmış: %q", reason)
	}

	// Veritabanı da doğrudan yazımı reddetmeli.
	if _, err := pool.Exec(ctx,
		`UPDATE reviews SET rejection_reason = 'sonradan eklendi' WHERE public_id = $1`,
		id); err == nil {
		t.Fatal("onaylı yoruma gerekçe yazılabildi")
	}
}

// TestPublicListShowsOnlyLatestApprovedPerUser bir kullanıcının sitede tek kez
// göründüğünü doğrular.
//
// Kullanıcı zaman içinde birden fazla yorum yazabilir; hepsini göstermek aynı
// kişiyi listede tekrarlar ve yorum sayısını olduğundan kalabalık gösterir.
func TestPublicListShowsOnlyLatestApprovedPerUser(t *testing.T) {
	ctx := context.Background()
	s := newService(t)
	user := seedUser(t, "latest")
	staff := seedUser(t, "lateststaff")

	first, err := s.Submit(ctx, reviewsvc.SubmitInput{
		UserID: user, Rating: 3, Body: "İlk izlenimim orta karar, biraz bekledim."})
	if err != nil {
		t.Fatalf("ilk yorum: %v", err)
	}
	if _, err := s.Decide(ctx, staff, first.PublicID, reviewdom.StatusApproved, ""); err != nil {
		t.Fatalf("ilk onay: %v", err)
	}

	second, err := s.Submit(ctx, reviewsvc.SubmitInput{
		UserID: user, Rating: 5, Body: "İkinci kez denedim, bu sefer çok hızlıydı."})
	if err != nil {
		t.Fatalf("ikinci yorum: %v", err)
	}
	// Saat sabit olduğu için reviewed_at ikisinde de aynı; ayrım id ile
	// yapılır (sorgu ORDER BY reviewed_at DESC, id DESC).
	if _, err := s.Decide(ctx, staff, second.PublicID, reviewdom.StatusApproved, ""); err != nil {
		t.Fatalf("ikinci onay: %v", err)
	}

	items, _, err := s.PublicList(ctx)
	if err != nil {
		t.Fatalf("site listesi: %v", err)
	}
	var seen int
	for _, it := range items {
		if it.PublicID == first.PublicID {
			t.Error("eski onaylı yorum hâlâ sitede görünüyor")
		}
		if it.PublicID == second.PublicID {
			seen++
		}
	}
	if seen != 1 {
		t.Fatalf("en son onaylı yorum %d kez göründü, 1 olmalıydı", seen)
	}
}

// TestUserCannotReadOthersReview sahiplik kısıtının SORGUDA olduğunu doğrular.
//
// Başkasının yorumu ile var olmayan yorum AYNI yanıtı alır: hangi
// kimliklerin geçerli olduğu sızdırılmaz.
func TestUserCannotReadOthersReview(t *testing.T) {
	ctx := context.Background()
	s := newService(t)
	sahip := seedUser(t, "sahip")
	yabanci := seedUser(t, "yabanci")
	id := newReview(t, s, sahip)

	if _, err := s.Get(ctx, yabanci, id); !errors.Is(err, reviewsvc.ErrNotFound) {
		t.Fatalf("başkasının yorumu okunabildi ya da farklı hata döndü: %v", err)
	}
	if _, err := s.Get(ctx, sahip, id); err != nil {
		t.Fatalf("sahibi kendi yorumunu okuyamadı: %v", err)
	}
	// Var olmayan kimlik de AYNI hatayı verir.
	if _, err := s.Get(ctx, sahip, uuid.New()); !errors.Is(err, reviewsvc.ErrNotFound) {
		t.Fatalf("var olmayan yorum farklı hata döndürdü: %v", err)
	}
}

// TestConcurrentDecisionsLeaveOneOutcome iki yöneticinin aynı yoruma aynı anda
// karar vermesini sınar.
//
// Kilit (LockReview ... FOR UPDATE) olmasaydı ikisi de PENDING görür, ikisi de
// yazar ve son yazan kazanırdı — "onayladım ama reddedilmiş görünüyor".
func TestConcurrentDecisionsLeaveOneOutcome(t *testing.T) {
	ctx := context.Background()
	s := newService(t)
	user := seedUser(t, "race")
	staff := seedUser(t, "racestaff")
	id := newReview(t, s, user)

	var g errgroup.Group
	results := make(chan string, 2)

	g.Go(func() error {
		if _, err := s.Decide(ctx, staff, id, reviewdom.StatusApproved, ""); err == nil {
			results <- "APPROVED"
		} else if !errors.Is(err, reviewsvc.ErrAlreadyDecided) {
			return fmt.Errorf("onay beklenmeyen hata: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		if _, err := s.Decide(ctx, staff, id, reviewdom.StatusRejected, "Reklam."); err == nil {
			results <- "REJECTED"
		} else if !errors.Is(err, reviewsvc.ErrAlreadyDecided) {
			return fmt.Errorf("red beklenmeyen hata: %w", err)
		}
		return nil
	})
	if err := g.Wait(); err != nil {
		t.Fatal(err)
	}
	close(results)

	// PENDING → APPROVED → REJECTED zinciri geçerli olduğu için İKİSİ DE
	// başarılı olabilir; ama sıra ne olursa olsun veritabanındaki durum
	// tek ve tutarlı bir sonuç olmalı ve asla PENDING kalmamalıdır.
	final := statusOf(t, id)
	if final == "PENDING" {
		t.Fatal("iki karardan sonra yorum hâlâ kuyrukta")
	}
	if final != "APPROVED" && final != "REJECTED" {
		t.Fatalf("beklenmeyen son durum: %s", final)
	}
	if len(results) == 0 {
		t.Fatal("hiçbir karar yazılamadı")
	}
}

// TestBodyLengthIsMeasuredInRunes veritabanı sınırının KARAKTER saydığını
// doğrular.
//
// `char_length()` karakter sayar, `octet_length()` bayt. Tamamı Türkçe 1000
// karakterlik bir yorum ~1800 bayttır; sınır bayt olsaydı kullanıcı hiçbir
// zaman erişemeyeceği bir sayaç görürdü.
func TestBodyLengthIsMeasuredInRunes(t *testing.T) {
	ctx := context.Background()
	s := newService(t)
	user := seedUser(t, "runes")

	// 1000 Türkçe karakter — bayt olarak sınırın çok üstünde, karakter
	// olarak tam sınırda.
	body := strings.Repeat("ş", reviewdom.MaxBodyLen)
	if _, err := s.Submit(ctx, reviewsvc.SubmitInput{UserID: user, Rating: 5, Body: body}); err != nil {
		t.Fatalf("tam sınırdaki Türkçe yorum reddedildi (bayt sayılıyor olabilir): %v", err)
	}
}

// TestListReviewsForUserDurumSuzgeci `GET /reviews/mine?status=` süzgecini
// doğrular.
//
// ÜÇ İDDİA TEK TESTTE: süzgeç süzer · SAYIM listeyle AYNI süzgeci alır ·
// SAHİPLİK süzgeçten üstündür (değişmez #7).
//
// Kullanıcı başına AYNI ANDA tek bekleyen yorum kısıtı var
// (reviews_one_pending_per_user_idx); ikinci yorumu yazabilmek için birincisi
// önce karara bağlanır.
func TestListReviewsForUserDurumSuzgeci(t *testing.T) {
	ctx := context.Background()
	s := newService(t)
	alice := seedUser(t, "suzgec-a")
	bob := seedUser(t, "suzgec-b")

	yayinda := newReview(t, s, alice)
	karara := func(id uuid.UUID) {
		t.Helper()
		if _, err := pool.Exec(ctx,
			`UPDATE reviews SET status='APPROVED', reviewed_at=now() WHERE public_id=$1`,
			id); err != nil {
			t.Fatalf("durum kurulumu: %v", err)
		}
	}
	karara(yayinda)
	bekleyen := newReview(t, s, alice)

	// Bob'un yorumu da APPROVED: Alice APPROVED süzdüğünde ONU GÖRMEMELİ.
	karara(newReview(t, s, bob))

	durum := func(v reviewdom.Status) *reviewdom.Status { return &v }

	kontrol := func(ad string, st *reviewdom.Status, bekleyenler ...uuid.UUID) {
		t.Helper()
		rows, total, err := s.List(ctx, alice, st, 20, 0)
		if err != nil {
			t.Fatalf("%s: liste: %v", ad, err)
		}
		if int(total) != len(rows) {
			t.Errorf("%s: SAYIM LİSTEDEN AYRIŞTI — total=%d satır=%d "+
				"(CountReviewsForUser süzgeci ListReviewsForUser ile aynı değil)",
				ad, total, len(rows))
		}
		if len(rows) != len(bekleyenler) {
			t.Fatalf("%s: %d yorum bekleniyordu, %d geldi", ad, len(bekleyenler), len(rows))
		}
		got := make(map[uuid.UUID]bool, len(rows))
		for _, r := range rows {
			got[r.PublicID] = true
		}
		for _, id := range bekleyenler {
			if !got[id] {
				t.Errorf("%s: %s listede yok", ad, id)
			}
		}
	}

	kontrol("süzgeçsiz", nil, yayinda, bekleyen)
	kontrol("onay bekleyen", durum(reviewdom.StatusPending), bekleyen)
	// 🔴 Bob'un yorumu da APPROVED — bu satır sahiplik iddiasıdır.
	kontrol("yayında", durum(reviewdom.StatusApproved), yayinda)
	kontrol("hiç olmayan durum", durum(reviewdom.StatusRejected))
}

// TestListReviewsForAdminAramasi `GET /admin/reviews?q=` aramasını doğrular.
//
// 🔴 ASIL İDDİA SAYIMIN LİSTEYLE AYNI SORGUYU KURMASI: `q` kullanıcı
// sütunlarında da arandığı için `count` sorgusuna `JOIN users` EKLENDİ.
// JOIN unutulsaydı sayfalama yanlış toplam gösterirdi.
func TestListReviewsForAdminAramasi(t *testing.T) {
	ctx := context.Background()
	s := newService(t)
	ada := seedUser(t, "ada")
	kemal := seedUser(t, "kemal")

	adaninki := newReview(t, s, ada)
	kemalinki := newReview(t, s, kemal)

	metin := func(v string) *string { return &v }

	kontrol := func(ad string, ara *string, bekleyenler ...uuid.UUID) {
		t.Helper()
		rows, total, _, err := s.AdminList(ctx, reviewsvc.AdminFilter{Q: ara}, 50, 0)
		if err != nil {
			t.Fatalf("%s: liste: %v", ad, err)
		}
		if int(total) != len(rows) {
			t.Errorf("%s: SAYIM LİSTEDEN AYRIŞTI — total=%d satır=%d "+
				"(CountReviewsForAdmin sorgusu ListReviewsForAdmin ile aynı değil)",
				ad, total, len(rows))
		}
		if len(rows) != len(bekleyenler) {
			t.Fatalf("%s: %d yorum bekleniyordu, %d geldi", ad, len(bekleyenler), len(rows))
		}
		got := make(map[uuid.UUID]bool, len(rows))
		for _, r := range rows {
			got[r.PublicID] = true
		}
		for _, id := range bekleyenler {
			if !got[id] {
				t.Errorf("%s: %s listede yok", ad, id)
			}
		}
	}

	kontrol("aramasız", nil, adaninki, kemalinki)
	kontrol("kullanıcıya göre", metin("ada"), adaninki)
	kontrol("büyük harf", metin("KEMAL"), kemalinki)
	// Gövde de aranır: `newReview` her ikisine de aynı metni yazıyor.
	kontrol("gövdeye göre", metin("saniyeler içinde"), adaninki, kemalinki)
	kontrol("eşleşmeyen", metin("bulunmayan-sey"))
}
