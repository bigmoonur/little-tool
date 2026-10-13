package library

import "slices"

// orderTags 去掉重复和已删除的标签，按标签列表的顺序排好。总是返回新的切片。
func (d *Data) orderTags(ids []string) []string {
	out := []string{}
	for _, t := range d.Tags {
		if slices.Contains(ids, t.ID) {
			out = append(out, t.ID)
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
		d.Tags = append(d.Tags, Tag{ID: id, Name: n, Color: nextTagColor(d.Tags)})
		return nil
	})
	return snap, id, err
}

// nextTagColor 给新标签挑一个用得最少的颜色，标签之间尽量一眼能分开。
func nextTagColor(tags []Tag) string {
	used := map[string]int{}
	for _, t := range tags {
		used[t.Color]++
	}
	best := GroupColors[0]
	for _, c := range GroupColors {
		if used[c] < used[best] {
			best = c
		}
	}
	return best
}

func (s *Store) SetTagColor(id, color string) (Snapshot, error) {
	return s.mutate("", func(d *Data) error {
		i := slices.IndexFunc(d.Tags, func(t Tag) bool { return t.ID == id })
		if i < 0 {
			return userErr("标签不存在，可能已被删除")
		}
		if !validColor(color) {
			return userErr("不支持的颜色")
		}
		d.Tags[i].Color = color
		return nil
	})
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
			// 换成新的切片，撤销时还能找回原来的标签。
			d.Lists[i].TagIDs = d.orderTags(d.Lists[i].TagIDs)
		}
		return nil
	})
}

// SetListTags 设置贴在分类上的标签。标签只用来在抽取页找分类，不改动素材内容和更新时间。
func (s *Store) SetListTags(id string, tagIDs []string) (Snapshot, error) {
	return s.mutate("", func(d *Data) error {
		i := d.listIndex(id)
		if i < 0 {
			return userErr("分类不存在，可能已被删除")
		}
		if err := d.checkTags(tagIDs); err != nil {
			return err
		}
		d.Lists[i].TagIDs = d.orderTags(tagIDs)
		return nil
	})
}
