package picker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestHistoryParametersAndSnapshots(t *testing.T) {
	f := newFixture(t)
	s, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p, err := Prepare(f.material, f.output, "第一批", map[string]Pick{"产品": {Count: 2, Repeat: 3}, "结尾": {Count: 1, Repeat: 2}}, ModeRandom, false, rng())
	if err != nil {
		t.Fatal(err)
	}
	r, err := Execute(context.Background(), p, nil)
	if err != nil {
		t.Fatal(err)
	}
	entry := s.AddHistory(r)
	want := s.History()
	if len(want) != 1 || !reflect.DeepEqual(want[0].Picks, p.Picks) || want[0].Mode != ModeRandom || want[0].Bytes != p.Bytes || !reflect.DeepEqual(want[0].Items, p.Items) {
		t.Fatalf("history lost parameters: %+v", want)
	}
	r.Picks["产品"] = Pick{Count: 99}
	r.Items[0].Name = "result mutation"
	entry.Picks["产品"] = Pick{Count: 88}
	entry.Items[0].Name = "entry mutation"
	got := s.History()
	got[0].Picks["产品"] = Pick{Count: 77}
	got[0].Items[0].Name = "history mutation"
	if !reflect.DeepEqual(want, s.History()) {
		t.Fatal("history aliases result, AddHistory return, or History snapshot")
	}
	reopened, err := OpenStore(s.Dir())
	if err != nil {
		t.Fatal(err)
	}
	loaded := reopened.History()
	if len(loaded) != 1 || !reflect.DeepEqual(loaded[0].Picks, p.Picks) || loaded[0].Mode != ModeRandom || loaded[0].Bytes != p.Bytes || loaded[0].Items[1].Origin != want[0].Items[1].Origin {
		t.Fatalf("reopened history = %+v", loaded)
	}
}

func TestOldHistoryDerivesPicksWithoutAliasing(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "history.json"), `[
		{"id":"old","items":[{"category":"开场","name":"a.mp4"},{"category":"开场","name":"b.mp4"},{"category":"离线","name":"c.mp4"}]},
		{"id":"empty","items":[]}
	]`)
	s, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]Pick{"开场": {Count: 2, Repeat: 1}, "离线": {Count: 1, Repeat: 1}}
	h := s.History()
	if len(h) != 2 || !reflect.DeepEqual(h[0].Picks, want) || h[0].Mode != ModeConsecutive || h[1].Picks == nil || len(h[1].Picks) != 0 || h[1].Mode != ModeConsecutive {
		t.Fatalf("legacy history = %+v", h)
	}
	h[0].Items[0].Category = "mutated"
	h[0].Picks["开场"] = Pick{Count: 99}
	h[1].Picks["new"] = Pick{Count: 1}
	if got := s.History(); !reflect.DeepEqual(got[0].Picks, want) || got[0].Items[0].Category != "开场" || len(got[1].Picks) != 0 {
		t.Fatalf("legacy snapshot aliases storage: %+v", got)
	}
}

func TestDraftNormalizationAndSnapshots(t *testing.T) {
	for _, tt := range []struct {
		name, mode string
		remember   bool
		wantMode   string
	}{
		{"old", "", true, ModeConsecutive},
		{"random", ModeRandom, true, ModeRandom},
		{"unknown", "other", true, ModeConsecutive},
		{"disabled", ModeRandom, false, ModeConsecutive},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			settings := Settings{RememberDraft: tt.remember, Draft: map[string]int{"离线": 2}, Mode: tt.mode}
			if err := writeJSON(filepath.Join(dir, "settings.json"), settings); err != nil {
				t.Fatal(err)
			}
			s, err := OpenStore(dir)
			if err != nil {
				t.Fatal(err)
			}
			got := s.Settings()
			if got.Draft == nil || got.DraftRepeat == nil || got.Mode != tt.wantMode || (tt.remember && got.Draft["离线"] != 2) || (!tt.remember && len(got.Draft) != 0) {
				t.Fatalf("loaded settings = %+v", got)
			}
			counts, repeats := map[string]int{"离线": 3}, map[string]int{"离线": 7}
			s.Update(func(st *Settings) {
				st.RememberDraft = true
				st.Draft, st.DraftRepeat, st.Mode = counts, repeats, ModeRandom
			})
			counts["离线"], repeats["离线"] = 99, 99
			got = s.Settings()
			got.Draft["离线"], got.DraftRepeat["离线"] = 88, 88
			if got := s.Settings(); got.Draft["离线"] != 3 || got.DraftRepeat["离线"] != 7 {
				t.Fatal("settings maps alias input or snapshot")
			}
			reopened, err := OpenStore(dir)
			if err != nil || !reflect.DeepEqual(s.Settings(), reopened.Settings()) {
				t.Fatalf("draft parameters not persisted: %v", err)
			}
			got = s.Update(func(st *Settings) { st.RememberDraft = false })
			if got.Draft == nil || got.DraftRepeat == nil || len(got.Draft) != 0 || len(got.DraftRepeat) != 0 || got.Mode != ModeConsecutive {
				t.Fatalf("disabled draft retained values: %+v", got)
			}
		})
	}
}

func TestPresetNamesAndPickCleaning(t *testing.T) {
	s, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, mode, message string
		picks               map[string]Pick
	}{
		{" \t\n　", "", "请填写方案名称", map[string]Pick{"A": {Count: 1}}},
		{strings.Repeat("字", 41), "", "方案名称最多 40 个字", map[string]Pick{"A": {Count: 1}}},
		{"empty", "", "请先为至少一个分类填写数量，再保存为方案", nil},
		{"nonpositive", "", "请先为至少一个分类填写数量，再保存为方案", map[string]Pick{"A": {Count: -1}, "B": {Repeat: 100}}},
		{"too large", "", "「离线」的乘数最多 99", map[string]Pick{"离线": {Count: 1, Repeat: 100}}},
		{"mode", "other", "不支持的乘数规则", map[string]Pick{"A": {Count: 1}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			id, err := s.SavePreset(tt.name, tt.picks, tt.mode)
			var user *UserError
			if id != "" || !errors.As(err, &user) || err.Error() != tt.message || len(s.Presets()) != 0 {
				t.Fatalf("save = %q %v, presets=%v", id, err, s.Presets())
			}
		})
	}
	picks := map[string]Pick{"离线": {Count: 2, Repeat: -5}, "零": {Repeat: 100}, "负": {Count: -3}, "另一分类": {Count: 1, Repeat: 99}}
	id, err := s.SavePreset(" \t晨间\n　方案  ", picks, "")
	if err != nil || id == "" {
		t.Fatalf("save cleaned preset: %s %v", id, err)
	}
	want := Preset{ID: id, Name: "晨间 方案", Mode: ModeConsecutive, Picks: map[string]Pick{"离线": {Count: 2, Repeat: 1}, "另一分类": {Count: 1, Repeat: 99}}}
	if got := s.Presets(); len(got) != 1 || !reflect.DeepEqual(got[0], want) {
		t.Fatalf("cleaned preset = %+v", got)
	}
	picks["离线"] = Pick{Count: 99}
	got := s.Presets()
	got[0].Picks["离线"] = Pick{Count: 88}
	got[0].Name = "changed"
	if !reflect.DeepEqual(s.Presets(), []Preset{want}) {
		t.Fatal("preset aliases input or returned state")
	}
	if _, err := s.SavePreset(strings.Repeat("字", 40), map[string]Pick{"A": {Count: 1}}, ModeRandom); err != nil {
		t.Fatalf("40-rune name rejected: %v", err)
	}
}

func TestPresetReplacementRenameDeleteAndPersistence(t *testing.T) {
	s, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.SavePreset("First Plan", map[string]Pick{"离线分类": {Count: 2}}, "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.SavePreset("Second", map[string]Pick{"B": {Count: 3}}, "")
	if err != nil || a == b {
		t.Fatalf("append = %s %v", b, err)
	}
	snapshot := s.Presets()
	replacement := map[string]Pick{"仍然离线": {Count: 4, Repeat: 5}}
	id, err := s.SavePreset(" first\tPLAN ", replacement, ModeRandom)
	if err != nil || id != a {
		t.Fatalf("replacement changed ID: %s %v", id, err)
	}
	got := s.Presets()
	if len(got) != 2 || got[0].ID != a || got[1].ID != b || got[0].Name != "first PLAN" || got[0].Mode != ModeRandom || !reflect.DeepEqual(got[0].Picks, replacement) || snapshot[0].Picks["离线分类"].Count != 2 {
		t.Fatalf("replacement changed order, values, or old snapshot: %+v", got)
	}
	before := s.Presets()
	for _, tt := range []struct {
		id, name, message string
	}{
		{"missing", "Valid", "方案不存在，可能已被删除"},
		{a, " \t", "请填写方案名称"},
		{a, strings.Repeat("字", 41), "方案名称最多 40 个字"},
		{a, " second ", "已有同名方案「Second」"},
	} {
		if err := s.RenamePreset(tt.id, tt.name); err == nil || err.Error() != tt.message || !reflect.DeepEqual(s.Presets(), before) {
			t.Fatalf("rename %q %q = %v", tt.id, tt.name, err)
		}
	}
	if err := s.RenamePreset(a, " FIRST  plan "); err != nil {
		t.Fatalf("self rename rejected: %v", err)
	}
	if err := s.RenamePreset(a, " 新\n　方案 "); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenStore(s.Dir())
	if err != nil || !reflect.DeepEqual(s.Presets(), reopened.Presets()) || reopened.Presets()[0].Name != "新 方案" {
		t.Fatalf("presets not persisted: %v", err)
	}
	before = reopened.Presets()
	if err := reopened.DeletePreset("missing"); err == nil || err.Error() != "方案不存在，可能已被删除" || !reflect.DeepEqual(reopened.Presets(), before) {
		t.Fatalf("delete unknown = %v", err)
	}
	if err := reopened.DeletePreset(a); err != nil {
		t.Fatal(err)
	}
	final, err := OpenStore(s.Dir())
	if err != nil || len(final.Presets()) != 1 || final.Presets()[0].ID != b {
		t.Fatalf("delete not persisted: %v", err)
	}
	if err := final.DeletePreset(b); err != nil {
		t.Fatal(err)
	}
	if got := final.Presets(); got == nil || len(got) != 0 {
		t.Fatalf("last delete = %#v", got)
	}
}

func TestLoadMissingAndCorruptPresets(t *testing.T) {
	for name, content := range map[string]string{
		"missing": "", "corrupt": "{", "null": "null",
		"partial type error": `[{"id":"partial","name":"partial","picks":{"A":{"count":1}}},{"picks":[]}]`,
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if content != "" {
				write(t, filepath.Join(dir, "presets.json"), content)
			}
			s, err := OpenStore(dir)
			if err != nil {
				t.Fatal(err)
			}
			if got := s.Presets(); got == nil || len(got) != 0 {
				t.Fatalf("Presets() = %#v", got)
			}
			id, err := s.SavePreset("恢复", map[string]Pick{"未连接": {Count: 1}}, "")
			if err != nil || id == "" {
				t.Fatalf("cannot recover corrupt presets: %s %v", id, err)
			}
			reopened, err := OpenStore(dir)
			if err != nil || !reflect.DeepEqual(s.Presets(), reopened.Presets()) {
				t.Fatalf("recovery not persisted: %v", err)
			}
		})
	}
}

func TestPresetSaveFailureKeepsState(t *testing.T) {
	s, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	id, err := s.SavePreset("已有", map[string]Pick{"A": {Count: 1}}, "")
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(s.Dir(), "presets.json"))
	if err != nil {
		t.Fatal(err)
	}
	state := s.Presets()
	if err := os.Mkdir(filepath.Join(s.Dir(), "presets.json.tmp"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, call := range map[string]func() error{
		"append": func() error {
			id, err := s.SavePreset("新方案", map[string]Pick{"B": {Count: 1}}, "")
			if id != "" {
				t.Fatal("failed save returned an ID")
			}
			return err
		},
		"replace": func() error {
			_, err := s.SavePreset("已有", map[string]Pick{"B": {Count: 2}}, ModeRandom)
			return err
		},
		"rename": func() error { return s.RenamePreset(id, "新名称") },
		"delete": func() error { return s.DeletePreset(id) },
	} {
		t.Run(name, func(t *testing.T) {
			err := call()
			var user *UserError
			if !errors.As(err, &user) || !strings.HasPrefix(err.Error(), "无法保存方案：") {
				t.Fatalf("write failure = %v", err)
			}
			after, err := os.ReadFile(filepath.Join(s.Dir(), "presets.json"))
			if err != nil || string(after) != string(before) || !reflect.DeepEqual(s.Presets(), state) {
				t.Fatalf("write failure changed state: %v", err)
			}
		})
	}
}

func TestConcurrentPresets(t *testing.T) {
	s, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Go(func() {
			name := fmt.Sprintf("方案 %d", i)
			picks := map[string]Pick{"离线": {Count: i + 1, Repeat: 2}}
			id, err := s.SavePreset(name, picks, ModeRandom)
			if err != nil {
				t.Error(err)
				return
			}
			if err := s.RenamePreset(id, name+"新"); err != nil {
				t.Error(err)
			}
			for _, p := range s.Presets() {
				if p.ID == id && !reflect.DeepEqual(p.Picks, picks) {
					t.Errorf("concurrent update lost picks: %+v", p)
				}
			}
		})
	}
	wg.Wait()
	reopened, err := OpenStore(s.Dir())
	if err != nil || len(reopened.Presets()) != 12 || !reflect.DeepEqual(s.Presets(), reopened.Presets()) {
		t.Fatalf("concurrent presets lost state: %v", err)
	}
	encoded, err := json.Marshal(reopened.Presets())
	if err != nil || strings.Contains(string(encoded), `"picks":null`) {
		t.Fatalf("preset picks JSON = %s %v", encoded, err)
	}
}
