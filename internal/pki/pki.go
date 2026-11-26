// Package pki tạo và quản lý Root CA + chứng chỉ server dùng cho MITM.
package pki

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Authority là Root CA của hệ thống.
type Authority struct {
	Cert    *x509.Certificate
	Key     *rsa.PrivateKey
	CertPEM []byte
	KeyPEM  []byte

	certPath string
	keyPath  string
}

// LoadOrCreate đọc CA trong dir, nếu chưa có thì sinh mới (hạn 10 năm).
func LoadOrCreate(dir, name string) (*Authority, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	certPath := filepath.Join(dir, "ca.crt")
	keyPath := filepath.Join(dir, "ca.key")

	if certPEM, err := os.ReadFile(certPath); err == nil {
		if keyPEM, err2 := os.ReadFile(keyPath); err2 == nil {
			a, err3 := parse(certPEM, keyPEM)
			if err3 == nil {
				a.certPath, a.keyPath = certPath, keyPath
				return a, nil
			}
		}
	}
	return generate(certPath, keyPath, name)
}

func parse(certPEM, keyPEM []byte) (*Authority, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, errors.New("pki: ca.crt không hợp lệ")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}
	kblock, _ := pem.Decode(keyPEM)
	if kblock == nil {
		return nil, errors.New("pki: ca.key không hợp lệ")
	}
	var key *rsa.PrivateKey
	if k, err := x509.ParsePKCS1PrivateKey(kblock.Bytes); err == nil {
		key = k
	} else {
		any, err2 := x509.ParsePKCS8PrivateKey(kblock.Bytes)
		if err2 != nil {
			return nil, err2
		}
		rk, ok := any.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("pki: ca.key không phải RSA")
		}
		key = rk
	}
	return &Authority{Cert: cert, Key: key, CertPEM: certPEM, KeyPEM: keyPEM}, nil
}

func generate(certPath, keyPath, name string) (*Authority, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	if err != nil {
		return nil, err
	}
	subject := pkix.Name{
		Organization: []string{"Locket Gold"},
		CommonName:   name,
	}
	tpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               subject,
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            1,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})

	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		return nil, err
	}
	return &Authority{Cert: cert, Key: key, CertPEM: certPEM, KeyPEM: keyPEM, certPath: certPath, keyPath: keyPath}, nil
}

// CertB64 trả về chứng chỉ DER dạng base64 (để nhúng vào .mobileconfig).
func (a *Authority) CertB64() string {
	return base64.StdEncoding.EncodeToString(a.Cert.Raw)
}

// NotAfter là ngày hết hạn của CA.
func (a *Authority) NotAfter() string { return a.Cert.NotAfter.Format("2006-01-02") }

// Subject là tên hiển thị trên iPhone.
func (a *Authority) Subject() string { return a.Cert.Subject.CommonName }

// Leaf tạo (hoặc dùng lại) chứng chỉ server cho danh sách host.
// Chứng chỉ được cache trong dir; nếu danh sách host đổi thì sinh lại.
func (a *Authority) Leaf(dir string, hosts []string, pathHost string) (tls.Certificate, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return tls.Certificate{}, err
	}
	hosts = normalizeHosts(append(append([]string{}, hosts...), pathHost))
	sig := strings.Join(hosts, ",")

	certPath := filepath.Join(dir, "leaf.crt")
	keyPath := filepath.Join(dir, "leaf.key")
	sigPath := filepath.Join(dir, "leaf.hosts")

	if cached, err := os.ReadFile(sigPath); err == nil && string(cached) == sig {
		if cert, err2 := tls.LoadX509KeyPair(certPath, keyPath); err2 == nil {
			return cert, nil
		}
	}
	return a.issueLeaf(dir, certPath, keyPath, sigPath, hosts, sig)
}

func (a *Authority) issueLeaf(dir, certPath, keyPath, sigPath string, hosts []string, sig string) (tls.Certificate, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	if err != nil {
		return tls.Certificate{}, err
	}
	tpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{"Locket Gold"},
			CommonName:   hosts[0],
		},
		NotBefore:   time.Now().Add(-time.Hour),
		NotAfter:    time.Now().AddDate(5, 0, 0),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			tpl.IPAddresses = append(tpl.IPAddresses, ip)
			continue
		}
		tpl.DNSNames = append(tpl.DNSNames, h)
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, a.Cert, &key.PublicKey, a.Key)
	if err != nil {
		return tls.Certificate{}, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		return tls.Certificate{}, err
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		return tls.Certificate{}, err
	}
	if err := os.WriteFile(sigPath, []byte(sig), 0o644); err != nil {
		return tls.Certificate{}, err
	}
	return tls.X509KeyPair(certPEM, keyPEM)
}

// LeafFor sinh chứng chỉ riêng cho 1 host (dùng khi host chưa nằm trong SAN).
func (a *Authority) LeafFor(dir, host string) (tls.Certificate, error) {
	dir = filepath.Join(dir, "sni", strings.ReplaceAll(host, "*", "wildcard"))
	return a.Leaf(dir, []string{host}, "")
}

// SANs liệt kê các domain có trong chứng chỉ server gần nhất.
func SANs(hosts []string, extra ...string) []string {
	return normalizeHosts(append(append([]string{}, hosts...), extra...))
}

func normalizeHosts(hosts []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, h := range hosts {
		h = strings.ToLower(strings.TrimSpace(h))
		if h == "" || seen[h] {
			continue
		}
		seen[h] = true
		out = append(out, h)
	}
	sort.Strings(out)
	if len(out) == 0 {
		out = []string{"localhost"}
	}
	return out
}

// Verify kiểm tra chuỗi chứng chỉ server có được CA này ký hay không (dùng cho selftest).
func (a *Authority) Verify(leaf tls.Certificate) error {
	if len(leaf.Certificate) == 0 {
		return errors.New("pki: chứng chỉ rỗng")
	}
	cert, err := x509.ParseCertificate(leaf.Certificate[0])
	if err != nil {
		return err
	}
	roots := x509.NewCertPool()
	roots.AddCert(a.Cert)
	_, err = cert.Verify(x509.VerifyOptions{Roots: roots, DNSName: cert.DNSNames[0]})
	if err != nil {
		return fmt.Errorf("pki: xác thực chứng chỉ lỗi: %w", err)
	}
	return nil
}
