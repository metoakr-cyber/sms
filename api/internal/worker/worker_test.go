package worker

// Runner'ın SAF birim testleri: veritabanı yok, ağ yok.
//
// Bu paketin davranışı bugüne kadar hiç sınanmamıştı ve altı iş ona güveniyor:
// bir işteki panik diğerlerini durdurmamalı, tanımı eksik bir iş sessizce
// atlanmalı (bağımlılığı olmayan işi devre dışı bırakmanın kabul edilen yolu).

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// waitFor koşul sağlanana kadar kısa aralıklarla bekler.
//
// Sabit bir `time.Sleep` yerine: yavaş bir CI makinesinde sabit uyku testi
// kararsız yapar, hızlı makinede boşuna bekletir.
func waitFor(t *testing.T, timeout time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(2 * time.Millisecond)
	}
	return cond()
}

// TestJobPanicDoesNotStopOtherJobs
//
// SÖZLEŞME (worker.go once): bir işteki panik sunucuyu ve diğer işleri
// düşürmez. `order-poller` ile `provider-refund-retry` bu davranışa güvenir:
// sağlayıcıdan gelen beklenmedik bir yanıt nil pointer'a yol açarsa iade işi
// durmamalıdır.
func TestJobPanicDoesNotStopOtherJobs(t *testing.T) {
	var healthy atomic.Int32
	var panics atomic.Int32

	r := New(
		Job{Name: "panik", Every: 5 * time.Millisecond, Run: func(context.Context) error {
			panics.Add(1)
			panic("kasıtlı panik")
		}},
		Job{Name: "saglikli", Every: 5 * time.Millisecond, Run: func(context.Context) error {
			healthy.Add(1)
			return nil
		}},
	)

	ctx, cancel := context.WithCancel(context.Background())
	r.Start(ctx)
	ok := waitFor(t, 2*time.Second, func() bool {
		return panics.Load() >= 2 && healthy.Load() >= 2
	})
	cancel()
	r.Wait()

	if !ok {
		t.Fatalf("panik=%d saglikli=%d — panik veren iş sağlıklı işi durdurdu",
			panics.Load(), healthy.Load())
	}
}

// TestJobErrorDoesNotStopTheLoop
//
// Hata yutulmaz ama iş DURMAZ: sessizce duran bir iş haftalar sonra
// "iadeler işlenmemiş" olarak fark edilir.
func TestJobErrorDoesNotStopTheLoop(t *testing.T) {
	var runs atomic.Int32
	r := New(Job{Name: "hatali", Every: 5 * time.Millisecond, Run: func(context.Context) error {
		runs.Add(1)
		return errors.New("kasıtlı hata")
	}})

	ctx, cancel := context.WithCancel(context.Background())
	r.Start(ctx)
	ok := waitFor(t, 2*time.Second, func() bool { return runs.Load() >= 3 })
	cancel()
	r.Wait()

	if !ok {
		t.Fatalf("iş %d kez koştu — hata döngüyü durdurmuş", runs.Load())
	}
}

// TestJobWithoutRunOrIntervalIsSkipped
//
// SÖZLEŞME (worker.go Start): `Every <= 0` veya `Run == nil` olan iş ATLANIR.
// `orderPoller`/`refundRetry`, bağımlılığı yokken tam olarak böyle bir iş
// döndürür (`Every: 0`); o dalın gerçekten sessiz kaldığının kanıtı budur.
// Atlanmasaydı `time.NewTicker(0)` panikler ve sunucu açılışta çökerdi.
func TestJobWithoutRunOrIntervalIsSkipped(t *testing.T) {
	var runs atomic.Int32
	inc := func(context.Context) error { runs.Add(1); return nil }

	r := New(
		Job{Name: "araliksiz", Every: 0, Run: inc},
		Job{Name: "govdesiz", Every: time.Millisecond, Run: nil},
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.Start(ctx)
	// Hiç goroutine açılmadıysa Wait ANINDA döner.
	done := make(chan struct{})
	go func() { r.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("geçersiz iş için goroutine açılmış — Wait dönmedi")
	}
	if runs.Load() != 0 {
		t.Errorf("geçersiz iş %d kez koştu, 0 bekleniyordu", runs.Load())
	}
}

// TestRunAtStartRunsBeforeFirstTick
//
// `fx-sync` buna dayanır: sunucu açıldığında kur bayatsa ilk turu beklemek
// 10 dakika satış kaybıdır.
func TestRunAtStartRunsBeforeFirstTick(t *testing.T) {
	var runs atomic.Int32
	r := New(Job{
		Name: "acilista", Every: time.Hour, RunAtStart: true,
		Run: func(context.Context) error { runs.Add(1); return nil },
	})

	ctx, cancel := context.WithCancel(context.Background())
	r.Start(ctx)
	ok := waitFor(t, 2*time.Second, func() bool { return runs.Load() == 1 })
	cancel()
	r.Wait()

	if !ok {
		t.Fatalf("açılışta çalışması gereken iş %d kez koştu, 1 bekleniyordu", runs.Load())
	}
}
