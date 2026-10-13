// Package library 保存素材分类、分组和抽取方案，并实现抽取与导入逻辑。
package library

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"unicode/utf8"
)

const (
	// MaxOutputLines 避免数量或重复次数输错时一次生成过量内容。
	MaxOutputLines = 100_000
	// MaxNameLength 分类、分组和方案名称的最大字符数。
	MaxNameLength = 40
)

// GroupColors 是分组可选的颜色，前端按主题映射到具体色值。
var GroupColors = []string{"indigo", "teal", "amber", "rose", "violet", "green", "orange", "slate"}

type Group struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Color     string `json:"color"`
	Collapsed bool   `json:"collapsed"`
}

type Tag struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// List 是一个素材分类，对应原来的一个 TXT 文件，每个条目是一行。
type List struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	GroupID   string     `json:"groupId"`
	Items     []string   `json:"items"`
	ItemTags  [][]string `json:"itemTags,omitempty"`
	UpdatedAt int64      `json:"updatedAt"`

	unique int
}

// Pick 保存数量、乘数规则和文本标签筛选条件。
type Pick struct {
	Count  int      `json:"count"`
	Repeat int      `json:"repeat"`
	Mode   string   `json:"mode,omitempty"`
	TagIDs []string `json:"tagIds,omitempty"`
}

type Preset struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Picks map[string]Pick `json:"picks"`
}

type Settings struct {
	AlwaysOnTop   bool   `json:"alwaysOnTop"`
	AfterCopy     string `json:"afterCopy"` // stay | minimize | quit
	RememberDraft bool   `json:"rememberDraft"`
	Theme         string `json:"theme"` // system | light | dark
	ImportDedupe  bool   `json:"importDedupe"`
}

type WindowState struct {
	Width     int  `json:"width"`
	Height    int  `json:"height"`
	Maximised bool `json:"maximised"`
}

// Data 是 library.json 的完整内容。
type Data struct {
	Version  int             `json:"version"`
	Groups   []Group         `json:"groups"`
	Tags     []Tag           `json:"tags"`
	Lists    []List          `json:"lists"`
	Presets  []Preset        `json:"presets"`
	Draft    map[string]Pick `json:"draft"`
	Settings Settings        `json:"settings"`
}

func DefaultSettings() Settings {
	return Settings{
		AlwaysOnTop:  true,
		AfterCopy:    "stay",
		Theme:        "system",
		ImportDedupe: true,
	}
}

func NewData() *Data {
	return &Data{
		Version:  1,
		Groups:   []Group{},
		Tags:     []Tag{},
		Lists:    []List{},
		Presets:  []Preset{},
		Draft:    map[string]Pick{},
		Settings: DefaultSettings(),
	}
}

// normalize 修正从磁盘或备份读到的数据，保证后续逻辑不必处理 nil 和非法值。
func (d *Data) normalize() {
	if d.Version == 0 {
		d.Version = 1
	}
	if d.Groups == nil {
		d.Groups = []Group{}
	}
	if d.Lists == nil {
		d.Lists = []List{}
	}
	if d.Tags == nil {
		d.Tags = []Tag{}
	}
	tagIDs := map[string]bool{}
	for i := range d.Tags {
		if d.Tags[i].ID == "" {
			d.Tags[i].ID = NewID()
		}
		tagIDs[d.Tags[i].ID] = true
	}
	if d.Presets == nil {
		d.Presets = []Preset{}
	}
	if d.Draft == nil {
		d.Draft = map[string]Pick{}
	}
	groupIDs := map[string]bool{}
	for i := range d.Groups {
		g := &d.Groups[i]
		if g.ID == "" {
			g.ID = NewID()
		}
		if !validColor(g.Color) {
			g.Color = GroupColors[i%len(GroupColors)]
		}
		groupIDs[g.ID] = true
	}
	for i := range d.Lists {
		l := &d.Lists[i]
		if l.ID == "" {
			l.ID = NewID()
		}
		if l.GroupID != "" && !groupIDs[l.GroupID] {
			l.GroupID = ""
		}
		l.setItems(cleanItems(l.Items))
		for j, ids := range l.ItemTags {
			l.ItemTags[j] = cleanTagIDs(ids, tagIDs)
		}
	}
	for i := range d.Presets {
		if d.Presets[i].Picks == nil {
			d.Presets[i].Picks = map[string]Pick{}
		}
	}
	d.Settings.normalize()
}

func (s *Settings) normalize() {
	switch s.AfterCopy {
	case "stay", "minimize", "quit":
	default:
		s.AfterCopy = "stay"
	}
	switch s.Theme {
	case "system", "light", "dark":
	default:
		s.Theme = "system"
	}
}

func (l *List) setItems(items []string) {
	// 按原文及出现次序保留标签，支持排序、追加和删除；相同行仍可有各自的标签。
	old := map[string][][]string{}
	for i, text := range l.Items {
		var tags []string
		if i < len(l.ItemTags) {
			tags = l.ItemTags[i]
		}
		key := strings.TrimSpace(text)
		old[key] = append(old[key], tags)
	}
	tags := make([][]string, len(items))
	for i, text := range items {
		if matches := old[text]; len(matches) > 0 {
			tags[i] = matches[0]
			old[text] = matches[1:]
		}
		if tags[i] == nil {
			tags[i] = []string{}
		}
	}
	l.Items = items
	l.ItemTags = tags
	l.unique = countUnique(items)
}

// Unique 返回去重后的条目数。
func (l *List) Unique() int { return l.unique }

func validColor(c string) bool {
	for _, k := range GroupColors {
		if k == c {
			return true
		}
	}
	return false
}

func NewID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// SplitLines 把文本拆成素材行：去掉每行首尾空白，忽略空行。
// 返回素材行和被忽略的空行数。
func SplitLines(text string) (lines []string, blank int) {
	text = strings.TrimPrefix(text, "\ufeff")
	if text == "" {
		return []string{}, 0
	}
	raw := strings.Split(text, "\n")
	lines = make([]string, 0, len(raw))
	for _, line := range raw {
		line = strings.TrimSpace(line)
		if line == "" {
			blank++
			continue
		}
		lines = append(lines, line)
	}
	// 文本以换行结尾时，最后的空字符串不算空行。
	if strings.HasSuffix(text, "\n") && blank > 0 {
		blank--
	}
	return lines, blank
}

func cleanItems(items []string) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		it = strings.TrimSpace(it)
		if it != "" && !strings.ContainsAny(it, "\r\n") {
			out = append(out, it)
		}
	}
	return out
}

func countUnique(items []string) int {
	seen := make(map[string]struct{}, len(items))
	for _, it := range items {
		seen[it] = struct{}{}
	}
	return len(seen)
}

// CleanName 校验并整理名称，失败时返回面向用户的错误。
func CleanName(name, what string) (string, error) {
	name = strings.Join(strings.Fields(name), " ")
	if name == "" {
		return "", userErr("请填写%s名称", what)
	}
	if utf8.RuneCountInString(name) > MaxNameLength {
		return "", userErr("%s名称最多 %d 个字", what, MaxNameLength)
	}
	return name, nil
}

func sameName(a, b string) bool { return strings.EqualFold(a, b) }
