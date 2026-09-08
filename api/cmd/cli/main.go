// Command cli yönetim komutlarını çalıştırır.
//
//	go run ./cmd/cli provider:add    --name=herosms --protocol=HEROSMS_V1 --env-key=HEROSMS_API_KEY
//	go run ./cmd/cli provider:add    --name=fake --protocol=FAKE
//	go run ./cmd/cli catalog:sync    --provider=fake
//	go run ./cmd/cli admin:grant     --email=... --role=admin
//	go run ./cmd/cli wallet:reconcile
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ikmetrik/sms-platform/api/internal/adapter/crypto"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/postgres"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/provider"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/provider/fake"
	"github.com/ikmetrik/sms-platform/api/internal/config"
	"github.com/ikmetrik/sms-platform/api/internal/db"
	"github.com/ikmetrik/sms-platform/api/internal/port"
	"github.com/ikmetrik/sms-platform/api/internal/service/catalog"
	"github.com/ikmetrik/sms-platform/api/internal/service/wallet"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}
	cmd := os.Args[1]
	args := os.Args[2:]

	config.LoadDotEnv()
	cfg, err := config.Load()
	if err != nil {
		fail(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	pool, err := postgres.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		fail(err)
	}
	defer pool.Close()

	box, err := crypto.New(cfg.EncryptionKey)
	if err != nil {
		fail(err)
	}

	app := &appCtx{ctx: ctx, cfg: cfg, q: db.New(pool), tx: postgres.NewTxRunner(pool), box: box}

	switch cmd {
	case "provider:add":
		err = app.providerAdd(args)
	case "provider:list":
		err = app.providerList()
	case "catalog:sync":
		err = app.catalogSync(args)
	case "admin:grant":
		err = app.adminGrant(args)
	case "wallet:reconcile":
		err = app.walletReconcile()
	case "help", "-h", "--help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "bilinmeyen komut: %s\n\n", cmd)
		usage()
		os.Exit(1)
	}
	if err != nil {
		fail(err)
	}
}

type appCtx struct {
	ctx context.Context
	cfg *config.Config
	q   *db.Queries
	tx  *postgres.TxRunner
	box *crypto.SecretBox
}

// providerAdd yeni bir sağlayıcı kaydeder.
//
// API anahtarı ORTAM DEĞİŞKENİNDEN okunur, komut satırından DEĞİL:
// komut satırı argümanları kabuk geçmişine ve `ps` çıktısına düşer.
func (a *appCtx) providerAdd(args []string) error {
	fs := flag.NewFlagSet("provider:add", flag.ExitOnError)
	name := fs.String("name", "", "sağlayıcı adı")
	proto := fs.String("protocol", "", "protokol: FAKE | HEROSMS_V1 | FIVE_SIM")
	baseURL := fs.String("base-url", "", "temel URL")
	envKey := fs.String("env-key", "", "API anahtarının okunacağı ortam değişkeni adı")
	priority := fs.Int("priority", 100, "tercih sırası (küçük = önce)")
	multiplier := fs.String("cost-multiplier", "1.0", "maliyet düzeltme çarpanı")
	active := fs.Bool("active", true, "hemen etkin olsun mu")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *name == "" || *proto == "" {
		return fmt.Errorf("--name ve --protocol zorunlu")
	}

	var enc []byte
	if *envKey != "" {
		raw := strings.TrimSpace(os.Getenv(*envKey))
		if raw == "" {
			return fmt.Errorf("ortam değişkeni %s boş veya tanımsız", *envKey)
		}
		var err error
		enc, err = a.box.SealString(raw)
		if err != nil {
			return err
		}
		fmt.Printf("  API anahtarı şifrelendi: %s (%d bayt)\n", crypto.Mask(raw), len(enc))
	}

	var mult pgNumeric
	if err := mult.Scan(*multiplier); err != nil {
		return fmt.Errorf("geçersiz çarpan %q: %w", *multiplier, err)
	}

	p, err := a.q.CreateProvider(a.ctx, db.CreateProviderParams{
		Name:           *name,
		Protocol:       db.ProviderProtocol(*proto),
		BaseUrl:        *baseURL,
		ApiKeyEnc:      enc,
		IsActive:       *active,
		Priority:       int32(*priority),
		CostMultiplier: mult,
		Capabilities:   []byte(`["SMS_ACTIVATION"]`),
	})
	if err != nil {
		return err
	}
	fmt.Printf("✓ sağlayıcı eklendi: id=%d name=%s protocol=%s active=%v\n",
		p.ID, p.Name, p.Protocol, p.IsActive)
	return nil
}

func (a *appCtx) providerList() error {
	rows, err := a.q.ListActiveProviders(a.ctx)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		fmt.Println("(etkin sağlayıcı yok)")
		return nil
	}
	fmt.Printf("%-4s %-16s %-14s %-10s %s\n", "ID", "AD", "PROTOKOL", "ÖNCELİK", "BAKİYE")
	for _, p := range rows {
		fmt.Printf("%-4d %-16s %-14s %-10d %.6f USD\n",
			p.ID, p.Name, p.Protocol, p.Priority, float64(p.AccountBalanceMicro)/1e6)
	}
	return nil
}

func (a *appCtx) catalogSync(args []string) error {
	fs := flag.NewFlagSet("catalog:sync", flag.ExitOnError)
	name := fs.String("provider", "", "sağlayıcı adı")
	offersOnly := fs.Bool("offers-only", false, "yalnız fiyat/stok senkronu")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *name == "" {
		return fmt.Errorf("--provider zorunlu")
	}

	prov, err := a.q.GetProviderByName(a.ctx, *name)
	if err != nil {
		return fmt.Errorf("sağlayıcı %q bulunamadı", *name)
	}

	reg := provider.NewRegistry()
	reg.Register(fake.New(port.RealClock{}))
	// HeroSMS adaptörü hazır olduğunda buraya kaydedilecek.

	svc := catalog.New(catalog.Deps{
		TxRunner: a.tx, Registry: reg, Secrets: a.box, Clock: port.RealClock{},
	})

	if !*offersOnly {
		rep, err := svc.SyncDimensions(a.ctx, prov.ID)
		if err != nil {
			return err
		}
		fmt.Printf("✓ boyutlar: %d ülke, %d servis (%v)\n", rep.Countries, rep.Services, rep.Duration.Round(time.Millisecond))
		for _, e := range rep.Errors {
			fmt.Printf("  ! %s\n", e)
		}
	}

	rep, err := svc.SyncOffers(a.ctx, prov.ID)
	if err != nil {
		return err
	}
	fmt.Printf("✓ teklifler: %d ürün, %d teklif, %d bayat (%v)\n",
		rep.Products, rep.Offers, rep.StaleMarked, rep.Duration.Round(time.Millisecond))
	for _, e := range rep.Errors {
		fmt.Printf("  ! %s\n", e)
	}

	if err := svc.SyncProviderBalance(a.ctx, prov.ID); err != nil {
		fmt.Printf("  ! bakiye senkronu: %v\n", err)
	}
	return nil
}

func (a *appCtx) adminGrant(args []string) error {
	fs := flag.NewFlagSet("admin:grant", flag.ExitOnError)
	email := fs.String("email", "", "kullanıcı e-postası")
	role := fs.String("role", "admin", "rol adı")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *email == "" {
		return fmt.Errorf("--email zorunlu")
	}
	u, err := a.q.GetUserByEmail(a.ctx, *email)
	if err != nil {
		return fmt.Errorf("kullanıcı bulunamadı: %s", *email)
	}
	r, err := a.q.GetRoleByName(a.ctx, *role)
	if err != nil {
		return fmt.Errorf("rol bulunamadı: %s", *role)
	}
	if err := a.q.AssignRole(a.ctx, db.AssignRoleParams{UserID: u.ID, RoleID: r.ID}); err != nil {
		return err
	}
	fmt.Printf("✓ %s kullanıcısına %q rolü verildi\n", *email, *role)
	return nil
}

func (a *appCtx) walletReconcile() error {
	rep, err := wallet.New(a.tx).Reconcile(a.ctx, 100)
	if err != nil {
		return err
	}
	if !rep.HasDrift() {
		fmt.Println("✓ mutabakat tamam — sapma yok")
		return nil
	}
	fmt.Printf("🔴 %d kullanıcıda SAPMA VAR:\n", len(rep.Drifts))
	for _, d := range rep.Drifts {
		fmt.Printf("  user=%d %s  önbellek=%s defter=%s sapma=%s\n",
			d.UserID, d.Email, d.CachedBalance, d.LedgerBalance, d.Amount)
	}
	return fmt.Errorf("mutabakat sapması tespit edildi — otomatik düzeltilmez, inceleyin")
}

func usage() {
	fmt.Print(`Kullanım: cli <komut> [seçenekler]

Komutlar:
  provider:add       Sağlayıcı ekle
                     --name --protocol --base-url --env-key --priority --cost-multiplier
                     API anahtarı ORTAM DEĞİŞKENİNDEN okunur (--env-key), komut
                     satırından değil: argümanlar kabuk geçmişine ve ps çıktısına düşer.
  provider:list      Etkin sağlayıcıları listele
  catalog:sync       Katalog senkronu   --provider [--offers-only]
  admin:grant        Rol ata            --email --role
  wallet:reconcile   Defter mutabakatı

Örnek:
  go run ./cmd/cli provider:add --name=fake --protocol=FAKE
  go run ./cmd/cli catalog:sync --provider=fake
`)
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "hata: %v\n", err)
	os.Exit(1)
}
