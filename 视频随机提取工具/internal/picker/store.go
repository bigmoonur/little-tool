package picker

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const MaxHistory = 50

// Settings 是 settings.json 的内容。
type Settings struct {
	MaterialRoot  string         `json:"materialRoot"`
	OutputRoot    string         `json:"outputRoot"`
	Folder        string         `json:"folder"`
	AlwaysOnTop   bool           `json:"alwaysOnTop"`
	OpenAfter     bool           `json:"openAfter"` // 复制完成后打开目标文件夹
	Theme         string         `json:"theme"`     // system | light | dark
	RememberDraft bool           `json:"rememberDraft"`
	Thumbnails    bool           `json:"thumbnails"`  // 缓存视频缩略图；默认开启
	LibraryView   string         `json:"libraryView"` // 素材库的显示方式：list | grid
	Draft         map[string]int `json:"draft"`
	DraftRepeat   map[string]int `json:"draftRepeat"`
	Mode          string         `json:"mode"`
}

// HistoryEntry 是一次完成的复制。
type HistoryEntry struct {
	ID          string          `json:"id"`
	Time        int64           `json:"time"`
	Destination string          `json:"destination"`
	Folder      string          `json:"folder"`
	Items       []Item          `json:"items"`
	Removed     int             `json:"removed"`
	Bytes       int64           `json:"bytes"`
	Picks       map[string]Pick `json:"picks"`
	Mode        string          `json:"mode"`
}

type WindowState struct {
	Width     int  `json:"width"`
	Height    int  `json:"height"`
	Maximised bool `json:"maximised"`
}

type Store struct {
	mu       sync.Mutex
	thumbMu  sync.Mutex
	dir      string
	settings Settings
	history  []HistoryEntry
	window   *WindowState
	tags     tagData
	presets  []Preset
}

func OpenStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, settings: Settings{Theme: "system", Thumbnails: true, Draft: map[string]int{}}, history: []HistoryEntry{}}
	readJSON(filepath.Join(dir, "settings.json"), &s.settings)
	readJSON(filepath.Join(dir, "history.json"), &s.history)
	var w WindowState
	if readJSON(filepath.Join(dir, "window.json"), &w) {
		s.window = &w
	}
	normalizeDraft(&s.settings)
	normalizeLibraryView(&s.settings)
	switch s.settings.Theme {
	case "system", "light", "dark":
	default:
		s.settings.Theme = "system"
	}
	if s.history == nil {
		s.history = []HistoryEntry{}
	}
	s.loadTags()
	s.loadPresets()
	return s, nil
}

func (s *Store) Dir() string { return s.dir }

func readJSON(path string, v any) bool {
	b, err := os.ReadFile(path)
	return err == nil && json.Unmarshal(b, v) == nil
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", " ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *Store) Settings() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.settings
	out.Draft = cloneDraft(s.settings.Draft)
	out.DraftRepeat = cloneDraft(s.settings.DraftRepeat)
	return out
}

func cloneDraft(draft map[string]int) map[string]int {
	out := make(map[string]int, len(draft))
	for name, n := range draft {
		out[name] = n
	}
	return out
}

func normalizeDraft(st *Settings) {
	mode, err := pickMode(st.Mode)
	if err != nil || !st.RememberDraft {
		mode = ModeConsecutive
	}
	st.Mode = mode
	if !st.RememberDraft {
		st.Draft, st.DraftRepeat = map[string]int{}, map[string]int{}
	} else {
		st.Draft = cloneDraft(st.Draft)
		st.DraftRepeat = cloneDraft(st.DraftRepeat)
	}
}

func normalizeLibraryView(st *Settings) {
	if st.LibraryView != "grid" {
		st.LibraryView = "list"
	}
}

// Update 修改设置并保存。
func (s *Store) Update(fn func(*Settings)) Settings {
	s.mu.Lock()
	fn(&s.settings)
	normalizeDraft(&s.settings)
	normalizeLibraryView(&s.settings)
	_ = writeJSON(filepath.Join(s.dir, "settings.json"), s.settings)
	s.mu.Unlock()
	return s.Settings()
}

// History 返回复制记录；没有记录时返回空列表（不是 nil），界面收到的是 [] 而不是 null。
func (s *Store) History() []HistoryEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]HistoryEntry, len(s.history))
	for i, e := range s.history {
		out[i] = cloneHistory(e)
	}
	return out
}

func cloneHistory(e HistoryEntry) HistoryEntry {
	e.Items = append([]Item{}, e.Items...)
	if e.Picks == nil {
		// 老记录只知道实际复制数量，按一倍恢复可再次使用的参数。
		e.Picks = map[string]Pick{}
		for _, it := range e.Items {
			pick := e.Picks[it.Category]
			pick.Count++
			pick.Repeat = 1
			e.Picks[it.Category] = pick
		}
	} else {
		e.Picks = clonePicks(e.Picks)
	}
	e.Mode = storedMode(e.Mode)
	return e
}

func (s *Store) AddHistory(r Result) HistoryEntry {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	e := cloneHistory(HistoryEntry{ID: hex.EncodeToString(b), Time: time.Now().UnixMilli(), Destination: r.Destination, Folder: r.Folder, Items: r.Items, Removed: r.Removed, Picks: r.Picks, Mode: r.Mode})
	for _, it := range r.Items {
		e.Bytes += it.Size
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = append([]HistoryEntry{e}, s.history...)
	if len(s.history) > MaxHistory {
		s.history = s.history[:MaxHistory]
	}
	_ = writeJSON(filepath.Join(s.dir, "history.json"), s.history)
	return cloneHistory(e)
}

func (s *Store) ClearHistory() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = []HistoryEntry{}
	return writeJSON(filepath.Join(s.dir, "history.json"), s.history)
}

func (s *Store) Window() *WindowState {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.window == nil {
		return nil
	}
	w := *s.window
	return &w
}

func (s *Store) SaveWindow(w WindowState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.window != nil && *s.window == w {
		return
	}
	s.window = &w
	_ = writeJSON(filepath.Join(s.dir, "window.json"), w)
}
