// Package catalog đọc danh mục filter / caption từ payload remote config của repo mhale.
package catalog

import (
	"encoding/json"
	"sort"
	"strings"
)

// Filter là một filter/khung ảnh mà app có thể gửi lên.
type Filter struct {
	ID         string
	Name       string
	Category   string
	Prompt     string
	Style      string
	Type       string
	FilterType string
	ImageURL   string
	Group      string
}

// Group là một nhóm filter hiển thị trong bảng chọn.
type Group struct {
	Title string
	IDs   []string
}

// Catalog chứa toàn bộ filter + caption lấy từ remote config.
type Catalog struct {
	Filters  map[string]Filter
	Order    []string
	Groups   []Group
	Captions []string
	Entries  map[string]any
}

// entriesFile là cấu trúc tối thiểu của remote config.
type filterConfig struct {
	PickerFilters []string `json:"picker_filters"`
	RemoteFilters []struct {
		UID            string `json:"uid"`
		ID             string `json:"id"`
		Name           string `json:"name"`
		Category       string `json:"category"`
		Prompt         string `json:"prompt"`
		Style          string `json:"style"`
		Type           string `json:"type"`
		FilterType     string `json:"filterType"`
		ImageURL       string `json:"imageUrl"`
		OverlayImage   string `json:"overlayImageUrl"`
	} `json:"remote_filters"`
	FilterGroups []struct {
		Title struct {
			EN string `json:"en"`
			VI string `json:"vi"`
		} `json:"title"`
		FilterIDs []string `json:"filter_ids"`
	} `json:"filter_groups"`
}

// Load dựng catalog từ file entries.json (đã trích từ Locket_Unlocked.js).
func Load(raw []byte) (*Catalog, error) {
	var entries map[string]any
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, err
	}
	c := &Catalog{Filters: map[string]Filter{}, Entries: entries}

	if s, ok := entries["camera_filter_config"].(string); ok {
		var fc filterConfig
		if err := json.Unmarshal([]byte(s), &fc); err == nil {
			groupOf := map[string]string{}
			for _, g := range fc.FilterGroups {
				title := g.Title.VI
				if title == "" {
					title = g.Title.EN
				}
				c.Groups = append(c.Groups, Group{Title: title, IDs: g.FilterIDs})
				for _, id := range g.FilterIDs {
					groupOf[id] = title
				}
			}
			for _, rf := range fc.RemoteFilters {
				id := rf.ID
				if id == "" {
					id = rf.UID
				}
				if id == "" {
					continue
				}
				img := rf.ImageURL
				if img == "" {
					img = rf.OverlayImage
				}
				f := Filter{
					ID:         id,
					Name:       rf.Name,
					Category:   rf.Category,
					Prompt:     rf.Prompt,
					Style:      rf.Style,
					Type:       rf.Type,
					FilterType: rf.FilterType,
					ImageURL:   img,
					Group:      groupOf[id],
				}
				c.Filters[id] = f
				c.Order = append(c.Order, id)
			}
			// picker_filters có thể chứa id chưa nằm trong remote_filters.
			for _, id := range fc.PickerFilters {
				if _, ok := c.Filters[id]; !ok {
					c.Filters[id] = Filter{ID: id, Name: id, Group: groupOf[id]}
					c.Order = append(c.Order, id)
				}
			}
		}
	}

	c.Captions = extractCaptions(entries)
	sort.Strings(c.Captions[0:0]) // no-op, giữ chỗ cho tương lai
	return c, nil
}

// Get tra cứu filter theo id.
func (c *Catalog) Get(id string) (Filter, bool) {
	f, ok := c.Filters[id]
	return f, ok
}

// ExpectedShots cho biết filter cần bao nhiêu ảnh (PhotoBooth nhiều khung).
func (f Filter) ExpectedShots() int {
	id := strings.ToLower(f.ID)
	group := strings.ToLower(f.Group)
	switch {
	case strings.Contains(id, "khung4"), strings.Contains(id, "khung_4"):
		return 4
	case strings.Contains(group, "bộ ba"), strings.Contains(group, "bo ba"):
		return 3
	case strings.Contains(group, "hai đứa"), strings.Contains(group, "hai dua"):
		return 2
	case strings.Contains(id, "khung3"):
		return 3
	case strings.Contains(id, "khung2"):
		return 2
	default:
		return 1
	}
}

// IsPhotobooth cho biết filter có phải khung nhiều ảnh.
func (f Filter) IsPhotobooth() bool { return f.ExpectedShots() > 1 }

// extractCaptions lấy các câu caption tĩnh trong overlay_items để dùng cho generateCaption.
func extractCaptions(entries map[string]any) []string {
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || len([]rune(s)) > 60 || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}
	raw, ok := entries["overlay_items"].(string)
	if !ok {
		return out
	}
	var sections []struct {
		Section string `json:"section"`
		Items   []struct {
			OverlayID string `json:"overlay_id"`
			Caption   struct {
				Text string `json:"text"`
			} `json:"caption"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(raw), &sections); err != nil {
		return out
	}
	for _, s := range sections {
		for _, it := range s.Items {
			add(it.Caption.Text)
		}
	}
	return out
}
