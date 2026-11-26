// Package config quản lý cấu hình của Locket Gold Server.
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// TLS giữ đường dẫn chứng chỉ thật của tên miền DoH (Let's Encrypt).
type TLS struct {
	Cert string `json:"cert"`
	Key  string `json:"key"`
}

// Config là toàn bộ cấu hình chạy server.
type Config struct {
	Host        string   `json:"host"`          // tên miền DoH, ví dụ doh.mhale.io.vn
	ServerIP    string   `json:"server_ip"`     // IP công khai của VPS
	Listen      string   `json:"listen"`        // mặc định ":443"
	DataDir     string   `json:"data_dir"`      // mặc định /var/lib/locket-gold
	UpstreamDNS string   `json:"upstream_dns"`  // 1.1.1.1:53
	MappedHosts []string `json:"mapped_hosts"`  // domain trỏ về VPS qua DoH
	CertHosts   []string `json:"cert_hosts"`    // domain có trong SAN của chứng chỉ MITM
	TLS         TLS      `json:"tls"`           // cert Let's Encrypt cho tên miền DoH
	LogRequests bool     `json:"log_requests"`  // ghi log request
	CAName      string   `json:"ca_name"`       // tên hiển thị của Root CA
	PublicDir   string   `json:"public_dir"`    // thư mục ảnh filter đã render
	MaxImageMB  int      `json:"max_image_mb"`  // giới hạn ảnh tải về

	path string
}

// DefaultHosts là các domain cần MITM, đúng theo file .sgmodule của Mhale.
//
// cdn.locketcamera.com KHÔNG nằm trong danh sách trỏ về VPS: ảnh/thumbnail đi thẳng
// ra CDN thật để không tốn băng thông VPS — nhưng vẫn có trong SAN của chứng chỉ.
var DefaultHosts = []string{
	"api.revenuecat.com",
	"firebaseremoteconfig.googleapis.com",
	"us-central1-locket-4252a.cloudfunctions.net",
	"api.locketcamera.com",
}

// CertHosts mặc định = domain MITM + CDN.
var CertHosts = []string{
	"api.revenuecat.com",
	"firebaseremoteconfig.googleapis.com",
	"us-central1-locket-4252a.cloudfunctions.net",
	"api.locketcamera.com",
	"cdn.locketcamera.com",
}

// Default trả về cấu hình mặc định.
func Default() *Config {
	return &Config{
		Host:        "doh.example.com",
		ServerIP:    "127.0.0.1",
		Listen:      ":443",
		DataDir:     "/var/lib/locket-gold",
		UpstreamDNS: "1.1.1.1:53",
		MappedHosts: append([]string{}, DefaultHosts...),
		CertHosts:   append([]string{}, CertHosts...),
		LogRequests: true,
		CAName:      "Locket Gold CA",
		MaxImageMB:  30,
	}
}

// Load đọc file cấu hình; nếu chưa có thì tạo mới với giá trị mặc định.
func Load(path string) (*Config, error) {
	cfg := Default()
	cfg.path = path

	raw, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		return cfg, nil
	}
	if err := json.Unmarshal(raw, cfg); err != nil {
		return nil, err
	}
	if len(cfg.MappedHosts) == 0 {
		cfg.MappedHosts = append([]string{}, DefaultHosts...)
	}
	if len(cfg.CertHosts) == 0 {
		cfg.CertHosts = append([]string{}, CertHosts...)
	}
	if cfg.Listen == "" {
		cfg.Listen = ":443"
	}
	if cfg.UpstreamDNS == "" {
		cfg.UpstreamDNS = "1.1.1.1:53"
	}
	if cfg.MaxImageMB <= 0 {
		cfg.MaxImageMB = 30
	}
	if cfg.CAName == "" {
		cfg.CAName = "Locket Gold CA"
	}
	cfg.path = path
	return cfg, nil
}

// Save ghi cấu hình ra đĩa (quyền 0600 vì chứa thông tin hạ tầng).
func (c *Config) Save() error {
	if c.path == "" {
		return errors.New("config: chưa có đường dẫn")
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.path, append(raw, '\n'), 0o600)
}

// Path trả về đường dẫn file cấu hình.
func (c *Config) Path() string { return c.path }

// IsMapped cho biết host có nằm trong danh sách cần trỏ về VPS hay không.
func (c *Config) IsMapped(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	for _, h := range c.MappedHosts {
		if strings.ToLower(h) == host {
			return true
		}
	}
	return false
}

// IsDoHHost kiểm tra host có phải tên miền DoH của mình.
func (c *Config) IsDoHHost(host string) bool {
	host = strings.ToLower(stripPort(host))
	return host == strings.ToLower(c.Host)
}

func stripPort(host string) string {
	if i := strings.LastIndex(host, ":"); i > 0 && !strings.Contains(host[i:], "]") {
		if strings.Count(host, ":") == 1 {
			return host[:i]
		}
	}
	if strings.HasPrefix(host, "[") {
		if i := strings.Index(host, "]"); i > 0 {
			return host[1:i]
		}
	}
	return host
}

// Dir trả về đường dẫn con trong thư mục dữ liệu, tự tạo nếu cần.
func (c *Config) Dir(elem ...string) string {
	p := filepath.Join(append([]string{c.DataDir}, elem...)...)
	_ = os.MkdirAll(p, 0o755)
	return p
}

// PublicRoot là thư mục gốc phục vụ file tĩnh (ảnh filter).
func (c *Config) PublicRoot() string {
	if c.PublicDir != "" {
		return c.PublicDir
	}
	return c.Dir("public")
}
