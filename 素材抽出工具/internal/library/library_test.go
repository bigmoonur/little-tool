package library

import (
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newList(name string, items ...string) List {
	l := List{ID: NewID(), Name: name}
	l.setItems(items)
	return l
}

func testRand() *rand.Rand { return rand.New(rand.NewPCG(1, 2)) }

func TestSplitLines(t *testing.T) {
	lines, blank := SplitLines(string(rune(0xfeff)) + "a\r\n  b \n\n\t\nc\n")
	if strings.Join(lines, "|") != "a|b|c" {
		t.Fatalf("lines = %q", lines)
	}
	if blank != 2 {
		t.Fatalf("blank = %d", blank)
	}
}

func TestExtractPlainCountsByLine(t *testing.T) {
	l := newList("A", "x", "x", "y")
	res, err := Extract([]List{l}, []PickRequest{{ListID: l.ID, Count: 3, Repeat: 1}}, testRand())
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Split(res.Text, "\n")
	if len(got) != 3 || res.Total != 3 {
		t.Fatalf("got %q total %d", got, res.Total)
	}
	if _, err := Extract([]List{l}, []PickRequest{{ListID: l.ID, Count: 4, Repeat: 1}}, testRand()); err == nil {
		t.Fatal("expected error when count exceeds lines")
	}
}

func TestExtractRepeatUsesDistinctContentAndKeepsRunsTogether(t *testing.T) {
	a := newList("A", "a1", "a1", "a2", "a3")
	b := newList("B", "b1", "b2")
	if _, err := Extract([]List{a}, []PickRequest{{ListID: a.ID, Count: 4, Repeat: 2}}, testRand()); err == nil {
		t.Fatal("expected error: only 3 distinct lines")
	}
	for seed := uint64(0); seed < 50; seed++ {
		r := rand.New(rand.NewPCG(seed, seed))
		res, err := Extract([]List{a, b}, []PickRequest{{ListID: a.ID, Count: 3, Repeat: 3}, {ListID: b.ID, Count: 2, Repeat: 1}}, r)
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(res.Text, "\n")
		if len(lines) != 11 || res.Total != 11 {
			t.Fatalf("len = %d total = %d", len(lines), res.Total)
		}
		// 每个 a 内容恰好出现一次连续 3 行。
		seen := map[string]int{}
		for i := 0; i < len(lines); {
			j := i
			for j < len(lines) && lines[j] == lines[i] {
				j++
			}
			if strings.HasPrefix(lines[i], "a") {
				if j-i != 3 {
					t.Fatalf("run of %q has length %d: %q", lines[i], j-i, lines)
				}
			}
			seen[lines[i]]++
			i = j
		}
		for _, k := range []string{"a1", "a2", "a3"} {
			if seen[k] != 1 {
				t.Fatalf("%s runs = %d in %q", k, seen[k], lines)
			}
		}
	}
}

func TestExtractLimits(t *testing.T) {
	l := newList("A", "a", "b")
	if _, err := Extract([]List{l}, []PickRequest{{ListID: l.ID, Count: 2, Repeat: MaxOutputLines}}, testRand()); err == nil {
		t.Fatal("expected max output error")
	}
	if _, err := Extract([]List{l}, []PickRequest{{ListID: l.ID, Count: 0, Repeat: 1}}, testRand()); err == nil {
		t.Fatal("expected error for empty request")
	}
	if _, err := Extract([]List{l}, []PickRequest{{ListID: "missing", Count: 1, Repeat: 1}}, testRand()); err == nil {
		t.Fatal("expected error for missing list")
	}
}

func TestPlanImportModes(t *testing.T) {
	d := NewData()
	d.Lists = []List{newList("上衣", "a", "b")}
	id := d.Lists[0].ID

	lists, rep := planImport(d, ImportPlan{Dedupe: true, Items: []ImportItem{
		{Source: "1", Text: "b\nc\nc\n\nd", Mode: ImportAppend, TargetID: id},
		{Source: "2", Text: "x\ny", Mode: ImportNew, NewName: "裤子"},
	}}, 0)
	if !rep.OK || rep.Added != 4 || rep.Duplicates != 2 || rep.Created != 1 {
		t.Fatalf("report = %+v", rep)
	}
	if strings.Join(lists[0].Items, ",") != "a,b,c,d" {
		t.Fatalf("items = %q", lists[0].Items)
	}
	if strings.Join(d.Lists[0].Items, ",") != "a,b" {
		t.Fatal("preview must not modify original items")
	}

	_, rep = planImport(d, ImportPlan{Items: []ImportItem{{Text: "z", Mode: ImportNew, NewName: " 上衣 "}}}, 0)
	if rep.OK || rep.Items[0].Error == "" {
		t.Fatalf("expected duplicate name error: %+v", rep)
	}

	lists, rep = planImport(d, ImportPlan{Items: []ImportItem{{Text: "q\nq", Mode: ImportReplace, TargetID: id}}}, 0)
	if !rep.OK || rep.Removed != 2 || strings.Join(lists[0].Items, ",") != "q,q" {
		t.Fatalf("replace: %+v %q", rep, lists[0].Items)
	}

	_, rep = planImport(d, ImportPlan{Dedupe: true, Items: []ImportItem{{Text: "a\nb", Mode: ImportAppend, TargetID: id}}}, 0)
	if rep.OK || rep.Duplicates != 2 {
		t.Fatalf("all duplicates should not be OK: %+v", rep)
	}
}

func TestStorePersistUndoAndRecover(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	_, gid, err := s.CreateGroup("服装")
	if err != nil {
		t.Fatal(err)
	}
	snap, _, err := s.ApplyImport(ImportPlan{Dedupe: true, Items: []ImportItem{
		{Text: "a\nb\nb", Mode: ImportNew, NewName: "上衣", GroupID: gid},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Lists) != 1 || snap.Lists[0].Count != 2 || snap.Undo == "" {
		t.Fatalf("snapshot = %+v", snap)
	}
	lid := snap.Lists[0].ID

	if _, _, err := s.CreateGroup(" 服装 "); err == nil {
		t.Fatal("expected duplicate group name error")
	}

	snap, err = s.DeleteGroup(gid, false)
	if err != nil || len(snap.Groups) != 0 || snap.Lists[0].GroupID != "" {
		t.Fatalf("delete group: %v %+v", err, snap)
	}
	snap, err = s.Undo()
	if err != nil || len(snap.Groups) != 1 || snap.Lists[0].GroupID != gid {
		t.Fatalf("undo: %v %+v", err, snap)
	}

	if _, err := s.SetListText(lid, "x\n\ny\nx"); err != nil {
		t.Fatal(err)
	}
	snap, removed, err := s.DedupeList(lid)
	if err != nil || removed != 1 || snap.Lists[0].Count != 2 {
		t.Fatalf("dedupe: %v %d %+v", err, removed, snap)
	}

	// 重新打开后数据仍在。
	s2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if text, _ := s2.ListText(lid); text != "x\ny" {
		t.Fatalf("reopened text = %q", text)
	}

	// 文件损坏时从备份恢复。
	if err := os.WriteFile(filepath.Join(dir, libraryFile), []byte("{broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	s3, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if s3.Notice == "" || len(s3.Snapshot().Lists) != 1 {
		t.Fatalf("recover: notice=%q snap=%+v", s3.Notice, s3.Snapshot())
	}
}

func TestMoveList(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, g, _ := s.CreateGroup("G")
	_, a, _ := s.CreateList("A", "")
	_, b, _ := s.CreateList("B", "")
	_, c, _ := s.CreateList("C", g)
	snap, err := s.MoveList(a, g, c)
	if err != nil {
		t.Fatal(err)
	}
	names := ""
	for _, l := range snap.Lists {
		names += l.Name
	}
	if names != "BAC" || snap.Lists[1].GroupID != g {
		t.Fatalf("order = %s", names)
	}
	snap, _ = s.MoveList(b, g, "")
	if snap.Lists[2].ID != b {
		t.Fatalf("append to group end failed: %+v", snap.Lists)
	}
}

func TestRestoreRejectsForeignJSON(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RestoreData([]byte(`{"foo":1}`)); err == nil {
		t.Fatal("expected error")
	}
	b, _ := s.ExportData()
	if _, err := s.RestoreData(b); err != nil {
		t.Fatal(err)
	}
}

func TestNewLibraryIsNotPinnedByDefault(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if s.Snapshot().Settings.AlwaysOnTop {
		t.Fatal("a fresh library must not start always-on-top")
	}
}
