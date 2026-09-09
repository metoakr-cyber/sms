-- ═══════════════════════════ SAKLAMA POLİTİKASI ═══════════════════════════
--
-- Gizlilik metninde ilan edilen süreler BURADA uygulanır. Metin ile bu dosya
-- ayrı düştüğü an kullanıcıya verilmiş bir söz karşılıksız kalır: sayfa
-- "90 gün sonra sileriz" der, veri durur. Beş sorgu bu yüzden tek dosyada
-- toplandı — süreyi değiştiren kişi hepsini bir arada görsün.
--
-- SÜRELER (9 Eylül 2026 kararı):
--   SMS içeriği (order_messages)         90 gün          → gövde boşaltılır, satır durur
--   oturum (sessions)                    süresi dolunca  → satır silinir
--   tek kullanımlık token (auth_tokens)  süresi dolunca  → satır silinir
--   tüketilmemiş teklif (price_quotes)   süresi dolunca  → satır silinir
--   denetim kaydı (audit_logs)           2 yıl           → satır silinir
--
-- BU DOSYADA OLMAYANLAR: orders, deposits, ledger_entries. Mali kayıt 10 yıl
-- durur; `ledger_entries` ayrıca değişmezdir (migration 00004 tetikleyicisi).
-- Temizlik işi o tabloların hiçbirine uzanmaz.
-- test: retention_integration_test.go#TestRetentionLeavesLedgerAndOrdersUntouched
--
-- HER SORGU PARTİLİDİR (LIMIT). Tek ifadeyle milyonlarca satır silmek tabloyu
-- kilitler, WAL'i şişirir ve temizliği bir kesintiye çevirir. Çağıran
-- (internal/worker/jobs.go) parti tükenene kadar döner.
--
-- ZAMAN DIŞARIDAN GELİR (`older_than`), `now()` değil: sınır davranışı ancak
-- sahte saatle sınanabilir ve saat kaynağı tek yerde kalır (ClockPort).

-- name: RedactOldOrderMessages :execrows
-- SMS içeriğini boşaltır, SATIRI BIRAKIR (90 gün).
--
-- NEDEN SATIR DURUYOR: "kod geldi mi gelmedi mi" tartışmasının tek kanıtı bu
-- satırdır (handler/order.go visibleMessages: iade edilmiş siparişin mesajı
-- gösterilmez ama veritabanında durur). Sipariş kaydı 10 yıl saklanırken mesaj
-- satırını yok etmek o kaydı eksik bırakırdı: iade edilmiş bir siparişte
-- "aslında kod gelmişti" bilgisi de giderdi. KVKK'nın veri minimizasyonu
-- İÇERİĞİ ister, OLGUYU değil — metin gider, `received_at` kalır.
--
-- provider_otp_id DE DEĞİŞİR: sağlayıcı kendi mesaj kimliğini vermediğinde bu
-- alan SHA-256(activationId + receivedAt + text) olur, yani METİNDEN TÜRER.
-- Gövdeyi boşaltıp metinden türeyen özeti bırakmak "içerik silindi" sözünü
-- yarım bırakırdı. Yerine satırın kendi kimliğinden türeyen ve çakışmayan bir
-- işaret yazılır. Dedup değerinin 90 gün sonra karşılığı yoktur: kapatılmış
-- aktivasyon için sağlayıcı 409 döner, geçmiş mesaj yeniden gelmez.
--
-- İŞARET AYNI ZAMANDA SÜZGEÇTİR: ikinci koşu boşaltılmış satıra dokunmaz,
-- `!~ '^redacted:'` onu dışarıda tutar.
-- test: retention_integration_test.go#TestRetentionRedactionIsIdempotent
--
-- ÖLÇÜT `created_at`, `received_at` DEĞİL: söz verdiğimiz şey BİZİM saklama
-- süremizdir. `received_at` sağlayıcının bildirdiği andır; geçmişe kaçık bir
-- değer satırı vaktinden önce, geleceğe kaçık bir değer hiç boşaltmazdı.
UPDATE order_messages
SET code = '', body = '', sender = '', provider_otp_id = 'redacted:' || id
WHERE id IN (
    SELECT id FROM order_messages
    WHERE created_at < sqlc.arg('older_than')::timestamptz
      AND provider_otp_id !~ '^redacted:'
    ORDER BY id
    LIMIT sqlc.arg('lim')
);

-- name: DeleteExpiredSessions :execrows
-- Süresi dolmuş oturum satırlarını siler.
--
-- 7 GÜNLÜK PAY KALDIRILDI: sorgu eskiden `expires_at < now() - interval '7 days'`
-- diyordu ve hiçbir yerden çağrılmıyordu. İlan edilen süre "oturum süresi
-- dolunca" olduğuna göre payın karşılığı yok. Süresi dolmuş satırın işlevi
-- zaten bitmiştir (GetSession `expires_at > now()` arar); geriye yalnız IP ve
-- tarayıcı imzası, yani kişisel veri kalır.
--
-- ÇIKIŞ YAPILMIŞ (revoked_at dolu) ama süresi DOLMAMIŞ oturum burada durur;
-- süresi geldiğinde aynı sorguya düşer. `sessions_expires_at_idx` alt sorguyu
-- indeksten karşılar.
DELETE FROM sessions
WHERE id IN (
    SELECT id FROM sessions
    WHERE expires_at < sqlc.arg('older_than')::timestamptz
    ORDER BY expires_at
    LIMIT sqlc.arg('lim')
);

-- name: DeleteExpiredAuthTokens :execrows
-- Süresi dolmuş e-posta doğrulama / şifre sıfırlama token'larını siler.
--
-- Satır ham token taşımaz, yalnız SHA-256 özeti (migration 00001) — ama
-- kullanıcı bağı ve zaman damgası kişisel veridir, süresi dolmuş token'ın
-- işlevi ise yoktur (ConsumeAuthToken `expires_at > now()` arar).
--
-- `expires_at` üzerinde indeks YOK: satır sayısı doğrulama/sıfırlama isteği
-- kadardır (ölçüm: 27) ve iş saatte bir koşar. İndeks eklemek her token
-- yazımına maliyet bindirir, kazancı ise bugün ölçülemez.
DELETE FROM auth_tokens
WHERE id IN (
    SELECT id FROM auth_tokens
    WHERE expires_at < sqlc.arg('older_than')::timestamptz
    ORDER BY id
    LIMIT sqlc.arg('lim')
);

-- name: DeleteExpiredQuotes :execrows
-- Tüketilmemiş ve süresi geçmiş fiyat tekliflerini siler.
--
-- TÜKETİLMİŞ TEKLİF BU SORGUYA DÜŞMEZ (`consumed_at IS NULL` süzgeci):
-- tüketilmiş teklif bir siparişin fiyat kanıtıdır (`orders.quote_id`) ve
-- sipariş kaydıyla aynı süre durur. Ayrıca yetim provizyon işi (FR-408) tam
-- olarak o satırları tarar; silinseydi kurtarılamayan para kalırdı.
-- test: retention_integration_test.go#TestRetentionKeepsConsumedQuotes
DELETE FROM price_quotes
WHERE id IN (
    SELECT id FROM price_quotes
    WHERE consumed_at IS NULL
      AND expires_at < sqlc.arg('older_than')::timestamptz
    ORDER BY id
    LIMIT sqlc.arg('lim')
);

-- name: DeleteOldAuditLogs :execrows
-- 2 yıldan eski denetim kayıtlarını siler.
--
-- Denetim kaydı "kim, ne zaman, neyi değiştirdi"yi tutar (FR-705) ve IP,
-- tarayıcı imzası, eski/yeni değerler gibi kişisel veri taşır. Ticari
-- zorunluluk kaydın süresiz durmasını gerektirmiyor; ilan edilen süre 2 yıl.
--
-- `created_at` tek başına indeksli değil (mevcut indeksler entity ve aktör
-- eksenli), alt sorgu tarama yapar. Tablo yönetici işlemleriyle sınırlı
-- büyüdüğü için saatlik tarama bugün ucuzdur; hacim büyürse (created_at)
-- indeksi eklenmelidir.
DELETE FROM audit_logs
WHERE id IN (
    SELECT id FROM audit_logs
    WHERE created_at < sqlc.arg('older_than')::timestamptz
    ORDER BY id
    LIMIT sqlc.arg('lim')
);
