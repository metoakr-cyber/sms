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
- [x] **M1 (kısmi)** Kimlik şeması (8 tablo), hata taksonomisi, HTTP iskeleti,
      `/healthz` + `/readyz` çalışıyor. Doğrulanmış kısıtlar: KK-203 (negatif bakiye reddi),
      CITEXT benzersizlik, yumuşak silme sonrası e-posta yeniden kullanımı
- [ ] M1 kalan: kayıt/giriş/oturum/RBAC · M2 Ledger · M3 Sağlayıcı · M4 Fiyat · M5 Sipariş+SSE · M6 Panel

### Yerel portlar

8080 ve 5432/6379 makinede doluydu; çakışmayı önlemek için:
postgres **55432**, redis **56379**, API **8091**. `.env` ile değiştirilebilir.
