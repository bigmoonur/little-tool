package library

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
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

func setTags(t *testing.T, s *Store, id string, indexes []int, tags []string, action string) {
	t.Helper()
	items, err := s.ListItems(id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetItemTags(id, indexes, tags, items.UpdatedAt, action); err != nil {
		t.Fatal(err)
	}
}

func TestTextTagsFilteringUsesEachLineAndUnion(t *testing.T) {
	s, id, a, b := tagStore(t)
	setTags(t, s, id, []int{0, 1}, []string{a}, "set")
	setTags(t, s, id, []int{1, 3}, []string{b}, "add")
	items, _ := s.ListItems(id)
	if len(items.Items[2].TagIDs) != 0 || !slices.Equal(items.Items[1].TagIDs, []string{a, b}) {
		t.Fatalf("identical lines must keep independent tags: %+v", items)
	}
	for _, test := range []struct {
		tags          []string
		count, unique int
	}{
		{nil, 4, 3}, {[]string{a}, 2, 2}, {[]string{b}, 2, 2}, {[]string{a, b}, 3, 3},
	} {
		lists, err := s.AvailableLists(test.tags)
		if err != nil || lists[0].Count != test.count || lists[0].Unique != test.unique {
			t.Fatalf("availability for %v = %+v, %v", test.tags, lists, err)
		}
	}
	res, err := s.Extract([]PickRequest{{ListID: id, Count: 2, Repeat: 3, Mode: RepeatCycle, TagIDs: []string{b}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(res.Text, "\n") {
		if line != "b" && line != "c" {
			t.Fatalf("extracted untagged text %q", line)
		}
	}
	if !slices.Equal(res.Parts[0].TagIDs, []string{b}) || res.Parts[0].Mode != RepeatCycle {
		t.Fatalf("missing repeat/filter metadata: %+v", res.Parts)
	}
	if _, err := s.Extract([]PickRequest{{ListID: id, Count: 3, Repeat: 1, TagIDs: []string{b}}}); err == nil {
		t.Fatal("must enforce filtered capacity in the backend")
	}
}

func TestTagsSurviveEditingImportAndUndo(t *testing.T) {
	s, id, a, b := tagStore(t)
	setTags(t, s, id, []int{0}, []string{a}, "set")
	setTags(t, s, id, []int{2}, []string{b}, "set")
	before, _ := s.ListItems(id)
	if _, err := s.SetListText(id, "c\na\nnew\na"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.ListItems(id)
	if !slices.Equal(got.Items[1].TagIDs, []string{a}) || !slices.Equal(got.Items[3].TagIDs, []string{b}) || len(got.Items[2].TagIDs) != 0 {
		t.Fatalf("tags did not follow unchanged text occurrences: %+v", got)
	}
	plan := ImportPlan{Items: []ImportItem{{Mode: ImportAppend, TargetID: id, Text: "extra"}}}
	s.PreviewImport(plan)
	afterPreview, _ := s.ListItems(id)
	if !reflect.DeepEqual(got, afterPreview) {
		t.Fatal("preview mutated text tags")
	}
	if _, _, err := s.ApplyImport(plan); err != nil {
		t.Fatal(err)
	}
	afterImport, _ := s.ListItems(id)
	if !slices.Equal(afterImport.Items[1].TagIDs, []string{a}) || len(afterImport.Items[4].TagIDs) != 0 {
		t.Fatal("append lost tags")
	}
	if _, err := s.Undo(); err != nil {
		t.Fatal(err)
	}
	undone, _ := s.ListItems(id)
	if !reflect.DeepEqual(got, undone) {
		t.Fatalf("undo lost tags: %+v", undone)
	}
	if _, err := s.SetItemTags(id, []int{0}, []string{a}, before.UpdatedAt, "set"); err == nil {
		t.Fatal("stale line revision accepted")
	}
	if _, err := s.DeleteTag(a); err != nil {
		t.Fatal(err)
	}
	deleted, _ := s.ListItems(id)
	if len(deleted.Items[1].TagIDs) != 0 {
		t.Fatal("deleted tag remains assigned")
	}
	if _, err := s.Extract([]PickRequest{{ListID: id, Count: 1, Repeat: 1, TagIDs: []string{a}}}); err == nil {
		t.Fatal("deleted filter must fail instead of widening the pool")
	}
	if _, err := s.Undo(); err != nil {
		t.Fatal(err)
	}
	restored, _ := s.ListItems(id)
	if !reflect.DeepEqual(got, restored) {
		t.Fatal("tag deletion undo did not restore assignments")
	}
}

func TestTextTagBatchValidationAndIsolation(t *testing.T) {
	s, id, a, b := tagStore(t)
	setTags(t, s, id, []int{0, 1}, []string{a, a}, "set")
	setTags(t, s, id, []int{1}, []string{b}, "add")
	setTags(t, s, id, []int{0, 1}, []string{a}, "remove")
	items, _ := s.ListItems(id)
	if len(items.Items[0].TagIDs) != 0 || !slices.Equal(items.Items[1].TagIDs, []string{b}) {
		t.Fatalf("remove erased other tags: %+v", items)
	}
	before, _ := s.ExportData()
	for _, tc := range []struct {
		indexes  []int
		tags     []string
		revision int64
		action   string
	}{
		{[]int{0, 99}, []string{a}, items.UpdatedAt, "set"},
		{[]int{0}, []string{"missing"}, items.UpdatedAt, "set"},
		{[]int{0}, []string{a}, items.UpdatedAt - 1, "set"},
		{nil, []string{a}, items.UpdatedAt, "set"},
		{[]int{0}, []string{a}, items.UpdatedAt, "invalid"},
	} {
		if _, err := s.SetItemTags(id, tc.indexes, tc.tags, tc.revision, tc.action); err == nil {
			t.Fatal("invalid batch accepted")
		}
		after, _ := s.ExportData()
		if string(before) != string(after) {
			t.Fatal("failed batch partially changed stored tags")
		}
	}
	items.Items[1].TagIDs[0] = "changed"
	stored, _ := s.ListItems(id)
	if stored.Items[1].TagIDs[0] != b {
		t.Fatal("returned item tags alias stored state")
	}
}

func TestTagsBackupPersistenceAndOldLibraryCompatibility(t *testing.T) {
	s, id, a, _ := tagStore(t)
	setTags(t, s, id, []int{1}, []string{a}, "set")
	picks := map[string]Pick{id: {Count: 1, Repeat: 4, Mode: RepeatRandom, TagIDs: []string{a}}}
	if _, _, err := s.SavePreset("新品链接", picks); err != nil {
		t.Fatal(err)
	}
	picks[id].TagIDs[0] = "alias"
	backup, _ := s.ExportData()
	reopened, err := Open(s.Dir())
	if err != nil {
		t.Fatal(err)
	}
	items, _ := reopened.ListItems(id)
	if !slices.Equal(items.Items[1].TagIDs, []string{a}) {
		t.Fatal("tags lost on reopen")
	}
	if reopened.Snapshot().Presets[0].Picks[id].TagIDs[0] != a {
		t.Fatal("preset tags alias caller state")
	}
	other, _ := Open(t.TempDir())
	if _, err := other.RestoreData(backup); err != nil {
		t.Fatal(err)
	}
	restored, _ := other.ListItems(id)
	if !reflect.DeepEqual(items, restored) {
		t.Fatal("backup restore lost tag assignments")
	}
	old := []byte(`{"version":1,"groups":[],"lists":[{"id":"old","name":"旧素材","items":["a","b"]}],"presets":[],"draft":{}}`)
	if _, err := other.RestoreData(old); err != nil {
		t.Fatal(err)
	}
	oldItems, _ := other.ListItems("old")
	if len(oldItems.Items) != 2 || oldItems.Items[0].TagIDs == nil {
		t.Fatalf("old data not normalized: %+v", oldItems)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(backup, &raw); err != nil || len(raw["tags"]) == 0 {
		t.Fatal("tags missing in backup")
	}
}
