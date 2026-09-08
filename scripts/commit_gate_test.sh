#!/usr/bin/env bash
# commit.sh gerçekten kapı mı, yoksa yalnız öyle mi yazıyor?
#
# Kontroller DÜŞTÜĞÜNDE commit.sh commit ATMAMALIDIR. Bunu kanıtlamak için
# check.sh'i düşen bir sahteyle değiştirir, commit.sh'i çalıştırır ve HEAD'in
# oynamadığını doğrularız. Kapının kendisi test edilmezse, "kapı var" cümlesi
# de tıpkı düzelttiğimiz diğer yorumlar gibi bir iddiadan ibaret kalır.
set -uo pipefail
cd "$(dirname "$0")/.."

command -v git >/dev/null || { echo "git yok"; exit 1; }
BEFORE=$(git rev-parse HEAD)
REAL=scripts/check.sh
BACKUP=$(mktemp)
cp "$REAL" "$BACKUP"
restore() { cp "$BACKUP" "$REAL"; chmod +x "$REAL"; rm -f "$BACKUP"; }
trap restore EXIT

printf '#!/usr/bin/env bash\nexit 1\n' > "$REAL"
chmod +x "$REAL"

OUT=$(./scripts/commit.sh "bu commit ASLA atılmamalı" 2>&1)
RC=$?
AFTER=$(git rev-parse HEAD)

if [ "$RC" -eq 0 ]; then
  echo "✗ kontroller düştüğü hâlde commit.sh başarı döndü"; exit 1
fi
if [ "$BEFORE" != "$AFTER" ]; then
  echo "✗ KAPI TUTMADI: kontroller düşerken commit atıldı ($BEFORE → $AFTER)"; exit 1
fi
case "$OUT" in
  *"COMMIT YAPILMADI"*) ;;
  *) echo "✗ beklenen açıklama yok; çıktı: $OUT"; exit 1 ;;
esac

echo "  ✓ commit kapısı tutuyor (kontroller düşerken commit yok)"
