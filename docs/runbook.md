# Çalışma kitabı

> Bu belge **arıza anında** okunur. Uzun açıklama yok; her bölüm
> **belirti → teşhis → müdahale → sonrasında** sırasıyla yazılmıştır.
>
> Tasarım gerekçeleri [design.md](design.md), kurulum [../deploy/README.md](../deploy/README.md).

Bütün komutlar üretim sunucusunda, `deploy/` dizininden çalıştırılır.
`api` kısaltması:

```bash
alias apix='docker compose -f deploy/docker-compose.prod.yml exec -T api'
```

---

## 0. Önce buraya bak — 60 saniyelik durum taraması

```bash
docker compose -f deploy/docker-compose.prod.yml ps
curl -sf https://ALANADI/readyz | jq .
docker compose -f deploy/docker-compose.prod.yml logs --since=15m api | grep -c 'level=ERROR'
```

`readyz` çıktısı `{"ready":true,"checks":{"postgres":"up","redis":"up"}}` değilse
doğrudan **§3 (Postgres)** veya **§4 (Redis)** bölümüne git.

---

## 1. Sağlayıcı çöktü — numara alınamıyor

**Belirti:** kullanıcılar "numara alınamadı" görüyor; log'da
`PROVIDER_UNAVAILABLE` veya `sağlayıcı zaman aşımı` artıyor.

**Teşhis**

```bash
apix /app/cli provider:list                     # sağlayıcı aktif mi, bakiye ne
docker compose -f deploy/docker-compose.prod.yml logs --since=30m api \
  | grep -E 'PROVIDER_(UNAVAILABLE|AUTH|TIMEOUT)' | tail -20
```

Üç ayrı sebep, üç ayrı müdahale:

| Log | Anlamı | Müdahale |
|---|---|---|
| `PROVIDER_AUTH` | API anahtarı geçersiz/iptal | Panelden anahtarı yenile (§1.1) |
| `PROVIDER_UNAVAILABLE` | Sağlayıcı ayakta değil | §1.2 |
| Bakiye 0'a yakın | Üst hesapta para bitti | Sağlayıcı panelinden yükle |

**§1.1 — API anahtarı yenileme.** Yönetim panelinden
*Sağlayıcılar → HeroSMS → API anahtarı*. Anahtar **yanıtta hiç dönmez**; yalnız
maskeli önizleme gösterilir. Anahtarı sohbete, log'a veya bu belgeye **yazma**.

**§1.2 — Sağlayıcı ayakta değil.** Yapılacak tek şey **sağlayıcıyı pasifleştirmek**:

```
Yönetim → Sağlayıcılar → HeroSMS → Aktif: kapat
```

Pasif sağlayıcıdan teklif üretilmez; kullanıcı "şu an numara yok" görür.
Bu, "satın al"a basıp parası düşülen ama numara gelmeyen bir kullanıcıdan
**iyidir**.

**Sonrasında:** açık `PENDING` siparişler kendiliğinden çözülür —
`order-expirer` süresi dolanları iptal edip **iadeyi otomatik yazar** (FR-405).
Elle iade yapma; çift iade riski var.

---

## 2. Kur bayat — fiyatlar yanlış veya teklif üretilmiyor

**Belirti:** log'da `kur bayat` / `FX_STALE`; teklif oluşturma hata veriyor.

Sistem **bilerek** bayat kurla satış yapmaz: maliyet USD, satış TL'dir; bayat
kur, zararına satış demektir.

**Teşhis ve müdahale**

```bash
apix /app/cli fx:sync                       # TCMB'den elle tazele
docker compose -f deploy/docker-compose.prod.yml logs --since=1h worker | grep -i 'kur'
```

`fx-sync` işi 10 dakikada bir koşar. Elle tazeleme de başarısızsa TCMB
erişilemiyordur (hafta sonu/tatilde **yeni kur yayımlanmaz** — bu normaldir,
`FX_MAX_AGE` bunu karşılayacak kadar geniş olmalıdır).

**Kalıcı çözüm gerekiyorsa:** `FX_MAX_AGE` değerini geçici olarak uzat, ama
**neden uzattığını ve ne zaman geri alacağını** yaz. Süresiz uzatılmış bir
tazelik sınırı, kur korumasının kapatılmasıdır.

---

## 3. Postgres erişilemiyor

**Belirti:** `readyz` → `postgres: down`; her istek 500.

```bash
docker compose -f deploy/docker-compose.prod.yml logs --tail=100 postgres
df -h                                        # disk dolu mu?
docker compose -f deploy/docker-compose.prod.yml restart postgres
```

**Disk doluysa → §6.**

🔴 **Yeniden başlatmak veriyi kurtarmazsa** `docker compose down -v` **YAPMA** —
`-v` veritabanı hacmini siler. Önce §7 (geri yükleme).

---

## 4. Redis erişilemiyor / kuyruk birikiyor

**Belirti:** oturum açılamıyor (oturumlar Redis'te), webhook bildirimleri
işlenmiyor, `sms_queue_depth` metriği artıyor.

```bash
docker compose -f deploy/docker-compose.prod.yml exec redis redis-cli -a "$REDIS_PASSWORD" ping
docker compose -f deploy/docker-compose.prod.yml exec redis redis-cli -a "$REDIS_PASSWORD" llen webhook:herosms
docker compose -f deploy/docker-compose.prod.yml logs --tail=50 redis
```

**`MISCONF ... No space left on device`** → §6. Bu hatada Redis **yazmayı
tümden reddeder**; oturum açılamaz, giriş 500 döner.

**Kuyruk uzunluğu sürekli artıyorsa** işçi süreci ölmüştür:

```bash
docker compose -f deploy/docker-compose.prod.yml ps worker
docker compose -f deploy/docker-compose.prod.yml restart worker
```

**Kaybolan bildirimler için endişelenme:** `order-poller` güvenlik ağıdır,
kodu en geç 30 saniyede sağlayıcıdan alır (FR-404).

---

## 5. 🔴 Mutabakat sapması — Σ defter ≠ bakiye

**Bu en ciddi alarmdır.** Bakiye, defterin sonucudur; ikisi ayrılmışsa bir
yerde defter dışı bir yazma olmuş demektir.

```bash
apix /app/cli wallet:reconcile
```

**Sapma varsa:**

1. **HİÇBİR ŞEYİ ELLE DÜZELTME.** `ledger_entries` üzerinde `UPDATE`/`DELETE`
   yoktur (değişmez #4); düzeltme ancak ters bir `ADJUSTMENT` kaydıdır.
2. Etkilenen kullanıcıları ve tutarı çıkar:

```sql
SELECT u.public_id, u.email, u.balance_minor,
       COALESCE(SUM(l.amount_minor), 0) AS defter,
       u.balance_minor - COALESCE(SUM(l.amount_minor), 0) AS sapma
FROM users u LEFT JOIN ledger_entries l ON l.user_id = u.id
GROUP BY u.id HAVING u.balance_minor <> COALESCE(SUM(l.amount_minor), 0);
```

3. Sapmanın **yönünü** belirle: bakiye defterden fazlaysa kullanıcıya fazla
   para verilmiş, azsa eksik. Her ikisi de düzeltilir ama **sebebi bulunmadan
   düzeltme sadece izi siler**.
4. Sebebi bul: o kullanıcının o zaman aralığındaki `audit_logs` ve `orders`
   kayıtlarına bak. `balance_minor`'a `WalletRepo` dışından yazan bir kod
   yolu varsa **asıl hata odur**.
5. Düzeltmeyi yönetim panelinden **bakiye düzeltme** ile yap (açıklama zorunlu,
   `ADJUSTMENT` tipiyle deftere yazılır, audit log'a düşer).

---

## 6. Disk doldu

**Belirti:** Postgres `No space left on device`, Redis `MISCONF`, konteynerler
yeniden başlıyor.

```bash
df -h
docker system df
du -sh /var/lib/docker/volumes/* 2>/dev/null | sort -h | tail -10
```

**Güvenli temizlik sırası** (üstten aşağı, her adımdan sonra `df -h`):

```bash
docker builder prune -af                     # derleme önbelleği — yeniden üretilir
docker image prune -f                        # başıboş katmanlar
docker compose -f deploy/docker-compose.prod.yml logs --tail=0 -f &  # log boyutu?
```

🔴 **`docker volume prune` ÇALIŞTIRMA.** Veritabanı hacmi bağlı değilse
(konteyner durmuşsa) o komut onu siler.

Log rotasyonu compose'da tanımlıdır; sürekli disk dolduran şey genelde
**eski yedekler**dir. `deploy/scripts/yedekle.sh` saklama süresini uygular;
elle kopyalanmış yedekleri kontrol et.

---

## 7. Yedekten geri yükleme

> Bu yordam **canlıda ilk kez denenmez.** Tatbikat için varsayılan hedef
> `smsplatform_tatbikat` veritabanıdır — betik canlıyı hedeflemeyi zorlaştırır.

**Tatbikat (ayda bir yapılmalı):**

```bash
deploy/scripts/geri-yukle.sh /yedek/smsplatform-2026-09-08.dump.age
apix /app/cli wallet:reconcile               # geri yüklenen veri tutarlı mı
```

**Gerçek felakette:**

```bash
docker compose -f deploy/docker-compose.prod.yml stop api worker
deploy/scripts/geri-yukle.sh --uretim-onayi /yedek/<dosya>.age
# betik veritabanı adını ELLE yazmanı ister — bu kasıtlıdır
docker compose -f deploy/docker-compose.prod.yml start api worker
apix /app/cli wallet:reconcile
```

**Sonrasında zorunlu:** yedek anındaki ile şimdiki arasında kaybolan
siparişler sağlayıcıda **açık kalmış** olabilir. `activation-reaper` onları
kapatır ama sağlayıcı panelinden de doğrula.

---

## 8. Webhook geliyor ama kod görünmüyor

**Teşhis sırası** (üç katmanın hangisinde durduğunu bul):

```bash
# 1. Bildirim bize ULAŞIYOR mu?
docker compose -f deploy/docker-compose.prod.yml logs --since=30m caddy | grep webhooks

# 2. İzin listesinden GEÇİYOR mu?
docker compose -f deploy/docker-compose.prod.yml logs --since=30m api \
  | grep 'izin listesi dışı'

# 3. Sağlayıcı TEYİDİ boş mu dönüyor?
docker compose -f deploy/docker-compose.prod.yml logs --since=30m worker \
  | grep 'provider_webhook_verify_failed_total'
```

| Durduğu yer | Sebep | Müdahale |
|---|---|---|
| 1'de hiç kayıt yok | Sağlayıcı panelinde webhook URL'i tanımlı değil | HeroSMS panelinden ekle |
| 2'de "izin listesi dışı" | `WEBHOOK_HEROSMS_ALLOWED_IPS` eksik **veya** `TRUSTED_PROXIES` yanlış | §8.1 |
| 3'te "sağlayıcıda kod YOK" | **Sahte webhook göstergesi** — veya sağlayıcı gecikmesi | §8.2 |

**§8.1 —** `TRUSTED_PROXIES`, Caddy konteynerinin ağ aralığını içermelidir.
İçermezse api vekil başlıklarına hiç bakmaz ve **her** bildirim elenir:

```bash
docker network inspect deploy_dis --format '{{range .IPAM.Config}}{{.Subnet}}{{end}}'
```

**§8.2 —** Bu metrik artıyorsa gizli webhook yolu sızmış olabilir. Sırrı
döndür (`WEBHOOK_HEROSMS_SECRET`) ve sağlayıcı panelindeki URL'i güncelle.
Kod hiç kaybolmaz: teyit boşsa durum **değiştirilmez** ve poller devreye girer.

---

## 9. TLS sertifikası yenilenmedi

**Belirti:** tarayıcı sertifika uyarısı; Caddy log'unda `obtain` hataları.

```bash
docker compose -f deploy/docker-compose.prod.yml logs --tail=100 caddy | grep -i 'certificate\|acme'
dig +short ALANADI                           # DNS hâlâ bu sunucuyu mu gösteriyor?
```

En sık iki sebep: **DNS değişmiş** ya da **80 portu kapalı** (ACME HTTP
doğrulaması oradan geçer). Güvenlik duvarını kontrol et:

```bash
ufw status
```

---

## 10. Geri alma (rollback)

```bash
# Hangi sürüm çalışıyor?
docker compose -f deploy/docker-compose.prod.yml images

# Önceki etikete dön
IMAGE_TAG=<önceki_git_sha> docker compose -f deploy/docker-compose.prod.yml up -d api worker web
```

🔴 **Migration geri alınmaz.** `goose down` üretimde çalıştırılmaz: veri
kaybettirebilir. Şema uyumsuzluğu varsa doğru yol **ileri düzeltmedir**
(yeni bir migration), geri alma değil.

---

## 11. Kimi ne zaman uyandırmalı

| Durum | Aciliyet |
|---|---|
| Mutabakat sapması | **Hemen** — para tutarsız |
| Ödeme/onay ucu 500 veriyor | **Hemen** — para alınıyor, yazılmıyor olabilir |
| Sağlayıcı çöktü | Yüksek — gelir duruyor ama para kaybı yok |
| Kur bayat | Orta — satış duruyor, zarar yok |
| Webhook gecikmesi | Düşük — poller güvenlik ağı çalışıyor |
| TLS uyarısı | Yüksek — kullanıcı siteye giremiyor |
