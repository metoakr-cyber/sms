package herosms

// Sipariş yaşam döngüsü işlemleri.
//
// BURASI PARA HARCAYAN KOD. Her karar docs/provider-herosms.md §5–§6'ya
// dayanır; tahmin yoktur. Bir davranış spec'te tanımsızsa, kod o belirsizliği
// varsayımla kapatmaz — sağlayıcının gerçek yanıtına tepki verir.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ikmetrik/sms-platform/api/internal/port"
)

/* ═══════════════════ Esnek JSON tipleri ═══════════════════ */

// flexString hem string hem sayı olarak gelebilen alanları okur.
//
// Sağlayıcı spec'i tutarsız: `phone` şemada integer (79991234567) ama
// örneklerde maskeli string ("79********1"). Webhook'taki `id` alanı ise
// `required` listesinde AMA `properties` içinde TANIMSIZ — tipi bilinmiyor.
// Katı bir tip seçmek, ilk farklı yanıtta ayrıştırmayı çökertir.
type flexString string

func (f *flexString) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		*f = ""
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*f = flexString(s)
		return nil
	}
	// Sayı: float aracılığıyla geçirmeyiz — büyük tam sayılarda basamak kaybı olur.
	*f = flexString(strings.Trim(string(b), `"`))
	return nil
}

func (f flexString) String() string { return string(f) }

/* ═══════════════════ Yanıt şemaları ═══════════════════ */

type activation struct {
	ID               flexString `json:"id"`
	Status           int        `json:"status"`
	Phone            flexString `json:"phone"`
	Service          string     `json:"service"`
	Country          int        `json:"country"`
	CountryPhoneCode flexString `json:"countryPhoneCode"`
	Operator         string     `json:"operator"`
	Price            float64    `json:"price"`
	VerificationType string     `json:"verificationType"`
	Subtype          int        `json:"subtype"`
	CreatedAt        string     `json:"createdAt"`
	ExpiredAt        string     `json:"expiredAt"`
	OtpList          []otpItem  `json:"otpList"`
}

// otpItem üç farklı kaynaktan üç farklı isimle gelir.
//
// Alan adları NORMALİZE EDİLİR (CLAUDE.md değişmez #26):
//
//	smsCode|code → Code · smsText|text → Body · receivedAt|date → ReceivedAt
type otpItem struct {
	ID         flexString `json:"id"`
	Code       string     `json:"code"`
	SMSCode    string     `json:"smsCode"`
	Text       string     `json:"text"`
	SMSText    string     `json:"smsText"`
	Sender     string     `json:"sender"`
	PhoneFrom  string     `json:"phoneFrom"`
	ReceivedAt string     `json:"receivedAt"`
	Date       string     `json:"date"`
}

// normalize ham OTP kaydını normalize mesaja çevirir.
//
// 🔴 UYDURMA KİMLİK ÜRETİLMEZ. Sağlayıcı `id` vermezse `RemoteID` BOŞ kalır ve
// dedup kararı servis katmanındaki tek noktaya (order.hashMessage) düşer.
//
// Eskiden üç çağrı yeri üç farklı yedek anahtar uyduruyordu ve ikisi de
// bozuktu:
//
//   - `/otp/last` → "{id}:last". SABİT bir anahtar: kiralıkta 2., 3., 40.
//     mesaj aynı anahtarı alır, ON CONFLICT DO NOTHING onları sessizce yutar
//     ve kullanıcı YALNIZ İLK KODU görür.
//   - `ListActive` → "{id}:{dizi indeksi}". Dizi sırası sağlayıcı sözleşmesi
//     değil: sıra değişirse aynı mesaj her turda yeni bir satır olur.
//
// Üstelik ikisi aynı SMS için FARKLI anahtar üretiyordu; yoklama ve webhook
// teyidi arka arkaya koştuğunda tek mesaj iki satır oluyordu — bu, kiralıktan
// bağımsız olarak aktivasyonda da yaşanıyordu.
// test: ../../../service/order/rental_integration_test.go#TestSameMessageFromBothPathsIsStoredOnce
func (o otpItem) normalize() port.RemoteMessage {
	pick := func(a, b string) string {
		if a != "" {
			return a
		}
		return b
	}
	body := pick(o.SMSText, o.Text)
	code := pick(o.SMSCode, o.Code)
	if code == "" {
		// Sağlayıcının `code` alanına GÜVENMEYİZ: `required` değil ve nullable.
		// Kodu kendi desenimizle çıkarırız (FR-410/10).
		code = extractCode(body)
	}
	return port.RemoteMessage{
		RemoteID:   o.ID.String(),
		Code:       code,
		Body:       body,
		Sender:     pick(o.Sender, o.PhoneFrom),
		ReceivedAt: parseTime(pick(o.ReceivedAt, o.Date)),
	}
}

// codePattern SMS metnindeki doğrulama kodunu bulur.
//
// 4–8 haneli, rakamlardan (veya rakam-tire grubundan) oluşan ilk öbek.
// Telefon numaralarını yakalamamak için üst sınır 8 hanededir; daha uzun
// sayı dizileri kod değil, numara veya referanstır.
var codePattern = regexp.MustCompile(`\b(\d{4,8}|\d{3}-\d{3})\b`)

func extractCode(body string) string {
	m := codePattern.FindString(body)
	return strings.ReplaceAll(m, "-", "")
}

func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	// YALNIZ RFC 3339. Boşluklu biçimi kabul etmeyiz: sağlayıcı bir gün
	// "2026-09-08 10:00:00" gönderirse bunu sessizce yanlış saat dilimiyle
	// ayrıştırmaktansa sıfır zaman dönmek yeğdir.
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

/* ═══════════════════ Hata ayrıştırma ═══════════════════ */

type apiError struct {
	Title   string `json:"title"`
	Details string `json:"details"`
	Info    struct {
		MinActivationTime int             `json:"minActivationTime"`
		RetryAfterSeconds int             `json:"retry_after_seconds"`
		Min               float64         `json:"min"`
		Data              json.RawMessage `json:"data"`
	} `json:"info"`
}

// mapLifecycleError iptal/kapatma yanıtlarını alan hatalarına çevirir.
//
// ÜÇ AYRI DAVRANIŞ, tek bir "iptal başarısız" DEĞİL (docs/provider-herosms.md §6.1):
//
//	EARLY_CANCEL_DENIED      → geçici: minActivationTime kadar bekle, tekrar dene
//	FREE_CANCELLATION_EXPIRED→ kalıcı: iade hakkı yandı, bizim giderimiz
//	OTP_RECEIVED             → kalıcı: kod gelmiş, iptal edilemez
//	NEW_OTP_RECEIVED         → kod geldi ve YANITTA: kaydet, sonra Finish()
//
// Üçünü tek hataya indirirsek ya sonsuz yeniden deneme (kalıcı red) ya da
// kaybedilmiş iade (geçici red) olur.
//
// 409 VE 422 BİRLİKTE ele alınır: modern uçta iş kuralı redlerinin hangi kodla
// geldiği spec'te tanımsız (❓H13); yalnız birine bakmak sebebi kör eder.
func mapLifecycleError(status int, body []byte) error {
	var e apiError
	_ = json.Unmarshal(body, &e)
	title := strings.ToUpper(strings.TrimSpace(e.Title))

	switch status {
	case http.StatusUnauthorized:
		return fmt.Errorf("%w: %s", port.ErrProviderAuth, e.Title)
	case http.StatusNotFound:
		return port.ErrOrderNotFound
	}

	switch title {
	case "EARLY_CANCEL_DENIED":
		wait := time.Duration(e.Info.MinActivationTime) * time.Second
		if wait <= 0 {
			// Sağlayıcı süre vermezse spec'teki tipik değeri kullanırız ama
			// SABİT BİR BACKOFF YAZMAYIZ: sağlayıcı söylüyorsa o kazanır.
			wait = 120 * time.Second
		}
		return port.NewRetryAfter("iptal için erken", wait, port.ErrCancelDenied)

	case "FREE_CANCELLATION_EXPIRED", "OTP_RECEIVED", "ACTIVATION_NOT_ACTIVE":
		return fmt.Errorf("%w: %s", port.ErrCancelDenied, title)

	case "NEW_OTP_RECEIVED":
		var items []otpItem
		if len(e.Info.Data) > 0 {
			_ = json.Unmarshal(e.Info.Data, &items)
		}
		msgs := make([]port.RemoteMessage, 0, len(items))
		for _, it := range items {
			msgs = append(msgs, it.normalize())
		}
		return port.NewOTPArrived(msgs, port.ErrCancelDenied)
	}

	if status >= 500 {
		return fmt.Errorf("%w: HTTP %d %s", port.ErrUnavailable, status, e.Title)
	}
	return fmt.Errorf("%w: HTTP %d %s", port.ErrCancelDenied, status, e.Title)
}

/* ═══════════════════ HTTP yardımcıları ═══════════════════ */

func (p *Provider) do(ctx context.Context, c port.Creds, method, path string, body any) (int, []byte, error) {
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return 0, nil, fmt.Errorf("herosms: gövde kodlanamadı: %w", err)
		}
		rdr = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, baseOf(c)+modernPath+path, rdr)
	if err != nil {
		return 0, nil, fmt.Errorf("herosms: istek kurulamadı: %w", err)
	}
	req.Header.Set("Authorization", "ApiKey "+c.APIKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("%w: %s %s: %v", port.ErrUnavailable, method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	buf, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("%w: gövde okunamadı: %v", port.ErrUnavailable, err)
	}
	return resp.StatusCode, buf, nil
}

/* ═══════════════════ Purchase ═══════════════════ */

// Purchase numara satın alır.
//
// 🔴 BU ÇAĞRI YENİDEN DENENMEZ. İdempotent değildir: spec'te "idempot" kelimesi
// hiç geçmiyor ve tanımlı tek istek başlığı Authorization. Toplu uç olduğu için
// bir yeniden deneme 10 numaraya kadar çift alım yapabilir.
//
// test: internal/service/order/order_integration_test.go#TestMaxPriceIsAlwaysSent
// ⭐ maxPrice HER ÇAĞRIDA gönderilir (ADR-018): fiyat teklifin üstüne çıktıysa
// satın alma sağlayıcı sınırında engellenir.
// 🔴 fixedPrice GÖNDERİLMEZ (ADR-027): o bir tavan değil SABİT fiyattır —
// piyasa düşse bile tavanı öderiz.
func (p *Provider) Purchase(ctx context.Context, c port.Creds, cmd port.PurchaseCmd) (*port.PurchaseResult, error) {
	country, err := strconv.Atoi(cmd.CountryCode)
	if err != nil {
		return nil, fmt.Errorf("%w: ülke kodu sayısal olmalı: %q", port.ErrMappingMissing, cmd.CountryCode)
	}

	vt := string(cmd.VerificationType)
	if vt == "" {
		vt = string(port.VerifySMS)
	}
	operator := cmd.OperatorCode
	if operator == "" {
		operator = "any"
	}

	// maxPrice USD cinsinden ondalık. Mikro-dolardan çeviriyoruz; sağlayıcının
	// alt sınırı 0.0067 olduğu için 4 ondalık yeterli değil, 6 ile gönderiyoruz.
	maxPrice := float64(cmd.MaxCost.Minor()) / 1_000_000

	req := map[string]any{
		"service":          cmd.ServiceCode,
		"country":          country,
		"amount":           1,
		"operator":         operator,
		"maxPrice":         maxPrice,
		"verificationType": vt,
	}
	if cmd.DurationHours > 0 {
		req["duration"] = cmd.DurationHours
	}

	status, body, err := p.do(ctx, c, http.MethodPost, "/activations", req)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK && status != http.StatusCreated {
		return nil, mapPurchaseError(status, body)
	}

	// 🔴 YANIT HER ZAMAN DİZİDİR — amount:1 gönderilse bile.
	// len != 1 hatadır: aksi hâlde parası çekilmiş ama siparişe dönmemiş
	// numara oluşur.
	var out struct {
		Data []activation `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("herosms: satın alma yanıtı ayrıştırılamadı: %w", err)
	}
	if len(out.Data) != 1 {
		return nil, fmt.Errorf("%w: satın alma yanıtında %d kayıt var, 1 bekleniyordu",
			port.ErrUnavailable, len(out.Data))
	}
	a := out.Data[0]
	if a.ID.String() == "" {
		return nil, fmt.Errorf("%w: satın alma yanıtında aktivasyon kimliği yok", port.ErrUnavailable)
	}

	cost, err := usdFromFloat(a.Price)
	if err != nil {
		return nil, err
	}

	// TTL SAĞLAYICI YANITINDAN gelir, koda gömülmez (FR-405 / ADR).
	// expiredAt boş gelirse hata veririz: uydurulmuş bir süre, kullanıcının
	// numarası hâlâ canlıyken iade tetikler ya da tersi.
	exp := parseTime(a.ExpiredAt)
	if exp.IsZero() {
		return nil, fmt.Errorf("%w: satın alma yanıtında expiredAt yok — süre uydurulamaz",
			port.ErrUnavailable)
	}

	kind := port.KindSMSActivation
	if a.Subtype == 2 {
		kind = port.KindSMSRental
	}

	return &port.PurchaseResult{
		RemoteOrderID: a.ID.String(),
		PhoneNumber:   normalizePhone(a.Phone.String(), a.CountryPhoneCode.String()),
		CountryCode:   strconv.Itoa(a.Country),
		OperatorCode:  a.Operator,
		Cost:          cost,
		ExpiresAt:     exp,
		Subtype:       kind,
	}, nil
}

// mapPurchaseError satın alma hatalarını çevirir.
//
// 🔴 NO_NUMBERS ve NO_BALANCE'ın MODERN uçtaki karşılığı SPEC'TE TANIMSIZ
// (yalnız legacy'de 404/402 olarak belgeli). Bu yüzden hem HTTP koduna hem
// gövdedeki başlığa bakarız — ikisinden biri tutar.
func mapPurchaseError(status int, body []byte) error {
	var e apiError
	_ = json.Unmarshal(body, &e)
	title := strings.ToUpper(strings.TrimSpace(e.Title))

	switch title {
	case "NO_NUMBERS":
		return port.ErrOutOfStock
	case "NO_BALANCE":
		return port.ErrProviderNoBalance
	case "WRONG_MAX_PRICE":
		return fmt.Errorf("%w: sağlayıcı fiyatı %v", port.ErrPriceChanged, e.Info.Min)
	case "WRONG_SERVICE", "WRONG_COUNTRY", "WRONG_OPERATOR":
		return fmt.Errorf("%w: %s", port.ErrMappingMissing, title)
	case "BANNED":
		return port.NewBanned(port.BanSpecific, time.Time{}, port.ErrUnavailable)
	}

	switch status {
	case http.StatusUnauthorized:
		return fmt.Errorf("%w: %s", port.ErrProviderAuth, e.Title)
	case http.StatusPaymentRequired:
		return port.ErrProviderNoBalance
	case http.StatusNotFound:
		return port.ErrOutOfStock
	case http.StatusForbidden:
		return fmt.Errorf("%w: %s", port.ErrUnavailable, e.Title)
	case http.StatusUnprocessableEntity:
		// 422 hem eşleştirme hem fiyat hatası olabilir; başlık boşsa
		// eşleştirme varsayarız — o durumda ADMIN ALARMI üretilir ve
		// insan bakar. Fiyat hatası varsaymak sessiz zarar demektir.
		return fmt.Errorf("%w: %s", port.ErrMappingMissing, e.Title)
	case http.StatusTooManyRequests, http.StatusTooEarly:
		wait := time.Duration(e.Info.RetryAfterSeconds) * time.Second
		if wait <= 0 {
			wait = 30 * time.Second
		}
		return port.NewRetryAfter(title, wait, port.ErrUnavailable)
	}
	return fmt.Errorf("%w: HTTP %d %s", port.ErrUnavailable, status, e.Title)
}

// normalizePhone numarayı +<ülke><numara> biçimine getirir.
//
// Sağlayıcı numarayı işaretsiz gönderiyor ("79991234567"); kullanıcıya
// gösterirken + koyarız. Maskeli gelirse (79********1) olduğu gibi bırakılır —
// maskeyi "düzeltmeye" çalışmak uydurma numara üretmektir.
func normalizePhone(phone, countryCode string) string {
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return ""
	}
	if strings.HasPrefix(phone, "+") || strings.ContainsAny(phone, "*x") {
		return phone
	}
	_ = countryCode // numara zaten ülke kodunu içeriyor; ayrıca eklenmez
	return "+" + phone
}

/* ═══════════════════ GetStatus ═══════════════════ */

// GetStatus siparişin son durumunu ve mesajlarını döner.
//
// 🔴 `GET /activations/{id}` diye bir uç nokta YOKTUR — o yolda yalnız `delete`
// tanımlıdır. Teyit `/otp/last` ile yapılır (ADR-022).
func (p *Provider) GetStatus(ctx context.Context, c port.Creds, remoteOrderID string) (*port.RemoteStatus, error) {
	status, body, err := p.do(ctx, c, http.MethodGet,
		"/activations/"+url.PathEscape(remoteOrderID)+"/otp/last", nil)
	if err != nil {
		return nil, err
	}

	switch status {
	case http.StatusOK:
		// Tekil obje döner (dizi değil).
		var out struct {
			Data otpItem `json:"data"`
		}
		if err := json.Unmarshal(body, &out); err != nil {
			return nil, fmt.Errorf("herosms: otp/last ayrıştırılamadı: %w", err)
		}
		msg := out.Data.normalize()
		if msg.Body == "" && msg.Code == "" {
			// Teyit BOŞ döndü. Durum DEĞİŞTİRİLMEZ (ADR-022): sahte bir
			// webhook bu noktada COMPLETED yazdırmayı başaramamalı.
			return &port.RemoteStatus{State: port.StateWaiting}, nil
		}
		return &port.RemoteStatus{
			State:    port.StateCompleted,
			Messages: []port.RemoteMessage{msg},
		}, nil

	case http.StatusNoContent:
		return &port.RemoteStatus{State: port.StateWaiting}, nil

	case http.StatusNotFound:
		return nil, port.ErrOrderNotFound

	case http.StatusConflict:
		// ACTIVATION_NOT_ACTIVE: sipariş kapanmış, geçmiş mesaj okunamaz.
		// Bu YÜZDEN mesajları kendi tarafımızda kalıcı tutuyoruz.
		return nil, port.ErrOrderClosed
	}
	return nil, mapLifecycleError(status, body)
}

/* ═══════════════════ Cancel / Finish ═══════════════════ */

// Cancel siparişi iptal eder ve sağlayıcıdan İADE TALEP EDER.
//
// Finish() ile aynı şey DEĞİLDİR (ADR-023): Cancel iade ister, Finish istemez.
// Kod gelmemiş siparişte Cancel, gelmiş siparişte Finish çağrılır.
func (p *Provider) Cancel(ctx context.Context, c port.Creds, remoteOrderID string) error {
	status, body, err := p.do(ctx, c, http.MethodDelete,
		"/activations/"+url.PathEscape(remoteOrderID), nil)
	if err != nil {
		return err
	}
	// Başarı kodu 204'tür, 200 DEĞİL. 200 bekleyen kod siparişi sonsuz
	// yeniden denemeye sokar.
	if status == http.StatusNoContent || status == http.StatusOK {
		return nil
	}
	return mapLifecycleError(status, body)
}

// Finish siparişi kapatır — İADE TALEP ETMEZ.
//
// Kod teslim edilmiş siparişler için kullanılır. Kod gelmemiş bir siparişte
// çağrılırsa iade hakkından vazgeçmiş oluruz.
func (p *Provider) Finish(ctx context.Context, c port.Creds, remoteOrderID string) error {
	status, body, err := p.do(ctx, c, http.MethodPost,
		"/activations/"+url.PathEscape(remoteOrderID)+"/finish", nil)
	if err != nil {
		return err
	}
	if status == http.StatusNoContent || status == http.StatusOK {
		return nil
	}
	return mapLifecycleError(status, body)
}

/* ═══════════════════ Toplu yoklama ═══════════════════ */

var _ port.BatchPoller = (*Provider)(nil)

// maxPageSize sağlayıcının sayfa üst sınırı.
const maxPageSize = 25

// ListActive açık aktivasyonları TOPLU olarak sayfalı döner.
//
// Sipariş başına tekil sorgu YAPILMAZ: eski prototip her bekleyen sipariş için
// ayrı istek atıyordu ve sağlayıcıya giden yük kullanıcı sayısıyla orantılı
// büyüyordu. Bir turda 100 bekleyen sipariş = en fazla 4 istek (FR-404).
//
// test: status_test.go#TestUnknownProviderStatusStaysWaiting
func (p *Provider) ListActive(ctx context.Context, c port.Creds, cursor string, size int) (port.ActivePage, error) {
	if size <= 0 || size > maxPageSize {
		size = maxPageSize
	}
	page := 1
	if cursor != "" {
		n, err := strconv.Atoi(cursor)
		if err != nil || n < 1 {
			return port.ActivePage{}, fmt.Errorf("herosms: geçersiz sayfa imleci %q", cursor)
		}
		page = n
	}

	q := url.Values{"size": {strconv.Itoa(size)}, "page": {strconv.Itoa(page)}}
	body, err := p.modernGet(ctx, c, "/activations", q)
	if err != nil {
		return port.ActivePage{}, err
	}

	var out struct {
		Data []activation `json:"data"`
		Meta struct {
			HasMore bool `json:"hasMore"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return port.ActivePage{}, fmt.Errorf("herosms: activations ayrıştırılamadı: %w", err)
	}

	items := make([]port.ActiveOrder, 0, len(out.Data))
	for _, a := range out.Data {
		msgs := make([]port.RemoteMessage, 0, len(a.OtpList))
		for _, o := range a.OtpList {
			msgs = append(msgs, o.normalize())
		}
		items = append(items, port.ActiveOrder{
			RemoteOrderID: a.ID.String(),
			State:         mapProviderStatus(a.Status, len(msgs) > 0),
			Messages:      msgs,
		})
	}

	next := ""
	// hasMore YOKSA sayfa doluluğuna bakarız: meta alanı spec'te her yanıtta
	// garanti edilmiyor ve eksikliğini "son sayfa" saymak kalan siparişleri
	// sessizce yoklamasız bırakırdı.
	if out.Meta.HasMore || len(out.Data) == size {
		next = strconv.Itoa(page + 1)
	}
	return port.ActivePage{Items: items, NextCursor: next}, nil
}

// mapProviderStatus sağlayıcının sayısal durumunu bizim duruma çevirir.
//
// 🔴 SAVUNMACI EŞLEME (ADR-025). Sağlayıcının `status` değerleri
// [1,2,3,4,6,7,8,10] ve 1,2,3,4,7'nin anlamı SPEC'TE YOK
// (`x-enum-descriptions` alanı yok). Tahmin etmek yanlış para hareketi
// tetikler; bilinmeyen değer BEKLEMEDE bırakır.
//
// test: status_test.go#TestUnknownProviderStatusStaysWaiting
func mapProviderStatus(code int, hasMessages bool) port.RemoteOrderState {
	switch code {
	case 6: // belgeli: tamamlandı
		return port.StateCompleted
	case 8: // belgeli: iptal edildi
		return port.StateCancelled
	case 10: // belgeli: iade edildi
		return port.StateRefunded
	}
	// Bilinmeyen kod: mesaj varsa tamamlanmış sayarız (mesajın varlığı
	// gözlemlenebilir bir gerçektir, kod ise yorum), yoksa BEKLEMEDE kalır.
	if hasMessages {
		return port.StateCompleted
	}
	return port.StateWaiting
}
