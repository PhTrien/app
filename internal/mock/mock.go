// Package mock giả lập phản hồi RevenueCat (Gold) và Firebase Remote Config.
package mock

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

const farFuture = "9999-01-09T10:10:14Z"
const past = "2005-01-09T10:10:14Z"

// goldProduct là mã sản phẩm Gold mà Locket dùng trong bản hiện tại.
const goldProduct = "locket_1600_1y"

// Gold trả về JSON RevenueCat với entitlement Gold còn hạn rất lâu.
func Gold(w http.ResponseWriter, r *http.Request) {
	GoldUntil(w, r, subscriberID(r.URL.Path), time.Time{})
}

// GoldUntil trả metadata Gold với hạn cụ thể (zero = vĩnh viễn).
func GoldUntil(w http.ResponseWriter, r *http.Request, userID string, exp time.Time) {
	if userID == "" {
		userID = subscriberID(r.URL.Path)
	}
	body := map[string]any{
		"request_date":    time.Now().UTC().Format(time.RFC3339),
		"request_date_ms": time.Now().UnixMilli(),
		"subscriber":      subscriberPayload(userID, exp),
	}
	writeJSON(w, body)
}

// Receipt trả về cùng cấu trúc subscriber cho endpoint /v1/receipts.
func Receipt(w http.ResponseWriter, r *http.Request) {
	var in struct {
		AppUserID string `json:"app_user_id"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in)
	}
	writeJSON(w, map[string]any{"subscriber": subscriberPayload(in.AppUserID, time.Time{})})
}

// ReceiptFor trả subscriber Gold cho một app_user_id với hạn cụ thể.
func ReceiptFor(w http.ResponseWriter, appUserID string, exp time.Time) {
	writeJSON(w, map[string]any{"subscriber": subscriberPayload(appUserID, exp)})
}

// subscriberID lấy app_user_id từ đường dẫn /v1/subscribers/<id>.
func subscriberID(path string) string {
	i := strings.Index(path, "/subscribers/")
	if i < 0 {
		return ""
	}
	id := path[i+len("/subscribers/"):]
	if j := strings.Index(id, "/"); j >= 0 {
		id = id[:j]
	}
	return strings.Trim(id, "/")
}

func subscriberPayload(userID string, exp time.Time) map[string]any {
	if userID == "" {
		userID = "$RCAnonymousID:locket-gold"
	}
	expires := farFuture
	if !exp.IsZero() {
		expires = exp.UTC().Format(time.RFC3339)
	}
	subscriptions := map[string]any{
		goldProduct: map[string]any{
			"auto_resume_date":           nil,
			"billing_issues_detected_at": nil,
			"display_name":               goldProduct,
			"expires_date":               expires,
			"grace_period_expires_date":  nil,
			"is_sandbox":                 false,
			"management_url":             "https://apps.apple.com/account/subscriptions",
			"original_purchase_date":     past,
			"ownership_type":             "PURCHASED",
			"period_type":                "normal",
			"price":                      map[string]any{"amount": 0, "currency": "VND"},
			"product_identifier":         goldProduct,
			"purchase_date":              past,
			"refunded_at":                nil,
			"store":                      "app_store",
			"store_transaction_id":       "2000001108724193",
			"unsubscribe_detected_at":    nil,
		},
	}
	entitlements := map[string]any{
		"Gold": map[string]any{
			"expires_date":              expires,
			"grace_period_expires_date": nil,
			"product_identifier":        goldProduct,
			"purchase_date":             past,
		},
	}
	return map[string]any{
		"entitlements":                 entitlements,
		"first_seen":                   past,
		"last_seen":                    time.Now().UTC().Format(time.RFC3339),
		"management_url":               nil,
		"non_subscriptions":            map[string]any{},
		"original_app_user_id":         userID,
		"original_application_version": nil,
		"original_purchase_date":       past,
		"other_purchases":              map[string]any{},
		"subscriptions":                subscriptions,
	}
}

// RemoteConfig phục vụ cấu hình Firebase đã mở khoá (Gold, video 60s, filter...).
type RemoteConfig struct {
	Entries map[string]any
	Version string
	// Full là payload đã bóc từ module (mở khoá toàn bộ).
	Full map[string]any
	// Raw trả nguyên phản hồi thật khi client chưa được xác thực Gold.
	PassHandler http.HandlerFunc
}

// NewRemoteConfig nạp entries từ JSON đã trích xuất từ repo mhale.
func NewRemoteConfig(entries map[string]any) *RemoteConfig {
	return &RemoteConfig{
		Entries: entries,
		Version: "locket-gold-" + time.Now().UTC().Format("20060102"),
	}
}

// ServeHTTP trả về payload Firebase Remote Config.
func (rc *RemoteConfig) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body := map[string]any{
		"entries":                rc.Entries,
		"state":                  "UPDATE_AVAILABLE",
		"templateVersion":        rc.Version,
		"experimentDescriptions": []any{},
	}
	writeJSON(w, body)
}

func writeJSON(w http.ResponseWriter, v any) {
	raw, err := json.Marshal(v)
	if err != nil {
		http.Error(w, `{"error":"encode"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", itoa(len(raw)))
	_, _ = w.Write(raw)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [12]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
