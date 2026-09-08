#!/usr/bin/env bash
# Kimlik akışının uçtan uca duman testi.
# Sunucuyu başlatır, senaryoları koşar, sonuçları raporlar, temizler.
# set -e KULLANILMIYOR: bu betik KASITLI olarak başarısız çağrılar yapar
# (401/403/409 senaryoları). Koruma `set -e`'den değil, aşağıdaki AÇIK
# ortam kapılarından gelir — onlar başarısızlıkta doğrudan exit 1 der.
set -uo pipefail
cd "$(dirname "$0")/.."

# MEVCUT ORTAM KAZANIR — .env yalnız BOŞLUKLARI doldurur.
#
# `set -a && source .env` çağıran tarafından verilen değişkenleri EZİYORDU.
# check.sh testleri ayrı bir veritabanına yönlendirmek için DATABASE_URL'i
# dışa aktarıyor; bu satır onu geliştirme veritabanına geri çeviriyor ve duman
# testi geliştirme kullanıcılarını siliyordu.
#
# Aynı kural Go tarafında da geçerli (config.LoadDotEnv / godotenv): dosya
# ortamı ezmez. İki yerde iki farklı öncelik olamaz.
_pre_db="${DATABASE_URL:-}"
_pre_redis="${REDIS_URL:-}"
set -a && source .env && set +a
[ -n "$_pre_db" ] && DATABASE_URL="$_pre_db"
[ -n "$_pre_redis" ] && REDIS_URL="$_pre_redis"
export DATABASE_URL REDIS_URL

PORT="${HTTP_ADDR#:}"
API="http://localhost:$PORT/api/v1"
LOG=/tmp/smoke-api.log
PASS=0; FAIL=0

pass() { printf '  \033[32m✓\033[0m %s\n' "$1"; PASS=$((PASS+1)); }
fail() { printf '  \033[31m✗\033[0m %s\n     %s\n' "$1" "${2:-}"; FAIL=$((FAIL+1)); }
# Yerelde Docker konteynerine, CI'da doğrudan servise bağlanırız.
#
# Docker yolunda bile DATABASE_URL onurlandırılır: sabit bir veritabanı adı
# kullanmak, betiğin uygulamadan FARKLI bir veritabanına konuşmasına yol açar
# ve aşağıdaki ortam kapısını anlamsızlaştırır.
#
# Konteynerin içinden host portuna (localhost:55432) erişilemez; bu yüzden
# docker yolunda URL'i olduğu gibi kullanamayız. Ama VERİTABANI ADINI URL'den
# alırız — kapının denetlediği şey budur.
if docker ps --format '{{.Names}}' 2>/dev/null | grep -q smsplatform-dev-postgres-1; then
  DB_FROM_URL="${DATABASE_URL##*/}"; DB_FROM_URL="${DB_FROM_URL%%\?*}"
  psql()  { docker exec -i smsplatform-dev-postgres-1 psql -U smsplatform -d "$DB_FROM_URL" -qtA "$@"; }
  rediscli() { docker exec smsplatform-dev-redis-1 redis-cli "$@"; }
else
  psql()  { command psql "$DATABASE_URL" -qtA "$@"; }
  rediscli() { command redis-cli -u "$REDIS_URL" "$@"; }
fi

# ─────────────────────── ORTAM KAPISI ───────────────────────
# Bu betik VERİ SİLER. 'make check' zincirinde olduğu için her gün çalışıyor.
# Yanlış bir .env (staging/üretim) ile çalıştırılması geri alınamaz kayıp demektir.
# Bu yüzden iki bağımsız kontrol var ve ikisi de geçmeden hiçbir şey silinmez.
[ "${APP_ENV:-}" = "development" ] || {
  echo "✗ smoke YALNIZ development'ta çalışır (APP_ENV=${APP_ENV:-tanımsız})"
  echo "  Bu betik veri siler; yanlış ortamda çalıştırılmasına izin verilmez."
  exit 1
}
DBNAME=$(psql -c "SELECT current_database();" 2>/dev/null | tr -d '[:space:]' || true)
case "$DBNAME" in
  smsplatform|smsplatform_test) : ;;
  *) echo "✗ beklenmeyen veritabanı: '${DBNAME:-okunamadı}' — silme yapılmadı"; exit 1 ;;
esac

# ── temiz başlangıç ──
pkill -f '/tmp/smoke-api' 2>/dev/null || true
sleep 0.5

# Test verisini temizle.
#
# ledger_entries DEĞİŞMEZDİR ve users'a referans verir; bu yüzden kullanıcılar
# defter kaydı silinmeden silinemez. Bu ÜRETİMDE İSTENEN davranıştır.
#
# ALTER TABLE ... DISABLE TRIGGER KULLANILMAZ: betik iki ALTER arasında
# kesilirse koruma KALICI olarak kapalı kalır ve kimse fark etmez.
# Onun yerine migration 00004'teki oturum kapsamlı kaçış kapısı kullanılır:
# SET LOCAL yalnız o transaction içinde geçerlidir, sızması imkânsızdır.
# test: aşağıdaki "ledger koruma tetikleyicisi" doğrulaması her koşuda çalışır
# TRUNCATE ... CASCADE KULLANILMAZ: pricing_rules ve diğer tablolar users'a
# referans verdiği için cascade migration TOHUM VERİSİNİ de siler (varsayılan
# GLOBAL fiyat kuralı dahil) ve sonraki testler gizemli şekilde kırılır.
# Yalnız bu betiğin ürettiği veriyi, doğru sırada sileriz.
clean_test_data() {
  psql -q -c "BEGIN;
              SET LOCAL app.allow_ledger_truncate = 'on';
              SET LOCAL session_replication_role = 'replica';
              DELETE FROM ledger_entries;
              DELETE FROM price_quotes;
              DELETE FROM sessions;
              DELETE FROM auth_tokens;
              DELETE FROM user_roles;
              DELETE FROM audit_logs;
              UPDATE pricing_rules SET created_by_user_id = NULL;
              DELETE FROM users;
              COMMIT;" >/dev/null
}
clean_test_data

# Tohum verisi KORUNMUŞ olmalı — cascade kazası olmadığının kanıtı.
# Eksikse geri koyarız: bir betik hatası veritabanını kalıcı bozuk bırakmamalı.
psql -q -c "INSERT INTO pricing_rules (scope, margin_percent, note)
            SELECT 'GLOBAL', 40.00, 'duman testi tohumu'
            WHERE NOT EXISTS (SELECT 1 FROM pricing_rules WHERE scope='GLOBAL' AND is_active);" >/dev/null
SEED=$(psql -c "SELECT count(*) FROM pricing_rules WHERE scope='GLOBAL' AND is_active;")
[ "$SEED" = "1" ] || { echo "✗ GLOBAL fiyat kuralı kurulamadı (adet=$SEED)"; exit 1; }
ROLES=$(psql -c "SELECT count(*) FROM roles;")
[ "$ROLES" -ge 2 ] || { echo "✗ rol tohumu kayboldu"; exit 1; }

# Korumanın hâlâ AÇIK olduğunu doğrula.
#
# BOŞ bir tabloda BEFORE DELETE tetikleyicisi ateşlenmez (silinecek satır yok),
# bu yüzden önce bir satır yazıp sonra silmeyi denemek gerekir. Aksi halde
# "koruma çalışıyor" sonucu yalancı olurdu.
GUARD_UID=$(psql -c "INSERT INTO users (email,username,password_hash,status)
                     VALUES ('guard@test.local','guardcheck','x','ACTIVE') RETURNING id;")
psql -c "INSERT INTO ledger_entries
           (user_id,amount_minor,entry_type,balance_after_minor,idempotency_key)
         VALUES ($GUARD_UID,1,'ADJUSTMENT',1,'guard-check');" >/dev/null
GUARD=$(psql -c "DELETE FROM ledger_entries WHERE idempotency_key='guard-check';" 2>&1 || true)
case "$GUARD" in
  *degistirilemez*) : ;;  # beklenen: tetikleyici engelledi
  *) echo "✗ ledger koruma tetikleyicisi ETKİN DEĞİL — durduruldu"; exit 1 ;;
esac
# Kontrol satırını temizle (yalnız izinli yoldan).
#
# BURADA TRUNCATE ... CASCADE VARDI VE TOHUM VERİSİNİ SİLİYORDU.
# Betiğin başındaki yorum cascade kullanılmadığını söylüyordu; kod ise tam da
# onu yapıyordu. Cascade `pricing_rules`a kadar iniyor, varsayılan GLOBAL
# fiyat kuralı yok oluyor ve uygulama sonraki her teklifte NO_PRICING_RULE ile
# düşüyordu — testler yeşil, uygulama bozuk.
#
# Yorumun iddia ettiği garanti artık koda bağlı: yalnız bu betiğin ürettiği
# veriyi siliyoruz.
clean_test_data

REMAIN=$(psql -c "SELECT count(*) FROM users;" 2>/dev/null)
[ "$REMAIN" = "0" ] || { echo "temizlik başarısız: $REMAIN kullanıcı kaldı"; exit 1; }

# Tohum verisi HÂLÂ yerinde olmalı — cascade'in geri gelmediğinin kanıtı.
SEED=$(psql -c "SELECT count(*) FROM pricing_rules WHERE scope='GLOBAL' AND is_active;")
[ "$SEED" = "1" ] || { echo "✗ temizlik GLOBAL fiyat kuralını sildi (adet=$SEED)"; exit 1; }
rediscli FLUSHDB >/dev/null 2>&1
SIZE=$(rediscli DBSIZE 2>/dev/null | tr -dc '0-9')
[ "${SIZE:-1}" = "0" ] || {
  echo "redis temizlenemedi (DBSIZE=${SIZE:-?}) — hız limiti sayaçları koşular arasında birikir"
  exit 1
}

# Port MEŞGUL MÜ: geliştirme sunucusu ayaktayken duman testi kendi sunucusunu
# başlatamaz ve "sunucu başlamadı" diye anlamsız bir hatayla düşerdi. Sebebi
# söylemeyen bir hata, hatanın kendisinden daha çok zaman kaybettirir.
if lsof -nP -iTCP:"$PORT" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "✗ $PORT portu zaten kullanımda — muhtemelen 'make dev' çalışıyor."
  echo "  Duman testi kendi sunucusunu başlatır; önce diğerini durdurun."
  exit 1
fi

(cd api && go build -o /tmp/smoke-api ./cmd/server) || { echo "derleme başarısız"; exit 1; }
rm -f "$LOG"; /tmp/smoke-api > "$LOG" 2>&1 &
SRV=$!
trap 'kill $SRV 2>/dev/null; wait $SRV 2>/dev/null' EXIT

for _ in $(seq 1 40); do curl -s -o /dev/null "http://localhost:$PORT/healthz" && break; sleep 0.3; done
grep -q "auth/register" "$LOG" || { echo "sunucu başlamadı:"; head -20 "$LOG"; exit 1; }

# ── yardımcılar ──
code()  { curl -sS -o /tmp/sm.body -w '%{http_code}' "$@"; }
body()  { cat /tmp/sm.body; }
field() { python3 -c "import json,sys;d=json.load(open('/tmp/sm.body'));print(d.get('error',{}).get('code') or d.get('$1',''))" 2>/dev/null; }

echo "─── Kayıt ───"
S=$(code -X POST "$API/auth/register" -H 'Content-Type: application/json' \
  -d '{"email":"ali@ornek.com","username":"ali_veli","password":"dogru-at-pil-zimba","acceptTerms":true}')
[ "$S" = 200 ] && pass "kayıt oluşturuldu" || fail "kayıt" "HTTP $S $(body)"

S=$(code -X POST "$API/auth/register" -H 'Content-Type: application/json' \
  -d '{"email":"z@ornek.com","username":"zayif","password":"password123!","acceptTerms":true}')
[ "$(field code)" = WEAK_PASSWORD ] && pass "zayıf şifre reddedildi" || fail "zayıf şifre" "$(body)"

S=$(code -X POST "$API/auth/register" -H 'Content-Type: application/json' \
  -d '{"email":"ALI@ORNEK.COM","username":"baskasi","password":"tamamen-baska-bir-sifre","acceptTerms":true}')
[ "$(field code)" = EMAIL_TAKEN ] && pass "büyük harfli aynı e-posta reddedildi (CITEXT)" || fail "e-posta tekrarı" "$(body)"

echo "─── E-posta doğrulama ───"
TOK=$(grep -oE 'dogrula\?token=[A-Za-z0-9_-]+' "$LOG" | tail -1 | cut -d= -f2)
[ -n "$TOK" ] && pass "doğrulama e-postası gönderildi" || fail "e-posta yok"

LEN=$(psql -c "SELECT length(token_hash) FROM auth_tokens LIMIT 1;")
[ "$LEN" = 32 ] && pass "veritabanında ham token yok, yalnız SHA-256 özeti" || fail "token saklama" "uzunluk=$LEN"

S=$(code -X POST "$API/auth/verify-email" -H 'Content-Type: application/json' -d "{\"token\":\"$TOK\"}")
[ "$S" = 200 ] && pass "e-posta doğrulandı" || fail "doğrulama" "HTTP $S $(body)"

S=$(code -X POST "$API/auth/verify-email" -H 'Content-Type: application/json' -d "{\"token\":\"$TOK\"}")
[ "$(field code)" = TOKEN_INVALID ] && pass "token tek kullanımlık — ikinci kez reddedildi" || fail "token tekrar" "$(body)"

echo "─── Giriş ───"
rm -f /tmp/cj
S=$(code -X POST "$API/auth/login" -H 'Content-Type: application/json' -c /tmp/cj \
  -d '{"email":"ali@ornek.com","password":"dogru-at-pil-zimba"}')
[ "$S" = 200 ] && pass "giriş başarılı" || fail "giriş" "HTTP $S $(body)"
grep -qi '#HttpOnly' /tmp/cj && pass "oturum çerezi HttpOnly" || fail "çerez HttpOnly değil"

S=$(code -X POST "$API/auth/login" -H 'Content-Type: application/json' \
  -d '{"email":"ali@ornek.com","password":"yanlis-sifre-bu"}')
C1=$(field code)
S=$(code -X POST "$API/auth/login" -H 'Content-Type: application/json' \
  -d '{"email":"hicyok@ornek.com","password":"yanlis-sifre-bu"}')
C2=$(field code)
[ "$C1" = INVALID_CREDENTIALS ] && [ "$C2" = INVALID_CREDENTIALS ] \
  && pass "yanlış şifre ve var olmayan hesap AYNI hatayı veriyor (hesap sayımı engellendi)" \
  || fail "hesap sayımı" "$C1 vs $C2"

echo "─── Oturum ───"
S=$(code "$API/me" -b /tmp/cj)
U=$(python3 -c "import json;d=json.load(open('/tmp/sm.body'));print(d.get('username',''),d.get('emailVerified',''),d.get('balance',{}).get('formatted',''))" 2>/dev/null)
[ "$S" = 200 ] && pass "/me çalışıyor → $U" || fail "/me" "HTTP $S $(body)"

S=$(code "$API/me")
[ "$S" = 401 ] && pass "oturumsuz /me → 401" || fail "oturumsuz erişim" "HTTP $S"

S=$(code "$API/me" -H 'Cookie: sid=uydurma-oturum-kimligi')
[ "$S" = 401 ] && pass "sahte oturum çerezi → 401" || fail "sahte çerez" "HTTP $S"

echo "─── Hesap kilidi (KK-102) ───"
# IP sayacını sıfırla: bu test HESAP bazlı kilidi ölçüyor, IP limitini değil.
rediscli --scan --pattern 'rl:auth:*' 2>/dev/null |   xargs -r rediscli DEL >/dev/null 2>&1
for i in $(seq 1 6); do
  code -X POST "$API/auth/login" -H 'Content-Type: application/json' \
    -d '{"email":"ali@ornek.com","password":"yanlis"}' >/dev/null
done
S=$(code -X POST "$API/auth/login" -H 'Content-Type: application/json' \
  -d '{"email":"ali@ornek.com","password":"dogru-at-pil-zimba"}')
[ "$(field code)" = TOO_MANY_ATTEMPTS ] \
  && pass "5 başarısız denemeden sonra hesap kilitlendi" || fail "hesap kilidi" "$(body)"

echo "─── Şifre sıfırlama (KK-103) ───"
rediscli FLUSHDB >/dev/null 2>&1  # kilidi temizle
S=$(code -X POST "$API/auth/password/forgot" -H 'Content-Type: application/json' -d '{"email":"ali@ornek.com"}')
M1=$(field message)
S=$(code -X POST "$API/auth/password/forgot" -H 'Content-Type: application/json' -d '{"email":"hicyok@ornek.com"}')
M2=$(field message)
[ -n "$M1" ] && [ "$M1" = "$M2" ] \
  && pass "var olan/olmayan e-posta AYNI yanıt (hesap sayımı engellendi)" || fail "sıfırlama yanıtı" "$M1 vs $M2"

RTOK=$(grep -oE 'sifre-sifirla\?token=[A-Za-z0-9_-]+' "$LOG" | tail -1 | cut -d= -f2)
S=$(code -X POST "$API/auth/password/reset" -H 'Content-Type: application/json' \
  -d "{\"token\":\"$RTOK\",\"password\":\"yepyeni-guclu-sifre\"}")
[ "$S" = 200 ] && pass "şifre sıfırlandı" || fail "sıfırlama" "HTTP $S $(body)"

S=$(code "$API/me" -b /tmp/cj)
[ "$S" = 401 ] && pass "şifre değişince ESKİ oturum anında düştü" || fail "oturum düşmedi" "HTTP $S"

S=$(code -X POST "$API/auth/login" -H 'Content-Type: application/json' \
  -d '{"email":"ali@ornek.com","password":"yepyeni-guclu-sifre"}' -c /tmp/cj)
[ "$S" = 200 ] && pass "yeni şifreyle giriş yapılabiliyor" || fail "yeni şifre" "HTTP $S $(body)"

echo "─── Cüzdan (M2) ───"
S=$(code "$API/wallet/balance" -b /tmp/cj)
B=$(python3 -c "import json;print(json.load(open('/tmp/sm.body'))['balance']['formatted'])" 2>/dev/null)
[ "$S" = 200 ] && [ "$B" = "0,00 ₺" ] && pass "yeni kullanıcı bakiyesi 0,00 ₺" || fail "bakiye" "HTTP $S $(body)"

# admin rolü ver ve manuel yükleme yap
psql -c "INSERT INTO user_roles (user_id, role_id) SELECT u.id, r.id FROM users u, roles r WHERE u.email='ali@ornek.com' AND r.name='admin' ON CONFLICT DO NOTHING;" >/dev/null
PUB=$(psql -c "SELECT public_id FROM users WHERE email='ali@ornek.com';")

ADJ='{"amountMinor":25050,"note":"hos geldin bakiyesi","idempotencyKey":"smoke-adj-0001"}'
S=$(code -X POST "$API/admin/users/$PUB/balance" -b /tmp/cj -H 'Content-Type: application/json' -d "$ADJ")
B=$(python3 -c "import json;print(json.load(open('/tmp/sm.body'))['balance']['formatted'])" 2>/dev/null)
[ "$S" = 200 ] && [ "$B" = "250,50 ₺" ] && pass "admin bakiye yükledi → $B" || fail "bakiye düzeltme" "HTTP $S $(body)"

# Aynı anahtarla TEKRAR: bakiye DEĞIŞMEMELİ, tek kayıt olmalı.
S=$(code -X POST "$API/admin/users/$PUB/balance" -b /tmp/cj -H 'Content-Type: application/json' -d "$ADJ")
B2=$(python3 -c "import json;d=json.load(open('/tmp/sm.body'));print(d['balance']['formatted'],d.get('alreadyApplied'))" 2>/dev/null)
N=$(psql -c "SELECT count(*) FROM ledger_entries WHERE entry_type='ADJUSTMENT';")
[ "$N" = "1" ] && [ "$B2" = "250,50 ₺ True" ] \
  && pass "aynı anahtarla tekrar → tek kayıt, alreadyApplied=true" \
  || fail "idempotency" "kayıt=$N yanıt=$B2"

# FARKLI anahtar AYRI işlemdir — sessizce yutulmamalı.
S=$(code -X POST "$API/admin/users/$PUB/balance" -b /tmp/cj -H 'Content-Type: application/json' \
  -d '{"amountMinor":25050,"note":"ikinci mesru duzeltme","idempotencyKey":"smoke-adj-0002"}')
N=$(psql -c "SELECT count(*) FROM ledger_entries WHERE entry_type='ADJUSTMENT';")
[ "$N" = "2" ] && pass "farklı anahtar → ayrı işlem (sessiz yutma yok)" || fail "farklı anahtar" "kayıt=$N $(body)"

# Anahtarsız istek REDDEDİLMELİ.
S=$(code -X POST "$API/admin/users/$PUB/balance" -b /tmp/cj -H 'Content-Type: application/json' \
  -d '{"amountMinor":100,"note":"anahtarsiz istek"}')
[ "$(field code)" = VALIDATION ] && pass "idempotencyKey'siz istek reddedildi" || fail "anahtar zorunluluğu" "$(body)"

S=$(code -X POST "$API/admin/users/$PUB/balance" -b /tmp/cj -H 'Content-Type: application/json' \
  -d '{"amountMinor":100,"note":"kisa","idempotencyKey":"smoke-adj-0003"}')
[ "$(field code)" = VALIDATION ] && pass "kısa açıklama reddedildi (denetlenebilirlik)" || fail "not zorunluluğu" "$(body)"

S=$(code "$API/wallet/entries" -b /tmp/cj)
N=$(python3 -c "import json;d=json.load(open('/tmp/sm.body'));print(d['total'],d['items'][0]['typeLabel'],d['items'][0]['amount']['formatted'])" 2>/dev/null)
[ "$S" = 200 ] && pass "hareket dökümü → $N" || fail "döküm" "HTTP $S $(body)"

DRIFT=$(psql -c "SELECT coalesce(sum(abs(drift)),0) FROM ledger_reconciliation;")
[ "$DRIFT" = "0" ] && pass "mutabakat sapması sıfır (KK-200)" || fail "mutabakat" "sapma=$DRIFT"

# yetkisiz kullanıcı bakiye değiştiremez
S=$(code -X POST "$API/auth/register" -H 'Content-Type: application/json' \
  -d '{"email":"sivil@ornek.com","username":"sivil","password":"bagimsiz-uzun-parola","acceptTerms":true}')
[ "$S" = 200 ] || fail "ikinci kullanıcı kaydı" "HTTP $S $(body)"
psql -c "UPDATE users SET status='ACTIVE', email_verified_at=now() WHERE email='sivil@ornek.com';" >/dev/null
rm -f /tmp/cj2
S=$(code -X POST "$API/auth/login" -H 'Content-Type: application/json' -c /tmp/cj2 \
  -d '{"email":"sivil@ornek.com","password":"bagimsiz-uzun-parola"}')
[ "$S" = 200 ] || fail "ikinci kullanıcı girişi" "HTTP $S $(body)"
S=$(code -X POST "$API/admin/users/$PUB/balance" -b /tmp/cj2 -H 'Content-Type: application/json' \
  -d '{"amountMinor":999999,"note":"kendime para"}')
[ "$(field code)" = FORBIDDEN ] && pass "izinsiz kullanıcı bakiye değiştiremiyor → 403" || fail "RBAC" "HTTP $S $(body)"

S=$(code "$API/wallet/entries" -b /tmp/cj2)
N=$(python3 -c "import json;print(json.load(open('/tmp/sm.body'))['total'])" 2>/dev/null)
[ "$N" = "0" ] && pass "kullanıcı başkasının hareketlerini göremiyor (KK-206)" || fail "sahiplik" "total=$N"

# FR-101: DOĞRULANMAMIŞ e-postayla satın alma/teklif YAPILAMAZ.
psql -c "UPDATE users SET email_verified_at = NULL, status='PENDING_VERIFICATION'
         WHERE email='sivil@ornek.com';" >/dev/null
S=$(code "$API/catalog/quote?serviceCode=tg&countryIso=RU" -b /tmp/cj2)
[ "$(field code)" = EMAIL_NOT_VERIFIED ] \
  && pass "doğrulanmamış kullanıcı teklif alamıyor (FR-101)" \
  || fail "e-posta doğrulama kapısı" "HTTP $S $(body)"

# Oturum listesi ham token SIZDIRMAMALI.
SID=$(awk '$6=="sid"{print $7}' /tmp/cj 2>/dev/null | tail -1)
S=$(code "$API/me/sessions" -b /tmp/cj)
LEAK=$(SID="$SID" python3 -c "
import json, os
d = json.load(open('/tmp/sm.body'))
sid = os.environ.get('SID', '')
print('SIZDI' if sid and any(i['id'] == sid for i in d.get('items', [])) else 'temiz')" 2>/dev/null)
[ "$S" = 200 ] && [ "$LEAK" = "temiz" ] \
  && pass "oturum listesi ham token sızdırmıyor (httpOnly korunuyor)" \
  || fail "oturum token sızıntısı" "HTTP $S sonuç=$LEAK"

# Handle bir kimlik değildir: oturum çerezi olarak kullanılamamalı.
HANDLE=$(python3 -c "import json;d=json.load(open('/tmp/sm.body'));print(d['items'][0]['id'])" 2>/dev/null)
S=$(code "$API/me" -H "Cookie: sid=$HANDLE")
[ "$S" = 401 ] && pass "handle oturum çerezi olarak kullanılamıyor" || fail "handle kabul edildi" "HTTP $S"

echo "─── Askıya alma (KK-104) ───"
psql -c "UPDATE users SET status='SUSPENDED' WHERE email='ali@ornek.com';" >/dev/null
S=$(code "$API/me" -b /tmp/cj)
[ "$(field code)" = ACCOUNT_SUSPENDED ] \
  && pass "askıya alınan kullanıcının açık oturumu bir sonraki istekte düştü" || fail "askıya alma" "HTTP $S $(body)"

echo
printf '\033[1m%d geçti, %d başarısız\033[0m\n' "$PASS" "$FAIL"
[ "$FAIL" -eq 0 ]
