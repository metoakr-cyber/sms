package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
	"github.com/ikmetrik/sms-platform/api/internal/port"
)

const (
	sessionPrefix     = "sess:"
	userSessionPrefix = "usess:" // kullanıcı → oturum kimlikleri kümesi
)

// SessionStore oturumları Redis'te tutar.
//
// Kullanıcı başına bir küme (SET) ayrıca tutulur; böylece "bu kullanıcının tüm
// oturumlarını düşür" işlemi tüm anahtarları taramadan (KEYS/SCAN olmadan)
// yapılabilir. KEYS üretimde bloklayıcıdır.
type SessionStore struct {
	rdb *goredis.Client
}

func NewSessionStore(rdb *goredis.Client) *SessionStore { return &SessionStore{rdb: rdb} }

var _ port.SessionStore = (*SessionStore)(nil)

func (s *SessionStore) Create(ctx context.Context, sess port.Session) error {
	ttl := time.Until(sess.ExpiresAt)
	if ttl <= 0 {
		return fmt.Errorf("redis: oturum süresi geçmiş")
	}
	blob, err := json.Marshal(sess)
	if err != nil {
		return fmt.Errorf("redis: oturum serileştirilemedi: %w", err)
	}

	pipe := s.rdb.TxPipeline()
	pipe.Set(ctx, sessionPrefix+sess.ID, blob, ttl)
	pipe.SAdd(ctx, userKey(sess.UserID), sess.ID)
	// Kümenin kendisi de süresi dolsun ki yetim kayıt birikmesin.
	pipe.Expire(ctx, userKey(sess.UserID), ttl+24*time.Hour)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("redis: oturum yazılamadı: %w", err)
	}
	return nil
}

func (s *SessionStore) Get(ctx context.Context, id string) (port.Session, error) {
	blob, err := s.rdb.Get(ctx, sessionPrefix+id).Bytes()
	if errors.Is(err, goredis.Nil) {
		return port.Session{}, apperr.ErrUnauthenticated
	}
	if err != nil {
		return port.Session{}, fmt.Errorf("redis: oturum okunamadı: %w", err)
	}
	var sess port.Session
	if err := json.Unmarshal(blob, &sess); err != nil {
		return port.Session{}, fmt.Errorf("redis: oturum çözülemedi: %w", err)
	}
	return sess, nil
}

func (s *SessionStore) Touch(ctx context.Context, id string, ttl time.Duration) error {
	// Kayan pencere: her istekte süre uzar. Hata yok sayılmaz ama çağıran
	// tarafın isteği bu yüzden başarısız OLMAZ (bkz. session middleware).
	return s.rdb.Expire(ctx, sessionPrefix+id, ttl).Err()
}

func (s *SessionStore) Revoke(ctx context.Context, id string) error {
	sess, err := s.Get(ctx, id)
	if err != nil {
		// Zaten yoksa iptal edilmiş sayılır — idempotent.
		if errors.Is(err, apperr.ErrUnauthenticated) {
			return nil
		}
		return err
	}
	pipe := s.rdb.TxPipeline()
	pipe.Del(ctx, sessionPrefix+id)
	pipe.SRem(ctx, userKey(sess.UserID), id)
	_, err = pipe.Exec(ctx)
	return err
}

func (s *SessionStore) RevokeAllForUser(ctx context.Context, userID int64) error {
	ids, err := s.rdb.SMembers(ctx, userKey(userID)).Result()
	if err != nil && !errors.Is(err, goredis.Nil) {
		return fmt.Errorf("redis: kullanıcı oturumları listelenemedi: %w", err)
	}
	if len(ids) == 0 {
		return nil
	}
	keys := make([]string, 0, len(ids))
	for _, id := range ids {
		keys = append(keys, sessionPrefix+id)
	}
	pipe := s.rdb.TxPipeline()
	pipe.Del(ctx, keys...)
	pipe.Del(ctx, userKey(userID))
	_, err = pipe.Exec(ctx)
	return err
}

func userKey(userID int64) string {
	return fmt.Sprintf("%s%d", userSessionPrefix, userID)
}
