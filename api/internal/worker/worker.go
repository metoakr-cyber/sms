// Package worker periyodik arka plan işlerini yürütür.
//
// NEDEN AYRI BİR SÜREÇ DEĞİL (henüz): tek bir VPS'te iki süreç yönetmek,
// kazandırdığından fazla operasyon yükü getiriyor. İşler sunucu süreci içinde
// çalışır ama TEK ÖRNEK garantisi Redis kilidiyle sağlanır — ikinci bir sunucu
// eklendiğinde iki poller aynı siparişi işlemez.
//
// Her iş şu üç kurala uyar:
//  1. Kendi hatasında DİĞER İŞLERİ ETKİLEMEZ (panik yakalanır).
//  2. Bir turda işlenen kayıt sayısı SINIRLIDIR — bir birikim tüm turu
//     kilitlemesin.
//  3. İdempotenttir: aynı tur iki kez çalışsa aynı sonucu verir.
package worker

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"
)

// Job periyodik bir iş.
type Job struct {
	Name     string
	Every    time.Duration
	Run      func(ctx context.Context) error
	// RunAtStart açılışta hemen bir kez çalıştırılsın mı.
	//
	// Kur senkronu için ZORUNLU: sunucu açıldığında kur bayatsa hizmet
	// kapalıdır ve ilk turu beklemek "site satış yapmıyor" demektir.
	RunAtStart bool
}

// Runner işleri zamanlar.
type Runner struct {
	jobs []Job
	wg   sync.WaitGroup
}

func New(jobs ...Job) *Runner { return &Runner{jobs: jobs} }

// Start tüm işleri başlatır. ctx iptal edilene kadar çalışır.
func (r *Runner) Start(ctx context.Context) {
	for _, j := range r.jobs {
		if j.Run == nil || j.Every <= 0 {
			slog.Error("geçersiz iş tanımı — atlanıyor", "job", j.Name)
			continue
		}
		r.wg.Add(1)
		go r.loop(ctx, j)
	}
}

// Wait tüm işlerin durmasını bekler.
func (r *Runner) Wait() { r.wg.Wait() }

func (r *Runner) loop(ctx context.Context, j Job) {
	defer r.wg.Done()

	if j.RunAtStart {
		r.once(ctx, j)
	}

	t := time.NewTicker(j.Every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			slog.Info("iş durduruldu", "job", j.Name)
			return
		case <-t.C:
			r.once(ctx, j)
		}
	}
}

// once bir turu çalıştırır ve HER TÜRLÜ hatayı yakalar.
//
// PANİK YAKALANIR: bir işteki nil pointer, tüm sunucuyu düşürmemeli.
// Aynı gerekçeyle hata da yutulmaz, log'lanır: sessizce duran bir iş,
// haftalar sonra "iadeler işlenmemiş" olarak fark edilir.
func (r *Runner) once(ctx context.Context, j Job) {
	defer func() {
		if p := recover(); p != nil {
			slog.Error("iş panik verdi — sunucu ayakta kalıyor",
				"job", j.Name, "panic", fmt.Sprint(p), "stack", string(debug.Stack()))
		}
	}()

	start := time.Now()
	if err := j.Run(ctx); err != nil {
		slog.Error("iş başarısız", "job", j.Name, "err", err, "duration", time.Since(start))
		return
	}
	slog.Debug("iş tamam", "job", j.Name, "duration", time.Since(start))
}
