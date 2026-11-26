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
	"os"
	"path/filepath"
	"strings"

	"github.com/miekg/dns"

	"github.com/PhTrien/app/internal/assets"
	"github.com/PhTrien/app/internal/catalog"
	"github.com/PhTrien/app/internal/config"
	"github.com/PhTrien/app/internal/filters"
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
	url := ts.URL + "/dns-query?dns=" + base64.RawURLEncoding.EncodeToString(packed)
	req, _ := http.NewRequest(http.MethodGet, url, nil)
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

	// 4. RevenueCat mock
	rcResp := doRequest(ts, cfg.Host, "api.revenuecat.com", "/v1/subscribers/test-user", nil)
	var rcBody struct {
		Subscriber struct {
			Entitlements map[string]any `json:"entitlements"`
			Subscriptions map[string]any `json:"subscriptions"`
		} `json:"subscriber"`
	}
	if err := json.Unmarshal(rcResp, &rcBody); err != nil {
		fail("RevenueCat JSON", err)
	} else if _, has := rcBody.Subscriber.Entitlements["Gold"]; !has {
		fail("RevenueCat có Gold", fmt.Errorf("thiếu entitlement"))
	} else {
		pass("RevenueCat Gold", fmt.Sprintf("%d subscription", len(rcBody.Subscriber.Subscriptions)))
	}

	// 5. Firebase Remote Config
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

	// 6. Catalog
	cat, err := catalog.Load(assets.EntriesJSON)
	if err != nil {
		fail("catalog filter", err)
	} else {
		pass("catalog filter", fmt.Sprintf("%d filter / %d nhóm", len(cat.Order), len(cat.Groups)))
	}

	// 7. Render filter (dùng ảnh test nội bộ)
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

	// 8. PhotoBooth cần nhiều ảnh → pending rồi đủ
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

	// 9. Caption
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

