package picker

import (
	"crypto/rand"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// 同名的隐藏文件夹恢复原来的标签，名称以磁盘上实际的大小写为准。
func (s *Store) CreateCategoryIn(root, name, groupID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ValidateFolderName(name); err != nil {
		return err
	}
	data := s.layout.clone()
	if groupID != "" && data.groupIndex(groupID) < 0 {
		return userErr("所选分组已被删除")
	}
	if err := checkRoot(root, "素材根目录"); err != nil {
		return err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return userErr("新建分类失败：%v", err)
	}
	restored := false
	for _, e := range entries {
		if !strings.EqualFold(e.Name(), name) {
			continue
		}
		fi, err := os.Lstat(filepath.Join(root, e.Name()))
		if err != nil || !isPlainDir(fi) || !data.Categories[e.Name()].Hidden {
			return userErr("「%s」已经存在，可以直接选择它或换一个名称", e.Name())
		}
		name, restored = e.Name(), true
		break
	}
	if !restored {
		if err := os.Mkdir(filepath.Join(root, name), 0o755); err != nil {
			return userErr("新建分类失败：%v", err)
		}
	}
	data.Categories[name] = categoryLayout{}
	if err := data.moveCategory(name, groupID, ""); err != nil {
		return err
	}
	if err := s.saveLayout(data); err != nil {
		if !restored {
			_ = os.Remove(filepath.Join(root, name))
		}
		return err
	}
	return nil
}

func renameFolder(root, oldName, newName string) error {
	oldPath, newPath := filepath.Join(root, oldName), filepath.Join(root, newName)
	if strings.EqualFold(oldName, newName) && oldName != newName {
		// Windows 也能可靠修改大小写，临时名称不与分类冲突。
		tmp := filepath.Join(root, TempPrefix+rand.Text())
		if err := os.Rename(oldPath, tmp); err != nil {
			return err
		}
		if err := os.Rename(tmp, newPath); err != nil {
			_ = os.Rename(tmp, oldPath)
			return err
		}
		return nil
	}
	return os.Rename(oldPath, newPath)
}

func (s *Store) RenameCategory(root, oldName, newName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ValidateFolderName(newName); err != nil {
		return err
	}
	if _, err := CategoryDir(root, oldName); err != nil {
		return err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return userErr("无法读取素材根目录：%v", err)
	}
	for _, e := range entries {
		if e.Name() != oldName && strings.EqualFold(e.Name(), newName) {
			return userErr("「%s」已经存在，请换一个名称", e.Name())
		}
	}
	if oldName == newName {
		return nil
	}
	if err := renameFolder(root, oldName, newName); err != nil {
		return userErr("分类「%s」重命名失败：%v", oldName, err)
	}
	data, tags, presets, settings := s.layout.clone(), s.tags.clone(), clonePresets(s.presets), s.settings
	settings.Draft, settings.DraftRepeat = cloneDraft(settings.Draft), cloneDraft(settings.DraftRepeat)
	if c, ok := data.Categories[oldName]; ok {
		data.Categories[newName] = c
		delete(data.Categories, oldName)
	}
	for i, name := range data.Order {
		if name == oldName {
			data.Order[i] = newName
		}
	}
	if ids, ok := tags.Categories[oldName]; ok {
		tags.Categories[newName] = ids
		delete(tags.Categories, oldName)
	}
	for i := range presets {
		if p, ok := presets[i].Picks[oldName]; ok {
			presets[i].Picks[newName] = p
			delete(presets[i].Picks, oldName)
		}
	}
	for _, draft := range []map[string]int{settings.Draft, settings.DraftRepeat} {
		if n, ok := draft[oldName]; ok {
			draft[newName] = n
			delete(draft, oldName)
		}
	}
	writes := []struct {
		file string
		next any
		prev any
	}{
		{"layout.json", data, s.layout}, {"tags.json", tags, s.tags},
		{"presets.json", presets, s.presets}, {"settings.json", settings, s.settings},
	}
	for i, w := range writes {
		if err := writeJSON(filepath.Join(s.dir, w.file), w.next); err != nil {
			// 保存失败时尽量恢复名称和已经写入的关联，避免只改了一半。
			for _, done := range writes[:i] {
				_ = writeJSON(filepath.Join(s.dir, done.file), done.prev)
			}
			_ = renameFolder(root, newName, oldName)
			return userErr("无法保存分类名称：%v", err)
		}
	}
	s.layout, s.tags, s.presets, s.settings = data, tags, presets, settings
	return nil
}

type CategoryDeletion struct {
	Removed int  `json:"removed"`
	Kept    bool `json:"kept"`
}

func (s *Store) DeleteCategory(root, name string, withVideos bool) (CategoryDeletion, error) {
	return s.deleteCategory(root, name, withVideos, os.Remove)
}

func (s *Store) deleteCategory(root, name string, withVideos bool, remove func(string) error) (CategoryDeletion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := CategoryDeletion{}
	settings := s.settings
	settings.DeleteWithVideos = withVideos
	if err := writeJSON(filepath.Join(s.dir, "settings.json"), settings); err != nil {
		return result, userErr("无法保存删除选项：%v", err)
	}
	s.settings = settings
	dir, err := CategoryDir(root, name)
	if err != nil {
		return result, err
	}
	if withVideos {
		videos, err := listVideos(dir)
		if err != nil {
			return result, userErr("无法读取分类「%s」：%v", name, err)
		}
		for _, v := range videos {
			if err := remove(v.path); err != nil {
				return result, userErr("无法删除视频「%s」：%v", v.name, err)
			}
			result.Removed++
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return result, userErr("无法读取分类「%s」：%v", name, err)
	}
	result.Kept = len(entries) > 0
	data := s.layout.clone()
	if result.Kept {
		c := data.Categories[name]
		c.Hidden = true
		data.Categories[name] = c
		if !slices.Contains(data.Order, name) {
			data.Order = append(data.Order, name)
		}
	} else {
		if err := os.Remove(dir); err != nil {
			return result, userErr("无法删除分类文件夹「%s」：%v", name, err)
		}
		delete(data.Categories, name)
		data.Order = slices.DeleteFunc(data.Order, func(n string) bool { return n == name })
		tags := s.tags.clone()
		delete(tags.Categories, name)
		if err := s.saveTags(tags); err != nil {
			return result, err
		}
	}
	return result, s.saveLayout(data)
}
