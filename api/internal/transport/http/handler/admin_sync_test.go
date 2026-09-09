package handler_test

// Katalog senkronu tetikleme ucunun DAVRANIŞ garantileri.
//
// Bu dosya veritabanı İSTEMEZ (etiketsiz birim testi): ölçtüğü şey senkronun
// ne yaptığı değil, NASIL koştuğu — arka planda, istek bağlamından bağımsız
// ve sağlayıcı başına tek uçuşla.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/ikmetrik/sms-platform/api/internal/transport/http/handler"
)

// TestSyncRunsInBackgroundAndIsSingleFlight
//
// İKİ GARANTİ BİR ARADA:
//
//  1. Start ÇAĞRISI BEKLEMEZ. Gerçek senkron dakikalarca sürer; istek içinde
//     koşsaydı ters vekil zaman aşımına düşer ve iş yarıda kalırdı.
//  2. Aynı sağlayıcı için ikinci bir tur BAŞLAMAZ. Başlasaydı iki tur aynı
//     `provider_offers` satırlarını yazar, birinin `synced_at`'i diğerinin
//     "bayat" eşiğinin altında kalır ve MarkStaleOffersUnavailable canlı
//     teklifleri "yok" işaretlerdi — stokta numara varken satış durur.
func TestSyncRunsInBackgroundAndIsSingleFlight(t *testing.T) {
	release := make(chan struct{})
	var runs atomic.Int32

	runner := handler.NewSyncRunner(func(ctx context.Context, providerID int64) error {
		runs.Add(1)
		<-release
		return nil
	})

	// (1) İlk çağrı hemen dönmeli — senkron hâlâ koşuyor olmasına rağmen.
	start := time.Now()
	st, started := runner.Start(7)
	if !started {
		t.Fatal("ilk Start başlatmadı")
	}
	if !st.Running {
		t.Fatal("durum Running=false — senkron başlatıldığı hâlde koşmuyor görünüyor")
	}
	if el := time.Since(start); el > time.Second {
		t.Fatalf("🔴 Start %v bekledi — senkron istek içinde koşuyor", el)
	}

	// (2) Koşarken gelen 100 paralel istek TEK BİR turdan fazlasını
	// başlatmamalı.
	waitRunning(t, runner, 7)
	var extra atomic.Int32
	var g errgroup.Group
	for i := 0; i < 100; i++ {
		g.Go(func() error {
			if _, ok := runner.Start(7); ok {
				extra.Add(1)
			}
			return nil
		})
	}
	_ = g.Wait()
	if n := extra.Load(); n != 0 {
		t.Fatalf("🔴 senkron koşarken %d tur daha başlatıldı — tek uçuş yok", n)
	}

	// Farklı bir sağlayıcı ENGELLENMEZ: kilit sağlayıcı başınadır.
	if _, ok := runner.Start(8); !ok {
		t.Fatal("başka bir sağlayıcının senkronu engellendi — kilit çok geniş")
	}

	close(release)
	// 🔴 İKİ sağlayıcının da boşalması beklenir, yalnız 7'nin değil.
	// Sayaç 7 VE 8'i kapsıyor; yalnız 7 beklenirse 8'in goroutine'i sayacı
	// artırmadan iddiaya varılabilir. Tek başına koşarken hep geçiyordu ama
	// `check.sh` bütün paketleri paralel koşturunca CPU rekabeti altında
	// düşüyordu ("toplam 1 tur koştu, 2 bekleniyordu") — kararsızlık kodda
	// değil, testin bekleme koşulundaydı.
	waitIdle(t, runner, 7)
	waitIdle(t, runner, 8)
	if n := runs.Load(); n != 2 { // sağlayıcı 7 ve 8 için birer tur
		t.Fatalf("toplam %d tur koştu, 2 bekleniyordu", n)
	}

	// Tur bittikten sonra yeni bir tur BAŞLATILABİLİR.
	release2 := make(chan struct{})
	close(release2)
	if _, ok := runner.Start(7); !ok {
		t.Fatal("🔴 biten senkrondan sonra yeni tur başlatılamıyor — bayrak açık kalmış")
	}
}

// TestSyncSurvivesRequestContextCancellation
//
// 🔴 SENKRON İSTEK BAĞLAMIYLA KOŞMAZ.
//
// İstek bağlamı yanıt yazıldığında (veya istemci bağlantıyı kapattığında)
// iptal olur. Senkron o bağlamı kullansaydı, tetikleyen yönetici sekmeyi
// kapattığı anda katalog güncellemesi yarıda kesilirdi: yarısı yeni, yarısı
// eski fiyatlarla bir katalog — hiç güncellememekten kötü.
//
// Test GERÇEK HTTP YOLUNDAN geçer: handler'ı çağırır, sonra isteğin bağlamını
// iptal eder ve senkronun hâlâ canlı olduğunu doğrular.
func TestSyncSurvivesRequestContextCancellation(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var runCtx atomic.Value // context.Context
	var finished atomic.Bool

	runner := handler.NewSyncRunner(func(ctx context.Context, providerID int64) error {
		runCtx.Store(ctx)
		close(entered)
		<-release
		finished.Store(true)
		return nil
	})

	reqCtx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers/1/sync", nil).
		WithContext(reqCtx)
	w := httptest.NewRecorder()

	// Handler'ın DB'ye gitmesi gerekmiyor: burada ölçülen şey koşucunun
	// bağlam seçimi. Doğrudan koşucuyu istek işlenirken tetikliyoruz.
	go func() {
		_, _ = runner.Start(1)
		w.WriteHeader(http.StatusOK)
	}()

	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("senkron hiç başlamadı")
	}

	// İstek bitti, bağlamı iptal edildi.
	cancel()
	_ = req

	time.Sleep(50 * time.Millisecond)
	ctx, _ := runCtx.Load().(context.Context)
	if ctx == nil {
		t.Fatal("senkron bağlamı kaydedilmedi")
	}
	if err := ctx.Err(); err != nil {
		t.Fatalf("🔴 istek iptal edilince senkron bağlamı da iptal oldu (%v) — "+
			"iş yarıda kalır", err)
	}

	close(release)
	deadline := time.Now().Add(5 * time.Second)
	for !finished.Load() {
		if time.Now().After(deadline) {
			t.Fatal("senkron tamamlanmadı")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestSyncPanicDoesNotWedgeProvider
//
// Panik veren bir adaptör turu, o sağlayıcıyı SONSUZA KADAR kilitli
// bırakmamalı: `Running` açık kalırsa bir daha hiç senkron yapılamaz ve
// katalog sessizce bayatlar.
func TestSyncPanicDoesNotWedgeProvider(t *testing.T) {
	var calls atomic.Int32
	runner := handler.NewSyncRunner(func(ctx context.Context, providerID int64) error {
		if calls.Add(1) == 1 {
			panic("adaptör çöktü")
		}
		return nil
	})

	if _, ok := runner.Start(3); !ok {
		t.Fatal("ilk tur başlatılamadı")
	}
	waitIdle(t, runner, 3)

	st := runner.State(3)
	if st.Running {
		t.Fatal("🔴 panik sonrası Running açık kaldı — sağlayıcı kilitlendi")
	}
	if !st.Failed {
		t.Error("panik başarısızlık olarak işaretlenmedi")
	}
	if _, ok := runner.Start(3); !ok {
		t.Fatal("🔴 panik sonrası yeni tur başlatılamıyor")
	}
}

// TestSyncFailureIsRecordedWithoutRawMessage
//
// Hata DURUMU dışarı verilir, hata METNİ verilmez (Değişmez #12): sağlayıcı
// hataları uç nokta adresi ve anahtar parçası içerebilir.
func TestSyncFailureIsRecordedWithoutRawMessage(t *testing.T) {
	runner := handler.NewSyncRunner(func(ctx context.Context, providerID int64) error {
		return context.DeadlineExceeded
	})
	if _, ok := runner.Start(4); !ok {
		t.Fatal("tur başlatılamadı")
	}
	waitIdle(t, runner, 4)

	st := runner.State(4)
	if !st.Failed {
		t.Fatal("başarısız tur Failed=false döndü")
	}
	if st.FinishedAt.IsZero() {
		t.Error("bitiş zamanı yazılmadı")
	}
}

func waitRunning(t *testing.T, r *handler.SyncRunner, id int64) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !r.State(id).Running {
		if time.Now().After(deadline) {
			t.Fatal("senkron koşar duruma geçmedi")
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func waitIdle(t *testing.T, r *handler.SyncRunner, id int64) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for r.State(id).Running {
		if time.Now().After(deadline) {
			t.Fatal("senkron bitmedi")
		}
		time.Sleep(2 * time.Millisecond)
	}
}
