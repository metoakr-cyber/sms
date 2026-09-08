# Üretim dağıtımı

Tek VPS · Docker Compose · Caddy (otomatik TLS). Bu dizindeki dosyalar üretim
yığınının tamamıdır.

| Dosya | Ne |
|---|---|
| `Dockerfile.api` | Go imajı: `server` + `worker` + `cli` + `goose` + migration'lar. Açılışta `goose up`. |
| `Dockerfile.web` | Next.js standalone imajı. `NEXT_PUBLIC_*` **derleme anında** gömülür. |
| `docker-compose.prod.yml` | caddy · api · worker · web · postgres · redis |
| `Caddyfile` | TLS, HTTP/2+3, `/api/*` → api, gerisi → web, SSE için ayrı blok |
| `.env.prod.example` | Uygulamanın okuduğu her değişken, açıklamalı |
| `scripts/yedekle.sh` | Şifreli günlük yedek + doğrulama + saklama |
| `scripts/geri-yukle.sh` | Onay isteyen geri yükleme / tatbikat |

Tüm komutlar **depo kökünden** çalıştırılır.

---

## 0. Bilinen engeller — kurulumdan ÖNCE okuyun

Bunlar bu paketin dışındaki kod eksikleridir. Bilmeden kurulum yapmak, yarım
saat sonra anlaşılmaz bir hataya çarpmak demektir.

### ✅ E2 — ÇÖZÜLDÜ: webhook ters vekil arkasında gerçek IP'yi görüyor

*(Bu madde bir engeldi; 2026-09-08'de kapatıldı ve burada kayıt olarak kalıyor.)*

**Sorun neydi:** api yalnız `127.0.0.1` ve `::1` vekiline güveniyordu. Caddy
ayrı bir konteyner olduğu için Go tarafında görünen istemci IP'si her zaman
Caddy'nin adresiydi; HeroSMS bildirimleri "izin listesi dışı" sayılıp
**200 dönerek sessizce** atılıyordu. Sistem çalışıyor görünür, gelir sıfır olurdu.

**Çözüm:** `TRUSTED_PROXIES` yapılandırması eklendi (aşağıda). Webhook el
tutucusu artık şu kuralı uyguluyor:

> Vekil başlıklarına **yalnız** bağlantı güvenilen bir vekilden geliyorsa
> bakılır. Zincirde **sağdan sola** yürünür ve güvenilmeyen **ilk** adres
> alınır — saldırganın gövdeye eklediği sahte adresler onun solunda kalır.

İki uç hata da testle kapalı:
`webhook_test.go#TestForwardedForHeaderIsIgnored` (uydurma başlık geçmez) ve
`webhook_test.go#TestTrustedProxyHeaderIsHonored` (vekil arkasında çalışır).
`0.0.0.0/0` yapılandırması açılışta **reddedilir**
(`config_test.go#TestTrustedProxiesRejectsOpenRange`).

**Sizin yapmanız gereken:** `.env` içinde `TRUSTED_PROXIES` değerini Docker
ağınızın CIDR'i olarak verin (varsayılan compose ağı için `172.16.0.0/12`).
Üretimde bu değişken **boş bırakılamaz** — süreç başlamaz.

### 🟡 E3 — `api` **tek örnek** çalışır, ölçeklenemez

Arka plan işlerinin çift koşmasını `WORKERS_IN_PROCESS` engelliyor, ama o
anahtar **süreç türü** ayrımı yapar, **örnek sayısı** değil: aynı imajdan iki
`api` (veya iki `worker`) kopyası aynı ayarı okur ve ikisi de aynı siparişi
yoklar. Bakiye bozulmaz (iade `order:{uuid}:refund` anahtarıyla idempotent),
ama sağlayıcıda çift `Cancel`/`Finish` çağrısı ve iki kat dış istek olur.

**Kural:** `--scale api=2` ve `--scale worker=2` yasaktır. Yatay ölçekleme
öncesi Redis kilidi gerekir.

### ℹ️ Çözülmüş engeller

Bu paket yazılırken iki engel paralel çalışmayla kapandı; not olarak duruyor:

- **E1 (mailer):** `resend` ve `smtp` adaptörleri eklendi
  (`api/internal/adapter/mailer/`), `APP_ENV=production` artık açılabiliyor.
  `MAIL_PROVIDER=smtp` seçilirse `SMTP_HOST` · `SMTP_USERNAME` ·
  `SMTP_PASSWORD` **ortamdan bağımsız olarak** zorunludur.
- **E4 (izleme):** Sentry ve Prometheus kablolandı
  (`api/internal/adapter/obs/`). `/metrics` ucu **uygulamanın önünde**, aynı
  `api:8080` portunda duruyor — bu yüzden Caddy `/metrics*` yolunu 404'lüyor
  ve Prometheus'a **konteyner ağı içinden** erişilir.

---

## 1. VPS hazırlığı

Ubuntu 24.04 LTS, en az 2 vCPU / 4 GB RAM / 40 GB disk.

```bash
# Docker + compose eklentisi
curl -fsSL https://get.docker.com | sh

# Güvenlik duvarı: yalnız SSH ve web
ufw default deny incoming && ufw default allow outgoing
ufw allow 22/tcp && ufw allow 80/tcp && ufw allow 443/tcp
ufw allow 443/udp          # HTTP/3 (QUIC)
ufw --force enable

# Otomatik güvenlik güncellemeleri
apt-get install -y unattended-upgrades && dpkg-reconfigure -f noninteractive unattended-upgrades

# Takas alanı (4 GB) — derleme sırasında OOM'u önler
fallocate -l 4G /swapfile && chmod 600 /swapfile && mkswap /swapfile && swapon /swapfile
echo '/swapfile none swap sw 0 0' >> /etc/fstab
```

**Doğrula:** `docker compose version` sürüm basmalı, `ufw status` yalnız dört
kuralı göstermeli.

---

## 2. DNS

| Kayıt | Ad | Değer |
|---|---|---|
| A | `onay360.com` | VPS IPv4 |
| A | `www.onay360.com` | VPS IPv4 |
| AAAA | (varsa) | VPS IPv6 |

**Doğrula:** `dig +short onay360.com` VPS IP'sini dönmeli. Caddy sertifikayı
DNS yayılmadan **alamaz**; ilk `up` öncesi bunu bekleyin.

---

## 3. Depo ve sırlar

```bash
git clone <depo-url> /opt/onay360 && cd /opt/onay360
cp deploy/.env.prod.example deploy/.env
chmod 600 deploy/.env
```

Sırları üretin ve `deploy/.env` içine **elle** yazın:

```bash
openssl rand -base64 32     # SESSION_SECRET
openssl rand -base64 32     # ENCRYPTION_KEY
openssl rand -hex 24        # POSTGRES_PASSWORD  (yalnız harf/rakam — URL'e gömülüyor)
openssl rand -hex 24        # REDIS_PASSWORD
openssl rand -hex 16        # WEBHOOK_HEROSMS_SECRET  (yol segmenti, anahtar değil)
```

🔴 **Bu adım TEK SEFERLİKTİR.** `SESSION_SECRET` veya `ENCRYPTION_KEY`'i ikinci
kez üretmek: tüm oturumlar düşer ve veritabanındaki şifreli sağlayıcı API
anahtarları **bir daha çözülemez**. Rotasyon için §9'a bakın.

🔴 **Satır sonuna yorum yazmayın.** Compose'un `.env` okuyucusu, değeri boş olan
bir satırdaki `# ...` metnini yorum değil **değer** sayar:
`SESSION_SECRET=   # doldurulacak` satırı uygulamaya gerçekten
`"# doldurulacak"` değerini verir ve hata "32 bayt değil" olur.

`deploy/.env` dosyasının depoya girmediğinden emin olun:

```bash
grep -q '^/deploy/.env$' .gitignore || echo '/deploy/.env' >> .gitignore
git check-ignore -v deploy/.env      # bir satır basmalı
```

**Doğrula:** boş kalan zorunlu alan olmamalı —

```bash
grep -nE '^(DOMAIN|ACME_EMAIL|PUBLIC_BASE_URL|POSTGRES_PASSWORD|REDIS_PASSWORD|SESSION_SECRET|ENCRYPTION_KEY|RECAPTCHA_SITE_KEY|RECAPTCHA_SECRET_KEY|NEXT_PUBLIC_RECAPTCHA_SITE_KEY|NEXT_PUBLIC_SITE_URL|MAIL_FROM|WEBHOOK_HEROSMS_SECRET|BACKUP_REMOTE)=$' deploy/.env
# hiçbir satır basmamalı
```

---

## 4. İlk açılış

```bash
docker compose --env-file deploy/.env -f deploy/docker-compose.prod.yml build
docker compose --env-file deploy/.env -f deploy/docker-compose.prod.yml up -d
```

Migration'lar `api` konteyneri açılırken `goose up` ile koşar; **başarısız
olursa konteyner ayağa kalkmaz** (yarı göçmüş şemayla çalışan bir para
sistemi, çalışmayan bir sistemden tehlikelidir).

**Doğrula:**

```bash
docker compose -f deploy/docker-compose.prod.yml ps          # 6 servis (worker healthcheck'siz)
docker compose -f deploy/docker-compose.prod.yml logs api | grep 'migration tamam'
curl -s https://onay360.com/readyz                            # {"status":"ready", ...}
curl -sI --http2 https://onay360.com/ | head -1               # HTTP/2 200
curl -sI https://onay360.com/ | grep -i strict-transport      # HSTS var
curl -so /dev/null -w '%{http_code}\n' https://onay360.com/metrics   # 404
```

Postgres/Redis'in host'a port açmadığını da doğrulayın:

```bash
docker compose -f deploy/docker-compose.prod.yml ps --format '{{.Service}} {{.Ports}}'
# yalnız caddy satırında 0.0.0.0:80 / :443 görünmeli
```

---

## 5. İlk tohum (seed)

Yönetim komutları imajın içindeki `cli` ikilisiyle çalışır.

```bash
D="docker compose --env-file deploy/.env -f deploy/docker-compose.prod.yml"

# 1) Sağlayıcı kaydı — API anahtarı ORTAMDAN okunur, komut satırından değil
#    (argümanlar `ps` çıktısına ve kabuk geçmişine düşer).
$D run --rm --entrypoint /app/cli api \
  provider:add --name=herosms --protocol=HEROSMS_V1 --base-url=https://hero-sms.com \
  --env-key=HEROSMS_API_KEY

# Kayıt oluştu: artık HEROSMS_API_KEY satırını deploy/.env içinde BOŞALTIN.
# Anahtar bundan sonra veritabanında AES-GCM şifreli durur.

# 2) Döviz kuru
$D run --rm --entrypoint /app/cli api fx:sync

# 3) Katalog
$D run --rm --entrypoint /app/cli api catalog:sync --provider=herosms

# 4) İlk yönetici (önce arayüzden kayıt olun, sonra rol verin)
$D run --rm --entrypoint /app/cli api admin:grant --email=siz@ornek.com --role=admin

# 5) Doğrulama
$D run --rm --entrypoint /app/cli api provider:list
$D run --rm --entrypoint /app/cli api wallet:reconcile     # "sapma yok" basmalı
```

---

## 6. 🔴 Elle panel adımı: webhook URL'i

Otomatikleştirilemez. HeroSMS panelinde webhook URL'i şudur:

```
https://onay360.com/api/v1/webhooks/herosms/<WEBHOOK_HEROSMS_SECRET>
```

Bilinmesi gerekenler:

- HeroSMS'te bunun **API ucu yoktur** — panelden elle girilir.
- En fazla **3 HTTPS slotu** vardır ve slotlar **hesap genelindedir**.
  Staging ile üretim aynı hesabı paylaşırsa her ortam diğerinin olaylarını da
  alır; tanımadığı `activationId` sessizce yok sayılmalıdır.
- Webhook gövdesinde **imza yoktur**; tek savunma IP izin listesidir — ve o
  liste bugün ters vekil arkasında çalışmıyor (§0/E2).

---

## 7. Yedekleme

```bash
apt-get install -y age rclone                # veya gnupg

# Şifreleme anahtarı (ÖZEL anahtar sunucuda TUTULMAZ)
age-keygen -o onay360-yedek.key
grep 'public key' onay360-yedek.key          # deploy/.env → BACKUP_AGE_RECIPIENT

# 🔴 onay360-yedek.key dosyasını sunucudan ALIN. Çevrimdışı, iki ayrı
#    fiziksel kopya. Bu anahtar kaybolursa TÜM yedekler geri getirilemez.

# Ayrı konum (NFR-808) — rclone hedefi
rclone config                                # deploy/.env → BACKUP_REMOTE

mkdir -p /var/backups/onay360 && chmod 700 /var/backups/onay360
```

İlk yedeği elle alın ve çıktıyı okuyun:

```bash
./deploy/scripts/yedekle.sh
```

Betik "yedek tamam" demeden önce üç kapıdan geçer: `pg_dump`'ın **kendi**
çıkış kodu, asgari boyut, ve `pg_restore --list` ile arşivin gerçekten
okunabilir olması. Ardından şifreler, sha256 yazar, ayrı konuma kopyalar ve
yalnız süresi dolmuş dosyaları siler.

Cron:

```bash
(crontab -l 2>/dev/null; echo '17 3 * * * cd /opt/onay360 && ./deploy/scripts/yedekle.sh >> /var/log/onay360-yedek.log 2>&1') | crontab -
```

### Geri yükleme tatbikatı (yılda en az bir kez — NFR-808)

```bash
# Özel anahtarı GEÇİCİ olarak sunucuya getirin
export BACKUP_AGE_KEY_FILE=/root/onay360-yedek.key

# Hedef vermezseniz varsayılan smsplatform_tatbikat'tır: CANLI VERİYE DOKUNMAZ
./deploy/scripts/geri-yukle.sh /var/backups/onay360/onay360-<damga>.dump.age

# 🔴 Tatbikat burada bitmez. Yedeğin "açılıyor olması" yetmez:
D="docker compose --env-file deploy/.env -f deploy/docker-compose.prod.yml"
$D run --rm --entrypoint /app/cli \
  -e DATABASE_URL="postgres://$POSTGRES_USER:$POSTGRES_PASSWORD@postgres:5432/smsplatform_tatbikat?sslmode=disable" \
  api wallet:reconcile

# Sonucu kayda geçirin (tarih · dosya · ledger sayısı · mutabakat) ve
# özel anahtarı sunucudan SİLİN.
```

Canlı veritabanının üzerine yazmak iki şey birden ister: `--uretim-onayi`
bayrağı **ve** veritabanı adının harfi harfine yazılması. Yanlışlıkla
çalıştırılabilen bir komut, er ya da geç yanlışlıkla çalıştırılır.

---

## 8. Rutin güncelleme

```bash
cd /opt/onay360 && git pull

# İmajları git SHA ile etiketleyin — `latest` geri dönülecek nokta bırakmaz
export IMAGE_TAG=$(git rev-parse --short=8 HEAD)
sed -i "s/^IMAGE_TAG=.*/IMAGE_TAG=$IMAGE_TAG/" deploy/.env

docker compose --env-file deploy/.env -f deploy/docker-compose.prod.yml build
docker compose --env-file deploy/.env -f deploy/docker-compose.prod.yml up -d
curl -s https://onay360.com/readyz
```

**Alan adı veya reCAPTCHA site anahtarı değiştiyse:** `web` imajını yeniden
derlemek **zorunludur** (`NEXT_PUBLIC_*` derleme anında gömülür). Konteyneri
yeniden başlatmak hiçbir şey değiştirmez; reCAPTCHA sessizce çizilmez ve
robots.txt localhost gösterir.

---

## 9. Geri alma (rollback)

```bash
sed -i "s/^IMAGE_TAG=.*/IMAGE_TAG=<önceki-sha>/" deploy/.env
docker compose --env-file deploy/.env -f deploy/docker-compose.prod.yml up -d
```

🔴 **Şema geri alınmaz.** Migration'lar açılışta ileri doğru koşar; eski imaja
dönmek şemayı geri **almaz** ve eski kod yeni şemayı görür. Para tablolarında
bu sessiz bozulmadır.

Kural: **şema değişikliği içeren bir sürüm geri alınmaz, ileriye doğru
düzeltilir.** Yeni bir migration yazın. `goose down` para tablolarında son
çaredir ve yalnız veri kaybı olmadığı ispatlandıktan sonra kullanılır.

### Sır rotasyonu

| Sır | Etkisi | Prosedür |
|---|---|---|
| `SESSION_SECRET` | Herkes çıkış yapar | Değiştir, `up -d`. Duyuru gerekir. |
| `WEBHOOK_HEROSMS_SECRET` | Webhook URL'i değişir | Önce `.env`, sonra **HeroSMS panelinde URL'i güncelle** (§6). |
| `POSTGRES_PASSWORD` | — | `ALTER ROLE ... PASSWORD`, sonra `.env`, sonra `up -d`. |
| `ENCRYPTION_KEY` | 🔴 **Sağlayıcı anahtarları çözülemez** | Rotasyondan **önce** tüm sağlayıcı API anahtarlarını yeniden gireceğinizi planlayın; sonra yönetim panelinden yeniden girin. |

---

## 10. Acil durum

```bash
D="docker compose --env-file deploy/.env -f deploy/docker-compose.prod.yml"

$D ps                       # ne ayakta
$D logs -f --tail=200 api   # api günlüğü (JSON, request_id ile)
$D restart api              # tek servis
$D stop                     # her şeyi durdur (veri kalır)
$D up -d                    # geri getir
```

🔴 **`docker compose down -v` ASLA çalıştırılmaz.** `-v` adlandırılmış
hacimleri siler: veritabanı (`pgdata`), Redis AOF (`redisdata`) **ve Caddy'nin
sertifikaları** (`caddy_data`). Sertifikaların kaybı Let's Encrypt oran
sınırına (alan adı başına haftada 5) takılmak demektir — site sertifikasız
kalır.

Disk dolduğunda sırayla: `docker image prune -a`, `docker builder prune`,
eski yedekler (uzak kopya varsa). **Postgres hacmine elle dokunulmaz.**

---

## 11. Bu paketin dışında kalanlar

| Eksik | Neden burada değil |
|---|---|
| `TRUSTED_PROXIES` + webhook `X-Real-IP` | Uygulama kodu — §0/E2'nin çözümü |
| Prometheus/Alertmanager yığını ve alarm kuralları | `/metrics` hazır; toplayıcı ayrı bir iş kalemi |
| GitHub Actions → registry → VPS boru hattı | Ayrı iş kalemi; compose hem `build:` hem `image:` taşıyor, boru hattı sonradan yalnız `IMAGE_TAG` vererek devreye girer |
| Yedekleme betiklerinin kabuk testleri | Kapılar elle sabote edilerek doğrulandı; kalıcı test dosyası (`deploy/scripts/yedekle_test.sh`) ayrıca eklenmelidir |
| KVKK metinleri, Lighthouse bütçesi, yük testi | M7/M8'in diğer kalemleri |
