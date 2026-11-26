// Locket Gold Server — DoH + MITM + filter/caption cho Locket (iOS).
package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/PhTrien/app/internal/assets"
	"github.com/PhTrien/app/internal/caption"
	"github.com/PhTrien/app/internal/catalog"
	"github.com/PhTrien/app/internal/config"
	"github.com/PhTrien/app/internal/doh"
	"github.com/PhTrien/app/internal/filters"
	"github.com/PhTrien/app/internal/httpapi"
	"github.com/PhTrien/app/internal/mitm"
	"github.com/PhTrien/app/internal/mock"
	"github.com/PhTrien/app/internal/pki"
	"github.com/PhTrien/app/internal/profile"
	"github.com/PhTrien/app/internal/store"
)

const version = "1.1.0"

func main() {
	args := os.Args[1:]
	cmd := "run"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd = args[0]
		args = args[1:]
	}

	switch cmd {
	case "run":
		runCmd(args)
	case "init":
		initCmd(args)
	case "selftest":
		selftestCmd(args)
	case "version", "-v", "--version":
		fmt.Printf("locket-gold %s\n", version)
	case "ca":
		caCmd(args)
	case "admin":
		adminCmd(args)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "lệnh không hợp lệ: %s\n\n", cmd)
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Print(`locket-gold — Locket Gold Server (DoH + MITM)

  locket-gold init      --host doh.example.com --ip 1.2.3.4 [--config ...] [--ca-name "..."]
  locket-gold run       [--config /etc/locket-gold/config.json]
  locket-gold selftest  [--config /etc/locket-gold/config.json]
  locket-gold ca show   [--config ...]
  locket-gold admin reset [--password MỚI] [--config ...]
  locket-gold version
`)
}

// bundle gom các thành phần đã dựng để tái sử dụng cho run/selftest.
type bundle struct {
	srv    *httpapi.Server
	cfg    *config.Config
	ca     *pki.Authority
	leaf   tls.Certificate
	logger *log.Logger
}

func buildBundle(cfg *config.Config, logger *log.Logger) (*bundle, error) {
	ca, err := pki.LoadOrCreate(cfg.Dir("ca"), cfg.CAName)
	if err != nil {
		return nil, fmt.Errorf("khởi tạo CA: %w", err)
	}
	leaf, err := ca.Leaf(cfg.Dir("ca", "leaf"), cfg.CertHosts, cfg.Host)
	if err != nil {
		return nil, fmt.Errorf("tạo chứng chỉ server: %w", err)
	}

	cat, err := catalog.Load(assets.EntriesJSON)
	if err != nil {
		return nil, fmt.Errorf("nạp catalog: %w", err)
	}
	var entries map[string]any
	if err := json.Unmarshal(assets.EntriesJSON, &entries); err != nil {
		return nil, fmt.Errorf("nạp remote config: %w", err)
	}

	engine := filters.New(cat, cfg.Host,
		filepath.Join(cfg.PublicRoot(), "locket-filters"),
		cfg.Dir("frames"), cfg.Dir("sessions"), logger)
	engine.MaxBytes = int64(cfg.MaxImageMB) << 20

	prof := profile.Build(profile.Options{
		Host:        cfg.Host,
		ServerIP:    cfg.ServerIP,
		MappedHosts: cfg.MappedHosts,
		CACert:      ca.Cert.Raw,
		CAName:      cfg.CAName,
		DisplayName: "Locket Gold",
		Identifier:  "com.locketgold.profile",
	})
	profilePath := filepath.Join(cfg.Dir("profiles"), "locket.mobileconfig")
	if err := os.WriteFile(profilePath, prof, 0o644); err != nil {
		logger.Printf("cảnh báo: không ghi được profile: %v", err)
	}

	ip := net.ParseIP(cfg.ServerIP)
	if ip == nil {
		return nil, fmt.Errorf("server_ip không hợp lệ: %q", cfg.ServerIP)
	}
	dohHandler := doh.New(ip, cfg.IsMapped, cfg.UpstreamDNS, logger, cfg.LogRequests)

	st, err := store.Open(cfg.Dir("data"))
	if err != nil {
		return nil, fmt.Errorf("mở kho dữ liệu: %w", err)
	}

	srv := &httpapi.Server{
		Cfg:      cfg,
		CA:       ca,
		Catalog:  cat,
		Engine:   engine,
		Caption:  caption.New(cat),
		RC:       mock.NewRemoteConfig(entries),
		DoH:      dohHandler,
		Upstream: mitm.NewUpstream(cfg.UpstreamDNS),
		Profile:  prof,
		Logger:   logger,
		Version:  version,
		Started:  time.Now(),
		Store:    st,
	}
	return &bundle{srv: srv, cfg: cfg, ca: ca, leaf: leaf, logger: logger}, nil
}

func runCmd(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	cfgPath := fs.String("config", "/etc/locket-gold/config.json", "đường dẫn file cấu hình")
	_ = fs.Parse(args)

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("đọc cấu hình: %v", err)
	}
	logger := log.New(os.Stdout, "", log.LstdFlags|log.LUTC)

	b, err := buildBundle(cfg, logger)
	if err != nil {
		log.Fatalf("%v", err)
	}

	// Mật khẩu admin: ưu tiên bản đã đổi trong /admin, sau đó tới config,
	// cuối cùng sinh mới và in ra một lần.
	if !b.srv.Store.HasPassword() && cfg.AdminPassword == "" {
		pw := randomPassword(12)
		cfg.AdminPassword = pw
		if err := cfg.Save(); err != nil {
			logger.Printf("cảnh báo: không ghi được mật khẩu admin vào config: %v", err)
		}
		logger.Printf("mật khẩu admin mới: %s  (đổi tại https://%s/admin)", pw, cfg.Host)
	}

	// Chứng chỉ cho tên miền DoH: ưu tiên Let's Encrypt, nếu chưa có thì dùng CA nội bộ.
	dohCert := b.leaf
	if cfg.TLS.Cert != "" && cfg.TLS.Key != "" {
		if c, err := tls.LoadX509KeyPair(cfg.TLS.Cert, cfg.TLS.Key); err == nil {
			dohCert = c
			logger.Printf("DoH dùng chứng chỉ: %s", cfg.TLS.Cert)
		} else {
			logger.Printf("cảnh báo: không nạp được chứng chỉ %s (%v) — tạm dùng CA nội bộ", cfg.TLS.Cert, err)
		}
	}

	tlsCfg := &tls.Config{
		MinVersion: tls.VersionTLS12,
		NextProtos: []string{"h2", "http/1.1"},
		GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			sni := strings.ToLower(strings.TrimSuffix(hello.ServerName, "."))
			if sni == "" || cfg.IsDoHHost(sni) {
				return &dohCert, nil
			}
			if c, err := b.ca.LeafFor(cfg.Dir("ca", "sni"), sni); err == nil {
				return &c, nil
			}
			return &b.leaf, nil
		},
	}

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           b.srv,
		TLSConfig:         tlsCfg,
		ReadHeaderTimeout: 20 * time.Second,
		IdleTimeout:       120 * time.Second,
		ErrorLog:          logger,
	}

	ln, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		log.Fatalf("mở cổng %s: %v", cfg.Listen, err)
	}

	// Dọn phiên PhotoBooth cũ mỗi 30 phút.
	go func() {
		for {
			time.Sleep(30 * time.Minute)
			b.srv.Engine.CleanupSessions()
		}
	}()

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		logger.Printf("đang dừng server...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	logger.Printf("locket-gold %s | host=%s ip=%s listen=%s", version, cfg.Host, cfg.ServerIP, cfg.Listen)
	logger.Printf("CA: %s (hết hạn %s)", b.ca.Subject(), b.ca.NotAfter())
	logger.Printf("profile: https://%s/p/locket.mobileconfig", cfg.Host)

	if err := srv.ServeTLS(ln, "", ""); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server dừng: %v", err)
	}
}

func initCmd(args []string) {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	cfgPath := fs.String("config", "/etc/locket-gold/config.json", "đường dẫn file cấu hình")
	host := fs.String("host", "", "tên miền DoH (vd: doh.example.com)")
	ip := fs.String("ip", "", "IP công khai của VPS")
	caName := fs.String("ca-name", "Locket Gold CA", "tên hiển thị của Root CA")
	dataDir := fs.String("data-dir", "/var/lib/locket-gold", "thư mục dữ liệu")
	force := fs.Bool("force", false, "ghi đè cấu hình đang có")
	_ = fs.Parse(args)

	if *host == "" || *ip == "" {
		fmt.Fprintln(os.Stderr, "cần --host và --ip")
		os.Exit(2)
	}
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("%v", err)
	}
	cfg.Host = *host
	cfg.ServerIP = *ip
	cfg.DataDir = *dataDir
	cfg.CAName = *caName
	if cfg.TLS.Cert == "" {
		cfg.TLS.Cert = "/etc/letsencrypt/live/" + *host + "/fullchain.pem"
		cfg.TLS.Key = "/etc/letsencrypt/live/" + *host + "/privkey.pem"
	}
	if *force {
		_ = os.Remove(*cfgPath)
	}
	if err := cfg.Save(); err != nil {
		log.Fatalf("ghi cấu hình: %v", err)
	}

	logger := log.New(os.Stdout, "", log.LstdFlags)
	b, err := buildBundle(cfg, logger)
	if err != nil {
		log.Fatalf("%v", err)
	}
	fmt.Printf("\nĐã tạo cấu hình: %s\n", *cfgPath)
	fmt.Printf("Root CA        : %s (hết hạn %s)\n", b.ca.Subject(), b.ca.NotAfter())
	fmt.Printf("Data dir       : %s\n", cfg.DataDir)
	fmt.Printf("Profile        : https://%s/p/locket.mobileconfig\n", cfg.Host)
	fmt.Printf("DoH endpoint   : https://%s/dns-query\n", cfg.Host)
	fmt.Printf("Filter catalog : %d filter\n\n", len(b.srv.Catalog.Order))

	// Mật khẩu admin cho lần cài đầu.
	if cfg.AdminPassword == "" {
		cfg.AdminPassword = randomPassword(12)
		if err := cfg.Save(); err != nil {
			log.Fatalf("ghi mật khẩu admin: %v", err)
		}
	}
	fmt.Printf("Trang quản trị : https://%s/admin\n", cfg.Host)
	fmt.Printf("Mật khẩu admin : %s\n", cfg.AdminPassword)
	fmt.Printf("Đổi mật khẩu   : locket-gold admin reset --password MATKHAU_MOI\n\n")
}

// randomPassword sinh mật khẩu dễ đọc (bỏ ký tự dễ nhầm).
func randomPassword(n int) string {
	const alphabet = "abcdefghjkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		for i := range b {
			b[i] = alphabet[i%len(alphabet)]
		}
		return string(b)
	}
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(b)
}

// adminCmd đổi mật khẩu trang quản trị.
func adminCmd(args []string) {
	if len(args) == 0 || args[0] != "reset" {
		fmt.Println("dùng: locket-gold admin reset [--password MẬT_KHẨU_MỚI]")
		return
	}
	fs := flag.NewFlagSet("admin reset", flag.ExitOnError)
	cfgPath := fs.String("config", "/etc/locket-gold/config.json", "cấu hình")
	pw := fs.String("password", "", "mật khẩu mới (bỏ trống để sinh ngẫu nhiên)")
	_ = fs.Parse(args[1:])

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("%v", err)
	}
	st, err := store.Open(cfg.Dir("data"))
	if err != nil {
		log.Fatalf("mở kho dữ liệu: %v", err)
	}
	newPw := strings.TrimSpace(*pw)
	if newPw == "" {
		newPw = randomPassword(12)
	}
	if err := st.SetPassword(newPw); err != nil {
		log.Fatalf("đặt mật khẩu: %v", err)
	}
	cfg.AdminPassword = newPw
	if err := cfg.Save(); err != nil {
		fmt.Printf("cảnh báo: không ghi được vào config: %v\n", err)
	}
	fmt.Printf("Mật khẩu admin mới: %s\n", newPw)
	fmt.Printf("Đăng nhập tại      : https://%s/admin\n", cfg.Host)
}

func caCmd(args []string) {
	if len(args) == 0 || args[0] != "show" {
		fmt.Println("dùng: locket-gold ca show [--config ...]")
		return
	}
	fs := flag.NewFlagSet("ca show", flag.ExitOnError)
	cfgPath := fs.String("config", "/etc/locket-gold/config.json", "cấu hình")
	_ = fs.Parse(args[1:])

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("%v", err)
	}
	ca, err := pki.LoadOrCreate(cfg.Dir("ca"), cfg.CAName)
	if err != nil {
		log.Fatalf("%v", err)
	}
	fmt.Printf("Tên CA      : %s\n", ca.Subject())
	fmt.Printf("Hết hạn     : %s\n", ca.NotAfter())
	fmt.Printf("File cert   : %s\n", filepath.Join(cfg.Dir("ca"), "ca.crt"))
	fmt.Printf("Profile     : https://%s/p/locket.mobileconfig\n", cfg.Host)
}
