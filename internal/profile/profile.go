// Package profile tạo file .mobileconfig (CA + DoH split-DNS) để cài trên iPhone.
package profile

import (
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"strings"
)

// Options là dữ liệu cần để dựng profile.
type Options struct {
	Host        string   // tên miền DoH
	ServerIP    string   // IP VPS
	MappedHosts []string // domain cần đi qua DoH
	CACert      []byte   // DER của Root CA
	CAName      string   // tên hiển thị của CA
	DisplayName string   // tên profile
	Identifier  string   // bundle id gốc của profile
}

// Build trả về nội dung .mobileconfig.
func Build(opt Options) []byte {
	if opt.DisplayName == "" {
		opt.DisplayName = "Locket Gold"
	}
	if opt.Identifier == "" {
		opt.Identifier = "com.locketgold.profile"
	}
	if opt.CAName == "" {
		opt.CAName = "Locket Gold CA"
	}

	caB64 := base64.StdEncoding.EncodeToString(opt.CACert)

	var hosts strings.Builder
	for _, h := range opt.MappedHosts {
		hosts.WriteString("\t\t\t\t\t<string>")
		hosts.WriteString(escapeXML(h))
		hosts.WriteString("</string>\n")
	}

	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>PayloadContent</key>
	<array>
		<dict>
			<key>PayloadCertificateFileName</key>
			<string>locket-gold-ca.crt</string>
			<key>PayloadContent</key>
			<data>
`)
	// base64 xuống dòng mỗi 64 ký tự cho dễ đọc.
	for i := 0; i < len(caB64); i += 64 {
		end := i + 64
		if end > len(caB64) {
			end = len(caB64)
		}
		b.WriteString("\t\t\t\t")
		b.WriteString(caB64[i:end])
		b.WriteString("\n")
	}
	b.WriteString(`			</data>
			<key>PayloadDescription</key>
			<string>Chứng chỉ gốc để Locket Gold giải mã kết nối HTTPS của Locket.</string>
			<key>PayloadDisplayName</key>
			<string>` + escapeXML(opt.CAName) + `</string>
			<key>PayloadIdentifier</key>
			<string>` + escapeXML(opt.Identifier) + `.ca</string>
			<key>PayloadType</key>
			<string>com.apple.security.root</string>
			<key>PayloadUUID</key>
			<string>` + uuidFor(opt.Identifier+".ca") + `</string>
			<key>PayloadVersion</key>
			<integer>1</integer>
		</dict>
		<dict>
			<key>DNSSettings</key>
			<dict>
				<key>DNSProtocol</key>
				<string>HTTPS</string>
				<key>ServerURL</key>
				<string>https://` + escapeXML(opt.Host) + `/dns-query</string>
				<key>ServerAddresses</key>
				<array>
					<string>` + escapeXML(opt.ServerIP) + `</string>
				</array>
				<key>SupplementalMatchDomains</key>
				<array>
`)
	b.WriteString(hosts.String())
	b.WriteString(`				</array>
			</dict>
			<key>PayloadDescription</key>
			<string>Chỉ chuyển các tên miền của Locket qua DoH nội bộ, phần còn lại dùng mạng thường.</string>
			<key>PayloadDisplayName</key>
			<string>` + escapeXML(opt.DisplayName) + ` DNS</string>
			<key>PayloadIdentifier</key>
			<string>` + escapeXML(opt.Identifier) + `.dns</string>
			<key>PayloadType</key>
			<string>com.apple.dnsSettings.managed</string>
			<key>PayloadUUID</key>
			<string>` + uuidFor(opt.Identifier+".dns") + `</string>
			<key>PayloadVersion</key>
			<integer>1</integer>
		</dict>
	</array>
	<key>PayloadDescription</key>
	<string>` + escapeXML(opt.DisplayName) + ` — DoH + Root CA để mở khoá tính năng Gold.</string>
	<key>PayloadDisplayName</key>
	<string>` + escapeXML(opt.DisplayName) + `</string>
	<key>PayloadIdentifier</key>
	<string>` + escapeXML(opt.Identifier) + `</string>
	<key>PayloadOrganization</key>
	<string>Locket Gold</string>
	<key>PayloadRemovalDisallowed</key>
	<false/>
	<key>PayloadType</key>
	<string>Configuration</string>
	<key>PayloadUUID</key>
	<string>` + uuidFor(opt.Identifier) + `</string>
	<key>PayloadVersion</key>
	<integer>1</integer>
</dict>
</plist>
`)
	return []byte(b.String())
}

// uuidFor sinh UUID ổn định từ một chuỗi (để cài lại không bị nhân đôi profile).
func uuidFor(seed string) string {
	sum := sha1.Sum([]byte("locket-gold:" + seed))
	b := sum[:16]
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func escapeXML(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;")
	return r.Replace(s)
}
