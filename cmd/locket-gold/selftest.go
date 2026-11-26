package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/miekg/dns"

	"github.com/PhTrien/app/internal/assets"
	"github.com/PhTrien/app/internal/catalog"
	"github.com/PhTrien/app/internal/config"
	"github.com/PhTrien/app/internal/filters"
	"github.com/PhTrien/app/internal/store"
)

// selftestCmd chạy kiểm tra end-to-end ngay trên máy chủ (không cần iPhone).
func selftestCmd(args []string) {
	_ = args
	ok := true
	pass := func(name string, detail string) {
		fmt.Printf("  ✓ %-38s %s\n", name, detail)
	}
	fail := func(name string, err error) {
		ok = false
		fmt.Printf("  ✗ %-38s %v\n", name, err)
	}

	tmp, err := os.MkdirTemp("", "locket-gold-selftest-")
	if err != nil {
		log.Fatalf("tạo thư mục tạm: %v", err)
	}
	defer os.RemoveAll(tmp)

	cfg := config.Default()
	cfg.Host = "test.local"
	cfg.ServerIP = "127.0.0.1"
	cfg.DataDir = filepath.Join(tmp, "data")
	cfg.LogRequests = false
	cfg.AdminPassword = "selftest-pass"

	logger := log.New(io.Discard, "", 0)
	b, err := buildBundle(cfg, logger)
	if err != nil {
		log.Fatalf("dựng server: %v", err)
	}

	fmt.Println("Locket Gold — tự kiểm tra")
	fmt.Println()

	// 1. CA + chứng chỉ server
	if err := b.ca.Verify(b.leaf); err != nil {
		fail("chứng chỉ server do CA ký", err)
	} else {
		pass("chứng chỉ server do CA ký", b.ca.Subject())
	}
	sans := strings.Join(b.srv.Cfg.MappedHosts, ", ")
	pass("SAN của chứng chỉ", trim(sans, 60))

	// 2. Profile
	prof := string(b.srv.Profile)
	checks := []string{"com.apple.security.root", "com.apple.dnsSettings.managed", "https://test.local/dns-query"}
	for _, c := range checks {
		if !strings.Contains(prof, c) {
			fail("profile chứa "+c, fmt.Errorf("không thấy"))
			continue
		}
	}
	for _, h := range cfg.MappedHosts {
		if !strings.Contains(prof, "<string>"+h+"</string>") {
			fail("profile có domain "+h, fmt.Errorf("thiếu"))
		}
	}
	pass("profile .mobileconfig", fmt.Sprintf("%d bytes, %d domain", len(prof), len(cfg.MappedHosts)))

	// 3. DoH
	ts := httptest.NewUnstartedServer(b.srv)
	ts.Start()
	defer ts.Close()

	q := new(dns.Msg)
	q.SetQuestion("api.revenuecat.com.", dns.TypeA)
	packed, _ := q.Pack()
	dohURL := ts.URL + "/dns-query?dns=" + base64.RawURLEncoding.EncodeToString(packed)
	req, _ := http.NewRequest(http.MethodGet, dohURL, nil)
	req.Host = cfg.Host
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fail("DoH trả lời", err)
	} else {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		var answer dns.Msg
		if err := answer.Unpack(body); err != nil {
			fail("DoH parse", err)
		} else if len(answer.Answer) == 0 {
			fail("DoH trả A record", fmt.Errorf("không có answer"))
		} else if a, okA := answer.Answer[0].(*dns.A); !okA || a.A.String() != cfg.ServerIP {
			fail("DoH trả đúng IP", fmt.Errorf("nhận %v", answer.Answer[0]))
		} else {
			pass("DoH api.revenuecat.com", "→ "+a.A.String())
		}
	}

	// 4. Tạo mã + kích Gold theo username
	codes, err := b.srv.Store.CreateCodes(store.CreateOptions{Count: 1, Days: 30, MaxUses: 1, Note: "selftest"})
	if err != nil || len(codes) == 0 {
		fail("tạo mã kích hoạt", err)
	}
	code := codes[0].Code
	u, err := b.srv.Store.Redeem(code, "@selftest.user")
	if err != nil {
		fail("kích Gold bằng mã", err)
	} else {
		pass("kích Gold bằng mã", fmt.Sprintf("%s → @%s (hạn %s)", code, u.Name, u.Expires.Format("02/01/2006")))
	}
	if _, reused := b.srv.Store.Redeem(code, "nguoikhac"); reused == nil {
		fail("mã chỉ dùng 1 lần", fmt.Errorf("vẫn kích được lần 2"))
	} else {
		pass("mã chỉ dùng 1 lần", reused.Error())
	}

	// 5. RevenueCat: username đã kích được trả Gold, username lạ thì không
	rcResp := doRequest(ts, cfg.Host, "api.revenuecat.com", "/v1/subscribers/selftest.user", nil)
	var rcBody struct {
		Subscriber struct {
			Entitlements map[string]struct {
				Expires string `json:"expires_date"`
			} `json:"entitlements"`
			Subscriptions map[string]any `json:"subscriptions"`
		} `json:"subscriber"`
	}
	if err := json.Unmarshal(rcResp, &rcBody); err != nil {
		fail("RevenueCat JSON", err)
	} else if ent, has := rcBody.Subscriber.Entitlements["Gold"]; !has {
		fail("RevenueCat có Gold cho username đã kích", fmt.Errorf("thiếu entitlement: %s", trim(string(rcResp), 90)))
	} else if strings.HasPrefix(ent.Expires, "9999") {
		fail("hạn Gold theo mã", fmt.Errorf("vẫn là vĩnh viễn"))
	} else {
		pass("RevenueCat Gold theo username", fmt.Sprintf("%d subscription, hạn %s", len(rcBody.Subscriber.Subscriptions), ent.Expires[:10]))
	}
	if _, okGold := b.srv.Store.GoldFor("nguoi.chua.kich"); okGold {
		fail("username chưa kích không có Gold", fmt.Errorf("vẫn trả Gold"))
	} else {
		pass("username chưa kích", "đi thẳng ra RevenueCat thật")
	}

	// 6. Firebase Remote Config
	rfResp := doRequest(ts, cfg.Host, "firebaseremoteconfig.googleapis.com", "/v1/projects/x/namespaces/firebase:fetch", []byte(`{}`))
	var rc struct {
		Entries map[string]any `json:"entries"`
		State   string         `json:"state"`
	}
	if err := json.Unmarshal(rfResp, &rc); err != nil {
		fail("Remote Config JSON", err)
	} else if len(rc.Entries) < 100 {
		fail("Remote Config entries", fmt.Errorf("chỉ có %d entry", len(rc.Entries)))
	} else if _, parseErr := json.Marshal(rc.Entries["camera_filter_config"]); parseErr != nil {
		fail("Remote Config filter config", parseErr)
	} else {
		pass("Remote Config", fmt.Sprintf("%d entry, state=%s", len(rc.Entries), rc.State))
	}

	// 7. Catalog
	cat, err := catalog.Load(assets.EntriesJSON)
	if err != nil {
		fail("catalog filter", err)
	} else {
		pass("catalog filter", fmt.Sprintf("%d filter / %d nhóm", len(cat.Order), len(cat.Groups)))
	}

	// 8. Render filter (dùng ảnh test nội bộ)
	srcSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		img := image.NewRGBA(image.Rect(0, 0, 800, 1000))
		for y := 0; y < 1000; y++ {
			for x := 0; x < 800; x++ {
				img.Set(x, y, color.RGBA{uint8(x / 4), uint8(y / 5), 180, 255})
			}
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_ = jpeg.Encode(w, img, &jpeg.Options{Quality: 85})
	}))
	defer srcSrv.Close()

	firstFilterID := ""
	for _, id := range b.srv.Catalog.Order {
		f, _ := b.srv.Catalog.Get(id)
		if !f.IsPhotobooth() {
			firstFilterID = id
			break
		}
	}
	res, err := b.srv.Engine.Render(context.Background(), filters.Input{
		FilterID:  firstFilterID,
		ImageURLs: []string{srcSrv.URL + "/photo.jpg"},
		UserKey:   "selftest",
	})
	if err != nil {
		fail("render filter "+firstFilterID, err)
	} else if res.Pending {
		fail("render filter", fmt.Errorf("trả pending ngoài dự kiến"))
	} else {
		name := filepath.Base(res.URL)
		path := filepath.Join(b.srv.Cfg.PublicRoot(), "locket-filters", name)
		f, err := os.Open(path)
		if err != nil {
			fail("ảnh filter ghi ra đĩa", err)
		} else {
			cfgImg, _, err := image.Decode(f)
			f.Close()
			if err != nil {
				fail("ảnh filter giải mã", err)
			} else if cfgImg.Bounds().Dx() != 1080 || cfgImg.Bounds().Dy() != 1080 {
				fail("ảnh filter 1080x1080", fmt.Errorf("nhận %v", cfgImg.Bounds().Size()))
			} else {
				pass("render filter "+firstFilterID, fmt.Sprintf("1080x1080, màu=%s, khung=%v", res.Grade, res.Framed))
			}
		}
	}

	// 9. PhotoBooth cần nhiều ảnh → pending rồi đủ
	pbID := ""
	for _, id := range b.srv.Catalog.Order {
		f, _ := b.srv.Catalog.Get(id)
		if f.ExpectedShots() == 4 {
			pbID = id
			break
		}
	}
	if pbID != "" {
		r1, err1 := b.srv.Engine.Render(context.Background(), filters.Input{FilterID: pbID, ImageURLs: []string{srcSrv.URL + "/shot1.jpg"}, UserKey: "pb"})
		r2, _ := b.srv.Engine.Render(context.Background(), filters.Input{FilterID: pbID, ImageURLs: []string{srcSrv.URL + "/shot2.jpg"}, UserKey: "pb"})
		if err1 != nil {
			fail("photobooth "+pbID, err1)
		} else if !r1.Pending || r1.Total != 4 || r2.Captured != 2 {
			fail("photobooth pending", fmt.Errorf("lần 1 captured=%d, lần 2 captured=%d", r1.Captured, r2.Captured))
		} else {
			pass("photobooth "+pbID, "pending 1/4 → 2/4 đúng")
		}
	}

	// 10. Caption
	capResp := doRequest(ts, cfg.Host, "api.locketcamera.com", "/generateCaption",
		[]byte(`{"data":{"image_base64":"aGVsbG8=","prompt":"ảnh đi biển"}}`))
	var capBody struct {
		Result struct {
			Caption string `json:"caption"`
		} `json:"result"`
	}
	if err := json.Unmarshal(capResp, &capBody); err != nil || capBody.Result.Caption == "" {
		fail("generateCaption", fmt.Errorf("không có caption: %s", trim(string(capResp), 80)))
	} else {
		pass("generateCaption", capBody.Result.Caption)
	}

	// 11. Trang kích hoạt + trang admin
	actCode, _ := b.srv.Store.CreateCodes(store.CreateOptions{Count: 1, Days: 365, MaxUses: 1})
	if len(actCode) > 0 {
		form := url.Values{}
		form.Set("username", "khach.moi")
		form.Set("code", actCode[0].Code)
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/kich-hoat", strings.NewReader(form.Encode()))
		req.Host = cfg.Host
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			fail("trang kích hoạt", err)
		} else {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if !strings.Contains(string(body), "Kích hoạt thành công") {
				fail("trang kích hoạt", fmt.Errorf("không thấy xác nhận: %s", trim(string(body), 80)))
			} else {
				pass("trang /kich-hoat", "kích thành công @khach.moi")
			}
		}
	}

	loginForm := url.Values{}
	loginForm.Set("password", cfg.AdminPassword)
	loginReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/admin/login", strings.NewReader(loginForm.Encode()))
	loginReq.Host = cfg.Host
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	jarClient := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	loginResp, err := jarClient.Do(loginReq)
	if err != nil {
		fail("đăng nhập admin", err)
	} else {
		var cookie string
		for _, c := range loginResp.Cookies() {
			if c.Name == "lg_admin" {
				cookie = c.Value
			}
		}
		loginResp.Body.Close()
		if cookie == "" {
			fail("đăng nhập admin", fmt.Errorf("không có cookie, mã %d", loginResp.StatusCode))
		} else {
			admReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/admin?tab=codes", nil)
			admReq.Host = cfg.Host
			admReq.AddCookie(&http.Cookie{Name: "lg_admin", Value: cookie})
			admResp, err := http.DefaultClient.Do(admReq)
			if err != nil {
				fail("trang admin", err)
			} else {
				body, _ := io.ReadAll(admResp.Body)
				admResp.Body.Close()
				if !strings.Contains(string(body), "Tạo mã kích hoạt") {
					fail("trang admin", fmt.Errorf("thiếu giao diện: %s", trim(string(body), 80)))
				} else {
					pass("trang /admin", fmt.Sprintf("%d bytes, đăng nhập + danh sách mã", len(body)))
				}
			}
		}
	}

	landResp := doRequest(ts, cfg.Host, cfg.Host, "/", nil)
	if !strings.Contains(string(landResp), "LOCKET") {
		fail("landing page", fmt.Errorf("không render"))
	} else {
		pass("landing page", fmt.Sprintf("%d bytes", len(landResp)))
	}

	fmt.Println()
	if ok {
		fmt.Println("KẾT QUẢ: OK — server sẵn sàng.")
		return
	}
	fmt.Println("KẾT QUẢ: CÓ LỖI — xem các dòng ✗ ở trên.")
	os.Exit(1)
}

func doRequest(ts *httptest.Server, dohHost, host, path string, body []byte) []byte {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, _ := http.NewRequest(http.MethodPost, ts.URL+path, reader)
	req.Host = host
	req.Header.Set("Content-Type", "application/json")
	if body == nil {
		req.Method = http.MethodGet
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return out
}

func trim(s string, n int) string {
	if len([]rune(s)) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}
