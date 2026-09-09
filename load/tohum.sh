#!/usr/bin/env bash
# Yük testi tohumu: test kullanıcıları, bakiye ve SSE için açık siparişler.
#
# TASARIM KARARLARI (rapor: docs/yuk-testi-sonuc.md §3)
#
# 1. Kullanıcılar SQL ile üretilir, /auth/register ile değil.
#    /auth/* IP başına 30 istek/dk ile sınırlı (NFR-802). 50 kullanıcıyı API
#    ile kaydetmek tek başına iki dakika sürer ve ölçülen şey kayıt akışı
#    değil. Parola hash'i, API ile kaydedilmiş bir ŞABLON kullanıcıdan
#    kopyalanır — yani gerçek uygulamanın ürettiği hash biçimi kullanılır.
#
# 2. Bakiye YALNIZ yönetim ucundan (POST /admin/users/:id/balance) verilir.
#    Değişmez #2: bakiye WalletRepo.ApplyEntry dışında değişmez. SQL ile
#    balance_minor yazmak defteri bozardı ve testin ölçmek istediği
#    mutabakatı en baştan anlamsız kılardı.
#
# 3. Oturumlar BİR KEZ açılır ve dosyaya yazılır. Her k6 koşumunda yeniden
#    giriş yapmak, 30/dk'lık IP limitine takılır ve koşumun ilk yarısını
#    401'e çevirir. Oturum TTL'i 720 saat; tohum tekrar kullanılabilir.
#
# Bu betik VERİ SİLMEZ. Yalnız INSERT ve kendi ürettiği satırlarda UPDATE
# yapar; temizlik operatörün işidir (load/README.md §Temizlik).
# test: scripts/yuk-testi_test.sh#silme_ifadesi_yok
set -uo pipefail

KOK="$(cd "$(dirname "$0")/.." && pwd)"
CIKTI="${YUK_CIKTI:-$KOK/load/sonuclar}"
PORT="${YUK_PORT:-8191}"
API="http://localhost:$PORT/api/v1"
N="${KULLANICI_SAYISI:-50}"
BAKIYE_KURUS="${BAKIYE_KURUS:-100000}" # 1.000,00 TL
PG_KAP="${PG_KAP:-smsplatform-dev-postgres-1}"
DBADI="${DBADI:-smsplatform}"
PAROLA="yuk-testi-parolasi-uzun"

# Hız limiti duvarları (NFR-802). Tohum bunlara ÇARPMAMAK için yavaşlar;
# çarparsa ölçtüğümüz şey sistemin hızı değil, limitin kendisi olur.
AUTH_BEKLEME="${AUTH_BEKLEME:-2.4}"   # /auth/*  : 30 istek/dk/IP
ADMIN_BEKLEME="${ADMIN_BEKLEME:-1.1}" # /admin/* : 60 istek/dk/kullanıcı

mkdir -p "$CIKTI"
psql() { docker exec -i "$PG_KAP" psql -U smsplatform -d "$DBADI" -qtA "$@"; }

log() { printf '  %s\n' "$*"; }
hata() { printf '\033[31m  ✗ %s\033[0m\n' "$*"; exit 1; }

giris() { # giris <eposta> → sid (boş dönerse başarısız)
  curl -s -i -X POST "$API/auth/login" -H 'Content-Type: application/json' \
    -d "{\"email\":\"$1\",\"password\":\"$PAROLA\"}" \
    | sed -n 's/^[Ss]et-[Cc]ookie: sid=\([^;]*\).*/\1/p'
}

# ── Oturum yenileme modu ──
#
# Oturumlar Redis'te durur. Geliştirme Redis'i sıfırlandığında (make reset,
# başka bir testin flush'ı) tohum kullanıcıları yerinde kalır ama çerezleri
# ölür ve k6 koşumu baştan sona 401 döner. O durumda kullanıcıları yeniden
# üretmek gereksiz; yalnız giriş yapmak yeter.
if [ "${1:-}" = "yenile" ]; then
  [ -s "$CIKTI/kullanicilar.json" ] || hata "yenilenecek tohum yok: $CIKTI/kullanicilar.json"
  TOPLAM=$(jq 'length' "$CIKTI/kullanicilar.json")
  log "oturumlar yenileniyor ($TOPLAM kullanıcı, ${AUTH_BEKLEME}s aralık — /auth 30 istek/dk/IP)"
  : > "$CIKTI/kullanicilar.ndjson"
  i=0
  while read -r satir; do
    i=$((i + 1))
    EPOSTA=$(echo "$satir" | jq -r .email)
    SID=""
    for deneme in 1 2 3; do
      SID=$(giris "$EPOSTA")
      [ -n "$SID" ] && break
      sleep 20
    done
    [ -n "$SID" ] || hata "giriş yapılamadı: $EPOSTA"
    echo "$satir" | jq -c --arg sid "$SID" '.sid = $sid' >> "$CIKTI/kullanicilar.ndjson"
    sleep "$AUTH_BEKLEME"
    printf '\r  oturum: %d/%d' "$i" "$TOPLAM"
  done < <(jq -c '.[]' "$CIKTI/kullanicilar.json")
  printf '\n'
  jq -s '.' "$CIKTI/kullanicilar.ndjson" > "$CIKTI/kullanicilar.json"
  log "oturumlar yenilendi"
  exit 0
fi

# ── SSE için açık sipariş ──
#
# SSE senaryosu TERMİNAL OLMAYAN sipariş ister (handler/order.go isTerminal).
#
# ÖNCE MEVCUT AÇIK SİPARİŞ ARANIR, sonra yenisi açılır. Sebep, FAKE
# sağlayıcının uzak sipariş kimliğini süreç içi bir sayaçtan üretmesidir
# ("fake-1", "fake-2", …): sunucu her yeniden başladığında sayaç 1'e döner ve
# veritabanında zaten duran satırlarla `orders_remote_uniq` çakışır. Kalıcı
# bir geliştirme veritabanında ikinci koşum bu yüzden satın alamaz.
# Bulgunun ayrıntısı: docs/yuk-testi-sonuc.md §6.2.
siparis_ac() {
  local toplam i sid pub oid q qid o
  [ -s "$CIKTI/kullanicilar.json" ] || hata "tohum yok: $CIKTI/kullanicilar.json"
  toplam=$(jq 'length' "$CIKTI/kullanicilar.json")
  log "$toplam kullanıcı için açık sipariş hazırlanıyor (SSE)"
  : > "$CIKTI/siparisler.ndjson"
  i=0
  while read -r satir; do
    i=$((i + 1))
    sid=$(echo "$satir" | jq -r .sid)
    pub=$(echo "$satir" | jq -r .publicId)
    # `< /dev/null` ŞART: psql `docker exec -i` ile koşuyor ve stdin'i
    # tüketiyor. Yönlendirme olmadan döngünün beslendiği akışı yutuyor ve
    # döngü ilk turdan sonra bitiyor — sonuç: 50 kullanıcı için 1 sipariş,
    # SSE senaryosunun %99'u 404. Bir kez yaşandı.
    oid=$(psql -c "
      SELECT o.public_id FROM orders o
      JOIN users u ON u.id = o.user_id
      WHERE u.public_id = '$pub' AND o.status NOT IN ('COMPLETED','REFUNDED')
      ORDER BY o.created_at DESC LIMIT 1;" < /dev/null | tr -d '[:space:]')
    if [ -z "$oid" ]; then
      q=$(curl -s "$API/catalog/quote?serviceCode=tg&countryIso=RU" -H "Cookie: sid=$sid")
      qid=$(echo "$q" | jq -r '.quoteId // empty')
      [ -n "$qid" ] || hata "teklif alınamadı (kullanıcı $i): $q"
      o=$(curl -s -X POST "$API/orders" -H "Cookie: sid=$sid" -H 'Content-Type: application/json' \
        -d "{\"quoteId\":\"$qid\"}")
      oid=$(echo "$o" | jq -r '.id // empty')
      [ -n "$oid" ] || hata "sipariş açılamadı (kullanıcı $i): $o"
    fi
    jq -nc --arg id "$oid" '{orderId:$id}' >> "$CIKTI/siparisler.ndjson"
    printf '\r  sipariş: %d/%d' "$i" "$toplam"
  done < <(jq -c '.[]' "$CIKTI/kullanicilar.json")
  printf '\n'
  jq -s '.' "$CIKTI/siparisler.ndjson" > "$CIKTI/siparisler.json"

  # Sayılar EŞİT olmalı: senaryo VU'yu kullanıcı ve siparişe aynı indeksle
  # eşler. Liste kısa kalırsa VU'lar başkasının siparişine bağlanır ve
  # sahiplik kontrolü doğru şekilde 404 döner — koşum çöp olur ama sebebi
  # ancak burada anlaşılır.
  local sayi
  sayi=$(jq 'length' "$CIKTI/siparisler.json")
  [ "$sayi" -eq "$toplam" ] || hata "sipariş sayısı ($sayi) kullanıcı sayısına ($toplam) eşit değil"
}

if [ "${1:-}" = "siparis" ]; then
  siparis_ac
  exit 0
fi

# ── Şablon kullanıcı: parola hash'ini uygulamanın kendisi üretsin ──
KOSUM="${YUK_KOSUM:-$(date +%s)}"
SABLON="yuk-sablon-$KOSUM@yuk.test"

log "şablon kullanıcı kaydediliyor (parola hash'i uygulamadan gelsin diye)"
KAYIT=$(curl -s -X POST "$API/auth/register" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$SABLON\",\"username\":\"yuks$KOSUM\",\"password\":\"$PAROLA\",\"acceptTerms\":true}")
echo "$KAYIT" | grep -q "Kaydınız alındı" || hata "şablon kaydı olmadı: $KAYIT"

HASH_VAR=$(psql -c "SELECT count(*) FROM users WHERE email='$SABLON';")
[ "$HASH_VAR" = "1" ] || hata "şablon kullanıcı bulunamadı"

# ── N kullanıcı: aynı hash, benzersiz e-posta ──
log "$N test kullanıcısı oluşturuluyor (yuk-$KOSUM-*@yuk.test)"
psql -c "
  INSERT INTO users (email, username, password_hash, status, email_verified_at)
  SELECT format('yuk-%s-%s@yuk.test', '$KOSUM', g),
         format('yuk_%s_%s', '$KOSUM', g),
         t.password_hash, 'ACTIVE', now()
  FROM generate_series(1, $N) g,
       (SELECT password_hash FROM users WHERE email = '$SABLON') t;
  UPDATE users SET email_verified_at = now(), status = 'ACTIVE'
  WHERE email = '$SABLON';" >/dev/null || hata "kullanıcılar oluşturulamadı"

# mapfile YOK: macOS'ta /bin/bash 3.2'dir ve mapfile 4.0 ile geldi.
# Betiğin geliştirici makinesinde çalışmaması, testin hiç koşulmaması demek.
SATIRLAR=()
_n=0
while IFS= read -r satir; do
  [ -n "$satir" ] || continue
  SATIRLAR[$_n]="$satir"; _n=$((_n + 1))
done < <(psql -c "
  SELECT email || '|' || public_id FROM users
  WHERE email LIKE 'yuk-$KOSUM-%@yuk.test' ORDER BY id;")
[ "$_n" -eq "$N" ] || hata "beklenen $N kullanıcı, bulunan $_n"

# ── Yönetici oturumu ──
YONETICI_EPOSTA="${YONETICI_EPOSTA:-test@ornek.com}"
YONETICI_PAROLA="${YONETICI_PAROLA:?YONETICI_PAROLA gerekli}"
ADMIN_SID=$(curl -s -i -X POST "$API/auth/login" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$YONETICI_EPOSTA\",\"password\":\"$YONETICI_PAROLA\"}" \
  | sed -n 's/^[Ss]et-[Cc]ookie: sid=\([^;]*\).*/\1/p')
[ -n "$ADMIN_SID" ] || hata "yönetici girişi başarısız"

# ── Bakiye: yönetim ucundan, defter üzerinden ──
log "bakiye yükleniyor ($((BAKIYE_KURUS / 100)) TL × $N) — yönetim ucu 60 istek/dk"
i=0
for s in "${SATIRLAR[@]}"; do
  i=$((i + 1))
  PUB="${s#*|}"
  for deneme in 1 2 3; do
    Y=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$API/admin/users/$PUB/balance" \
      -H "Cookie: sid=$ADMIN_SID" -H 'Content-Type: application/json' \
      -d "{\"amountMinor\":$BAKIYE_KURUS,\"note\":\"yuk testi tohumu $KOSUM\",\"idempotencyKey\":\"yuk-$KOSUM-$i\"}")
    [ "$Y" = "200" ] && break
    # 429: limit penceresinin kapanmasını bekle. Anahtar aynı olduğu için
    # tekrar denemek çift kayıt üretmez (idempotency_key).
    [ "$Y" = "429" ] && { sleep 20; continue; }
    hata "bakiye yüklenemedi (kullanıcı $i): HTTP $Y"
  done
  [ "$Y" = "200" ] || hata "bakiye yüklenemedi (kullanıcı $i): HTTP $Y"
  sleep "$ADMIN_BEKLEME"
  printf '\r  bakiye: %d/%d' "$i" "$N"
done
printf '\n'

# ── Oturumlar ──
log "oturumlar açılıyor — /auth 30 istek/dk/IP olduğu için ${AUTH_BEKLEME}s aralıkla"
: > "$CIKTI/kullanicilar.ndjson"
i=0
for s in "${SATIRLAR[@]}"; do
  i=$((i + 1))
  EPOSTA="${s%%|*}"; PUB="${s#*|}"
  SID=""
  for deneme in 1 2 3; do
    SID=$(giris "$EPOSTA")
    [ -n "$SID" ] && break
    sleep 20
  done
  [ -n "$SID" ] || hata "giriş yapılamadı: $EPOSTA"
  jq -nc --arg e "$EPOSTA" --arg p "$PAROLA" --arg id "$PUB" --arg sid "$SID" \
    '{email:$e, sifre:$p, publicId:$id, sid:$sid}' >> "$CIKTI/kullanicilar.ndjson"
  sleep "$AUTH_BEKLEME"
  printf '\r  oturum: %d/%d' "$i" "$N"
done
printf '\n'
jq -s '.' "$CIKTI/kullanicilar.ndjson" > "$CIKTI/kullanicilar.json"

# ── SSE için açık sipariş ──
siparis_ac

echo "$KOSUM" > "$CIKTI/kosum-kimligi.txt"
log "tohum hazır: $N kullanıcı, $N açık sipariş  (koşum kimliği: $KOSUM)"
