#!/usr/bin/env bash
#
# Onay360 — KOD GÜNCELLEMESİ. Soru sormaz.
#
#   sudo /opt/onay360/deploy/guncelle.sh
#
# ══════════════════════════════════════════════════════════════════════════
# NEDEN AYRI BETİK
# ══════════════════════════════════════════════════════════════════════════
# `kurulum.sh` her çalıştığında 15 soru sorar — kurulum için doğru, güncelleme
# için değil. Kod her değiştiğinde aynı cevapları tekrar yazmak hem yorucu hem
# HATALI: bir soruyu yanlış cevaplamak çalışan bir kurulumu bozabilir.
#
# Bu betik YAPILANDIRMAYA HİÇ DOKUNMAZ. `.env`, `/etc/caddy/*`, sertifikalar,
# systemd birimleri, veritabanı parolası — hepsi olduğu gibi kalır. Yalnız
# şunları yapar:
#
#   kod çek → derle → migration → servis logoları → servisleri yenile → sağlık
#
# Yapılandırma değişecekse (port, alan adı, sertifika, üretim moduna geçiş)
# `kurulum.sh` çalıştırılır; o zaman sorular yerindedir.
#
# ══════════════════════════════════════════════════════════════════════════
# 🔴 CADDY'YE DOKUNULMAZ
# ══════════════════════════════════════════════════════════════════════════
# Ters vekil yapılandırması kuruluma aittir. Burada yeniden yazılsaydı,
# `kurulum.sh`taki sertifika mantığı ikinci bir yerde daha durur ve ikisi
# zamanla ayrışırdı. Caddy yalnız Caddyfile deposu değiştiyse yeniden
# yüklenir — o da aşağıda, karşılaştırmayla.
set -euo pipefail

KOK="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_YOLU="$KOK/.env"

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

ADIM="başlangıç"
trap 'hata "güncelleme \"$ADIM\" adımında durdu (satır $LINENO). Servisler ESKİ sürümle çalışmaya devam ediyor."' ERR

# ─────────────────────────── Ön kontroller ───────────────────────────
ADIM="ön kontroller"
[[ $EUID -eq 0 ]] || hata "root gerekli:  sudo $KOK/deploy/guncelle.sh"
[[ -f "$ENV_YOLU" ]] || hata "$ENV_YOLU yok — bu sunucuda kurulum yapılmamış. Önce: sudo bash deploy/kurulum.sh"
[[ -d "$KOK/.git" ]] || hata "$KOK bir git deposu değil"

# 🔴 GO APT PAKETİ DEĞİL. `kurulum.sh` onu /usr/local/go altına açar ve o dizin
# root'un VARSAYILAN PATH'inde YOKTUR — kurulum kendi içinde export ettiği için
# orada sorun çıkmaz, ama bu betik ayrı bir kabukta koşar. Ölçüldü (11 Eylül
# 2026, gerçek sunucu): "line 98: go: command not found" ile derleme adımında
# durdu. `goose` GOBIN=/usr/local/bin ile kurulduğu için zaten yoldadır;
# node/npm apt'tan gelir. Yine de üçü de burada AÇIKÇA aranır: eksik bir aracı
# derleme ortasında keşfetmek, yarım kalmış bir güncelleme demektir.
export PATH="/usr/local/go/bin:/root/go/bin:$PATH"
for arac in go npm goose; do
  command -v "$arac" >/dev/null 2>&1 \
    || hata "$arac bulunamadı — bu sunucuda kurulum eksik. Çalıştırın: sudo bash $KOK/deploy/kurulum.sh"
done

# Yapılandırma .env'den OKUNUR, sorulmaz. Derlemenin ihtiyaç duyduğu üç değer:
set -a; . "$ENV_YOLU"; set +a
KOK_URL="${PUBLIC_BASE_URL:?PUBLIC_BASE_URL .env içinde yok}"
RECAPTCHA_SITE="${RECAPTCHA_SITE_KEY:-}"
# Servis kullanıcısı systemd biriminden okunur: .env'de yazmıyor ve tahmin
# etmek, dosyaların yanlış kullanıcıya geçmesi demek olurdu.
SERVIS_KULLANICI="$(awk -F= '/^User=/{print $2; exit}' /etc/systemd/system/onay360-api.service 2>/dev/null || true)"
[[ -n "$SERVIS_KULLANICI" ]] || hata "servis kullanıcısı okunamadı (/etc/systemd/system/onay360-api.service)"

DAL="$(git -C "$KOK" rev-parse --abbrev-ref HEAD 2>/dev/null || echo main)"
[[ "$DAL" == "HEAD" ]] && DAL="main"

baslik "Onay360 güncelleme"
bilgi "dizin   : $KOK"
bilgi "dal     : $DAL"
bilgi "adres   : $KOK_URL"
bilgi "kullanıcı: $SERVIS_KULLANICI"

# ─────────────────────────── Kod ───────────────────────────
ADIM="kod"
baslik "Kod"
ONCE="$(git -C "$KOK" rev-parse --short HEAD)"
CADDY_ONCE="$(sha256sum "$KOK/deploy/Caddyfile" 2>/dev/null | cut -d' ' -f1 || echo yok)"

# 🔴 `git clean` ÇAĞRILMAZ. `.env` ve yüklenen dekontlar izlenmiyor; temizlik
# onları siler ve ENCRYPTION_KEY gidince veritabanındaki şifreli sağlayıcı
# anahtarları BİR DAHA AÇILAMAZ. `reset --hard` izlenmeyen dosyalara dokunmaz.
git -C "$KOK" fetch --depth 1 origin "$DAL" >/dev/null 2>&1 \
  || hata "uzak depoya erişilemedi (dal: $DAL)"
git -C "$KOK" reset --hard "origin/$DAL" >/dev/null
SONRA="$(git -C "$KOK" rev-parse --short HEAD)"

if [[ "$ONCE" == "$SONRA" ]]; then
  tamam "zaten güncel ($SONRA) — yine de derlenip yeniden başlatılacak"
else
  tamam "$ONCE → $SONRA"
  git -C "$KOK" log --oneline "$ONCE..$SONRA" 2>/dev/null | head -10 | sed 's/^/    /'
fi

# ─────────────────────────── Derleme ───────────────────────────
ADIM="derleme"
baslik "Derleme"
install -d "$KOK/bin"
( cd "$KOK/api" && go build -trimpath -ldflags="-s -w" -o "$KOK/bin/api"    ./cmd/server )
( cd "$KOK/api" && go build -trimpath -ldflags="-s -w" -o "$KOK/bin/worker" ./cmd/worker )
( cd "$KOK/api" && go build -trimpath -ldflags="-s -w" -o "$KOK/bin/cli"    ./cmd/cli )
tamam "api · worker · cli"

ADIM="ön yüz derlemesi"
( cd "$KOK/web" \
  && npm ci --no-audit --no-fund >/dev/null 2>&1 \
  && NEXT_TELEMETRY_DISABLED=1 NODE_ENV=production \
     NEXT_PUBLIC_SITE_URL="$KOK_URL" \
     NEXT_PUBLIC_RECAPTCHA_SITE_KEY="$RECAPTCHA_SITE" \
     npm run build >/dev/null )
tamam "ön yüz (standalone)"
chown -R "$SERVIS_KULLANICI":"$SERVIS_KULLANICI" "$KOK/bin" "$KOK/web/.next"

# ─────────────────────────── Veritabanı ───────────────────────────
ADIM="migration"
baslik "Veritabanı şeması"
( cd "$KOK/api" && goose -dir migrations postgres "$DATABASE_URL" up )
tamam "migration'lar uygulandı"

# Servis logoları: yeni servisler eklenmiş olabilir, dosya her seferinde
# uygulanır (idempotent: `WHERE code = ...` ile günceller).
ADIM="servis logoları"
IKON_SQL="$KOK/web/scripts/servis-logolari.sql"
DB_ADI="$(printf '%s' "$DATABASE_URL" | sed -E 's#.*/([^/?]+).*#\1#')"
if [[ -f "$IKON_SQL" ]] && sudo -u postgres psql -q -d "$DB_ADI" -f "$IKON_SQL" >/dev/null 2>&1; then
  tamam "servis logoları bağlandı"
else
  uyari "servis logoları bağlanamadı — site logosuz açılabilir"
fi

# ─────────────────────────── Servisler ───────────────────────────
ADIM="servisler"
baslik "Servisler"
systemctl restart onay360-api.service
systemctl restart onay360-web.service
tamam "onay360-api · onay360-web yeniden başlatıldı"

# Caddy YALNIZ yapılandırma dosyası değiştiyse yenilenir. Koşulsuz yeniden
# yüklemek, TLS oturumlarını gereksiz yere kesmek demektir.
CADDY_SONRA="$(sha256sum "$KOK/deploy/Caddyfile" 2>/dev/null | cut -d' ' -f1 || echo yok)"
if [[ "$CADDY_ONCE" != "$CADDY_SONRA" ]]; then
  cp "$KOK/deploy/Caddyfile" /etc/caddy/Caddyfile
  if systemctl reload caddy >/dev/null 2>&1; then
    tamam "caddy yenilendi (Caddyfile değişmişti)"
  else
    uyari "caddy yenilenemedi — yapılandırmayı kontrol edin: caddy validate --config /etc/caddy/Caddyfile"
  fi
fi

# ─────────────────────────── Sağlık ───────────────────────────
ADIM="sağlık"
baslik "Sağlık"
API_PORT_OKU="$(awk -F'PORT=' '/Environment=PORT=/{print $2; exit}' /etc/systemd/system/onay360-api.service 2>/dev/null || true)"
API_PORT_OKU="${API_PORT_OKU:-${API_PORT:-8091}}"
for _ in $(seq 1 30); do
  if curl -fsS -o /dev/null --max-time 2 "http://127.0.0.1:${API_PORT_OKU}/readyz" 2>/dev/null; then
    tamam "API hazır"; break
  fi
  sleep 1
done
curl -fsS -o /dev/null --max-time 2 "http://127.0.0.1:${API_PORT_OKU}/readyz" 2>/dev/null \
  || uyari "API /readyz yanıt vermedi — journalctl -u onay360-api -n 50"

printf '\n%s──── Güncelleme tamam (%s) ────%s\n\n' "$KALIN$MAVI" "$SONRA" "$SIFIR"
