// Package httpapi gom toàn bộ endpoint công khai và endpoint MITM của server.
package httpapi

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/PhTrien/app/internal/caption"
	"github.com/PhTrien/app/internal/catalog"
	"github.com/PhTrien/app/internal/config"
	"github.com/PhTrien/app/internal/doh"
	"github.com/PhTrien/app/internal/filters"
	"github.com/PhTrien/app/internal/mitm"
	"github.com/PhTrien/app/internal/mock"
	"github.com/PhTrien/app/internal/pki"
)

// Server là http.Handler gốc, phân nhánh theo Host.
type Server struct {
	Cfg      *config.Config
	CA       *pki.Authority
	Catalog  *catalog.Catalog
	Engine   *filters.Engine
	Caption  *caption.Generator
	RC       *mock.RemoteConfig
	DoH      *doh.Handler
	Upstream *mitm.Upstream
	Profile  []byte
	Logger   *log.Logger
	Version  string
	Started  time.Time
}

// ServeHTTP chọn nhánh xử lý theo tên miền trong request.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host := strings.ToLower(stripPort(r.Host))
	if s.Cfg.LogRequests {
		s.logf("%s %s%s", host, r.Method, r.URL.Path)
	}
	if s.Cfg.IsMapped(host) {
		s.serveMITM(w, r, host)
		return
	}
	s.servePublic(w, r)
}

func (s *Server) servePublic(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/dns-query":
		s.DoH.ServeHTTP(w, r)
	case r.URL.Path == "/healthz":
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "uptime": time.Since(s.Started).String()})
	case r.URL.Path == "/version":
		writeJSON(w, http.StatusOK, s.versionInfo())
	case r.URL.Path == "/ca.crt":
		w.Header().Set("Content-Type", "application/x-x509-ca-cert")
		w.Header().Set("Content-Disposition", `attachment; filename="locket-gold-ca.crt"`)
		_, _ = w.Write(s.CA.CertPEM)
	case r.URL.Path == "/p/locket.mobileconfig", r.URL.Path == "/locket.mobileconfig":
		w.Header().Set("Content-Type", "application/x-apple-aspen-config; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="Locket-Gold.mobileconfig"`)
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(s.Profile)
	case strings.HasPrefix(r.URL.Path, "/locket-filters/"):
		s.serveRendered(w, r)
	case isFilterPath(r.URL.Path):
		s.handleFilter(w, r)
	case isCaptionPath(r.URL.Path):
		s.handleCaption(w, r)
	case r.URL.Path == "/" || r.URL.Path == "/index.html":
		s.serveLanding(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) serveMITM(w http.ResponseWriter, r *http.Request, host string) {
	switch {
	case host == "api.revenuecat.com":
		if strings.Contains(r.URL.Path, "receipts") {
			mock.Receipt(w, r)
			return
		}
		mock.Gold(w, r)
	case host == "firebaseremoteconfig.googleapis.com":
		s.RC.ServeHTTP(w, r)
	case isFilterPath(r.URL.Path):
		s.handleFilter(w, r)
	case isCaptionPath(r.URL.Path):
		s.handleCaption(w, r)
	default:
		// Mọi request khác (feed, upload, ảnh...) đi thẳng ra server thật.
		s.Upstream.Forward(w, r)
	}
}

func isFilterPath(p string) bool {
	return strings.HasSuffix(p, "/generateFilteredImage") || p == "/generateFilteredImage"
}

func isCaptionPath(p string) bool {
	return strings.HasSuffix(p, "/generateCaption") || p == "/generateCaption"
}

// ---- generateFilteredImage ----

type locketRequest struct {
	Data map[string]any `json:"data"`
}

func (s *Server) handleFilter(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorBody("METHOD_NOT_ALLOWED", "chỉ nhận POST"))
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 48<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody("BODY_READ_FAILED", err.Error()))
		return
	}
	var payload locketRequest
	if err := json.Unmarshal(raw, &payload); err != nil || payload.Data == nil {
		writeJSON(w, http.StatusBadRequest, errorBody("INVALID_NATIVE_REQUEST", "body không đúng schema"))
		return
	}

	images := pickImages(payload.Data)
	filterID := pickString(payload.Data, "filter_id", "filterId", "filter", "photo_effect", "photoEffect", "effect_id", "overlay_id", "uid")
	prompt := pickString(payload.Data, "prompt", "caption", "text")
	if filterID == "" {
		filterID = guessFilterFromPrompt(prompt)
	}

	ctx, cancel := contextWithTimeout(r, 120*time.Second)
	defer cancel()

	res, err := s.Engine.Render(ctx, filters.Input{
		FilterID:  filterID,
		ImageURLs: images,
		Prompt:    prompt,
		UserKey:   userKey(r),
	})
	if err != nil {
		status := "FILTER_BACKEND_ERROR"
		code := http.StatusBadGateway
		if strings.Contains(err.Error(), "IMAGE_HOST_DENIED") {
			status, code = "IMAGE_HOST_DENIED", http.StatusForbidden
		}
		if strings.Contains(err.Error(), "SOURCE_IMAGE_MISSING") {
			status, code = "SOURCE_IMAGE_MISSING", http.StatusBadRequest
		}
		s.logf("filter lỗi: %s (%v)", filterID, err)
		writeJSON(w, code, errorBody(status, err.Error()))
		return
	}
	if res.Pending {
		writeJSON(w, http.StatusAccepted, map[string]any{
			"status": "waiting",
			"result": map[string]any{"data": map[string]any{
				"phase":    "pending",
				"captured": res.Captured,
				"total":    res.Total,
			}},
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"result": map[string]any{"data": map[string]any{"output_url": res.URL}},
	})
}

// ---- generateCaption ----

func (s *Server) handleCaption(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorBody("METHOD_NOT_ALLOWED", "chỉ nhận POST"))
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 32<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody("BODY_READ_FAILED", err.Error()))
		return
	}
	var payload locketRequest
	if err := json.Unmarshal(raw, &payload); err != nil || payload.Data == nil {
		writeJSON(w, http.StatusBadRequest, errorBody("INVALID_NATIVE_REQUEST", "body không đúng schema"))
		return
	}
	img := pickString(payload.Data, "image_base64", "imageBase64", "image", "photo", "base64")
	prompt := pickString(payload.Data, "prompt", "text", "hint", "mood")
	if img == "" {
		writeJSON(w, http.StatusBadRequest, errorBody("CAPTION_IMAGE_MISSING", "thiếu ảnh"))
		return
	}
	text := s.Caption.Generate(img, prompt)
	writeJSON(w, http.StatusOK, map[string]any{
		"result": map[string]any{
			"caption": text,
			"data":    map[string]any{"caption": text},
		},
	})
}

// ---- tiện ích ----

func pickImages(data map[string]any) []string {
	keys := []string{"image_url", "imageUrl", "source_image_url", "sourceImageUrl"}
	var out []string
	for _, k := range keys {
		if v, ok := data[k].(string); ok && v != "" {
			out = append(out, v)
		}
	}
	for _, k := range []string{"image_urls", "imageUrls", "images", "photos"} {
		v, ok := data[k]
		if !ok {
			continue
		}
		switch vv := v.(type) {
		case []any:
			for _, item := range vv {
				switch it := item.(type) {
				case string:
					if it != "" {
						out = append(out, it)
					}
				case map[string]any:
					for _, key := range []string{"image_url", "url", "imageUrl", "download_url"} {
						if sv, ok := it[key].(string); ok && sv != "" {
							out = append(out, sv)
							break
						}
					}
				}
			}
		case string:
			if vv != "" {
				out = append(out, vv)
			}
		}
	}
	return out
}

func pickString(data map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := data[k].(string); ok {
			if s := strings.TrimSpace(v); s != "" {
				return s
			}
		}
	}
	return ""
}

func guessFilterFromPrompt(prompt string) string {
	p := strings.ToLower(prompt)
	cases := map[string]string{
		"photobooth": "pb_khung4_soulmate",
		"khung":      "pb_khung4_film",
		"film":       "photo_effect_film",
		"retro":      "photo_effect_retro",
		"anime":      "photo_effect_anime",
		"glow":       "photo_effect_glow",
	}
	for k, v := range cases {
		if strings.Contains(p, k) {
			return v
		}
	}
	return ""
}

func userKey(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return "anon"
	}
	sum := sha1.Sum([]byte(auth))
	return hex.EncodeToString(sum[:8])
}

func stripPort(host string) string {
	if i := strings.LastIndex(host, ":"); i > 0 && strings.Count(host, ":") == 1 {
		return host[:i]
	}
	if strings.HasPrefix(host, "[") {
		if i := strings.Index(host, "]"); i > 0 {
			return host[1:i]
		}
	}
	return host
}

func (s *Server) versionInfo() map[string]any {
	return map[string]any{
		"name":         "locket-gold",
		"version":      s.Version,
		"host":         s.Cfg.Host,
		"server_ip":    s.Cfg.ServerIP,
		"mapped_hosts": s.Cfg.MappedHosts,
		"ca":           s.CA.Subject(),
		"ca_expires":   s.CA.NotAfter(),
		"filters":      len(s.Catalog.Order),
		"rc_entries":   len(s.RC.Entries),
		"uptime":       time.Since(s.Started).String(),
	}
}

func (s *Server) serveRendered(w http.ResponseWriter, r *http.Request) {
	name := filepath.Base(r.URL.Path)
	if !strings.HasSuffix(name, ".jpg") || strings.Contains(name, "..") {
		http.NotFound(w, r)
		return
	}
	path := filepath.Join(s.Cfg.PublicRoot(), "locket-filters", name)
	f, err := os.Open(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	http.ServeContent(w, r, name, info.ModTime(), f)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	raw, err := json.Marshal(v)
	if err != nil {
		http.Error(w, `{"error":"encode"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_, _ = w.Write(raw)
}

func errorBody(status, msg string) map[string]any {
	return map[string]any{"error": map[string]any{"status": status, "message": msg}}
}

func (s *Server) logf(format string, args ...any) {
	if s.Logger != nil {
		s.Logger.Printf(format, args...)
	}
}
