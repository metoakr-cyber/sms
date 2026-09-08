package catalog

// Kiralık katalog senkronu.
//
// AKTİVASYONDAN AYRI BİR TUR çünkü sağlayıcı toplu kiralık katalog sunmuyor:
// `getRentServicesAndCountries` servis listesini BOŞ döndürüyor ve fiyatlar
// yalnız servis bazında (`serviceCountRent&service=X`) alınabiliyor.
//
// Aktivasyon tarafı TEK istekle 20.000 teklif getiriyor; kiralık taraf servis
// başına bir istek istiyor. Bu farkı gizlemek yerine ayrı bir tur yapıyoruz —
// aksi hâlde her aktivasyon senkronu yüzlerce ek istek atardı.

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/ikmetrik/sms-platform/api/internal/db"
	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
	"github.com/ikmetrik/sms-platform/api/internal/port"
)

// rentalConcurrency eşzamanlı servis sorgusu sayısı.
//
// 810 servisi sırayla sormak ~10 dakika sürer. Sekiz paralel istek sağlayıcıyı
// zorlamadan bunu ~1,5 dakikaya indiriyor. Daha yükseği `CHANNELS_LIMIT`
// riskini artırır.
const rentalConcurrency = 8

// SyncRentals kiralık fiyat ve stoklarını senkronlar.
func (s *Service) SyncRentals(ctx context.Context, providerID int64) (SyncReport, error) {
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
	rental, ok := adapter.(port.RentalProvider)
	if !ok {
		// Sağlayıcı kiralık desteklemiyor. HATA DEĞİLDİR — rapora yazılır ve
		// geçilir. Hata saymak, tek sağlayıcılı bir kurulumda her senkronu
		// kırmızı gösterirdi.
		rep.Errors = append(rep.Errors, "sağlayıcı kiralık desteklemiyor")
		return rep, nil
	}
	creds, err := s.creds(prov)
	if err != nil {
		return rep, apperr.Internal(err)
	}

	// Hangi servisler? Aktivasyon tarafında zaten eşleştirilmiş olanlar.
	// Eşleştirilmemiş bir servisi sormanın anlamı yok: yanıtı yerel bir
	// ürüne bağlayamayız.
	maps, err := q.ListDimensionMaps(ctx, db.ListDimensionMapsParams{
		ProviderID: providerID, Dimension: string(port.DimService),
	})
	if err != nil {
		return rep, apperr.Internal(err)
	}

	roundStamp := s.clock.Now()

	type result struct {
		offers []port.RentOffer
		err    error
	}
	results := make([]result, len(maps))

	var wg sync.WaitGroup
	sem := make(chan struct{}, rentalConcurrency)
	for i, m := range maps {
		wg.Add(1)
		go func(i int, remoteCode string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			// Tek servis için zaman aşımı: yavaş bir servis tüm turu
			// kilitlemesin.
			cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			offers, err := rental.ListRentOffers(cctx, creds, remoteCode)
			results[i] = result{offers: offers, err: err}
		}(i, m.RemoteCode)
	}
	wg.Wait()

	var unmatched, failed int
	for i, r := range results {
		if r.err != nil {
			failed++
			continue
		}
		for _, o := range r.offers {
			if o.Stock <= 0 {
				continue
			}
			svcID, err := q.ResolveDimensionLocal(ctx, db.ResolveDimensionLocalParams{
				ProviderID: providerID, Dimension: string(port.DimService),
				RemoteCode: maps[i].RemoteCode,
			})
			if err != nil {
				unmatched++
				continue
			}
			ctryID, err := q.ResolveDimensionLocal(ctx, db.ResolveDimensionLocalParams{
				ProviderID: providerID, Dimension: string(port.DimCountry),
				RemoteCode: o.CountryCode,
			})
			if err != nil {
				unmatched++
				continue
			}

			// Süre DAKİKA olarak saklanır (şema öyle); sağlayıcı SAAT veriyor.
			// Çevrimi tek yerde yaparız — iki birim arasında gidip gelmek
			// er geç 60 kat hataya yol açar.
			minutes := int32(o.DurationHours * 60)

			prod, err := q.UpsertRentalProduct(ctx, db.UpsertRentalProductParams{
				ServiceID: &svcID, CountryID: &ctryID, DurationMinutes: &minutes,
			})
			if err != nil {
				rep.Errors = append(rep.Errors,
					fmt.Sprintf("kiralık ürün %s/%s/%dsa: %v",
						maps[i].RemoteCode, o.CountryCode, o.DurationHours, err))
				continue
			}
			rep.Products++

			if err := q.UpsertOffer(ctx, db.UpsertOfferParams{
				ProviderID: providerID, ProductID: prod.ID,
				CostMicro: o.Cost.Minor(), CostCurrency: db.CurrencyCodeUSD,
				Stock: int32(o.Stock), IsAvailable: o.Stock > 0,
				SyncedAt: roundStamp,
			}); err != nil {
				rep.Errors = append(rep.Errors, fmt.Sprintf("kiralık teklif: %v", err))
				continue
			}
			rep.Offers++
		}
	}

	if failed > 0 {
		rep.Errors = append(rep.Errors,
			fmt.Sprintf("%d servis sorgulanamadı", failed))
	}
	if unmatched > 0 {
		rep.Errors = append(rep.Errors,
			fmt.Sprintf("%d kiralık teklif eşleştirilemedi", unmatched))
	}

	// BAYAT İŞARETLEME BURADA YAPILMAZ.
	// test: country_ref_integration_test.go#TestRentalSyncDoesNotWipeActivationOffers
	//
	// Aktivasyon turu da aynı `provider_offers` tablosunu kullanıyor ve kendi
	// damgasıyla bayat işaretliyor. Kiralık turu da işaretlerse, iki tur
	// birbirinin tekliflerini "görülmedi" sayıp karşılıklı siler.
	// Kiralık stok düşüşü, stoksuz tekliflerin hiç yazılmamasıyla yansır.

	rep.Duration = s.clock.Now().Sub(start)
	slog.Info("kiralık senkronu tamam",
		"provider", prov.Name, "services", len(maps),
		"products", rep.Products, "offers", rep.Offers,
		"errors", len(rep.Errors), "duration", rep.Duration)
	return rep, nil
}
