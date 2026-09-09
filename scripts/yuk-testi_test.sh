#!/usr/bin/env bash
# yuk-testi.sh gerçekten kapı mı, yoksa yalnız öyle mi yazıyor?
#
# Betiğin verdiği üç söz var:
#   1. development dışında hiçbir istek göndermez
#   2. defter mutabakatı bozuksa sıfırdan farklı kod döner
#   3. veri silmez (DELETE / TRUNCATE / DROP çalıştırmaz)
#
# Üçü de yalnız yorumda dursaydı, commit_gate_test.sh'in yazılma sebebi olan
# hatanın aynısı olurdu: "kapı var" cümlesi, kapının kendisi değildir.
set -uo pipefail
cd "$(dirname "$0")/.."

GECTI=0; DUSTU=0
ok()   { printf '  \033[32m✓\033[0m %s\n' "$1"; GECTI=$((GECTI+1)); }
kotu() { printf '  \033[31m✗\033[0m %s\n     %s\n' "$1" "${2:-}"; DUSTU=$((DUSTU+1)); }

# ── 1. development dışında koşmaz ──
dev_disi_ortam_reddedilir() {
  local cikti rc
  cikti=$(APP_ENV=production ./scripts/yuk-testi.sh mutabakat 2>&1); rc=$?
  if [ $rc -eq 0 ]; then
    kotu "dev dışı ortam reddedilmeli" "çıkış kodu 0"; return
  fi
  case "$cikti" in
    *"YALNIZ development"*) ok "dev dışı ortam reddediliyor (APP_ENV=production → çıkış $rc)" ;;
    *) kotu "dev dışı ortam reddedilmeli" "beklenen açıklama yok: $cikti" ;;
  esac
}

# ── 2. mutabakat sapması kapıyı düşürür ──
#
# Gerçek `wallet:reconcile` yerine SAPMA BİLDİREN sahte bir komut konur.
# Kapı tutuyorsa betik sıfırdan farklı kod döner.
mutabakat_sapmasi_kapiyi_dusurur() {
  local cikti rc
  cikti=$(YUK_RECONCILE_CMD='sh -c "echo \"🔴 1 kullanıcıda SAPMA VAR\"; exit 1"' \
    ./scripts/yuk-testi.sh mutabakat 2>&1); rc=$?
  if [ $rc -eq 0 ]; then
    kotu "sapma bildiren mutabakat kapıyı düşürmeli" "çıkış kodu 0 — kapı tutmadı"; return
  fi
  case "$cikti" in
    *"MUTABAKAT SAPMASI"*) ok "mutabakat sapması kapıyı düşürüyor (çıkış $rc)" ;;
    *) kotu "sapma bildiren mutabakat kapıyı düşürmeli" "beklenen açıklama yok: $cikti" ;;
  esac
}

# Karşı sınav: sapma YOKKEN kapı geçmeli. Aksi hâlde yukarıdaki test,
# "her koşulda düşen bir betik" ile de geçerdi ve hiçbir şey kanıtlamazdı.
mutabakat_temizken_gecer() {
  local rc
  YUK_RECONCILE_CMD='sh -c "echo \"✓ mutabakat tamam — sapma yok\"; exit 0"' \
    ./scripts/yuk-testi.sh mutabakat >/dev/null 2>&1; rc=$?
  [ $rc -eq 0 ] && ok "sapma yokken kapı geçiyor" \
    || kotu "sapma yokken kapı geçmeli" "çıkış kodu $rc"
}

# ── 3. veri silinmez ──
#
# Yük testi GELİŞTİRME veritabanında koşar. Bir DELETE, geliştiricinin
# katalogunu ve hesabını götürür — bu depoda beş kez yaşanmış bir kaza
# (Makefile'daki test-integration yorumu).
silme_ifadesi_yok() {
  local bulunan
  bulunan=$(grep -nE '(DELETE[[:space:]]+FROM|TRUNCATE|DROP[[:space:]]+(TABLE|DATABASE))' \
    scripts/yuk-testi.sh load/tohum.sh load/olcum.sh 2>/dev/null \
    | grep -v '^[^:]*:[0-9]*:[[:space:]]*#')
  if [ -n "$bulunan" ]; then
    kotu "yük testi betikleri veri silmemeli" "$bulunan"; return
  fi
  ok "yük testi betiklerinde yıkıcı SQL yok"
}

echo "─── yük testi kapıları ───"
dev_disi_ortam_reddedilir
mutabakat_sapmasi_kapiyi_dusurur
mutabakat_temizken_gecer
silme_ifadesi_yok

echo
if [ "$DUSTU" -eq 0 ]; then
  printf '\033[32m  %d kapı testi geçti\033[0m\n' "$GECTI"
  exit 0
fi
printf '\033[31m  %d kapı testi DÜŞTÜ\033[0m\n' "$DUSTU"
exit 1
