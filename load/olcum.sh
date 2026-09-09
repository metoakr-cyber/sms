#!/usr/bin/env bash
# Koşum boyunca sunucu ve altyapı kaynaklarını örnekler.
#
# k6 yalnız İSTEMCİ tarafını görür: gecikme ve hata oranı. Darboğazın nerede
# olduğunu söylemez. "p95 tuttu" ile "p95 tuttu çünkü havuz doymadı" arasındaki
# fark bu dosyadır.
#
# Kullanım:  load/olcum.sh <cikti.csv> <etiket>
# Durdurma:  sinyal (TERM) — çağıran betik öldürür.
set -uo pipefail

CIKTI="${1:?kullanım: olcum.sh <cikti.csv> <etiket>}"
ETIKET="${2:-koşum}"
ARALIK="${OLCUM_ARALIK:-2}"
PORT="${YUK_PORT:-8191}"
PG_KAP="${PG_KAP:-smsplatform-dev-postgres-1}"
REDIS_KAP="${REDIS_KAP:-smsplatform-dev-redis-1}"
DBADI="${DBADI:-smsplatform}"

if [ ! -s "$CIKTI" ]; then
  echo "zaman,etiket,goroutine,rss_mb,acik_fd,pg_toplam,pg_aktif,pg_idle_tx,redis_istemci,redis_kanal" > "$CIKTI"
fi

metrik() { # metrik <ad>  → değer ya da boş
  printf '%s' "$1" | grep -q . || return 0
  echo "$METRIKLER" | awk -v a="$1" '$1==a {print $2; exit}'
}

while :; do
  T=$(date +%s)

  METRIKLER=$(curl -s -m 2 "http://localhost:$PORT/metrics" 2>/dev/null)
  GOR=$(metrik go_goroutines)
  RSS=$(metrik process_resident_memory_bytes)
  FD=$(metrik process_open_fds)
  RSS_MB=$(awk -v b="${RSS:-0}" 'BEGIN{printf "%.1f", b/1048576}')

  PG=$(docker exec "$PG_KAP" psql -U smsplatform -d postgres -qtA -c "
    SELECT count(*) FILTER (WHERE datname='$DBADI'),
           count(*) FILTER (WHERE datname='$DBADI' AND state='active'),
           count(*) FILTER (WHERE datname='$DBADI' AND state='idle in transaction')
    FROM pg_stat_activity;" 2>/dev/null | tr -d ' ')
  PG_TOPLAM=$(echo "$PG" | cut -d'|' -f1)
  PG_AKTIF=$(echo "$PG" | cut -d'|' -f2)
  PG_IDLETX=$(echo "$PG" | cut -d'|' -f3)

  R_ISTEMCI=$(docker exec "$REDIS_KAP" redis-cli info clients 2>/dev/null \
    | awk -F: '/^connected_clients/{gsub(/\r/,"");print $2}')
  R_KANAL=$(docker exec "$REDIS_KAP" redis-cli pubsub channels 2>/dev/null | grep -c . )

  printf '%s,%s,%s,%s,%s,%s,%s,%s,%s,%s\n' \
    "$T" "$ETIKET" "${GOR:-}" "$RSS_MB" "${FD:-}" \
    "${PG_TOPLAM:-}" "${PG_AKTIF:-}" "${PG_IDLETX:-}" \
    "${R_ISTEMCI:-}" "${R_KANAL:-0}" >> "$CIKTI"

  sleep "$ARALIK"
done
