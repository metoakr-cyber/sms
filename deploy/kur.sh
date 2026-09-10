#!/usr/bin/env bash
#
# Onay360 — TEK KOMUTLUK sunucu kurulumu. Docker YOK.
#
#   sudo bash -c "$(curl -fsSL https://raw.githubusercontent.com/metoakr-cyber/sms/main/deploy/kur.sh)"
#
# Bu betik yalnız ÖNYÜKLEYİCİDİR: git'i kurar, depoyu /opt/onay360'a çeker ve
# asıl kurulumu (deploy/kurulum.sh) çalıştırır. Kurulumun kendisi orada.
#
# ══════════════════════════════════════════════════════════════════════════
# NEDEN `bash -c "$(curl …)"`, NEDEN `curl … | bash` DEĞİL
# ══════════════════════════════════════════════════════════════════════════
# Asıl kurulum İNTERAKTİFTİR: port, parola, alan adı sorar. `curl … | bash`
# betiği STDIN'den okur, yani `read` komutlarının okuyacağı girdi kalmaz —
# bütün sorular boş cevapla geçilir ve kullanıcı bunu fark etmez. Komut
# ikamesinde ise betik metni ARGÜMAN olarak gelir, stdin terminalde kalır ve
# sorular normal çalışır.
#
# ══════════════════════════════════════════════════════════════════════════
# 🔴 GÖVDE NEDEN TEK BİR FONKSİYON
# ══════════════════════════════════════════════════════════════════════════
# Ağ, indirme ortasında koparsa bash ELİNDEKİ YARIM METNİ ÇALIŞTIRIR. Yarım
# kalmış bir `rm -rf "$HEDEF"` satırı, değişkeni henüz atanmamışken felakettir.
# Gövde bir fonksiyonun içindeyse yarım indirilen dosya yalnız sözdizimi
# hatası verir; son satırdaki çağrıya hiç ulaşılmaz, dolayısıyla HİÇBİR ŞEY
# çalışmaz. Bu yüzden çağrı dosyanın EN SON satırındadır.
#
# Ortam değişkenleriyle değiştirilebilir:
#   ONAY360_DEPO  ONAY360_DAL  ONAY360_HEDEF

kur360() {
  set -euo pipefail

  local DEPO="${ONAY360_DEPO:-https://github.com/metoakr-cyber/sms.git}"
  local DAL="${ONAY360_DAL:-main}"
  local HEDEF="${ONAY360_HEDEF:-/opt/onay360}"

  local KIRMIZI='' YESIL='' SARI='' MAVI='' KALIN='' SIFIR=''
  if [[ -t 1 ]]; then
    KIRMIZI=$'\e[31m'; YESIL=$'\e[32m'; SARI=$'\e[33m'
    MAVI=$'\e[36m';    KALIN=$'\e[1m';  SIFIR=$'\e[0m'
  fi
  baslik() { printf '\n%s──── %s ────%s\n' "$KALIN$MAVI" "$1" "$SIFIR"; }
  tamam()  { printf '  %s✓%s %s\n' "$YESIL" "$SIFIR" "$1"; }
  uyari()  { printf '  %s!%s %s\n' "$SARI" "$SIFIR" "$1"; }
  hata()   { printf '\n%s✗ %s%s\n\n' "$KIRMIZI$KALIN" "$1" "$SIFIR" >&2; exit 1; }

  baslik "Onay360 kurulumu"

  # ── Ön kontroller ──
  # Kurulum apt kullanır, systemd birimi yazar ve sistem kullanıcısı açar.
  # Root olmadan hepsi yarı yolda düşer; en baştan söylemek daha dürüst.
  [[ $EUID -eq 0 ]] || hata 'root gerekli. Komut şöyle:
  sudo bash -c "$(curl -fsSL https://raw.githubusercontent.com/metoakr-cyber/sms/main/deploy/kur.sh)"'

  [[ -f /etc/os-release ]] || hata "Ubuntu/Debian bekleniyor (/etc/os-release yok)"
  # shellcheck disable=SC1091
  . /etc/os-release
  [[ "${ID:-}" == "ubuntu" || "${ID_LIKE:-}" == *debian* ]] \
    || hata "yalnız Ubuntu/Debian destekleniyor (bulunan: ${ID:-bilinmiyor})"
  tamam "${PRETTY_NAME:-$ID}"

  # ── git ──
  # Yalnız git ve indirme araçları kurulur. Go, Node, PostgreSQL, Caddy asıl
  # kurulum betiğinin işidir — burada tekrarlanmaz, tek yerde dursun.
  if ! command -v git >/dev/null 2>&1; then
    baslik "git"
    export DEBIAN_FRONTEND=noninteractive
    apt-get update -qq
    apt-get install -y -qq --no-install-recommends git ca-certificates curl >/dev/null
    tamam "git kuruldu"
  else
    tamam "git zaten var"
  fi

  # ── Depo ──
  baslik "Kaynak"
  if [[ -d "$HEDEF/.git" ]]; then
    # 🔴 `git reset --hard` İZLENMEYEN DOSYALARA DOKUNMAZ. `.env` (sırlar,
    # ENCRYPTION_KEY) ve yüklenen dekontlar izlenmiyor, dolayısıyla güncelleme
    # onları KORUR. `git clean` ÇAĞRILMAZ — çağrılsaydı sırlar silinir ve
    # veritabanındaki şifreli sağlayıcı anahtarları bir daha açılamazdı.
    local MEVCUT
    MEVCUT="$(git -C "$HEDEF" remote get-url origin 2>/dev/null || echo '')"
    if [[ -n "$MEVCUT" && "$MEVCUT" != "$DEPO" ]]; then
      uyari "$HEDEF başka bir depoya bağlı: $MEVCUT"
      uyari "güncelleme yine de $DEPO değil, mevcut origin üzerinden yapılacak"
    fi
    git -C "$HEDEF" fetch --depth 1 origin "$DAL" >/dev/null 2>&1 \
      || hata "depo güncellenemedi (dal: $DAL). Ağ ya da dal adı hatalı olabilir."
    git -C "$HEDEF" reset --hard "origin/$DAL" >/dev/null
    tamam "güncellendi: $HEDEF ($DAL)"
  else
    [[ -e "$HEDEF" && -n "$(ls -A "$HEDEF" 2>/dev/null)" ]] \
      && hata "$HEDEF dolu ama git deposu değil. Taşıyın ya da ONAY360_HEDEF ile başka bir dizin verin."
    git clone --depth 1 -b "$DAL" "$DEPO" "$HEDEF" >/dev/null 2>&1 \
      || hata "klonlanamadı: $DEPO ($DAL). Depo genel mi, dal adı doğru mu?"
    tamam "indirildi: $HEDEF ($DAL)"
  fi

  local BETIK="$HEDEF/deploy/kurulum.sh"
  [[ -f "$BETIK" ]] || hata "kurulum betiği bulunamadı: $BETIK"

  # ── Asıl kurulum ──
  # `exec` DEĞİL, normal çağrı: exec süreci değiştirir ve buradaki `trap`/çıkış
  # kodu yorumu kaybolurdu. Kurulum kendi hatalarını zaten anlatıyor.
  bash "$BETIK"
}

# 🔴 EN SON SATIR — yukarıdaki "yarım indirme" gerekçesi. Buraya taşınmadan
# önce hiçbir şey çalışmaz.
kur360 "$@"
