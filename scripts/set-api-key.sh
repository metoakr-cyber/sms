#!/usr/bin/env bash
# HeroSMS API anahtarını .env'e güvenle yazar ve canlı doğrular.
# Anahtar ekranda görünmez ve kabuk geçmişine düşmez.
set -euo pipefail
cd "$(dirname "$0")/.."

[ -f .env ] || { echo "✗ .env yok. Önce: cp .env.example .env"; exit 1; }

printf 'HeroSMS API anahtarı (yazarken görünmez): '
read -rs KEY
echo

[ -n "$KEY" ] || { echo "✗ boş girdi"; exit 1; }
case "$KEY" in
  YENI_ANAHTARINIZ|buraya*|xxx*) echo "✗ yer tutucu girdiniz, gerçek anahtarı yapıştırın"; exit 1 ;;
esac

echo -n "→ doğrulanıyor... "
BAL=$(curl -sS --max-time 15 -H 'User-Agent: sms-platform/0.1' \
  "https://hero-sms.com/stubs/handler_api.php?api_key=${KEY}&action=getBalance" || true)

case "$BAL" in
  ACCESS_BALANCE:*)
    echo "✓ geçerli — bakiye: ${BAL#ACCESS_BALANCE:} USD" ;;
  *)
    echo "✗ REDDEDİLDİ"
    echo "  sağlayıcı yanıtı: ${BAL:0:160}"
    echo "  .env DEĞİŞTİRİLMEDİ."
    exit 1 ;;
esac

# yalnız doğrulandıysa yaz
TMP=$(mktemp)
trap 'rm -f "$TMP"' EXIT
KEY="$KEY" awk '
  /^HEROSMS_API_KEY=/ { print "HEROSMS_API_KEY=" ENVIRON["KEY"]; found=1; next }
  { print }
  END { if (!found) print "HEROSMS_API_KEY=" ENVIRON["KEY"] }
' .env > "$TMP"
cat "$TMP" > .env
chmod 600 .env

echo "✓ .env güncellendi (izinler 600, git yok sayıyor)"
echo "  ${KEY:0:4}…${KEY: -4}  (${#KEY} karakter)"
