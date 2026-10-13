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
	Draft         map[string]int `json:"draft"`
}

// HistoryEntry 是一次完成的复制。
type HistoryEntry struct {
	ID          string `json:"id"`
	Time        int64  `json:"time"`
	Destination string `json:"destination"`
	Folder      string `json:"folder"`
	Items       []Item `json:"items"`
	Removed     int    `json:"removed"`
	Bytes       int64  `json:"bytes"`
}

type WindowState struct {
	Width     int  `json:"width"`
	Height    int  `json:"height"`
	Maximised bool `json:"maximised"`
}

type Store struct {
	mu       sync.Mutex
	dir      string
	settings Settings
	history  []HistoryEntry
	window   *WindowState
	tags     tagData
}

func OpenStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, settings: Settings{Theme: "system", Draft: map[string]int{}}, history: []HistoryEntry{}}
	readJSON(filepath.Join(dir, "settings.json"), &s.settings)
	readJSON(filepath.Join(dir, "history.json"), &s.history)
	var w WindowState
	if readJSON(filepath.Join(dir, "window.json"), &w) {
		s.window = &w
	}
	if s.settings.Draft == nil {
		s.settings.Draft = map[string]int{}
	}
	switch s.settings.Theme {
	case "system", "light", "dark":
	default:
		s.settings.Theme = "system"
	}
	if s.history == nil {
		s.history = []HistoryEntry{}
	}
	s.loadTags()
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
	out.Draft = make(map[string]int, len(s.settings.Draft))
	for k, v := range s.settings.Draft {
		out.Draft[k] = v
	}
	return out
}

// Update 修改设置并保存。
func (s *Store) Update(fn func(*Settings)) Settings {
	s.mu.Lock()
	fn(&s.settings)
	if !s.settings.RememberDraft || s.settings.Draft == nil {
		s.settings.Draft = map[string]int{}
	}
	_ = writeJSON(filepath.Join(s.dir, "settings.json"), s.settings)
	s.mu.Unlock()
	return s.Settings()
}

// History 返回复制记录；没有记录时返回空列表（不是 nil），界面收到的是 [] 而不是 null。
func (s *Store) History() []HistoryEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]HistoryEntry, len(s.history))
	copy(out, s.history)
	return out
}

func (s *Store) AddHistory(r Result) HistoryEntry {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	e := HistoryEntry{ID: hex.EncodeToString(b), Time: time.Now().UnixMilli(), Destination: r.Destination, Folder: r.Folder, Items: r.Items, Removed: r.Removed}
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
	return e
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
