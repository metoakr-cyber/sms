package handler

// Sağlayıcı webhook'u — FR-410.
//
// test: ../../../service/order/webhook_integration_test.go#TestFakeWebhookCannotCompleteOrder
//
// 🔴 WEBHOOK GÖVDESİNE ASLA GÜVENİLMEZ.
//
// HeroSMS imza (HMAC) doğrulaması SUNMUYOR — spec'te hiçbir imza başlığı yok.
// URL'i bilen herkes "kod geldi" bildirimi gönderebilir. Bu yüzden webhook bir
// VERİ KAYNAĞI değil, yalnız bir TETİKLEYİCİDİR: gövdedeki `code` alanı
// kullanılmaz, kod sağlayıcıdan `GET /activations/{id}/otp/last` ile teyit
// edilerek alınır (ADR-022).
//
// ÜÇ KATMANLI SAVUNMA:
//  1. Tahmin edilemez yol (128 bit, ortam değişkeninden)
//  2. Kaynak IP izin listesi (aşağıya bakın — başlığa körlemesine güvenilmez)
//  3. Sağlayıcıdan teyit (gövdeye güvenmeme)
//
// Üçü de tek başına yetersiz; birlikte anlamlı.

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// webhookBudget sağlayıcının yanıt zaman aşımı.
//
// 3 SANİYE. Aşılırsa sağlayıcı "en az 7 kez", 20–30 saniye arayla, 3 dakika
// boyunca yeniden dener ve aynı SMS sekiz kez işlenir. Bu yüzden handler'da
// veritabanı yazımı, sağlayıcı teyidi veya Redis yayını YAPILMAZ — hepsi
// kuyruğa gider.
//
// test: webhook_test.go#TestHandlerDoesNoWorkBeyondEnqueue
const webhookBudget = 3 * time.Second

// maxWebhookBody gövde üst sınırı.
//
// Kimliksiz bir uç: sınırsız okumak, tek bir istekle belleği tüketme
// vektörüdür.
const maxWebhookBody = 64 << 10 // 64 KiB

// WebhookQueue ham webhook gövdesini asenkron işleme kuyruğuna alır.
type WebhookQueue interface {
	Enqueue(ctx context.Context, raw []byte) error
}

// Webhook sağlayıcı bildirimlerini alır.
type Webhook struct {
	queue      WebhookQueue
	secret     string
	allowedIPs []net.IP
	// trusted GÜVENİLEN ters vekillerin ağ aralıkları. Vekil başlıklarına
	// YALNIZ bu aralıklardan gelen bir bağlantıda bakılır.
	trusted []*net.IPNet
	r       Responder
}

func NewWebhook(q WebhookQueue, secret string, allowedIPs, trustedProxies []string, r Responder) *Webhook {
	ips := make([]net.IP, 0, len(allowedIPs))
	for _, s := range allowedIPs {
		if ip := net.ParseIP(s); ip != nil {
			ips = append(ips, ip)
		} else {
			slog.Error("webhook izin listesinde geçersiz IP — YOK SAYILDI", "value", s)
		}
	}
	nets := make([]*net.IPNet, 0, len(trustedProxies))
	for _, s := range trustedProxies {
		if _, n, err := net.ParseCIDR(s); err == nil {
			nets = append(nets, n)
		} else {
			slog.Error("güvenilen vekil listesinde geçersiz CIDR — YOK SAYILDI", "value", s)
		}
	}
	return &Webhook{queue: q, secret: secret, allowedIPs: ips, trusted: nets, r: r}
}

// HeroSMS POST /webhooks/herosms/:secret
//
// 🔴 HER ZAMAN 200 DÖNER — hata durumlarında bile.
//
// Ayrıştırma hatası, bilinmeyen aktivasyon, kuyruk arızası: hepsinde 200.
// 200 dışı bir yanıt sağlayıcıyı yeniden deneme fırtınasına sokar ve aynı
// bildirim dakikalarca tekrar gelir. Bizim hatamızı sağlayıcıya yük olarak
// geri vermeyiz.
func (h *Webhook) HeroSMS(c *gin.Context) {
	// (a) GİZLİ YOL.
	//
	// Sabit zamanlı karşılaştırma GEREKMEZ: yol segmenti bir kimlik doğrulama
	// sırrı değil, tahmin edilemezlik katmanıdır ve zamanlama saldırısıyla
	// 128 bit çıkarmak pratikte mümkün değil. Yine de eşleşmezse HİÇBİR
	// bilgi vermeyiz.
	if h.secret == "" || c.Param("secret") != h.secret {
		// 404: "yanlış sır" demek, doğru sırrın var olduğunu söylemektir.
		c.Status(http.StatusNotFound)
		return
	}

	// (b) KAYNAK IP.
	if !h.ipAllowed(h.clientIP(c)) {
		slog.Warn("webhook izin listesi dışı IP'den geldi — işlenmedi",
			"remote", c.Request.RemoteAddr)
		// Yine 200: saldırgana izin listesinin varlığını sızdırmayız ve
		// meşru bir IP değişikliğinde sağlayıcıyı fırtınaya sokmayız.
		c.Status(http.StatusOK)
		return
	}

	// (c) GÖVDEYİ AL, KUYRUĞA AT, HEMEN DÖN.
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxWebhookBody))
	if err != nil {
		slog.Warn("webhook gövdesi okunamadı", "err", err)
		c.Status(http.StatusOK)
		return
	}
	if len(body) == 0 {
		c.Status(http.StatusOK)
		return
	}
	// Geçerli JSON mu — kuyruğa çöp atmayalım. Ayrıştırma SONUCU
	// yalnız biçim kontrolü içindir.
	if !json.Valid(body) {
		slog.Warn("webhook gövdesi geçerli JSON değil", "size", len(body))
		c.Status(http.StatusOK)
		return
	}

	// Kuyruğa alma isteğin bağlamından KOPARILIR: istemci bağlantıyı kesse
	// bile bildirim işlenmeli.
	qctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), webhookBudget)
	defer cancel()

	if err := h.queue.Enqueue(qctx, body); err != nil {
		// Kuyruk arızası: bildirim KAYBOLUR. Sessiz kalmayız — ama yine 200
		// döneriz, çünkü sağlayıcının tekrar denemesi kuyruğu düzeltmez.
		// Güvenlik ağı `order-poller`dır: kod en geç 30 saniyede gelir.
		slog.Error("webhook kuyruğa alınamadı — yoklama devreye girecek", "err", err)
	}
	c.Status(http.StatusOK)
}

// clientIP bildirimi GERÇEKTEN gönderen adresi çözer.
//
// 🔴 BAŞLIĞA KOŞULSUZ GÜVENİLMEZ, KOŞULSUZ YOK DA SAYILMAZ. İki uç da yanlış:
//
//   - Başlığa her zaman bakmak: `X-Forwarded-For` istemci tarafından
//     uydurulabilir. Herkes izin listesindeki bir IP'yi yazıp geçerdi.
//   - Hiç bakmamak: ters vekil arkasında peer HER ZAMAN vekildir (Docker'da
//     Caddy ayrı bir konteyner). Sağlayıcının gerçek IP'si görülemez ve
//     her meşru bildirim elenir — sistem "çalışıyor" görünür, kod hiç gelmez.
//
// Doğru kural: başlığa YALNIZ bağlantı güvenilen bir vekilden geliyorsa bakılır.
// Vekil zincirinde sağdan sola yürünür ve güvenilmeyen İLK adres alınır;
// saldırganın gövdeye eklediği sahte adresler o noktanın solunda kalır.
//
// test: webhook_test.go#TestForwardedForHeaderIsIgnored
// test: webhook_test.go#TestTrustedProxyHeaderIsHonored
// test: webhook_test.go#TestForgedChainStopsAtFirstUntrusted
func (h *Webhook) clientIP(c *gin.Context) net.IP {
	peer := parseHost(c.Request.RemoteAddr)
	if peer == nil || !h.isTrustedProxy(peer) {
		return peer
	}
	// Sağdan sola: en sağdaki, güvenilen vekilin kendi eklediği adrestir.
	chain := c.Request.Header.Values("X-Forwarded-For")
	var parts []string
	for _, v := range chain {
		for _, p := range strings.Split(v, ",") {
			parts = append(parts, strings.TrimSpace(p))
		}
	}
	for i := len(parts) - 1; i >= 0; i-- {
		if ip := net.ParseIP(parts[i]); ip != nil && !h.isTrustedProxy(ip) {
			return ip
		}
	}
	// Zincir yoksa vekilin yazdığı tek adres.
	if ip := net.ParseIP(strings.TrimSpace(c.Request.Header.Get("X-Real-IP"))); ip != nil {
		return ip
	}
	return peer
}

func (h *Webhook) isTrustedProxy(ip net.IP) bool {
	for _, n := range h.trusted {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

func parseHost(remoteAddr string) net.IP {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	return net.ParseIP(host)
}

// ipAllowed kaynak adresin izin listesinde olup olmadığını söyler.
//
// İzin listesi BOŞSA hiçbir istek kabul edilmez. "Boşsa hepsine izin ver"
// davranışı, yapılandırma unutulduğunda ucu herkese açardı.
func (h *Webhook) ipAllowed(ip net.IP) bool {
	if len(h.allowedIPs) == 0 || ip == nil {
		return false
	}
	for _, allowed := range h.allowedIPs {
		// IPv4/IPv6 eşdeğerliği: 1.2.3.4 ile ::ffff:1.2.3.4 aynı adrestir.
		if allowed.Equal(ip) {
			return true
		}
	}
	return false
}
