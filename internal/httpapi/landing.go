package httpapi

import (
	"context"
	"html/template"
	"net/http"
	"time"
)

// contextWithTimeout tạo context có thời hạn từ request.
func contextWithTimeout(r *http.Request, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), d)
}

var landingTmpl = template.Must(template.New("landing").Parse(`<!doctype html>
<html lang="vi">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Locket Gold — DoH Server</title>
<style>
  :root { color-scheme: dark; }
  * { box-sizing: border-box; }
  body { margin: 0; min-height: 100vh; font-family: -apple-system, "Segoe UI", Roboto, sans-serif;
         background: radial-gradient(1200px 600px at 15% -10%, #ff3b8d22, transparent 60%),
                     radial-gradient(900px 500px at 90% 0%, #6d5cff26, transparent 55%), #0b0b12;
         color: #eef0f6; display: flex; align-items: center; justify-content: center; padding: 28px; }
  .card { width: 100%; max-width: 620px; background: #14141dee; border: 1px solid #ffffff14;
          border-radius: 22px; padding: 28px; box-shadow: 0 24px 60px #00000066; }
  h1 { margin: 0 0 6px; font-size: 26px; letter-spacing: -.4px; }
  .sub { color: #a3a7b8; font-size: 14px; margin-bottom: 22px; }
  .pill { display: inline-flex; align-items: center; gap: 8px; font-size: 12px; padding: 5px 10px;
          border-radius: 999px; background: #22c55e1a; color: #4ade80; border: 1px solid #22c55e33; margin-bottom: 18px; }
  a.btn { display: flex; align-items: center; justify-content: space-between; gap: 12px; text-decoration: none;
          color: #fff; background: linear-gradient(135deg, #ff3b8d, #6d5cff); padding: 15px 18px;
          border-radius: 14px; font-weight: 600; margin-bottom: 12px; }
  a.ghost { display: block; text-decoration: none; color: #cdd2e4; background: #ffffff0a;
            border: 1px solid #ffffff12; padding: 12px 16px; border-radius: 12px; margin-bottom: 10px; font-size: 14px; }
  a.ghost span { color: #8b90a4; font-size: 13px; }
  .steps { margin-top: 22px; border-top: 1px solid #ffffff0f; padding-top: 18px; }
  .steps h2 { font-size: 15px; margin: 0 0 10px; color: #cdd2e4; }
  ol { margin: 0; padding-left: 20px; color: #a3a7b8; font-size: 14px; line-height: 1.85; }
  code { background: #ffffff0f; padding: 2px 6px; border-radius: 6px; font-size: 13px; color: #ffd6e7; }
  .foot { margin-top: 18px; color: #6b7086; font-size: 12px; text-align: center; }
</style>
</head>
<body>
  <div class="card">
    <div class="pill">● Server đang chạy · {{.Version}}</div>
    <h1>Locket Gold</h1>
    <div class="sub">{{.Host}} · DoH + MITM · {{.Filters}} filter · {{.CA}}</div>

    <a class="btn" href="/p/locket.mobileconfig">Cài profile Locket Gold <span>↧</span></a>
    <a class="ghost" href="/ca.crt">Tải Root CA <span>(.crt)</span></a>
    <a class="ghost" href="/version">Trạng thái server <span>(JSON)</span></a>

    <div class="steps">
      <h2>Cài trên iPhone</h2>
      <ol>
        <li>Mở link profile bằng <b>Safari</b> → Cài đặt.</li>
        <li>Vào <code>Cài đặt → Đã tải về hồ sơ</code> → Cài đặt.</li>
        <li>Vào <code>Cài đặt → Cài đặt chung → Giới thiệu → Cài đặt tin cậy chứng nhận</code> → bật CA.</li>
        <li>Mở Locket → kiểm tra Gold.</li>
      </ol>
    </div>
    <div class="foot">Locket Gold Server · {{.ClientIP}}</div>
  </div>
</body>
</html>`))

func (s *Server) serveLanding(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = landingTmpl.Execute(w, map[string]any{
		"Version":  s.Version,
		"Host":     s.Cfg.Host,
		"Filters":  len(s.Catalog.Order),
		"CA":       s.CA.Subject(),
		"ClientIP": r.RemoteAddr,
	})
}
