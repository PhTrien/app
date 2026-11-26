// Package filters render ảnh cho endpoint /generateFilteredImage của Locket.
package filters

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	"github.com/PhTrien/app/internal/catalog"
)

const canvasSize = 1080

// Input là yêu cầu render từ app.
type Input struct {
	FilterID  string
	ImageURLs []string
	Prompt    string
	UserKey   string
}

// Result là kết quả trả về app.
type Result struct {
	URL      string
	Pending  bool
	Captured int
	Total    int
	Filter   catalog.Filter
	Grade    string
	Framed   bool
}

// Engine render ảnh: ghép khung, chỉnh màu, xuất JPEG 1080x1080.
type Engine struct {
	Catalog  *catalog.Catalog
	Host     string
	OutDir   string
	FrameDir string
	SessDir  string
	MaxBytes int64
	Logger   *log.Logger

	client *http.Client

	mu       sync.Mutex
	inflight map[string]bool
}

// New tạo engine.
func New(cat *catalog.Catalog, host, outDir, frameDir, sessDir string, logger *log.Logger) *Engine {
	for _, d := range []string{outDir, frameDir, sessDir} {
		_ = os.MkdirAll(d, 0o755)
	}
	return &Engine{
		Catalog:  cat,
		Host:     host,
		OutDir:   outDir,
		FrameDir: frameDir,
		SessDir:  sessDir,
		MaxBytes: 30 << 20,
		Logger:   logger,
		client: &http.Client{
			Timeout: 45 * time.Second,
			Transport: &http.Transport{
				Proxy:               http.ProxyFromEnvironment,
				MaxIdleConns:        32,
				IdleConnTimeout:     30 * time.Second,
				ForceAttemptHTTP2:   true,
				TLSHandshakeTimeout: 15 * time.Second,
			},
		},
		inflight: map[string]bool{},
	}
}

// Render xử lý một yêu cầu. Nếu khung cần nhiều ảnh mà chưa đủ thì trả Pending.
func (e *Engine) Render(ctx context.Context, in Input) (Result, error) {
	filter, _ := e.Catalog.Get(in.FilterID)
	if filter.ID == "" {
		filter = catalog.Filter{ID: in.FilterID, Name: in.FilterID}
	}
	total := filter.ExpectedShots()

	images := dedupeURLs(in.ImageURLs)
	if total > 1 {
		images = e.collectShots(in.UserKey, filter.ID, images, total)
		if len(images) < total {
			return Result{Pending: true, Captured: len(images), Total: total, Filter: filter}, nil
		}
		if len(images) > total {
			images = images[:total]
		}
	}
	if len(images) == 0 {
		return Result{}, errors.New("SOURCE_IMAGE_MISSING")
	}
	if len(images) > 1 && total == 1 {
		images = images[:1]
	}

	shots := make([]image.Image, 0, len(images))
	for _, u := range images {
		img, err := e.fetchImage(ctx, u)
		if err != nil {
			e.logf("filter %s: tải ảnh lỗi: %v", filter.ID, err)
			return Result{}, fmt.Errorf("SOURCE_IMAGE_UNAVAILABLE: %w", err)
		}
		shots = append(shots, img)
	}

	canvas := compose(shots)
	g := gradeFor(filter)
	canvas = applyGrade(canvas, g)

	framed := false
	if filter.ImageURL != "" {
		if frame, err := e.fetchFrame(ctx, filter); err == nil && frame != nil {
			drawOver(canvas, frame)
			framed = true
		} else if err != nil {
			e.logf("filter %s: khung lỗi (vẫn trả ảnh): %v", filter.ID, err)
		}
	}

	name := uuid4() + ".jpg"
	path := filepath.Join(e.OutDir, name)
	if err := writeJPEG(path, canvas); err != nil {
		return Result{}, err
	}
	return Result{
		URL:      "https://" + e.Host + "/locket-filters/" + name,
		Filter:   filter,
		Captured: len(shots),
		Total:    len(shots),
		Grade:    g.Name,
		Framed:   framed,
	}, nil
}

// collectShots gộp ảnh của cùng một phiên PhotoBooth cho tới khi đủ số khung.
func (e *Engine) collectShots(userKey, filterID string, incoming []string, total int) []string {
	sum := sha1.Sum([]byte(userKey + "|" + filterID))
	path := filepath.Join(e.SessDir, hex.EncodeToString(sum[:])+".json")

	type session struct {
		Images  []string `json:"images"`
		Updated string   `json:"updated"`
	}
	cur := session{}
	if raw, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(raw, &cur)
	}
	for _, u := range incoming {
		dup := false
		for _, old := range cur.Images {
			if old == u {
				dup = true
				break
			}
		}
		if !dup {
			cur.Images = append(cur.Images, u)
		}
	}
	cur.Updated = time.Now().UTC().Format(time.RFC3339)
	if len(cur.Images) < total {
		if raw, err := json.Marshal(cur); err == nil {
			_ = os.WriteFile(path, raw, 0o644)
		}
		return cur.Images
	}
	_ = os.Remove(path)
	return cur.Images
}

// CleanupSessions xoá phiên PhotoBooth cũ hơn 2 giờ.
func (e *Engine) CleanupSessions() {
	entries, err := os.ReadDir(e.SessDir)
	if err != nil {
		return
	}
	for _, it := range entries {
		info, err := it.Info()
		if err != nil {
			continue
		}
		if time.Since(info.ModTime()) > 2*time.Hour {
			_ = os.Remove(filepath.Join(e.SessDir, it.Name()))
		}
	}
}

func (e *Engine) fetchImage(ctx context.Context, rawURL string) (image.Image, error) {
	body, err := e.download(ctx, rawURL, true)
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	return img, nil
}

func (e *Engine) fetchFrame(ctx context.Context, f catalog.Filter) (image.Image, error) {
	cachePath := filepath.Join(e.FrameDir, safeName(f.ID)+".png")
	if raw, err := os.ReadFile(cachePath); err == nil {
		if img, _, err2 := image.Decode(bytes.NewReader(raw)); err2 == nil {
			return img, nil
		}
	}
	body, err := e.download(ctx, f.ImageURL, false)
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(cachePath, body, 0o644); err != nil {
		e.logf("không ghi được cache khung %s: %v", f.ID, err)
	}
	return img, nil
}

func (e *Engine) download(ctx context.Context, rawURL string, enforceHost bool) ([]byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return nil, errors.New("IMAGE_URL_INVALID")
	}
	if enforceHost && !e.hostAllowed(u.Hostname()) {
		return nil, errors.New("IMAGE_HOST_DENIED")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Locket-Gold/1.0")
	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, e.MaxBytes))
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, errors.New("IMAGE_EMPTY")
	}
	return data, nil
}

// hostAllowed giới hạn nguồn ảnh: chỉ ảnh thuộc hạ tầng Locket/Firebase (giống worker gốc).
func (e *Engine) hostAllowed(host string) bool {
	host = strings.ToLower(host)
	if host == "" {
		return false
	}
	if host == strings.ToLower(e.Host) || host == "localhost" || strings.HasPrefix(host, "127.0.0.1") {
		return true
	}
	allowed := []string{
		".locketcamera.com",
		"firebasestorage.googleapis.com",
		"storage.googleapis.com",
		"lh3.googleusercontent.com",
		"googleusercontent.com",
		"huggingface.co",
		"cdn-lfs.huggingface.co",
	}
	if host == "locketcamera.com" {
		return true
	}
	for _, a := range allowed {
		if strings.HasSuffix(host, a) {
			return true
		}
	}
	return false
}

func (e *Engine) logf(format string, args ...any) {
	if e.Logger != nil {
		e.Logger.Printf(format, args...)
	}
}

func compose(shots []image.Image) *image.RGBA {
	canvas := image.NewRGBA(image.Rect(0, 0, canvasSize, canvasSize))
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{color.White}, image.Point{}, draw.Src)
	n := len(shots)
	switch n {
	case 0:
		return canvas
	case 1:
		fill(canvas, image.Rect(0, 0, canvasSize, canvasSize), shots[0])
	case 2:
		h := canvasSize / 2
		fill(canvas, image.Rect(0, 0, canvasSize, h-2), shots[0])
		fill(canvas, image.Rect(0, h+2, canvasSize, canvasSize), shots[1])
	case 3:
		h := canvasSize / 3
		for i := 0; i < 3; i++ {
			fill(canvas, image.Rect(0, i*h+2, canvasSize, (i+1)*h-2), shots[i])
		}
	default:
		half := canvasSize / 2
		pad := 4
		rects := []image.Rectangle{
			image.Rect(pad, pad, half-pad, half-pad),
			image.Rect(half+pad, pad, canvasSize-pad, half-pad),
			image.Rect(pad, half+pad, half-pad, canvasSize-pad),
			image.Rect(half+pad, half+pad, canvasSize-pad, canvasSize-pad),
		}
		for i, r := range rects {
			if i < len(shots) {
				fill(canvas, r, shots[i])
			}
		}
	}
	return canvas
}

// fill vẽ ảnh vào ô đích theo kiểu cover (cắt cho đầy khung).
func fill(dst *image.RGBA, rect image.Rectangle, src image.Image) {
	b := src.Bounds()
	wantAspect := float64(rect.Dx()) / float64(rect.Dy())
	srcAspect := float64(b.Dx()) / float64(b.Dy())

	crop := b
	if srcAspect > wantAspect {
		newW := int(float64(b.Dy()) * wantAspect)
		x0 := b.Min.X + (b.Dx()-newW)/2
		crop = image.Rect(x0, b.Min.Y, x0+newW, b.Max.Y)
	} else if srcAspect < wantAspect {
		newH := int(float64(b.Dx()) / wantAspect)
		y0 := b.Min.Y + (b.Dy()-newH)/2
		crop = image.Rect(b.Min.X, y0, b.Max.X, y0+newH)
	}
	scaled := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	xdraw.CatmullRom.Scale(scaled, scaled.Bounds(), src, crop, xdraw.Over, nil)
	draw.Draw(dst, rect, scaled, image.Point{}, draw.Src)
}

func drawOver(dst *image.RGBA, frame image.Image) {
	scaled := image.NewRGBA(image.Rect(0, 0, canvasSize, canvasSize))
	xdraw.CatmullRom.Scale(scaled, scaled.Bounds(), frame, frame.Bounds(), xdraw.Over, nil)
	draw.Draw(dst, dst.Bounds(), scaled, image.Point{}, draw.Over)
}

func writeJPEG(path string, img image.Image) error {
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: 90}); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

func dedupeURLs(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, u := range in {
		u = strings.TrimSpace(u)
		if u == "" || seen[u] {
			continue
		}
		seen[u] = true
		out = append(out, u)
	}
	return out
}

func safeName(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "frame"
	}
	return b.String()
}

func uuid4() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// ListRendered liệt kê ảnh đã render (dùng cho panel/giám sát).
func (e *Engine) ListRendered(limit int) []string {
	entries, err := os.ReadDir(e.OutDir)
	if err != nil {
		return nil
	}
	type item struct {
		name string
		mod  time.Time
	}
	var items []item
	for _, it := range entries {
		if !strings.HasSuffix(it.Name(), ".jpg") {
			continue
		}
		info, err := it.Info()
		if err != nil {
			continue
		}
		items = append(items, item{it.Name(), info.ModTime()})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].mod.After(items[j].mod) })
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, "/locket-filters/"+it.name)
	}
	return out
}
