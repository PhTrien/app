package filters

import (
	"image"
	"image/color"
	"math"
	"strings"

	"github.com/PhTrien/app/internal/catalog"
)

// Grade mô tả một preset chỉnh màu giống LUT.
type Grade struct {
	Name       string
	Saturation float64 // 1 = giữ nguyên
	Contrast   float64 // 1 = giữ nguyên
	Brightness float64 // cộng thêm, -0.2..0.2
	Gamma      float64 // 1 = giữ nguyên
	Temperature float64 // -1 (lạnh) .. 1 (ấm)
	Tint       float64 // -1 (xanh lá) .. 1 (hồng)
	Fade       float64 // nâng vùng tối
	Sepia      float64
	Vignette   float64
}

var presets = map[string]Grade{
	"neutral": {Name: "neutral", Saturation: 1, Contrast: 1, Gamma: 1},
	"film":    {Name: "film", Saturation: 0.86, Contrast: 1.08, Gamma: 1.02, Temperature: 0.16, Fade: 0.05, Vignette: 0.12},
	"retro":   {Name: "retro", Saturation: 0.78, Contrast: 0.96, Gamma: 1.06, Temperature: 0.3, Sepia: 0.18, Fade: 0.08, Vignette: 0.2},
	"cold":    {Name: "cold", Saturation: 0.95, Contrast: 1.06, Temperature: -0.28, Tint: -0.05},
	"warm":    {Name: "warm", Saturation: 1.06, Contrast: 1.02, Temperature: 0.3, Brightness: 0.02},
	"vivid":   {Name: "vivid", Saturation: 1.35, Contrast: 1.12, Gamma: 0.98},
	"pastel":  {Name: "pastel", Saturation: 0.82, Contrast: 0.9, Brightness: 0.06, Fade: 0.1, Temperature: 0.1},
	"glow":    {Name: "glow", Saturation: 1.08, Contrast: 0.94, Brightness: 0.08, Gamma: 0.95, Temperature: 0.12},
	"mono":    {Name: "mono", Saturation: 0, Contrast: 1.12},
	"noir":    {Name: "noir", Saturation: 0, Contrast: 1.3, Gamma: 0.9, Vignette: 0.28},
	"dream":   {Name: "dream", Saturation: 1.1, Contrast: 0.92, Brightness: 0.05, Tint: 0.12, Fade: 0.09},
	"anime":   {Name: "anime", Saturation: 1.25, Contrast: 1.1, Gamma: 1.03},
	"sunset":  {Name: "sunset", Saturation: 1.12, Contrast: 1.04, Temperature: 0.42, Brightness: 0.02},
	"cyber":   {Name: "cyber", Saturation: 1.2, Contrast: 1.16, Temperature: -0.25, Tint: 0.2},
	"fade":    {Name: "fade", Saturation: 0.9, Contrast: 0.92, Fade: 0.14},
}

var gradeKeywords = []struct {
	keys []string
	name string
}{
	{[]string{"noir", "đen trắng", "den trang", "b&w", "mono", "black"}, "noir"},
	{[]string{"retro", "vintage", "hoài cổ", "hoai co", "cổ điển", "co dien"}, "retro"},
	{[]string{"film", "analog", "phim", "kodak"}, "film"},
	{[]string{"cold", "lạnh", "lanh", "ice", "blue", "xanh dương", "tuyết", "tuyet"}, "cold"},
	{[]string{"warm", "ấm", "am", "sunset", "hoàng hôn", "hoang hon", "golden"}, "sunset"},
	{[]string{"pastel", "nhẹ nhàng", "nhe nhang", "soft", "powder"}, "pastel"},
	{[]string{"glow", "lung linh", "toả sáng", "toa sang", "shine", "bloom"}, "glow"},
	{[]string{"dream", "mơ", "mo mộng", "fairy", "tiên", "tien"}, "dream"},
	{[]string{"anime", "manga", "cartoon", "hoạt hình", "hoat hinh"}, "anime"},
	{[]string{"cyber", "neon", "tương lai", "tuong lai", "future"}, "cyber"},
	{[]string{"vivid", "rực rỡ", "ruc ro", "pop", "color"}, "vivid"},
	{[]string{"fade", "nhạt", "nhat", "mờ", "mo"}, "fade"},
}

// gradeFor chọn preset dựa trên id / tên / nhóm / prompt của filter.
func gradeFor(f catalog.Filter) Grade {
	// Khung PhotoBooth nhiều ảnh: giữ màu gốc, khung đã tạo điểm nhấn.
	if f.IsPhotobooth() && f.ImageURL != "" {
		return presets["neutral"]
	}
	hay := strings.ToLower(strings.Join([]string{f.ID, f.Name, f.Group, f.Category, f.Style}, " "))
	for _, kw := range gradeKeywords {
		for _, k := range kw.keys {
			if strings.Contains(hay, k) {
				return presets[kw.name]
			}
		}
	}
	if f.ImageURL != "" {
		// Filter AI/overlay một ảnh: chỉnh nhẹ để ảnh "ăn" hơn.
		return Grade{Name: "natural+", Saturation: 1.06, Contrast: 1.04, Gamma: 1.01}
	}
	return Grade{Name: "natural", Saturation: 1.12, Contrast: 1.05, Gamma: 1.0, Brightness: 0.01}
}

// applyGrade áp preset màu lên ảnh (trả về ảnh mới).
func applyGrade(src *image.RGBA, g Grade) *image.RGBA {
	if g.Name == "" || (g.Saturation == 1 && g.Contrast == 1 && g.Brightness == 0 && g.Gamma == 1 &&
		g.Temperature == 0 && g.Tint == 0 && g.Fade == 0 && g.Sepia == 0 && g.Vignette == 0) {
		return src
	}
	dst := image.NewRGBA(src.Bounds())
	w := src.Bounds().Dx()
	h := src.Bounds().Dy()
	cx, cy := float64(w)/2, float64(h)/2
	maxDist := math.Hypot(cx, cy)

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, gg, b, a := src.At(x, y).RGBA()
			rf, gf, bf := float64(r)/65535, float64(gg)/65535, float64(b)/65535

			// Gamma
			if g.Gamma != 1 && g.Gamma > 0 {
				rf, gf, bf = math.Pow(rf, g.Gamma), math.Pow(gf, g.Gamma), math.Pow(bf, g.Gamma)
			}
			// Brightness
			rf += g.Brightness
			gf += g.Brightness
			bf += g.Brightness
			// Contrast
			if g.Contrast != 1 {
				rf = (rf-0.5)*g.Contrast + 0.5
				gf = (gf-0.5)*g.Contrast + 0.5
				bf = (bf-0.5)*g.Contrast + 0.5
			}
			// Temperature / tint
			if g.Temperature != 0 {
				rf += g.Temperature * 0.12
				bf -= g.Temperature * 0.12
			}
			if g.Tint != 0 {
				rf += g.Tint * 0.06
				gf -= g.Tint * 0.03
			}
			// Saturation
			if g.Saturation != 1 {
				lum := 0.299*rf + 0.587*gf + 0.114*bf
				rf = lum + (rf-lum)*g.Saturation
				gf = lum + (gf-lum)*g.Saturation
				bf = lum + (bf-lum)*g.Saturation
			}
			// Sepia
			if g.Sepia > 0 {
				sr := 0.393*rf + 0.769*gf + 0.189*bf
				sg := 0.349*rf + 0.686*gf + 0.168*bf
				sb := 0.272*rf + 0.534*gf + 0.131*bf
				rf = rf*(1-g.Sepia) + sr*g.Sepia
				gf = gf*(1-g.Sepia) + sg*g.Sepia
				bf = bf*(1-g.Sepia) + sb*g.Sepia
			}
			// Fade: nâng vùng tối
			if g.Fade > 0 {
				rf = rf*(1-g.Fade) + g.Fade
				gf = gf*(1-g.Fade) + g.Fade
				bf = bf*(1-g.Fade) + g.Fade
			}
			// Vignette
			if g.Vignette > 0 {
				d := math.Hypot(float64(x)-cx, float64(y)-cy) / maxDist
				f := 1 - g.Vignette*math.Pow(math.Max(0, d-0.45)/0.55, 2)
				rf *= f
				gf *= f
				bf *= f
			}

			dst.SetRGBA(x, y, colorRGBA(rf, gf, bf, float64(a)/65535))
		}
	}
	return dst
}

func colorRGBA(r, g, b, a float64) color.RGBA {
	return color.RGBA{
		R: clamp8(r * 255),
		G: clamp8(g * 255),
		B: clamp8(b * 255),
		A: clamp8(a * 255),
	}
}

func clamp8(v float64) uint8 {
	switch {
	case v <= 0:
		return 0
	case v >= 255:
		return 255
	default:
		return uint8(v + 0.5)
	}
}
