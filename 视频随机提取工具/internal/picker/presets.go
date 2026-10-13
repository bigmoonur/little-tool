package picker

import (
	"crypto/rand"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"
)

type Preset struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Mode  string          `json:"mode"`
	Picks map[string]Pick `json:"picks"`
}

func clonePresets(presets []Preset) []Preset {
	out := make([]Preset, len(presets))
	for i, preset := range presets {
		out[i] = preset
		out[i].Mode = storedMode(preset.Mode)
		out[i].Picks = clonePicks(preset.Picks)
	}
	return out
}

func (s *Store) loadPresets() {
	var presets []Preset
	if !readJSON(filepath.Join(s.dir, "presets.json"), &presets) {
		presets = nil
	}
	s.presets = clonePresets(presets)
}

func (s *Store) Presets() []Preset {
	s.mu.Lock()
	defer s.mu.Unlock()
	return clonePresets(s.presets)
}

func presetName(name string) (string, error) {
	name = strings.Join(strings.Fields(name), " ")
	if name == "" {
		return "", userErr("请填写方案名称")
	}
	if utf8.RuneCountInString(name) > 40 {
		return "", userErr("方案名称最多 40 个字")
	}
	return name, nil
}

func (s *Store) savePresets(presets []Preset) error {
	if err := writeJSON(filepath.Join(s.dir, "presets.json"), presets); err != nil {
		return userErr("无法保存方案：%v", err)
	}
	// 磁盘写入成功后再更新内存，失败时界面仍能拿到原来的方案。
	s.presets = presets
	return nil
}

func (s *Store) SavePreset(name string, picks map[string]Pick, mode string) (id string, err error) {
	name, err = presetName(name)
	if err != nil {
		return "", err
	}
	mode, err = pickMode(mode)
	if err != nil {
		return "", err
	}
	names := make([]string, 0, len(picks))
	for category := range picks {
		names = append(names, category)
	}
	sort.Strings(names)
	cleaned := map[string]Pick{}
	for _, category := range names {
		pick := picks[category]
		if pick.Count <= 0 {
			continue
		}
		pick.Repeat = max(1, pick.Repeat)
		if pick.Repeat > MaxRepeat {
			return "", userErr("「%s」的乘数最多 %d", category, MaxRepeat)
		}
		cleaned[category] = pick
	}
	if len(cleaned) == 0 {
		return "", userErr("请先为至少一个分类填写数量，再保存为方案")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	presets := clonePresets(s.presets)
	i := slices.IndexFunc(presets, func(p Preset) bool { return strings.EqualFold(p.Name, name) })
	if i >= 0 {
		id = presets[i].ID
	} else {
		for {
			id = rand.Text()
			if !slices.ContainsFunc(presets, func(p Preset) bool { return p.ID == id }) {
				break
			}
		}
		i = len(presets)
		presets = append(presets, Preset{})
	}
	presets[i] = Preset{ID: id, Name: name, Mode: mode, Picks: cleaned}
	if err := s.savePresets(presets); err != nil {
		return "", err
	}
	return id, nil
}

func (s *Store) RenamePreset(id, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := slices.IndexFunc(s.presets, func(p Preset) bool { return p.ID == id })
	if i < 0 {
		return userErr("方案不存在，可能已被删除")
	}
	name, err := presetName(name)
	if err != nil {
		return err
	}
	for _, p := range s.presets {
		if p.ID != id && strings.EqualFold(p.Name, name) {
			return userErr("已有同名方案「%s」", p.Name)
		}
	}
	presets := clonePresets(s.presets)
	presets[i].Name = name
	return s.savePresets(presets)
}

func (s *Store) DeletePreset(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := slices.IndexFunc(s.presets, func(p Preset) bool { return p.ID == id })
	if i < 0 {
		return userErr("方案不存在，可能已被删除")
	}
	presets := clonePresets(s.presets)
	presets = slices.Delete(presets, i, i+1)
	return s.savePresets(presets)
}
