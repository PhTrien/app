// Package store lưu danh sách username được kích Gold, mã kích hoạt,
// các ID RevenueCat đã thấy và phiên đăng nhập của trang admin.
//
// Toàn bộ dữ liệu nằm trong một file JSON (0600) để backup/chuyển máy dễ dàng.
package store

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const codeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// Lỗi trả về khi kích hoạt mã.
var (
	ErrCodeNotFound  = errors.New("mã kích hoạt không tồn tại")
	ErrCodeRevoked   = errors.New("mã này đã bị thu hồi")
	ErrCodeExpired   = errors.New("mã này đã hết hạn")
	ErrCodeUsedUp    = errors.New("mã này đã dùng hết lượt")
	ErrUsernameEmpty = errors.New("chưa nhập username Locket")
	ErrUsernameBad   = errors.New("username không hợp lệ (chỉ chữ, số, dấu chấm, gạch dưới, gạch ngang)")
	ErrCodeFormat    = errors.New("mã không hợp lệ (3-32 ký tự, chỉ chữ và số)")
)

// User là một username đã được kích Gold.
type User struct {
	Key     string    `json:"key"`     // khoá chuẩn hoá (lowercase)
	Name    string    `json:"name"`    // hiển thị
	IDs     []string  `json:"ids"`     // các ID RevenueCat khớp với user này
	Code    string    `json:"code"`    // mã đã dùng (rỗng nếu thêm tay)
	Created time.Time `json:"created"`
	Expires time.Time `json:"expires"` // zero = vĩnh viễn
	Note    string    `json:"note"`
	Revoked bool      `json:"revoked"`
	Hits    int64     `json:"hits"`
	LastHit time.Time `json:"last_hit"`
}

// Active cho biết user còn hạn ở thời điểm now.
func (u *User) Active(now time.Time) bool {
	if u == nil || u.Revoked {
		return false
	}
	return u.Expires.IsZero() || u.Expires.After(now)
}

// Code là một mã kích hoạt do admin tạo.
type Code struct {
	Code    string    `json:"code"`
	Days    int       `json:"days"` // 0 = vĩnh viễn
	Uses    int       `json:"uses"`
	MaxUses int       `json:"max_uses"`
	Created time.Time `json:"created"`
	Expires time.Time `json:"expires"` // hạn của chính mã, zero = không hạn
	Note    string    `json:"note"`
	Revoked bool      `json:"revoked"`
	Users   []string  `json:"users"`
}

// SeenID là một ID RevenueCat server đã thấy (dùng để map username ↔ ID thật).
type SeenID struct {
	ID    string    `json:"id"`
	Count int       `json:"count"`
	First time.Time `json:"first"`
	Last  time.Time `json:"last"`
	UA    string    `json:"ua"`
}

// Data là nội dung file lưu trữ.
type Data struct {
	Users  map[string]*User  `json:"users"`
	Codes  map[string]*Code  `json:"codes"`
	Seen   map[string]*SeenID `json:"seen"`
	Salt   string            `json:"salt"`
	PwHash string            `json:"pw_hash"`
}

// Store là kho dữ liệu có khoá đọc/ghi.
type Store struct {
	mu       sync.Mutex
	path     string
	data     Data
	index    map[string]string    // id (lowercase) -> user key
	goldIP   map[string]time.Time // IP đã xác thực Gold gần đây
	sessions map[string]time.Time // token admin -> hết hạn
	dirty    bool
}

// Open mở (hoặc tạo mới) file dữ liệu trong thư mục dir.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s := &Store{
		path:     filepath.Join(dir, "store.json"),
		index:    map[string]string{},
		goldIP:   map[string]time.Time{},
		sessions: map[string]time.Time{},
		data: Data{
			Users: map[string]*User{},
			Codes: map[string]*Code{},
			Seen:  map[string]*SeenID{},
		},
	}
	raw, err := os.ReadFile(s.path)
	switch {
	case err == nil:
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &s.data); err != nil {
				return nil, err
			}
		}
	case errors.Is(err, os.ErrNotExist):
		// file mới
	default:
		return nil, err
	}
	if s.data.Users == nil {
		s.data.Users = map[string]*User{}
	}
	if s.data.Codes == nil {
		s.data.Codes = map[string]*Code{}
	}
	if s.data.Seen == nil {
		s.data.Seen = map[string]*SeenID{}
	}
	if s.data.Salt == "" {
		s.data.Salt = randomHex(8)
	}
	s.rebuildIndex()
	if err := s.saveLocked(); err != nil {
		return nil, err
	}
	go s.loop()
	return s, nil
}

// Path trả về đường dẫn file dữ liệu.
func (s *Store) Path() string { return s.path }

func (s *Store) rebuildIndex() {
	s.index = map[string]string{}
	for key, u := range s.data.Users {
		if u == nil {
			continue
		}
		s.index[lookup(u.Name)] = key
		s.index[key] = key
		for _, id := range u.IDs {
			if id != "" {
				s.index[lookup(id)] = key
			}
		}
	}
}

func (s *Store) loop() {
	t := time.NewTicker(4 * time.Second)
	for range t.C {
		s.mu.Lock()
		if s.dirty {
			s.dirty = false
			_ = s.saveLocked()
		}
		s.mu.Unlock()
	}
}

func (s *Store) markDirty() { s.dirty = true }

// saveLocked ghi dữ liệu xuống đĩa (phải gọi khi đã giữ khoá).
func (s *Store) saveLocked() error {
	raw, err := json.MarshalIndent(&s.data, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// Save ghi ngay xuống đĩa.
func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dirty = false
	return s.saveLocked()
}

// ---- mật khẩu admin ----

// SetPassword đặt (hoặc đổi) mật khẩu admin.
func (s *Store) SetPassword(plain string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.PwHash = hashPassword(s.data.Salt, plain)
	s.dirty = false
	return s.saveLocked()
}

// HasPassword cho biết đã đặt mật khẩu admin chưa.
func (s *Store) HasPassword() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.PwHash != ""
}

// CheckPassword so khớp mật khẩu admin.
func (s *Store) CheckPassword(plain string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.PwHash == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(hashPassword(s.data.Salt, plain)), []byte(s.data.PwHash)) == 1
}

// hashPassword băm mật khẩu nhiều vòng bằng SHA-256 + salt.
func hashPassword(salt, plain string) string {
	sum := sha256.Sum256([]byte(salt + ":" + plain))
	buf := sum[:]
	for range 60000 {
		h := sha256.New()
		h.Write(buf)
		h.Write([]byte(salt))
		buf = h.Sum(nil)
	}
	return hex.EncodeToString(buf)
}

// ---- phiên admin ----

// NewSession tạo token đăng nhập admin còn hạn d hours.
func (s *Store) NewSession(d time.Duration) string {
	tok := randomHex(32)
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for t, exp := range s.sessions {
		if now.After(exp) {
			delete(s.sessions, t)
		}
	}
	s.sessions[tok] = now.Add(d)
	return tok
}

// SessionValid kiểm tra token còn hạn.
func (s *Store) SessionValid(tok string) bool {
	if tok == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	exp, ok := s.sessions[tok]
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		delete(s.sessions, tok)
		return false
	}
	return true
}

// DropSession huỷ một token.
func (s *Store) DropSession(tok string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, tok)
}

// ---- Gold / username ----

// GoldFor trả về user nếu id này đang được kích Gold còn hạn.
func (s *Store) GoldFor(id string) (*User, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key, ok := s.index[lookup(id)]
	if !ok {
		return nil, false
	}
	u := s.data.Users[key]
	if !u.Active(time.Now()) {
		return nil, false
	}
	cp := *u
	return &cp, true
}

// RecordHit ghi nhận một lần app gọi RevenueCat của user đã kích.
func (s *Store) RecordHit(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if u := s.data.Users[key]; u != nil {
		u.Hits++
		u.LastHit = time.Now()
		s.dirty = true
	}
}

// MarkGoldIP ghi nhớ IP của một phiên đã xác thực Gold.
func (s *Store) MarkGoldIP(ip string) {
	if ip == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.goldIP[ip] = time.Now()
}

// GoldIPRecent cho biết IP đã xác thực Gold trong khoảng d gần đây.
func (s *Store) GoldIPRecent(ip string, d time.Duration) bool {
	if ip == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.goldIP[ip]
	return ok && time.Since(t) < d
}

// RecordSeen lưu ID RevenueCat nhìn thấy trên đường truyền.
func (s *Store) RecordSeen(id, ua string) {
	id = strings.TrimSpace(id)
	if id == "" || len(id) > 128 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	rec := s.data.Seen[id]
	if rec == nil {
		if len(s.data.Seen) > 400 {
			// bỏ bản ghi cũ nhất
			var oldestID string
			var oldest time.Time
			for k, v := range s.data.Seen {
				if oldestID == "" || v.Last.Before(oldest) {
					oldestID, oldest = k, v.Last
				}
			}
			delete(s.data.Seen, oldestID)
		}
		rec = &SeenID{ID: id, First: now, UA: truncate(ua, 90)}
		s.data.Seen[id] = rec
	}
	rec.Count++
	rec.Last = now
	if rec.UA == "" {
		rec.UA = truncate(ua, 90)
	}
	s.dirty = true
}

// ---- mã kích hoạt ----

// CreateOptions là tham số tạo mã.
type CreateOptions struct {
	Count    int
	Days     int    // 0 = vĩnh viễn
	MaxUses  int    // số lượt dùng mỗi mã
	TTLDays  int    // hạn của chính mã, 0 = không hạn
	Note     string
	Custom   string // mã tự đặt (chỉ tạo 1 mã)
}

// CreateCodes tạo mã mới.
func (s *Store) CreateCodes(opt CreateOptions) ([]*Code, error) {
	if opt.Count <= 0 {
		opt.Count = 1
	}
	if opt.Count > 200 {
		opt.Count = 200
	}
	if opt.MaxUses <= 0 {
		opt.MaxUses = 1
	}
	if opt.MaxUses > 1000 {
		opt.MaxUses = 1000
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	var expires time.Time
	if opt.TTLDays > 0 {
		expires = now.AddDate(0, 0, opt.TTLDays)
	}

	var out []*Code
	if strings.TrimSpace(opt.Custom) != "" {
		code, err := normalizeCode(opt.Custom)
		if err != nil {
			return nil, err
		}
		if _, exists := s.data.Codes[code]; exists {
			return nil, errors.New("mã này đã tồn tại rồi")
		}
		c := &Code{
			Code: code, Days: opt.Days, MaxUses: opt.MaxUses,
			Created: now, Expires: expires, Note: opt.Note,
		}
		s.data.Codes[code] = c
		out = append(out, c)
	} else {
		for range opt.Count {
			var code string
			for {
				code = "LG-" + randomCode(4) + "-" + randomCode(4)
				if _, exists := s.data.Codes[code]; !exists {
					break
				}
			}
			c := &Code{
				Code: code, Days: opt.Days, MaxUses: opt.MaxUses,
				Created: now, Expires: expires, Note: opt.Note,
			}
			s.data.Codes[code] = c
			out = append(out, c)
		}
	}
	s.dirty = false
	if err := s.saveLocked(); err != nil {
		return out, err
	}
	return out, nil
}

// Redeem kích Gold cho username bằng mã.
func (s *Store) Redeem(codeRaw, username string) (*User, error) {
	name, key, err := NormalizeUsername(username)
	if err != nil {
		return nil, err
	}
	code, err := normalizeCode(codeRaw)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	c := s.data.Codes[code]
	switch {
	case c == nil:
		return nil, ErrCodeNotFound
	case c.Revoked:
		return nil, ErrCodeRevoked
	case !c.Expires.IsZero() && time.Now().After(c.Expires):
		return nil, ErrCodeExpired
	case c.Uses >= c.MaxUses:
		return nil, ErrCodeUsedUp
	}

	now := time.Now()
	u := s.data.Users[key]
	if u == nil {
		u = &User{Key: key, Name: name, Created: now, Code: code}
		s.data.Users[key] = u
	}
	u.Revoked = false
	if u.Code == "" {
		u.Code = code
	}
	if c.Days <= 0 {
		u.Expires = time.Time{}
	} else {
		base := now
		if u.Expires.After(now) {
			base = u.Expires
		}
		u.Expires = base.AddDate(0, 0, c.Days)
	}
	if !contains(u.IDs, name) {
		u.IDs = append(u.IDs, name)
	}

	c.Uses++
	if !contains(c.Users, name) {
		c.Users = append(c.Users, name)
	}

	s.rebuildIndex()
	s.dirty = false
	if err := s.saveLocked(); err != nil {
		return nil, err
	}
	cp := *u
	return &cp, nil
}

// AddUser kích tay một username (không cần mã).
func (s *Store) AddUser(username string, days int, note string) (*User, error) {
	name, key, err := NormalizeUsername(username)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	u := s.data.Users[key]
	if u == nil {
		u = &User{Key: key, Name: name, Created: now}
		s.data.Users[key] = u
	}
	u.Revoked = false
	if days <= 0 {
		u.Expires = time.Time{}
	} else {
		base := now
		if u.Expires.After(now) {
			base = u.Expires
		}
		u.Expires = base.AddDate(0, 0, days)
	}
	if note != "" {
		u.Note = note
	}
	if !contains(u.IDs, name) {
		u.IDs = append(u.IDs, name)
	}
	s.rebuildIndex()
	s.dirty = false
	if err := s.saveLocked(); err != nil {
		return nil, err
	}
	cp := *u
	return &cp, nil
}

// Extend cộng thêm ngày cho user (0 = vĩnh viễn).
func (s *Store) Extend(key string, days int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u := s.data.Users[key]
	if u == nil {
		return errors.New("không tìm thấy user")
	}
	u.Revoked = false
	if days <= 0 {
		u.Expires = time.Time{}
	} else {
		base := time.Now()
		if u.Expires.After(base) {
			base = u.Expires
		}
		u.Expires = base.AddDate(0, 0, days)
	}
	s.dirty = false
	return s.saveLocked()
}

// RevokeUser ngừng kích Gold cho user (giữ lại để xem lịch sử).
func (s *Store) RevokeUser(key string, revoke bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u := s.data.Users[key]
	if u == nil {
		return errors.New("không tìm thấy user")
	}
	u.Revoked = revoke
	s.dirty = false
	return s.saveLocked()
}

// DeleteUser xoá hẳn user khỏi danh sách.
func (s *Store) DeleteUser(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data.Users, key)
	s.rebuildIndex()
	s.dirty = false
	return s.saveLocked()
}

// RevokeCode thu hồi (hoặc mở lại) một mã.
func (s *Store) RevokeCode(code string, revoke bool) error {
	c, err := normalizeCode(code)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item := s.data.Codes[c]
	if item == nil {
		return errors.New("không tìm thấy mã")
	}
	item.Revoked = revoke
	s.dirty = false
	return s.saveLocked()
}

// DeleteCode xoá mã.
func (s *Store) DeleteCode(code string) error {
	c, err := normalizeCode(code)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data.Codes, c)
	s.dirty = false
	return s.saveLocked()
}

// ---- truy vấn danh sách ----

// Users trả về toàn bộ user, mới nhất trước.
func (s *Store) Users() []*User {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*User, 0, len(s.data.Users))
	for _, u := range s.data.Users {
		cp := *u
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	return out
}

// User lấy một user theo key.
func (s *Store) User(key string) *User {
	s.mu.Lock()
	defer s.mu.Unlock()
	if u := s.data.Users[key]; u != nil {
		cp := *u
		return &cp
	}
	return nil
}

// Codes trả về toàn bộ mã, mới nhất trước.
func (s *Store) Codes() []*Code {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Code, 0, len(s.data.Codes))
	for _, c := range s.data.Codes {
		cp := *c
		cp.Users = append([]string{}, c.Users...)
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	return out
}

// SeenList trả về các ID RevenueCat đã thấy, mới nhất trước.
func (s *Store) SeenList() []*SeenID {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*SeenID, 0, len(s.data.Seen))
	for _, v := range s.data.Seen {
		cp := *v
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Last.After(out[j].Last) })
	return out
}

// Stats là số liệu tổng quan cho trang admin.
type Stats struct {
	Users       int
	ActiveUsers int
	Expiring    int
	Codes       int
	UnusedCodes int
}

// Stats tính số liệu tổng quan.
func (s *Store) Stats() Stats {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	st := Stats{Users: len(s.data.Users), Codes: len(s.data.Codes)}
	for _, u := range s.data.Users {
		if u.Active(now) {
			st.ActiveUsers++
			if !u.Expires.IsZero() && u.Expires.Before(now.AddDate(0, 0, 7)) {
				st.Expiring++
			}
		}
	}
	for _, c := range s.data.Codes {
		if !c.Revoked && c.Uses < c.MaxUses && (c.Expires.IsZero() || c.Expires.After(now)) {
			st.UnusedCodes++
		}
	}
	return st
}

// ---- tiện ích ----

// NormalizeUsername chuyển input (link locket, @username, username) thành name + key.
func NormalizeUsername(raw string) (name, key string, err error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", "", ErrUsernameEmpty
	}
	if i := strings.Index(s, "locket.cam/"); i >= 0 {
		s = s[i+len("locket.cam/"):]
	} else if i := strings.Index(s, "locketcamera.com/"); i >= 0 {
		s = s[i+len("locketcamera.com/"):]
	}
	if i := strings.IndexAny(s, "?#"); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "@")
	s = strings.TrimRight(s, "/")
	s = strings.ReplaceAll(s, " ", "")
	if len(s) < 2 || len(s) > 64 {
		return "", "", ErrUsernameBad
	}
	for _, r := range s {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-'
		if !ok {
			return "", "", ErrUsernameBad
		}
	}
	return s, strings.ToLower(s), nil
}

func normalizeCode(raw string) (string, error) {
	s := strings.ToUpper(strings.TrimSpace(raw))
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "_", "-")
	if len(s) < 3 || len(s) > 32 {
		return "", ErrCodeFormat
	}
	for _, r := range s {
		ok := (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-'
		if !ok {
			return "", ErrCodeFormat
		}
	}
	return s, nil
}

func lookup(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "@")
	return strings.ToLower(s)
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if strings.EqualFold(x, v) {
			return true
		}
	}
	return false
}

func randomCode(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		for i := range b {
			b[i] = byte(time.Now().UnixNano() >> (i * 3))
		}
	}
	out := make([]byte, n)
	for i := range b {
		out[i] = codeAlphabet[int(b[i])%len(codeAlphabet)]
	}
	return string(out)
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
