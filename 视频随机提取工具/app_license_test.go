package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"videopicker/internal/picker"

	"github.com/bigmoonur/little-tool/shared/license"
)

type testLicense struct {
	status      license.Status
	deactivated bool
}

func (l *testLicense) Start(func(license.Status))              {}
func (l *testLicense) Wait() license.Status                    { return l.status }
func (l *testLicense) Status() license.Status                  { return l.status }
func (l *testLicense) Refresh() license.Status                 { return l.status }
func (l *testLicense) Activate(string) (license.Status, error) { return l.status, nil }
func (l *testLicense) Deactivate() (license.Status, error) {
	l.deactivated = true
	return l.status, nil
}

func licensedFixture(t *testing.T, lic licenseManager) *App {
	t.Helper()
	root := t.TempDir()
	material, output := filepath.Join(root, "素材"), filepath.Join(root, "目标")
	for _, dir := range []string{filepath.Join(material, "开场"), filepath.Join(output, "第一批")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(material, "开场", "sample.mp4"), []byte("source video"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := picker.OpenStore(filepath.Join(root, "设置"))
	if err != nil {
		t.Fatal(err)
	}
	store.Update(func(s *picker.Settings) {
		s.MaterialRoot, s.OutputRoot, s.Folder = material, output, "第一批"
	})
	store.AddHistory(picker.Result{Destination: filepath.Join(output, "第一批"), Folder: "第一批"})
	return NewApp(store, lic, nil)
}

func TestUnlicensedOperationsCannotChangeUserData(t *testing.T) {
	if licenseBypass {
		t.Skip("development-only bypass")
	}
	for _, missing := range []bool{false, true} {
		name := "rejected"
		var lic licenseManager = &testLicense{status: license.Status{State: license.StateInactive, Message: "TEST_LICENSE_REJECTED"}}
		if missing {
			name, lic = "missing", nil
		}
		t.Run(name, func(t *testing.T) {
			a := licensedFixture(t, lic)
			tagID, err := a.store.CreateTag("已有标签")
			if err != nil {
				t.Fatal(err)
			}
			if err := a.store.SetCategoryTags("开场", []string{tagID}); err != nil {
				t.Fatal(err)
			}
			beforeSettings, beforeHistory := a.store.Settings(), a.store.History()
			beforeTags, beforeCategoryTags := a.store.Tags(), a.store.CategoryTags("开场")
			plan, err := picker.Prepare(beforeSettings.MaterialRoot, beforeSettings.OutputRoot, beforeSettings.Folder, map[string]int{"开场": 1}, false, a.rng)
			if err != nil {
				t.Fatal(err)
			}
			a.pending, a.pendingID = &plan, "prepared"
			calls := map[string]func() error{
				"choose material":      func() error { _, err := a.ChooseMaterialRoot(); return err },
				"choose output":        func() error { _, err := a.ChooseOutputRoot(); return err },
				"select folder":        func() error { _, err := a.SetFolder("另一批"); return err },
				"create folder":        func() error { _, err := a.CreateFolder("另一批"); return err },
				"prepare":              func() error { _, err := a.Prepare(map[string]int{"开场": 1}, true); return err },
				"start prepared batch": func() error { return a.Start("prepared") },
				"clear history":        a.ClearHistory,
				"create tag":           func() error { _, err := a.CreateTag("新标签"); return err },
				"rename tag":           func() error { _, err := a.RenameTag(tagID, "新名称"); return err },
				"set tag color":        func() error { _, err := a.SetTagColor(tagID, "rose"); return err },
				"delete tag":           func() error { _, err := a.DeleteTag(tagID); return err },
				"set category tags":    func() error { _, err := a.SetCategoryTags("开场", nil); return err },
			}
			for name, call := range calls {
				t.Run(name, func(t *testing.T) {
					err := call()
					if err == nil || (!strings.Contains(err.Error(), "尚未激活") && err.Error() != "TEST_LICENSE_REJECTED") {
						t.Fatalf("expected licensing rejection, got %v", err)
					}
					if !reflect.DeepEqual(beforeSettings, a.store.Settings()) || !reflect.DeepEqual(beforeHistory, a.store.History()) {
						t.Fatal("unlicensed call changed settings or history")
					}
					if !reflect.DeepEqual(beforeTags, a.store.Tags()) || !reflect.DeepEqual(beforeCategoryTags, a.store.CategoryTags("开场")) {
						t.Fatal("unlicensed call changed tags or category assignments")
					}
					if a.pending != &plan || a.busy {
						t.Fatal("unlicensed call consumed or started the prepared batch")
					}
					entries, err := os.ReadDir(beforeSettings.OutputRoot)
					if err != nil || len(entries) != 1 {
						t.Fatalf("output folders changed: %v %v", entries, err)
					}
					files, err := os.ReadDir(filepath.Join(beforeSettings.OutputRoot, "第一批"))
					if err != nil || len(files) != 0 {
						t.Fatalf("destination changed: %v %v", files, err)
					}
				})
			}
			// 失效后用户仍可查看设置、记录和磁盘状态。
			if _, err := a.Init(); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(beforeHistory, a.History()) || a.State().Folder != "第一批" {
				t.Fatal("user data became inaccessible")
			}
		})
	}
}

func TestLicenseExpiryBetweenPrepareAndStart(t *testing.T) {
	if licenseBypass {
		t.Skip("development-only bypass")
	}
	lic := &testLicense{status: license.Status{State: license.StateActive}}
	a := licensedFixture(t, lic)
	p, err := a.Prepare(map[string]int{"开场": 1}, false)
	if err != nil || p.Count != 1 {
		t.Fatalf("active license could not prepare a batch: %+v %v", p, err)
	}
	lic.status = license.Status{State: license.StateInactive, Message: "授权已到期"}
	if err := a.Start(p.ID); err == nil || err.Error() != "授权已到期" {
		t.Fatalf("expired license started a batch: %v", err)
	}
	if a.busy || a.pending == nil {
		t.Fatal("rejected start changed the pending batch")
	}
}

func TestCancelStillWorksWithoutLicenseAndDeactivateWaitsForCopy(t *testing.T) {
	lic := &testLicense{status: license.Status{State: license.StateInactive}}
	a := licensedFixture(t, lic)
	cancelled := false
	a.busy, a.cancel = true, func() { cancelled = true }
	a.Cancel()
	if !cancelled {
		t.Fatal("licensing prevented cancellation")
	}
	if !licenseBypass {
		if _, err := a.DeactivateLicense(); err == nil || lic.deactivated {
			t.Fatal("deactivation proceeded during a copy")
		}
	}
}
