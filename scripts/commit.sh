#!/usr/bin/env bash
# Commit KAPISI: check.sh geçmeden commit YAPILMAZ.
# test: scripts/commit_gate_test.sh
#
# NEDEN VAR
# ─────────
# Bu depoda ÜÇ KEZ başarısız kontrollerle commit atıldı. Her seferinde sebep
# aynıydı: doğrulama ile commit AYNI kabuk komutunda çalıştırıldı, doğrulamanın
# çıktısı okunmadan commit satırı yürüdü.
#
# Disipline güvenmek işe yaramadı. Kapı artık kodda:
#
#   ./scripts/commit.sh "mesaj"          → check.sh geçerse commit eder
#   ./scripts/commit.sh -f "mesaj"       → BİLEREK atlar (gerekçe zorunlu)
#
# Bu, projenin kendi "Örüntü A" dersinin kendimize uygulanmış hali:
# bir garanti veriliyorsa (burada: "kontroller geçti"), o garanti KODA
# bağlanmalı, yoruma değil.
set -uo pipefail
cd "$(dirname "$0")/.."

FORCE=0
if [ "${1:-}" = "-f" ] || [ "${1:-}" = "--force" ]; then
  FORCE=1; shift
fi

MSG="${1:-}"
[ -n "$MSG" ] || { echo "kullanım: ./scripts/commit.sh [-f] \"commit mesajı\""; exit 1; }

if [ "$FORCE" -eq 1 ]; then
  echo "⚠️  KONTROLLER ATLANIYOR — bunun gerekçesi commit mesajında olmalı."
else
  echo "─── commit öncesi kontroller ───"
  if ! ./scripts/check.sh; then
    echo
    echo "✗ COMMIT YAPILMADI — önce kontrolleri düzeltin."
    echo "  Bilerek atlamak için: ./scripts/commit.sh -f \"mesaj (atlama gerekçesi ile)\""
    exit 1
  fi
  echo
fi

git add -A
git -c user.name="ikmetrik" -c user.email="mekanimcepte@gmail.com" \
    commit -q -F - <<COMMITMSG
$MSG

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
COMMITMSG
echo "✓ $(git log --oneline -1)"
