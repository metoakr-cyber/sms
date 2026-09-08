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

# 5. Çalıştır
make dev            # API      → http://localhost:8080
make worker         # işçiler  (ayrı terminal)
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
| [CLAUDE.md](CLAUDE.md) | Çalışma kuralları ve değişmezler |

## Durum

Geliştirme aşamasında — `docs/roadmap.md`'ye bakın.

- [x] **M0** Temel: monorepo, Docker Compose, Makefile, sqlc + goose, yapılandırma doğrulaması
- [x] **M2 (kısmi)** `domain/money` — tam sayı para, kayıpsız oran aritmetiği.
      **%95,5 kapsam**, KK-304 altın testi geçiyor: `0,35 USD × 43,20 × 1,40 = 2117 kuruş`
- [x] **M1** Kimlik: kayıt · e-posta doğrulama · giriş · oturum (Redis) · şifre sıfırlama ·
      RBAC · hız limiti
- [x] **M2** Ledger: değişmez defter · `FOR UPDATE` kilidi · idempotency · mutabakat ·
      hareket dökümü · admin düzeltme.
      **KK-200…KK-203 gerçek PostgreSQL'e karşı ispatlandı** (`make test-integration`)
- [x] **M3 (sahte sağlayıcı ile)** Genişletilebilir katalog · `ProviderPort` ·
      `FakeProvider` + 13 maddelik sözleşme testi · AES-GCM anahtar şifreleme ·
      katalog senkronu · yönetim CLI'ı. **HeroSMS adaptörü sağlayıcı bakiyesi bekliyor**
- [x] **M4** Fiyatlandırma: kur (TCMB) · kapsam öncelikli marj kuralları ·
      tek kullanımlık fiyat teklifi. **KK-302/305/402 ispatlandı**
- [ ] M5 Sipariş+SSE · M6 Panel

**Uçtan uca 26/26 duman testi geçiyor** (`make smoke`)

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
