package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"videopicker/internal/picker"

	"github.com/bigmoonur/little-tool/shared/license"
)

func TestStateTagLists(t *testing.T) {
	assertLists := func(t *testing.T, st State) {
		t.Helper()
		encoded, err := json.Marshal(st)
		if err != nil || st.Tags == nil || !bytes.Contains(encoded, []byte(`"tags":[]`)) {
			t.Fatalf("empty tags JSON = %s, %v", encoded, err)
		}
		for _, cat := range st.Categories {
			if cat.TagIDs == nil || len(cat.TagIDs) != 0 {
				t.Fatalf("untagged category = %+v", cat)
			}
		}
	}
	t.Run("nil store", func(t *testing.T) {
		assertLists(t, NewApp(nil, nil, nil).State())
	})
	for _, name := range []string{"scanned", "no root", "unavailable root"} {
		t.Run(name, func(t *testing.T) {
			a := licensedFixture(t, nil)
			switch name {
			case "no root":
				a.store.Update(func(s *picker.Settings) { s.MaterialRoot = "" })
			case "unavailable root":
				a.store.Update(func(s *picker.Settings) { s.MaterialRoot = filepath.Join(t.TempDir(), "missing") })
			}
			assertLists(t, a.State())
		})
	}
}

func TestTagMethodsDuringCopy(t *testing.T) {
	a := licensedFixture(t, &testLicense{status: license.Status{State: license.StateActive}})
	a.busy = true
	a.pendingID = "prepared"
	created, err := a.CreateTag(" 标 签 ")
	if err != nil || created.ID == "" || len(created.State.Tags) != 1 || created.State.Tags[0].ID != created.ID || !created.State.Busy {
		t.Fatalf("CreateTag while busy = %+v, %v", created, err)
	}
	id := created.ID
	st, err := a.SetCategoryTags("开场", []string{id, id})
	if err != nil || len(st.Categories) != 1 || !reflect.DeepEqual(st.Categories[0].TagIDs, []string{id}) {
		t.Fatalf("SetCategoryTags while busy = %+v, %v", st, err)
	}
	st, err = a.RenameTag(id, "新名称")
	if err != nil || st.Tags[0].Name != "新名称" {
		t.Fatalf("RenameTag while busy = %+v, %v", st, err)
	}
	st, err = a.SetTagColor(id, "rose")
	if err != nil || st.Tags[0].Color != "rose" {
		t.Fatalf("SetTagColor while busy = %+v, %v", st, err)
	}
	before := a.State()
	rejected, err := a.CreateTag("")
	if err == nil || rejected.ID != "" || !reflect.DeepEqual(rejected.State, before) {
		t.Fatalf("failed CreateTag did not return current state: %+v, %v", rejected, err)
	}
	for name, call := range map[string]func() (State, error){
		"rename": func() (State, error) { return a.RenameTag(id, "") },
		"color":  func() (State, error) { return a.SetTagColor(id, "blue") },
		"delete": func() (State, error) { return a.DeleteTag("missing") },
		"assign": func() (State, error) { return a.SetCategoryTags("开场", []string{"missing"}) },
	} {
		t.Run(name, func(t *testing.T) {
			st, err := call()
			if err == nil || !reflect.DeepEqual(st, before) {
				t.Fatalf("failed %s did not return current state: %+v, %v", name, st, err)
			}
		})
	}
	settings := a.store.Settings()
	categoryDir := filepath.Join(settings.MaterialRoot, "开场")
	if err := os.RemoveAll(categoryDir); err != nil {
		t.Fatal(err)
	}
	if st := a.State(); len(st.Categories) != 0 || len(st.Tags) != 1 {
		t.Fatalf("offline category state = %+v", st)
	}
	if err := os.Mkdir(categoryDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if st := a.State(); len(st.Categories) != 1 || !reflect.DeepEqual(st.Categories[0].TagIDs, []string{id}) {
		t.Fatalf("reconnected category lost tags: %+v", st)
	}
	st, err = a.DeleteTag(id)
	if err != nil || st.Tags == nil || len(st.Tags) != 0 || st.Categories[0].TagIDs == nil || len(st.Categories[0].TagIDs) != 0 {
		t.Fatalf("DeleteTag while busy = %+v, %v", st, err)
	}
	if !a.busy || a.pendingID != "prepared" {
		t.Fatal("tag operations changed copy state")
	}
}
