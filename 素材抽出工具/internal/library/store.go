package library

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math/rand/v2"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	libraryFile    = "library.json"
	historyFile    = "history.json"
	windowFile     = "window.json"
	backupDir      = "backups"
	keepBackups    = 20
	backupInterval = 30 * time.Minute

	MaxHistory          = 30
	maxHistoryTextBytes = 2 << 20
)

type ListInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	GroupID   string `json:"groupId"`
	Count     int    `json:"count"`
	Unique    int    `json:"unique"`
	UpdatedAt int64  `json:"updatedAt"`
}

// Snapshot 是发给界面的素材库概要，不含条目正文。
type Snapshot struct {
	Groups   []Group         `json:"groups"`
	Tags     []Tag           `json:"tags"`
	Lists    []ListInfo      `json:"lists"`
	Presets  []Preset        `json:"presets"`
	Draft    map[string]Pick `json:"draft"`
	Settings Settings        `json:"settings"`
	Undo     string          `json:"undo"`
}

type HistoryEntry struct {
	ID        string     `json:"id"`
	Time      int64      `json:"time"`
	Total     int        `json:"total"`
	Parts     []PickPart `json:"parts"`
	Text      string     `json:"text,omitempty"`
	Truncated bool       `json:"truncated"`
}

type undoEntry struct {
	label   string
	groups  []Group
	tags    []Tag
	lists   []List
	presets []Preset
}

type Store struct {
	mu         sync.Mutex
	dir        string
	data       *Data
	history    []HistoryEntry
	window     *WindowState
	undo       *undoEntry
	lastBackup time.Time
	rng        *rand.Rand
	now        func() time.Time

	// Notice 在启动时从备份恢复等情况下提示用户。
	Notice string
}

// Open 读取 dir 中的素材库；文件不存在时创建空库，文件损坏时尝试从最近的备份恢复。
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s := &Store{
		dir: dir,
		rng: rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64())),
		now: time.Now,
	}
	data, err := readData(filepath.Join(dir, libraryFile))
	switch {
	case err == nil:
		s.data = data
	case errors.Is(err, os.ErrNotExist):
		s.data = NewData()
	default:
		// 保留损坏的文件以便人工处理，再尝试最近的备份。
		broken := filepath.Join(dir, fmt.Sprintf("library.damaged-%s.json", time.Now().Format("20060102-150405")))
		_ = os.Rename(filepath.Join(dir, libraryFile), broken)
		s.data = NewData()
		for _, b := range s.backups() {
			if d, err := readData(b); err == nil {
				s.data = d
				s.Notice = "素材库文件损坏，已从 " + backupTime(b) + " 的自动备份恢复。"
				break
			}
		}
		if s.Notice == "" {
			s.Notice = "素材库文件损坏且没有可用备份，已新建空素材库。损坏的文件已保留在数据文件夹中。"
		}
		if err := s.save(); err != nil {
			return nil, err
		}
	}
	s.history = readHistory(filepath.Join(dir, historyFile))
	if b, err := os.ReadFile(filepath.Join(dir, windowFile)); err == nil {
		var w WindowState
		if json.Unmarshal(b, &w) == nil {
			s.window = &w
		}
	}
	return s, nil
}

func readData(path string) (*Data, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	d := NewData()
	if err := json.Unmarshal(b, d); err != nil {
		return nil, err
	}
	d.normalize()
	return d, nil
}

func readHistory(path string) []HistoryEntry {
	b, err := os.ReadFile(path)
	if err != nil {
		return []HistoryEntry{}
	}
	var h []HistoryEntry
	if json.Unmarshal(b, &h) != nil {
		return []HistoryEntry{}
	}
	return h
}

func (s *Store) Dir() string { return s.dir }

// ---------- 持久化 ----------

func writeFileAtomic(path string, b []byte) error {
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

func (s *Store) save() error {
	s.backupIfDue(false)
	b, err := json.Marshal(s.data)
	if err != nil {
		return err
	}
	if err := writeFileAtomic(filepath.Join(s.dir, libraryFile), b); err != nil {
		return fmt.Errorf("保存素材库失败：%w", err)
	}
	return nil
}

func (s *Store) saveHistory() error {
	b, err := json.Marshal(s.history)
	if err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(s.dir, historyFile), b)
}

// backupIfDue 在覆盖 library.json 之前把旧文件复制到 backups 目录。
func (s *Store) backupIfDue(force bool) {
	if !force && time.Since(s.lastBackup) < backupInterval {
		return
	}
	src := filepath.Join(s.dir, libraryFile)
	b, err := os.ReadFile(src)
	if err != nil {
		return
	}
	dir := filepath.Join(s.dir, backupDir)
	if os.MkdirAll(dir, 0o755) != nil {
		return
	}
	name := "library-" + s.now().Format("20060102-150405") + ".json"
	if writeFileAtomic(filepath.Join(dir, name), b) != nil {
		return
	}
	s.lastBackup = time.Now()
	files := s.backups()
	for _, old := range files[min(len(files), keepBackups):] {
		os.Remove(old)
	}
}

// backups 返回自动备份文件，最新的在前。
func (s *Store) backups() []string {
	files, _ := filepath.Glob(filepath.Join(s.dir, backupDir, "library-*.json"))
	sort.Sort(sort.Reverse(sort.StringSlice(files)))
	return files
}

func backupTime(path string) string {
	base := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(path), "library-"), ".json")
	t, err := time.ParseInLocation("20060102-150405", base, time.Local)
	if err != nil {
		return base
	}
	return t.Format("2006-01-02 15:04")
}

// ---------- 撤销 ----------

// remember 在修改内容前记录当前状态，界面可以撤销这一步。
func (s *Store) remember(label string) {
	s.undo = &undoEntry{
		label:   label,
		groups:  slices.Clone(s.data.Groups),
		tags:    slices.Clone(s.data.Tags),
		lists:   slices.Clone(s.data.Lists),
		presets: clonePresets(s.data.Presets),
	}
}

func clonePresets(ps []Preset) []Preset {
	out := slices.Clone(ps)
	for i := range out {
		out[i].Picks = clonePicks(out[i].Picks)
	}
	return out
}

func clonePicks(ps map[string]Pick) map[string]Pick {
	out := maps.Clone(ps)
	for id, p := range out {
		p.TagIDs = slices.Clone(p.TagIDs)
		out[id] = p
	}
	return out
}

// mutate 执行一次内容修改并保存；undoLabel 为空表示这次修改不可撤销。
// 条目切片只会整体替换、从不原地修改，所以撤销记录可以与当前数据共享底层数组。
func (s *Store) mutate(undoLabel string, fn func(d *Data) error) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev := s.undo
	if undoLabel != "" {
		s.remember(undoLabel)
	} else {
		s.undo = nil
	}
	backup := *s.data
	backup.Groups = slices.Clone(s.data.Groups)
	backup.Tags = slices.Clone(s.data.Tags)
	backup.Lists = slices.Clone(s.data.Lists)
	backup.Presets = clonePresets(s.data.Presets)
	backup.Draft = clonePicks(s.data.Draft)
	if err := fn(s.data); err != nil {
		*s.data = backup
		s.undo = prev
		return s.snapshot(), err
	}
	if err := s.save(); err != nil {
		*s.data = backup
		s.undo = prev
		return s.snapshot(), err
	}
	return s.snapshot(), nil
}

// Undo 撤销最近一次可撤销的修改。
func (s *Store) Undo() (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.undo == nil {
		return s.snapshot(), userErr("没有可以撤销的操作")
	}
	u := s.undo
	cur := *s.data
	s.data.Groups, s.data.Tags, s.data.Lists, s.data.Presets = u.groups, u.tags, u.lists, u.presets
	s.undo = nil
	if err := s.save(); err != nil {
		*s.data = cur
		s.undo = u
		return s.snapshot(), err
	}
	return s.snapshot(), nil
}

// ---------- 读取 ----------

func (s *Store) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshot()
}

func (s *Store) snapshot() Snapshot {
	d := s.data
	snap := Snapshot{
		Groups:   slices.Clone(d.Groups),
		Tags:     slices.Clone(d.Tags),
		Lists:    make([]ListInfo, len(d.Lists)),
		Presets:  clonePresets(d.Presets),
		Draft:    clonePicks(d.Draft),
		Settings: d.Settings,
	}
	for i, l := range d.Lists {
		snap.Lists[i] = ListInfo{l.ID, l.Name, l.GroupID, len(l.Items), l.unique, l.UpdatedAt}
	}
	if s.undo != nil {
		snap.Undo = s.undo.label
	}
	return snap
}

func (s *Store) ListText(id string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.data.listIndex(id)
	if i < 0 {
		return "", userErr("分类不存在，可能已被删除")
	}
	return strings.Join(s.data.Lists[i].Items, "\n"), nil
}

func (s *Store) ListName(id string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if i := s.data.listIndex(id); i >= 0 {
		return s.data.Lists[i].Name
	}
	return ""
}

func (s *Store) Settings() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.Settings
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

func (d *Data) listIndex(id string) int {
	return slices.IndexFunc(d.Lists, func(l List) bool { return l.ID == id })
}

func (d *Data) groupIndex(id string) int {
	return slices.IndexFunc(d.Groups, func(g Group) bool { return g.ID == id })
}

func (d *Data) listNameTaken(name, exceptID string) bool {
	for _, l := range d.Lists {
		if l.ID != exceptID && sameName(l.Name, name) {
			return true
		}
	}
	return false
}

func (d *Data) groupNameTaken(name, exceptID string) bool {
	for _, g := range d.Groups {
		if g.ID != exceptID && sameName(g.Name, name) {
			return true
		}
	}
	return false
}

// ---------- 分组 ----------

func (s *Store) CreateGroup(name string) (Snapshot, string, error) {
	var id string
	snap, err := s.mutate("", func(d *Data) error {
		n, err := CleanName(name, "分组")
		if err != nil {
			return err
		}
		if d.groupNameTaken(n, "") {
			return userErr("已有同名分组「%s」", n)
		}
		id = NewID()
		d.Groups = append(d.Groups, Group{ID: id, Name: n, Color: GroupColors[len(d.Groups)%len(GroupColors)]})
		return nil
	})
	return snap, id, err
}

func (s *Store) RenameGroup(id, name string) (Snapshot, error) {
	return s.mutate("", func(d *Data) error {
		i := d.groupIndex(id)
		if i < 0 {
			return userErr("分组不存在，可能已被删除")
		}
		n, err := CleanName(name, "分组")
		if err != nil {
			return err
		}
		if d.groupNameTaken(n, id) {
			return userErr("已有同名分组「%s」", n)
		}
		d.Groups[i].Name = n
		return nil
	})
}

func (s *Store) SetGroupColor(id, color string) (Snapshot, error) {
	return s.mutate("", func(d *Data) error {
		i := d.groupIndex(id)
		if i < 0 {
			return userErr("分组不存在，可能已被删除")
		}
		if !validColor(color) {
			return userErr("不支持的颜色")
		}
		d.Groups[i].Color = color
		return nil
	})
}

func (s *Store) SetGroupCollapsed(id string, collapsed bool) Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	if i := s.data.groupIndex(id); i >= 0 && s.data.Groups[i].Collapsed != collapsed {
		s.data.Groups[i].Collapsed = collapsed
		_ = s.save()
	}
	return s.snapshot()
}

// MoveGroup 把分组移到 index 位置（按移除自身后的顺序计算）。
func (s *Store) MoveGroup(id string, index int) (Snapshot, error) {
	return s.mutate("", func(d *Data) error {
		i := d.groupIndex(id)
		if i < 0 {
			return userErr("分组不存在，可能已被删除")
		}
		g := d.Groups[i]
		d.Groups = slices.Delete(d.Groups, i, i+1)
		index = max(0, min(index, len(d.Groups)))
		d.Groups = slices.Insert(d.Groups, index, g)
		return nil
	})
}

// DeleteGroup 删除分组；deleteLists 为 false 时其中的分类移到“未分组”。
func (s *Store) DeleteGroup(id string, deleteLists bool) (Snapshot, error) {
	s.mu.Lock()
	name := ""
	if i := s.data.groupIndex(id); i >= 0 {
		name = s.data.Groups[i].Name
	}
	s.mu.Unlock()
	return s.mutate("删除分组「"+name+"」", func(d *Data) error {
		i := d.groupIndex(id)
		if i < 0 {
			return userErr("分组不存在，可能已被删除")
		}
		s.backupIfDue(true)
		d.Groups = slices.Delete(d.Groups, i, i+1)
		if deleteLists {
			d.Lists = slices.DeleteFunc(d.Lists, func(l List) bool { return l.GroupID == id })
		} else {
			for k := range d.Lists {
				if d.Lists[k].GroupID == id {
					d.Lists[k].GroupID = ""
				}
			}
		}
		return nil
	})
}

// ---------- 分类 ----------

func (s *Store) CreateList(name, groupID string) (Snapshot, string, error) {
	var id string
	snap, err := s.mutate("", func(d *Data) error {
		n, err := CleanName(name, "分类")
		if err != nil {
			return err
		}
		if d.listNameTaken(n, "") {
			return userErr("已有同名分类「%s」", n)
		}
		if groupID != "" && d.groupIndex(groupID) < 0 {
			return userErr("所选分组已被删除")
		}
		id = NewID()
		l := List{ID: id, Name: n, GroupID: groupID, UpdatedAt: s.now().UnixMilli()}
		l.setItems([]string{})
		d.Lists = append(d.Lists, l)
		return nil
	})
	return snap, id, err
}

func (s *Store) RenameList(id, name string) (Snapshot, error) {
	return s.mutate("", func(d *Data) error {
		i := d.listIndex(id)
		if i < 0 {
			return userErr("分类不存在，可能已被删除")
		}
		n, err := CleanName(name, "分类")
		if err != nil {
			return err
		}
		if d.listNameTaken(n, id) {
			return userErr("已有同名分类「%s」", n)
		}
		d.Lists[i].Name = n
		return nil
	})
}

// MoveList 把分类移到 groupID 分组中，放在 beforeID 之前；beforeID 为空时放在该分组末尾。
func (s *Store) MoveList(id, groupID, beforeID string) (Snapshot, error) {
	return s.mutate("", func(d *Data) error {
		i := d.listIndex(id)
		if i < 0 {
			return userErr("分类不存在，可能已被删除")
		}
		if groupID != "" && d.groupIndex(groupID) < 0 {
			return userErr("分组不存在，可能已被删除")
		}
		l := d.Lists[i]
		l.GroupID = groupID
		d.Lists = slices.Delete(d.Lists, i, i+1)
		at := -1
		if beforeID != "" && beforeID != id {
			at = d.listIndex(beforeID)
		}
		if at < 0 {
			at = len(d.Lists)
			for k := len(d.Lists) - 1; k >= 0; k-- {
				if d.Lists[k].GroupID == groupID {
					at = k + 1
					break
				}
			}
		}
		d.Lists = slices.Insert(d.Lists, at, l)
		return nil
	})
}

func (s *Store) DeleteList(id string) (Snapshot, error) {
	return s.mutate("删除分类「"+s.ListName(id)+"」", func(d *Data) error {
		i := d.listIndex(id)
		if i < 0 {
			return userErr("分类不存在，可能已被删除")
		}
		s.backupIfDue(true)
		d.Lists = slices.Delete(d.Lists, i, i+1)
		delete(d.Draft, id)
		return nil
	})
}

// SetListText 用编辑器中的全文替换分类内容。
func (s *Store) SetListText(id, text string) (Snapshot, error) {
	return s.mutate("修改「"+s.ListName(id)+"」", func(d *Data) error {
		i := d.listIndex(id)
		if i < 0 {
			return userErr("分类不存在，可能已被删除")
		}
		lines, _ := SplitLines(text)
		d.Lists[i].setItems(lines)
		d.Lists[i].UpdatedAt = max(d.Lists[i].UpdatedAt+1, s.now().UnixMilli())
		return nil
	})
}

// DedupeList 删除分类中的重复行，保留每条内容第一次出现的位置。
func (s *Store) DedupeList(id string) (Snapshot, int, error) {
	removed := 0
	snap, err := s.mutate("去除「"+s.ListName(id)+"」的重复内容", func(d *Data) error {
		i := d.listIndex(id)
		if i < 0 {
			return userErr("分类不存在，可能已被删除")
		}
		l := &d.Lists[i]
		if l.unique == len(l.Items) {
			return userErr("「%s」没有重复内容", l.Name)
		}
		removed = len(l.Items) - l.unique
		l.setItems(dedupe(l.Items))
		l.UpdatedAt = max(l.UpdatedAt+1, s.now().UnixMilli())
		return nil
	})
	return snap, removed, err
}

// ---------- 导入 ----------

func (s *Store) PreviewImport(plan ImportPlan) ImportReport {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, rep := planImport(s.data, plan, s.now().UnixMilli())
	return rep
}

func (s *Store) ApplyImport(plan ImportPlan) (Snapshot, ImportReport, error) {
	var rep ImportReport
	snap, err := s.mutate("添加素材", func(d *Data) error {
		var lists []List
		lists, rep = planImport(d, plan, s.now().UnixMilli())
		if !rep.OK {
			return userErr("%s", rep.Error)
		}
		if rep.Removed > 0 {
			s.backupIfDue(true)
		}
		d.Lists = lists
		return nil
	})
	return snap, rep, err
}

// ---------- 抽取 ----------

func (s *Store) Extract(reqs []PickRequest) (ExtractResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, req := range reqs {
		if err := s.data.checkTags(req.TagIDs); err != nil {
			return ExtractResult{}, err
		}
	}
	return Extract(s.data.Lists, reqs, s.rng)
}

func (s *Store) AddHistory(res ExtractResult) HistoryEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := HistoryEntry{
		ID:    NewID(),
		Time:  s.now().UnixMilli(),
		Total: res.Total,
		Parts: res.Parts,
		Text:  res.Text,
	}
	if len(e.Text) > maxHistoryTextBytes {
		e.Text = ""
		e.Truncated = true
	}
	s.history = append([]HistoryEntry{e}, s.history...)
	if len(s.history) > MaxHistory {
		s.history = s.history[:MaxHistory]
	}
	_ = s.saveHistory()
	return e
}

// History 返回抽取记录，正文只保留前几行用于列表预览。
func (s *Store) History() []HistoryEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]HistoryEntry, len(s.history))
	for i, e := range s.history {
		e.Text = firstLines(e.Text, 3)
		out[i] = e
	}
	return out
}

func (s *Store) HistoryText(id string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.history {
		if e.ID == id {
			if e.Truncated {
				return "", userErr("这次抽取的内容过大，没有保存在记录中")
			}
			return e.Text, nil
		}
	}
	return "", userErr("记录不存在")
}

func (s *Store) ClearHistory() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = []HistoryEntry{}
	return s.saveHistory()
}

func firstLines(text string, n int) string {
	idx := 0
	for i := 0; i < n; i++ {
		k := strings.IndexByte(text[idx:], '\n')
		if k < 0 {
			return text
		}
		idx += k + 1
	}
	return strings.TrimSuffix(text[:idx], "\n")
}

// ---------- 方案 ----------

// SavePreset 保存抽取方案；同名方案会被覆盖。
func (s *Store) SavePreset(name string, picks map[string]Pick) (Snapshot, string, error) {
	var id string
	snap, err := s.mutate("", func(d *Data) error {
		n, err := CleanName(name, "方案")
		if err != nil {
			return err
		}
		clean := map[string]Pick{}
		for listID, p := range picks {
			if p.Count > 0 && d.listIndex(listID) >= 0 {
				if err := d.checkTags(p.TagIDs); err != nil {
					return err
				}
				if !validRepeatMode(p.Mode) {
					return userErr("不支持的乘数规则")
				}
				clean[listID] = Pick{Count: p.Count, Repeat: max(1, p.Repeat), Mode: p.Mode, TagIDs: slices.Clone(p.TagIDs)}
			}
		}
		if len(clean) == 0 {
			return userErr("请先为至少一个分类填写数量，再保存为方案")
		}
		for i := range d.Presets {
			if sameName(d.Presets[i].Name, n) {
				d.Presets[i].Picks = clean
				id = d.Presets[i].ID
				return nil
			}
		}
		id = NewID()
		d.Presets = append(d.Presets, Preset{ID: id, Name: n, Picks: clean})
		return nil
	})
	return snap, id, err
}

func (s *Store) RenamePreset(id, name string) (Snapshot, error) {
	return s.mutate("", func(d *Data) error {
		n, err := CleanName(name, "方案")
		if err != nil {
			return err
		}
		idx := -1
		for i := range d.Presets {
			if d.Presets[i].ID == id {
				idx = i
			} else if sameName(d.Presets[i].Name, n) {
				return userErr("已有同名方案「%s」", n)
			}
		}
		if idx < 0 {
			return userErr("方案不存在，可能已被删除")
		}
		d.Presets[idx].Name = n
		return nil
	})
}

func (s *Store) DeletePreset(id string) (Snapshot, error) {
	name := ""
	s.mu.Lock()
	for _, p := range s.data.Presets {
		if p.ID == id {
			name = p.Name
		}
	}
	s.mu.Unlock()
	return s.mutate("删除方案「"+name+"」", func(d *Data) error {
		n := len(d.Presets)
		d.Presets = slices.DeleteFunc(d.Presets, func(p Preset) bool { return p.ID == id })
		if len(d.Presets) == n {
			return userErr("方案不存在，可能已被删除")
		}
		return nil
	})
}

// ---------- 设置与界面状态（不影响撤销） ----------

func (s *Store) UpdateSettings(st Settings) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev := s.data.Settings
	s.data.Settings = st
	s.data.Settings.normalize()
	if !st.RememberDraft {
		s.data.Draft = map[string]Pick{}
	}
	if err := s.save(); err != nil {
		s.data.Settings = prev
		return s.snapshot(), err
	}
	return s.snapshot(), nil
}

func (s *Store) SaveDraft(draft map[string]Pick) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.data.Settings.RememberDraft {
		return
	}
	clean := map[string]Pick{}
	for id, p := range draft {
		if (p.Count > 0 || p.Repeat > 1) && s.data.listIndex(id) >= 0 {
			p.TagIDs = slices.Clone(p.TagIDs)
			clean[id] = p
		}
	}
	if reflect.DeepEqual(clean, s.data.Draft) {
		return
	}
	s.data.Draft = clean
	_ = s.save()
}

func (s *Store) SaveWindow(w WindowState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// 窗口大小单独保存在小文件里，关闭窗口时不必重写整个素材库。
	if s.window != nil && *s.window == w {
		return
	}
	s.window = &w
	if b, err := json.Marshal(w); err == nil {
		_ = writeFileAtomic(filepath.Join(s.dir, windowFile), b)
	}
}

// ---------- 备份 ----------

// ExportData 返回用于“导出备份”的 JSON。
func (s *Store) ExportData() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return json.MarshalIndent(s.data, "", " ")
}

// RestoreData 用备份文件的内容替换素材库；当前内容会先写入自动备份，且可撤销。
func (s *Store) RestoreData(b []byte) (Snapshot, error) {
	var probe struct {
		Lists json.RawMessage `json:"lists"`
	}
	d := NewData()
	if json.Unmarshal(b, &probe) != nil || len(probe.Lists) == 0 || json.Unmarshal(b, d) != nil {
		return s.Snapshot(), userErr("这不是有效的链接抽取工具备份文件")
	}
	d.normalize()
	return s.mutate("从备份恢复", func(cur *Data) error {
		s.backupIfDue(true)
		cur.Groups, cur.Tags, cur.Lists, cur.Presets = d.Groups, d.Tags, d.Lists, d.Presets
		cur.Draft = map[string]Pick{}
		return nil
	})
}
