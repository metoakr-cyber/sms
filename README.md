# SMS Platform

Sanal numara (SMS doğrulama) satış platformu. Kullanıcı ön ödemeli **TL** bakiyesiyle servis
(WhatsApp, Telegram…) ve ülke seçer, numara alır, gelen SMS kodunu görür. Kod gelmezse
ücret otomatik iade edilir.

> **Bu bir para sistemidir.** Katkı vermeden önce [CLAUDE.md](CLAUDE.md) içindeki
> **Değişmezler** bölümünü okuyun.

## Yığın

| Katman | Teknoloji |
|---|---|
| Backend | Go 1.26 · Gin · **sqlc + pgx** · goose |
| Veritabanı | PostgreSQL 16 |
| Önbellek / kuyruk | Redis 7 · asynq |
| Frontend | Next.js 15 · React · TypeScript · Tailwind CSS 4 |
| Dağıtım | Docker Compose · Caddy (tek alan adı, ters vekil) |

## 15 dakikada ayağa kaldırma

```bash
# 1. Araçlar (bir kez)
make tools

# 2. Ortam değişkenleri
cp .env.example .env
# SESSION_SECRET ve ENCRYPTION_KEY üret:
echo "SESSION_SECRET=$(openssl rand -base64 32)" >> .env
echo "ENCRYPTION_KEY=$(openssl rand -base64 32)" >> .env
echo "WEBHOOK_HEROSMS_SECRET=$(openssl rand -hex 16)" >> .env

# 3. Altyapı
make up

# 4. Şema
set -a && source .env && set +a
make migrate-up

# 5. Çalıştır — ÜÇ AYRI TERMİNAL
make dev            # API   → http://localhost:8091
make web            # Web   → http://localhost:3000   (asıl arayüz burası)
make worker         # işçiler

# NOT: `make dev` yalnız API'yi başlatır. Arayüzü görmek için `make web` de
# gerekir; Next.js /api isteklerini API'ye kendisi taşır (tek alan adı
# topolojisi üretimle aynı, bkz. web/next.config.ts).
#
# WORKERS_IN_PROCESS=true (varsayılan) iken işler API sürecinde koşar ve
# `make worker` hata verip çıkar — bu kasıtlıdır, aynı iş iki süreçte
# koşmasın diye. Ayrı işçi süreci istiyorsanız .env'de false yapın.
```

## Sık kullanılan komutlar

```bash
make help           # tüm komutlar
make test           # testler (-race)
make test-cover     # kapsam raporu
make check          # birleştirmeden ÖNCE: sqlc diff + lint + test
make gen            # sqlc kodunu üret (queries/*.sql değiştiyse)
make migrate-new name=add_x
```

## Dokümantasyon

| Dosya | İçerik |
|---|---|
| [docs/intent.md](docs/intent.md) | Neden var, kapsam, "bitti" tanımı |
| [docs/design.md](docs/design.md) | Mimari, para modeli, akışlar, 29 ADR |
| [docs/trd.md](docs/trd.md) | Numaralı gereksinimler + kabul kriterleri |
| [docs/provider-herosms.md](docs/provider-herosms.md) | Sağlayıcı API referansı (doğrulanmış) |
| [docs/frontend-contract.md](docs/frontend-contract.md) | Responsive, tarayıcı uyumluluğu, SSE |
| [docs/roadmap.md](docs/roadmap.md) | Yapım sırası ve çıkış kriterleri |
| [docs/memory.md](docs/memory.md) | Karar günlüğü, tuzaklar, açık sorular |
| [docs/TESLIM.md](docs/TESLIM.md) | **Devralan kişi için**: ne hazır, ne değil, ilk gün |
| [docs/runbook.md](docs/runbook.md) | **Arıza anında**: sağlayıcı çöktü, mutabakat sapması, disk doldu |
| [deploy/README.md](deploy/README.md) | Üretime alma: sıfırdan kurulum, yedek, geri alma |
| [CLAUDE.md](CLAUDE.md) | Çalışma kuralları ve değişmezler |

## Durum

Kilometre taşları için [docs/roadmap.md](docs/roadmap.md).

| | Durum |
|---|---|
| **M0** Temel · monorepo, Compose, sqlc + goose, yapılandırma doğrulaması | ✅ |
| **M1** Kimlik · kayıt, e-posta doğrulama, oturum (Redis), şifre sıfırlama, RBAC, hız limiti | ✅ |
| **M2** Defter · değişmez kayıt, `FOR UPDATE`, idempotency, mutabakat, admin düzeltme | ✅ |
| **M3** Sağlayıcı · `ProviderPort`, HeroSMS adaptörü, AES-GCM anahtar, katalog senkronu | ✅ |
| **M4** Fiyatlandırma · TCMB kuru, kapsam öncelikli marj, tek kullanımlık teklif | ✅ |
| **M5** Sipariş · satın alma, webhook, SSE, iptal/iade, yoklama, iade mutabakatı | ✅ |
| **M6** Panel · bakiye yükleme, yönetim ekranları, denetim kaydı | ✅ |
| **M7** Sertleştirme · ikinci sağlayıcı, yük testi, gerçek cihaz turu | ⬜ |
| **M8** Üretim · dağıtım paketi ✅ · izleme ✅ · yedek ✅ · **geri yükleme tatbikatı yapılmadı** | 🟨 |
| **M9** Lansman kapısı · ticari/yasal karar | ⬜ |

**Uçtan uca doğrulanmış akışlar** — gerçek sunucuya karşı, test değil:

- Numara alma → SSE ile kod → iptal → tam iade → sağlayıcıda kapatma
- 20 eşzamanlı bakiye onayı → **tek** defter kaydı (KK-502)
- `image/jpeg` başlıklı PHP dosyası dekont olarak reddedildi (KK-500)
- Mutabakat: `Σ defter == bakiye`, sapma sıfır (KK-200)
- 115 sayfa×genişlik responsive denetimi temiz (23 sayfa × 320…1440 px)

**Teslim öncesi bilinmesi gerekenler:** [docs/TESLIM.md](docs/TESLIM.md) —
neyin hazır olduğu, neyin olmadığı ve ilk gün yapılacaklar.

### Duman testi

```bash
make up && make smoke              # uçtan uca: kimlik + cüzdan (26 senaryo)
make test-integration              # eşzamanlılık: KK-200..KK-203
```

### Sağlayıcı kurulumu

```bash
go run ./cmd/cli provider:add --name=fake --protocol=FAKE
go run ./cmd/cli catalog:sync --provider=fake
go run ./cmd/cli provider:list
```
Gerçek sağlayıcı için API anahtarı **ortam değişkeninden** okunur:
`--env-key=HEROSMS_API_KEY` — komut satırından değil, çünkü argümanlar kabuk
geçmişine ve `ps` çıktısına düşer.

### Yerel portlar

8080 ve 5432/6379 makinede doluydu; çakışmayı önlemek için:
postgres **55432**, redis **56379**, API **8091**. `.env` ile değiştirilebilir.
