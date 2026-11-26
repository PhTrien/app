# STATUS — Locket Gold Server

## Phiên bản: 1.1.0 (2026-11-26)

### Đã làm
- **DoH server** RFC 8484 (GET + POST, HTTP/2, `application/dns-message`). Domain trong `mapped_hosts`
  → trả IP VPS; còn lại chuyển tiếp `upstream_dns` (mặc định 1.1.1.1).
- **MITM 443 dùng chung cổng với DoH**: phân nhánh theo SNI. Tên miền DoH dùng cert Let's Encrypt
  (`tls.cert`/`tls.key`), domain Locket dùng cert do Root CA nội bộ ký (SAN: revenuecat, remoteconfig,
  cloudfunctions, api.locketcamera.com, cdn.locketcamera.com).
- **Gold theo username (whitelist)** — thay cho mock toàn cục của bản 1.0.0:
  - `/v1/subscribers/<username>` chỉ được trả `entitlements.Gold` khi username nằm trong danh sách đã kích,
    hạn Gold = hạn ghi trong danh sách (mã 30/365 ngày hoặc vĩnh viễn).
  - Username lạ → chuyển nguyên request ra `api.revenuecat.com` thật (kèm `/v1/receipts`).
  - Ghi lại mọi ID RevenueCat nhìn thấy ("ID đã thấy") để map username ↔ ID thật.
- **Mã kích hoạt + trang công khai `/kich-hoat`**: khách nhập username (hoặc dán link `locket.cam/...`)
  + mã → ghi danh sách, hiện hạn Gold.
- **Trang quản trị `/admin`** (mật khẩu, phiên cookie HttpOnly/Secure):
  - Tạo mã: số lượng, 7/30/90/180/365 ngày hoặc vĩnh viễn, số lượt/mã, hạn của mã, ghi chú, **mã tự đặt**.
  - Copy mã, thu hồi, xoá; kích tay theo username; +30 ngày; vĩnh viễn; thu hồi; xoá.
  - Tab "ID đã thấy" + nút kích nhanh; tab hướng dẫn vận hành.
  - Đổi mật khẩu: `locket-gold admin reset --password ...`
- **Giao diện mới theo mẫu 2707226.xyz**: nền đen #0a0a0a, vàng gold #ffb000, hero điện thoại,
  thẻ tính năng, 4 bước cài, 3 gói, FAQ, CTA. Responsive tới 390px.
- Giữ nguyên: RevenueCat/Firebase mock nội dung, `/generateFilteredImage` (PhotoBooth 2/3/4 ảnh,
  overlay AI, preset màu, JPEG 1080×1080, UUID 36 ký tự), `/generateCaption`, reverse proxy,
  profile `.mobileconfig` (CA + DoH split-DNS 4 domain, CDN đi trực tiếp).
- Cấu hình mới: `admin_user`, `admin_password`, `session_hours`, `gate_remote_config` (mặc định false).
- Dữ liệu: `/var/lib/locket-gold/data/store.json` (0600) — username, mã, hạn, ID đã thấy, hash mật khẩu.

### Đã kiểm chứng (local, binary 1.1.0)
- `go build ./...`, `go vet ./...` OK.
- `selftest` **16/16 OK**: CA ký cert, profile plist, DoH trả IP, tạo mã + kích Gold, mã chỉ dùng 1 lần,
  RevenueCat trả Gold đúng hạn cho username đã kích, username lạ không được trả Gold, 179 entry RC,
  296 filter, render 1080×1080, PhotoBooth 1/4→2/4, caption, trang `/kich-hoat` kích thành công,
  `/admin` đăng nhập + render bảng mã, landing page.
- Chụp màn hình bằng Chromium (desktop 1280px + mobile 390px) và soi lại: landing, `/kich-hoat`,
  trang thành công, `/admin` (tab mã + tab người dùng) — không tràn ngang, không đè chữ.

### Chưa kiểm chứng
- Chưa deploy lên VPS thật trong phiên này (không có mật khẩu SSH).
- Chưa test trên iPhone thật (cần cài profile + bật trust CA).
- Bộ lọc AI (`ai_*`) là ghép overlay PNG có sẵn trong remote config, không gọi model sinh ảnh.
- `gate_remote_config` mặc định tắt: remote config vẫn mở khoá cho mọi client như module gốc.

### Việc còn treo
1. Deploy lên VPS: `sudo bash scripts/install.sh --host doh.mhale.io.vn --ip 103.56.158.91 --email ...`
   (nhớ dừng service cũ đang giữ 443).
2. Sau khi deploy: mọi máy phải **xoá profile + CA cũ** rồi cài profile mới (CA khác CA của apppre-dns).
3. Vào `/admin` tạo mã đầu tiên, gửi khách, test 1 tài khoản thật.
