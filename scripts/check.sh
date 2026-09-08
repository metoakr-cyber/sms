#!/usr/bin/env bash
# Birleştirmeden ÖNCE çalıştırılır. Herhangi bir adım düşerse SIFIRDAN FARKLI
# kod döner ve neyin düştüğünü açıkça söyler.
#
# Bu betik var çünkü kayan test çıktısına bakıp "geçti sandım" hatası iki kez
# yapıldı. Artık tek bir özet satırı var ve göz kaçırılamıyor.
set -uo pipefail
cd "$(dirname "$0")/.."
[ -f .env ] && { set -a; source .env; set +a; }
export PATH="$PATH:$(go env GOPATH)/bin"

# Her adımın TAM çıktısı diske yazılır.
#
# Önceden yalnız süzülmüş 12 satır ekrana basılıyordu ve gerisi atılıyordu.
# Kararsız (bazen düşen) bir adımla karşılaşıldığında geriye bakılacak hiçbir
# şey kalmıyordu — hata bir daha üretilemezse teşhis de edilemiyor. Kapının
# kendisi kararsızsa, kanıtı saklamak kapının bir parçasıdır.
LOGDIR="${CHECK_LOG_DIR:-.check-logs}"
rm -rf "$LOGDIR"; mkdir -p "$LOGDIR"

FAILED=()
step() {
  local name="$1"; shift
  local slug; slug=$(printf '%s' "$name" | tr ' /' '__')
  local log="$LOGDIR/$slug.log"
  printf '  %-28s ' "$name"
  if "$@" >"$log" 2>&1; then
    printf '\033[32m✓\033[0m\n'
  else
    printf '\033[31m✗\033[0m\n'
    FAILED+=("$name")
    grep -E "FAIL|Error|error|✗|--- FAIL" "$log" | head -12 | sed 's/^/       /'
    printf '       \033[2mtam çıktı: %s\033[0m\n' "$log"
  fi
}

echo "─── kontroller ───"
step "garanti yorumları"    python3 scripts/check-guarantees.py
step "sqlc üretimi güncel"  bash -c 'cd api && sqlc diff'
step "go vet"               bash -c 'cd api && go vet ./...'
step "go vet (integration)" bash -c 'cd api && go vet -tags=integration ./...'
step "derleme"              bash -c 'cd api && go build ./...'
step "birim testler"        bash -c 'cd api && go test ./... -race -count=1'
step "entegrasyon testleri" bash -c 'cd api && go test -tags=integration -p 1 ./... -race -count=1'
step "duman testi"          ./scripts/smoke-auth.sh
step "web tip denetimi"     bash -c 'cd web && npx tsc --noEmit'
step "web derleme"          bash -c 'cd web && NEXT_DIST_DIR=.next-check npm run build >/dev/null'
step "commit kapısı"        ./scripts/commit_gate_test.sh

# NOT: responsive denetimi (web/scripts/responsive-check.mjs) BURADA DEĞİL.
# İki sunucunun da ayakta olmasını gerektirir; birleştirme kapısını dış
# duruma bağımlı kılmak, kapının rastgele düşmesi demektir. Elle:
#   make responsive

echo
if [ ${#FAILED[@]} -eq 0 ]; then
  printf '\033[32m\033[1mTÜM KONTROLLER GEÇTİ\033[0m\n'
  exit 0
fi
printf '\033[31m\033[1m%d KONTROL DÜŞTÜ:\033[0m %s\n' "${#FAILED[@]}" "${FAILED[*]}"
echo "Birleştirme yapılmamalı."
exit 1
