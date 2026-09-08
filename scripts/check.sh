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

FAILED=()
step() {
  local name="$1"; shift
  printf '  %-28s ' "$name"
  if out=$("$@" 2>&1); then
    printf '\033[32m✓\033[0m\n'
  else
    printf '\033[31m✗\033[0m\n'
    FAILED+=("$name")
    printf '%s\n' "$out" | grep -E "FAIL|Error|error|✗|--- FAIL" | head -12 | sed 's/^/       /'
  fi
}

echo "─── kontroller ───"
step "sqlc üretimi güncel"  bash -c 'cd api && sqlc diff'
step "go vet"               bash -c 'cd api && go vet ./...'
step "go vet (integration)" bash -c 'cd api && go vet -tags=integration ./...'
step "derleme"              bash -c 'cd api && go build ./...'
step "birim testler"        bash -c 'cd api && go test ./... -race -count=1'
step "entegrasyon testleri" bash -c 'cd api && go test -tags=integration -p 1 ./... -race -count=1'
step "duman testi"          ./scripts/smoke-auth.sh

echo
if [ ${#FAILED[@]} -eq 0 ]; then
  printf '\033[32m\033[1mTÜM KONTROLLER GEÇTİ\033[0m\n'
  exit 0
fi
printf '\033[31m\033[1m%d KONTROL DÜŞTÜ:\033[0m %s\n' "${#FAILED[@]}" "${FAILED[*]}"
echo "Birleştirme yapılmamalı."
exit 1
