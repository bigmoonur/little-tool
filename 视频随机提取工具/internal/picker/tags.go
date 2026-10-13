package picker

import (
	"crypto/rand"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"
)

type Tag struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

var TagColors = []string{"indigo", "teal", "amber", "rose", "violet", "green", "orange", "slate"}

type tagData struct {
	Tags       []Tag               `json:"tags"`
	Categories map[string][]string `json:"categories"`
}

func newTagID(used map[string]bool) string {
	for {
		id := rand.Text()
		if !used[id] {
			used[id] = true
			return id
		}
	}
}

func orderedTagIDs(tags []Tag, ids []string) []string {
	selected := make(map[string]bool, len(ids))
	for _, id := range ids {
		selected[id] = true
	}
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		if selected[tag.ID] {
			out = append(out, tag.ID)
		}
	}
	return out
}

func (s *Store) loadTags() {
	var data tagData
	if !readJSON(filepath.Join(s.dir, "tags.json"), &data) {
		data = tagData{}
	}
	s.tags = tagData{Tags: []Tag{}, Categories: map[string][]string{}}
	used := make(map[string]bool, len(data.Tags))
	for _, tag := range data.Tags {
		if tag.ID != "" {
			used[tag.ID] = true
		}
	}
	seen := make(map[string]bool, len(data.Tags))
	for i, tag := range data.Tags {
		if tag.ID == "" {
			tag.ID = newTagID(used)
		}
		if seen[tag.ID] {
			continue
		}
		seen[tag.ID] = true
		if !slices.Contains(TagColors, tag.Color) {
			tag.Color = TagColors[i%len(TagColors)]
		}
		s.tags.Tags = append(s.tags.Tags, tag)
	}
	// 离线的素材盘可能稍后接回，只清理失效标签，不按磁盘上的分类删关联。
	for name, ids := range data.Categories {
		if ordered := orderedTagIDs(s.tags.Tags, ids); len(ordered) != 0 {
			s.tags.Categories[name] = ordered
		}
	}
}

func (data tagData) clone() tagData {
	out := tagData{Tags: make([]Tag, len(data.Tags)), Categories: make(map[string][]string, len(data.Categories))}
	copy(out.Tags, data.Tags)
	for name, ids := range data.Categories {
		out.Categories[name] = append([]string{}, ids...)
	}
	return out
}

func (data tagData) tagIndex(id string) int {
	return slices.IndexFunc(data.Tags, func(tag Tag) bool { return tag.ID == id })
}

func (s *Store) saveTags(data tagData) error {
	if err := writeJSON(filepath.Join(s.dir, "tags.json"), data); err != nil {
		return userErr("无法保存标签：%v", err)
	}
	// 保存成功后再替换内存，写入失败时仍可返回原来的状态。
	s.tags = data
	return nil
}

func (s *Store) Tags() []Tag {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Tag, len(s.tags.Tags))
	copy(out, s.tags.Tags)
	return out
}

func (s *Store) CategoryTags(name string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return orderedTagIDs(s.tags.Tags, s.tags.Categories[name])
}

func tagName(name string, tags []Tag, exceptID string) (string, error) {
	name = strings.Join(strings.Fields(name), " ")
	if name == "" {
		return "", userErr("请填写标签名称")
	}
	if utf8.RuneCountInString(name) > 40 {
		return "", userErr("标签名称最多 40 个字")
	}
	for _, tag := range tags {
		if tag.ID != exceptID && strings.EqualFold(tag.Name, name) {
			return "", userErr("已有同名标签「%s」", tag.Name)
		}
	}
	return name, nil
}

func (s *Store) CreateTag(name string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	name, err := tagName(name, s.tags.Tags, "")
	if err != nil {
		return "", err
	}
	used := make(map[string]bool, len(s.tags.Tags))
	counts := make(map[string]int, len(TagColors))
	for _, tag := range s.tags.Tags {
		used[tag.ID] = true
		counts[tag.Color]++
	}
	color := TagColors[0]
	for _, candidate := range TagColors[1:] {
		if counts[candidate] < counts[color] {
			color = candidate
		}
	}
	id := newTagID(used)
	data := s.tags.clone()
	data.Tags = append(data.Tags, Tag{ID: id, Name: name, Color: color})
	if err := s.saveTags(data); err != nil {
		return "", err
	}
	return id, nil
}

func (s *Store) RenameTag(id, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.tags.tagIndex(id)
	if i < 0 {
		return userErr("标签不存在，可能已被删除")
	}
	name, err := tagName(name, s.tags.Tags, id)
	if err != nil {
		return err
	}
	data := s.tags.clone()
	data.Tags[i].Name = name
	return s.saveTags(data)
}

func (s *Store) SetTagColor(id, color string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.tags.tagIndex(id)
	if i < 0 {
		return userErr("标签不存在，可能已被删除")
	}
	if !slices.Contains(TagColors, color) {
		return userErr("不支持的颜色")
	}
	data := s.tags.clone()
	data.Tags[i].Color = color
	return s.saveTags(data)
}

func (s *Store) DeleteTag(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.tags.tagIndex(id)
	if i < 0 {
		return userErr("标签不存在，可能已被删除")
	}
	data := s.tags.clone()
	data.Tags = slices.Delete(data.Tags, i, i+1)
	for name, ids := range data.Categories {
		if ordered := orderedTagIDs(data.Tags, ids); len(ordered) != 0 {
			data.Categories[name] = ordered
		} else {
			delete(data.Categories, name)
		}
	}
	return s.saveTags(data)
}

func (s *Store) SetCategoryTags(category string, tagIDs []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if category == "" {
		return userErr("分类不存在")
	}
	for _, id := range tagIDs {
		if s.tags.tagIndex(id) < 0 {
			return userErr("所选标签已被删除，请重新选择")
		}
	}
	data := s.tags.clone()
	if ids := orderedTagIDs(data.Tags, tagIDs); len(ids) != 0 {
		data.Categories[category] = ids
	} else {
		delete(data.Categories, category)
	}
	return s.saveTags(data)
}
