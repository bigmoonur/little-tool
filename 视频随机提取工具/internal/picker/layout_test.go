package picker

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func layoutStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func categoryNames(cats []Category) []string {
	out := []string{}
	for _, c := range cats {
		out = append(out, c.Name)
	}
	return out
}

func mustLayout(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestLayoutOrderingPersistenceAndMissingCategories(t *testing.T) {
	s := layoutStore(t)
	cats := []Category{{Name: "z"}, {Name: "b"}, {Name: "a"}, {Name: "c"}}
	if got := categoryNames(s.ArrangeCategories(cats)); !reflect.DeepEqual(got, []string{"a", "b", "c", "z"}) {
		t.Fatal(got)
	}
	g1, err := s.CreateGroup("一组")
	mustLayout(t, err)
	g2, err := s.CreateGroup("二组")
	mustLayout(t, err)
	mustLayout(t, s.MoveCategory("c", g1, ""))
	mustLayout(t, s.MoveCategory("b", g1, "c"))
	mustLayout(t, s.MoveCategory("a", g2, ""))
	mustLayout(t, s.MoveGroup(g2, g1))
	mustLayout(t, s.MoveCategory("z", "", ""))
	want := []string{"a", "b", "c", "z"}
	if got := categoryNames(s.ArrangeCategories(cats)); !reflect.DeepEqual(got, want) {
		t.Fatal(got)
	}
	before := s.layout.clone()
	if got := s.ArrangeCategories(nil); len(got) != 0 || !reflect.DeepEqual(before, s.layout) {
		t.Fatal("missing disk changed layout")
	}
	if got := categoryNames(s.ArrangeCategories([]Category{{Name: "c"}})); !reflect.DeepEqual(got, []string{"c"}) || !reflect.DeepEqual(before, s.layout) {
		t.Fatal("missing categories lost layout")
	}
	reopened, err := OpenStore(s.Dir())
	mustLayout(t, err)
	if !reflect.DeepEqual(before, reopened.layout) {
		t.Fatal("layout not persisted")
	}
	cats = append(cats, Category{Name: "y"}, Category{Name: "d"})
	want = append(want, "d", "y")
	if got := categoryNames(reopened.ArrangeCategories(cats)); !reflect.DeepEqual(got, want) {
		t.Fatalf("new categories: %v", got)
	}
	mustLayout(t, reopened.MoveCategory("z", g1, "b"))
	mustLayout(t, reopened.MoveCategory("b", g2, "a"))
	mustLayout(t, reopened.MoveCategory("c", g1, "unknown"))
	if got := categoryNames(reopened.ArrangeCategories(cats)); !reflect.DeepEqual(got, []string{"b", "a", "z", "c", "d", "y"}) {
		t.Fatalf("cross-group moves: %v", got)
	}
	mustLayout(t, reopened.MoveGroup(g2, ""))
	if got := categoryNames(reopened.ArrangeCategories(cats)); !reflect.DeepEqual(got, []string{"z", "c", "b", "a", "d", "y"}) {
		t.Fatalf("group to end: %v", got)
	}
}

func TestGroupCRUDValidationAndCopies(t *testing.T) {
	s := layoutStore(t)
	g1, err := s.CreateGroup("  A   B  ")
	mustLayout(t, err)
	g2, err := s.CreateGroup("第二组")
	mustLayout(t, err)
	if s.Groups()[0].Name != "A B" || s.Groups()[0].Color != "indigo" || s.Groups()[1].Color != "teal" {
		t.Fatal(s.Groups())
	}
	for _, name := range []string{"", "a b", strings.Repeat("字", 41)} {
		if _, err := s.CreateGroup(name); err == nil {
			t.Fatalf("accepted %q", name)
		}
	}
	if err := s.RenameGroup(g2, "a b"); err == nil {
		t.Fatal("duplicate rename")
	}
	if err := s.SetGroupColor(g1, "invalid"); err == nil {
		t.Fatal("invalid color")
	}
	mustLayout(t, s.RenameGroup(g1, "改名"))
	for _, color := range TagColors {
		mustLayout(t, s.SetGroupColor(g1, color))
	}
	mustLayout(t, s.SetGroupCollapsed(g1, true))
	mustLayout(t, s.MoveGroup(g1, g1))
	mustLayout(t, s.MoveGroup(g1, "missing"))
	groups := s.Groups()
	groups[1].Name = "外部改动"
	if s.Groups()[1].Name != "改名" {
		t.Fatal("returned groups alias store")
	}
	s.ArrangeCategories([]Category{{Name: "分类"}})
	mustLayout(t, s.MoveCategory("分类", g1, ""))
	mustLayout(t, s.DeleteGroup(g1))
	if s.ArrangeCategories([]Category{{Name: "分类"}})[0].GroupID != "" {
		t.Fatal("deleted group retained membership")
	}
	for _, call := range []func() error{
		func() error { return s.RenameGroup("missing", "名称") },
		func() error { return s.SetGroupColor("missing", "rose") },
		func() error { return s.SetGroupCollapsed("missing", false) },
		func() error { return s.MoveGroup("missing", "") },
		func() error { return s.DeleteGroup("missing") },
		func() error { return s.MoveCategory("missing", "", "") },
		func() error { return s.MoveCategory("分类", "missing", "") },
	} {
		before := s.layout.clone()
		if err := call(); err == nil || !reflect.DeepEqual(before, s.layout) {
			t.Fatalf("invalid mutation: %v", err)
		}
	}
	reopened, err := OpenStore(s.Dir())
	mustLayout(t, err)
	if !reflect.DeepEqual(s.Groups(), reopened.Groups()) {
		t.Fatal("group changes not persisted")
	}
}

func TestLayoutNormalizesWithoutDroppingMissingNames(t *testing.T) {
	dir := t.TempDir()
	mustLayout(t, writeJSON(filepath.Join(dir, "layout.json"), layoutData{
		Groups:     []Group{{ID: "g", Color: "bad", Collapsed: true}, {ID: "g"}, {ID: ""}},
		Order:      []string{"offline", "offline", "unknown"},
		Categories: map[string]categoryLayout{"offline": {GroupID: "g", Hidden: true}, "missing": {GroupID: "gone"}},
	}))
	s, err := OpenStore(dir)
	mustLayout(t, err)
	if len(s.Groups()) != 1 || s.Groups()[0].Color != "indigo" || !s.Groups()[0].Collapsed || !s.layout.Categories["offline"].Hidden || s.layout.Categories["missing"].GroupID != "" || !reflect.DeepEqual(s.layout.Order, []string{"offline", "missing"}) {
		t.Fatalf("normalization: %+v", s.layout)
	}
}

func TestLayoutWriteFailurePreservesMemory(t *testing.T) {
	s := layoutStore(t)
	mustLayout(t, os.Mkdir(filepath.Join(s.Dir(), "layout.json.tmp"), 0o755))
	before := s.layout.clone()
	if id, err := s.CreateGroup("失败"); err == nil || id != "" || !reflect.DeepEqual(before, s.layout) {
		t.Fatalf("failed write: %q, %v", id, err)
	}
}

func TestAfterCopyNormalizationAndPersistence(t *testing.T) {
	for _, value := range []string{"", "stay", "minimize", "quit", "QUIT", "other"} {
		t.Run(value, func(t *testing.T) {
			s := layoutStore(t)
			want := value
			if value != "minimize" && value != "quit" {
				want = "stay"
			}
			if got := s.Update(func(st *Settings) { st.AfterCopy = value }); got.AfterCopy != want {
				t.Fatal(got.AfterCopy)
			}
			reopened, err := OpenStore(s.Dir())
			mustLayout(t, err)
			if reopened.Settings().AfterCopy != want {
				t.Fatal("afterCopy not persisted")
			}
			mustLayout(t, writeJSON(filepath.Join(s.Dir(), "settings.json"), Settings{AfterCopy: value}))
			reopened, err = OpenStore(s.Dir())
			mustLayout(t, err)
			if reopened.Settings().AfterCopy != want {
				t.Fatal("afterCopy not normalized on load")
			}
		})
	}
}

func TestHiddenScanKeepsDiskInventoryAvailable(t *testing.T) {
	s := layoutStore(t)
	root := t.TempDir()
	mustLayout(t, s.CreateCategoryIn(root, "隐藏", ""))
	mustLayout(t, s.CreateCategoryIn(root, "可见", ""))
	mustLayout(t, os.WriteFile(filepath.Join(root, "隐藏", "sample.mp4"), []byte("keep"), 0o644))
	_, err := s.DeleteCategory(root, "隐藏", false)
	mustLayout(t, err)
	raw, err := ScanCategories(root)
	mustLayout(t, err)
	visible, err := s.ScanCategories(root)
	mustLayout(t, err)
	if len(raw) != 2 || !reflect.DeepEqual(categoryNames(visible), []string{"可见"}) {
		t.Fatalf("raw/visible scans: %+v / %+v", raw, visible)
	}
	keys, err := LibraryKeys(root)
	mustLayout(t, err)
	if len(keys) != 1 {
		t.Fatal("hidden videos disappeared from thumbnail inventory")
	}
}
