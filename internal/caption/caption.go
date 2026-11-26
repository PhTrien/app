// Package caption tạo caption cho endpoint /generateCaption.
package caption

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"strings"

	"github.com/PhTrien/app/internal/catalog"
)

// Generator sinh caption tiếng Việt dựa trên ảnh + prompt.
type Generator struct {
	presets []string
}

// New tạo generator, dùng caption tĩnh trong remote config làm nguồn chính.
func New(cat *catalog.Catalog) *Generator {
	base := []string{
		"Hôm nay chỉ muốn xinh thôi 🌸",
		"Khoảnh khắc này phải giữ lại 💫",
		"Góc nhỏ bình yên của tôi 🌿",
		"Ảnh chưa chỉnh mà đã thấy ổn 😌",
		"Một ngày đáng nhớ ✨",
		"Đang sống, đang vui, đang ổn 💛",
		"Nhìn lại vẫn thấy thương 💭",
		"Chụp nhanh mà được cả tấm ảnh 🫶",
		"Ngày hôm nay có nắng ☀️",
		"Giữ lấy chút ngọt ngào này 🍬",
		"Ảnh này là kỷ niệm nha 📸",
		"Tâm trạng: khá là ổn 💅",
		"Chill thôi, đời còn dài 🍃",
		"Vibe hôm nay rất được 🌙",
		"Bình yên là khi có những tấm ảnh thế này 🕊️",
	}
	if cat != nil {
		for _, c := range cat.Captions {
			if len([]rune(c)) >= 6 {
				base = append(base, c)
			}
		}
	}
	return &Generator{presets: base}
}

// Prefer chọn caption theo từ khoá trong prompt (nếu có) rồi mới tới ảnh.
func (g *Generator) Prefer(prompt string) []string {
	if len(g.presets) == 0 {
		return nil
	}
	p := strings.ToLower(prompt)
	var out []string
	for _, c := range g.presets {
		lc := strings.ToLower(c)
		for _, kw := range promptKeywords(p) {
			if strings.Contains(lc, kw) {
				out = append(out, c)
				break
			}
		}
	}
	return out
}

func promptKeywords(p string) []string {
	var out []string
	table := map[string][]string{
		"love":    {"yêu", "thương", "love"},
		"tet":     {"tết", "xuân", "lì xì"},
		"birth":   {"sinh nhật", "birthday", "tuổi"},
		"school":  {"trường", "học", "lớp"},
		"travel":  {"biển", "du lịch", "travel", "nắng"},
		"food":    {"ăn", "food", "món"},
		"night":   {"đêm", "tối", "night", "trăng"},
		"rain":    {"mưa", "rain"},
		"friend":  {"bạn", "friend", "hội"},
		"pet":     {"mèo", "chó", "pet", "cưng"},
		"gym":     {"gym", "tập", "thể thao"},
		"newyear": {"năm mới", "new year"},
	}
	for _, kws := range table {
		for _, k := range kws {
			if strings.Contains(p, k) {
				out = append(out, k)
			}
		}
	}
	return out
}

// Generate trả caption cho ảnh (deterministic: cùng ảnh → cùng caption).
func (g *Generator) Generate(imageB64, prompt string) string {
	if pref := g.Prefer(prompt); len(pref) > 0 {
		return pref[0]
	}
	if len(g.presets) == 0 {
		return "Khoảnh khắc đáng nhớ ✨"
	}
	return g.presets[int(hash32(imageB64, prompt))%len(g.presets)]
}

// Candidates trả vài lựa chọn (app có thể chỉ dùng cái đầu).
func (g *Generator) Candidates(imageB64, prompt string, n int) []string {
	if n <= 1 {
		return []string{g.Generate(imageB64, prompt)}
	}
	var out []string
	seen := map[string]bool{}
	start := int(hash32(imageB64, prompt))
	for i := 0; i < n && i < len(g.presets); i++ {
		c := g.presets[(start+i*7)%len(g.presets)]
		if seen[c] {
			continue
		}
		seen[c] = true
		out = append(out, c)
	}
	return out
}

func hash32(imageB64, prompt string) uint32 {
	h := sha1.New()
	data := imageB64
	if len(data) > 4096 {
		data = data[:2048] + data[len(data)-1024:]
	}
	h.Write([]byte(data))
	h.Write([]byte(prompt))
	sum := h.Sum(nil)
	return binary.BigEndian.Uint32(sum[:4])
}

// DecodeLen trả số byte ảnh sau khi giải mã base64 (dùng để log/giám sát).
func DecodeLen(imageB64 string) int {
	if i := strings.Index(imageB64, ","); i >= 0 && strings.HasPrefix(imageB64, "data:") {
		imageB64 = imageB64[i+1:]
	}
	raw, err := base64.StdEncoding.DecodeString(imageB64)
	if err != nil {
		return 0
	}
	return len(raw)
}
