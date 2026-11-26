// Package mitm chuyển tiếp request của app Locket ra server thật.
package mitm

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// Upstream là reverse proxy dùng DNS công cộng để phân giải domain gốc.
// Cần thiết vì DoH của mình đã trỏ các domain này về VPS.
type Upstream struct {
	client      *http.Client
	upstreamDNS string
}

// NewUpstream tạo proxy với resolver riêng (không dùng DNS hệ thống đã bị đổi).
func NewUpstream(upstreamDNS string) *Upstream {
	if upstreamDNS == "" {
		upstreamDNS = "1.1.1.1:53"
	}
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{Timeout: 5 * time.Second}
			return d.DialContext(ctx, "udp", upstreamDNS)
		},
	}
	dialer := &net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second, Resolver: resolver}
	tr := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          64,
		MaxIdleConnsPerHost:   16,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ExpectContinueTimeout: 5 * time.Second,
		// Không nén lại: giữ nguyên body gốc để app không thấy khác biệt.
		DisableCompression: true,
	}
	return &Upstream{
		client:      &http.Client{Transport: tr, Timeout: 0},
		upstreamDNS: upstreamDNS,
	}
}

var hopHeaders = map[string]bool{
	"connection":          true,
	"proxy-connection":    true,
	"keep-alive":          true,
	"proxy-authenticate":  true,
	"proxy-authorization": true,
	"te":                  true,
	"trailer":             true,
	"transfer-encoding":   true,
	"upgrade":             true,
}

// Forward gửi nguyên request (method, header, body) tới server thật rồi trả nguyên phản hồi.
func (u *Upstream) Forward(w http.ResponseWriter, r *http.Request) {
	target := "https://" + r.Host + r.URL.RequestURI()
	req, err := http.NewRequestWithContext(r.Context(), r.Method, target, r.Body)
	if err != nil {
		http.Error(w, `{"error":"BAD_REQUEST"}`, http.StatusBadRequest)
		return
	}
	req.Host = r.Host
	req.ContentLength = r.ContentLength
	for k, vs := range r.Header {
		if hopHeaders[strings.ToLower(k)] {
			continue
		}
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	req.Header.Del("Accept-Encoding") // để transport tự xử lý nén

	resp, err := u.client.Do(req)
	if err != nil {
		http.Error(w, `{"error":"UPSTREAM_UNAVAILABLE"}`, http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for k, vs := range resp.Header {
		if hopHeaders[strings.ToLower(k)] {
			continue
		}
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// Healthy kiểm tra nhanh khả năng phân giải + kết nối ra ngoài.
func (u *Upstream) Healthy(ctx context.Context, host string) error {
	d := &net.Dialer{Timeout: 5 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", host+":443")
	if err != nil {
		return err
	}
	return conn.Close()
}
