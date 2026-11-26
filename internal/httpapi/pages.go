// Package httpapi — trang công khai: landing page và trang kích hoạt Gold.
package httpapi

import (
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/PhTrien/app/internal/store"
)

// pageFuncs là helper dùng trong template.
var pageFuncs = template.FuncMap{
	"date": func(t time.Time) string {
		if t.IsZero() {
			return "—"
		}
		return t.In(time.FixedZone("ICT", 7*3600)).Format("02/01/2006 15:04")
	},
	"day": func(t time.Time) string {
		if t.IsZero() {
			return "—"
		}
		return t.In(time.FixedZone("ICT", 7*3600)).Format("02/01/2006")
	},
	"expiry": func(t time.Time) string {
		if t.IsZero() {
			return "Vĩnh viễn"
		}
		return t.In(time.FixedZone("ICT", 7*3600)).Format("02/01/2006")
	},
	"left": func(t time.Time) string {
		if t.IsZero() {
			return "vĩnh viễn"
		}
		d := time.Until(t)
		if d <= 0 {
			return "đã hết hạn"
		}
		days := int(d.Hours() / 24)
		if days >= 1 {
			return strconv.Itoa(days) + " ngày"
		}
		return strconv.Itoa(int(d.Hours())) + " giờ"
	},
	"join":  func(list []string, sep string) string { return strings.Join(list, sep) },
	"money": func(n int) string { return strconv.Itoa(n) },
	"codeState": func(c *store.Code) stateInfo {
		now := time.Now()
		switch {
		case c.Revoked:
			return stateInfo{"Đã thu hồi", "bad"}
		case !c.Expires.IsZero() && now.After(c.Expires):
			return stateInfo{"Hết hạn", "bad"}
		case c.Uses >= c.MaxUses:
			return stateInfo{"Hết lượt", "off"}
		default:
			return stateInfo{"Còn dùng", "ok"}
		}
	},
	"userState": func(u *store.User) stateInfo {
		switch {
		case u.Revoked:
			return stateInfo{"Đã thu hồi", "bad"}
		case u.Expires.IsZero():
			return stateInfo{"Vĩnh viễn", "ok"}
		case u.Active(time.Now()):
			return stateInfo{"Đang hoạt động", "ok"}
		default:
			return stateInfo{"Hết hạn", "off"}
		}
	},
}

// pageTmpl gom toàn bộ template HTML của site.
var pageTmpl = template.Must(template.New("page").Funcs(pageFuncs).Parse(
	landingHTML + activateHTML + adminLoginHTML + adminDashHTML))

// baseCSS dùng chung cho mọi trang.
const baseCSS = `
*,*::before,*::after{box-sizing:border-box}
:root{
  --bg:#0a0a0a;--panel:#111110;--panel-2:#171613;
  --gold:#ffb000;--gold-2:#ffd166;--gold-deep:#8a5a00;
  --text:#f7f2e7;--muted:#a49c8d;--line:rgba(255,255,255,.10);
  --r:24px;--max:1120px;
}
html{scroll-behavior:smooth}
body{margin:0;min-height:100vh;color:var(--text);line-height:1.55;overflow-x:hidden;
  font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,"Helvetica Neue",Arial,"Noto Sans",sans-serif;
  -webkit-font-smoothing:antialiased;
  background:
    radial-gradient(900px 520px at 10% -10%,rgba(255,176,0,.18),transparent 62%),
    radial-gradient(760px 460px at 92% 0%,rgba(255,120,0,.12),transparent 60%),
    #0a0a0a;}
a{color:inherit;text-decoration:none}
img{max-width:100%}
.wrap{width:100%;max-width:var(--max);margin:0 auto;padding:0 22px}
.nav{position:sticky;top:0;z-index:50;backdrop-filter:blur(16px);-webkit-backdrop-filter:blur(16px);
  background:rgba(10,10,10,.74);border-bottom:1px solid var(--line)}
.nav-inner{display:flex;align-items:center;justify-content:space-between;gap:18px;height:68px}
.brand{display:flex;align-items:center;gap:10px;font-weight:800;letter-spacing:.02em}
.brand svg{width:34px;height:34px;flex:none}
.brand b{font-size:16px}
.brand span{color:var(--gold)}
.nav-links{display:flex;align-items:center;gap:26px;font-size:14px;color:var(--muted)}
.nav-links a:hover{color:var(--text)}
.nav-cta{display:inline-flex;align-items:center;gap:8px;padding:10px 18px;border-radius:12px;font-size:14px;
  font-weight:700;color:#1a1200;background:linear-gradient(180deg,#ffc63d,#ffb000 60%,#f59e00);
  box-shadow:0 10px 26px -14px rgba(255,176,0,.9)}
.btn{display:inline-flex;align-items:center;justify-content:center;gap:10px;padding:15px 26px;border-radius:16px;
  font-weight:700;font-size:15px;border:0;cursor:pointer;transition:transform .16s ease,box-shadow .16s ease;white-space:nowrap}
.btn-gold{background:linear-gradient(180deg,#ffc63d,#ffb000 58%,#f59e00);color:#1a1200;
  box-shadow:0 16px 34px -16px rgba(255,176,0,.95),inset 0 1px 0 rgba(255,255,255,.45)}
.btn-gold:hover{transform:translateY(-2px)}
.btn-ghost{background:rgba(255,255,255,.05);color:var(--text);border:1px solid var(--line)}
.btn-ghost:hover{background:rgba(255,255,255,.09)}
.btn-block{width:100%}
.pill{display:inline-flex;align-items:center;gap:8px;padding:7px 14px;border-radius:999px;font-size:12.5px;
  font-weight:700;letter-spacing:.04em;text-transform:uppercase;color:var(--gold);
  background:rgba(255,176,0,.10);border:1px solid rgba(255,176,0,.28)}
.pill .dot{width:7px;height:7px;border-radius:50%;background:var(--gold);box-shadow:0 0 10px var(--gold)}
.hero{position:relative;padding:64px 0 40px}
.hero-grid{display:grid;grid-template-columns:1.05fr .95fr;gap:40px;align-items:center}
.hero h1{font-size:clamp(34px,4.6vw,58px);line-height:1.05;letter-spacing:-.03em;margin:18px 0 16px;font-weight:850}
.hero h1 em{font-style:normal;background:linear-gradient(180deg,#ffd98a,#ffb000 60%,#e08a00);
  -webkit-background-clip:text;background-clip:text;color:transparent}
.hero p.lead{font-size:16.5px;color:var(--muted);max-width:520px;margin:0 0 26px}
.hero-cta{display:flex;flex-wrap:wrap;gap:12px}
.trust{display:flex;flex-wrap:wrap;gap:20px;margin-top:26px;color:var(--muted);font-size:13.5px}
.trust b{color:var(--text);display:block;font-size:14px}
.trust .t{display:flex;gap:10px;align-items:flex-start}
.trust svg{width:20px;height:20px;flex:none;margin-top:2px;color:var(--gold)}
/* phone mockup */
.stage{position:relative;display:flex;justify-content:center;padding:10px 0}
.glow{position:absolute;inset:12% 6% 6% 6%;background:radial-gradient(circle at 50% 40%,rgba(255,176,0,.30),transparent 66%);
  filter:blur(28px);border-radius:50%}
.phone{position:relative;width:288px;border-radius:44px;padding:10px;background:linear-gradient(160deg,#33302a,#121110 40%,#2a2723);
  box-shadow:0 40px 90px -30px rgba(0,0,0,.95),0 0 0 1px rgba(255,255,255,.08);transform:rotate(-3deg)}
.phone-inner{border-radius:36px;background:#070707;overflow:hidden;position:relative;padding:18px 16px 22px;min-height:470px}
.phone-notch{position:absolute;top:16px;left:50%;transform:translateX(-50%);width:88px;height:22px;border-radius:12px;background:#000;z-index:3}
.phone-head{display:flex;align-items:center;gap:8px;font-size:11px;font-weight:800;letter-spacing:.16em;color:#fff9ea;margin:26px 0 16px}
.phone-head i{width:18px;height:18px;border-radius:6px;background:linear-gradient(180deg,#ffc63d,#f59e00);display:inline-block}
.avatar{width:78px;height:78px;border-radius:50%;margin:6px auto 10px;display:grid;place-items:center;font-size:34px;
  background:radial-gradient(circle at 30% 25%,#ffe3a3,#ffb000 55%,#c67700);box-shadow:0 0 0 4px rgba(255,176,0,.18)}
.phone-name{text-align:center;font-size:14px;font-weight:700;color:#fff8e8}
.badge-gold{display:flex;align-items:center;justify-content:center;gap:6px;margin:8px auto 16px;width:fit-content;
  padding:5px 12px;border-radius:999px;font-size:11px;font-weight:800;letter-spacing:.08em;color:#2a1c00;
  background:linear-gradient(180deg,#ffd98a,#ffb000)}
.mini-grid{display:grid;grid-template-columns:1fr 1fr;gap:8px}
.mini{border-radius:14px;background:linear-gradient(150deg,rgba(255,176,0,.16),rgba(255,255,255,.04));
  border:1px solid rgba(255,255,255,.07);aspect-ratio:1/1.12;display:grid;place-items:end center;padding:8px;font-size:10px;color:#ffe9bd}
.sticker{position:absolute;font-size:30px;filter:drop-shadow(0 12px 18px rgba(0,0,0,.6));animation:float 6s ease-in-out infinite}
.sticker.s1{top:2%;left:-2%;animation-delay:.2s}
.sticker.s2{top:16%;right:-4%;font-size:26px;animation-delay:1.1s}
.sticker.s3{bottom:10%;left:-6%;font-size:34px;animation-delay:.7s}
.sticker.s4{bottom:22%;right:-2%;font-size:24px;animation-delay:1.6s}
@keyframes float{0%,100%{transform:translateY(0) rotate(-4deg)}50%{transform:translateY(-14px) rotate(5deg)}}
/* sections */
section.block{padding:64px 0}
.sec-head{max-width:640px;margin-bottom:34px}
.sec-head h2{font-size:clamp(26px,3vw,38px);letter-spacing:-.02em;margin:12px 0 10px;font-weight:820}
.sec-head p{color:var(--muted);margin:0;font-size:15.5px}
.grid-3{display:grid;grid-template-columns:repeat(3,1fr);gap:18px}
.grid-2{display:grid;grid-template-columns:repeat(2,1fr);gap:18px}
.card{background:linear-gradient(180deg,rgba(255,255,255,.055),rgba(255,255,255,.018));
  border:1px solid var(--line);border-radius:var(--r);padding:26px;transition:transform .18s ease,border-color .18s ease}
.card:hover{transform:translateY(-3px);border-color:rgba(255,176,0,.35)}
.card .ico{width:46px;height:46px;border-radius:14px;display:grid;place-items:center;font-size:22px;margin-bottom:16px;
  background:rgba(255,176,0,.12);border:1px solid rgba(255,176,0,.25)}
.card h3{margin:0 0 8px;font-size:17px}
.card p{margin:0;color:var(--muted);font-size:14.4px}
.steps{display:grid;grid-template-columns:repeat(4,1fr);gap:16px}
.step{position:relative;padding:24px 20px;border-radius:20px;background:rgba(255,255,255,.035);border:1px solid var(--line)}
.step .n{width:34px;height:34px;border-radius:11px;display:grid;place-items:center;font-weight:800;font-size:14px;
  color:#1a1200;background:linear-gradient(180deg,#ffd98a,#ffb000);margin-bottom:14px}
.step h4{margin:0 0 6px;font-size:15px}
.step p{margin:0;color:var(--muted);font-size:13.6px}
.plans{display:grid;grid-template-columns:repeat(3,1fr);gap:18px;align-items:stretch}
.plan{position:relative;display:flex;flex-direction:column;gap:16px;padding:28px;border-radius:26px;
  background:linear-gradient(180deg,rgba(255,255,255,.05),rgba(255,255,255,.015));border:1px solid var(--line)}
.plan.hot{border-color:rgba(255,176,0,.55);background:linear-gradient(180deg,rgba(255,176,0,.13),rgba(255,255,255,.02));
  box-shadow:0 30px 70px -40px rgba(255,176,0,.65)}
.plan .tag{position:absolute;top:-12px;left:24px;font-size:11px;font-weight:800;letter-spacing:.06em;text-transform:uppercase;
  padding:5px 12px;border-radius:999px;background:linear-gradient(180deg,#ffd98a,#ffb000);color:#211500}
.plan h3{margin:0;font-size:19px}
.plan .price{font-size:34px;font-weight:850;letter-spacing:-.02em}
.plan .price small{font-size:14px;color:var(--muted);font-weight:600;margin-left:6px}
.plan ul{list-style:none;margin:0;padding:0;display:grid;gap:10px;color:var(--muted);font-size:14px;flex:1}
.plan ul li{display:flex;gap:9px;align-items:flex-start}
.plan ul li b{color:var(--gold);font-weight:800}
.cta-band{margin:70px 0 40px;border-radius:32px;padding:44px 32px;text-align:center;position:relative;overflow:hidden;
  background:linear-gradient(135deg,#ffcf5c,#ffb000 45%,#e08a00);color:#1d1300}
.cta-band h2{margin:0 0 10px;font-size:clamp(24px,3vw,34px);letter-spacing:-.02em}
.cta-band p{margin:0 auto 22px;max-width:560px;color:#3a2a00;font-size:15.4px;font-weight:600}
.cta-band .btn{background:#0a0a0a;color:#fff}
.faq{display:grid;gap:12px;max-width:820px}
.faq details{border:1px solid var(--line);border-radius:16px;padding:16px 20px;background:rgba(255,255,255,.035)}
.faq summary{cursor:pointer;font-weight:650;list-style:none}
.faq summary::-webkit-details-marker{display:none}
.faq p{margin:10px 0 0;color:var(--muted);font-size:14.2px}
footer{border-top:1px solid var(--line);padding:30px 0 44px;color:var(--muted);font-size:13px}
.foot-inner{display:flex;flex-wrap:wrap;gap:14px;justify-content:space-between;align-items:center}
footer a{color:var(--muted)}
footer a:hover{color:var(--gold)}
/* forms */
.form-card{max-width:520px;margin:0 auto;padding:34px;border-radius:28px;
  background:linear-gradient(180deg,rgba(255,255,255,.06),rgba(255,255,255,.02));border:1px solid var(--line);
  box-shadow:0 40px 90px -50px rgba(0,0,0,.9)}
.form-card h1{font-size:26px;margin:0 0 8px;letter-spacing:-.02em}
.form-card p.sub{color:var(--muted);font-size:14.4px;margin:0 0 24px}
label{display:block;font-size:13px;font-weight:650;color:#d8d0c0;margin:0 0 7px}
input[type=text],input[type=password],input[type=number],textarea,select{
  width:100%;padding:14px 16px;border-radius:14px;background:#0d0d0c;border:1px solid var(--line);
  color:var(--text);font-size:15px;outline:none;transition:border-color .15s ease,box-shadow .15s ease;font-family:inherit}
input:focus,textarea:focus,select:focus{border-color:rgba(255,176,0,.6);box-shadow:0 0 0 4px rgba(255,176,0,.12)}
.field{margin-bottom:16px}
.hint{color:var(--muted);font-size:12.5px;margin-top:7px}
.alert{padding:13px 16px;border-radius:14px;font-size:14px;margin-bottom:18px;border:1px solid}
.alert.ok{background:rgba(34,197,94,.10);border-color:rgba(34,197,94,.35);color:#86efac}
.alert.err{background:rgba(239,68,68,.10);border-color:rgba(239,68,68,.35);color:#fca5a5}
.big-ok{text-align:center;padding:8px 0 4px}
.big-ok .ring{width:74px;height:74px;border-radius:50%;margin:0 auto 16px;display:grid;place-items:center;font-size:34px;
  background:radial-gradient(circle at 30% 25%,#ffe3a3,#ffb000 60%,#c67700);box-shadow:0 0 0 8px rgba(255,176,0,.12)}
.kv{display:grid;gap:10px;margin:22px 0 0;padding:18px;border-radius:16px;background:rgba(255,255,255,.04);border:1px solid var(--line)}
.kv div{display:flex;justify-content:space-between;gap:16px;font-size:14px}
.kv span{color:var(--muted)}
.kv b{text-align:right}
@media (max-width:900px){
  .hero-grid{grid-template-columns:1fr;gap:26px}
  .stage{order:-1;transform:scale(.94)}
  .grid-3,.plans,.steps,.grid-2{grid-template-columns:1fr 1fr}
  .nav-links{display:none}
}
@media (max-width:620px){
  .hero{padding:34px 0 20px}
  .grid-3,.plans,.steps,.grid-2{grid-template-columns:1fr}
  section.block{padding:44px 0}
  .form-card{padding:24px 20px}
  .phone{width:262px}
  .hero-cta .btn{width:100%}
}
`

const logoHeart = `<svg viewBox="0 0 32 32" fill="none" aria-hidden="true"><rect width="32" height="32" rx="9" fill="url(#g)"/><path d="M16 24.3c.18 0 .44-.12.63-.23 4.87-3.11 7.96-6.73 7.96-10.42 0-3.06-2.1-5.22-4.82-5.22-1.69 0-2.99.93-3.77 2.36-.77-1.42-2.09-2.36-3.77-2.36-2.72 0-4.82 2.16-4.82 5.22 0 3.69 3.09 7.31 7.97 10.42.18.11.44.23.62.23Z" fill="#C67700"/><defs><linearGradient id="g" x1="16" y1="0" x2="16" y2="32" gradientUnits="userSpaceOnUse"><stop stop-color="#FFB000"/><stop offset="1" stop-color="#FFB300"/></linearGradient></defs></svg>`

const siteNav = `
<header class="nav">
  <div class="wrap nav-inner">
    <a class="brand" href="/">
      ` + logoHeart + `
      <b>LOCKET <span>GOLD</span></b>
    </a>
    <nav class="nav-links">
      <a href="/#tinh-nang">Tính năng</a>
      <a href="/#cach-cai">Cách cài</a>
      <a href="/#goi">Gói</a>
      <a href="/#faq">Hỏi đáp</a>
    </nav>
    <a class="nav-cta" href="/kich-hoat">Kích hoạt</a>
  </div>
</header>`

const siteFooter = `
<footer>
  <div class="wrap foot-inner">
    <div>LOCKET GOLD · Locket Gold là dịch vụ độc lập, không liên kết với Locket Labs, Inc.</div>
    <div><a href="/kich-hoat">Kích hoạt</a> · <a href="/p/locket.mobileconfig">Profile</a> · <a href="/admin">Quản trị</a></div>
  </div>
</footer>`

// landingHTML là trang chủ.
const landingHTML = `
{{define "landing"}}<!doctype html>
<html lang="vi">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Locket Gold — Nâng cấp Locket Gold miễn phí</title>
<meta name="description" content="Nâng cấp Locket Gold: quay video 30s, đăng ảnh thư viện, caption dài, filter AI + PhotoBooth. Kích hoạt bằng username ngay trên web.">
<meta name="theme-color" content="#0a0a0a">
<style>` + baseCSS + `</style>
</head>
<body>
` + siteNav + `
<main>
  <section class="hero">
    <div class="wrap hero-grid">
      <div>
        <div class="pill"><span class="dot"></span>Server đang chạy · {{.Version}}</div>
        <h1>Nâng cấp <em>Locket Gold</em>, dùng ngay trên điện thoại</h1>
        <p class="lead">Quay video 30 giây, đăng ảnh từ thư viện, caption dài, filter AI và PhotoBooth. Kích hoạt bằng username Locket của bạn — không cần tải app thứ 3, không đụng tới iCloud.</p>
        <div class="hero-cta">
          <a class="btn btn-gold" href="/kich-hoat">Kích hoạt bằng mã →</a>
          <a class="btn btn-ghost" href="/p/locket.mobileconfig">Cài profile</a>
        </div>
        <div class="trust">
          <div class="t">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M20 6 9 17l-5-5"/></svg>
            <div><b>Không tải app thứ 3</b>Chỉ cần profile DNS</div>
          </div>
          <div class="t">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M20 6 9 17l-5-5"/></svg>
            <div><b>Không đụng tới iCloud</b>Tài khoản Apple an toàn</div>
          </div>
          <div class="t">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M20 6 9 17l-5-5"/></svg>
            <div><b>Không cần mật khẩu</b>Chỉ nhập username</div>
          </div>
        </div>
      </div>
      <div class="stage">
        <div class="glow"></div>
        <span class="sticker s1">💛</span>
        <span class="sticker s2">✨</span>
        <span class="sticker s3">🔒</span>
        <span class="sticker s4">⭐️</span>
        <div class="phone">
          <div class="phone-notch"></div>
          <div class="phone-inner">
            <div class="phone-head"><i></i>LOCKET</div>
            <div class="avatar">💛</div>
            <div class="phone-name">@{{.SampleUser}}</div>
            <div class="badge-gold">★ GOLD</div>
            <div class="mini-grid">
              <div class="mini">Video 30s</div>
              <div class="mini">Ảnh thư viện</div>
              <div class="mini">Caption dài</div>
              <div class="mini">PhotoBooth</div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </section>

  <section class="block" id="tinh-nang">
    <div class="wrap">
      <div class="sec-head">
        <div class="pill"><span class="dot"></span>Đặc quyền</div>
        <h2>Những gì bạn nhận được</h2>
        <p>Mở khoá toàn bộ quyền Locket Gold trên tài khoản của bạn, giữ nguyên giao diện gốc của app.</p>
      </div>
      <div class="grid-3">
        <div class="card"><div class="ico">🎥</div><h3>Video 30 giây</h3><p>Quay video dài tối đa 30s thay vì 15s, đăng thoải mái trong ngày.</p></div>
        <div class="card"><div class="ico">🖼️</div><h3>Đăng ảnh thư viện</h3><p>Chọn ảnh có sẵn trong máy, không bắt buộc chụp trực tiếp.</p></div>
        <div class="card"><div class="ico">👀</div><h3>Xem ai đã xem</h3><p>Theo dõi danh sách người xem ảnh và bạn bè không giới hạn.</p></div>
        <div class="card"><div class="ico">✍️</div><h3>Caption dài</h3><p>Viết chú thích dài hơn, không bị cắt ngắn khi đăng.</p></div>
        <div class="card"><div class="ico">🎨</div><h3>Filter AI + PhotoBooth</h3><p>Hơn 290 filter, khung ghép 2-4 ảnh, phong cách film, retro, anime.</p></div>
        <div class="card"><div class="ico">🚫</div><h3>Không quảng cáo</h3><p>Trải nghiệm sạch, mở khoá toàn bộ tính năng VIP của Locket Gold.</p></div>
      </div>
    </div>
  </section>

  <section class="block" id="cach-cai">
    <div class="wrap">
      <div class="sec-head">
        <div class="pill"><span class="dot"></span>4 bước</div>
        <h2>Cách cài trong 4 bước</h2>
        <p>Tất cả làm trên iPhone, mất khoảng 2 phút.</p>
      </div>
      <div class="steps">
        <div class="step"><div class="n">1</div><h4>Cài profile</h4><p>Mở link profile bằng Safari, bấm Cho phép rồi vào Cài đặt để cài.</p></div>
        <div class="step"><div class="n">2</div><h4>Bật tin cậy</h4><p>Cài đặt chung → Giới thiệu → Cài đặt tin cậy chứng nhận → bật CA.</p></div>
        <div class="step"><div class="n">3</div><h4>Nhận mã</h4><p>Liên hệ quản trị để nhận mã kích hoạt Gold.</p></div>
        <div class="step"><div class="n">4</div><h4>Kích hoạt</h4><p>Nhập username Locket + mã, mở lại app là có Gold.</p></div>
      </div>
    </div>
  </section>

  <section class="block" id="goi">
    <div class="wrap">
      <div class="sec-head">
        <div class="pill"><span class="dot"></span>Gói kích hoạt</div>
        <h2>Chọn thời hạn, nhập mã là xong</h2>
        <p>Mã do quản trị tạo cho từng người. Một mã kích cho một username.</p>
      </div>
      <div class="plans">
        <div class="plan">
          <h3>1 tháng</h3>
          <div class="price">Mã 30 ngày</div>
          <ul>
            <li><b>✓</b> Đầy đủ 6 đặc quyền Gold</li>
            <li><b>✓</b> Video 30s, ảnh thư viện</li>
            <li><b>✓</b> Filter AI + PhotoBooth</li>
            <li><b>✓</b> Hỗ trợ qua quản trị</li>
          </ul>
          <a class="btn btn-ghost btn-block" href="/kich-hoat">Tôi có mã</a>
        </div>
        <div class="plan hot">
          <span class="tag">Phổ biến</span>
          <h3>1 năm</h3>
          <div class="price">Mã 365 ngày</div>
          <ul>
            <li><b>✓</b> Đầy đủ 6 đặc quyền Gold</li>
            <li><b>✓</b> Video 30s, ảnh thư viện</li>
            <li><b>✓</b> Filter AI + PhotoBooth</li>
            <li><b>✓</b> Dùng ổn định cả năm</li>
          </ul>
          <a class="btn btn-gold btn-block" href="/kich-hoat">Kích hoạt ngay</a>
        </div>
        <div class="plan">
          <h3>Vĩnh viễn</h3>
          <div class="price">Không giới hạn<small>trả một lần</small></div>
          <ul>
            <li><b>✓</b> Đầy đủ 6 đặc quyền Gold</li>
            <li><b>✓</b> Không lo hết hạn</li>
            <li><b>✓</b> Filter AI + PhotoBooth</li>
            <li><b>✓</b> Cập nhật server miễn phí</li>
          </ul>
          <a class="btn btn-ghost btn-block" href="/kich-hoat">Tôi có mã</a>
        </div>
      </div>
    </div>
  </section>

  <section class="block" id="faq">
    <div class="wrap">
      <div class="sec-head">
        <div class="pill"><span class="dot"></span>Hỏi đáp</div>
        <h2>Câu hỏi thường gặp</h2>
      </div>
      <div class="faq">
        <details open><summary>Cài profile có ảnh hưởng gì tới máy không?</summary>
          <p>Profile chỉ đổi DNS cho 4 tên miền của Locket và cài một chứng nhận nội bộ. Toàn bộ lưu lượng khác vẫn đi bình thường, ảnh vẫn tải trực tiếp từ CDN.</p></details>
        <details><summary>Kích hoạt theo username nghĩa là gì?</summary>
          <p>Mỗi mã gắn với một username Locket. Server chỉ trả quyền Gold cho đúng username đã kích hoạt, các tài khoản khác dùng như bình thường.</p></details>
        <details><summary>Đổi máy có phải kích lại không?</summary>
          <p>Không. Quyền Gold gắn với username, bạn chỉ cần cài lại profile trên máy mới và đăng nhập đúng tài khoản.</p></details>
        <details><summary>Mã dùng được mấy lần?</summary>
          <p>Tuỳ mã quản trị tạo: có mã dùng 1 lần, có mã dùng nhiều lần. Khi nhập, hệ thống báo rõ mã còn hiệu lực hay không.</p></details>
      </div>
    </div>
  </section>

  <div class="wrap">
    <div class="cta-band">
      <h2>Sẵn sàng nâng cấp Locket Gold?</h2>
      <p>Nhập username và mã kích hoạt — quyền Gold bật ngay sau khi mở lại app.</p>
      <a class="btn btn-gold" href="/kich-hoat">Kích hoạt ngay</a>
    </div>
  </div>
</main>
` + siteFooter + `
</body>
</html>{{end}}`

// activateHTML là trang nhập username + mã kích hoạt.
const activateHTML = `
{{define "activate"}}<!doctype html>
<html lang="vi">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Kích hoạt Locket Gold</title>
<meta name="theme-color" content="#0a0a0a">
<style>` + baseCSS + `</style>
</head>
<body>
` + siteNav + `
<main style="padding:52px 0 70px">
  <div class="wrap">
    {{if .Done}}
    <div class="form-card">
      <div class="big-ok">
        <div class="ring">✓</div>
        <h1>Kích hoạt thành công</h1>
        <p class="sub">Mở lại Locket để nhận quyền Gold.</p>
      </div>
      <div class="kv">
        <div><span>Username</span><b>@{{.Username}}</b></div>
        <div><span>Hạn Gold</span><b>{{.ExpiryText}}</b></div>
        <div><span>Mã đã dùng</span><b>{{.Code}}</b></div>
      </div>
      <div style="margin-top:22px;display:grid;gap:10px">
        <a class="btn btn-gold btn-block" href="/p/locket.mobileconfig">Cài / cài lại profile</a>
        <a class="btn btn-ghost btn-block" href="/kich-hoat">Kích hoạt tài khoản khác</a>
      </div>
      <p class="hint" style="text-align:center;margin-top:16px">Nếu app chưa nhận, hãy tắt hẳn Locket rồi mở lại sau 10 giây.</p>
    </div>
    {{else}}
    <div class="form-card">
      <div class="pill" style="margin-bottom:14px"><span class="dot"></span>Kích hoạt Gold</div>
      <h1>Nhập username và mã</h1>
      <p class="sub">Dùng đúng username Locket của bạn (phần sau <b>locket.cam/</b>). Mỗi mã kích cho một tài khoản.</p>
      {{if .Error}}<div class="alert err">{{.Error}}</div>{{end}}
      <form method="post" action="/kich-hoat">
        <div class="field">
          <label for="u">Username Locket</label>
          <input id="u" type="text" name="username" placeholder="vd: nguyenvana hoặc link locket.cam/nguyenvana" value="{{.Username}}" autocomplete="off" autocapitalize="none" spellcheck="false">
          <div class="hint">Có thể dán nguyên link profile Locket, hệ thống tự tách username.</div>
        </div>
        <div class="field">
          <label for="c">Mã kích hoạt</label>
          <input id="c" type="text" name="code" placeholder="LG-XXXX-XXXX" value="{{.Code}}" autocomplete="off" autocapitalize="characters" spellcheck="false" style="letter-spacing:.08em;text-transform:uppercase">
        </div>
        <button class="btn btn-gold btn-block" type="submit">Kích hoạt ngay</button>
      </form>
      <p class="hint" style="margin-top:18px">Chưa có mã? Liên hệ quản trị viên của server này để được cấp mã.</p>
    </div>
    {{end}}
  </div>
</main>
` + siteFooter + `
</body>
</html>{{end}}`

func (s *Server) serveLanding(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pageTmpl.ExecuteTemplate(w, "landing", map[string]any{
		"Version":    s.Version,
		"Host":       s.Cfg.Host,
		"SampleUser": "locket.user",
	})
}

func (s *Server) serveActivate(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{
		"Done":     false,
		"Username": strings.TrimSpace(r.URL.Query().Get("username")),
		"Code":     strings.TrimSpace(r.URL.Query().Get("code")),
	}
	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			data["Error"] = "Không đọc được dữ liệu gửi lên."
		} else {
			username := r.PostFormValue("username")
			code := r.PostFormValue("code")
			u, err := s.Store.Redeem(code, username)
			if err != nil {
				data["Error"] = err.Error()
				data["Username"] = username
				data["Code"] = code
			} else {
				data["Done"] = true
				data["Username"] = u.Name
				data["Code"] = code
				data["ExpiryText"] = "Vĩnh viễn"
				if !u.Expires.IsZero() {
					data["ExpiryText"] = u.Expires.In(time.FixedZone("ICT", 7*3600)).Format("02/01/2006")
				}
			}
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = pageTmpl.ExecuteTemplate(w, "activate", data)
}
