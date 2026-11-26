// Package doh phục vụ truy vấn DNS qua HTTPS (RFC 8484) cho iPhone.
package doh

import (
	"encoding/base64"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/miekg/dns"
)

// Handler trả lời DoH: domain trong danh sách thì trỏ về IP của VPS,
// còn lại chuyển tiếp lên resolver công cộng.
type Handler struct {
	AnswerIP net.IP
	Mapped   func(host string) bool
	Upstream string
	Logger   *log.Logger
	Debug    bool

	client *dns.Client
}

// New tạo handler DoH.
func New(answerIP net.IP, mapped func(string) bool, upstream string, logger *log.Logger, debug bool) *Handler {
	if upstream == "" {
		upstream = "1.1.1.1:53"
	}
	return &Handler{
		AnswerIP: answerIP,
		Mapped:   mapped,
		Upstream: upstream,
		Logger:   logger,
		Debug:    debug,
		client:   &dns.Client{Net: "udp", Timeout: 5 * time.Second},
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw, err := readQuery(r)
	if err != nil {
		http.Error(w, "bad dns query", http.StatusBadRequest)
		return
	}
	req := new(dns.Msg)
	if err := req.Unpack(raw); err != nil {
		http.Error(w, "bad dns message", http.StatusBadRequest)
		return
	}
	if len(req.Question) == 0 {
		http.Error(w, "no question", http.StatusBadRequest)
		return
	}

	resp := h.answer(req)
	packed, err := resp.Pack()
	if err != nil {
		http.Error(w, "pack error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/dns-message")
	w.Header().Set("Cache-Control", "max-age=60")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Method == http.MethodGet {
		w.Header().Set("Content-Length", itoa(len(packed)))
	}
	_, _ = w.Write(packed)
}

func readQuery(r *http.Request) ([]byte, error) {
	switch r.Method {
	case http.MethodGet:
		q := r.URL.Query().Get("dns")
		if q == "" {
			return nil, io.ErrUnexpectedEOF
		}
		// base64url không padding (RFC 8484 §4.1), chấp nhận cả padding.
		if b, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(q, "=")); err == nil {
			return b, nil
		}
		return base64.URLEncoding.DecodeString(q)
	case http.MethodPost:
		if ct := r.Header.Get("Content-Type"); ct != "" && !strings.Contains(ct, "application/dns-message") {
			return nil, io.ErrUnexpectedEOF
		}
		return io.ReadAll(io.LimitReader(r.Body, 8192))
	default:
		return nil, io.ErrUnexpectedEOF
	}
}

func (h *Handler) answer(req *dns.Msg) *dns.Msg {
	q := req.Question[0]
	name := strings.ToLower(dns.Fqdn(q.Name))
	host := strings.TrimSuffix(name, ".")

	if h.Mapped(host) {
		resp := new(dns.Msg)
		resp.SetReply(req)
		resp.Authoritative = true
		resp.RecursionAvailable = true
		switch q.Qtype {
		case dns.TypeA:
			if v4 := h.AnswerIP.To4(); v4 != nil {
				resp.Answer = append(resp.Answer, &dns.A{
					Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
					A:   v4,
				})
			}
		case dns.TypeAAAA:
			// Cố tình không trả AAAA: iPhone sẽ dùng A record trỏ về VPS.
		case dns.TypeHTTPS:
			// Không quảng cáo SVCB/HTTPS để client không đi đường khác.
		}
		if h.Debug && h.Logger != nil {
			h.Logger.Printf("doh: %s type=%s -> %s (mapped)", host, dns.TypeToString[q.Qtype], h.AnswerIP)
		}
		return resp
	}

	out, _, err := h.client.Exchange(req, h.Upstream)
	if err != nil || out == nil {
		resp := new(dns.Msg)
		resp.SetRcode(req, dns.RcodeServerFailure)
		return resp
	}
	out.Id = req.Id
	if h.Debug && h.Logger != nil {
		h.Logger.Printf("doh: %s type=%s -> upstream (%d answers)", host, dns.TypeToString[q.Qtype], len(out.Answer))
	}
	return out
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
