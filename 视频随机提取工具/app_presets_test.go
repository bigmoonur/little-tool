package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"videopicker/internal/picker"

	"github.com/bigmoonur/little-tool/shared/license"
)

func TestStatePresetLists(t *testing.T) {
	apps := map[string]*App{"nil store": NewApp(nil, nil, nil), "store": licensedFixture(t, nil)}
	for name, a := range apps {
		t.Run(name, func(t *testing.T) {
			st := a.State()
			encoded, err := json.Marshal(st)
			if err != nil || st.Presets == nil || !bytes.Contains(encoded, []byte(`"presets":[]`)) {
				t.Fatalf("presets JSON = %s %v", encoded, err)
			}
		})
	}
	info, err := NewApp(nil, nil, errors.New("unavailable")).Init()
	if err == nil || info.Settings.Draft == nil || info.Settings.DraftRepeat == nil || info.Settings.Mode != picker.ModeRepeat {
		t.Fatalf("failed Init settings = %+v %v", info.Settings, err)
	}
}

func TestPresetMethodsDuringCopy(t *testing.T) {
	a := licensedFixture(t, &testLicense{status: license.Status{State: license.StateActive}})
	a.busy, a.pendingID = true, "prepared"
	picks := map[string]picker.Pick{"离线分类": {Count: 2, Repeat: 3}}
	saved, err := a.SavePreset(" 早间\t方案 ", picks, picker.ModeRandom)
	if err != nil || saved.ID == "" || !saved.State.Busy || len(saved.State.Presets) != 1 || saved.State.Presets[0].ID != saved.ID || saved.State.Presets[0].Name != "早间 方案" || !reflect.DeepEqual(saved.State.Presets[0].Picks, picks) {
		t.Fatalf("SavePreset during copy = %+v %v", saved, err)
	}
	id := saved.ID
	saved.State.Presets[0].Picks["离线分类"] = picker.Pick{Count: 99}
	if a.State().Presets[0].Picks["离线分类"].Count != 2 {
		t.Fatal("State presets alias stored picks")
	}
	st, err := a.RenamePreset(id, "新名称")
	if err != nil || !st.Busy || st.Presets[0].Name != "新名称" {
		t.Fatalf("RenamePreset during copy = %+v %v", st, err)
	}
	before := a.State()
	rejected, err := a.SavePreset("", picks, "")
	if err == nil || rejected.ID != "" || !reflect.DeepEqual(rejected.State, before) {
		t.Fatalf("failed save did not return current state: %+v %v", rejected, err)
	}
	for name, call := range map[string]func() (State, error){
		"rename": func() (State, error) { return a.RenamePreset(id, "") },
		"delete": func() (State, error) { return a.DeletePreset("missing") },
	} {
		t.Run(name, func(t *testing.T) {
			st, err := call()
			if err == nil || !reflect.DeepEqual(st, before) {
				t.Fatalf("failed %s did not return current state: %+v %v", name, st, err)
			}
		})
	}
	st, err = a.DeletePreset(id)
	if err != nil || !st.Busy || st.Presets == nil || len(st.Presets) != 0 || !a.busy || a.pendingID != "prepared" {
		t.Fatalf("DeletePreset during copy = %+v %v", st, err)
	}
}

func TestSaveDraftCountsRepeatsAndMode(t *testing.T) {
	a := licensedFixture(t, nil)
	counts, repeats := map[string]int{"开场": 1}, map[string]int{"开场": 4}
	a.SaveDraft(counts, repeats, picker.ModeRandom)
	assertEmpty := func(t *testing.T) {
		t.Helper()
		st := a.store.Settings()
		if st.Draft == nil || st.DraftRepeat == nil || len(st.Draft) != 0 || len(st.DraftRepeat) != 0 || st.Mode != picker.ModeRepeat {
			t.Fatalf("disabled drafts = %+v", st)
		}
	}
	assertEmpty(t)
	a.UpdateSettings(UISettings{RememberDraft: true})
	a.SaveDraft(counts, repeats, picker.ModeRandom)
	counts["开场"], repeats["开场"] = 99, 99
	info, err := a.Init()
	if err != nil || info.Settings.Draft["开场"] != 1 || info.Settings.DraftRepeat["开场"] != 4 || info.Settings.Mode != picker.ModeRandom {
		t.Fatalf("draft settings = %+v %v", info.Settings, err)
	}
	counts["开场"], repeats["开场"] = 2, 5
	for _, mode := range []string{"", "unknown"} {
		a.SaveDraft(counts, repeats, mode)
		st := a.store.Settings()
		if st.Mode != picker.ModeRepeat || st.Draft["开场"] != 2 || st.DraftRepeat["开场"] != 5 {
			t.Fatalf("draft mode %q = %+v", mode, st)
		}
	}
	a.UpdateSettings(UISettings{RememberDraft: false})
	assertEmpty(t)
	a.SaveDraft(counts, repeats, picker.ModeRandom)
	assertEmpty(t)
	reopened, err := picker.OpenStore(a.store.Dir())
	if err != nil || !reflect.DeepEqual(a.store.Settings(), reopened.Settings()) {
		t.Fatalf("disabled drafts were not persisted: %v", err)
	}
}

func TestAppPrepareMultiplierParameters(t *testing.T) {
	a := licensedFixture(t, &testLicense{status: license.Status{State: license.StateActive}})
	picks := map[string]picker.Pick{"开场": {Count: 1, Repeat: 3}}
	summary, err := a.Prepare(picks, picker.ModeRandom, false)
	if err != nil || summary.Count != 3 || summary.ID == "" || summary.Bytes != int64(len("source video")*3) || len(summary.Items) != 3 || a.pending == nil || a.pending.Mode != picker.ModeRandom || !reflect.DeepEqual(a.pending.Picks, picks) {
		t.Fatalf("App.Prepare = %+v %v", summary, err)
	}
}
