#!/usr/bin/env bash
# Yük testi koşturucusu (k6) — NFR-800 hedeflerine karşı ölçüm.
#
# Kullanım:
#   ./scripts/yuk-testi.sh              # tam koşum: tohum + 5 senaryo + mutabakat
#   ./scripts/yuk-testi.sh tohum        # yalnız tohum verisi
#   ./scripts/yuk-testi.sh kos          # 5 senaryo + mutabakat (mevcut tohumla)
#   ./scripts/yuk-testi.sh mutabakat    # yalnız defter mutabakatı kapısı
#   ./scripts/yuk-testi.sh teklif       # tek senaryo (tohum hazırsa)
#
# NEDEN AYRI BİR SUNUCU ÖRNEĞİ:
# Betik kendi API sürecini YUK_PORT üzerinde başlatır (varsayılan 8191), 8091'i
# kullanmaz. Aynı makinede `make dev` çalışıyorken ölçüm yapmak, iki tarafın da
# birbirinin gecikmesini ölçmesi demektir; ayrıca FAKE sağlayıcının stok ve
# bakiyesi SÜREÇ İÇİDİR ve paylaşılan bir süreçte ölçüm tekrarlanabilir olmaz.
#
# Bu betik VERİ SİLMEZ: ne DELETE, ne TRUNCATE, ne DROP çalıştırır.
# test: scripts/yuk-testi_test.sh#silme_ifadesi_yok
set -uo pipefail
cd "$(dirname "$0")/.."
KOK="$(pwd)"

# MEVCUT ORTAM KAZANIR — .env yalnız boşlukları doldurur (smoke-auth.sh ile
# aynı kural; iki betikte iki farklı öncelik olamaz).
_pre_db="${DATABASE_URL:-}"; _pre_env="${APP_ENV:-}"
[ -f .env ] && { set -a; . ./.env; set +a; }
[ -n "$_pre_db" ] && DATABASE_URL="$_pre_db"
[ -n "$_pre_env" ] && APP_ENV="$_pre_env"
export DATABASE_URL APP_ENV
export PATH="$PATH:$(go env GOPATH 2>/dev/null)/bin"

ADIM="${1:-hepsi}"
PORT="${YUK_PORT:-8191}"
API="http://localhost:$PORT/api/v1"
CIKTI="${YUK_CIKTI:-$KOK/load/sonuclar}"
PG_KAP="${PG_KAP:-smsplatform-dev-postgres-1}"
REDIS_KAP="${REDIS_KAP:-smsplatform-dev-redis-1}"
SENARYOLAR="${SENARYOLAR:-katalog panel teklif satinalma sse}"
export YUK_PORT PG_KAP REDIS_KAP

# Mutabakat ve k6 komutları DIŞARIDAN değiştirilebilir. İki sebep var:
# derlenmiş bir ikiliyle koşmak (go run her seferinde derliyor) ve kapıların
# kendisini test edebilmek (scripts/yuk-testi_test.sh sahte bir komut verir).
YUK_RECONCILE_CMD="${YUK_RECONCILE_CMD:-go run ./cmd/cli wallet:reconcile}"
YUK_K6_CMD="${YUK_K6_CMD:-k6}"

# ÖLÇÜM SUNUCUSU AYRI BİR REDIS VERİTABANI KULLANIR (varsayılan 3).
#
# scripts/smoke-auth.sh paylaşılan geliştirme Redis'inde `FLUSHDB` çağırır
# (satır 143 ve 239) ve bunu DATABASE_URL nereyi gösterirse göstersin yapar.
# Aynı makinede `make check` koşan başka biri, yük testinin 50 oturumunu
# koşumun ortasında siler; ölçüm baştan sona 401 döner ve "hata oranı %100"
# gibi görünen sonuç aslında ölçüm değildir. Bu tam olarak bir kez yaşandı.
YUK_REDIS_DB="${YUK_REDIS_DB:-3}"
YUK_REDIS_URL="$(printf '%s' "${REDIS_URL:-redis://localhost:56379/0}" | sed "s|/[0-9]*$|/$YUK_REDIS_DB|")"

renk_ok()  { printf '\033[32m%s\033[0m\n' "$*"; }
renk_err() { printf '\033[31m%s\033[0m\n' "$*"; }
baslik()   { printf '\n\033[1m─── %s ───\033[0m\n' "$*"; }
log()      { printf '  %s\n' "$*"; }
dur()      { renk_err "✗ $*"; exit 1; }

# ═══════════════════════ ORTAM KAPISI ═══════════════════════
#
# Yük testi bir yönetici hesabıyla bakiye basar ve yüzlerce sipariş açar.
# Yanlış ortamda çalıştırılması gerçek para hareketi demektir. İki bağımsız
# kontrol var ve ikisi de geçmeden hiçbir istek gönderilmez.
# test: scripts/yuk-testi_test.sh#dev_disi_ortam_reddedilir
kapi() {
  [ "${APP_ENV:-}" = "development" ] || dur \
    "yük testi YALNIZ development'ta koşar (APP_ENV=${APP_ENV:-tanımsız})"

  DBADI="${DATABASE_URL##*/}"; DBADI="${DBADI%%\?*}"
  case "$DBADI" in
    smsplatform|smsplatform_test) : ;;
    *) dur "beklenmeyen veritabanı: '${DBADI:-okunamadı}'" ;;
  esac
  export DBADI

  docker ps --format '{{.Names}}' 2>/dev/null | grep -q "$PG_KAP" \
    || dur "postgres konteyneri yok: $PG_KAP"
}

# ═══════════════════════ SUNUCU ═══════════════════════
SUNUCU_PID=""
# ISLER: sunucu süreci arka plan işlerini de koşsun mu (WORKERS_IN_PROCESS).
# Varsayılan "true" gerçekçi olandır; SSE senaryosu bunu kapatır (aşağıda).
sunucu_baslat() { # sunucu_baslat [true|false]
  local isler="${1:-true}"
  if curl -fs -m 2 "http://localhost:$PORT/healthz" >/dev/null 2>&1; then
    log "sunucu zaten ayakta: :$PORT (yeniden kullanılıyor)"
    return
  fi
  log "API derleniyor…"
  (cd api && go build -o /tmp/yuk-api ./cmd/server) || dur "API derlenemedi"
  log "API başlatılıyor: :$PORT  (redis db $YUK_REDIS_DB · arka plan işleri: $isler)"
  HTTP_ADDR=":$PORT" LOG_LEVEL=warn GIN_MODE=release REDIS_URL="$YUK_REDIS_URL" \
    WORKERS_IN_PROCESS="$isler" \
    nohup /tmp/yuk-api >> "$CIKTI/api.log" 2>&1 &
  SUNUCU_PID=$!
  for _ in $(seq 1 40); do
    curl -fs -m 2 "http://localhost:$PORT/healthz" >/dev/null 2>&1 && break
    sleep 0.5
  done
  curl -fs -m 2 "http://localhost:$PORT/readyz" >/dev/null 2>&1 \
    || dur "sunucu hazır olmadı — $CIKTI/api.log"
}

sunucu_durdur() {
  [ -n "$SUNUCU_PID" ] && kill "$SUNUCU_PID" 2>/dev/null
  SUNUCU_PID=""
  # Kendi başlattığımız ikili dışında hiçbir şeye dokunmayız.
  pkill -f '/tmp/yuk-api' 2>/dev/null
  for _ in $(seq 1 20); do
    curl -fs -m 1 "http://localhost:$PORT/healthz" >/dev/null 2>&1 || return 0
    sleep 0.5
  done
}

sunucu_yeniden() { # sunucu_yeniden [true|false]
  sunucu_durdur
  sunucu_baslat "${1:-true}"
}

# ═══════════════════════ MUTABAKAT KAPISI ═══════════════════════
#
# YÜK ALTINDA BOZULAN BİR DEFTER, YAVAŞ BİR SİSTEMDEN CİDDİDİR.
# Bu yüzden mutabakat bir "bilgi satırı" değil, koşumun geçme koşuludur:
# sapma varsa betik sıfırdan farklı kod döner.
# test: scripts/yuk-testi_test.sh#mutabakat_sapmasi_kapiyi_dusurur
mutabakat() {
  local etiket="$1" cikti rc
  cikti=$(cd api && eval "$YUK_RECONCILE_CMD" 2>&1)
  rc=$?
  printf '%s\n' "$cikti" | sed 's/^/    /'
  if [ $rc -ne 0 ]; then
    renk_err "  ✗ MUTABAKAT SAPMASI ($etiket) — Σ defter ≠ bakiye"
    return 1
  fi
  renk_ok "  ✓ mutabakat tamam ($etiket)"
  return 0
}

# ═══════════════════════ ÖRNEKLEYİCİ ═══════════════════════
OLCUM_PID=""
olcum_basla() {
  load/olcum.sh "$CIKTI/kaynak.csv" "$1" >/dev/null 2>&1 &
  OLCUM_PID=$!
}
olcum_dur() {
  [ -n "$OLCUM_PID" ] && kill "$OLCUM_PID" 2>/dev/null
  OLCUM_PID=""
}

ozet_kaynak() { # ozet_kaynak <etiket>
  awk -F, -v e="$1" '
    $2==e {
      if ($3+0 > gor) gor = $3+0
      if ($4+0 > rss) rss = $4+0
      if ($6+0 > pg)  pg  = $6+0
      if ($7+0 > pga) pga = $7+0
      if ($9+0 > rc)  rc  = $9+0
      if ($10+0 > rk) rk  = $10+0
      n++
    }
    END {
      if (n == 0) { print "    (örnek yok)"; exit }
      printf "    tepe: goroutine=%d  rss=%.1f MB  pg_bağlantı=%d (aktif %d)  redis_istemci=%d  pubsub_kanal=%d\n",
             gor, rss, pg, pga, rc, rk
    }' "$CIKTI/kaynak.csv"
}

# ═══════════════════════ SIRA İLERLETME (satın alma ön koşulu) ═══════════════════════
#
# 🔴 GEÇİCİ ÇÖZÜM — ASIL KUSUR KAYNAKTA (docs/yuk-testi-sonuc.md §6.2).
#
# FAKE sağlayıcı uzak sipariş kimliğini SÜREÇ İÇİ bir sayaçtan üretir
# (fake.go: `id := fmt.Sprintf("fake-%d", p.seq)`, seq her açılışta 0'dan
# başlar). Kalıcı geliştirme veritabanında önceki koşumların satırları durur;
# yeni süreç aynı kimlikleri yeniden üretir ve `orders_remote_uniq`
# (provider_id, remote_order_id) çakışır:
#
#   duplicate key value violates unique constraint "orders_remote_uniq" (23505)
#
# Sonuç: ikinci koşumda POST /orders sıfırıncı istekten itibaren 500 döner ve
# satın alma senaryosu ÖLÇÜLEMEZ. Bu adım, sayacı veritabanındaki en yüksek
# değerin üstüne çıkarana kadar satın alma dener; ilk 200 geldiğinde durur.
#
# BEDELİ VARDIR ve rapora yazılır: her ilerletme denemesi sağlayıcıdan gerçek
# bir numara alır (FAKE bakiyesinden düşer) ve sipariş yazılamadığı için
# numara açıkta kalır — `Cancel` ilk 120 saniye boyunca hata döner
# (minActivationTime). Yani ölçüm bütçesi, ilerletme kadar küçülür.
#
# Kalıcı çözüm bir satırlık: FAKE sağlayıcı kimliğe süreç başına benzersiz bir
# ek koysun (örn. `fake-<nonce>-<seq>`). O dosya bu görevin kapsamı dışında.
sira_ilerlet() {
  local enbuyuk deneme sid pub q qid kod i toplam
  [ -s "$CIKTI/kullanicilar.json" ] || dur "tohum yok — önce ./scripts/yuk-testi.sh tohum"

  enbuyuk=$(docker exec "$PG_KAP" psql -U smsplatform -d "$DBADI" -qtA -c "
    SELECT COALESCE(MAX(CAST(split_part(o.remote_order_id, '-', 2) AS BIGINT)), 0)
    FROM orders o JOIN providers p ON p.id = o.provider_id
    WHERE p.protocol = 'FAKE' AND o.remote_order_id ~ '^fake-[0-9]+\$';" < /dev/null | tr -d '[:space:]')
  enbuyuk="${enbuyuk:-0}"

  if [ "$enbuyuk" = "0" ]; then
    log "sıra ilerletme gerekmiyor (veritabanında fake-N siparişi yok)"
    return 0
  fi

  toplam=$(jq 'length' "$CIKTI/kullanicilar.json")
  log "FAKE sıra çakışması: veritabanındaki en büyük kimlik fake-$enbuyuk"
  log "sayaç bu değerin üstüne çıkarılıyor (en çok $((enbuyuk + 20)) deneme)…"

  i=0
  deneme=0
  while [ "$deneme" -lt $((enbuyuk + 20)) ]; do
    deneme=$((deneme + 1))
    # Kullanıcılar sırayla dolaşılır: sipariş ucu kullanıcı başına 20/dk ile
    # sınırlı (NFR-802); tek kullanıcıyla ilerletmek limite çarpar ve
    # ilerletme hiç bitmez.
    i=$(( (i % toplam) + 1 ))
    sid=$(jq -r ".[$((i - 1))].sid" "$CIKTI/kullanicilar.json")
    # EN UCUZ kombinasyon (tg/UA, 88.000 mikro-USD): ilerletmenin sağlayıcı
    # bakiyesinden götürdüğü pay en aza insin.
    q=$(curl -s -m 5 "$API/catalog/quote?serviceCode=tg&countryIso=UA" -H "Cookie: sid=$sid")
    qid=$(printf '%s' "$q" | jq -r '.quoteId // empty')
    [ -n "$qid" ] || { sleep 1; continue; }
    kod=$(curl -s -m 10 -o /dev/null -w '%{http_code}' -X POST "$API/orders" \
      -H "Cookie: sid=$sid" -H 'Content-Type: application/json' -d "{\"quoteId\":\"$qid\"}")
    case "$kod" in
      200) log "sıra ilerledi: $deneme deneme sonrası ilk başarılı satın alma"; return 0 ;;
      429) sleep 3 ;;
      503) renk_err "  ✗ sıra ilerletilemedi: sağlayıcı 503 (bakiye/stok tükendi)"; return 1 ;;
    esac
    [ $((deneme % 100)) -eq 0 ] && printf '\r  ilerletme: %d/%d' "$deneme" "$enbuyuk"
  done
  printf '\n'
  renk_err "  ✗ sıra ilerletilemedi ($deneme deneme)"
  return 1
}

# ═══════════════════════ SENARYO KOŞUMU ═══════════════════════
# SSE senaryosunun ön koşulu: TERMİNAL OLMAYAN sipariş.
#
# FAKE sağlayıcı kodu ANINDA verir (SMSDelay = 0) ve sunucu süreci
# `order-poller` işini 30 saniyede bir koşar; sipariş yarım dakika içinde
# COMPLETED olur, akış da terminal durumda kapanır (handler/order.go
# isTerminal). Gerçek sağlayıcıda kod dakikalar sürer, akış açık kalır.
# Ölçmek istediğimiz şey akışın kapasitesi olduğu için bu senaryoda ölçüm
# sunucusu arka plan işleri KAPALI başlatılır ve siparişler tazelenir.
# Bu bir hile değil, ölçüm koşulunun kurulmasıdır ve raporda yazar
# (docs/yuk-testi-sonuc.md §6.3).
sse_hazirlik() {
  log "SSE için sunucu arka plan işleri KAPALI yeniden başlatılıyor"
  sunucu_yeniden false
  YUK_CIKTI="$CIKTI" load/tohum.sh siparis || dur "SSE siparişleri açılamadı"
}

k6_kos() { # k6_kos <senaryo>
  local s="$1" rc
  [ "$s" = "sse" ] && sse_hazirlik
  if [ "$s" = "satinalma" ]; then
    baslik "satın alma ön koşulu: FAKE sıra ilerletme"
    sira_ilerlet || { renk_err "  ✗ satinalma ölçülemedi"; return 1; }
  fi
  baslik "senaryo: $s"
  olcum_basla "$s"
  # Ham CSV yalnız istek/yanıt senaryolarında tutulur: SSE koşumunda her
  # örnek uzun ömürlü bir bağlantıya ait ve dosya yüz megabaytı aşıyor.
  local ham="--out csv=sonuclar/ham-$s.csv"
  [ "$s" = "sse" ] && ham=""
  # shellcheck disable=SC2086
  (cd load && SENARYO="$s" API_TABAN="$API" "$YUK_K6_CMD" run \
      --summary-export "sonuclar/ozet-$s.json" $ham \
      senaryo.js 2>&1 | tee "$CIKTI/k6-$s.log")
  rc=${PIPESTATUS[0]}
  olcum_dur
  ozet_kaynak "$s"
  if [ $rc -ne 0 ]; then
    renk_err "  ✗ $s: k6 eşikleri TUTMADI (çıkış $rc)"
    return 1
  fi
  renk_ok "  ✓ $s: eşikler tuttu"
  return 0
}

# ═══════════════════════ OTURUM TAZELİĞİ ═══════════════════════
#
# Oturumlar Redis'te durur; geliştirme Redis'i sıfırlandığında tohum
# kullanıcıları yerinde kalır ama çerezleri ölür. Bunu koşumdan ÖNCE fark
# etmezsek, iki buçuk dakikalık bir senaryo baştan sona 401 döner ve
# "hata oranı %100" gibi görünen sonuç aslında ölçüm değil, çöptür.
oturum_tazele() {
  local sid kod
  [ -s "$CIKTI/kullanicilar.json" ] || dur "tohum yok — önce ./scripts/yuk-testi.sh tohum"
  sid=$(jq -r '.[0].sid // empty' "$CIKTI/kullanicilar.json")
  kod=$(curl -s -o /dev/null -w '%{http_code}' "$API/me" -H "Cookie: sid=$sid")
  if [ "$kod" = "200" ]; then
    log "tohum oturumları geçerli"
    return
  fi
  log "tohum oturumları geçersiz (HTTP $kod) — yeniden giriş yapılıyor"
  YUK_CIKTI="$CIKTI" load/tohum.sh yenile || dur "oturumlar yenilenemedi"
}

isinma() {
  baslik "ısınma (ölçüme girmez)"
  # İlk istekler TCP + TLS + havuz kurulumu içerir ve p99'u yanıltır.
  local i=0
  while [ $i -lt 200 ]; do
    curl -s -o /dev/null "$API/catalog/services-in-stock"
    curl -s -o /dev/null "$API/catalog/countries"
    i=$((i + 1))
  done
  log "200 ısınma isteği gönderildi"
}

# ═══════════════════════ AKIŞ ═══════════════════════
kapi
mkdir -p "$CIKTI"
trap 'olcum_dur; sunucu_durdur' EXIT

# Kaynak izi HER KOŞUMDA sıfırlanır. Dosya biriktiğinde tepe değerler önceki
# koşumlardan geliyordu ve rapor yanlış sayı gösteriyordu — bir kez yaşandı.
if [ "$ADIM" != "mutabakat" ] && [ "$ADIM" != "tohum" ]; then
  [ -f "$CIKTI/kaynak.csv" ] && mv "$CIKTI/kaynak.csv" "$CIKTI/kaynak-onceki.csv"
  : > "$CIKTI/api.log"
fi

case "$ADIM" in
  mutabakat)
    mutabakat "tekil" || exit 1
    exit 0
    ;;
esac

command -v "$YUK_K6_CMD" >/dev/null 2>&1 || [ "$ADIM" = "tohum" ] \
  || dur "k6 kurulu değil — 'brew install k6'"

baslik "ortam"
log "donanım : $(sysctl -n hw.model 2>/dev/null || uname -m) · $(sysctl -n hw.ncpu 2>/dev/null || nproc) çekirdek"
log "işletim : $(uname -sr)"
log "veritabanı: $DBADI (konteyner $PG_KAP)"
log "sunucu  : :$PORT"

sunucu_baslat

# Kur bayatsa teklif ucu satışı DURDURUR (FR-302) ve ölçtüğümüz şey hata
# yolu olur. Ölçümden önce kuru tazeleriz.
baslik "kur tazeleniyor"
(cd api && go run ./cmd/cli fx:sync 2>&1 | tail -1 | sed 's/^/  /') \
  || log "(kur tazelenemedi — teklif senaryosu hata dönebilir)"

baslik "mutabakat (başlangıç)"
mutabakat "başlangıç" || dur \
  "koşumdan ÖNCE sapma var — bu koşum yük altındaki sapmayı kanıtlayamaz, önce mevcut sapmayı inceleyin"

if [ "$ADIM" = "tohum" ] || [ "$ADIM" = "hepsi" ]; then
  baslik "tohum"
  YUK_CIKTI="$CIKTI" load/tohum.sh || dur "tohum başarısız"
  [ "$ADIM" = "tohum" ] && exit 0
fi

baslik "oturum tazeliği"
oturum_tazele

isinma

DUSEN=""
if [ "$ADIM" = "hepsi" ] || [ "$ADIM" = "kos" ]; then
  for s in $SENARYOLAR; do
    k6_kos "$s" || DUSEN="$DUSEN $s"
  done
else
  k6_kos "$ADIM" || DUSEN="$ADIM"
fi

baslik "mutabakat (koşum sonrası)"
MUTABAKAT_OK=1
mutabakat "koşum sonrası" || MUTABAKAT_OK=0

baslik "özet"
log "k6 özetleri : $CIKTI/ozet-*.json"
log "kaynak izi  : $CIKTI/kaynak.csv"
log "sunucu log  : $CIKTI/api.log"

if [ "$MUTABAKAT_OK" -eq 0 ]; then
  renk_err "YÜK ALTINDA DEFTER BOZULDU — bu, gecikme hedefinden ÖNCE gelir."
  exit 2
fi
if [ -n "$DUSEN" ]; then
  renk_err "NFR-800 hedefi tutmayan senaryolar:$DUSEN"
  exit 1
fi
renk_ok "TÜM SENARYOLAR HEDEFİ TUTTU · defter mutabık"
