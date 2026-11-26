# Locket Gold Server

Server tự dựng cho **Locket Gold** trên VPS: DoH + MITM + render filter/caption, không cần Shadowrocket.
Dựa trên bộ script client của repo [`PhTrien/mhale`](https://github.com/PhTrien/mhale) (module, `Locket_Unlocked.js`,
`Locket_Mhale.js`, `Locket_Filters.js`, `deleteHeader.js`) và biến nó thành một server độc lập.

## Nó làm gì

| Thành phần | Chức năng |
|---|---|
| **DoH server** (`/dns-query`) | Trả lời DNS qua HTTPS. Chỉ những domain của Locket được trỏ về VPS, phần còn lại chuyển tiếp lên `1.1.1.1`. Nhờ vậy máy chỉ đổi DNS cho Locket, không ảnh hưởng app khác. |
| **MITM 443** (SNI) | Cùng cổng 443 phục vụ 2 loại chứng chỉ: tên miền DoH dùng cert Let's Encrypt, các domain Locket dùng cert do Root CA nội bộ ký. |
| **RevenueCat gate theo username** | Chỉ tài khoản nằm trong danh sách đã kích mới được trả `entitlements.Gold` (kèm đúng hạn). Tài khoản khác đi thẳng ra RevenueCat thật. |
| **Firebase Remote Config mock** | Trả nguyên bộ cấu hình 179 key (feature gates, video 60s, 296 filter, 17 nhóm, caption…) trích từ `Locket_Unlocked.js`. |
| **`/generateFilteredImage`** | Nhận JSON của app, tải ảnh gốc, **ghép khung PhotoBooth** (2/3/4 ảnh), **ghép overlay AI**, **chỉnh màu theo preset**, xuất JPEG 1080×1080 và trả `output_url` đúng schema app cần. |
| **`/generateCaption`** | Sinh caption tiếng Việt theo ảnh + prompt (không cần API key ngoài). |
| **Reverse proxy** | Mọi request khác của `api.locketcamera.com` được chuyển nguyên vẹn ra server thật, nên feed/đăng ảnh/nhắn tin vẫn hoạt động bình thường. |
| **Trang kích hoạt** (`/kich-hoat`) | Khách nhập username Locket + mã → server ghi danh sách Gold và trả xác nhận hạn dùng. |
| **Trang quản trị** (`/admin`) | Tự tạo mã (hạn 7/30/90/180/365 ngày hoặc vĩnh viễn, số lượt, mã tự đặt), kích tay, gia hạn, thu hồi, xem ID thật app gọi lên. |
| **Profile `.mobileconfig`** | Tự sinh profile chứa Root CA + DoH split-DNS (4 domain; CDN ảnh đi trực tiếp để tiết kiệm băng thông). |

## Cài nhanh

```bash
git clone https://github.com/PhTrien/app.git
cd app
sudo bash scripts/install.sh --host doh.tenmiencuaban.com --ip 1.2.3.4 --email ban@email.com
```

Sau đó mở Safari trên iPhone: `https://doh.tenmiencuaban.com/p/locket.mobileconfig`

Installer in ra **mật khẩu admin** ở cuối. Trang quản trị: `https://doh.tenmiencuaban.com/admin`
(tạo mã kích hoạt, kích tay theo username, gia hạn, thu hồi).

Hướng dẫn chi tiết từng bước: **[HUONG-DAN-CAI-DAT.md](HUONG-DAN-CAI-DAT.md)**

## Lệnh

```bash
locket-gold init --host doh.x.com --ip 1.2.3.4   # tạo config + CA + profile
locket-gold run  --config /etc/locket-gold/config.json
locket-gold selftest                            # tự kiểm tra 16 mục
locket-gold ca show                             # thông tin Root CA
locket-gold admin reset --password MATKHAU_MOI  # đổi mật khẩu trang quản trị
locket-gold version
```

## API

| Endpoint | Mô tả |
|---|---|
| `GET /` | landing page + link cài profile |
| `GET/POST /kich-hoat` | khách nhập username + mã kích hoạt |
| `GET /admin` | trang quản trị (mật khẩu in ra khi cài) |
| `GET /version` | JSON trạng thái (host, CA, số filter, uptime) |
| `GET /healthz` | health check |
| `GET /p/locket.mobileconfig` | profile cài iPhone |
| `GET /ca.crt` | Root CA (.crt) |
| `GET/POST /dns-query` | DoH chuẩn RFC 8484 |
| `POST /generateFilteredImage` | render filter (cũng nhận qua host `api.locketcamera.com`) |
| `POST /generateCaption` | sinh caption |
| `GET /locket-filters/<uuid>.jpg` | ảnh đã render |

## Cấu trúc

```
cmd/locket-gold/        main + selftest
internal/config/        cấu hình JSON
internal/pki/           Root CA + chứng chỉ server (SAN, cache)
internal/doh/           DNS-over-HTTPS (miekg/dns)
internal/mitm/          reverse proxy ra server thật (resolver riêng)
internal/mock/          RevenueCat + Firebase Remote Config
internal/catalog/       đọc 296 filter / 17 nhóm / caption từ remote config
internal/filters/       engine render ảnh + preset màu
internal/caption/       sinh caption
internal/profile/       tạo .mobileconfig
internal/httpapi/       router theo Host + các handler
internal/store/         danh sách username Gold, mã kích hoạt, ID đã thấy (JSON 0600)
internal/assets/        entries.json (trích từ repo mhale)
bin/                    binary tĩnh linux amd64 / arm64
scripts/install.sh      cài đặt 1 lệnh
```

## Kích hoạt Gold theo username

1. Vào `https://<host>/admin`, đăng nhập bằng mật khẩu installer in ra.
2. Tab **Mã kích hoạt** → chọn thời hạn (30/365 ngày hoặc vĩnh viễn), số lượt, bấm **Tạo mã**.
   Có thể tự đặt mã riêng (ô "Mã tự đặt"), ví dụ `TET2026`.
3. Gửi khách mã + link `https://<host>/kich-hoat`.
4. Khách nhập username Locket (hoặc dán link `locket.cam/...`) + mã → hệ thống ghi vào danh sách.
5. Từ lúc đó, khi app gọi `api.revenuecat.com/v1/subscribers/<username>`, server trả Gold đúng hạn.
   Username lạ → chuyển nguyên request ra RevenueCat thật, không bị ảnh hưởng.

Khách không biết username? Tab **ID đã thấy** liệt kê các ID app thật sự gọi lên, bấm **Kích 1 năm** là map trực tiếp.

Dữ liệu (username, mã, hạn) nằm trong `/var/lib/locket-gold/data/store.json` — copy file này là đủ để backup/chuyển máy.

## Ghi chú kỹ thuật

- Cổng 443 dùng chung cho DoH và MITM, phân biệt bằng **SNI** → không cần 2 IP.
- Chứng chỉ server có SAN cho: `api.revenuecat.com`, `firebaseremoteconfig.googleapis.com`,
  `us-central1-locket-4252a.cloudfunctions.net`, `api.locketcamera.com`, `cdn.locketcamera.com`
  (CDN chỉ nằm trong SAN, không trỏ DoH về VPS → ảnh tải trực tiếp, không tốn băng thông).
- Ảnh filter lưu ở `/var/lib/locket-gold/public/locket-filters/`, tên UUID 36 ký tự đúng như app yêu cầu
  (`/locket-filters/[0-9a-f-]{36}.jpg`).
- Khung PhotoBooth/AI tải từ `huggingface.co` (đúng URL trong remote config) và cache ở `/var/lib/locket-gold/frames/`.
- Mật khẩu admin lưu dạng băm (SHA-256 + salt, 60k vòng) trong `store.json`; bản trong
  `config.json` chỉ dùng cho lần đăng nhập đầu và có thể đổi bằng `locket-gold admin reset`.
- Tuỳ chọn `gate_remote_config` (mặc định `false`): bật lên thì Firebase Remote Config chỉ trả bản
  mở khoá cho IP vừa xác thực Gold trong 6 giờ, còn lại trả cấu hình gốc.
- Không có API key nào được nhúng trong repo hay binary.

## License

Dùng cho mục đích cá nhân. Phần script client thuộc tác giả Mhale / duyvinh09 (giữ nguyên nguồn).
