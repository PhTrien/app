// Package assets nhúng payload remote config đã trích xuất từ repo mhale.
package assets

import _ "embed"

// EntriesJSON là toàn bộ entries Firebase Remote Config (179 key, 296 filter).
//
//go:embed entries.json
var EntriesJSON []byte
