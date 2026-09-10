#!/usr/bin/env bash
#
# Onay360 — Ubuntu sunucuya DOCKER'SIZ tek seferlik kurulum.
#
# ══════════════════════════════════════════════════════════════════════════
# NE YAPAR
# ══════════════════════════════════════════════════════════════════════════
#   1. Soruları sorar (alan adı, portlar, parolalar) ve özeti ONAYLATIR
#   2. Bağımlılıkları kurar: PostgreSQL 16, Redis, Go, Node, Caddy
#   3. Veritabanını ve rolü açar
#   4. .env üretir — sırlar `openssl rand` ile, elle yazılmaz
#   5. API, worker, CLI ve ön yüzü derler
#   6. goose migration'larını uygular
#   7. TOHUMLAR: yönetici hesabı, sağlayıcı, kur, katalog
#   8. systemd birimlerini yazar, Caddy'yi yapılandırır, başlatır
#   9. Sağlık kontrolü yapar ve özet basar
#
# ══════════════════════════════════════════════════════════════════════════
# 🔴 TEKRAR ÇALIŞTIRILABİLİR
# ══════════════════════════════════════════════════════════════════════════
# Her adım "zaten varsa atla" mantığıyla yazıldı. Yarıda kalan bir kurulumu
# baştan çalıştırmak veriyi silmez: veritabanı varsa yeniden oluşturulmaz,
# .env varsa sırlar KORUNUR (yeniden üretmek tüm oturumları ve şifreli
# sağlayıcı anahtarlarını çöpe atardı).
#
# Kullanım:
#   sudo bash deploy/kurulum.sh
set -euo pipefail

# ─────────────────────────── Sabitler ───────────────────────────
readonly GO_SURUM="1.26.0"
readonly NODE_ANA_SURUM="22"
readonly GOOSE_SURUM="v3.28.0"
readonly PG_ANA_SURUM="16"

KOK="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly KOK

# ─────────────────────────── Çıktı ───────────────────────────
if [[ -t 1 ]]; then
  KIRMIZI=$'\e[31m'; YESIL=$'\e[32m'; SARI=$'\e[33m'; MAVI=$'\e[36m'; KALIN=$'\e[1m'; SIFIR=$'\e[0m'
else
  KIRMIZI=''; YESIL=''; SARI=''; MAVI=''; KALIN=''; SIFIR=''
fi

baslik() { printf '\n%s──── %s ────%s\n' "$KALIN$MAVI" "$1" "$SIFIR"; }
bilgi()  { printf '  %s\n' "$1"; }
tamam()  { printf '  %s✓%s %s\n' "$YESIL" "$SIFIR" "$1"; }
uyari()  { printf '  %s!%s %s\n' "$SARI" "$SIFIR" "$1"; }
hata()   { printf '\n%s✗ %s%s\n\n' "$KIRMIZI$KALIN" "$1" "$SIFIR" >&2; exit 1; }

# Beklenmedik bir hatada NEREDE durduğunu söyle: "kurulum yarıda kaldı" mesajı
# tek başına hangi adımın sorumlu olduğunu göstermez.
ADIM="başlangıç"
trap 'hata "kurulum \"$ADIM\" adımında durdu (satır $LINENO). Sorunu giderip betiği TEKRAR çalıştırabilirsiniz."' ERR

# ─────────────────────────── Ön kontroller ───────────────────────────
ADIM="ön kontroller"
[[ $EUID -eq 0 ]] || hata "root gerekli:  sudo bash deploy/kurulum.sh"
[[ -f /etc/os-release ]] || hata "Ubuntu/Debian bekleniyor (/etc/os-release yok)"
. /etc/os-release
[[ "${ID:-}" == "ubuntu" || "${ID_LIKE:-}" == *debian* ]] || hata "yalnız Ubuntu/Debian destekleniyor (bulunan: ${ID:-bilinmiyor})"
[[ -d "$KOK/api" && -d "$KOK/web" ]] || hata "depo kökü bulunamadı — betiği depo içinden çalıştırın"
# /root ve /home altı ÇALIŞIR (ProtectHome=read-only) ama /opt daha uygundur:
# servis hesabının ev dizini karışmaz ve yedekleme yolları tahmin edilebilir olur.
case "$KOK" in
  /home/*|/root/*) KONUM_UYARISI="depo $KOK altında — çalışır, ama /opt/onay360 önerilir" ;;
  *) KONUM_UYARISI="" ;;
esac

# ─────────────────────────── Sorular ───────────────────────────
sor() { # sor <değişken> <soru> [varsayılan]
  local ad="$1" soru="$2" varsayilan="${3:-}" cevap
  if [[ -n "$varsayilan" ]]; then
    read -rp "  ${soru} [${varsayilan}]: " cevap || true
    cevap="${cevap:-$varsayilan}"
  else
    read -rp "  ${soru}: " cevap || true
  fi
  printf -v "$ad" '%s' "$cevap"
}

sor_gizli() { # sor_gizli <değişken> <soru> — iki kez sorar, eşleşmesini bekler
  local ad="$1" soru="$2" bir iki
  while true; do
    read -rsp "  ${soru}: " bir; echo
    [[ -z "$bir" ]] && { uyari "boş olamaz"; continue; }
    read -rsp "  ${soru} (tekrar): " iki; echo
    [[ "$bir" == "$iki" ]] && break
    uyari "parolalar eşleşmedi, tekrar deneyin"
  done
  printf -v "$ad" '%s' "$bir"
}

evet_mi() { [[ "${1,,}" =~ ^(e|evet|y|yes)$ ]]; }

printf '%s\n' "$KALIN"
cat <<'BANNER'
  ╭──────────────────────────────────────────────╮
  │   Onay360 — Ubuntu kurulumu (Docker'sız)     │
  ╰──────────────────────────────────────────────╯
BANNER
printf '%s\n' "$SIFIR"
bilgi "Enter'a basarak köşeli parantezdeki varsayılanı kabul edebilirsiniz."
# `|| true`: koşulu yanlış olan bir `[[ ]] && cmd` listesi başarısız döner.
# Ölçüldü — bash bunu betiğin ORTASINDA yok sayar, betik kesilmez; ama böyle
# bir satır betiğin SON komutu olursa çıkış kodu 1 olur ve kurulum başarısız
# görünür. `|| true` bu ihtimali kapatır ve niyeti görünür kılar.
[[ -n "$KONUM_UYARISI" ]] && uyari "$KONUM_UYARISI" || true

baslik "1/4 · Adres"
sor ALAN_ADI "Alan adı (boş bırakırsanız yalnız IP üzerinden HTTP)" ""
if [[ -n "$ALAN_ADI" ]]; then
  sor ACME_EPOSTA "Let's Encrypt bildirim e-postası" ""
  [[ -n "$ACME_EPOSTA" ]] || hata "alan adı verdiyseniz ACME e-postası zorunlu"
  KOK_URL="https://$ALAN_ADI"
else
  IP_TAHMIN="$(hostname -I 2>/dev/null | awk '{print $1}')"
  uyari "Alan adı yok: TLS kurulmayacak, site http://${IP_TAHMIN:-<sunucu-ip>} üzerinden açılacak."
  ACME_EPOSTA=""
  KOK_URL="http://${IP_TAHMIN:-localhost}"
fi

baslik "2/4 · Yönetici hesabı"
sor YONETICI_EPOSTA "Yönetici e-postası" ""
[[ "$YONETICI_EPOSTA" == *@*.* ]] || hata "geçerli bir e-posta girin"
sor YONETICI_KULLANICI "Yönetici kullanıcı adı" "admin"
sor_gizli YONETICI_PAROLA "Yönetici parolası (en az 10 karakter)"
[[ ${#YONETICI_PAROLA} -ge 10 ]] || hata "parola en az 10 karakter olmalı"

baslik "3/4 · Portlar ve veritabanı"
sor API_PORT "API portu (yalnız yerel dinler)" "8091"
sor WEB_PORT "Ön yüz portu (yalnız yerel dinler)" "3000"
sor DB_ADI "Veritabanı adı" "onay360"
sor DB_KULLANICI "Veritabanı kullanıcısı" "onay360"
sor DB_PAROLA "Veritabanı parolası (boş = rastgele üret)" ""
[[ -n "$DB_PAROLA" ]] || DB_PAROLA="$(openssl rand -base64 24 | tr -d '/+=' | head -c 32)"
sor SERVIS_KULLANICI "Servisleri çalıştıracak sistem kullanıcısı" "onay360"

baslik "4/4 · Sağlayıcı ve ortam"
sor HEROSMS_ANAHTAR "HeroSMS API anahtarı (boş geçebilirsiniz, sonra ekleyin)" ""
HEROSMS_URL="https://hero-sms.com"
# 🔴 IP LİSTESİ BOŞ BIRAKILAMAZ. Webhook doğrulaması FAIL-CLOSED çalışır
# (handler/webhook.go ipAllowed: liste boşsa false döner) — boş bırakılırsa
# HeroSMS'ten gelen HER bildirim sessizce düşer ve kod hiç görünmez.
# Varsayılan, sağlayıcının spec'inde yazılı iki adrestir.
HEROSMS_IPLER="84.32.223.53,185.138.88.87"
if [[ -n "$HEROSMS_ANAHTAR" ]]; then
  sor HEROSMS_URL "HeroSMS API adresi" "$HEROSMS_URL"
  sor HEROSMS_IPLER "HeroSMS webhook IP'leri (virgülle)" "$HEROSMS_IPLER"
fi
sor KATALOG_DK "Katalog senkron sıklığı, dakika (stok ve fiyat tazeliği)" "30"
sor URETIM_MI "Üretim modu mu? (üretim reCAPTCHA, Sentry ve gerçek posta ZORUNLU kılar) [e/H]" "h"

RECAPTCHA_SITE=""; RECAPTCHA_GIZLI=""; SENTRY_DSN=""; POSTA_SAGLAYICI="console"; RESEND_ANAHTAR=""
if evet_mi "$URETIM_MI"; then
  [[ -n "$ALAN_ADI" ]] || hata "üretim modu HTTPS ister; alan adı olmadan seçilemez"
  APP_ORTAM="production"
  sor RECAPTCHA_SITE "reCAPTCHA site anahtarı" ""
  sor RECAPTCHA_GIZLI "reCAPTCHA gizli anahtarı" ""
  sor SENTRY_DSN "Sentry DSN" ""
  sor POSTA_SAGLAYICI "Posta sağlayıcısı (resend | smtp)" "resend"
  if [[ "$POSTA_SAGLAYICI" == "resend" ]]; then sor RESEND_ANAHTAR "Resend API anahtarı" ""; fi
  for ad in RECAPTCHA_SITE RECAPTCHA_GIZLI SENTRY_DSN; do
    [[ -n "${!ad}" ]] || hata "üretim modunda $ad zorunlu (config açılışta reddeder)"
  done
else
  APP_ORTAM="staging"
  uyari "Staging modunda kurulacak: reCAPTCHA kapalı, e-postalar log'a yazılır."
  uyari "Üretime geçerken .env içinde APP_ENV=production yapıp eksikleri doldurun."
fi

# ─────────────────────────── Özet ve onay ───────────────────────────
baslik "Özet"
cat <<OZET
  Adres              : ${KOK_URL}
  Ortam              : ${APP_ORTAM}
  Kurulum dizini     : ${KOK}
  Servis kullanıcısı : ${SERVIS_KULLANICI}
  API portu          : 127.0.0.1:${API_PORT}
  Ön yüz portu       : 127.0.0.1:${WEB_PORT}
  Veritabanı         : ${DB_ADI} (kullanıcı: ${DB_KULLANICI})
  Yönetici           : ${YONETICI_EPOSTA}
  HeroSMS anahtarı   : $([[ -n "$HEROSMS_ANAHTAR" ]] && echo "verildi" || echo "yok — sonra eklenecek")
OZET
echo
read -rp "  Kuruluma başlansın mı? [E/h]: " ONAY || true
evet_mi "${ONAY:-e}" || { echo "  İptal edildi."; exit 0; }

# ─────────────────────────── Paketler ───────────────────────────
ADIM="sistem paketleri"
baslik "Sistem paketleri"
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq ca-certificates curl gnupg lsb-release build-essential git openssl ufw >/dev/null
tamam "temel paketler"

# PostgreSQL — PGDG deposu, çünkü Ubuntu 22.04 deposunda 16 YOKTUR.
ADIM="postgresql"
if ! command -v psql >/dev/null 2>&1; then
  install -d /usr/share/postgresql-common/pgdg
  curl -fsSL https://www.postgresql.org/media/keys/ACCC4CF8.asc \
    -o /usr/share/postgresql-common/pgdg/apt.postgresql.org.asc
  echo "deb [signed-by=/usr/share/postgresql-common/pgdg/apt.postgresql.org.asc] \
https://apt.postgresql.org/pub/repos/apt $(lsb_release -cs)-pgdg main" \
    > /etc/apt/sources.list.d/pgdg.list
  apt-get update -qq
  apt-get install -y -qq "postgresql-${PG_ANA_SURUM}" >/dev/null
fi
systemctl enable --now postgresql >/dev/null 2>&1 || true
tamam "postgresql $(psql --version | awk '{print $3}')"

ADIM="redis"
command -v redis-server >/dev/null 2>&1 || apt-get install -y -qq redis-server >/dev/null
systemctl enable --now redis-server >/dev/null 2>&1 || true
tamam "redis $(redis-server --version | awk '{print $3}' | cut -d= -f2)"

ADIM="go"
if ! command -v /usr/local/go/bin/go >/dev/null 2>&1 || \
   [[ "$(/usr/local/go/bin/go version 2>/dev/null | awk '{print $3}')" != "go${GO_SURUM}" ]]; then
  MIMARI="$(dpkg --print-architecture)"
  case "$MIMARI" in amd64) GOARCH=amd64 ;; arm64) GOARCH=arm64 ;; *) hata "desteklenmeyen mimari: $MIMARI" ;; esac
  curl -fsSL "https://go.dev/dl/go${GO_SURUM}.linux-${GOARCH}.tar.gz" -o /tmp/go.tgz
  rm -rf /usr/local/go && tar -C /usr/local -xzf /tmp/go.tgz && rm -f /tmp/go.tgz
fi
export PATH="/usr/local/go/bin:/root/go/bin:$PATH"
tamam "go $(go version | awk '{print $3}')"

ADIM="node"
if ! command -v node >/dev/null 2>&1 || [[ "$(node -v | cut -c2- | cut -d. -f1)" -lt "$NODE_ANA_SURUM" ]]; then
  curl -fsSL "https://deb.nodesource.com/setup_${NODE_ANA_SURUM}.x" | bash - >/dev/null 2>&1
  apt-get install -y -qq nodejs >/dev/null
fi
tamam "node $(node -v)"

ADIM="caddy"
if ! command -v caddy >/dev/null 2>&1; then
  curl -fsSL https://dl.cloudsmith.io/public/caddy/stable/gpg.key \
    | gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
  curl -fsSL https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt \
    > /etc/apt/sources.list.d/caddy-stable.list
  apt-get update -qq
  apt-get install -y -qq caddy >/dev/null
fi
tamam "caddy $(caddy version | head -1)"

ADIM="goose"
command -v goose >/dev/null 2>&1 || \
  GOBIN=/usr/local/bin go install "github.com/pressly/goose/v3/cmd/goose@${GOOSE_SURUM}" >/dev/null 2>&1
tamam "goose"

# ─────────────────────────── Sistem kullanıcısı ───────────────────────────
ADIM="sistem kullanıcısı"
baslik "Sistem kullanıcısı"
if ! id -u "$SERVIS_KULLANICI" >/dev/null 2>&1; then
  # --system: giriş yapılamayan servis hesabı. Uygulamanın kendi kullanıcısı
  # olması, bir güvenlik açığının root'a değil bu hesaba düşmesi demektir.
  useradd --system --create-home --shell /usr/sbin/nologin "$SERVIS_KULLANICI"
fi
tamam "$SERVIS_KULLANICI"

# ─────────────────────────── Veritabanı ───────────────────────────
ADIM="veritabanı"
baslik "Veritabanı"
VAR_MI="$(sudo -u postgres psql -tAc "SELECT 1 FROM pg_roles WHERE rolname='${DB_KULLANICI}'" || true)"
if [[ "$VAR_MI" != "1" ]]; then
  sudo -u postgres psql -qc "CREATE ROLE ${DB_KULLANICI} LOGIN PASSWORD '${DB_PAROLA}'" >/dev/null
  tamam "rol oluşturuldu: ${DB_KULLANICI}"
else
  # Rol zaten varsa parolayı .env ile HİZALA: yeniden çalıştırmada .env yeni
  # parola üretmiş olabilir ve eşleşmezse API açılışta bağlanamaz.
  sudo -u postgres psql -qc "ALTER ROLE ${DB_KULLANICI} PASSWORD '${DB_PAROLA}'" >/dev/null
  tamam "rol vardı, parola güncellendi"
fi
DB_VAR="$(sudo -u postgres psql -tAc "SELECT 1 FROM pg_database WHERE datname='${DB_ADI}'" || true)"
if [[ "$DB_VAR" != "1" ]]; then
  sudo -u postgres createdb -O "$DB_KULLANICI" "$DB_ADI"
  tamam "veritabanı oluşturuldu: ${DB_ADI}"
else
  tamam "veritabanı zaten var: ${DB_ADI} (VERİYE DOKUNULMADI)"
fi

# ─────────────────────────── .env ───────────────────────────
ADIM=".env"
baslik "Yapılandırma"
ENV_YOLU="$KOK/.env"
if [[ -f "$ENV_YOLU" ]]; then
  # 🔴 SIRLAR KORUNUR. Yeniden üretmek tüm oturumları düşürür ve — çok daha
  # kötüsü — ENCRYPTION_KEY değişirse veritabanındaki şifreli sağlayıcı API
  # anahtarları BİR DAHA AÇILAMAZ.
  OTURUM_SIRRI="$(grep -E '^SESSION_SECRET=' "$ENV_YOLU" | cut -d= -f2- || true)"
  SIFRELEME_ANAHTARI="$(grep -E '^ENCRYPTION_KEY=' "$ENV_YOLU" | cut -d= -f2- || true)"
  WEBHOOK_SIRRI="$(grep -E '^WEBHOOK_HEROSMS_SECRET=' "$ENV_YOLU" | cut -d= -f2- || true)"
  cp "$ENV_YOLU" "${ENV_YOLU}.yedek.$(date +%s)"
  uyari "mevcut .env yedeklendi, sırlar korunuyor"
fi
[[ -n "${OTURUM_SIRRI:-}" ]]       || OTURUM_SIRRI="$(openssl rand -base64 32)"
[[ -n "${SIFRELEME_ANAHTARI:-}" ]] || SIFRELEME_ANAHTARI="$(openssl rand -base64 32)"
[[ -n "${WEBHOOK_SIRRI:-}" ]]      || WEBHOOK_SIRRI="$(openssl rand -hex 16)"

cat > "$ENV_YOLU" <<ENVDOSYASI
# Onay360 — deploy/kurulum.sh tarafından üretildi ($(date -Iseconds))
# 🔴 BU DOSYA SIR İÇERİR. Depoya eklenmez, yedeği şifreli tutulur.

APP_ENV=${APP_ORTAM}
HTTP_ADDR=127.0.0.1:${API_PORT}
LOG_LEVEL=info
PUBLIC_BASE_URL=${KOK_URL}

DATABASE_URL=postgres://${DB_KULLANICI}:${DB_PAROLA}@127.0.0.1:5432/${DB_ADI}?sslmode=disable
REDIS_URL=redis://127.0.0.1:6379/0

SESSION_SECRET=${OTURUM_SIRRI}
# Oturum süresi: config katmanı 60 dakikanın altını REDDEDER.
SESSION_TTL=720h
ENCRYPTION_KEY=${SIFRELEME_ANAHTARI}

FX_PROVIDER=tcmb
FX_SAFETY_MARGIN_PCT=2.0
FX_MAX_AGE=30m

RECAPTCHA_SITE_KEY=${RECAPTCHA_SITE}
RECAPTCHA_SECRET_KEY=${RECAPTCHA_GIZLI}

MAIL_PROVIDER=${POSTA_SAGLAYICI}
MAIL_FROM=noreply@${ALAN_ADI:-localhost}
RESEND_API_KEY=${RESEND_ANAHTAR}

# İşler API sürecinde koşar: ayrı bir worker birimi AÇILMAZ. false yaparsanız
# onay360-worker.service'i de etkinleştirmeniz gerekir, yoksa iade ve
# yoklama işleri HİÇ çalışmaz.
WORKERS_IN_PROCESS=true

WEBHOOK_HEROSMS_SECRET=${WEBHOOK_SIRRI}
WEBHOOK_HEROSMS_ALLOWED_IPS=${HEROSMS_IPLER}
# Caddy aynı makinede; yalnız yerel geri döngü güvenilir.
TRUSTED_PROXIES=127.0.0.1/32,::1/128

UPLOAD_DIR=/var/lib/onay360/uploads

SENTRY_DSN=${SENTRY_DSN}
METRICS_ENABLED=true
ENVDOSYASI

chmod 600 "$ENV_YOLU"
chown "$SERVIS_KULLANICI":"$SERVIS_KULLANICI" "$ENV_YOLU"
install -d -o "$SERVIS_KULLANICI" -g "$SERVIS_KULLANICI" -m 750 /var/lib/onay360/uploads
tamam ".env yazıldı (600, ${SERVIS_KULLANICI})"

# ─────────────────────────── Derleme ───────────────────────────
ADIM="api derlemesi"
baslik "Derleme"
install -d "$KOK/bin"
( cd "$KOK/api" && go build -trimpath -ldflags="-s -w" -o "$KOK/bin/api"    ./cmd/server )
( cd "$KOK/api" && go build -trimpath -ldflags="-s -w" -o "$KOK/bin/worker" ./cmd/worker )
( cd "$KOK/api" && go build -trimpath -ldflags="-s -w" -o "$KOK/bin/cli"    ./cmd/cli )
tamam "api · worker · cli"

ADIM="ön yüz derlemesi"
# 🔴 NEXT_PUBLIC_* DERLEME ANINDA GÖMÜLÜR. Sonradan .env'e yazmak ETKİSİZDİR;
# değiştirmek için ön yüzü YENİDEN DERLEMEK gerekir (deploy/Dockerfile.web
# aynı tuzağı belgeliyor).
( cd "$KOK/web" \
  && npm ci --no-audit --no-fund >/dev/null 2>&1 \
  && NEXT_TELEMETRY_DISABLED=1 NODE_ENV=production \
     NEXT_PUBLIC_SITE_URL="$KOK_URL" \
     NEXT_PUBLIC_RECAPTCHA_SITE_KEY="$RECAPTCHA_SITE" \
     npm run build >/dev/null )
tamam "ön yüz (standalone)"

chown -R "$SERVIS_KULLANICI":"$SERVIS_KULLANICI" "$KOK/bin" "$KOK/web/.next"

# ─────────────────────────── Migration ───────────────────────────
ADIM="migration"
baslik "Veritabanı şeması"
set -a; . "$ENV_YOLU"; set +a
( cd "$KOK/api" && goose -dir migrations postgres "$DATABASE_URL" up )
tamam "migration'lar uygulandı"

# ─────────────────────────── Tohumlama ───────────────────────────
ADIM="tohumlama"
baslik "Tohumlama"
CLI="$KOK/bin/cli"

# Yönetici hesabı. Parola ORTAM DEĞİŞKENİYLE geçer, komut satırıyla DEĞİL:
# argümanlar `ps` çıktısına ve kabuk geçmişine düşer.
if ONAY360_KURULUM_PAROLA="$YONETICI_PAROLA" "$CLI" user:create \
     --email="$YONETICI_EPOSTA" --username="$YONETICI_KULLANICI" \
     --env-password=ONAY360_KURULUM_PAROLA --role=admin 2>/dev/null; then
  tamam "yönetici hesabı: $YONETICI_EPOSTA"
else
  uyari "yönetici hesabı zaten var — atlandı"
fi

if [[ -n "$HEROSMS_ANAHTAR" ]]; then
  if HEROSMS_API_KEY="$HEROSMS_ANAHTAR" "$CLI" provider:add \
       --name=herosms --protocol=HEROSMS_V1 \
       --base-url="$HEROSMS_URL" --env-key=HEROSMS_API_KEY \
       --priority=10 --cost-multiplier=1.0 --active=true 2>/dev/null; then
    tamam "sağlayıcı eklendi: herosms"
  else
    uyari "sağlayıcı zaten var — atlandı"
  fi
fi

# `seed` kuru ve katalogu çeker, sonra "satışa hazır mı" raporunu basar.
# Rapor bilerek EKRANA düşer: kurulumun açılması ile satış yapılabilmesi ayrı
# şeylerdir ve fark yalnız burada görünür.
echo
"$CLI" seed || uyari "tohumlama kısmen başarısız — sonra: ${KOK}/bin/cli seed"

# ─────────────────────────── systemd ───────────────────────────
ADIM="systemd"
baslik "Servisler"
# systemd sertleştirmesi — servis hesabının yetkisini gerektiği kadarla sınırlar.
#
# 🔴 `ProtectHome=read-only`, `true` DEĞİL. `true` /home ve /root'u tamamen
# GİZLER; depo oraya klonlanmışsa (yaygın: `git clone` doğrudan /root altında)
# servis kendi ikili dosyasını bile göremez ve birim "Exec format error" ya da
# "No such file" ile ölür — sertleştirme yüzünden olduğu hata mesajından
# ANLAŞILMAZ. `read-only` gizlemez, yalnız yazmayı engeller; uygulamanın
# ihtiyacı zaten okumaktır.
#
# `ProtectSystem=strict` tüm dosya sistemini salt okunur yapar; yazılabilir
# yollar AÇIKÇA sayılır. Bu yüzden yükleme dizini burada, ön yüzün önbelleği
# ise kendi biriminde ayrıca eklenir.
ortak_sertlestirme() {
  cat <<'SERT'
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=read-only
ReadWritePaths=/var/lib/onay360
ProtectKernelTunables=true
ProtectControlGroups=true
RestrictSUIDSGID=true
LockPersonality=true
SERT
}

cat > /etc/systemd/system/onay360-api.service <<APIBIRIM
[Unit]
Description=Onay360 API
After=network-online.target postgresql.service redis-server.service
Wants=network-online.target
Requires=postgresql.service redis-server.service

[Service]
Type=simple
User=${SERVIS_KULLANICI}
WorkingDirectory=${KOK}/api
EnvironmentFile=${ENV_YOLU}
ExecStart=${KOK}/bin/api
Restart=always
RestartSec=3
$(ortak_sertlestirme)

[Install]
WantedBy=multi-user.target
APIBIRIM

# Ön yüz: Next standalone kendi sunucusunu taşır. `server.js` .next/standalone
# içindedir ve statik dosyaları AYRI dizinlerden okur — bu yüzden çalışma
# dizini standalone kökü olmalıdır.
cat > /etc/systemd/system/onay360-web.service <<WEBBIRIM
[Unit]
Description=Onay360 web (Next.js standalone)
After=network-online.target onay360-api.service
Wants=network-online.target

[Service]
Type=simple
User=${SERVIS_KULLANICI}
WorkingDirectory=${KOK}/web/.next/standalone
Environment=NODE_ENV=production
Environment=PORT=${WEB_PORT}
Environment=HOSTNAME=127.0.0.1
ExecStart=/usr/bin/node server.js
Restart=always
RestartSec=3
$(ortak_sertlestirme)
# Next standalone çalışma anında `.next/cache` altına yazar; `ProtectSystem=strict`
# altında bu yol açıkça yazılabilir olmalıdır, yoksa ilk istekte EACCES alınır.
ReadWritePaths=${KOK}/web/.next

[Install]
WantedBy=multi-user.target
WEBBIRIM

# Worker birimi YAZILIR AMA ETKİNLEŞTİRİLMEZ: WORKERS_IN_PROCESS=true iken
# worker açılışta bilerek hata verir (aynı iş iki süreçte koşarsa sağlayıcıya
# çift istek gider). .env'de false yaparsanız `systemctl enable --now
# onay360-worker` demeniz yeterli.
cat > /etc/systemd/system/onay360-worker.service <<WORKERBIRIM
[Unit]
Description=Onay360 arka plan işleri (YALNIZ WORKERS_IN_PROCESS=false iken)
After=network-online.target postgresql.service redis-server.service
Requires=postgresql.service redis-server.service

[Service]
Type=simple
User=${SERVIS_KULLANICI}
WorkingDirectory=${KOK}/api
EnvironmentFile=${ENV_YOLU}
ExecStart=${KOK}/bin/worker
Restart=always
RestartSec=5
$(ortak_sertlestirme)

[Install]
WantedBy=multi-user.target
WORKERBIRIM

# Next standalone statik dosyaları kendi kopyalamaz — derleme çıktısındaki
# `static` ve `public` elle taşınır. Atlanırsa site CSS'siz açılır.
install -d "$KOK/web/.next/standalone/.next"
cp -r "$KOK/web/.next/static" "$KOK/web/.next/standalone/.next/static"
if [[ -d "$KOK/web/public" ]]; then cp -r "$KOK/web/public" "$KOK/web/.next/standalone/public"; fi
chown -R "$SERVIS_KULLANICI":"$SERVIS_KULLANICI" "$KOK/web/.next"

# ══════════════════════════════════════════════════════════════════════════
# KATALOG SENKRONU — systemd ZAMANLAYICI (bu kurulumun en kolay atlanan yanı)
# ══════════════════════════════════════════════════════════════════════════
# 🔴 API SÜRECİNDE KATALOG SENKRONU YOKTUR. `worker/jobs.go` içindeki işler
# şunlardır: fx-sync (10 dk), rental-sync (6 sa), order-poller, expirer,
# reaper'lar, webhook-ingest. AKTİVASYON KATALOGU (servisler, ülkeler, fiyat
# ve STOK) bu listede YOK — yalnız `cli catalog:sync` ile çekilir.
#
# Sonuç: zamanlayıcı kurulmazsa katalog KURULUM ANINDA DONAR. Stoklar aylarca
# değişmez, tükenmiş ürünler listede kalır, kullanıcı satın alır, sağlayıcı
# "numara yok" der ve para çekilip iade edilir. Sistem "çalışıyor" görünür.
#
# `Persistent=true`: sunucu kapalıyken kaçan tur, açılışta bir kez koşar.
# `RandomizedDelaySec`: birden çok kurulum aynı dakikada sağlayıcıya yığılmasın.
cat > /etc/systemd/system/onay360-katalog.service <<KATALOGBIRIM
[Unit]
Description=Onay360 katalog senkronu (HeroSMS servis/ülke/fiyat/stok)
After=network-online.target onay360-api.service

[Service]
Type=oneshot
User=${SERVIS_KULLANICI}
WorkingDirectory=${KOK}/api
EnvironmentFile=${ENV_YOLU}
ExecStart=${KOK}/bin/cli seed
$(ortak_sertlestirme)
KATALOGBIRIM

cat > /etc/systemd/system/onay360-katalog.timer <<KATALOGZAMAN
[Unit]
Description=Onay360 katalog senkronunu her ${KATALOG_DK} dakikada bir çalıştırır

[Timer]
OnBootSec=5min
OnUnitActiveSec=${KATALOG_DK}min
RandomizedDelaySec=60
Persistent=true

[Install]
WantedBy=timers.target
KATALOGZAMAN

systemctl daemon-reload
systemctl enable --now onay360-api.service >/dev/null
systemctl enable --now onay360-web.service >/dev/null
systemctl enable --now onay360-katalog.timer >/dev/null
tamam "onay360-api · onay360-web · onay360-katalog.timer (${KATALOG_DK} dk)"

# ─────────────────────────── Caddy ───────────────────────────
ADIM="caddy yapılandırması"
baslik "Ters vekil"
if [[ -n "$ALAN_ADI" ]]; then
  # Depodaki Caddyfile KULLANILIR — kopyalanmaz. SSE tamponlaması, güvenlik
  # başlıkları ve sıkıştırma istisnaları oradaki ölçülmüş kararlardır.
  install -d /etc/caddy
  cp "$KOK/deploy/Caddyfile" /etc/caddy/Caddyfile
  cat > /etc/caddy/env <<CADDYENV
DOMAIN=${ALAN_ADI}
ACME_EMAIL=${ACME_EPOSTA}
API_UPSTREAM=127.0.0.1:${API_PORT}
WEB_UPSTREAM=127.0.0.1:${WEB_PORT}
CADDYENV
  install -d /etc/systemd/system/caddy.service.d
  cat > /etc/systemd/system/caddy.service.d/onay360.conf <<'CADDYOVR'
[Service]
EnvironmentFile=/etc/caddy/env
CADDYOVR
  systemctl daemon-reload
else
  # Alan adı yok: TLS yok, yalnız 80. Depodaki dosya `{$DOMAIN}` bekliyor ve
  # boş bir ad geçersiz bir site bloğu üretir; bu yüzden sade bir yapılandırma.
  cat > /etc/caddy/Caddyfile <<CADDYIP
:80 {
	handle /api/* {
		reverse_proxy 127.0.0.1:${API_PORT} {
			flush_interval -1
			header_up X-Real-IP {http.request.remote.host}
		}
	}
	handle /healthz { reverse_proxy 127.0.0.1:${API_PORT} }
	handle /readyz  { reverse_proxy 127.0.0.1:${API_PORT} }
	handle /metrics* { respond 404 }
	handle /debug/*  { respond 404 }
	handle { reverse_proxy 127.0.0.1:${WEB_PORT} }
}
CADDYIP
fi
caddy validate --config /etc/caddy/Caddyfile >/dev/null 2>&1 || hata "Caddy yapılandırması geçersiz"
systemctl enable --now caddy >/dev/null
systemctl reload caddy >/dev/null 2>&1 || systemctl restart caddy
tamam "caddy"

# ─────────────────────────── Güvenlik duvarı ───────────────────────────
ADIM="güvenlik duvarı"
if command -v ufw >/dev/null 2>&1; then
  ufw allow OpenSSH >/dev/null 2>&1 || true
  ufw allow 80/tcp  >/dev/null 2>&1 || true
  ufw allow 443/tcp >/dev/null 2>&1 || true
  # API ve web YALNIZ 127.0.0.1 dinliyor; dışarıya açılmaları gerekmiyor.
  yes | ufw enable >/dev/null 2>&1 || true
  tamam "ufw: 22, 80, 443"
fi

# ─────────────────────────── Sağlık ───────────────────────────
ADIM="sağlık kontrolü"
baslik "Sağlık kontrolü"
SAGLIK=1
for _ in $(seq 1 30); do
  if curl -fsS "http://127.0.0.1:${API_PORT}/readyz" >/dev/null 2>&1; then SAGLIK=0; break; fi
  sleep 2
done
if [[ $SAGLIK -eq 0 ]]; then tamam "API hazır"; else uyari "API hazır değil → journalctl -u onay360-api -n 50"; fi

WEB_SAGLIK=1
for _ in $(seq 1 30); do
  if curl -fsS -o /dev/null "http://127.0.0.1:${WEB_PORT}/" 2>/dev/null; then WEB_SAGLIK=0; break; fi
  sleep 2
done
if [[ $WEB_SAGLIK -eq 0 ]]; then tamam "ön yüz hazır"; else uyari "ön yüz hazır değil → journalctl -u onay360-web -n 50"; fi

# ─────────────────────────── Özet ───────────────────────────
trap - ERR
baslik "Kurulum tamam"
cat <<SON
  Adres        : ${KOK_URL}
  Yönetim      : ${KOK_URL}/yonetim
  Yönetici     : ${YONETICI_EPOSTA}

  Servisler    : systemctl status onay360-api onay360-web caddy
  Katalog      : systemctl list-timers onay360-katalog.timer
  Log          : journalctl -u onay360-api -f
  CLI          : ${KOK}/bin/cli help
  Yapılandırma : ${ENV_YOLU}  (600, ${SERVIS_KULLANICI})

  Güncelleme   : git pull && sudo bash deploy/kurulum.sh
SON

# ── HeroSMS panelinde yapılacak ayar ──
# Webhook bizim tarafımızda hazır ama sağlayıcı onu BİLMİYOR. Adres onların
# panelinden kaydedilmeden tek bir bildirim gelmez; kodlar yalnız 30 saniyelik
# yoklamayla görünür (çalışır ama yavaştır).
if [[ -n "$HEROSMS_ANAHTAR" ]]; then
  baslik "HeroSMS panelinde yapmanız gereken"
  bilgi "Webhook adresini sağlayıcının paneline kaydedin:"
  printf '\n    %s%s/api/v1/webhooks/herosms/%s%s\n\n' "$KALIN" "$KOK_URL" "$WEBHOOK_SIRRI" "$SIFIR"
  bilgi "Bu adres bir SIR içerir — kimseyle paylaşmayın, ekran görüntüsüne almayın."
  bilgi "İzin verilen kaynak IP'ler: ${HEROSMS_IPLER}"
  bilgi "  (liste .env içinde WEBHOOK_HEROSMS_ALLOWED_IPS; boş bırakılırsa"
  bilgi "   her bildirim SESSİZCE düşer — doğrulama fail-closed çalışır.)"
fi

baslik "Kurulumdan sonra"
bilgi "1. /yonetim/odeme-yontemleri → IBAN ve USDT cüzdanını doldurup etkinleştirin."
bilgi "   Yapılmazsa kullanıcı bakiye YÜKLEYEMEZ; sistem satış yapamaz."
bilgi "2. /yonetim/fiyatlar → GLOBAL marj %40 tohumlandı, kendi marjınızı yazın."
bilgi "3. /yonetim/saglayicilar → çarpanı kontrol edin (1.0 = maliyet düzeltmesi yok)."
bilgi "4. Yedekleme: deploy/scripts/yedekle.sh — cron'a ekleyin."

if [[ -z "$HEROSMS_ANAHTAR" ]]; then
  echo
  uyari "Sağlayıcı eklenmedi — katalog boş, satış YAPILAMAZ. Eklemek için:"
  bilgi "  HEROSMS_API_KEY=... ${KOK}/bin/cli provider:add --name=herosms \\"
  bilgi "      --protocol=HEROSMS_V1 --base-url=https://hero-sms.com --env-key=HEROSMS_API_KEY"
  bilgi "  ${KOK}/bin/cli fx:sync && ${KOK}/bin/cli catalog:sync --provider=herosms"
fi

if [[ "$APP_ORTAM" != "production" ]]; then
  echo
  uyari "Staging modunda: e-postalar log'a yazılıyor, reCAPTCHA kapalı."
  bilgi "  Üretime geçmek için ${ENV_YOLU} içinde APP_ENV=production yapın ve"
  bilgi "  RECAPTCHA_*, SENTRY_DSN, MAIL_PROVIDER değerlerini doldurun."
  bilgi "  Eksik olursa API AÇILMAZ — config açılışta reddeder (bilinçli)."
fi
echo
