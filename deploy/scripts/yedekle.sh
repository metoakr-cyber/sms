#!/usr/bin/env bash
# Onay360 — şifreli Postgres yedeği.
#
# Kullanım (depo kökünden):
#     ./deploy/scripts/yedekle.sh
#
# Cron (her gece 03:17):
#     17 3 * * * cd /opt/onay360 && ./deploy/scripts/yedekle.sh >> /var/log/onay360-yedek.log 2>&1
#
# NFR-808 sözleşmesi: günlük · ŞİFRELİ · AYRI KONUMDA · 30 gün saklama.
#
# Bu betik "yedek aldım" demeden önce yedeği DOĞRULAR:
#   1. pg_dump'ın kendi çıkış kodu (boru içinde kaybolmasın diye ayrıca)
#   2. asgari boyut
#   3. `pg_restore --list` ile içeriğin gerçekten okunabilir olması
# Üçü de geçmeden dosya şifrelenmez ve diskte bırakılmaz. Sebep: bozuk bir
# yedeği saklamak, yedeğin olmadığını fark etmemek demektir.
set -uo pipefail

KOK="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
COMPOSE_DOSYA="${COMPOSE_DOSYA:-$KOK/deploy/docker-compose.prod.yml}"
ENV_DOSYA="${ENV_DOSYA:-$KOK/deploy/.env}"

# .env yüklenir ama MEVCUT ORTAM KAZANIR: cron'dan gelen bir değişkeni
# dosyadaki eski değerin ezmesi, gece yarısı fark edilmeyen bir arızadır.
if [ -f "$ENV_DOSYA" ]; then
  while IFS= read -r satir; do
    case "$satir" in ''|\#*) continue ;; esac
    ad="${satir%%=*}"
    case "$ad" in *[!A-Za-z0-9_]*|'') continue ;; esac
    # Zaten tanımlıysa dokunma.
    if [ -z "${!ad:-}" ]; then export "${ad}=${satir#*=}"; fi
  done < "$ENV_DOSYA"
fi

BACKUP_DIR="${BACKUP_DIR:-}"
BACKUP_RETENTION_DAYS="${BACKUP_RETENTION_DAYS:-30}"
BACKUP_MIN_BYTES="${BACKUP_MIN_BYTES:-65536}"
BACKUP_REMOTE="${BACKUP_REMOTE:-}"
BACKUP_AGE_RECIPIENT="${BACKUP_AGE_RECIPIENT:-}"
BACKUP_GPG_RECIPIENT="${BACKUP_GPG_RECIPIENT:-}"
APP_ENV="${APP_ENV:-development}"

# Test dikişleri: testler gerçek bir veritabanı kurmadan kapıları
# sınayabilsin diye komutlar dışarıdan değiştirilebilir.
PG_DUMP_CMD="${PG_DUMP_CMD:-}"
PG_RESTORE_CMD="${PG_RESTORE_CMD:-pg_restore}"
UZAK_KOPYA_CMD="${UZAK_KOPYA_CMD:-}"

ok()   { printf '  \033[32m✓\033[0m %s\n' "$*"; }
hata() { printf '  \033[31m✗\033[0m %s\n' "$*" >&2; }

echo "── Onay360 yedekleme ──"

# ─────────────────────────── Kapılar ───────────────────────────

if [ -z "$BACKUP_DIR" ]; then
  hata "BACKUP_DIR tanımsız — nereye yazılacağı belli değil"
  exit 1
fi

if [ -z "$BACKUP_AGE_RECIPIENT" ] && [ -z "$BACKUP_GPG_RECIPIENT" ]; then
  hata "BACKUP_AGE_RECIPIENT veya BACKUP_GPG_RECIPIENT gerekli"
  hata "şifresiz yedek, veritabanının açık bir kopyasıdır: parola özetleri,"
  hata "oturumlar ve şifreli sağlayıcı anahtarları içinde"
  exit 1
fi

# NFR-808: "ayrı konumda". Aynı diskteki yedek, disk arızasında yedek
# değildir — ve cron her gece "başarılı" demeye devam eder.
if [ "$APP_ENV" = "production" ] && [ -z "$BACKUP_REMOTE" ]; then
  hata "BACKUP_REMOTE boş — üretimde yedek AYRI KONUMA kopyalanmadan"
  hata "başarılı sayılmaz (NFR-808)"
  exit 1
fi

case "$BACKUP_RETENTION_DAYS" in
  ''|*[!0-9]*) hata "BACKUP_RETENTION_DAYS sayı olmalı: '$BACKUP_RETENTION_DAYS'"; exit 1 ;;
esac

mkdir -p "$BACKUP_DIR" || { hata "$BACKUP_DIR oluşturulamadı"; exit 1; }
chmod 700 "$BACKUP_DIR" 2>/dev/null || true

# ─────────────────────────── Dump ───────────────────────────

DAMGA="$(date -u +%Y%m%dT%H%M%SZ)"
HAM="$BACKUP_DIR/.gecici-$DAMGA.dump"

# Şifrelenmemiş ara dosya HER ÇIKIŞTA silinir — hata yolunda da, kesintide de.
temizle() { rm -f "$HAM"; }
trap temizle EXIT INT TERM

if [ -n "$PG_DUMP_CMD" ]; then
  # shellcheck disable=SC2086
  $PG_DUMP_CMD > "$HAM"
else
  docker compose -f "$COMPOSE_DOSYA" exec -T postgres \
    pg_dump -U "${POSTGRES_USER:?POSTGRES_USER tanımsız}" \
            -d "${POSTGRES_DB:?POSTGRES_DB tanımsız}" \
            --format=custom --no-owner > "$HAM"
fi
DUMP_KOD=$?

if [ "$DUMP_KOD" -ne 0 ]; then
  hata "pg_dump başarısız (çıkış kodu $DUMP_KOD)"
  exit 1
fi
ok "dump alındı"

# ─────────────────────── Doğrulama (üç kapı) ───────────────────────

BOYUT=$(wc -c < "$HAM" | tr -d ' ')
if [ "$BOYUT" -lt "$BACKUP_MIN_BYTES" ]; then
  hata "dump çok küçük: $BOYUT bayt (asgari $BACKUP_MIN_BYTES)"
  hata "boş bir dosyayı yedek diye saklamak, yedeğin olmadığını fark etmemektir"
  exit 1
fi
ok "boyut kapısı geçildi ($BOYUT bayt)"

# 🔴 ASIL DOĞRULAMA: dosya gerçekten geri yüklenebilir bir arşiv mi?
# Şifreleme AÇIK METİN üzerinde yapılmadan önce burada bakılır; şifrelendikten
# sonra özel anahtar sunucuda olmadığı için (ve olmaması gerektiği için)
# okunamaz. `pg_restore --list` arşivin içindekileri sayar.
if command -v "$PG_RESTORE_CMD" >/dev/null 2>&1; then
  NESNE=$("$PG_RESTORE_CMD" --list "$HAM" 2>/dev/null | grep -c ';' || true)
else
  NESNE=$(docker compose -f "$COMPOSE_DOSYA" exec -T postgres \
            pg_restore --list /dev/stdin < "$HAM" 2>/dev/null | grep -c ';' || true)
fi
if [ "${NESNE:-0}" -lt 1 ]; then
  hata "pg_restore --list dosyayı okuyamadı — arşiv bozuk"
  exit 1
fi
ok "arşiv okunabilir ($NESNE giriş)"

# ─────────────────────────── Şifreleme ───────────────────────────

if [ -n "$BACKUP_AGE_RECIPIENT" ]; then
  HEDEF="$BACKUP_DIR/onay360-$DAMGA.dump.age"
  if ! command -v age >/dev/null 2>&1; then
    hata "age kurulu değil (BACKUP_AGE_RECIPIENT tanımlı)"
    exit 1
  fi
  age -r "$BACKUP_AGE_RECIPIENT" -o "$HEDEF" "$HAM" || { hata "age şifreleme başarısız"; rm -f "$HEDEF"; exit 1; }
else
  HEDEF="$BACKUP_DIR/onay360-$DAMGA.dump.gpg"
  if ! command -v gpg >/dev/null 2>&1; then
    hata "gpg kurulu değil (BACKUP_GPG_RECIPIENT tanımlı)"
    exit 1
  fi
  gpg --batch --yes --trust-model always \
      --recipient "$BACKUP_GPG_RECIPIENT" \
      --output "$HEDEF" --encrypt "$HAM" \
    || { hata "gpg şifreleme başarısız"; rm -f "$HEDEF"; exit 1; }
fi
chmod 600 "$HEDEF"
ok "şifrelendi: $(basename "$HEDEF")"

# Bütünlük yan dosyası — geri yükleme bunu doğrular.
if command -v sha256sum >/dev/null 2>&1; then
  (cd "$BACKUP_DIR" && sha256sum "$(basename "$HEDEF")" > "$(basename "$HEDEF").sha256")
else
  (cd "$BACKUP_DIR" && shasum -a 256 "$(basename "$HEDEF")" > "$(basename "$HEDEF").sha256")
fi
ok "sha256 yazıldı"

# ─────────────────────────── Ayrı konum ───────────────────────────

UZAK_DURUM="atlandı (BACKUP_REMOTE boş)"
if [ -n "$BACKUP_REMOTE" ]; then
  if [ -n "$UZAK_KOPYA_CMD" ]; then
    # shellcheck disable=SC2086
    $UZAK_KOPYA_CMD "$HEDEF" "$BACKUP_REMOTE"
    KOPYA_KOD=$?
  elif command -v rclone >/dev/null 2>&1; then
    rclone copy "$HEDEF" "$BACKUP_REMOTE" && rclone copy "$HEDEF.sha256" "$BACKUP_REMOTE"
    KOPYA_KOD=$?
  elif command -v scp >/dev/null 2>&1; then
    scp -q "$HEDEF" "$HEDEF.sha256" "$BACKUP_REMOTE"
    KOPYA_KOD=$?
  else
    hata "ne rclone ne scp bulundu — uzak kopya yapılamıyor"
    exit 1
  fi

  if [ "$KOPYA_KOD" -ne 0 ]; then
    hata "uzak kopya BAŞARISIZ ($BACKUP_REMOTE)"
    hata "yerel dosya duruyor ama bu yedek NFR-808'i karşılamıyor"
    exit 1
  fi
  UZAK_DURUM="$BACKUP_REMOTE"
  ok "ayrı konuma kopyalandı"
fi

# ─────────────────────────── Saklama süresi ───────────────────────────
#
# `-mtime +N` → SADECE N günden ESKİ dosyalar. `+` işareti olmadan (veya
# N=0 ile) bugünün yedeği de silinir; yanlış yazılmış tek bir find, tüm
# yedek geçmişini bir gecede yok eder.
SILINEN=0
while IFS= read -r eski; do
  [ -n "$eski" ] || continue
  rm -f "$eski"
  SILINEN=$((SILINEN + 1))
done < <(find "$BACKUP_DIR" -maxdepth 1 -type f \
           \( -name 'onay360-*.dump.age' -o -name 'onay360-*.dump.gpg' -o -name 'onay360-*.sha256' \) \
           -mtime +"$BACKUP_RETENTION_DAYS" 2>/dev/null)
ok "saklama temizliği: $SILINEN dosya silindi (> $BACKUP_RETENTION_DAYS gün)"

echo "── yedek tamam · $(basename "$HEDEF") · $BOYUT bayt · uzak: $UZAK_DURUM ──"
