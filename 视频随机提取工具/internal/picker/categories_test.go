package picker

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRenameCategoryCarriesAllNameKeys(t *testing.T) {
	s := layoutStore(t)
	root := t.TempDir()
	mustLayout(t, CreateCategory(root, "Old"))
	mustLayout(t, CreateCategory(root, "Other"))
	mustLayout(t, os.WriteFile(filepath.Join(root, "Old", "sample.mp4"), []byte("video"), 0o644))
	s.ArrangeCategories([]Category{{Name: "Old"}, {Name: "Other"}})
	g, err := s.CreateGroup("组")
	mustLayout(t, err)
	mustLayout(t, s.MoveCategory("Old", g, ""))
	tag, err := s.CreateTag("标签")
	mustLayout(t, err)
	mustLayout(t, s.SetCategoryTags("Old", []string{tag}))
	pick := Pick{Count: 3, Repeat: 4}
	_, err = s.SavePreset("方案", map[string]Pick{"Old": pick, "Other": {Count: 1}}, ModeCycle)
	mustLayout(t, err)
	s.Update(func(st *Settings) {
		st.RememberDraft = true
		st.Draft, st.DraftRepeat = map[string]int{"Old": 3, "Other": 1}, map[string]int{"Old": 4}
	})
	s.AddHistory(Result{Items: []Item{{Category: "Old", Name: "sample.mp4"}}, Picks: map[string]Pick{"Old": pick}})
	history, layout := s.History(), s.layout.clone()
	for _, newName := range []string{"New", "new"} {
		oldName := "Old"
		if newName == "new" {
			oldName = "New"
		}
		mustLayout(t, s.RenameCategory(root, oldName, newName))
		if b, err := os.ReadFile(filepath.Join(root, newName, "sample.mp4")); err != nil || string(b) != "video" {
			t.Fatalf("renamed folder: %q, %v", b, err)
		}
		if !reflect.DeepEqual(s.CategoryTags(newName), []string{tag}) || len(s.CategoryTags(oldName)) != 0 || s.Presets()[0].Picks[newName] != pick || s.Settings().Draft[newName] != 3 || s.Settings().DraftRepeat[newName] != 4 || s.layout.Categories[newName].GroupID != g || !reflect.DeepEqual(s.History(), history) {
			t.Fatalf("keys not carried: %+v, %+v", s.Settings(), s.layout)
		}
		if _, ok := s.Presets()[0].Picks[oldName]; ok {
			t.Fatal("old preset key survived")
		}
		if _, ok := s.Settings().Draft[oldName]; ok {
			t.Fatal("old draft key survived")
		}
		if s.layout.Order[layoutNameIndex(layout.Order, "Old")] != newName {
			t.Fatal("layout position changed")
		}
	}
	reopened, err := OpenStore(s.Dir())
	mustLayout(t, err)
	if !reflect.DeepEqual(s.layout, reopened.layout) || !reflect.DeepEqual(s.Presets(), reopened.Presets()) || !reflect.DeepEqual(s.Settings(), reopened.Settings()) || !reflect.DeepEqual(s.CategoryTags("new"), reopened.CategoryTags("new")) {
		t.Fatal("rename associations not persisted")
	}
	mustLayout(t, os.WriteFile(filepath.Join(root, "Taken"), []byte("non-folder"), 0o644))
	for _, name := range []string{"other", "TAKEN", "../bad", "NUL", "bad.", ""} {
		if err := s.RenameCategory(root, "new", name); err == nil {
			t.Fatalf("accepted rename %q", name)
		}
	}
	if err := s.RenameCategory(root, "missing", "valid"); err == nil {
		t.Fatal("missing folder renamed")
	}
	mustLayout(t, s.RenameCategory(root, "new", "new"))
}

func layoutNameIndex(names []string, name string) int {
	for i, n := range names {
		if n == name {
			return i
		}
	}
	return -1
}

func TestRenameCategoryRollsBackOnSaveFailure(t *testing.T) {
	s := layoutStore(t)
	root := t.TempDir()
	mustLayout(t, s.CreateCategoryIn(root, "Old", ""))
	before := s.layout.clone()
	mustLayout(t, os.Mkdir(filepath.Join(s.Dir(), "tags.json.tmp"), 0o755))
	if err := s.RenameCategory(root, "Old", "New"); err == nil {
		t.Fatal("save failure accepted")
	}
	if _, err := CategoryDir(root, "Old"); err != nil || !reflect.DeepEqual(before, s.layout) {
		t.Fatal("rename was not rolled back")
	}
	reopened, err := OpenStore(s.Dir())
	mustLayout(t, err)
	if !reflect.DeepEqual(before, reopened.layout) {
		t.Fatal("persisted layout was not rolled back")
	}
}

func TestDeleteCategoryModesAndRestore(t *testing.T) {
	for _, withVideos := range []bool{false, true} {
		for _, content := range []string{"empty", "videos", "other", "nested", "mixed"} {
			t.Run(content+map[bool]string{false: "-keep", true: "-delete"}[withVideos], func(t *testing.T) {
				s := layoutStore(t)
				root := t.TempDir()
				g, err := s.CreateGroup("组")
				mustLayout(t, err)
				mustLayout(t, s.CreateCategoryIn(root, "Category", g))
				dir := filepath.Join(root, "Category")
				hasVideos := content == "videos" || content == "mixed"
				if hasVideos {
					for _, name := range []string{"a.mp4", "B.MOV"} {
						mustLayout(t, os.WriteFile(filepath.Join(dir, name), []byte("video"), 0o644))
					}
				}
				if content == "other" || content == "mixed" {
					mustLayout(t, os.WriteFile(filepath.Join(dir, "note.txt"), []byte("keep"), 0o644))
				}
				if content == "nested" || content == "mixed" {
					// 名称像视频的子文件夹也不属于直接的视频文件。
					mustLayout(t, os.Mkdir(filepath.Join(dir, "child.mp4"), 0o755))
					mustLayout(t, os.WriteFile(filepath.Join(dir, "child.mp4", "nested.mp4"), []byte("keep"), 0o644))
				}
				tag, err := s.CreateTag("标签")
				mustLayout(t, err)
				mustLayout(t, s.SetCategoryTags("Category", []string{tag}))
				s.Update(func(st *Settings) { st.DeleteWithVideos = !withVideos })
				result, err := s.DeleteCategory(root, "Category", withVideos)
				mustLayout(t, err)
				kept := content != "empty" && !(withVideos && content == "videos")
				removed := 0
				if withVideos && hasVideos {
					removed = 2
				}
				if result.Kept != kept || result.Removed != removed || s.Settings().DeleteWithVideos != withVideos {
					t.Fatalf("result: %+v", result)
				}
				cats, err := ScanCategories(root)
				mustLayout(t, err)
				if len(s.ArrangeCategories(cats)) != 0 || len(s.VisiblePicks(map[string]Pick{"Category": {Count: 1}})) != map[bool]int{true: 0, false: 1}[kept] {
					t.Fatal("hidden category visible")
				}
				reopened, err := OpenStore(s.Dir())
				mustLayout(t, err)
				if reopened.Settings().DeleteWithVideos != withVideos || !reflect.DeepEqual(reopened.layout, s.layout) {
					t.Fatal("deletion not persisted")
				}
				if kept {
					if !reflect.DeepEqual(reopened.CategoryTags("Category"), []string{tag}) {
						t.Fatal("hidden category tags lost")
					}
					if content == "other" || content == "mixed" {
						if b, err := os.ReadFile(filepath.Join(dir, "note.txt")); err != nil || string(b) != "keep" {
							t.Fatal("other file deleted")
						}
					}
					if content == "nested" || content == "mixed" {
						if _, err := os.Stat(filepath.Join(dir, "child.mp4", "nested.mp4")); err != nil {
							t.Fatal("nested video deleted")
						}
					}
					mustLayout(t, reopened.CreateCategoryIn(root, "category", ""))
					cats, err := ScanCategories(root)
					mustLayout(t, err)
					shown := reopened.ArrangeCategories(cats)
					if len(shown) != 1 || shown[0].Name != "Category" || shown[0].GroupID != "" || !reflect.DeepEqual(shown[0].TagIDs, []string{tag}) {
						t.Fatalf("restore: %+v", shown)
					}
				} else {
					if _, err := os.Stat(dir); !os.IsNotExist(err) || len(reopened.CategoryTags("Category")) != 0 || len(reopened.layout.Categories) != 0 || len(reopened.layout.Order) != 0 {
						t.Fatal("removed folder retained tags/layout")
					}
					mustLayout(t, reopened.CreateCategoryIn(root, "Category", g))
				}
			})
		}
	}
}

func TestDeleteCategoryPartialFailureStaysVisible(t *testing.T) {
	s := layoutStore(t)
	root := t.TempDir()
	mustLayout(t, s.CreateCategoryIn(root, "分类", ""))
	for _, name := range []string{"a.mp4", "b.mp4", "c.mp4"} {
		mustLayout(t, os.WriteFile(filepath.Join(root, "分类", name), []byte("video"), 0o644))
	}
	// 注入删除失败，避免依赖权限或删除打开文件的系统差异。
	result, err := s.deleteCategory(root, "分类", true, func(path string) error {
		if filepath.Base(path) == "b.mp4" {
			return errors.New("test failure")
		}
		return os.Remove(path)
	})
	var userError *UserError
	if !errors.As(err, &userError) || !strings.Contains(err.Error(), "b.mp4") || result.Removed != 1 || !s.Settings().DeleteWithVideos {
		t.Fatalf("partial delete: %+v, %v", result, err)
	}
	cats, err := ScanCategories(root)
	mustLayout(t, err)
	shown := s.ArrangeCategories(cats)
	if len(shown) != 1 || shown[0].Count != 2 {
		t.Fatal("partial deletion hid category")
	}
	if _, err := os.Stat(filepath.Join(root, "分类", "a.mp4")); !os.IsNotExist(err) {
		t.Fatal("already deleted video restored")
	}
}

func TestCreateCategoryInValidationAndWriteFailure(t *testing.T) {
	s := layoutStore(t)
	root := t.TempDir()
	mustLayout(t, s.CreateCategoryIn(root, "Taken", ""))
	mustLayout(t, os.WriteFile(filepath.Join(root, "File"), []byte("keep"), 0o644))
	for _, tc := range [][2]string{{"taken", ""}, {"file", ""}, {"Bad?", ""}, {"New", "missing"}} {
		if err := s.CreateCategoryIn(root, tc[0], tc[1]); err == nil {
			t.Fatal(tc)
		}
	}
	if err := s.CreateCategoryIn(filepath.Join(root, "missing"), "New", ""); err == nil {
		t.Fatal("missing root accepted")
	}
	mustLayout(t, os.Mkdir(filepath.Join(s.Dir(), "layout.json.tmp"), 0o755))
	if err := s.CreateCategoryIn(root, "New", ""); err == nil {
		t.Fatal("failed layout write accepted")
	}
	if _, err := os.Stat(filepath.Join(root, "New")); !os.IsNotExist(err) {
		t.Fatal("failed creation left folder")
	}
}
