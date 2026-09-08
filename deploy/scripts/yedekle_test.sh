#!/usr/bin/env bash
# Yedekleme kapıları — sabotaj testi.
#
# NEDEN KONTEYNER İÇİNDE: betik pg_dump, pg_restore ve age ikililerini
# kullanıyor; bunları host'ta varsaymak testi "bende çalışıyor"a çevirirdi.
#
# ÇALIŞTIRMA (depo kökünden):
#   docker run --rm -v "$PWD:/repo:ro" -v /tmp/yedektest:/calisma \
#     --network smsplatform-dev_default \
#     -e PGPASSWORD -e BACKUP_DB_URL \
#     postgres:16-alpine sh -c 'apk add --no-cache age bash >/dev/null &&
#       bash /repo/deploy/scripts/yedekle_test.sh'
#
# Bu betik check.sh'e BAĞLI DEĞİLDİR (docker gerektirir); yedekleme
# betiklerine dokunulduğunda elle koşturulur.
set -u
KOK=/repo
BETIK="$KOK/deploy/scripts/yedekle.sh"
GERI="$KOK/deploy/scripts/geri-yukle.sh"
T=/calisma/t
GECTI=0; KALDI=0
gec() { GECTI=$((GECTI+1)); printf '  \033[32mGEÇTİ\033[0m %s\n' "$1"; }
kal() { KALDI=$((KALDI+1)); printf '  \033[31mKALDI\033[0m %s\n' "$1"; }

age-keygen -o /calisma/age.key 2>/dev/null
ALICI=$(grep 'public key' /calisma/age.key | sed 's/.*: //')

hazirla() {
  rm -rf "$T"; mkdir -p "$T"
  export BACKUP_DIR="$T" BACKUP_RETENTION_DAYS=30 BACKUP_MIN_BYTES=100
  export BACKUP_AGE_RECIPIENT="$ALICI" BACKUP_GPG_RECIPIENT=""
  export BACKUP_REMOTE="" APP_ENV=staging
  export ENV_DOSYA=/dev/null
  export POSTGRES_USER=t POSTGRES_DB=t
  # Gerçek bir pg_dump çıktısı üret (custom format), veritabanı olmadan:
  export PG_DUMP_CMD="cat /calisma/ornek.dump"
  export UZAK_KOPYA_CMD=""
}

echo "örnek dump: $(wc -c < /calisma/ornek.dump) bayt"

echo
echo "── 1) alıcı yoksa reddeder ──"
hazirla; BACKUP_AGE_RECIPIENT="" BACKUP_GPG_RECIPIENT="" bash "$BETIK" >/tmp/o 2>&1
if [ $? -ne 0 ] && grep -q "RECIPIENT" /tmp/o; then gec "şifreleme alıcısı yoksa çıkış≠0"; else kal "alıcısız yedek kabul edildi"; cat /tmp/o; fi

echo "── 2) üretimde ayrı konum yoksa reddeder ──"
hazirla; APP_ENV=production bash "$BETIK" >/tmp/o 2>&1
if [ $? -ne 0 ] && grep -q "AYRI KONUMA" /tmp/o; then gec "BACKUP_REMOTE boşken üretimde çıkış≠0"; else kal "uzak konumsuz üretim yedeği kabul edildi"; cat /tmp/o; fi

echo "── 3) küçük dump reddedilir ve diskte KALMAZ ──"
hazirla; PG_DUMP_CMD="printf x" bash "$BETIK" >/tmp/o 2>&1
kod=$?; kalan=$(ls "$T" 2>/dev/null | wc -l | tr -d ' ')
if [ $kod -ne 0 ] && [ "$kalan" = "0" ]; then gec "1 baytlık dump reddedildi, dosya bırakılmadı"; else kal "küçük dump kabul edildi (kod=$kod kalan=$kalan)"; cat /tmp/o; fi

echo "── 4) bozuk (pg_restore okuyamayan) dump reddedilir ──"
hazirla; PG_DUMP_CMD="head -c 5000 /dev/urandom" bash "$BETIK" >/tmp/o 2>&1
kod=$?; kalan=$(ls "$T" 2>/dev/null | wc -l | tr -d ' ')
if [ $kod -ne 0 ] && grep -q "arşiv bozuk" /tmp/o && [ "$kalan" = "0" ]; then gec "pg_restore --list kapısı bozuk arşivi yakaladı"; else kal "bozuk arşiv kabul edildi (kod=$kod kalan=$kalan)"; cat /tmp/o; fi

echo "── 5) mutlu yol: şifreli dosya + sha256 üretilir ──"
hazirla; bash "$BETIK" >/tmp/o 2>&1
kod=$?; adet=$(ls "$T"/onay360-*.dump.age 2>/dev/null | wc -l | tr -d ' '); sha=$(ls "$T"/*.sha256 2>/dev/null | wc -l | tr -d ' ')
if [ $kod -eq 0 ] && [ "$adet" = "1" ] && [ "$sha" = "1" ]; then gec "yedek alındı, şifrelendi, sha256 yazıldı"; else kal "mutlu yol düştü (kod=$kod age=$adet sha=$sha)"; cat /tmp/o; fi
YEDEK=$(ls "$T"/onay360-*.dump.age 2>/dev/null | head -1)

echo "── 6) saklama: YALNIZ süresi dolmuş dosyaları siler ──"
touch -d '40 days ago' "$T/onay360-20250101T000000Z.dump.age" 2>/dev/null || touch -t 202501010000 "$T/onay360-20250101T000000Z.dump.age"
bash "$BETIK" >/tmp/o 2>&1
eski=$([ -f "$T/onay360-20250101T000000Z.dump.age" ] && echo VAR || echo YOK)
yeni=$([ -f "$YEDEK" ] && echo VAR || echo YOK)
if [ "$eski" = "YOK" ] && [ "$yeni" = "VAR" ]; then gec "40 günlük silindi, yeni yedek duruyor"; else kal "saklama yanlış (eski=$eski yeni=$yeni)"; fi

echo "── 7) geri-yukle: canlı veritabanını onaysız EZMEZ ──"
export POSTGRES_DB=t
sayac=/calisma/sayac; rm -f $sayac
cat > /calisma/sahte_restore <<'S'
#!/bin/sh
echo cagrildi >> /calisma/sayac
S
chmod +x /calisma/sahte_restore
PG_RESTORE_CMD=/calisma/sahte_restore BACKUP_AGE_KEY_FILE=/calisma/age.key \
  bash "$GERI" "$YEDEK" --hedef-db t >/tmp/o 2>&1
kod=$?; cagri=$([ -f $sayac ] && wc -l < $sayac || echo 0)
if [ $kod -ne 0 ] && [ "$(echo $cagri|tr -d ' ')" = "0" ]; then gec "--uretim-onayi olmadan canlı hedef reddedildi, pg_restore HİÇ çağrılmadı"; else kal "canlı veritabanı onaysız hedeflendi (kod=$kod cagri=$cagri)"; cat /tmp/o; fi

echo "── 8) geri-yukle: sha256 tutmazsa reddeder ──"
rm -f $sayac
printf 'bozuk' >> "$YEDEK"
PG_RESTORE_CMD=/calisma/sahte_restore BACKUP_AGE_KEY_FILE=/calisma/age.key \
  bash "$GERI" "$YEDEK" --hedef-db tatbikat >/tmp/o 2>&1
kod=$?; cagri=$([ -f $sayac ] && wc -l < $sayac || echo 0)
if [ $kod -ne 0 ] && grep -q "sha256 EŞLEŞMEDİ" /tmp/o && [ "$(echo $cagri|tr -d ' ')" = "0" ]; then gec "bozulmuş yedek geri yüklenmedi"; else kal "bozuk yedek geri yüklendi (kod=$kod cagri=$cagri)"; cat /tmp/o; fi

echo
echo "SONUÇ: $GECTI geçti, $KALDI kaldı"
[ "$KALDI" -eq 0 ]
