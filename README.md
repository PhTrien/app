# Locket Gold Server

Server tự dựng cho **Locket Gold** trên VPS: DoH + MITM + render filter/caption, không cần Shadowrocket.
Dựa trên bộ script client của repo [`PhTrien/mhale`](https://github.com/PhTrien/mhale) (module, `Locket_Unlocked.js`,
`Locket_Mhale.js`, `Locket_Filters.js`, `deleteHeader.js`) và biến nó thành một server độc lập.

## Nó làm gì

| Thành phần | Chức năng |
|---|---|
| **DoH server** (`/dns-query`) | Trả lời DNS qua HTTPS. Chỉ những domain của Locket được trỏ về VPS, phần còn lại chuyển tiếp lên `1.1.1.1`. Nhờ vậy máy chỉ đổi DNS cho Locket, không ảnh hưởng app khác. |
| **MITM 443** (SNI) | Cùng cổng 443 phục vụ 2 loại chứng chỉ: tên miền DoH dùng cert Let's Encrypt, các domain Locket dùng cert do Root CA nội bộ ký. |
| **RevenueCat mock** | Trả `subscriber.entitlements.Gold` còn hạn tới năm 9999 → mở khoá Gold. |
| **Firebase Remote Config mock** | Trả nguyên bộ cấu hình 179 key (feature gates, video 60s, 296 filter, 17 nhóm, caption…) trích từ `Locket_Unlocked.js`. |
| **`/generateFilteredImage`** | Nhận JSON của app, tải ảnh gốc, **ghép khung PhotoBooth** (2/3/4 ảnh), **ghép overlay AI**, **chỉnh màu theo preset**, xuất JPEG 1080×1080 và trả `output_url` đúng schema app cần. |
| **`/generateCaption`** | Sinh caption tiếng Việt theo ảnh + prompt (không cần API key ngoài). |
| **Reverse proxy** | Mọi request khác của `api.locketcamera.com` được chuyển nguyên vẹn ra server thật, nên feed/đăng ảnh/nhắn tin vẫn hoạt động bình thường. |
| **Profile `.mobileconfig`** | Tự sinh profile chứa Root CA + DoH split-DNS (5 domain) để cài trên iPhone. |

## Cài nhanh

```bash
git clone https://github.com/PhTrien/app.git
cd app
sudo bash scripts/install.sh --host doh.tenmiencuaban.com --ip 1.2.3.4 --email ban@email.com
```

Sau đó mở Safari trên iPhone: `https://doh.tenmiencuaban.com/p/locket.mobileconfig`

Hướng dẫn chi tiết từng bước: **[HUONG-DAN-CAI-DAT.md](HUONG-DAN-CAI-DAT.md)**

## Lệnh

```bash
locket-gold init --host doh.x.com --ip 1.2.3.4   # tạo config + CA + profile
locket-gold run  --config /etc/locket-gold/config.json
locket-gold selftest                            # tự kiểm tra 10 mục
locket-gold ca show                             # thông tin Root CA
locket-gold version
```

## API

| Endpoint | Mô tả |
|---|---|
| `GET /` | trang trạng thái + link cài profile |
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
internal/assets/        entries.json (trích từ repo mhale)
bin/                    binary tĩnh linux amd64 / arm64
scripts/install.sh      cài đặt 1 lệnh
```

## Ghi chú kỹ thuật

- Cổng 443 dùng chung cho DoH và MITM, phân biệt bằng **SNI** → không cần 2 IP.
- Chứng chỉ server có SAN cho: `api.revenuecat.com`, `firebaseremoteconfig.googleapis.com`,
  `us-central1-locket-4252a.cloudfunctions.net`, `api.locketcamera.com`, `cdn.locketcamera.com`.
- Ảnh filter lưu ở `/var/lib/locket-gold/public/locket-filters/`, tên UUID 36 ký tự đúng như app yêu cầu
  (`/locket-filters/[0-9a-f-]{36}.jpg`).
- Khung PhotoBooth/AI tải từ `huggingface.co` (đúng URL trong remote config) và cache ở `/var/lib/locket-gold/frames/`.
- Không có API key nào được nhúng trong repo hay binary.

## License

Dùng cho mục đích cá nhân. Phần script client thuộc tác giả Mhale / duyvinh09 (giữ nguyên nguồn).
