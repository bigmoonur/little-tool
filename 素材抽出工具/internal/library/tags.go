package library

import "slices"

type TextItem struct {
	Index  int      `json:"index"`
	Text   string   `json:"text"`
	TagIDs []string `json:"tagIds"`
}

type ListItems struct {
	Items     []TextItem `json:"items"`
	UpdatedAt int64      `json:"updatedAt"`
}

func cleanTagIDs(ids []string, allowed map[string]bool) []string {
	out := []string{}
	for _, id := range ids {
		if allowed[id] && !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}

func (d *Data) checkTags(ids []string) error {
	for _, id := range ids {
		if !slices.ContainsFunc(d.Tags, func(t Tag) bool { return t.ID == id }) {
			return userErr("所选标签已被删除，请重新选择")
		}
	}
	return nil
}

func (s *Store) CreateTag(name string) (Snapshot, string, error) {
	var id string
	snap, err := s.mutate("", func(d *Data) error {
		n, err := CleanName(name, "标签")
		if err != nil {
			return err
		}
		if slices.ContainsFunc(d.Tags, func(t Tag) bool { return sameName(t.Name, n) }) {
			return userErr("已有同名标签「%s」", n)
		}
		id = NewID()
		d.Tags = append(d.Tags, Tag{ID: id, Name: n})
		return nil
	})
	return snap, id, err
}

func (s *Store) RenameTag(id, name string) (Snapshot, error) {
	return s.mutate("", func(d *Data) error {
		n, err := CleanName(name, "标签")
		if err != nil {
			return err
		}
		i := slices.IndexFunc(d.Tags, func(t Tag) bool { return t.ID == id })
		if i < 0 {
			return userErr("标签不存在，可能已被删除")
		}
		if slices.ContainsFunc(d.Tags, func(t Tag) bool { return t.ID != id && sameName(t.Name, n) }) {
			return userErr("已有同名标签「%s」", n)
		}
		d.Tags[i].Name = n
		return nil
	})
}

func (s *Store) DeleteTag(id string) (Snapshot, error) {
	return s.mutate("删除标签", func(d *Data) error {
		if err := d.checkTags([]string{id}); err != nil {
			return err
		}
		d.Tags = slices.DeleteFunc(d.Tags, func(t Tag) bool { return t.ID == id })
		for i := range d.Lists {
			l := &d.Lists[i]
			tags := slices.Clone(l.ItemTags)
			changed := false
			for j, ids := range tags {
				if slices.Contains(ids, id) {
					tags[j] = slices.DeleteFunc(slices.Clone(ids), func(v string) bool { return v == id })
					changed = true
				}
			}
			if changed {
				l.ItemTags = tags
				l.UpdatedAt = max(l.UpdatedAt+1, s.now().UnixMilli())
			}
		}
		// 保留方案中的已删标签：使用旧方案时明确提示，避免静默扩大抽取范围。
		return nil
	})
}

func (s *Store) ListItems(id string) (ListItems, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.data.listIndex(id)
	if i < 0 {
		return ListItems{}, userErr("分类不存在，可能已被删除")
	}
	l := s.data.Lists[i]
	out := ListItems{Items: make([]TextItem, len(l.Items)), UpdatedAt: l.UpdatedAt}
	for j, text := range l.Items {
		tags := []string{}
		if j < len(l.ItemTags) {
			tags = append(tags, l.ItemTags[j]...)
		}
		out.Items[j] = TextItem{Index: j, Text: text, TagIDs: tags}
	}
	return out, nil
}

// SetItemTags 仅修改指定行。版本不一致时拒绝写入，防止标签被贴到编辑后的其他文本上。
func (s *Store) SetItemTags(id string, indexes []int, tagIDs []string, updatedAt int64, action string) (Snapshot, error) {
	return s.mutate("修改文本标签", func(d *Data) error {
		i := d.listIndex(id)
		if i < 0 {
			return userErr("分类不存在，可能已被删除")
		}
		l := &d.Lists[i]
		if l.UpdatedAt != updatedAt {
			return userErr("文本或标签已更新，请重新打开标签管理后再试")
		}
		if err := d.checkTags(tagIDs); err != nil {
			return err
		}
		if len(indexes) == 0 {
			return userErr("请先选择要设置标签的文本")
		}
		if action != "set" && action != "add" && action != "remove" {
			return userErr("不支持的标签操作")
		}
		for _, index := range indexes {
			if index < 0 || index >= len(l.Items) {
				return userErr("所选文本已不存在")
			}
		}
		tags := slices.Clone(l.ItemTags)
		allowed := map[string]bool{}
		for _, t := range d.Tags {
			allowed[t.ID] = true
		}
		for _, index := range indexes {
			switch action {
			case "set":
				tags[index] = cleanTagIDs(tagIDs, allowed)
			case "add":
				tags[index] = cleanTagIDs(append(slices.Clone(tags[index]), tagIDs...), allowed)
			case "remove":
				tags[index] = slices.DeleteFunc(slices.Clone(tags[index]), func(id string) bool { return slices.Contains(tagIDs, id) })
			}
		}
		l.ItemTags = tags
		l.UpdatedAt = max(l.UpdatedAt+1, s.now().UnixMilli())
		return nil
	})
}

// matchingItems 对标签取并集；未选择标签时使用全部文本。
func matchingItems(l List, tagIDs []string) []string {
	if len(tagIDs) == 0 {
		return l.Items
	}
	out := []string{}
	for i, text := range l.Items {
		if i >= len(l.ItemTags) {
			continue
		}
		if slices.ContainsFunc(l.ItemTags[i], func(id string) bool { return slices.Contains(tagIDs, id) }) {
			out = append(out, text)
		}
	}
	return out
}

func (s *Store) AvailableLists(tagIDs []string) ([]ListInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.data.checkTags(tagIDs); err != nil {
		return nil, err
	}
	out := make([]ListInfo, len(s.data.Lists))
	for i, l := range s.data.Lists {
		items := matchingItems(l, tagIDs)
		out[i] = ListInfo{l.ID, l.Name, l.GroupID, len(items), countUnique(items), l.UpdatedAt}
	}
	return out, nil
}
