#!/usr/bin/env bash
# Uçtan uca (Playwright) test koşucusu.
#
# Kendi veritabanını kurar, Go API'sini ve Next sunucusunu KENDİ portlarında
# başlatır, ikisinin de HAZIR olmasını yoklayarak bekler, testleri koşar ve
# çıkarken hepsini durdurur.
#
# set -e KULLANILMIYOR: aşağıdaki kapılar başarısızlıkta AÇIKÇA `exit 1` der ve
# sebebini yazar. `set -e` sessizce çıkar, hangi adımda düşüldüğü kaybolur
# (scripts/smoke-auth.sh ile aynı gerekçe).
set -uo pipefail
cd "$(dirname "$0")/.."
ROOT="$PWD"

# ─────────────────── ortam ───────────────────
#
# MEVCUT ORTAM KAZANIR — .env yalnız BOŞLUKLARI doldurur. Tersi olsaydı
# çağıranın verdiği DATABASE_URL sessizce geliştirme veritabanına çevrilirdi
# (smoke-auth.sh'te tam olarak bu hata yaşandı).
_pre_db="${DATABASE_URL:-}"
_pre_redis="${REDIS_URL:-}"
[ -f .env ] && { set -a; source .env; set +a; }
[ -n "$_pre_db" ] && DATABASE_URL="$_pre_db"
[ -n "$_pre_redis" ] && REDIS_URL="$_pre_redis"
export PATH="$PATH:$(go env GOPATH 2>/dev/null)/bin"

API_PORT="${E2E_API_PORT:-8092}"
WEB_PORT="${E2E_WEB_PORT:-3100}"
BASE_URL="http://localhost:$WEB_PORT"
API_BASE="http://127.0.0.1:$API_PORT/api/v1"
LOGDIR="$ROOT/.e2e-logs"
API_LOG="$LOGDIR/api.log"
WEB_LOG="$LOGDIR/web.log"

# ─────────────────── ortam kapısı ───────────────────
[ "${APP_ENV:-}" = "development" ] || {
  echo "✗ e2e YALNIZ development'ta çalışır (APP_ENV=${APP_ENV:-tanımsız})"
  exit 1
}

# ─────────────────── veritabanı ───────────────────
#
# UÇTAN UCA TESTLER KENDİ VERİTABANINDA KOŞAR — geliştirme veritabanında DEĞİL.
#
# İki bağımsız sebep:
#  1) Bu betik veritabanını SIFIRDAN KURAR (drop + create). Geliştirme
#     veritabanına yapılsaydı katalogunuz ve hesabınız her koşuda silinirdi;
#     bu depoda tam olarak bu hata birden çok kez yaşandı.
#  2) FAKE sağlayıcı uzak sipariş kimliğini süreç içi bir sayaçtan üretir
#     ("fake-1", "fake-2", …) ve sayaç her API başlangıcında SIFIRLANIR.
#     `orders(provider_id, remote_order_id)` benzersizdir; kalıcı bir
#     veritabanında ikinci koşunun ilk satın alması 23505 ile düşer.
#     Temiz veritabanı bu çakışmayı kökünden kaldırır.
#
# sed KULLANILIYOR, bash desen değiştirme DEĞİL: ${VAR/desen/yeni} içinde `?`
# bir JOKER karakterdir ve URL'yi bozar (bkz. scripts/check.sh).
if [ -z "${E2E_DATABASE_URL:-}" ]; then
  E2E_DATABASE_URL=$(printf '%s' "$DATABASE_URL" \
    | sed -e 's|/smsplatform?|/smsplatform_e2e?|' -e 's|/smsplatform$|/smsplatform_e2e|')
fi
E2E_DBNAME="${E2E_DATABASE_URL##*/}"; E2E_DBNAME="${E2E_DBNAME%%\?*}"

# İKİ BAĞIMSIZ KAPI. Biri yanlış yapılandırmaya, diğeri elle verilen yanlış
# bir URL'ye karşıdır; ikisi de geçmeden hiçbir veritabanı silinmez.
if [ "$E2E_DATABASE_URL" = "$DATABASE_URL" ]; then
  echo "✗ E2E_DATABASE_URL, DATABASE_URL ile AYNI — bu betik veritabanını SİLER."
  exit 1
fi
case "$E2E_DBNAME" in
  *_e2e) : ;;
  *) echo "✗ e2e veritabanının adı '_e2e' ile bitmeli (verilen: '$E2E_DBNAME')"; exit 1 ;;
esac

# Yerelde Docker konteynerine, CI'da doğrudan servise bağlanırız
# (scripts/smoke-auth.sh ile aynı desen).
if docker ps --format '{{.Names}}' 2>/dev/null | grep -q smsplatform-dev-postgres-1; then
  yonet() { docker exec -i smsplatform-dev-postgres-1 psql -U smsplatform -d postgres -qtA "$@"; }
else
  ADMIN_DB=$(printf '%s' "$E2E_DATABASE_URL" | sed -e "s|/$E2E_DBNAME|/postgres|")
  yonet() { command psql "$ADMIN_DB" -qtA "$@"; }
fi

# ─────────────────── Redis ───────────────────
#
# AYRI bir Redis veritabanı: hız limiti sayaçları ve oturumlar geliştirme
# sunucusuyla paylaşılırsa, siz `make dev` ile gezinirken testler 429 alır ve
# ÜRÜNDE HATA YOKKEN düşer.
REDIS_URL=$(printf '%s' "$REDIS_URL" | sed -E 's|/[0-9]+$||')/15
export REDIS_URL
export DATABASE_URL="$E2E_DATABASE_URL"

# ─────────────────── uygulama ayarları ───────────────────
#
# Posta sağlayıcısı KONSOL olmalı: e-posta doğrulama token'ı yalnız log'a
# düşer (veritabanında SHA-256 özeti tutulur, ham token yoktur). Başka bir
# sağlayıcıda kayıt akışı token'ı hiç göremez.
export MAIL_PROVIDER=console
export LOG_LEVEL="${LOG_LEVEL:-info}"
# Arka plan işleri API sürecinde koşmalı: SMS kodu `order-poller` ile gelir.
export WORKERS_IN_PROCESS=true
export HTTP_ADDR=":$API_PORT"
export PUBLIC_BASE_URL="$BASE_URL"
# reCAPTCHA: anahtar verilirse kayıt formu bot tarafından doldurulamaz ve
# ilgili test kendini ATLAR (sebebini rapora yazarak).
export NEXT_PUBLIC_RECAPTCHA_SITE_KEY="${NEXT_PUBLIC_RECAPTCHA_SITE_KEY:-}"

# YÖNETİCİ HESABI BU KOŞUYA AİTTİR ve parolası burada ÜRETİLİR.
#
# Depoya bir parola yazmak, onu bu deponun her kopyasında geçerli bir tahmin
# hâline getirirdi. Hesap her koşuda sıfırdan kurulduğu için sabit bir parolaya
# ihtiyaç da yok.
export E2E_ADMIN_EMAIL="${E2E_ADMIN_EMAIL:-e2e-yonetici@ornek.test}"
export E2E_ADMIN_PASSWORD="${E2E_ADMIN_PASSWORD:-$(openssl rand -base64 24 | tr -d '/+=' )}"
ADMIN_USERNAME="e2e_yonetici"

# ─────────────────── port kontrolü ───────────────────
#
# Geliştirme sunucusu ayaktayken bu betik kendi sunucusunu başlatamaz ve
# "sunucu başlamadı" gibi sebebini söylemeyen bir hatayla düşerdi.
for p in "$API_PORT" "$WEB_PORT"; do
  if lsof -nP -iTCP:"$p" -sTCP:LISTEN >/dev/null 2>&1; then
    echo "✗ $p portu zaten kullanımda."
    echo "  e2e kendi sunucularını başlatır; önce diğerini durdurun ya da"
    echo "  E2E_API_PORT / E2E_WEB_PORT ile başka port verin."
    exit 1
  fi
done

# ─────────────────── tarayıcılar ───────────────────
if [ "${E2E_INSTALL_BROWSERS:-0}" = "1" ] || [ -n "${CI:-}" ]; then
  (cd web && npx playwright install --with-deps chromium webkit) || {
    echo "✗ tarayıcılar kurulamadı"; exit 1; }
fi

mkdir -p "$LOGDIR"
# Dizin KENDİNİ yok sayar: log, iz ve ekran görüntüleri depoya girmemeli ve
# bunu hatırlamak `git add -A` yazan kişiye kalmamalı.
printf '*\n' > "$LOGDIR/.gitignore"
: > "$API_LOG"; : > "$WEB_LOG"

API_PID=""; WEB_PID=""
temizle() {
  [ -n "$WEB_PID" ] && kill "$WEB_PID" 2>/dev/null
  [ -n "$API_PID" ] && kill "$API_PID" 2>/dev/null

  # PORTTAN DA TEMİZLE.
  #
  # `npx next start` bir ARA SÜREÇ yaratır; ebeveyni öldürmek çocuğu bırakır ve
  # port bir sonraki koşuda meşgul kalır — betik o zaman "port kullanımda" deyip
  # kendi bıraktığı artık yüzünden hiç başlamaz. Yalnız BİZİM portlarımıza
  # dokunuruz ve başlangıçta o portların boş olduğunu zaten doğruladık.
  local kalan
  for p in "$WEB_PORT" "$API_PORT"; do
    kalan=$(lsof -ti tcp:"$p" -sTCP:LISTEN 2>/dev/null)
    [ -n "$kalan" ] && kill $kalan 2>/dev/null
  done
  wait 2>/dev/null
}
trap temizle EXIT INT TERM

# hazir_mi <url> <saniye> — SABİT UYKU YOK, koşul sağlanınca hemen döner.
hazir_mi() {
  local url="$1" limit="$2" i=0
  while [ "$i" -lt $((limit * 5)) ]; do
    # -s: henüz ayakta olmayan sunucu için "bağlanılamadı" gürültüsü basılmaz;
    # gerçek hata zaten çağıranın net mesajıyla bildirilir.
    curl -s -o /dev/null --max-time 3 "$url" && return 0
    i=$((i + 1)); sleep 0.2
  done
  return 1
}

# ─────────────────── veritabanını kur ───────────────────
echo "─── Veritabanı ($E2E_DBNAME) ───"
# WITH (FORCE): açık bir bağlantı (önceki koşudan kalan bir istemci) DROP'u
# engellerdi ve betik "oluşturulamadı" deyip dururdu.
yonet -c "DROP DATABASE IF EXISTS $E2E_DBNAME WITH (FORCE);" >/dev/null 2>&1
yonet -c "CREATE DATABASE $E2E_DBNAME;" >/dev/null || {
  echo "✗ $E2E_DBNAME oluşturulamadı"; exit 1; }

(cd api && goose -dir migrations postgres "$E2E_DATABASE_URL" up >/dev/null 2>&1) || {
  echo "✗ migration'lar uygulanamadı"; exit 1; }

# Sağlayıcı + katalog + kur. Roller ve varsayılan GLOBAL fiyat kuralı
# migration tohumundan gelir.
(cd api && go run ./cmd/cli provider:add --name=e2e-fake --protocol=FAKE >/dev/null 2>&1) || {
  echo "✗ sahte sağlayıcı eklenemedi"; exit 1; }
(cd api && go run ./cmd/cli catalog:sync --provider=e2e-fake >/dev/null 2>&1) || {
  echo "✗ katalog senkronu başarısız"; exit 1; }
# Kur olmadan hiçbir teklif üretilemez (maliyet USD, satış TL).
(cd api && go run ./cmd/cli fx:sync >/dev/null 2>&1) || {
  echo "✗ döviz kuru alınamadı (ağ gerekiyor)"; exit 1; }
echo "  şema + katalog hazır"

# ─────────────────── API ───────────────────
echo "─── API derleniyor ───"
(cd api && go build -o "$LOGDIR/api-bin" ./cmd/server) || {
  echo "✗ API derlenemedi"; exit 1; }

"$LOGDIR/api-bin" >"$API_LOG" 2>&1 &
API_PID=$!
hazir_mi "http://127.0.0.1:$API_PORT/healthz" 30 || {
  echo "✗ API $API_PORT portunda ayağa kalkmadı:"; tail -30 "$API_LOG"; exit 1; }
echo "  API hazır  :$API_PORT"

# ─────────────────── yönetici hesabı ───────────────────
#
# Hesap uygulamanın KENDİ kayıt akışıyla açılır; yalnız rol ataması CLI ile
# yapılır (rol atamak için bir HTTP ucu yoktur ve olmamalıdır).
curl -s -o /dev/null -X POST "$API_BASE/auth/register" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"$E2E_ADMIN_EMAIL\",\"username\":\"$ADMIN_USERNAME\",\"password\":\"$E2E_ADMIN_PASSWORD\",\"acceptTerms\":true}" || {
  echo "✗ yönetici kaydı yapılamadı"; exit 1; }

TOKEN=$(grep -oE 'dogrula\?token=[A-Za-z0-9_-]+' "$API_LOG" | tail -1 | cut -d= -f2)
[ -n "$TOKEN" ] || { echo "✗ doğrulama token'ı log'da bulunamadı"; tail -20 "$API_LOG"; exit 1; }
curl -s -o /dev/null -X POST "$API_BASE/auth/verify-email" \
  -H 'Content-Type: application/json' -d "{\"token\":\"$TOKEN\"}"

(cd api && go run ./cmd/cli admin:grant --email="$E2E_ADMIN_EMAIL" --role=admin >/dev/null 2>&1) || {
  echo "✗ yönetici rolü atanamadı"; exit 1; }
echo "  yönetici hesabı hazır ($E2E_ADMIN_EMAIL)"

# ─────────────────── WEB ───────────────────
export API_ORIGIN="http://127.0.0.1:$API_PORT"
export NEXT_PUBLIC_SITE_URL="$BASE_URL"
export NEXT_DIST_DIR=".next-e2e"
# Derleme çıktısı depoya girmemeli. Kök .gitignore yalnız `/web/.next/`i
# tanıyor; ayrı bir dist dizini kullanan herkes kendi yok saymasını yanına
# koyar (check.sh'in `.next-check` deseniyle aynı sorun, aynı çözüm).
mkdir -p "web/$NEXT_DIST_DIR" && printf '*\n' > "web/$NEXT_DIST_DIR/.gitignore"

if [ "${E2E_WEB_DEV:-0}" = "1" ]; then
  # Hızlı yineleme yolu. Sayfalar ilk açılışta derlendiği için ilk gezinme
  # yavaştır; gerçek koşu (ve CI) üretim derlemesini kullanır.
  echo "─── Web (geliştirme kipi) ───"
  (cd web && npx next dev -p "$WEB_PORT" >"$WEB_LOG" 2>&1) &
  WEB_PID=$!
else
  echo "─── Web derleniyor ───"
  (cd web && npx next build >"$WEB_LOG" 2>&1) || {
    echo "✗ web derlenemedi:"; tail -40 "$WEB_LOG"; exit 1; }
  printf '*\n' > "web/$NEXT_DIST_DIR/.gitignore"
  (cd web && npx next start -p "$WEB_PORT" >>"$WEB_LOG" 2>&1) &
  WEB_PID=$!
fi

hazir_mi "$BASE_URL/giris" 120 || {
  echo "✗ web $WEB_PORT portunda ayağa kalkmadı:"; tail -40 "$WEB_LOG"; exit 1; }
echo "  Web hazır  :$WEB_PORT"

# Vekil gerçekten API'ye ulaşıyor mu: yanlış API_ORIGIN ile her test
# "öğe bulunamadı" diye düşerdi ve sebebi görünmezdi.
hazir_mi "$BASE_URL/api/v1/catalog/services" 15 || {
  echo "✗ web → API vekili çalışmıyor ($BASE_URL/api/v1)"; exit 1; }

# ─────────────────── testler ───────────────────
export E2E_BASE_URL="$BASE_URL"
export E2E_API_LOG="$API_LOG"
export E2E_REDIS_URL="$REDIS_URL"

echo "─── Playwright ───"
(cd web && npx playwright test "$@")
SONUC=$?

echo
if [ "$SONUC" -eq 0 ]; then
  printf '\033[32m\033[1mUÇTAN UCA TESTLER GEÇTİ\033[0m\n'
else
  printf '\033[31m\033[1mUÇTAN UCA TESTLER DÜŞTÜ\033[0m (çıkış %s)\n' "$SONUC"
  echo "  rapor:   $LOGDIR/rapor/index.html"
  echo "  api log: $API_LOG"
  echo "  web log: $WEB_LOG"
fi
exit "$SONUC"
