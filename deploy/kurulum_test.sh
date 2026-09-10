#!/usr/bin/env bash
#
# kurulum.sh'in GERÇEK Ubuntu üzerinde uçtan uca testi.
#
# ══════════════════════════════════════════════════════════════════════════
# NEDEN VAR
# ══════════════════════════════════════════════════════════════════════════
# Kurulum betiği yalnız "sözdizimi doğru mu" diye sınanamaz: hataları apt
# depolarında, systemd birimlerinde ve Caddy yapılandırmasında çıkar ve
# hiçbiri `bash -n` ile görünmez. Bu test ilk koşuşunda ÜÇ GERÇEK HATA
# yakaladı ve üçü de göz kararıyla görünmüyordu:
#
#   1. IP modundaki Caddyfile'da tek satırlık `handle /x { ... }` blokları —
#      Caddy bunu kabul etmiyor ("Unexpected next token after '{'").
#   2. `user:create` başarısızlığı "yönetici zaten var" diye raporlanıyordu;
#      parola politikaya takıldığında kurulum YÖNETİCİSİZ bitiyor ve bunu
#      söylemiyordu.
#   3. `cp -r static hedef` ikinci çalıştırmada `static/static` üretiyordu —
#      site CSS'siz açılırdı.
#
# ══════════════════════════════════════════════════════════════════════════
# NE SINAR
# ══════════════════════════════════════════════════════════════════════════
#   · Betik baştan sona hatasız biter (çıkış 0)
#   · systemd birimleri gerçekten AYAĞA KALKAR (konteynerde systemd koşar)
#   · Caddy yapılandırması geçerli ve site 80. porttan servis edilir
#   · Statik dosyalar doğru yoldan gelir (CSS 200)
#   · Tekrar çalıştırma veriyi silmez ve SIRLARI korur
#
# 🔴 KONTEYNER systemd İLE KOŞAR (--privileged). Bu bir üretim deseni değil,
# test gereğidir: systemd olmadan birimler hiç sınanamaz.
#
# Kullanım:
#   bash deploy/kurulum_test.sh
#
# Gereksinim: docker · ~3 GB boş disk · internet
set -euo pipefail

KOK="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
KONTEYNER="onay360-kurulum-testi"
IMAJ="onay360-ubuntu-test"
GECICI="$(mktemp -d)"
trap 'rm -rf "$GECICI"' EXIT

gec()  { printf '  \e[32m✓\e[0m %s\n' "$1"; }
dus()  { printf '  \e[31m✗\e[0m %s\n' "$1"; BASARISIZ=$((BASARISIZ + 1)); }
bolum(){ printf '\n\e[1;36m── %s ──\e[0m\n' "$1"; }
BASARISIZ=0

bolum "Ortam"
command -v docker >/dev/null || { echo "docker gerekli"; exit 1; }

cat > "$GECICI/Dockerfile" <<'EOF'
FROM ubuntu:24.04
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends \
      systemd systemd-sysv dbus sudo ca-certificates curl \
    && rm -rf /var/lib/apt/lists/* \
    && rm -f /lib/systemd/system/multi-user.target.wants/* \
             /etc/systemd/system/*.wants/* \
             /lib/systemd/system/local-fs.target.wants/* \
             /lib/systemd/system/sockets.target.wants/*udev* \
             /lib/systemd/system/basic.target.wants/*
STOPSIGNAL SIGRTMIN+3
CMD ["/sbin/init"]
EOF
docker build -q -t "$IMAJ" "$GECICI" >/dev/null
gec "systemd'li Ubuntu imajı"

docker rm -f "$KONTEYNER" >/dev/null 2>&1 || true
docker run -d --name "$KONTEYNER" --privileged --cgroupns=host \
  -v /sys/fs/cgroup:/sys/fs/cgroup:rw --tmpfs /run --tmpfs /run/lock \
  "$IMAJ" >/dev/null
for _ in $(seq 1 30); do
  docker exec "$KONTEYNER" systemctl is-system-running 2>/dev/null | grep -qE 'running|degraded' && break
  timeout 2 tail -f /dev/null || true
done
gec "konteyner ayakta, systemd koşuyor"

# Kaynak `git archive` ile gider: commit'lenmiş ağaç, yani node_modules,
# .next ve .env kendiliğinden dışarıda kalır.
DAL="$(git -C "$KOK" branch --show-current)"
git -C "$KOK" archive --format=tar "$DAL" \
  | docker exec -i "$KONTEYNER" bash -c 'mkdir -p /opt/onay360 && tar -x -C /opt/onay360'
# Çalışma ağacındaki (henüz commit'lenmemiş) betikle test edilir.
docker cp "$KOK/deploy/kurulum.sh" "$KONTEYNER:/opt/onay360/deploy/kurulum.sh" >/dev/null
gec "kaynak kopyalandı (dal: $DAL)"

# Cevaplar: alan adı yok (IP modu), sağlayıcı anahtarı yok, staging.
# 🔴 Parola e-postanın yerel parçasını ya da kullanıcı adını İÇEREMEZ —
# sunucudaki politika bunu reddeder (testin ilk koşuşunda buna takıldı).
cat > "$GECICI/cevaplar" <<'EOF'

kurulum@ornek.test

Gk7-pil-zimba-vadi-91
Gk7-pil-zimba-vadi-91






















EOF

bolum "Kurulum (ilk koşu — derleme dâhil, uzun sürer)"
if docker exec -i "$KONTEYNER" bash -c 'cd /opt/onay360 && bash deploy/kurulum.sh' \
     < "$GECICI/cevaplar" > "$GECICI/kurulum1.log" 2>&1; then
  gec "betik çıkış 0 ile bitti"
else
  dus "betik düştü — son satırlar:"
  tail -15 "$GECICI/kurulum1.log" | sed 's/^/      /'
fi

bolum "Servisler"
for birim in onay360-api onay360-web caddy onay360-katalog.timer; do
  if docker exec "$KONTEYNER" systemctl is-active "$birim" 2>/dev/null | grep -q active; then
    gec "$birim etkin"
  else
    dus "$birim ETKİN DEĞİL"
  fi
done

bolum "HTTP (Caddy üzerinden, 80. port)"
kontrol_http() { # kontrol_http <yol> <beklenen kod>
  local kod
  kod="$(docker exec "$KONTEYNER" curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1$1" || echo 000)"
  [[ "$kod" == "$2" ]] && gec "$1 → $kod" || dus "$1 → $kod (beklenen $2)"
}
kontrol_http /        200
kontrol_http /giris   200
kontrol_http /readyz  200
kontrol_http /metrics 404   # dışarıya kapalı olmalı

# Statik dosyalar: `cp -r` hatası burada görünür (CSS 404 döner).
CSS="$(docker exec "$KONTEYNER" bash -c "curl -s http://127.0.0.1/ | grep -oE '/_next/static/css/[^\"]+\.css' | head -1")"
if [[ -n "$CSS" ]]; then
  kontrol_http "$CSS" 200
else
  dus "ana sayfada CSS bağlantısı yok"
fi

bolum "Tekrar çalıştırma (yapılandırma yenileme)"
if docker exec -i -e KURULUM_DERLEMEYI_ATLA=1 "$KONTEYNER" \
     bash -c 'cd /opt/onay360 && bash deploy/kurulum.sh' \
     < "$GECICI/cevaplar" > "$GECICI/kurulum2.log" 2>&1; then
  gec "ikinci koşu çıkış 0"
else
  dus "ikinci koşu düştü"
  tail -10 "$GECICI/kurulum2.log" | sed 's/^/      /'
fi
grep -q "VERİYE DOKUNULMADI" "$GECICI/kurulum2.log" \
  && gec "veritabanı korundu" || dus "veritabanı korunma mesajı yok"

# 🔴 EN KRİTİK İDDİA: ENCRYPTION_KEY değişirse veritabanındaki şifreli
# sağlayıcı anahtarları BİR DAHA AÇILAMAZ.
if docker exec "$KONTEYNER" bash -c '
    A=$(grep "^ENCRYPTION_KEY=" /opt/onay360/.env | cut -d= -f2-)
    for f in /opt/onay360/.env.yedek.*; do
      [ "$A" = "$(grep "^ENCRYPTION_KEY=" "$f" | cut -d= -f2-)" ] || exit 1
    done' 2>/dev/null; then
  gec "sırlar korundu (ENCRYPTION_KEY değişmedi)"
else
  dus "SIRLAR DEĞİŞTİ — şifreli sağlayıcı anahtarları açılamaz hale gelir"
fi

docker exec "$KONTEYNER" bash -c 'ls -l /opt/onay360/.env' | grep -q '^-rw-------' \
  && gec ".env izinleri 600" || dus ".env izinleri gevşek"

bolum "Sonuç"
if [[ $BASARISIZ -eq 0 ]]; then
  printf '  \e[1;32mTÜM KONTROLLER GEÇTİ\e[0m\n\n'
  printf '  Konteyner inceleme için ayakta: docker exec -it %s bash\n' "$KONTEYNER"
  printf '  Silmek için: docker rm -f %s\n\n' "$KONTEYNER"
  exit 0
fi
printf '  \e[1;31m%d KONTROL DÜŞTÜ\e[0m — log: %s\n\n' "$BASARISIZ" "$GECICI"
exit 1
