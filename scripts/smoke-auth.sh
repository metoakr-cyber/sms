#!/usr/bin/env bash
# Kimlik akışının uçtan uca duman testi.
# Sunucuyu başlatır, senaryoları koşar, sonuçları raporlar, temizler.
set -uo pipefail
cd "$(dirname "$0")/.."
set -a && source .env && set +a

PORT="${HTTP_ADDR#:}"
API="http://localhost:$PORT/api/v1"
LOG=/tmp/smoke-api.log
PASS=0; FAIL=0

pass() { printf '  \033[32m✓\033[0m %s\n' "$1"; PASS=$((PASS+1)); }
fail() { printf '  \033[31m✗\033[0m %s\n     %s\n' "$1" "${2:-}"; FAIL=$((FAIL+1)); }
# Yerelde Docker konteynerine, CI'da doğrudan servise bağlanırız.
if docker ps --format '{{.Names}}' 2>/dev/null | grep -q smsplatform-dev-postgres-1; then
  psql()  { docker exec smsplatform-dev-postgres-1 psql -U smsplatform -d smsplatform -qtA "$@"; }
  rediscli() { docker exec smsplatform-dev-redis-1 redis-cli "$@"; }
else
  psql()  { command psql "$DATABASE_URL" -qtA "$@"; }
  rediscli() { command redis-cli -u "$REDIS_URL" "$@"; }
fi

# ── temiz başlangıç ──
pkill -f '/tmp/smoke-api' 2>/dev/null; sleep 0.5

# Test verisini temizle.
#
# ledger_entries DEĞİŞMEZDİR ve users'a referans verir; bu yüzden kullanıcılar
# defter kaydı silinmeden silinemez. Bu ÜRETİMDE İSTENEN davranıştır — mali
# kayıt kendini korur. Test ortamında tetikleyiciyi geçici olarak kapatıyoruz.
psql -c "ALTER TABLE ledger_entries DISABLE TRIGGER ledger_no_delete;" >/dev/null 2>&1
psql -c "DELETE FROM ledger_entries;" >/dev/null 2>&1
psql -c "ALTER TABLE ledger_entries ENABLE TRIGGER ledger_no_delete;" >/dev/null 2>&1
psql -c "DELETE FROM users;" >/dev/null 2>&1

REMAIN=$(psql -c "SELECT count(*) FROM users;" 2>/dev/null)
[ "$REMAIN" = "0" ] || { echo "temizlik başarısız: $REMAIN kullanıcı kaldı"; exit 1; }
rediscli FLUSHDB >/dev/null 2>&1
SIZE=$(rediscli DBSIZE 2>/dev/null | tr -dc '0-9')
[ "${SIZE:-1}" = "0" ] || {
  echo "redis temizlenemedi (DBSIZE=${SIZE:-?}) — hız limiti sayaçları koşular arasında birikir"
  exit 1
}

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

S=$(code -X POST "$API/admin/users/$PUB/balance" -b /tmp/cj -H 'Content-Type: application/json' \
  -d '{"amountMinor":25050,"note":"hos geldin bakiyesi"}')
B=$(python3 -c "import json;print(json.load(open('/tmp/sm.body'))['balance']['formatted'])" 2>/dev/null)
[ "$S" = 200 ] && [ "$B" = "250,50 ₺" ] && pass "admin bakiye yükledi → $B" || fail "bakiye düzeltme" "HTTP $S $(body)"

S=$(code -X POST "$API/admin/users/$PUB/balance" -b /tmp/cj -H 'Content-Type: application/json' \
  -d '{"amountMinor":100,"note":"kisa"}')
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

echo "─── Askıya alma (KK-104) ───"
psql -c "UPDATE users SET status='SUSPENDED' WHERE email='ali@ornek.com';" >/dev/null
S=$(code "$API/me" -b /tmp/cj)
[ "$(field code)" = ACCOUNT_SUSPENDED ] \
  && pass "askıya alınan kullanıcının açık oturumu bir sonraki istekte düştü" || fail "askıya alma" "HTTP $S $(body)"

echo
printf '\033[1m%d geçti, %d başarısız\033[0m\n' "$PASS" "$FAIL"
[ "$FAIL" -eq 0 ]
