// Package catalog ürün kataloğunu yönetir ve sağlayıcılarla senkronlar.
package catalog

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/ikmetrik/sms-platform/api/internal/adapter/crypto"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/postgres"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/provider"
	"github.com/ikmetrik/sms-platform/api/internal/db"
	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
	"github.com/ikmetrik/sms-platform/api/internal/port"
)

type Service struct {
	tx       *postgres.TxRunner
	registry *provider.Registry
	secrets  *crypto.SecretBox
	clock    port.Clock
}

type Deps struct {
	TxRunner *postgres.TxRunner
	Registry *provider.Registry
	Secrets  *crypto.SecretBox
	Clock    port.Clock
}

func New(d Deps) *Service {
	if d.Clock == nil {
		d.Clock = port.RealClock{}
	}
	return &Service{tx: d.TxRunner, registry: d.Registry, secrets: d.Secrets, clock: d.Clock}
}

// SyncReport bir senkron turunun sonucu.
type SyncReport struct {
	ProviderName string
	Services     int
	Countries    int
	Products     int
	Offers       int
	StaleMarked  int64
	Duration     time.Duration
	Errors       []string
}

// creds bir sağlayıcının şifresi çözülmüş kimlik bilgisini döner.
//
// Şifre çözme YALNIZ burada yapılır ve sonuç bellekten çıkmaz: ham anahtar
// log'a, hata mesajına veya API yanıtına asla girmez.
// test: crypto/secretbox_test.go#TestSealOpenRoundTrip (ham anahtar şifreli metinde yok)
func (s *Service) creds(p db.Provider) (port.Creds, error) {
	c := port.Creds{BaseURL: p.BaseUrl}
	if len(p.ApiKeyEnc) == 0 {
		return c, nil // FAKE sağlayıcı anahtar istemez
	}
	key, err := s.secrets.OpenString(p.ApiKeyEnc)
	if err != nil {
		return port.Creds{}, fmt.Errorf("sağlayıcı %q: API anahtarı çözülemedi: %w", p.Name, err)
	}
	c.APIKey = key
	return c, nil
}

// SyncDimensions ülke ve servis listelerini senkronlar, eşleştirmeleri günceller.
func (s *Service) SyncDimensions(ctx context.Context, providerID int64) (SyncReport, error) {
	start := s.clock.Now()
	q := s.tx.Queries()

	prov, err := q.GetProvider(ctx, providerID)
	if err != nil {
		return SyncReport{}, apperr.ErrNotFound
	}
	rep := SyncReport{ProviderName: prov.Name}

	adapter, err := s.registry.Resolve(string(prov.Protocol))
	if err != nil {
		return rep, apperr.Internal(err)
	}
	creds, err := s.creds(prov)
	if err != nil {
		return rep, apperr.Internal(err)
	}

	// ── Ülkeler ──
	countries, err := adapter.ListCountries(ctx, creds)
	if err != nil {
		return rep, apperr.ErrProviderUnavailable.Wrap(err)
	}
	unresolved := 0
	for _, rc := range countries {
		iso, nameTR, phone, rent := s.interpretCountry(ctx, q, rc)
		if iso == "" {
			// Referansta karşılığı yok. SESSİZCE ATLAMAYIZ: sayı rapora
			// yazılır, yoksa katalogda eksik ülkeler kimsenin fark etmediği
			// bir şekilde birikir.
			unresolved++
			continue
		}
		row, err := q.UpsertCountry(ctx, db.UpsertCountryParams{
			Iso2: iso, Name: rc.Name, NameTr: nameTR,
			PhoneCode: phone, SupportsRent: rent,
		})
		if err != nil {
			rep.Errors = append(rep.Errors, fmt.Sprintf("ülke %s: %v", iso, err))
			continue
		}
		if err := q.UpsertDimensionMap(ctx, db.UpsertDimensionMapParams{
			ProviderID: providerID, Dimension: string(port.DimCountry),
			LocalID: row.ID, RemoteCode: rc.RemoteCode,
		}); err != nil {
			rep.Errors = append(rep.Errors, fmt.Sprintf("ülke eşleştirme %s: %v", iso, err))
			continue
		}
		rep.Countries++
	}

	if unresolved > 0 {
		rep.Errors = append(rep.Errors, fmt.Sprintf(
			"%d ülke ISO2'ye çevrilemedi — country_reference tablosuna takma ad eklenmeli", unresolved))
	}

	// ── Servisler ──
	services, err := adapter.ListServices(ctx, creds)
	if err != nil {
		return rep, apperr.ErrProviderUnavailable.Wrap(err)
	}
	for i, rs := range services {
		row, err := q.UpsertService(ctx, db.UpsertServiceParams{
			Code: rs.RemoteCode, Name: rs.Name, NameTr: "", SortOrder: int32(i * 10),
		})
		if err != nil {
			rep.Errors = append(rep.Errors, fmt.Sprintf("servis %s: %v", rs.RemoteCode, err))
			continue
		}
		if err := q.UpsertDimensionMap(ctx, db.UpsertDimensionMapParams{
			ProviderID: providerID, Dimension: string(port.DimService),
			LocalID: row.ID, RemoteCode: rs.RemoteCode,
		}); err != nil {
			rep.Errors = append(rep.Errors, fmt.Sprintf("servis eşleştirme %s: %v", rs.RemoteCode, err))
			continue
		}
		rep.Services++
	}

	rep.Duration = s.clock.Now().Sub(start)
	slog.Info("boyut senkronu tamam",
		"provider", prov.Name, "countries", rep.Countries, "services", rep.Services,
		"errors", len(rep.Errors), "duration", rep.Duration)
	return rep, nil
}

// SyncOffers fiyat/stok anlık görüntüsünü tazeler.
//
// TOPLU çalışır: HeroSMS tek çağrıda ~20.000 kombinasyon döndürüyor (canlı
// ölçüm: 1,09 MB / ~600 ms). Eski prototip ülke başına ayrı istek atıyordu.
func (s *Service) SyncOffers(ctx context.Context, providerID int64) (SyncReport, error) {
	start := s.clock.Now()
	q := s.tx.Queries()

	prov, err := q.GetProvider(ctx, providerID)
	if err != nil {
		return SyncReport{}, apperr.ErrNotFound
	}
	rep := SyncReport{ProviderName: prov.Name}

	adapter, err := s.registry.Resolve(string(prov.Protocol))
	if err != nil {
		return rep, apperr.Internal(err)
	}
	creds, err := s.creds(prov)
	if err != nil {
		return rep, apperr.Internal(err)
	}

	offers, err := adapter.ListOffers(ctx, creds, port.VerifySMS)
	if err != nil {
		return rep, apperr.ErrProviderUnavailable.Wrap(err)
	}

	// Tur damgası. Bu turda yazılan her teklif BU damgayı alır; turdan sonra
	// damgası daha eski olan teklifler "bu turda görülmedi" demektir.
	//
	// Damga uygulama saatinden gelir ve HEM yazımda HEM karşılaştırmada
	// kullanılır. Yazımda now() (veritabanı saati) kullanılsaydı iki saat
	// arasındaki kayma tüm teklifleri sessizce "yok" işaretlerdi.
	roundStamp := s.clock.Now()

	// EŞLEŞTİRME TABLOSU ÜZERİNDEN ÇÖZÜM.
	//
	// Eskiden servis `services.code`, ülke `countries.iso2` üzerinden aranıyordu.
	// Bu yalnız sağlayıcının kodu tesadüfen bizim kodumuzla aynıysa çalışır.
	// HeroSMS ülkeleri "62" diye gönderir; `countries.iso2` ise "TR"dir ve
	// arama boş döner. Sonuç: TEK BİR HATA OLMADAN sıfır teklif — sağlayıcı
	// kodu ile yerel kimlik arasındaki köprü `provider_dimension_maps`tir.
	//
	// test: country_ref_integration_test.go#TestCountryIsoComesFromReferenceNotProviderCode
	unmatched := 0
	for _, o := range offers {
		svcID, err := q.ResolveDimensionLocal(ctx, db.ResolveDimensionLocalParams{
			ProviderID: providerID, Dimension: string(port.DimService), RemoteCode: o.ServiceCode,
		})
		if err != nil {
			unmatched++
			continue // eşleştirilmemiş servis; boyut senkronu bunu çözer
		}
		ctryID, err := q.ResolveDimensionLocal(ctx, db.ResolveDimensionLocalParams{
			ProviderID: providerID, Dimension: string(port.DimCountry), RemoteCode: o.CountryCode,
		})
		if err != nil {
			unmatched++
			continue
		}
		svc := struct{ ID int64 }{ID: svcID}
		ctry := struct{ ID int64 }{ID: ctryID}

		prod, err := q.UpsertProduct(ctx, db.UpsertProductParams{
			Kind:             db.ProductKindSMSACTIVATION,
			ServiceID:        &svc.ID,
			CountryID:        &ctry.ID,
			VerificationType: db.VerificationTypeSms,
		})
		if err != nil {
			rep.Errors = append(rep.Errors, fmt.Sprintf("ürün %s/%s: %v", o.ServiceCode, o.CountryCode, err))
			continue
		}
		rep.Products++

		// test: sync_integration_test.go#TestSyncOffersPopulatesCatalog
		// Stok = GERÇEK stok. Sağlayıcının havuz sayacı kullanılmaz:
		// canlı ölçümde WhatsApp/TR için havuz 56964 iken gerçek stok 0'dı.
		if err := q.UpsertOffer(ctx, db.UpsertOfferParams{
			ProviderID:   providerID,
			ProductID:    prod.ID,
			CostMicro:    o.Cost.Minor(),
			CostCurrency: db.CurrencyCodeUSD,
			Stock:        int32(o.Stock),
			IsAvailable:  o.Stock > 0,
			SyncedAt:     roundStamp,
		}); err != nil {
			rep.Errors = append(rep.Errors, fmt.Sprintf("teklif %s/%s: %v", o.ServiceCode, o.CountryCode, err))
			continue
		}
		rep.Offers++
	}

	// EŞLEŞMEYEN TEKLİF SAYISI RAPORA GİRER.
	//
	// Eskiden eşleşmeyen satırlar sessizce atlanıyordu. Ülke araması bozulduğunda
	// senkron "0 teklif, 0 hata" diyerek BAŞARILI göründü ve tüm katalog bayat
	// işaretlendi. Sessiz `continue`, bir hata sınıfını görünmez kılar.
	if unmatched > 0 {
		rep.Errors = append(rep.Errors, fmt.Sprintf(
			"%d teklif yerel ürüne eşleştirilemedi — önce boyut senkronu çalıştırın", unmatched))
	}
	// Hiç teklif yazılmadıysa BAYAT İŞARETLEME YAPILMAZ.
	// test: country_ref_integration_test.go#TestUnmatchedOffersDoNotWipeCatalog
	//
	// Aksi halde tek bir eşleştirme hatası tüm katalogu "stok yok" hâline
	// getirir: kullanıcı boş bir site görür, log'da tek bir hata satırı yoktur.
	if rep.Offers == 0 && len(offers) > 0 {
		rep.Errors = append(rep.Errors,
			"sağlayıcı teklif döndürdü ama hiçbiri eşleştirilemedi — katalog KORUNDU, bayat işaretleme atlandı")
		rep.Duration = s.clock.Now().Sub(start)
		slog.Error("teklif senkronu eşleştiremedi",
			"provider", prov.Name, "gelen", len(offers), "eslesen", 0)
		return rep, nil
	}

	// Bu turda görülmeyenleri "yok" işaretle.
	n, err := q.MarkStaleOffersUnavailable(ctx, db.MarkStaleOffersUnavailableParams{
		ProviderID: providerID, SyncedAt: roundStamp,
	})
	if err != nil {
		rep.Errors = append(rep.Errors, fmt.Sprintf("bayat teklifler: %v", err))
	}
	rep.StaleMarked = n

	rep.Duration = s.clock.Now().Sub(start)
	slog.Info("teklif senkronu tamam",
		"provider", prov.Name, "offers", rep.Offers, "stale", rep.StaleMarked,
		"errors", len(rep.Errors), "duration", rep.Duration)
	return rep, nil
}

// SyncProviderBalance sağlayıcıdaki bakiyemizi günceller ve düşükse uyarır.
func (s *Service) SyncProviderBalance(ctx context.Context, providerID int64) error {
	q := s.tx.Queries()
	prov, err := q.GetProvider(ctx, providerID)
	if err != nil {
		return apperr.ErrNotFound
	}
	adapter, err := s.registry.Resolve(string(prov.Protocol))
	if err != nil {
		return apperr.Internal(err)
	}
	creds, err := s.creds(prov)
	if err != nil {
		return apperr.Internal(err)
	}

	bal, err := adapter.GetBalance(ctx, creds)
	if err != nil {
		return apperr.ErrProviderUnavailable.Wrap(err)
	}
	if err := q.UpdateProviderBalance(ctx, db.UpdateProviderBalanceParams{
		ID: providerID, AccountBalanceMicro: bal.Minor(),
	}); err != nil {
		return apperr.Internal(err)
	}

	// Sağlayıcı bakiyesi tükenirse SATIŞ DURUR. Bu sessizce olmamalı.
	const lowBalanceMicro = 5_000_000 // 5 USD
	if bal.Minor() < lowBalanceMicro {
		slog.Error("SAĞLAYICI BAKİYESİ DÜŞÜK — satış durabilir",
			"provider", prov.Name, "balance", bal.String())
	}
	return nil
}

// interpretCountry sağlayıcının ülke kaydını yerel alanlara çevirir.
//
// 🔴 ISO2, SAĞLAYICININ KODUNDAN TÜRETİLMEZ.
//
// Eskiden `iso = rc.RemoteCode` idi. FakeProvider zaten ISO2 kullandığı için
// testler geçiyordu; HeroSMS sayısal kimlik veriyor ve veritabanına
// `iso2 = '62'` gibi satırlar yazıldı — Türkçe ad ve telefon kodu olmadan.
// Sağlayıcının kodu `provider_dimension_maps`e aittir.
//
// ISO2 artık `country_reference` tablosundan, İNGİLİZCE ADA göre çözülür.
// Çözülemezse boş dönülür ve çağıran ülkeyi ATLAR — uydurma bir kod yazmaz.
//
// test: sync_integration_test.go#TestCountryIsoComesFromReferenceNotProviderCode
func (s *Service) interpretCountry(
	ctx context.Context, q *db.Queries, rc port.RemoteDimension,
) (iso, nameTR, phone string, rent bool) {
	if v, ok := rc.Extra["rent"].(bool); ok {
		rent = v
	}

	ref, err := q.ResolveCountryRef(ctx, rc.Name)
	if err != nil {
		return "", "", "", rent
	}
	iso, nameTR, phone = ref.Iso2, ref.NameTr, ref.PhoneCode

	// Sağlayıcı daha iyi bir Türkçe ad veya telefon kodu verdiyse o kazanır.
	if v, ok := rc.Extra["nameTR"].(string); ok && v != "" {
		nameTR = v
	}
	if v, ok := rc.Extra["phoneCode"].(string); ok && v != "" {
		phone = v
	}
	return iso, nameTR, phone, rent
}
