// Package deposit bakiye yükleme kullanım senaryolarını yürütür
// (FR-500 … FR-503).
//
// İki ayrı akış vardır ve ikisi de bu pakettedir:
//
//	kullanıcı  → yöntemi seç, tutar + referans bildir, dekont yükle  (PENDING)
//	yönetici   → onayla (bakiye yazılır) veya reddet (bakiye değişmez)
//
// Onay TEK TRANSACTION'dır: satır kilidi → koşullu UPDATE → defter kaydı →
// denetim kaydı. Bu paket dış HTTP çağrısı YAPMAZ; USDT doğrulaması
// yöneticinin manuel işidir (docs/design.md §8.5).
//
// test: deposit_integration_test.go#TestApproveWritesAuditLog
package deposit

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/ikmetrik/sms-platform/api/internal/adapter/storage"
	"github.com/ikmetrik/sms-platform/api/internal/db"
	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
	"github.com/ikmetrik/sms-platform/api/internal/port"
	walletsvc "github.com/ikmetrik/sms-platform/api/internal/service/wallet"
)

// tableMinAmountMinor / tableMaxAmountMinor deposits tablosundaki
// deposit_amount_range CHECK'inin aynısıdır (00008_orders.sql).
//
// Kodda TEKRARLANIYOR çünkü yönetici bir yöntemin sınırlarını serbestçe
// değiştirebilir; yöntem sınırı tablo sınırının dışına çıkarsa INSERT ham bir
// 23514 ile düşerdi. Burada önce yakalanır ve kullanıcı Türkçe bir uyarı görür.
const (
	tableMinAmountMinor int64 = 1000    //     10,00 ₺
	tableMaxAmountMinor int64 = 5000000 // 50.000,00 ₺
)

type txRunner interface {
	Queries() *db.Queries
	InTx(ctx context.Context, fn func(*db.Queries) error) error
}

// ReceiptStore dekont dosyalarını saklar.
//
// Arayüz TÜKETİCİDE tanımlıdır: servis yerel disk, S3 ya da başka bir depoyu
// tanımak zorunda değildir. nil olabilir — UPLOAD_DIR tanımsızsa dekont
// yükleme ucu ErrReceiptDisabled döner ve kullanıcı "yükledim sandım"
// durumunda kalmaz.
type ReceiptStore interface {
	SaveReceipt(ctx context.Context, r io.Reader) (storage.Receipt, error)
	OpenReceipt(ctx context.Context, rel string) (io.ReadCloser, storage.Receipt, error)
}

// Deps servis bağımlılıkları.
type Deps struct {
	TxRunner txRunner
	Wallet   *walletsvc.Service
	Clock    port.Clock
	Receipts ReceiptStore
}

// Service bakiye yükleme servisi.
type Service struct {
	tx       txRunner
	wallet   *walletsvc.Service
	clock    port.Clock
	receipts ReceiptStore
}

func New(d Deps) *Service {
	return &Service{tx: d.TxRunner, wallet: d.Wallet, clock: d.Clock, receipts: d.Receipts}
}

/* ═══════════════════════ Ödeme yöntemleri ═══════════════════════ */

// Methods kullanıcıya gösterilecek AKTİF yöntemleri döner.
//
// 🔴 PASİF YÖNTEM DÖNMEZ. Yöntemin config'i IBAN veya cüzdan adresi taşır;
// pasif bir yöntemin bilgisi ya eksiktir (para hiçbir yere gitmez) ya da
// bilerek kapatılmıştır. İkisinde de kullanıcının görmesi zarar verir.
//
// test: deposit_integration_test.go#TestInactiveMethodIsNotVisibleToUser
func (s *Service) Methods(ctx context.Context) ([]db.DepositMethod, error) {
	rows, err := s.tx.Queries().ListActiveDepositMethods(ctx)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return rows, nil
}

/* ═══════════════════════ Talep oluşturma ═══════════════════════ */

// CreateInput kullanıcı yükleme talebi.
//
// Ne SAĞLAYICI ne AĞ ne de YÖNTEM ADI istemciden alınır: hepsi yöntemin
// veritabanındaki kaydından okunur (değişmez #9'un ruhu).
type CreateInput struct {
	UserID         int64
	MethodPublicID uuid.UUID
	AmountMinor    int64
	// Reference havalede açıklama/dekont numarası, kriptoda işlem hash'i.
	Reference string
	Note      string
	// IdempotencyKey İSTEMCİDEN gelir (Idempotency-Key başlığı).
	// Boşsa koruma devrede değildir — eski istemciler kırılmasın diye.
	IdempotencyKey string
}

// Create yeni bir PENDING talep açar.
func (s *Service) Create(ctx context.Context, in CreateInput) (db.Deposit, error) {
	q := s.tx.Queries()

	// 🔴 İDEMPOTENS ÖNCE KONTROL EDİLİR.
	//
	// Kullanıcı havaleyi yapmış, formu göndermiş, ağ yanıtı yutmuş olabilir.
	// Tekrar denediğinde İKİNCİ bir talep oluşursa yönetici aynı havale için
	// iki satır görür; ikisini de onaylarsa 500 ₺'lik havaleye 1000 ₺ yazılır.
	// Defterin idempotency anahtarı bunu YAKALAMAZ: o anahtar talebin
	// public_id'sinden türüyor ve iki talebin iki ayrı kimliği var.
	//
	// test: deposit_integration_test.go#TestDepositCreateIsIdempotent
	anahtar := strings.TrimSpace(in.IdempotencyKey)
	if anahtar != "" {
		mevcut, err := q.GetDepositByIdempotencyKey(ctx, db.GetDepositByIdempotencyKeyParams{
			UserID: in.UserID, IdempotencyKey: &anahtar,
		})
		if err == nil {
			return mevcut, nil // aynı istek — yeni satır yazılmaz
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return db.Deposit{}, apperr.Internal(err)
		}
	}

	m, err := q.GetDepositMethod(ctx, in.MethodPublicID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Var olmayan yöntem ile pasif yöntem AYNI hataya düşer:
			// aksi hâlde hangi kimliklerin var olduğu sızardı.
			return db.Deposit{}, ErrMethodInactive
		}
		return db.Deposit{}, apperr.Internal(err)
	}
	if !m.IsActive {
		return db.Deposit{}, ErrMethodInactive
	}
	if err := checkAmount(in.AmountMinor, m); err != nil {
		return db.Deposit{}, err
	}

	params := db.CreateDepositParams{
		UserID:      in.UserID,
		MethodID:    &m.ID,
		MethodName:  m.Name, // ANLIK GÖRÜNTÜ: yöntem silinse de geçmiş okunur
		AmountMinor: in.AmountMinor,
		UserNote:    strings.TrimSpace(in.Note),
	}
	if anahtar != "" {
		params.IdempotencyKey = &anahtar
	}

	ref := strings.TrimSpace(in.Reference)
	switch m.Kind {
	case db.DepositMethodKindCRYPTO:
		// Ağ YÖNTEMİN yapılandırmasından gelir, istemciden değil: kullanıcı
		// "ERC-20" yazıp TRC-20 adresine gönderemesin.
		params.Network = configValue(m.Config, "ag")
		params.TxHash = &ref
	default:
		// 🔴 HAVALEDE tx_hash NULL BIRAKILIR. Boş string yazılırsa
		// deposits_tx_hash_uniq kısmi indeksi ikinci havale talebini 23505 ile
		// reddeder ve kullanıcı bir daha havale bildiremez.
		// test: deposit_integration_test.go#TestBankTransferDepositLeavesTxHashNull
		params.TxHash = nil
		params.UserNote = strings.TrimSpace(ref + "\n" + params.UserNote)
	}

	dep, err := q.CreateDeposit(ctx, params)
	if err != nil {
		// Yarış: iki istek aynı anda geldi ve ikisi de kontrolü geçti.
		// Tekil indeks birini reddeder; o zaman kazananı döneriz.
		// test: deposit_integration_test.go#TestDepositCreateIsIdempotentUnderConcurrency
		if anahtar != "" && isUniqueViolation(err, "deposits_idempotency_uniq") {
			mevcut, gerr := q.GetDepositByIdempotencyKey(ctx, db.GetDepositByIdempotencyKeyParams{
				UserID: in.UserID, IdempotencyKey: &anahtar,
			})
			if gerr == nil {
				return mevcut, nil
			}
		}
		return db.Deposit{}, mapDBErr(err)
	}
	return dep, nil
}

// checkAmount tutarı hem YÖNTEMİN hem TABLONUN sınırlarına göre doğrular.
func checkAmount(amount int64, m db.DepositMethod) error {
	if amount < tableMinAmountMinor || amount > tableMaxAmountMinor {
		return ErrAmountRange
	}
	if amount < m.MinAmountMinor {
		return ErrAmountRange
	}
	// 🔴 max_amount_minor = 0 "ÜST SINIR YOK" demektir (dm_max_gte_min kısıtı
	// sıfırı açıkça muaf tutuyor). Sıfırı 0,00 ₺ tavan sanan bir kontrol o
	// yöntemdeki TÜM yüklemeleri reddeder ve hata "tutarınız çok yüksek"
	// olarak görünür — teşhisi zor.
	// test: deposit_integration_test.go#TestAmountOutOfMethodRangeRejected
	if m.MaxAmountMinor > 0 && amount > m.MaxAmountMinor {
		return ErrAmountRange
	}
	return nil
}

/* ═══════════════════════ Kullanıcı okumaları ═══════════════════════ */

// Get kullanıcının KENDİ talebini döner.
//
// Sahiplik sorgunun parçasıdır (değişmez #7); burada ayrı bir if yoktur.
//
// test: deposit_integration_test.go#TestUserCannotSeeOthersDeposit
func (s *Service) Get(ctx context.Context, userID int64, publicID uuid.UUID) (db.Deposit, error) {
	dep, err := s.tx.Queries().GetDepositForUser(ctx, db.GetDepositForUserParams{
		PublicID: publicID, UserID: userID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.Deposit{}, ErrNotFound
		}
		return db.Deposit{}, apperr.Internal(err)
	}
	return dep, nil
}

// List kullanıcının kendi taleplerini sayfalı döner.
//
// `status` NIL ise durum süzgeci UYGULANMAZ. Sayım listeyle AYNI süzgeci alır;
// ayrışırsa sayfalama yalan söyler (queries/deposits.sql notu).
//
// 🔴 `userID` süzgeçten AYRI bir parametredir ve sorguya her zaman girer
// (değişmez #7); hiçbir `status` değeri başkasının talebini döndüremez.
// test: deposit_integration_test.go#TestListUserDepositsDurumSuzgeci
func (s *Service) List(
	ctx context.Context, userID int64, status *db.DepositStatus, limit, offset int32,
) ([]db.Deposit, int64, error) {
	q := s.tx.Queries()
	rows, err := q.ListUserDeposits(ctx, db.ListUserDepositsParams{
		UserID: userID, Status: status, Lim: limit, Off: offset,
	})
	if err != nil {
		return nil, 0, apperr.Internal(err)
	}
	total, err := q.CountUserDeposits(ctx, db.CountUserDepositsParams{
		UserID: userID, Status: status,
	})
	if err != nil {
		return nil, 0, apperr.Internal(err)
	}
	return rows, total, nil
}

/* ═══════════════════════ Dekont (FR-500) ═══════════════════════ */

// AttachReceipt dekontu doğrular, depoya yazar ve talebe bağlar.
//
// Dosyanın tipi SİHİRLİ BAYT ile belirlenir (depo katmanında); istemcinin
// dosya adı ve Content-Type'ı buraya hiç ulaşmaz. Yol veritabanına yazılır ama
// hiçbir yanıtta dışarı verilmez — erişim yalnız yetkili uçtandır (KK-500).
//
// test: deposit_integration_test.go#TestReceiptUploadRejectsFakeImage
func (s *Service) AttachReceipt(ctx context.Context, userID int64, publicID uuid.UUID, body io.Reader) (db.Deposit, error) {
	if s.receipts == nil {
		return db.Deposit{}, ErrReceiptDisabled
	}

	// Sahiplik ve durum ÖNCE doğrulanır: başkasının talebi için dosya yazmak,
	// diski yetkisiz bir kullanıcıya açmaktır.
	dep, err := s.Get(ctx, userID, publicID)
	if err != nil {
		return db.Deposit{}, err
	}
	if dep.Status != db.DepositStatusPENDING {
		return db.Deposit{}, ErrNotPending
	}

	rec, err := s.receipts.SaveReceipt(ctx, body)
	if err != nil {
		return db.Deposit{}, mapStorageErr(err)
	}

	updated, err := s.tx.Queries().SetDepositReceipt(ctx, db.SetDepositReceiptParams{
		ReceiptPath: rec.Path, PublicID: publicID, UserID: userID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Arada sonuçlandırılmış olabilir.
			return db.Deposit{}, ErrNotPending
		}
		return db.Deposit{}, apperr.Internal(err)
	}
	return updated, nil
}

// OpenReceipt bir talebin dekontunu okumak için açar.
//
// scoped=true ise sahiplik sorgunun parçasıdır (kullanıcı yolu); false ise
// çağıranın `deposits:read` iznini taşıdığı middleware tarafından
// doğrulanmıştır (yönetim yolu).
//
// test: deposit_integration_test.go#TestOtherUserCannotReadReceipt
func (s *Service) OpenReceipt(ctx context.Context, userID *int64, publicID uuid.UUID) (io.ReadCloser, storage.Receipt, error) {
	if s.receipts == nil {
		return nil, storage.Receipt{}, ErrReceiptDisabled
	}

	var dep db.Deposit
	var err error
	if userID != nil {
		dep, err = s.Get(ctx, *userID, publicID)
	} else {
		dep, err = s.tx.Queries().GetDeposit(ctx, publicID)
		if errors.Is(err, pgx.ErrNoRows) {
			err = ErrNotFound
		} else if err != nil {
			err = apperr.Internal(err)
		}
	}
	if err != nil {
		return nil, storage.Receipt{}, err
	}
	if strings.TrimSpace(dep.ReceiptPath) == "" {
		return nil, storage.Receipt{}, ErrReceiptMissing
	}

	rc, meta, err := s.receipts.OpenReceipt(ctx, dep.ReceiptPath)
	if err != nil {
		return nil, storage.Receipt{}, mapStorageErr(err)
	}
	return rc, meta, nil
}

/* ═══════════════════════ Yardımcılar ═══════════════════════ */

// configValue yöntemin JSONB yapılandırmasından bir alan okur.
//
// Bozuk yapılandırma boş dizeye düşer: bir JSON hatası yüzünden talep açmayı
// engellemek, yöneticinin fark edemeyeceği bir arıza üretirdi. Zorunlu
// alanların dolu olması yöntemi aktifleştirirken zaten denetleniyor
// (handler/admin.go missingConfigFields).
func configValue(raw []byte, key string) string {
	if len(raw) == 0 {
		return ""
	}
	var cfg map[string]string
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return ""
	}
	return strings.TrimSpace(cfg[key])
}
