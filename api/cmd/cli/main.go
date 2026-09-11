// Command cli yönetim komutlarını çalıştırır.
//
//	go run ./cmd/cli provider:add    --name=herosms --protocol=HEROSMS_V1 --env-key=HEROSMS_API_KEY
//	go run ./cmd/cli provider:add    --name=fake --protocol=FAKE
//	go run ./cmd/cli catalog:sync    --provider=fake
//	go run ./cmd/cli user:create     --email=... --username=... --env-password=ADMIN_PAROLA --role=admin
//	go run ./cmd/cli admin:grant     --email=... --role=admin
//	go run ./cmd/cli wallet:credit    --email=... --by=admin@... --amount=1000 --note="test bakiyesi" --key=t1
//	go run ./cmd/cli seed            --provider=herosms
//	go run ./cmd/cli wallet:reconcile
package main

import (
	"context"
	"flag"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ikmetrik/sms-platform/api/internal/adapter/crypto"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/fx"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/postgres"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/provider"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/provider/fake"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/provider/herosms"
	"github.com/ikmetrik/sms-platform/api/internal/config"
	"github.com/ikmetrik/sms-platform/api/internal/db"
	domauth "github.com/ikmetrik/sms-platform/api/internal/domain/auth"
	"github.com/ikmetrik/sms-platform/api/internal/domain/money"
	"github.com/ikmetrik/sms-platform/api/internal/port"
	auditsvc "github.com/ikmetrik/sms-platform/api/internal/service/audit"
	"github.com/ikmetrik/sms-platform/api/internal/service/catalog"
	pricingsvc "github.com/ikmetrik/sms-platform/api/internal/service/pricing"
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

	app := &appCtx{ctx: ctx, cfg: cfg, q: db.New(pool), tx: postgres.NewTxRunner(pool),
		box: box, fx: fx.NewTCMB(), pool: pool}

	switch cmd {
	case "provider:add":
		err = app.providerAdd(args)
	case "provider:list":
		err = app.providerList()
	case "catalog:sync":
		err = app.catalogSync(args)
	case "fx:sync":
		err = app.fxSync()
	case "catalog:rentals":
		err = app.catalogRentals(args)
	case "catalog:icon":
		err = app.catalogIcon(args)
	case "seed":
		err = app.seed(args)
	case "user:create":
		err = app.userCreate(args)
	case "admin:grant":
		err = app.adminGrant(args)
	case "wallet:credit":
		err = app.walletCredit(args)
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
	fx  port.FXProvider
	// pool YALNIZ `seed` raporundaki sayımlar için. İş mantığı sqlc üzerinden
	// gider; burada amaç "kaç kayıt var" sorusuna cevap vermek ve bunun için
	// queries/*.sql'e rapor sorguları eklemek gereksiz yük olurdu.
	pool *pgxpool.Pool
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
	reg.Register(herosms.New())
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

// seed kurulum sonrası veriyi tazeler ve sistemin SATIŞ YAPABİLİR olup
// olmadığını rapor eder.
//
// ══════════════════════════════════════════════════════════════════════════
// NE TOHUMLAR — VE NEYİ TOHUMLAMAZ
// ══════════════════════════════════════════════════════════════════════════
// Şema tohumları zaten migration'larda: GLOBAL fiyat kuralı (00006) ve iki
// ödeme yöntemi (00008 — PASİF ve IBAN'ı boş; operatör doldurmadan
// etkinleştirilemez, doğrusu da budur). Burada tekrarlanmazlar.
//
// Bu komut DIŞ DÜNYADAN gelen veriyi çeker: döviz kuru ve sağlayıcı katalogu.
// İkisi de kurulum anında yoktur ve ikisi de olmadan tek bir numara bile
// satılamaz — kur yoksa fiyat hesaplanamaz, katalog yoksa gösterilecek servis
// yoktur. Tekrar çalıştırılabilir: ikisi de üzerine yazar, kopya üretmez.
//
// 🔴 RAPOR KISMI SÜS DEĞİL. Bu sistemde "kurulum başarılı" ile "satış
// yapılabilir" AYNI ŞEY DEĞİLDİR: sağlayıcısı eklenmemiş, kuru çekilememiş ya
// da ödeme yöntemi pasif bir kurulum sorunsuz açılır, sağlıklı görünür ve
// KAZANÇ ÜRETMEZ. Rapor o boşluğu kurulum anında görünür kılar.
func (a *appCtx) seed(args []string) error {
	fs := flag.NewFlagSet("seed", flag.ExitOnError)
	saglayici := fs.String("provider", "", "yalnız bu sağlayıcıyı senkronla (boş = etkin olanların hepsi)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	fmt.Println("── Dış veri ──")
	if err := a.fxSync(); err != nil {
		fmt.Printf("  ! kur çekilemedi: %v\n", err)
	}

	// `ListActiveProviders` zaten yalnız etkin olanları döner; ayrıca
	// `IsActive` kontrolü yapmaya gerek yok.
	provs, err := a.q.ListActiveProviders(a.ctx)
	if err != nil {
		return err
	}
	// Başarı ve başarısızlık AYRI sayılır. Tek bir sayaçla "senkronlanan == 0"
	// hem "hiç sağlayıcı yok" hem "sağlayıcı var ama senkron düştü" demekti ve
	// ekrana ikincisinde de "etkin sağlayıcı yok" yazıyordu — bir satır yukarıda
	// sağlayıcının adıyla hata basıldığı hâlde. Ölçüldü: gerçek kurulum,
	// 11 Eylül 2026, BAD_API_KEY.
	senkronlanan, senkronDusen := 0, 0
	for _, p := range provs {
		if *saglayici != "" && p.Name != *saglayici {
			continue
		}
		if err := a.catalogSync([]string{"--provider=" + p.Name}); err != nil {
			fmt.Printf("  ! %s katalogu senkronlanamadı: %v\n", p.Name, err)
			senkronDusen++
			continue
		}
		senkronlanan++
	}
	switch {
	case len(provs) == 0:
		fmt.Println("  ! etkin sağlayıcı yok — katalog senkronu atlandı")
	case senkronDusen > 0:
		fmt.Printf("  ! %d sağlayıcının katalogu alınamadı — stok ve fiyat BAYAT\n", senkronDusen)
	}

	say := func(sorgu string) int64 {
		var n int64
		if err := a.pool.QueryRow(a.ctx, sorgu).Scan(&n); err != nil {
			return -1
		}
		return n
	}
	satir := func(tamam bool, ad, ipucu string) bool {
		if tamam {
			fmt.Printf("  ✓ %s\n", ad)
			return true
		}
		fmt.Printf("  ✗ %s — %s\n", ad, ipucu)
		return false
	}

	fmt.Println("\n── Satışa hazır mı ──")
	hazir := true
	hazir = satir(say("SELECT count(*) FROM users u JOIN user_roles ur ON ur.user_id=u.id JOIN roles r ON r.id=ur.role_id WHERE r.name='admin'") > 0,
		"yönetici hesabı", "cli user:create --email=... --username=... --env-password=... --role=admin") && hazir
	// 🔴 ANAHTARIN VAR OLMASI YETMEZ, ÇALIŞMASI GEREKİR.
	//
	// Bu satır yalnız `length(api_key_enc) > 0` sorgusuna bakıyordu ve
	// sağlayıcı iki satır yukarıda BAD_API_KEY ile reddedilmişken ✓ basıyordu.
	// Rapor tam da "kurulum açıldı" ile "satış yapılabilir" farkını göstermek
	// için var; orada yanlış yeşil vermek raporun kendi amacını bozar.
	// Senkron bu çalıştırmada düştüyse anahtar çalışmıyor demektir.
	//
	// test: seed_hazirlik_test.go#TestSaglayiciSatiriSenkronDustugundeKirmiziOlur
	anahtarVar := say("SELECT count(*) FROM providers WHERE is_active AND api_key_enc IS NOT NULL AND length(api_key_enc)>0") > 0
	hazir = satir(saglayiciHazirMi(anahtarVar, senkronDusen),
		"etkin sağlayıcı + API anahtarı",
		func() string {
			if anahtarVar && senkronDusen > 0 {
				return "anahtar kayıtlı ama sağlayıcı REDDETTİ — /yonetim/saglayicilar → API anahtarı"
			}
			return "cli provider:add ... --env-key=HEROSMS_API_KEY"
		}()) && hazir
	hazir = satir(say("SELECT count(*) FROM fx_rates WHERE fetched_at > now() - interval '1 day'") > 0,
		"güncel döviz kuru", "cli fx:sync") && hazir
	hazir = satir(say("SELECT count(*) FROM provider_offers WHERE is_available AND stock > 0") > 0,
		"stoklu ürün", "cli catalog:sync --provider=<ad>") && hazir
	hazir = satir(say("SELECT count(*) FROM pricing_rules WHERE is_active AND scope='GLOBAL'") > 0,
		"GLOBAL fiyat kuralı", "migration 00006 tohumlar — silinmişse /yonetim/fiyatlar") && hazir
	// Ödeme yöntemi AYRI tutulur: olmadan sistem satar ama kullanıcı bakiye
	// YÜKLEYEMEZ. "hazır" bayrağını düşürmez, ama sessizce de geçilmez.
	satir(say("SELECT count(*) FROM deposit_methods WHERE is_active") > 0,
		"etkin ödeme yöntemi", "/yonetim/odeme-yontemleri — IBAN/cüzdan doldurulmadan etkinleştirilemez")

	fmt.Println()
	if hazir {
		fmt.Println("  Sistem satışa hazır.")
	} else {
		fmt.Println("  Yukarıdaki ✗ maddeleri tamamlanmadan satış YAPILAMAZ.")
	}
	return nil
}

// userCreate kurulum sırasında ilk hesabı açar.
//
// ══════════════════════════════════════════════════════════════════════════
// NEDEN VAR — kurulumun tavuk-yumurta sorunu
// ══════════════════════════════════════════════════════════════════════════
// `admin:grant` var olan bir kullanıcıya rol verir; ama taze bir sunucuda
// HİÇ kullanıcı yoktur. Kayıt akışından geçmek e-posta doğrulaması ister,
// doğrulama e-postası da henüz yapılandırılmamış bir posta sağlayıcısından
// gider. Kurulumun ilk hesabı bu yüzden komut satırından açılır.
//
// 🔴 PAROLA ORTAM DEĞİŞKENİNDEN OKUNUR, KOMUT SATIRINDAN DEĞİL.
// Komut satırı argümanları kabuk geçmişine ve `ps` çıktısına düşer; bir
// yönetici parolasının oraya yazılması, kurulumun ilk dakikasında kalıcı bir
// sızıntı üretir. `provider:add` API anahtarı için aynı kuralı uyguluyor.
//
// E-posta DOĞRULANMIŞ işaretlenir: bu hesap kurulumu yapan kişinindir ve
// doğrulama bağlantısını alacağı bir posta kutusu henüz kurulmamış olabilir.
// Numara satın alma doğrulanmış e-posta ister (FR-101); işaretlemezsek
// kurucu kendi panelinde hiçbir şey satın alamaz.
func (a *appCtx) userCreate(args []string) error {
	fs := flag.NewFlagSet("user:create", flag.ExitOnError)
	email := fs.String("email", "", "e-posta")
	username := fs.String("username", "", "kullanıcı adı")
	envPass := fs.String("env-password", "", "parolanın okunacağı ortam değişkeni adı")
	role := fs.String("role", "", "verilecek rol (boş bırakılırsa rol atanmaz)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*email) == "" || strings.TrimSpace(*username) == "" {
		return fmt.Errorf("--email ve --username zorunlu")
	}
	if strings.TrimSpace(*envPass) == "" {
		return fmt.Errorf("--env-password zorunlu (parola ortam değişkeninden okunur)")
	}
	pass := os.Getenv(*envPass)
	if pass == "" {
		return fmt.Errorf("%s ortam değişkeni boş", *envPass)
	}
	// Parola politikası kayıt akışıyla AYNI kapıdan geçer: kurulumdan gelen
	// hesap, arayüzden açılamayacak kadar zayıf bir parola taşımamalı.
	if p := domauth.CheckPasswordStrength(pass, *email, *username); p != domauth.PasswordOK {
		return fmt.Errorf("parola kabul edilmedi: %s", p.Message())
	}

	hash, err := domauth.HashPassword(pass)
	if err != nil {
		return err
	}
	u, err := a.q.CreateUser(a.ctx, db.CreateUserParams{
		Email: strings.ToLower(strings.TrimSpace(*email)), Username: strings.TrimSpace(*username),
		PasswordHash: hash, Status: db.UserStatusACTIVE,
	})
	if err != nil {
		return fmt.Errorf("kullanıcı oluşturulamadı (e-posta ya da kullanıcı adı zaten var olabilir): %w", err)
	}
	if err := a.q.MarkEmailVerified(a.ctx, u.ID); err != nil {
		return fmt.Errorf("e-posta doğrulanmış işaretlenemedi: %w", err)
	}

	if r := strings.TrimSpace(*role); r != "" {
		rol, err := a.q.GetRoleByName(a.ctx, r)
		if err != nil {
			return fmt.Errorf("rol bulunamadı: %s", r)
		}
		if err := a.q.AssignRole(a.ctx, db.AssignRoleParams{UserID: u.ID, RoleID: rol.ID}); err != nil {
			return err
		}
		fmt.Printf("✓ %s oluşturuldu ve %q rolü verildi\n", u.Email, r)
		return nil
	}
	fmt.Printf("✓ %s oluşturuldu\n", u.Email)
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

// catalogRentals kiralık fiyat ve stoklarını senkronlar.
//
// AKTİVASYON SENKRONUNDAN AYRI: sağlayıcı toplu kiralık katalog sunmuyor,
// servis başına bir istek gerekiyor. Her aktivasyon senkronuna eklemek
// yüzlerce ek istek demekti.
func (a *appCtx) catalogRentals(args []string) error {
	fs := flag.NewFlagSet("catalog:rentals", flag.ExitOnError)
	name := fs.String("provider", "", "sağlayıcı adı")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *name == "" {
		return fmt.Errorf("--provider zorunlu")
	}
	prov, err := a.q.GetProviderByName(a.ctx, *name)
	if err != nil {
		return fmt.Errorf("sağlayıcı bulunamadı: %s", *name)
	}

	reg := provider.NewRegistry()
	reg.Register(fake.New(port.RealClock{}))
	reg.Register(herosms.New())
	svc := catalog.New(catalog.Deps{
		TxRunner: a.tx, Registry: reg, Secrets: a.box, Clock: port.RealClock{},
	})

	rep, err := svc.SyncRentals(a.ctx, prov.ID)
	if err != nil {
		return err
	}
	fmt.Printf("✓ kiralık: %d ürün, %d teklif (%.1fs)\n",
		rep.Products, rep.Offers, rep.Duration.Seconds())
	for _, e := range rep.Errors {
		fmt.Printf("  ! %s\n", e)
	}
	return nil
}

// fxSync döviz kurunu sağlayıcıdan çeker ve kaydeder.
//
// NEDEN CLI'DA VAR: kuru periyodik tazeleyen işçi henüz yazılmadı (M5).
// test: quote_integration_test.go#TestKK302_StaleFXStopsSelling
// O zamana kadar kur bayatladığında hizmet DURUR — bu doğru davranıştır
// (KK-302: bayat kurla satış yapılmaz) ama elle tazeleyecek bir yol olmadan
// geliştirme ortamı birkaç saatte kullanılamaz hâle geliyordu.
func (a *appCtx) fxSync() error {
	svc := pricingsvc.NewFXService(a.tx, a.fx, port.RealClock{}, a.cfg.FXMaxAge)
	if err := svc.Refresh(a.ctx); err != nil {
		return err
	}
	q, err := svc.Current(a.ctx)
	if err != nil {
		return err
	}
	fmt.Printf("✓ kur güncellendi: 1 USD = %s TRY  (kaynak: %s, %s)\n",
		q.Rate.String(), q.Source, q.FetchedAt.Format("2006-01-02 15:04:05 MST"))
	return nil
}

// catalogIcon bir servisin logosunu ayarlar.
//
// Logo dosyası web/public/servis-logolari/ altında durur; burada yalnız ona
// işaret eden yol saklanır. Dosyanın varlığı BURADA DOĞRULANMAZ: CLI arka uçta,
// dosya ön yüzde durur ve ikisi ayrı makinelerde çalışabilir. Dosya eksikse
// arayüz kırık ikon değil, harf rozeti gösterir (components/service-icon.tsx).
func (a *appCtx) catalogIcon(args []string) error {
	fs := flag.NewFlagSet("catalog:icon", flag.ExitOnError)
	service := fs.String("service", "", "servis kodu (örn. wa)")
	url := fs.String("url", "", "logo yolu (örn. /servis-logolari/wa.svg)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *service == "" || *url == "" {
		return fmt.Errorf("--service ve --url zorunlu")
	}
	row, err := a.q.SetServiceIcon(a.ctx, db.SetServiceIconParams{
		Code: *service, IconUrl: *url,
	})
	if err != nil {
		return fmt.Errorf("servis bulunamadı veya güncellenemedi: %s", *service)
	}
	fmt.Printf("✓ %s (%s) logosu: %s\n", row.Code, row.Name, row.IconUrl)
	return nil
}

// walletCredit bir kullanıcının bakiyesine elle giriş yapar.
//
// ══════════════════════════════════════════════════════════════════════════
// 🔴 DOĞRUDAN `UPDATE users SET balance_minor` YAPMAZ (değişmez #2)
// ══════════════════════════════════════════════════════════════════════════
// Bakiye yalnız defter üzerinden değişir. Bu komut `wallet.Adjust`i çağırır,
// yani yönetim panelindeki "Bakiye düzelt" ekranıyla AYNI yoldan gider:
// `ledger_entries` satırı, idempotency anahtarı ve denetim kaydı üretilir.
// Elle bir SQL güncellemesi bunların üçünü de atlar ve mutabakatı (Σ defter ==
// bakiye) sessizce bozar.
//
// ANAHTAR ZORUNLUDUR ve deterministiktir: aynı `--key` ile ikinci çağrı YENİ
// kayıt üretmez, öncekinin sonucunu döner. Test sırasında komutu iki kez
// çalıştırmak bakiyeyi iki kez artırmaz — ikinci artışı gerçekten istiyorsanız
// farklı bir anahtar verirsiniz. Şema panel ile aynı: `manual:<public_id>:<key>`.
func (a *appCtx) walletCredit(args []string) error {
	fs := flag.NewFlagSet("wallet:credit", flag.ExitOnError)
	email := fs.String("email", "", "bakiyesi değişecek kullanıcının e-postası")
	adminEmail := fs.String("by", "", "işlemi yapan yönetici e-postası (denetim kaydına yazılır)")
	amount := fs.String("amount", "", "TL cinsinden tutar; eksi değer düşer (örn. 1000 ya da -250.50)")
	note := fs.String("note", "", "açıklama — ZORUNLU, sebepsiz bakiye değişikliği denetlenemez")
	key := fs.String("key", "", "idempotency anahtarı — aynı anahtar tek kez uygulanır")
	if err := fs.Parse(args); err != nil {
		return err
	}
	for ad, v := range map[string]string{
		"--email": *email, "--by": *adminEmail, "--amount": *amount,
		"--note": *note, "--key": *key,
	} {
		if strings.TrimSpace(v) == "" {
			return fmt.Errorf("%s zorunlu", ad)
		}
	}

	minor, err := tutarKurusa(*amount)
	if err != nil {
		return err
	}

	target, err := a.q.GetUserByEmail(a.ctx, *email)
	if err != nil {
		return fmt.Errorf("kullanıcı bulunamadı: %s", *email)
	}
	admin, err := a.q.GetUserByEmail(a.ctx, *adminEmail)
	if err != nil {
		return fmt.Errorf("yönetici bulunamadı: %s", *adminEmail)
	}

	res, err := wallet.New(a.tx).Adjust(a.ctx, wallet.AdjustInput{
		AdminID:      admin.ID,
		UserID:       target.ID,
		UserPublicID: target.PublicID.String(),
		Amount:       minor,
		Note:         *note,
		IdemKey:      fmt.Sprintf("manual:%s:%s", target.PublicID, strings.TrimSpace(*key)),
		// Denetim üstverisi: CLI'da istek yok, IP ve tarayıcı da yok.
		// `RequestID` komutun kendisini işaretler ki denetim kaydına bakan kişi
		// bunun panelden değil komut satırından geldiğini görsün.
		Audit: auditsvc.Meta{UserAgent: "cli", RequestID: "cli:wallet:credit"},
	})
	if err != nil {
		return err
	}
	if res.AlreadyApplied {
		fmt.Printf("• %s: bu anahtar zaten uygulanmış, yeni kayıt YAZILMADI. Bakiye: %s\n",
			*email, res.NewBalance.String())
		return nil
	}
	fmt.Printf("✓ %s bakiyesi güncellendi. Yeni bakiye: %s\n", *email, res.NewBalance.String())
	return nil
}

// tutarKurusa TL metnini kuruşa çevirir.
//
// 🔴 TAM KESİRLE ÇEVİRİR, `float64` KULLANMAZ (değişmez #1).
//
// Ölçüldü: `strconv.ParseFloat("19.99", 64)` = 19.98999999999999843681 ve
// `int64(f*100)` = 1998 — BİR KURUŞ buharlaşır. (Her değerde olmaz: "1000.10"
// float yolunda da doğru çıkar; hatanın sinsiliği tam olarak budur, göz
// kararıyla yakalanmaz.) `big.Rat` tam sayı oranı tutar ve kesir kuruşa tam
// oturmuyorsa HATA verir, sessizce yuvarlamaz.
//
// Türkçe virgül kabul edilir ("1000,50") ama NOKTA İLE BİRLİKTE kullanılamaz:
// "1.000,50" binlik ayırıcı mı ondalık mı belirsizdir ve bir para komutunda
// belirsizliği tahminle kapatmak yanlış tutar yazmaktır.
// test: main_test.go#TestTutarKurusa
func tutarKurusa(ham string) (money.Money, error) {
	s := strings.TrimSpace(ham)
	if strings.Contains(s, ",") {
		if strings.Contains(s, ".") {
			return money.Zero(money.TRY), fmt.Errorf(
				"tutarda hem nokta hem virgül var, hangisi ondalık belirsiz: %q", ham)
		}
		s = strings.Replace(s, ",", ".", 1)
	}
	oran, ok := new(big.Rat).SetString(s)
	if !ok {
		return money.Zero(money.TRY), fmt.Errorf("geçersiz tutar: %q", ham)
	}
	kurus := new(big.Rat).Mul(oran, big.NewRat(100, 1))
	if !kurus.IsInt() {
		return money.Zero(money.TRY), fmt.Errorf(
			"tutar kuruştan küçük bir kesir içeriyor: %q", ham)
	}
	if !kurus.Num().IsInt64() {
		return money.Zero(money.TRY), fmt.Errorf("tutar çok büyük: %q", ham)
	}
	m := money.New(kurus.Num().Int64(), money.TRY)
	if m.IsZero() {
		return money.Zero(money.TRY), fmt.Errorf("tutar sıfır olamaz")
	}
	return m, nil
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
  fx:sync            Döviz kurunu tazele (işçi yazılana kadar elle)
  catalog:rentals    Kiralık katalog senkronu  --provider
  catalog:icon       Servis logosu ayarla  --service --url
  admin:grant        Rol ata            --email --role
  seed               Kur + katalog senkronu ve hazırlık raporu
  user:create        Kullanıcı oluştur (kurulumun ilk hesabı)
  wallet:credit      Bakiyeye elle giriş (defter üzerinden, denetim kaydıyla)
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

// saglayiciHazirMi, sağlayıcı bağlantısının SATIŞ YAPILABİLİR olup olmadığını
// söyler.
//
// İki koşul da gereklidir: anahtar kayıtlı OLMALI ve bu çalıştırmadaki katalog
// senkronu DÜŞMEMİŞ olmalı. Yalnız varlığa bakmak, sağlayıcı anahtarı
// reddetmişken raporun ✓ basmasına yol açıyordu.
//
// test: seed_hazirlik_test.go#TestSaglayiciSatiriSenkronDustugundeKirmiziOlur
func saglayiciHazirMi(anahtarVar bool, senkronDusen int) bool {
	return anahtarVar && senkronDusen == 0
}
