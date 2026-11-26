# HƯỚNG DẪN CÀI ĐẶT LOCKET GOLD SERVER TRÊN VPS (TỪNG BƯỚC)

Tài liệu này cài **từ đầu** trên một VPS trắng: từ trỏ DNS, mở firewall, chạy installer,
tới cài profile trên iPhone và kiểm tra Gold.

Thời gian: ~10 phút (chưa tính chờ DNS và Let's Encrypt).

---

## 0. Chuẩn bị

| Thứ cần có | Yêu cầu |
|---|---|
| VPS | Ubuntu 22.04 / 24.04 hoặc Debian 12, **RAM ≥ 512MB**, 1 vCPU là đủ |
| Tên miền | 1 domain hoặc subdomain, ví dụ `doh.mhale.io.vn` |
| Quyền | `root` (hoặc user có sudo) |
| iPhone | iOS 14+ để cài profile DoH |

Cổng cần mở: **22** (SSH), **80** (xin chứng chỉ), **443** (DoH + MITM).

> ⚠️ Nếu VPS đang chạy một server Locket/DNS khác chiếm cổng 443 (ví dụ `apppre-dns`, `node`, `nginx`),
> phải dừng nó trước — xem mục 10.

---

## 1. Trỏ DNS về VPS

Vào DNS của tên miền (Cloudflare / Namecheap / Mắt Bão…) thêm bản ghi:

| Type | Name | Value | Proxy |
|---|---|---|---|
| A | `doh` | `IP_CỦA_VPS` | **DNS only (mây xám)** |

> 🔴 **Quan trọng:** nếu dùng Cloudflare thì **phải tắt proxy (mây xám)**.
> Để mây cam thì Cloudflare chặn/đổi TLS, DoH và MITM sẽ không hoạt động.

Kiểm tra đã trỏ đúng (chạy trên máy bạn):

```bash
dig +short doh.mhale.io.vn
# phải ra đúng IP VPS
```

Chờ tới khi lệnh trên trả về đúng IP rồi mới làm bước sau.

---

## 2. SSH vào VPS và cập nhật hệ thống

```bash
ssh root@IP_CỦA_VPS
```

```bash
apt update && apt upgrade -y
apt install -y curl git ca-certificates
```

Mở firewall (nếu có ufw):

```bash
ufw allow 22/tcp
ufw allow 80/tcp
ufw allow 443/tcp
ufw --force enable
```

Nếu VPS dùng firewall của nhà cung cấp (Security Group), mở thêm **80** và **443** ở panel.

---

## 3. Tải mã nguồn lên VPS

**Cách A — clone từ GitHub (khuyến nghị):**

```bash
cd /opt
git clone https://github.com/PhTrien/app.git locket-gold
cd locket-gold
```

**Cách B — không có git:** tải file `locket-gold.tar.gz` trên máy bạn rồi đẩy lên:

```bash
# chạy trên máy bạn
scp locket-gold.tar.gz root@IP_CỦA_VPS:/opt/

# chạy trên VPS
cd /opt && tar -xzf locket-gold.tar.gz && cd locket-gold
```

Trong thư mục phải thấy: `bin/`, `cmd/`, `internal/`, `scripts/install.sh`, `README.md`.

---

## 4. Chạy installer (1 lệnh)

```bash
cd /opt/locket-gold
sudo bash scripts/install.sh \
  --host doh.mhale.io.vn \
  --ip IP_CỦA_VPS \
  --email ban@email.com
```

Giải thích tham số:

| Tham số | Ý nghĩa |
|---|---|
| `--host` | tên miền DoH (đúng như bản ghi A ở bước 1) |
| `--ip` | IP công khai của VPS (DoH sẽ trả IP này cho app) |
| `--email` | email cho Let's Encrypt (bỏ trống + thêm `--no-certbot` nếu muốn dùng CA nội bộ) |
| `--port` | mặc định 443, chỉ đổi nếu 443 đã bị chiếm |

Script sẽ làm tuần tự:

1. Cài binary tĩnh `locket-gold` vào `/usr/local/bin/`
2. Tạo `Root CA` (hạn 10 năm) + chứng chỉ server + profile `.mobileconfig`
3. (nếu có `--email`) xin chứng chỉ Let's Encrypt cho tên miền DoH
4. Tạo service `systemd` và bật chạy 24/7
5. Chạy tự kiểm tra 16 mục

Cuối phần `locket-gold init` sẽ in ra:

```
Trang quản trị : https://doh.mhale.io.vn/admin
Mật khẩu admin : XXXXXXXXXXXX
```

**Lưu lại mật khẩu này** — dùng để vào trang quản trị tạo mã kích hoạt. Kết quả đúng của installer
là dòng cuối:

```
KẾT QUẢ: OK — server sẵn sàng.
```

---

## 5. Kiểm tra server trên VPS

```bash
systemctl status locket-gold --no-pager
```

Phải thấy `Active: active (running)`.

```bash
locket-gold selftest
```

Ví dụ đầu ra:

```
  ✓ chứng chỉ server do CA ký              Locket Gold CA
  ✓ profile .mobileconfig                  3657 bytes, 4 domain
  ✓ DoH api.revenuecat.com                 → IP_VPS
  ✓ kích Gold bằng mã                      LG-XXXX-XXXX → @selftest.user
  ✓ mã chỉ dùng 1 lần                      mã này đã dùng hết lượt
  ✓ RevenueCat Gold theo username          1 subscription, hạn 2026-12-26
  ✓ username chưa kích                     đi thẳng ra RevenueCat thật
  ✓ Remote Config                          179 entry, state=UPDATE_AVAILABLE
  ✓ catalog filter                         296 filter / 17 nhóm
  ✓ render filter ai_tai_tho               1080x1080, khung=true
  ✓ photobooth pb_khung4_merry_christmas   pending 1/4 → 2/4 đúng
  ✓ generateCaption                        ...
  ✓ trang /kich-hoat                       kích thành công @khach.moi
  ✓ trang /admin                           đăng nhập + danh sách mã
  ✓ landing page                           21718 bytes
KẾT QUẢ: OK — server sẵn sàng.
```

Kiểm tra từ ngoài (chạy trên máy bạn):

```bash
curl -s https://doh.mhale.io.vn/version | python3 -m json.tool
```

Phải trả JSON có `"version"`, `"filters": 296`, `"ca": "Locket Gold CA"`.

Kiểm tra DoH thật:

```bash
curl -s -H 'accept: application/dns-message' \
  "https://doh.mhale.io.vn/dns-query?dns=AAABAAABAAAAAAAAA2FwaQpyZXZlbnVlY2F0A2NvbQAAAQAB" \
  --output /tmp/doh.bin && xxd /tmp/doh.bin | tail -2
```

Nếu ra `http 200` + vài byte cuối là IP VPS → DoH đã đúng.

---

## 6. Chứng chỉ TLS cho tên miền DoH

Server dùng **2 loại chứng chỉ** trên cùng cổng 443:

| Loại | Ai ký | Dùng cho |
|---|---|---|
| TLS công khai | Let's Encrypt | `https://doh.mhale.io.vn/dns-query` (iOS yêu cầu cert tin cậy) |
| Cert MITM | Root CA của bạn | `api.revenuecat.com`, `firebaseremoteconfig…`, `api.locketcamera.com`… |

Nếu lúc cài chưa có `--email`, lấy chứng chỉ bây giờ:

```bash
apt install -y certbot
certbot certonly --standalone -d doh.mhale.io.vn --agree-tos -m ban@email.com --non-interactive
systemctl restart locket-gold
```

Kiểm tra:

```bash
curl -sI https://doh.mhale.io.vn/version | head -3
```

Không còn cảnh báo chứng chỉ là đúng.

> Gia hạn tự động: certbot trên Ubuntu đã có timer sẵn (`systemctl list-timers | grep certbot`).
> Sau khi gia hạn nhớ `systemctl restart locket-gold` (hoặc thêm hook renew).

---

## 7. Cài trên iPhone

1. Mở **Safari** (bắt buộc Safari, Chrome không cài được profile):
   `https://doh.mhale.io.vn/p/locket.mobileconfig`
2. Hiện “Hồ sơ đã tải về” → **Cài đặt** → **Cài đặt** → nhập mật khẩu máy → **Cài đặt**.
3. Vào **Cài đặt → Cài đặt chung → Giới thiệu (About) → Cài đặt tin cậy chứng nhận**
   → bật công tắc cho **Locket Gold CA**.
   *(Bước này bắt buộc, không bật thì Locket sẽ báo lỗi mạng.)*
4. Kiểm tra DNS đã bật: **Cài đặt → Cài đặt chung → VPN & Quản lý thiết bị → DNS**
   (hoặc Cài đặt → Mạng) → thấy profile **Locket Gold DNS**.
5. Mở **Locket** → vào phần Gold/Filters → thấy đã mở khoá.

> Profile chỉ chuyển 4 domain Locket qua DoH của bạn (split-DNS). Riêng `cdn.locketcamera.com` (ảnh/thumbnail)
> cố tình KHÔNG đi qua VPS để tiết kiệm băng thông — nó vẫn dùng mạng thường.
> Facebook, YouTube, TikTok… vẫn dùng mạng bình thường, không bị ảnh hưởng tốc độ.

---

## 8. Kích hoạt Gold: tạo mã và cấp cho khách

Gold được bật **theo username**, nên mỗi khách cần một mã.

### 8.1 Vào trang quản trị

Mở: `https://doh.mhale.io.vn/admin` → nhập mật khẩu admin (in ra ở bước 4).

Quên mật khẩu:

```bash
locket-gold admin reset --password MATKHAU_MOI
```

### 8.2 Tạo mã

Tab **Mã kích hoạt**:

| Ô | Ý nghĩa |
|---|---|
| Số lượng | tạo 1 lần bao nhiêu mã (tối đa 200) |
| Thời hạn Gold | 7 / 30 / 90 / 180 / 365 ngày hoặc **Vĩnh viễn** |
| Số lượt dùng / mã | 1 = một người dùng; >1 = dùng chung được nhiều người |
| Ghi chú | để bạn tự nhớ (khách Zalo, đợt 1…) |
| Mã tự đặt | bỏ trống = hệ thống sinh `LG-XXXX-XXXX`; điền = dùng đúng mã bạn muốn, ví dụ `TET2026` |
| Hạn của mã | mã hết hiệu lực sau bao nhiêu ngày (0 = không hạn) |

Bấm **Tạo mã** → mã hiện ngay trong bảng, nút **Copy** để gửi khách.

### 8.3 Khách kích hoạt

Gửi khách: link `https://doh.mhale.io.vn/kich-hoat` + mã.

Khách nhập:
- **Username Locket** — phần sau `locket.cam/` (dán nguyên link cũng được, hệ thống tự tách).
- **Mã** — `LG-XXXX-XXXX`.

Bấm **Kích hoạt ngay** → hiện hạn Gold. Mở lại Locket là có Gold.

### 8.4 Quản lý người dùng

Tab **Người dùng**: danh sách username đã kích, hạn, số lần app gọi, nút **+30 ngày**, **Vĩnh viễn**,
**Thu hồi** (Gold tắt ngay), **Xoá**.

### 8.5 Khách không biết username

Mở Locket trên máy khách một lần → vào tab **ID đã thấy**, server đã ghi lại ID thật app gọi lên.
Bấm **Kích 1 năm** để map trực tiếp ID đó (kèm ghi chú).

### 8.6 Nguyên tắc hoạt động

- Username **có** trong danh sách → server trả `entitlements.Gold` đúng hạn của mã.
- Username **không** có → request đi nguyên ra `api.revenuecat.com` thật (không bị ảnh hưởng).
- Hết hạn → Gold tự tắt, không cần thao tác.
- Dữ liệu: `/var/lib/locket-gold/data/store.json` (quyền 0600).

---

## 9. Kiểm tra tính năng

**Gold:** mở Locket → mục Gold/Profile badge/Video dài 60s → dùng được.

**Filter thường:** chọn 1 filter màu (Camera Filter) → chụp → ảnh trả về có chỉnh màu.

**PhotoBooth (khung 4 ảnh):** chọn khung `Thư Giáng Sinh` → app yêu cầu chụp 4 lần
→ lần 4 trả ảnh ghép 1080×1080 có khung.

**Caption:** bấm gợi ý caption → trả câu tiếng Việt.

Xem log realtime khi test:

```bash
journalctl -u locket-gold -f
```

---

## 10. Vận hành

```bash
systemctl restart locket-gold     # khởi động lại
systemctl stop locket-gold        # dừng
journalctl -u locket-gold -n 100  # 100 dòng log cuối
locket-gold ca show               # xem tên + hạn của Root CA
locket-gold admin reset --password MATKHAU_MOI   # đổi mật khẩu trang quản trị
```

**Cập nhật phiên bản mới (giữ nguyên dữ liệu, CA, profile):**

```bash
cd /opt/locket-gold
git pull
sudo bash scripts/install.sh --upgrade
```

**Backup:**

```bash
tar -czf /root/locket-gold-backup-$(date +%F).tar.gz /etc/locket-gold /var/lib/locket-gold
```

Trong đó:
- `/var/lib/locket-gold/ca/` là **Root CA** — mất là mọi máy phải cài lại profile.
- `/var/lib/locket-gold/data/store.json` là **danh sách username Gold + mã kích hoạt** — mất là phải cấp lại mã.

**Gỡ cài đặt (giữ dữ liệu):**

```bash
sudo bash scripts/install.sh --uninstall
```

**Đổi tên CA / làm mới CA (mọi máy phải cài lại profile):**

```bash
rm -rf /var/lib/locket-gold/ca
locket-gold init --config /etc/locket-gold/config.json \
  --host doh.mhale.io.vn --ip IP_CỦA_VPS --ca-name "Locket Gold CA" --force
systemctl restart locket-gold
```

---

## 11. Xử lý sự cố

| Hiện tượng | Nguyên nhân thường gặp | Cách xử lý |
|---|---|---|
| `systemctl status` báo `failed` | cổng 443 bị chiếm | `ss -tlnp \| grep :443` → dừng service đang giữ cổng (`systemctl stop apppre-dns` / `nginx` / `pm2 stop …`) rồi `systemctl restart locket-gold` |
| `curl https://doh…` báo certificate | chưa có cert Let's Encrypt | làm mục 6 |
| Không mở được link profile | Cloudflare đang bật proxy (mây cam) | tắt proxy, chờ 1–2 phút |
| iPhone báo “Không có kết nối mạng” trong Locket | chưa bật **Cài đặt tin cậy chứng nhận** | mục 7 bước 3 |
| Locket vẫn không Gold | DoH chưa được bật / DNS chưa trỏ đúng | kiểm tra DNS trong Cài đặt; `dig +short` trên máy tính phải ra IP VPS |
| Filter báo lỗi | ảnh gốc không tải được hoặc filter chưa có trong catalog | `journalctl -u locket-gold -n 50` xem dòng `filter lỗi` |
| Ảnh ghép PhotoBooth chỉ có 1 khung | app chưa gửi đủ số ảnh | xem log: `pending x/4` — chụp đủ 4 lần |
| `selftest` lỗi mục “render filter” | VPS không ra được Internet (khung tải từ huggingface) | `curl -I https://huggingface.co` — mở outbound hoặc đổi mạng |
| DoH trả 404 | gọi sai path | đúng là `/dns-query` |
| Muốn xem có ai đang dùng | log mỗi request | bật `"log_requests": true` trong `/etc/locket-gold/config.json` |
| Nhập mã báo “mã không tồn tại” | gõ sai / mã đã xoá | copy lại đúng mã trong tab Mã kích hoạt |
| Nhập mã báo “đã dùng hết lượt” | mã đặt số lượt = 1 | tạo mã mới hoặc kích tay trong tab Người dùng |
| Đã kích mà Locket vẫn không Gold | app đang cache trạng thái cũ | tắt hẳn Locket rồi mở lại; kiểm tra username đúng chưa ở tab Người dùng |
| Không đăng nhập được /admin | quên mật khẩu | `locket-gold admin reset --password MATKHAU_MOI` |
| Vào /admin bị quay lại trang đăng nhập | mở bằng `http://` nên cookie `Secure` bị bỏ | luôn dùng `https://` |

Xem cấu hình hiện tại:

```bash
cat /etc/locket-gold/config.json
```

---

## 12. Bảo mật & lưu ý

- **Root CA là tài sản nhạy cảm.** Ai có `ca.key` đều có thể giả lập mọi trang HTTPS với máy đã tin cậy CA đó.
  Không chia sẻ thư mục `/var/lib/locket-gold/ca/`, không đưa lên GitHub.
- Repo này **không chứa key, CA hay mật khẩu** — chỉ mã nguồn và binary.
- Server chỉ mở đúng những gì cần: DoH công khai + MITM cho 5 domain Locket.
  Request lạ tới `api.locketcamera.com/…` được chuyển nguyên vẹn ra server thật, không đọc nội dung.
- Giới hạn người dùng đã có sẵn: Gold chỉ bật cho username nằm trong danh sách kích hoạt
  (tab **Người dùng**). Người lạ cài profile vẫn dùng Locket bình thường nhưng không có Gold.
- Mật khẩu admin lưu dạng băm trong `store.json`; bản trong `config.json` chỉ để đăng nhập lần đầu,
  đổi bằng `locket-gold admin reset`.
- Nếu lộ mã: tab **Mã kích hoạt** → **Thu hồi**. Người đã kích vẫn giữ Gold tới hết hạn;
  muốn tắt ngay thì **Thu hồi** ở tab Người dùng.

---

## 13. Phụ lục

### 13.1 Cài thủ công (không dùng install.sh)

```bash
install -m 0755 bin/locket-gold-linux-amd64 /usr/local/bin/locket-gold
locket-gold init --config /etc/locket-gold/config.json \
  --host doh.mhale.io.vn --ip IP_CỦA_VPS
```

Tạo `/etc/systemd/system/locket-gold.service`:

```ini
[Unit]
Description=Locket Gold Server
After=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/locket-gold run --config /etc/locket-gold/config.json
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
```

```bash
systemctl daemon-reload && systemctl enable --now locket-gold
```

### 13.2 Build từ mã nguồn (nếu không dùng binary có sẵn)

```bash
apt install -y golang-go
cd /opt/locket-gold
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /usr/local/bin/locket-gold ./cmd/locket-gold
```

### 13.3 Cấu hình mặc định (`/etc/locket-gold/config.json`)

```json
{
  "host": "doh.mhale.io.vn",
  "server_ip": "IP_CỦA_VPS",
  "listen": ":443",
  "data_dir": "/var/lib/locket-gold",
  "upstream_dns": "1.1.1.1:53",
  "mapped_hosts": [
    "api.revenuecat.com",
    "firebaseremoteconfig.googleapis.com",
    "us-central1-locket-4252a.cloudfunctions.net",
    "api.locketcamera.com"
  ],
  "cert_hosts": [
    "api.revenuecat.com",
    "firebaseremoteconfig.googleapis.com",
    "us-central1-locket-4252a.cloudfunctions.net",
    "api.locketcamera.com",
    "cdn.locketcamera.com"
  ],
  "tls": {
    "cert": "/etc/letsencrypt/live/doh.mhale.io.vn/fullchain.pem",
    "key": "/etc/letsencrypt/live/doh.mhale.io.vn/privkey.pem"
  },
  "log_requests": true,
  "ca_name": "Locket Gold CA",
  "max_image_mb": 30
}
```

### 13.4 Dữ liệu nằm ở đâu

| Đường dẫn | Nội dung |
|---|---|
| `/etc/locket-gold/config.json` | cấu hình |
| `/var/lib/locket-gold/ca/` | Root CA + chứng chỉ server (giữ kỹ) |
| `/var/lib/locket-gold/profiles/locket.mobileconfig` | profile đã sinh |
| `/var/lib/locket-gold/public/locket-filters/` | ảnh filter đã render |
| `/var/lib/locket-gold/frames/` | cache khung PhotoBooth |
| `/var/lib/locket-gold/sessions/` | phiên chụp nhiều ảnh (tự dọn sau 2 giờ) |
