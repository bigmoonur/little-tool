package picker

import (
	"math/rand/v2"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"
)

type Group struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Color     string `json:"color"`
	Collapsed bool   `json:"collapsed"`
}

type categoryLayout struct {
	GroupID string `json:"groupId"`
	Hidden  bool   `json:"hidden"`
}

type layoutData struct {
	Groups     []Group                   `json:"groups"`
	Order      []string                  `json:"order"`
	Categories map[string]categoryLayout `json:"categories"`
}

func (d layoutData) clone() layoutData {
	out := layoutData{Groups: append([]Group{}, d.Groups...), Order: append([]string{}, d.Order...), Categories: map[string]categoryLayout{}}
	for name, c := range d.Categories {
		out.Categories[name] = c
	}
	return out
}

func (d layoutData) groupIndex(id string) int {
	return slices.IndexFunc(d.Groups, func(g Group) bool { return g.ID == id })
}

func (s *Store) loadLayout() {
	var data layoutData
	readJSON(filepath.Join(s.dir, "layout.json"), &data)
	s.layout = layoutData{Groups: []Group{}, Order: []string{}, Categories: map[string]categoryLayout{}}
	used := map[string]bool{}
	for i, g := range data.Groups {
		if g.ID == "" || used[g.ID] {
			continue
		}
		used[g.ID] = true
		if !slices.Contains(TagColors, g.Color) {
			g.Color = TagColors[i%len(TagColors)]
		}
		s.layout.Groups = append(s.layout.Groups, g)
	}
	for name, c := range data.Categories {
		if c.GroupID != "" && !used[c.GroupID] {
			c.GroupID = ""
		}
		s.layout.Categories[name] = c
	}
	seen := map[string]bool{}
	for _, name := range data.Order {
		if _, ok := s.layout.Categories[name]; ok && !seen[name] {
			s.layout.Order = append(s.layout.Order, name)
			seen[name] = true
		}
	}
	missing := []string{}
	for name := range s.layout.Categories {
		if !seen[name] {
			missing = append(missing, name)
		}
	}
	sortNames(missing, func(name string) string { return name })
	s.layout.Order = append(s.layout.Order, missing...)
}

func (s *Store) saveLayout(data layoutData) error {
	if err := writeJSON(filepath.Join(s.dir, "layout.json"), data); err != nil {
		return userErr("无法保存分类布局：%v", err)
	}
	s.layout = data
	return nil
}

func (s *Store) Groups() []Group {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Group{}, s.layout.Groups...)
}

// 新发现的分类按名称追加，离线或暂时缺少的分类仍保留位置。
func (s *Store) ArrangeCategories(cats []Category) []Category {
	s.mu.Lock()
	defer s.mu.Unlock()
	data := s.layout.clone()
	unknown := []string{}
	byName := make(map[string]Category, len(cats))
	for _, c := range cats {
		byName[c.Name] = c
		if _, ok := data.Categories[c.Name]; !ok {
			unknown = append(unknown, c.Name)
		}
	}
	sortNames(unknown, func(name string) string { return name })
	for _, name := range unknown {
		data.Categories[name] = categoryLayout{}
		data.Order = append(data.Order, name)
	}
	if len(unknown) > 0 {
		_ = s.saveLayout(data)
	}
	out := make([]Category, 0, len(cats))
	appendGroup := func(id string) {
		for _, name := range data.Order {
			layout := data.Categories[name]
			if c, ok := byName[name]; ok && !layout.Hidden && layout.GroupID == id {
				c.GroupID = id
				c.TagIDs = orderedTagIDs(s.tags.Tags, s.tags.Categories[name])
				out = append(out, c)
			}
		}
	}
	for _, g := range data.Groups {
		appendGroup(g.ID)
	}
	appendGroup("")
	return out
}

func groupName(name string, groups []Group, exceptID string) (string, error) {
	name = strings.Join(strings.Fields(name), " ")
	if name == "" {
		return "", userErr("请填写分组名称")
	}
	if utf8.RuneCountInString(name) > 40 {
		return "", userErr("分组名称最多 40 个字")
	}
	for _, g := range groups {
		if g.ID != exceptID && strings.EqualFold(g.Name, name) {
			return "", userErr("已有同名分组「%s」", g.Name)
		}
	}
	return name, nil
}

func (s *Store) CreateGroup(name string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	name, err := groupName(name, s.layout.Groups, "")
	if err != nil {
		return "", err
	}
	data := s.layout.clone()
	used := map[string]bool{}
	for _, g := range data.Groups {
		used[g.ID] = true
	}
	id := newTagID(used)
	data.Groups = append(data.Groups, Group{ID: id, Name: name, Color: TagColors[len(data.Groups)%len(TagColors)]})
	if err := s.saveLayout(data); err != nil {
		return "", err
	}
	return id, nil
}

func (s *Store) changeGroup(id string, fn func(*Group, *layoutData) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data := s.layout.clone()
	i := data.groupIndex(id)
	if i < 0 {
		return userErr("分组不存在，可能已被删除")
	}
	if err := fn(&data.Groups[i], &data); err != nil {
		return err
	}
	return s.saveLayout(data)
}

func (s *Store) RenameGroup(id, name string) error {
	return s.changeGroup(id, func(g *Group, data *layoutData) error {
		n, err := groupName(name, data.Groups, id)
		if err == nil {
			g.Name = n
		}
		return err
	})
}

func (s *Store) SetGroupColor(id, color string) error {
	return s.changeGroup(id, func(g *Group, _ *layoutData) error {
		if !slices.Contains(TagColors, color) {
			return userErr("不支持的颜色")
		}
		g.Color = color
		return nil
	})
}

func (s *Store) SetGroupCollapsed(id string, collapsed bool) error {
	return s.changeGroup(id, func(g *Group, _ *layoutData) error {
		g.Collapsed = collapsed
		return nil
	})
}

func (s *Store) MoveGroup(id, beforeID string) error {
	return s.changeGroup(id, func(g *Group, data *layoutData) error {
		if id == beforeID {
			return nil
		}
		group := *g
		i := data.groupIndex(id)
		data.Groups = slices.Delete(data.Groups, i, i+1)
		at := data.groupIndex(beforeID)
		if at < 0 {
			at = len(data.Groups)
		}
		data.Groups = slices.Insert(data.Groups, at, group)
		return nil
	})
}

func (s *Store) DeleteGroup(id string) error {
	return s.changeGroup(id, func(_ *Group, data *layoutData) error {
		i := data.groupIndex(id)
		data.Groups = slices.Delete(data.Groups, i, i+1)
		for name, c := range data.Categories {
			if c.GroupID == id {
				c.GroupID = ""
				data.Categories[name] = c
			}
		}
		return nil
	})
}

func (d *layoutData) moveCategory(name, groupID, beforeName string) error {
	c, ok := d.Categories[name]
	if !ok || c.Hidden {
		return userErr("分类不存在，可能已被移除")
	}
	if groupID != "" && d.groupIndex(groupID) < 0 {
		return userErr("分组不存在，可能已被删除")
	}
	c.GroupID = groupID
	d.Categories[name] = c
	d.Order = slices.DeleteFunc(d.Order, func(n string) bool { return n == name })
	at := -1
	if beforeName != "" && beforeName != name {
		at = slices.Index(d.Order, beforeName)
	}
	if at < 0 {
		at = len(d.Order)
		for i := len(d.Order) - 1; i >= 0; i-- {
			if d.Categories[d.Order[i]].GroupID == groupID {
				at = i + 1
				break
			}
		}
	}
	d.Order = slices.Insert(d.Order, at, name)
	return nil
}

func (s *Store) MoveCategory(name, groupID, beforeName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data := s.layout.clone()
	if err := data.moveCategory(name, groupID, beforeName); err != nil {
		return err
	}
	return s.saveLayout(data)
}

// 隐藏分类也不能通过旧方案或旧草稿参与抽取。
func (s *Store) VisiblePicks(picks map[string]Pick) map[string]Pick {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]Pick{}
	for name, pick := range picks {
		if !s.layout.Categories[name].Hidden {
			out[name] = pick
		}
	}
	return out
}

func (s *Store) hiddenCategories() map[string]bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]bool{}
	for name, c := range s.layout.Categories {
		if c.Hidden {
			out[name] = true
		}
	}
	return out
}

// 隐藏文件夹不参与扫描，缩略图清理仍使用完整的磁盘清单。
func (s *Store) ScanCategories(root string) ([]Category, error) {
	cats, _, err := scanFiltered(root, s.hiddenCategories())
	return cats, err
}

func (s *Store) Prepare(materialRoot, outputRoot, folder string, picks map[string]Pick, mode string, clean bool, r *rand.Rand) (Plan, error) {
	hidden := s.hiddenCategories()
	visible := map[string]Pick{}
	for name, pick := range picks {
		if !hidden[name] {
			visible[name] = pick
		}
	}
	return prepare(materialRoot, outputRoot, folder, visible, mode, clean, r, hidden)
}
