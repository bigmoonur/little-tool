package library

import (
	"encoding/json"
	"slices"
	"testing"
)

func tagStore(t *testing.T) (*Store, string, string, string) {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, id, err := s.CreateList("链接", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetListText(id, "a\nb\na\nc"); err != nil {
		t.Fatal(err)
	}
	_, a, err := s.CreateTag("常用")
	if err != nil {
		t.Fatal(err)
	}
	_, b, err := s.CreateTag("新品")
	if err != nil {
		t.Fatal(err)
	}
	return s, id, a, b
}

func listTags(t *testing.T, s *Store, id string) []string {
	t.Helper()
	for _, l := range s.Snapshot().Lists {
		if l.ID == id {
			return l.TagIDs
		}
	}
	t.Fatalf("list %s missing", id)
	return nil
}

func TestListTagsBelongToTheWholeList(t *testing.T) {
	s, id, a, b := tagStore(t)
	if got := listTags(t, s, id); got == nil || len(got) != 0 {
		t.Fatalf("untagged list must report an empty, non-nil tag list: %#v", got)
	}
	updated := s.Snapshot().Lists[0].UpdatedAt
	// 重复的标签只算一次，顺序跟标签列表走。
	if _, err := s.SetListTags(id, []string{b, a, b}); err != nil {
		t.Fatal(err)
	}
	if got := listTags(t, s, id); !slices.Equal(got, []string{a, b}) {
		t.Fatalf("tags not deduplicated in tag order: %v", got)
	}
	if s.Snapshot().Lists[0].UpdatedAt != updated {
		t.Fatal("tagging a list must not count as editing its content")
	}

	before, _ := s.ExportData()
	if _, err := s.SetListTags(id, []string{a, "missing"}); err == nil {
		t.Fatal("deleted tag accepted")
	}
	if _, err := s.SetListTags("missing", []string{a}); err == nil {
		t.Fatal("tags set on a missing list")
	}
	if after, _ := s.ExportData(); string(before) != string(after) {
		t.Fatal("rejected change still modified stored tags")
	}

	// 标签不影响抽取：贴了标签的分类照样从全部内容里抽。
	res, err := s.Extract([]PickRequest{{ListID: id, Count: 4, Repeat: 1}})
	if err != nil || res.Total != 4 {
		t.Fatalf("tags must not narrow extraction: %+v %v", res, err)
	}

	got := listTags(t, s, id)
	got[0] = "changed"
	if listTags(t, s, id)[0] != a {
		t.Fatal("snapshot tags alias stored state")
	}
	if _, err := s.SetListTags(id, nil); err != nil || len(listTags(t, s, id)) != 0 {
		t.Fatal("tags could not be cleared")
	}
}

func TestListTagsSurviveEditingImportDeleteAndUndo(t *testing.T) {
	s, id, a, b := tagStore(t)
	if _, err := s.SetListTags(id, []string{a, b}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetListText(id, "c\nnew"); err != nil {
		t.Fatal(err)
	}
	plan := ImportPlan{Items: []ImportItem{{Mode: ImportAppend, TargetID: id, Text: "extra"}}}
	if _, _, err := s.ApplyImport(plan); err != nil {
		t.Fatal(err)
	}
	if got := listTags(t, s, id); !slices.Equal(got, []string{a, b}) {
		t.Fatalf("editing or importing content dropped the list's tags: %v", got)
	}

	if _, err := s.DeleteTag(a); err != nil {
		t.Fatal(err)
	}
	if got := listTags(t, s, id); !slices.Equal(got, []string{b}) {
		t.Fatalf("deleted tag still on the list: %v", got)
	}
	if _, err := s.Undo(); err != nil {
		t.Fatal(err)
	}
	if got := listTags(t, s, id); !slices.Equal(got, []string{a, b}) {
		t.Fatalf("undoing a tag deletion did not put it back on the list: %v", got)
	}
}

func TestTagColors(t *testing.T) {
	s, _, a, _ := tagStore(t)
	tags := s.Snapshot().Tags
	if !validColor(tags[0].Color) || !validColor(tags[1].Color) || tags[0].Color == tags[1].Color {
		t.Fatalf("new tags must get distinct valid colors: %+v", tags)
	}
	if _, err := s.SetTagColor(a, "nope"); err == nil {
		t.Fatal("invalid color accepted")
	}
	if _, err := s.SetTagColor("missing", "rose"); err == nil {
		t.Fatal("color set on a missing tag")
	}
	snap, err := s.SetTagColor(a, "rose")
	if err != nil || snap.Tags[0].Color != "rose" {
		t.Fatalf("color not saved: %+v %v", snap.Tags, err)
	}
}

func TestTagsBackupAndOldLibraryCompatibility(t *testing.T) {
	s, id, a, _ := tagStore(t)
	if _, err := s.SetListTags(id, []string{a}); err != nil {
		t.Fatal(err)
	}
	backup, _ := s.ExportData()
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(backup, &raw); err != nil || len(raw["tags"]) == 0 {
		t.Fatal("tags missing in backup")
	}
	reopened, err := Open(s.Dir())
	if err != nil {
		t.Fatal(err)
	}
	if got := listTags(t, reopened, id); !slices.Equal(got, []string{a}) {
		t.Fatalf("list tags lost on reopen: %v", got)
	}
	other, _ := Open(t.TempDir())
	if _, err := other.RestoreData(backup); err != nil {
		t.Fatal(err)
	}
	if got := listTags(t, other, id); !slices.Equal(got, []string{a}) {
		t.Fatalf("backup restore lost list tags: %v", got)
	}

	// 没有标签的旧素材库。
	plain := []byte(`{"version":1,"groups":[],"lists":[{"id":"old","name":"旧素材","items":["a","b"]}],"presets":[],"draft":{}}`)
	if _, err := other.RestoreData(plain); err != nil {
		t.Fatal(err)
	}
	if got := listTags(t, other, "old"); got == nil || len(got) != 0 {
		t.Fatalf("old data not normalized: %#v", got)
	}

	// 早先按行贴标签的素材库：行上的标签并到分类上，没有颜色的标签补上颜色，
	// 方案里按标签筛选的条件不再使用，数量和乘数规则照旧。
	lines := []byte(`{"version":1,"groups":[],
		"tags":[{"id":"t1","name":"甲"},{"id":"t2","name":"乙","color":"bad"},{"id":"t3","name":"丙"}],
		"lists":[{"id":"l1","name":"链接","items":["a","b","c"],"itemTags":[["t2"],[],["t1","t2","gone"]]}],
		"presets":[{"id":"p1","name":"方案","picks":{"l1":{"count":2,"repeat":3,"mode":"cycle","tagIds":["t1"]}}}],
		"draft":{}}`)
	snap, err := other.RestoreData(lines)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(snap.Lists[0].TagIDs, []string{"t1", "t2"}) {
		t.Fatalf("line tags not merged onto the list: %v", snap.Lists[0].TagIDs)
	}
	if snap.Tags[0].Color != GroupColors[0] || snap.Tags[1].Color != GroupColors[1] {
		t.Fatalf("old tags not given colors: %+v", snap.Tags)
	}
	if p := snap.Presets[0].Picks["l1"]; p.Count != 2 || p.Repeat != 3 || p.Mode != RepeatCycle {
		t.Fatalf("old preset lost its numbers: %+v", p)
	}
	saved, _ := other.ExportData()
	if err := json.Unmarshal(saved, &raw); err != nil || containsKey(raw["lists"], "itemTags") {
		t.Fatal("per-line tags written back out")
	}
}

func containsKey(raw json.RawMessage, key string) bool {
	var lists []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &lists); err != nil {
		return false
	}
	for _, l := range lists {
		if _, ok := l[key]; ok {
			return true
		}
	}
	return false
}
