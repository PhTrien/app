# STATUS — Locket Gold Server

## Phiên bản: 1.0.0 (2026-11-26)

### Đã làm
- **DoH server** RFC 8484 (GET + POST, HTTP/2, `application/dns-message`). Domain trong `mapped_hosts`
  → trả IP VPS; còn lại chuyển tiếp `upstream_dns` (mặc định 1.1.1.1).
- **MITM 443 dùng chung cổng với DoH**: phân nhánh theo SNI. Tên miền DoH dùng cert Let's Encrypt
  (`tls.cert`/`tls.key`), domain Locket dùng cert do Root CA nội bộ ký (SAN: revenuecat, remoteconfig,
  cloudfunctions, api.locketcamera.com, cdn.locketcamera.com).
- **RevenueCat mock**: `subscriber.entitlements.Gold` hạn 9999 → mở Gold.
- **Firebase Remote Config mock**: 179 entry trích tự động từ `Locket_Unlocked.js`
  (feature gates, camera 60s, 296 filter / 17 nhóm, caption pack…).
- **`/generateFilteredImage`**: tải ảnh gốc (allowlist host Locket/Firebase), ghép khung PhotoBooth
  2/3/4 ảnh, overlay PNG (AI/cutout), preset màu (film/retro/cold/warm/noir/glow…), xuất **JPEG 1080×1080**,
  tên `UUID 36 ký tự` đúng schema app, trả `{"result":{"data":{"output_url":...}}}`.
  PhotoBooth trả `202 pending` + `captured/total` cho tới khi đủ ảnh.
- **`/generateCaption`**: caption tiếng Việt (preset từ remote config + bộ mặc định).
- **Reverse proxy** cho mọi request khác của `api.locketcamera.com` / cloudfunctions → server thật
  (resolver riêng, không bị ảnh hưởng DoH nội bộ).
- **Profile `.mobileconfig`** tự sinh: Root CA + DoH split-DNS 4 domain (CDN đi trực tiếp).
- **install.sh**: cài 1 lệnh (binary tĩnh → systemd → certbot tuỳ chọn → selftest).
- Binary tĩnh `linux/amd64` + `linux/arm64` trong `bin/`.

### Đã kiểm chứng (local, binary 1.0.0)
- `go build ./...`, `go vet ./...` OK; clone sạch từ GitHub build lại OK.
- `selftest` 10/10 OK (CA ký cert, profile, DoH trả IP, RevenueCat Gold, 179 entry, 296 filter,
  render 1080×1080 có khung, PhotoBooth pending 1/4→2/4, caption).
- TLS thật trên `:9443`: `/version`, `/healthz`, `/dns-query` (A record = IP), `/p/locket.mobileconfig`
  (plistlib parse OK, 2 payload), HTTP/2 negotiate OK.
- E2E qua Host `api.locketcamera.com`: 4 lần `generateFilteredImage` (pb_khung4_merry_christmas)
  → 202,202,202,200 + `output_url`; ảnh ghép 4 khung có caption "Merry Christmas" (kiểm bằng vision).
  `ai_tai_tho` (overlay PNG), `photo_effect_film` (grade), `cmt_locket` đều 200.

### Chưa kiểm chứng
- Chưa deploy lên VPS thật `103.56.158.91` — **chưa có mật khẩu SSH** trong phiên này.
- Chưa test trên iPhone thật (cần cài profile + bật trust CA).
- Bộ lọc AI (`ai_*`) hiện là **ghép overlay PNG** có sẵn trong remote config, không gọi model sinh ảnh.
- `/getFilters` (nếu app gọi) không giả lập — trả rỗng như bản mhale.

### Việc còn treo
1. Deploy lên VPS: cần `--host doh.mhale.io.vn --ip 103.56.158.91 --email ...`
   (nhớ dừng service cũ đang giữ 443).
2. Sau khi deploy: mọi máy phải **xoá profile + CA cũ** rồi cài profile mới (CA khác CA của apppre-dns).
