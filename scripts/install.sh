#!/usr/bin/env bash
# ============================================================
#  Locket Gold Server — script cài đặt tự động trên Ubuntu/Debian
#  Dùng: sudo bash install.sh --host doh.example.com --ip 1.2.3.4
# ============================================================
set -euo pipefail

APP_NAME="locket-gold"
BIN_PATH="/usr/local/bin/${APP_NAME}"
CONF_DIR="/etc/${APP_NAME}"
CONF_FILE="${CONF_DIR}/config.json"
DATA_DIR="/var/lib/${APP_NAME}"
SERVICE="/etc/systemd/system/${APP_NAME}.service"
REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

HOST=""
IP=""
EMAIL=""
CA_NAME="Locket Gold CA"
DO_CERTBOT=1
PORT="443"

log()  { printf '\033[1;36m==>\033[0m %s\n' "$*"; }
ok()   { printf '\033[1;32m  ✓\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m  !\033[0m %s\n' "$*"; }
die()  { printf '\033[1;31mLỖI:\033[0m %s\n' "$*" >&2; exit 1; }

usage() {
  cat <<EOF
Cài Locket Gold Server

  sudo bash install.sh --host doh.example.com --ip 1.2.3.4 [tuỳ chọn]

Tuỳ chọn:
  --host <domain>     tên miền DoH (bắt buộc), ví dụ doh.mhale.io.vn
  --ip <ip>           IP công khai của VPS (bắt buộc)
  --email <email>     email cho Let's Encrypt (tự xin chứng chỉ TLS)
  --port <port>       cổng HTTPS, mặc định 443
  --ca-name <name>    tên Root CA hiển thị trên iPhone
  --no-certbot        không tự xin chứng chỉ (dùng CA nội bộ)
  --upgrade           chỉ cập nhật binary + service, giữ nguyên dữ liệu
  --uninstall         gỡ cài đặt (giữ thư mục dữ liệu)
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --host) HOST="${2:-}"; shift 2 ;;
    --ip) IP="${2:-}"; shift 2 ;;
    --email) EMAIL="${2:-}"; shift 2 ;;
    --port) PORT="${2:-}"; shift 2 ;;
    --ca-name) CA_NAME="${2:-}"; shift 2 ;;
    --no-certbot) DO_CERTBOT=0; shift ;;
    --upgrade) UPGRADE=1; shift ;;
    --uninstall) UNINSTALL=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) die "tham số không hiểu: $1 (xem --help)" ;;
  esac
done

[[ $EUID -eq 0 ]] || die "cần chạy bằng root: sudo bash install.sh ..."

# ---------- gỡ cài đặt ----------
if [[ "${UNINSTALL:-0}" == "1" ]]; then
  log "Gỡ ${APP_NAME}"
  systemctl disable --now "${APP_NAME}" 2>/dev/null || true
  rm -f "$SERVICE" "$BIN_PATH"
  systemctl daemon-reload || true
  ok "Đã gỡ. Dữ liệu vẫn còn ở ${DATA_DIR} (xoá tay nếu không cần)."
  exit 0
fi

# ---------- cập nhật binary ----------
install_binary() {
  local src=""
  case "$(uname -m)" in
    x86_64|amd64) src="${REPO_DIR}/bin/${APP_NAME}-linux-amd64" ;;
    aarch64|arm64) src="${REPO_DIR}/bin/${APP_NAME}-linux-arm64" ;;
    *) die "kiến trúc không hỗ trợ: $(uname -m)" ;;
  esac
  if [[ -x "$src" ]]; then
    install -m 0755 "$src" "$BIN_PATH"
    ok "Cài binary có sẵn: $(basename "$src")"
    return
  fi
  if command -v go >/dev/null 2>&1; then
    log "Không thấy binary có sẵn — build từ mã nguồn"
    ( cd "$REPO_DIR" && CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o "$BIN_PATH" ./cmd/locket-gold )
    ok "Build xong: $("$BIN_PATH" version)"
    return
  fi
  die "không có binary cho máy này và cũng không có Go để build"
}

if [[ "${UPGRADE:-0}" == "1" ]]; then
  log "Cập nhật ${APP_NAME} (giữ nguyên dữ liệu)"
  install_binary
  systemctl restart "${APP_NAME}"
  ok "Đã cập nhật. Kiểm tra: systemctl status ${APP_NAME}"
  UP_HOST="$(python3 - "$CONF_FILE" <<'PYEOF'
import json,sys
try:
    print(json.load(open(sys.argv[1]))["host"])
except Exception:
    print("TEN_MIEN")
PYEOF
)"
  echo
  echo "  Trang quản trị: https://${UP_HOST}/admin"
  echo "  Nếu quên mật khẩu: ${APP_NAME} admin reset --password MATKHAU_MOI"
  echo
  exit 0
fi

[[ -n "$HOST" && -n "$IP" ]] || { usage; die "thiếu --host hoặc --ip"; }

log "1/6 Kiểm tra hệ thống"
command -v systemctl >/dev/null || die "máy này không dùng systemd"
ok "$(uname -srm)"

log "2/6 Cài binary"
install_binary

log "3/6 Tạo cấu hình"
mkdir -p "$CONF_DIR" "$DATA_DIR"
"$BIN_PATH" init --config "$CONF_FILE" --host "$HOST" --ip "$IP" \
  --ca-name "$CA_NAME" --data-dir "$DATA_DIR"

# sửa cổng nếu khác 443
if [[ "$PORT" != "443" ]]; then
  python3 - "$CONF_FILE" "$PORT" <<'PY'
import json,sys
p,port=sys.argv[1],sys.argv[2]
d=json.load(open(p)); d["listen"]=f":{port}"; json.dump(d,open(p,"w"),indent=2,ensure_ascii=False)
PY
  ok "Cổng HTTPS: ${PORT}"
fi

log "4/6 Xin chứng chỉ TLS cho ${HOST}"
if [[ "$DO_CERTBOT" == "1" && -n "$EMAIL" ]]; then
  if [[ -f "/etc/letsencrypt/live/${HOST}/fullchain.pem" ]]; then
    ok "Đã có chứng chỉ Let's Encrypt cho ${HOST}"
  else
    export DEBIAN_FRONTEND=noninteractive
    apt-get update -qq
    apt-get install -y -qq certbot >/dev/null
    warn "Certbot cần cổng 80 trống và DNS đã trỏ về ${IP}"
    if certbot certonly --standalone -d "$HOST" --email "$EMAIL" --agree-tos --non-interactive --keep-until-expiring; then
      ok "Đã lấy chứng chỉ: /etc/letsencrypt/live/${HOST}/"
    else
      warn "Không lấy được chứng chỉ — server vẫn chạy bằng CA nội bộ"
    fi
  fi
else
  warn "Bỏ qua Let's Encrypt (không có --email hoặc --no-certbot). DoH sẽ dùng CA nội bộ."
fi

log "5/6 Tạo service systemd"
cat > "$SERVICE" <<EOF
[Unit]
Description=Locket Gold Server (DoH + MITM + Filters)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=${BIN_PATH} run --config ${CONF_FILE}
Restart=always
RestartSec=3
LimitNOFILE=65535
AmbientCapabilities=CAP_NET_BIND_SERVICE
NoNewPrivileges=true
ProtectSystem=full
PrivateTmp=true
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable "${APP_NAME}" >/dev/null 2>&1
systemctl restart "${APP_NAME}"
sleep 2

log "6/6 Tự kiểm tra"
if "$BIN_PATH" selftest | tail -20; then
  ok "Server đã chạy"
else
  warn "Tự kiểm tra có lỗi — xem: journalctl -u ${APP_NAME} -n 50"
fi

if command -v ufw >/dev/null 2>&1 && ufw status | grep -q "Status: active"; then
  ufw allow 443/tcp >/dev/null 2>&1 || true
  ufw allow 80/tcp >/dev/null 2>&1 || true
  ok "Đã mở cổng 443/80 trên ufw"
fi

cat <<EOF

──────────────────────────────────────────────
 XONG. Các bước tiếp theo trên iPhone:
   1. Mở Safari:  https://${HOST}/p/locket.mobileconfig
   2. Cài đặt → Đã tải về hồ sơ → Cài đặt
   3. Cài đặt chung → Giới thiệu → Cài đặt tin cậy chứng nhận → bật "${CA_NAME}"
   4. Mở Locket và kiểm tra Gold

 Tạo mã kích hoạt Gold (theo username):
   Mở https://${HOST}/admin  →  đăng nhập bằng mật khẩu admin ở trên
   Tab "Mã kích hoạt" → chọn thời hạn → Tạo mã → gửi khách
   Khách vào https://${HOST}/kich-hoat nhập username + mã

 Lệnh hữu ích:
   systemctl status ${APP_NAME}        # trạng thái
   journalctl -u ${APP_NAME} -f        # log trực tiếp
   ${APP_NAME} selftest                # tự kiểm tra
   ${APP_NAME} ca show                 # thông tin Root CA
   ${APP_NAME} admin reset --password MỚI   # đổi mật khẩu quản trị
──────────────────────────────────────────────
EOF
