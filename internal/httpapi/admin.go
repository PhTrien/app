// Package httpapi — trang quản trị: tạo mã kích hoạt, quản lý username, xem ID thật.
package httpapi

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/PhTrien/app/internal/store"
)

const adminCSS = `
.a-top{position:sticky;top:0;z-index:40;backdrop-filter:blur(14px);background:rgba(10,10,10,.82);border-bottom:1px solid var(--line)}
.a-top .wrap{display:flex;align-items:center;justify-content:space-between;gap:16px;height:64px}
.a-user{display:flex;align-items:center;gap:12px;font-size:13.5px;color:var(--muted)}
.stats{display:grid;grid-template-columns:repeat(4,1fr);gap:14px;margin:26px 0 24px}
.stat{padding:18px 20px;border-radius:18px;background:linear-gradient(180deg,rgba(255,255,255,.055),rgba(255,255,255,.02));border:1px solid var(--line)}
.stat .k{font-size:12px;color:var(--muted);text-transform:uppercase;letter-spacing:.06em}
.stat .v{font-size:26px;font-weight:800;margin-top:6px;letter-spacing:-.02em}
.tabs{display:flex;gap:8px;flex-wrap:wrap;margin-bottom:18px}
.tab{padding:9px 16px;border-radius:12px;font-size:14px;font-weight:650;color:var(--muted);background:rgba(255,255,255,.04);border:1px solid var(--line)}
.tab.on{color:#1a1200;background:linear-gradient(180deg,#ffd98a,#ffb000);border-color:transparent}
.panel{border-radius:22px;border:1px solid var(--line);background:rgba(255,255,255,.025);padding:22px;margin-bottom:22px}
.panel h2{margin:0 0 4px;font-size:17px}
.panel .desc{color:var(--muted);font-size:13.5px;margin:0 0 18px}
.row{display:grid;gap:12px;grid-template-columns:repeat(4,1fr);align-items:end}
.row.row-3{grid-template-columns:repeat(3,1fr)}
.row.row-2{grid-template-columns:1fr 1fr}
.row .field{margin:0}
.mini-btn{padding:7px 11px;border-radius:9px;font-size:12.5px;font-weight:650;cursor:pointer;border:1px solid var(--line);
  background:rgba(255,255,255,.06);color:var(--text);white-space:nowrap}
.mini-btn:hover{background:rgba(255,255,255,.12)}
.mini-btn.gold{background:linear-gradient(180deg,#ffd98a,#ffb000);color:#1a1200;border-color:transparent}
.mini-btn.danger{color:#fca5a5;border-color:rgba(239,68,68,.35);background:rgba(239,68,68,.10)}
.tbl-wrap{overflow-x:auto;margin-top:6px}
table{width:100%;border-collapse:collapse;font-size:13.4px;min-width:720px}
th{text-align:left;color:var(--muted);font-weight:600;font-size:11.5px;text-transform:uppercase;letter-spacing:.06em;
  padding:10px 12px;border-bottom:1px solid var(--line);white-space:nowrap}
td{padding:12px;border-bottom:1px solid rgba(255,255,255,.05);vertical-align:middle}
tr:hover td{background:rgba(255,255,255,.025)}
.mono{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:13px;letter-spacing:.04em}
.pill-s{display:inline-block;padding:4px 10px;border-radius:999px;font-size:11.5px;font-weight:700;white-space:nowrap}
.pill-s.ok{background:rgba(34,197,94,.14);color:#86efac;border:1px solid rgba(34,197,94,.3)}
.pill-s.warn{background:rgba(255,176,0,.14);color:#ffd166;border:1px solid rgba(255,176,0,.32)}
.pill-s.off{background:rgba(255,255,255,.07);color:#b6afa3;border:1px solid var(--line)}
.pill-s.bad{background:rgba(239,68,68,.12);color:#fca5a5;border:1px solid rgba(239,68,68,.3)}
.acts{display:flex;gap:6px;flex-wrap:wrap}
.empty{padding:26px;text-align:center;color:var(--muted);font-size:13.5px}
.note{color:var(--muted);font-size:13.2px;line-height:1.7}
.note code{background:rgba(255,255,255,.08);padding:2px 6px;border-radius:6px;font-size:12.5px}
.login-wrap{min-height:100vh;display:grid;place-items:center;padding:24px}
@media (max-width:820px){.stats{grid-template-columns:1fr 1fr}.row,.row.row-3,.row.row-2{grid-template-columns:1fr}}
`

const adminLoginHTML = `
{{define "adminLogin"}}<!doctype html>
<html lang="vi">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Quản trị · Locket Gold</title>
<meta name="theme-color" content="#0a0a0a">
<style>` + baseCSS + adminCSS + `</style>
</head>
<body>
<div class="login-wrap">
  <div class="form-card">
    <div class="pill" style="margin-bottom:14px"><span class="dot"></span>Quản trị</div>
    <h1>Đăng nhập</h1>
    <p class="sub">Trang quản trị tạo mã kích hoạt và quản lý username Gold.</p>
    {{if .Error}}<div class="alert err">{{.Error}}</div>{{end}}
    <form method="post" action="/admin/login">
      <div class="field">
        <label for="p">Mật khẩu quản trị</label>
        <input id="p" type="password" name="password" autofocus autocomplete="current-password" placeholder="••••••••">
      </div>
      <button class="btn btn-gold btn-block" type="submit">Vào quản trị</button>
    </form>
    <p class="hint" style="margin-top:16px">Quên mật khẩu: chạy <code>locket-gold admin reset</code> trên VPS.</p>
  </div>
</div>
</body>
</html>{{end}}`

const adminDashHTML = `
{{define "adminDash"}}<!doctype html>
<html lang="vi">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Quản trị · Locket Gold</title>
<meta name="theme-color" content="#0a0a0a">
<style>` + baseCSS + adminCSS + `</style>
</head>
<body>
<div class="a-top">
  <div class="wrap">
    <a class="brand" href="/">` + logoHeart + `<b>LOCKET <span>GOLD</span></b></a>
    <div class="a-user">
      <span>{{.Cfg.Host}} · {{.Version}}</span>
      <form method="post" action="/admin/logout"><button class="mini-btn" type="submit">Đăng xuất</button></form>
    </div>
  </div>
</div>

<div class="wrap">
  {{if .Msg}}<div class="alert ok" style="margin-top:20px">{{.Msg}}</div>{{end}}
  {{if .Err}}<div class="alert err" style="margin-top:20px">{{.Err}}</div>{{end}}

  <div class="stats">
    <div class="stat"><div class="k">Username đã kích</div><div class="v">{{.Stats.Users}}</div></div>
    <div class="stat"><div class="k">Đang hoạt động</div><div class="v">{{.Stats.ActiveUsers}}</div></div>
    <div class="stat"><div class="k">Sắp hết hạn (7 ngày)</div><div class="v">{{.Stats.Expiring}}</div></div>
    <div class="stat"><div class="k">Mã còn dùng được</div><div class="v">{{.Stats.UnusedCodes}}/{{.Stats.Codes}}</div></div>
  </div>

  <div class="tabs">
    <a class="tab {{if eq .Tab "codes"}}on{{end}}" href="/admin?tab=codes">Mã kích hoạt</a>
    <a class="tab {{if eq .Tab "users"}}on{{end}}" href="/admin?tab=users">Người dùng</a>
    <a class="tab {{if eq .Tab "seen"}}on{{end}}" href="/admin?tab=seen">ID đã thấy ({{.SeenCount}})</a>
    <a class="tab {{if eq .Tab "guide"}}on{{end}}" href="/admin?tab=guide">Hướng dẫn</a>
  </div>

{{if eq .Tab "codes"}}
  <div class="panel">
    <h2>Tạo mã kích hoạt</h2>
    <p class="desc">Mỗi mã gắn thời hạn Gold. Để trống ô "mã tự đặt" thì hệ thống sinh mã ngẫu nhiên.</p>
    <form method="post" action="/admin/codes">
      <div class="row">
        <div class="field"><label>Số lượng</label><input type="number" name="count" value="1" min="1" max="200"></div>
        <div class="field"><label>Thời hạn Gold</label>
          <select name="days">
            <option value="7">7 ngày</option>
            <option value="30" selected>30 ngày</option>
            <option value="90">90 ngày</option>
            <option value="180">180 ngày</option>
            <option value="365">365 ngày</option>
            <option value="0">Vĩnh viễn</option>
          </select>
        </div>
        <div class="field"><label>Số lượt dùng / mã</label><input type="number" name="uses" value="1" min="1" max="1000"></div>
        <div class="field"><label>Ghi chú</label><input type="text" name="note" placeholder="vd: khách Zalo, đợt 1"></div>
      </div>
      <div class="row row-2" style="margin-top:12px">
        <div class="field"><label>Mã tự đặt (tuỳ chọn)</label><input type="text" name="custom" placeholder="vd: TET2026" autocomplete="off" style="text-transform:uppercase"></div>
        <div class="field"><label>Hạn của mã (ngày, 0 = không hạn)</label><input type="number" name="ttl" value="0" min="0" max="3650"></div>
      </div>
      <div style="margin-top:16px"><button class="btn btn-gold" type="submit">Tạo mã</button></div>
    </form>
  </div>

  <div class="panel">
    <h2>Danh sách mã</h2>
    <p class="desc">Copy mã gửi khách. Thu hồi = khách không kích được nữa, người đã kích vẫn giữ.</p>
    <div class="tbl-wrap">
    <table>
      <thead><tr><th>Mã</th><th>Thời hạn</th><th>Lượt</th><th>Trạng thái</th><th>Người dùng</th><th>Tạo</th><th></th></tr></thead>
      <tbody>
      {{range .Codes}}
        <tr>
          <td><span class="mono">{{.Code}}</span></td>
          <td>{{if eq .Days 0}}Vĩnh viễn{{else}}{{.Days}} ngày{{end}}</td>
          <td>{{.Uses}}/{{.MaxUses}}</td>
          <td>{{with codeState .}}<span class="pill-s {{.Class}}">{{.Label}}</span>{{end}}</td>
          <td>{{if .Users}}{{join .Users ", "}}{{else}}<span style="color:var(--muted)">—</span>{{end}}</td>
          <td>{{day .Created}}</td>
          <td>
            <div class="acts">
              <button class="mini-btn" type="button" data-code="{{.Code}}">Copy</button>
              <form method="post" action="/admin/codes/action">
                <input type="hidden" name="code" value="{{.Code}}">
                <input type="hidden" name="action" value="{{if .Revoked}}unrevoke{{else}}revoke{{end}}">
                <button class="mini-btn" type="submit">{{if .Revoked}}Mở lại{{else}}Thu hồi{{end}}</button>
              </form>
              <form method="post" action="/admin/codes/action" onsubmit="return confirm('Xoá mã này?')">
                <input type="hidden" name="code" value="{{.Code}}">
                <input type="hidden" name="action" value="delete">
                <button class="mini-btn danger" type="submit">Xoá</button>
              </form>
            </div>
          </td>
        </tr>
      {{else}}
        <tr><td colspan="7"><div class="empty">Chưa có mã nào — tạo ở khung phía trên.</div></td></tr>
      {{end}}
      </tbody>
    </table>
    </div>
  </div>
{{end}}

{{if eq .Tab "users"}}
  <div class="panel">
    <h2>Kích Gold thủ công</h2>
    <p class="desc">Dùng khi khách đã thanh toán nhưng không nhập mã. Nhập username hoặc ID RevenueCat.</p>
    <form method="post" action="/admin/users">
      <div class="row row-3">
        <div class="field"><label>Username / ID</label><input type="text" name="username" placeholder="nguyenvana" required></div>
        <div class="field"><label>Thời hạn</label>
          <select name="days">
            <option value="30">30 ngày</option>
            <option value="90">90 ngày</option>
            <option value="365" selected>365 ngày</option>
            <option value="0">Vĩnh viễn</option>
          </select>
        </div>
        <div class="field"><label>Ghi chú</label><input type="text" name="note" placeholder="tuỳ chọn"></div>
      </div>
      <div style="margin-top:16px"><button class="btn btn-gold" type="submit">Kích Gold</button></div>
    </form>
  </div>

  <div class="panel">
    <h2>Username đã kích</h2>
    <p class="desc">Gold chỉ bật cho đúng các username này. Xoá = app trở về trạng thái thật.</p>
    <div class="tbl-wrap">
    <table>
      <thead><tr><th>Username</th><th>Hạn</th><th>Trạng thái</th><th>Mã</th><th>Lượt gọi</th><th>Ngày tạo</th><th></th></tr></thead>
      <tbody>
      {{range .Users}}
        <tr>
          <td><b>@{{.Name}}</b>{{if .Note}}<div style="color:var(--muted);font-size:12px">{{.Note}}</div>{{end}}</td>
          <td>{{expiry .Expires}}{{if .Expires}}<div style="color:var(--muted);font-size:12px">còn {{left .Expires}}</div>{{end}}</td>
          <td>{{with userState .}}<span class="pill-s {{.Class}}">{{.Label}}</span>{{end}}</td>
          <td class="mono">{{if .Code}}{{.Code}}{{else}}—{{end}}</td>
          <td>{{.Hits}}{{if .LastHit}}<div style="color:var(--muted);font-size:12px">{{date .LastHit}}</div>{{end}}</td>
          <td>{{day .Created}}</td>
          <td>
            <div class="acts">
              <form method="post" action="/admin/users/action">
                <input type="hidden" name="key" value="{{.Key}}"><input type="hidden" name="action" value="extend">
                <input type="hidden" name="days" value="30"><button class="mini-btn" type="submit">+30 ngày</button>
              </form>
              <form method="post" action="/admin/users/action">
                <input type="hidden" name="key" value="{{.Key}}"><input type="hidden" name="action" value="forever">
                <button class="mini-btn" type="submit">Vĩnh viễn</button>
              </form>
              <form method="post" action="/admin/users/action">
                <input type="hidden" name="key" value="{{.Key}}"><input type="hidden" name="action" value="{{if .Revoked}}unrevoke{{else}}revoke{{end}}">
                <button class="mini-btn" type="submit">{{if .Revoked}}Bật lại{{else}}Thu hồi{{end}}</button>
              </form>
              <form method="post" action="/admin/users/action" onsubmit="return confirm('Xoá username này?')">
                <input type="hidden" name="key" value="{{.Key}}"><input type="hidden" name="action" value="delete">
                <button class="mini-btn danger" type="submit">Xoá</button>
              </form>
            </div>
          </td>
        </tr>
      {{else}}
        <tr><td colspan="7"><div class="empty">Chưa có ai được kích. Tạo mã rồi gửi khách, hoặc kích tay ở trên.</div></td></tr>
      {{end}}
      </tbody>
    </table>
    </div>
  </div>
{{end}}

{{if eq .Tab "seen"}}
  <div class="panel">
    <h2>ID RevenueCat đã thấy</h2>
    <p class="desc">Mỗi lần app gọi RevenueCat, server ghi lại ID. Nếu ID này khác username, bấm "Kích 1 năm" để map trực tiếp.</p>
    <div class="tbl-wrap">
    <table>
      <thead><tr><th>ID</th><th>Số lần</th><th>Lần cuối</th><th>Thiết bị</th><th></th></tr></thead>
      <tbody>
      {{range .Seen}}
        <tr>
          <td class="mono">{{.ID}}</td>
          <td>{{.Count}}</td>
          <td>{{date .Last}}</td>
          <td style="color:var(--muted);font-size:12px;max-width:260px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">{{.UA}}</td>
          <td>
            <form method="post" action="/admin/users">
              <input type="hidden" name="username" value="{{.ID}}">
              <input type="hidden" name="days" value="365">
              <input type="hidden" name="note" value="kích từ ID đã thấy">
              <button class="mini-btn gold" type="submit">Kích 1 năm</button>
            </form>
          </td>
        </tr>
      {{else}}
        <tr><td colspan="5"><div class="empty">Chưa có ID nào. Mở Locket trên máy đã cài profile để server ghi nhận.</div></td></tr>
      {{end}}
      </tbody>
    </table>
    </div>
  </div>
{{end}}

{{if eq .Tab "guide"}}
  <div class="panel">
    <h2>Cách vận hành</h2>
    <div class="note">
      <p><b>1. Tạo mã</b> — tab Mã kích hoạt, chọn thời hạn (30/365 ngày hoặc vĩnh viễn), bấm Tạo mã. Có thể tự đặt mã riêng.</p>
      <p><b>2. Gửi khách</b> — khách mở <code>https://{{.Cfg.Host}}/kich-hoat</code>, nhập username Locket + mã.</p>
      <p><b>3. Gold bật theo username</b> — server chỉ trả quyền Gold cho username nằm trong danh sách; tài khoản khác đi thẳng ra RevenueCat thật.</p>
      <p><b>4. Khách chưa biết username?</b> — tab "ID đã thấy" hiện ID thật app gọi lên, bấm Kích 1 năm là xong.</p>
      <p><b>5. Hết hạn</b> — Gold tự tắt đúng ngày. Gia hạn bằng nút +30 ngày hoặc Vĩnh viễn.</p>
      <p><b>6. Sao lưu</b> — toàn bộ dữ liệu ở <code>{{.StorePath}}</code>, copy file này là đủ.</p>
    </div>
  </div></div>
{{end}}
</div>
<script>
document.querySelectorAll('[data-code]').forEach(function(b){
  b.addEventListener('click',function(){
    var t=b.getAttribute('data-code');
    navigator.clipboard.writeText(t).then(function(){
      var old=b.textContent; b.textContent='Đã copy'; b.classList.add('gold');
      setTimeout(function(){b.textContent=old; b.classList.remove('gold')},1200);
    });
  });
});
</script>
</body>
</html>{{end}}`

// stateInfo là nhãn + màu trạng thái cho bảng admin.
type stateInfo struct {
	Label string
	Class string
}

// serveAdmin điều phối mọi route /admin.
func (s *Server) serveAdmin(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSuffix(r.URL.Path, "/")
	if path == "" {
		path = "/"
	}
	switch {
	case path == "/admin/login" && r.Method == http.MethodPost:
		s.adminLogin(w, r)
	case path == "/admin/logout" && r.Method == http.MethodPost:
		http.SetCookie(w, &http.Cookie{Name: adminCookie, Value: "", Path: "/", MaxAge: -1})
		if c, err := r.Cookie(adminCookie); err == nil {
			s.Store.DropSession(c.Value)
		}
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
	case path == "/admin/codes" && r.Method == http.MethodPost:
		s.adminCreateCodes(w, r)
	case path == "/admin/codes/action" && r.Method == http.MethodPost:
		s.adminCodeAction(w, r)
	case path == "/admin/users" && r.Method == http.MethodPost:
		s.adminAddUser(w, r)
	case path == "/admin/users/action" && r.Method == http.MethodPost:
		s.adminUserAction(w, r)
	default:
		s.adminPage(w, r)
	}
}

const adminCookie = "lg_admin"

func (s *Server) adminAuthed(r *http.Request) bool {
	c, err := r.Cookie(adminCookie)
	if err != nil {
		return false
	}
	return s.Store.SessionValid(c.Value)
}

func (s *Server) adminPasswordOK(pw string) bool {
	if s.Store.HasPassword() {
		return s.Store.CheckPassword(pw)
	}
	return s.Cfg.AdminPassword != "" && pw == s.Cfg.AdminPassword
}

func (s *Server) adminLogin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || !s.adminPasswordOK(r.PostFormValue("password")) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		_ = pageTmpl.ExecuteTemplate(w, "adminLogin", map[string]any{
			"Error": "Mật khẩu không đúng.",
		})
		return
	}
	hours := s.Cfg.SessionHours
	if hours <= 0 {
		hours = 24
	}
	tok := s.Store.NewSession(time.Duration(hours) * time.Hour)
	http.SetCookie(w, &http.Cookie{
		Name:     adminCookie,
		Value:    tok,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   hours * 3600,
	})
	http.Redirect(w, r, "/admin?tab=codes", http.StatusSeeOther)
}

func (s *Server) adminPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !s.adminAuthed(r) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = pageTmpl.ExecuteTemplate(w, "adminLogin", map[string]any{"Error": ""})
		return
	}
	tab := r.URL.Query().Get("tab")
	switch tab {
	case "users", "seen", "guide":
	default:
		tab = "codes"
	}
	data := map[string]any{
		"Tab":       tab,
		"Msg":       r.URL.Query().Get("msg"),
		"Err":       r.URL.Query().Get("err"),
		"Stats":     s.Store.Stats(),
		"Users":     s.Store.Users(),
		"Codes":     s.Store.Codes(),
		"Seen":      s.Store.SeenList(),
		"SeenCount": len(s.Store.SeenList()),
		"Cfg":       s.Cfg,
		"Version":   s.Version,
		"StorePath": s.Store.Path(),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pageTmpl.ExecuteTemplate(w, "adminDash", data)
}

func (s *Server) flash(w http.ResponseWriter, r *http.Request, tab, msg, errMsg string) {
	q := url.Values{}
	q.Set("tab", tab)
	if msg != "" {
		q.Set("msg", msg)
	}
	if errMsg != "" {
		q.Set("err", errMsg)
	}
	http.Redirect(w, r, "/admin?"+q.Encode(), http.StatusSeeOther)
}

func (s *Server) adminCreateCodes(w http.ResponseWriter, r *http.Request) {
	if !s.adminAuthed(r) {
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.flash(w, r, "codes", "", "Dữ liệu gửi lên không hợp lệ.")
		return
	}
	atoi := func(v string, def int) int {
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return def
		}
		return n
	}
	opt := store.CreateOptions{
		Count:   atoi(r.PostFormValue("count"), 1),
		Days:    atoi(r.PostFormValue("days"), 30),
		MaxUses: atoi(r.PostFormValue("uses"), 1),
		TTLDays: atoi(r.PostFormValue("ttl"), 0),
		Note:    strings.TrimSpace(r.PostFormValue("note")),
		Custom:  strings.TrimSpace(r.PostFormValue("custom")),
	}
	codes, err := s.Store.CreateCodes(opt)
	if err != nil {
		s.flash(w, r, "codes", "", err.Error())
		return
	}
	msg := "Đã tạo " + strconv.Itoa(len(codes)) + " mã: "
	for i, c := range codes {
		if i > 0 {
			msg += ", "
		}
		msg += c.Code
	}
	s.flash(w, r, "codes", msg, "")
}

func (s *Server) adminCodeAction(w http.ResponseWriter, r *http.Request) {
	if !s.adminAuthed(r) {
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.flash(w, r, "codes", "", "Dữ liệu không hợp lệ.")
		return
	}
	code := r.PostFormValue("code")
	var err error
	msg := "Đã cập nhật mã " + code
	switch r.PostFormValue("action") {
	case "revoke":
		err = s.Store.RevokeCode(code, true)
		msg = "Đã thu hồi mã " + code
	case "unrevoke":
		err = s.Store.RevokeCode(code, false)
		msg = "Đã mở lại mã " + code
	case "delete":
		err = s.Store.DeleteCode(code)
		msg = "Đã xoá mã " + code
	default:
		s.flash(w, r, "codes", "", "Hành động không hợp lệ.")
		return
	}
	if err != nil {
		s.flash(w, r, "codes", "", err.Error())
		return
	}
	s.flash(w, r, "codes", msg, "")
}

func (s *Server) adminAddUser(w http.ResponseWriter, r *http.Request) {
	if !s.adminAuthed(r) {
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.flash(w, r, "users", "", "Dữ liệu không hợp lệ.")
		return
	}
	days, _ := strconv.Atoi(strings.TrimSpace(r.PostFormValue("days")))
	u, err := s.Store.AddUser(r.PostFormValue("username"), days, strings.TrimSpace(r.PostFormValue("note")))
	if err != nil {
		s.flash(w, r, "users", "", err.Error())
		return
	}
	s.flash(w, r, "users", "Đã kích Gold cho @"+u.Name, "")
}

func (s *Server) adminUserAction(w http.ResponseWriter, r *http.Request) {
	if !s.adminAuthed(r) {
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.flash(w, r, "users", "", "Dữ liệu không hợp lệ.")
		return
	}
	key := r.PostFormValue("key")
	var err error
	switch r.PostFormValue("action") {
	case "extend":
		days, _ := strconv.Atoi(r.PostFormValue("days"))
		if days <= 0 {
			days = 30
		}
		err = s.Store.Extend(key, days)
	case "forever":
		err = s.Store.Extend(key, 0)
	case "revoke":
		err = s.Store.RevokeUser(key, true)
	case "unrevoke":
		err = s.Store.RevokeUser(key, false)
	case "delete":
		err = s.Store.DeleteUser(key)
	default:
		s.flash(w, r, "users", "", "Hành động không hợp lệ.")
		return
	}
	if err != nil {
		s.flash(w, r, "users", "", err.Error())
		return
	}
	s.flash(w, r, "users", "Đã cập nhật.", "")
}
