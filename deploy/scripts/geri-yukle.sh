#!/usr/bin/env bash
# Onay360 — şifreli yedekten geri yükleme.
#
# Kullanım:
#     ./deploy/scripts/geri-yukle.sh <yedek-dosyasi> [--hedef-db AD] [--uretim-onayi]
#
# Varsayılan hedef `smsplatform_tatbikat` veritabanıdır — yani betiği hiçbir
# bayrak vermeden çalıştırmak CANLI VERİYE DOKUNMAZ. Tatbikat (NFR-808'in
# "geri yükleme test edilir" şartı) tam olarak budur.
#
# Canlı veritabanının üzerine yazmak için İKİ şey birden gerekir:
#   1. --uretim-onayi bayrağı
#   2. veritabanı adının elle, harfi harfine yazılması (etkileşimli onay)
# Sebep: bu adım tüm defteri (ledger) ezer ve geri alınamaz. Yanlışlıkla
# çalıştırılabilen bir komut, er ya da geç yanlışlıkla çalıştırılır.
#
# Geri yükleme sonrası MUTLAKA mutabakat çalıştırın:
#     docker compose -f deploy/docker-compose.prod.yml \
#       run --rm --entrypoint /app/cli api wallet:reconcile
# Yedeğin "açılıyor olması" yetmez; içindeki paranın tutarlı olması gerekir.
set -uo pipefail

KOK="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
COMPOSE_DOSYA="${COMPOSE_DOSYA:-$KOK/deploy/docker-compose.prod.yml}"
ENV_DOSYA="${ENV_DOSYA:-$KOK/deploy/.env}"

if [ -f "$ENV_DOSYA" ]; then
  while IFS= read -r satir; do
    case "$satir" in ''|\#*) continue ;; esac
    ad="${satir%%=*}"
    case "$ad" in *[!A-Za-z0-9_]*|'') continue ;; esac
    if [ -z "${!ad:-}" ]; then export "${ad}=${satir#*=}"; fi
  done < "$ENV_DOSYA"
fi

# Test dikişi: testler gerçek bir veritabanı olmadan kapıları sınayabilsin.
PG_RESTORE_CMD="${PG_RESTORE_CMD:-}"

ok()   { printf '  \033[32m✓\033[0m %s\n' "$*"; }
hata() { printf '  \033[31m✗\033[0m %s\n' "$*" >&2; }

HEDEF_DB="${HEDEF_DB:-smsplatform_tatbikat}"
URETIM_ONAYI=0
DOSYA=""

while [ $# -gt 0 ]; do
  case "$1" in
    --hedef-db)     HEDEF_DB="${2:?--hedef-db bir ad ister}"; shift 2 ;;
    --uretim-onayi) URETIM_ONAYI=1; shift ;;
    -h|--yardim)
      sed -n '2,30p' "$0" | sed 's/^# \{0,1\}//'
      exit 0 ;;
    -*) hata "bilinmeyen seçenek: $1"; exit 1 ;;
    *)  DOSYA="$1"; shift ;;
  esac
done

if [ -z "$DOSYA" ]; then
  hata "kullanım: $0 <yedek-dosyasi> [--hedef-db AD] [--uretim-onayi]"
  exit 1
fi
if [ ! -f "$DOSYA" ]; then
  hata "dosya yok: $DOSYA"
  exit 1
fi

echo "── Onay360 geri yükleme ──"
echo "   kaynak : $DOSYA"
echo "   hedef  : $HEDEF_DB"

# ─────────────────────── 1) Bütünlük ───────────────────────
#
# Bozuk bir yedeği geri yüklemek, yedeğin olduğunu sanmaktan daha kötüdür:
# yarım bir defterin üstüne yazılır ve fark edilmesi günler alır.
if [ -f "$DOSYA.sha256" ]; then
  DIZIN="$(cd "$(dirname "$DOSYA")" && pwd)"
  if command -v sha256sum >/dev/null 2>&1; then
    (cd "$DIZIN" && sha256sum -c "$(basename "$DOSYA").sha256" >/dev/null 2>&1)
  else
    (cd "$DIZIN" && shasum -a 256 -c "$(basename "$DOSYA").sha256" >/dev/null 2>&1)
  fi
  if [ $? -ne 0 ]; then
    hata "sha256 EŞLEŞMEDİ — dosya bozulmuş, geri yükleme yapılmıyor"
    exit 1
  fi
  ok "sha256 doğrulandı"
else
  hata "sha256 yan dosyası yok: $DOSYA.sha256"
  hata "doğrulanamayan bir yedek geri yüklenmez"
  exit 1
fi

# ─────────────────────── 2) Hedef kapısı ───────────────────────
if [ "$HEDEF_DB" = "${POSTGRES_DB:-}" ]; then
  if [ "$URETIM_ONAYI" -ne 1 ]; then
    hata "hedef CANLI veritabanı ($HEDEF_DB) ama --uretim-onayi verilmedi"
    hata "tatbikat için hedefi boş bırakın (varsayılan: smsplatform_tatbikat)"
    exit 1
  fi
  echo
  echo "  🔴 CANLI VERİTABANININ ÜZERİNE YAZILACAK: $HEDEF_DB"
  echo "     Mevcut tüm kayıtlar (ledger dâhil) SİLİNİP yedekle değiştirilecek."
  echo "     Onaylamak için veritabanı adını harfi harfine yazın:"
  printf '     > '
  read -r yazilan
  if [ "$yazilan" != "$HEDEF_DB" ]; then
    hata "yazılan ad eşleşmedi — hiçbir şey yapılmadı"
    exit 1
  fi
  ok "üretim onayı alındı"
else
  ok "hedef canlı veritabanı değil"
fi

# ─────────────────────── 3) Çözme + geri yükleme ───────────────────────
COZ=""
case "$DOSYA" in
  *.age)
    command -v age >/dev/null 2>&1 || { hata "age kurulu değil"; exit 1; }
    : "${BACKUP_AGE_KEY_FILE:?BACKUP_AGE_KEY_FILE tanımsız — özel anahtar dosyası gerekli}"
    COZ="age --decrypt -i $BACKUP_AGE_KEY_FILE" ;;
  *.gpg)
    command -v gpg >/dev/null 2>&1 || { hata "gpg kurulu değil"; exit 1; }
    COZ="gpg --batch --quiet --decrypt" ;;
  *.dump)
    COZ="cat" ;;
  *)
    hata "tanınmayan uzantı: $DOSYA (.age | .gpg | .dump bekleniyor)"; exit 1 ;;
esac

if [ -n "$PG_RESTORE_CMD" ]; then
  # shellcheck disable=SC2086
  $COZ "$DOSYA" | $PG_RESTORE_CMD --clean --if-exists --no-owner -d "$HEDEF_DB"
  KOD=$?
else
  # Hedef veritabanı yoksa oluşturulur (tatbikat yolu).
  docker compose -f "$COMPOSE_DOSYA" exec -T postgres \
    psql -U "${POSTGRES_USER:?}" -d postgres -tAc \
    "SELECT 1 FROM pg_database WHERE datname='$HEDEF_DB'" | grep -q 1 \
    || docker compose -f "$COMPOSE_DOSYA" exec -T postgres \
         createdb -U "${POSTGRES_USER:?}" "$HEDEF_DB"

  # shellcheck disable=SC2086
  $COZ "$DOSYA" | docker compose -f "$COMPOSE_DOSYA" exec -T postgres \
    pg_restore --clean --if-exists --no-owner -U "${POSTGRES_USER:?}" -d "$HEDEF_DB"
  KOD=$?
fi

if [ "$KOD" -ne 0 ]; then
  hata "pg_restore başarısız (çıkış kodu $KOD)"
  exit 1
fi
ok "geri yüklendi"

# ─────────────────────── 4) Sonrası ───────────────────────
if [ -z "$PG_RESTORE_CMD" ]; then
  SAYIM=$(docker compose -f "$COMPOSE_DOSYA" exec -T postgres \
    psql -U "${POSTGRES_USER:?}" -d "$HEDEF_DB" -tAc \
    'SELECT count(*) FROM ledger_entries' 2>/dev/null | tr -d '[:space:]')
  if [ -z "$SAYIM" ]; then
    hata "ledger_entries okunamadı — geri yükleme eksik olabilir"
    exit 1
  fi
  ok "ledger_entries: $SAYIM kayıt"
fi

echo
echo "── geri yükleme tamam · $HEDEF_DB ──"
echo "   SIRADAKİ ADIM (atlamayın): defter mutabakatı"
echo "     docker compose -f deploy/docker-compose.prod.yml \\"
echo "       run --rm --entrypoint /app/cli \\"
echo "       -e DATABASE_URL=\"postgres://\$POSTGRES_USER:\$POSTGRES_PASSWORD@postgres:5432/$HEDEF_DB?sslmode=disable\" \\"
echo "       api wallet:reconcile"
echo "   Σ ledger == bakiye çıkmazsa bu yedek KULLANILAMAZ."
