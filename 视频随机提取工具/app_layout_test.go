package main

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"videopicker/internal/picker"

	"github.com/bigmoonur/little-tool/shared/license"
)

func activeLayoutApp(t *testing.T) *App {
	t.Helper()
	return licensedFixture(t, &testLicense{status: license.Status{State: license.StateActive}})
}

func TestLayoutBindingsAndOfflineState(t *testing.T) {
	a := activeLayoutApp(t)
	created, err := a.CreateGroup("组")
	if err != nil || created.ID == "" || len(created.State.Groups) != 1 {
		t.Fatalf("create group: %+v, %v", created, err)
	}
	g := created.ID
	if st, err := a.RenameGroup(g, "改名"); err != nil || st.Groups[0].Name != "改名" {
		t.Fatalf("rename group: %+v, %v", st, err)
	}
	if st, err := a.SetGroupColor(g, "rose"); err != nil || st.Groups[0].Color != "rose" {
		t.Fatalf("color: %+v, %v", st, err)
	}
	if st, err := a.SetGroupCollapsed(g, true); err != nil || !st.Groups[0].Collapsed {
		t.Fatalf("collapse: %+v, %v", st, err)
	}
	if st, err := a.MoveCategory("开场", g, ""); err != nil || st.Categories[0].GroupID != g {
		t.Fatalf("move category: %+v, %v", st, err)
	}
	if st, err := a.CreateCategoryIn("新分类", g); err != nil || len(st.Categories) != 2 || st.Categories[1].GroupID != g {
		t.Fatalf("create in group: %+v, %v", st, err)
	}
	second, err := a.CreateGroup("第二组")
	if err != nil {
		t.Fatal(err)
	}
	if st, err := a.MoveGroup(second.ID, g); err != nil || st.Groups[0].ID != second.ID {
		t.Fatalf("move group: %+v, %v", st, err)
	}
	if st, err := a.RenameCategory("新分类", "改名分类"); err != nil || st.Categories[1].Name != "改名分类" || st.Categories[1].GroupID != g {
		t.Fatalf("rename category: %+v, %v", st, err)
	}
	before := a.State()
	root := a.store.Settings().MaterialRoot
	a.store.Update(func(s *picker.Settings) { s.MaterialRoot = filepath.Join(root, "missing") })
	if st := a.State(); st.MaterialError == "" || len(st.Categories) != 0 || !reflect.DeepEqual(st.Groups, before.Groups) {
		t.Fatal("offline state lost groups")
	}
	reopened, err := picker.OpenStore(a.store.Dir())
	if err != nil {
		t.Fatal(err)
	}
	a.store = reopened
	a.store.Update(func(s *picker.Settings) { s.MaterialRoot = root })
	if st := a.State(); !reflect.DeepEqual(before.Categories, st.Categories) || !reflect.DeepEqual(before.Groups, st.Groups) {
		t.Fatal("offline scan lost order/membership")
	}
	if st, err := a.DeleteGroup(g); err != nil || len(st.Groups) != 1 || st.Categories[0].GroupID != "" || st.Categories[1].GroupID != "" {
		t.Fatalf("delete group: %+v, %v", st, err)
	}
	if _, err := os.Stat(filepath.Join(root, "改名分类")); err != nil {
		t.Fatal("delete group changed folders")
	}
}

func TestHiddenCategoriesExcludedFromStatePrepareAndPruning(t *testing.T) {
	a := activeLayoutApp(t)
	a.State()
	p, err := a.Prepare(map[string]picker.Pick{"开场": {Count: 1}}, "", false)
	if err != nil || p.Count != 1 {
		t.Fatalf("initial prepare: %+v, %v", p, err)
	}
	tag, err := a.store.CreateTag("标签")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.store.SetCategoryTags("开场", []string{tag}); err != nil {
		t.Fatal(err)
	}
	vs, err := a.Videos("开场")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.store.MarkNoThumb(vs[0].Key); err != nil {
		t.Fatal(err)
	}
	deleted, err := a.DeleteCategory("开场", false)
	if err != nil || !deleted.Kept || deleted.Removed != 0 || len(deleted.State.Categories) != 0 || a.store.Settings().DeleteWithVideos {
		t.Fatalf("hide: %+v, %v", deleted, err)
	}
	if a.pending != nil {
		t.Fatal("delete retained pending plan")
	}
	if _, err := a.Prepare(map[string]picker.Pick{"开场": {Count: 1}}, "", false); err == nil {
		t.Fatal("hidden-only picks prepared")
	}
	if n := a.pruneThumbnails(); n != 0 || a.store.ThumbStates()[vs[0].Key] != "none" {
		t.Fatal("hidden category affected pruning")
	}
	if _, err := a.CreateCategory("可见"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(a.store.Settings().MaterialRoot, "可见", "new.mp4"), []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err = a.Prepare(map[string]picker.Pick{"开场": {Count: 1}, "可见": {Count: 1}}, "", false)
	if err != nil || p.Count != 1 || p.Items[0].Category != "可见" {
		t.Fatalf("hidden picks leaked: %+v, %v", p, err)
	}
	if st, err := a.CreateCategory("开场"); err != nil || len(st.Categories) != 2 || !reflect.DeepEqual(a.store.CategoryTags("开场"), []string{tag}) {
		t.Fatalf("unhide: %+v, %v", st, err)
	}
	deleted, err = a.DeleteCategory("开场", true)
	if err != nil || deleted.Kept || deleted.Removed != 1 || !a.store.Settings().DeleteWithVideos || len(deleted.State.Categories) != 1 {
		t.Fatalf("delete videos: %+v, %v", deleted, err)
	}
}

func TestLayoutMutationsRefuseBusyAndUnlicensed(t *testing.T) {
	for _, reason := range []string{"busy", "unlicensed"} {
		t.Run(reason, func(t *testing.T) {
			if reason == "unlicensed" && licenseBypass {
				t.Skip("development-only bypass")
			}
			lic := &testLicense{status: license.Status{State: license.StateActive}}
			a := licensedFixture(t, lic)
			created, err := a.CreateGroup("组")
			if err != nil {
				t.Fatal(err)
			}
			before := a.State()
			settings := a.store.Settings()
			if reason == "busy" {
				a.busy = true
			} else {
				lic.status = license.Status{State: license.StateInactive, Message: "TEST_LICENSE_REJECTED"}
			}
			calls := map[string]func() error{
				"create group":           func() error { _, err := a.CreateGroup("新组"); return err },
				"rename group":           func() error { _, err := a.RenameGroup(created.ID, "改名"); return err },
				"color":                  func() error { _, err := a.SetGroupColor(created.ID, "rose"); return err },
				"delete group":           func() error { _, err := a.DeleteGroup(created.ID); return err },
				"move group":             func() error { _, err := a.MoveGroup(created.ID, ""); return err },
				"move category":          func() error { _, err := a.MoveCategory("开场", created.ID, ""); return err },
				"create category":        func() error { _, err := a.CreateCategory("新分类"); return err },
				"create category in":     func() error { _, err := a.CreateCategoryIn("新分类", created.ID); return err },
				"rename category":        func() error { _, err := a.RenameCategory("开场", "改名"); return err },
				"delete category keep":   func() error { _, err := a.DeleteCategory("开场", false); return err },
				"delete category videos": func() error { _, err := a.DeleteCategory("开场", true); return err },
			}
			if reason == "busy" {
				calls["collapse"] = func() error { _, err := a.SetGroupCollapsed(created.ID, true); return err }
			}
			for name, call := range calls {
				t.Run(name, func(t *testing.T) {
					want := "TEST_LICENSE_REJECTED"
					if reason == "busy" {
						want = "正在复制，请等待完成或取消后再操作"
					}
					if err := call(); err == nil || err.Error() != want {
						t.Fatalf("refusal: %v", err)
					}
					st := a.State()
					st.Busy = false
					if !reflect.DeepEqual(before, st) || !reflect.DeepEqual(settings, a.store.Settings()) {
						t.Fatal("refused mutation changed state/settings")
					}
				})
			}
			if reason == "unlicensed" {
				if st, err := a.SetGroupCollapsed(created.ID, true); err != nil || !st.Groups[0].Collapsed {
					t.Fatal("collapse required license")
				}
			}
		})
	}
}

func TestAfterCopyActionsAndImportIsolation(t *testing.T) {
	a := activeLayoutApp(t)
	for _, mode := range []string{"stay", "minimize", "quit", "invalid"} {
		for _, openAfter := range []bool{false, true} {
			for _, ok := range []bool{false, true} {
				settings := a.UpdateSettings(UISettings{AfterCopy: mode, OpenAfter: openAfter, Thumbnails: true})
				if mode == "invalid" && settings.AfterCopy != "stay" {
					t.Fatal("UI afterCopy not normalized")
				}
				calls := []string{}
				a.afterSuccessfulCopy(ok, "destination", func(path string) error {
					if path != "destination" {
						t.Fatal(path)
					}
					calls = append(calls, "open")
					return nil
				}, func() { calls = append(calls, "minimize") }, func() { calls = append(calls, "quit") })
				want := []string{}
				if ok {
					if openAfter {
						want = append(want, "open")
					}
					if mode == "minimize" || mode == "quit" {
						want = append(want, mode)
					}
				}
				if !reflect.DeepEqual(calls, want) {
					t.Fatalf("afterCopy %s/%v/%v: %v != %v", mode, openAfter, ok, calls, want)
				}
			}
		}
	}
	a.UpdateSettings(UISettings{AfterCopy: "quit", Thumbnails: true})
	path := filepath.Join(t.TempDir(), "add.mp4")
	if err := os.WriteFile(path, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	completed := false
	a.runImport(ctx, cancel, a.store.Settings().MaterialRoot, "开场", []string{path}, func(event string, value any) {
		if event == "import-done" {
			completed = value.(ImportDone).OK
		}
	})
	if !completed || a.quitAfter {
		t.Fatal("afterCopy quit affected import")
	}
}

func TestCategoryRenameInvalidatesPreparedPlan(t *testing.T) {
	a := activeLayoutApp(t)
	plan, err := a.Prepare(map[string]picker.Pick{"开场": {Count: 1}}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.RenameCategory("开场", "改名"); err != nil {
		t.Fatal(err)
	}
	if err := a.Start(plan.ID); err == nil || a.busy || a.pending != nil {
		t.Fatal("renamed category retained old plan")
	}
}
