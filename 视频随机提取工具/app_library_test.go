package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"videopicker/internal/picker"

	"github.com/bigmoonur/little-tool/shared/license"
)

var appTestJPEG = []byte{0xff, 0xd8, 0xff, 0xe0, 1, 2, 0xff, 0xd9}

func TestVideosAndSaveThumbnail(t *testing.T) {
	a := licensedFixture(t, nil)
	vs, err := a.Videos("开场")
	if err != nil || len(vs) != 1 || len(vs[0].Key) != 40 || vs[0].Thumb != "" {
		t.Fatalf("videos = %+v, %v", vs, err)
	}
	key := vs[0].Key
	if err := a.SaveThumbnail("开场", "sample.mp4", ""); err != nil {
		t.Fatal(err)
	}
	vs, err = a.Videos("开场")
	if err != nil || vs[0].Key != key || vs[0].Thumb != "none" {
		t.Fatalf("failed thumbnail = %+v, %v", vs, err)
	}
	encoded := base64.StdEncoding.EncodeToString(appTestJPEG)
	for _, data := range []string{encoded, "data:image/jpeg;base64," + encoded} {
		if err := a.SaveThumbnail("开场", "sample.mp4", data); err != nil {
			t.Fatal(err)
		}
		vs, err = a.Videos("开场")
		if err != nil || vs[0].Thumb != "ready" || vs[0].Key != key {
			t.Fatalf("saved thumbnail = %+v, %v", vs, err)
		}
		path, ok := a.store.ThumbPath(key)
		b, err := os.ReadFile(path)
		if !ok || err != nil || !bytes.Equal(b, appTestJPEG) {
			t.Fatalf("JPEG cache = %q, %v, %v", b, ok, err)
		}
	}
	for _, tc := range []struct{ category, name, data string }{
		{"unknown", "sample.mp4", encoded}, {"开场", "unknown.mp4", encoded},
		{"..", "sample.mp4", encoded}, {filepath.Join("开场", "child"), "sample.mp4", encoded},
		{"开场", filepath.Join("..", "sample.mp4"), encoded}, {"开场", `..\sample.mp4`, encoded},
		{"开场", "sample.mp4", "not base64"}, {"开场", "sample.mp4", base64.StdEncoding.EncodeToString([]byte("not JPEG"))},
		{"开场", "sample.mp4", base64.StdEncoding.EncodeToString(append(append([]byte{}, appTestJPEG...), make([]byte, picker.MaxThumbBytes)...))},
	} {
		if err := a.SaveThumbnail(tc.category, tc.name, tc.data); err == nil {
			t.Fatalf("accepted invalid thumbnail: %+v", tc)
		}
	}
	if got, err := a.Videos("unknown"); err == nil || got == nil || len(got) != 0 {
		t.Fatalf("unknown videos = %#v, %v", got, err)
	}
	a.store.Update(func(s *picker.Settings) { s.Thumbnails = false })
	vs, err = a.Videos("开场")
	if err != nil || vs[0].Key != "" || vs[0].Thumb != "" {
		t.Fatalf("disabled videos = %+v, %v", vs, err)
	}
	if err := a.SaveThumbnail("unknown", "../unknown.mp4", "bad"); err != nil {
		t.Fatalf("disabled SaveThumbnail = %v", err)
	}
	if got, err := NewApp(nil, nil, nil).Videos("unknown"); err == nil || got == nil {
		t.Fatalf("nil-store videos = %#v, %v", got, err)
	}
}

func TestLibraryViewAndDisableThumbnailSettings(t *testing.T) {
	a := licensedFixture(t, nil)
	for _, mode := range []string{"grid", "list", "", "GRID", "other"} {
		want := "list"
		if mode == "grid" {
			want = "grid"
		}
		if got := a.SetLibraryView(mode); got.LibraryView != want || !got.Thumbnails {
			t.Fatalf("SetLibraryView(%q) = %+v", mode, got)
		}
	}
	if err := a.SaveThumbnail("开场", "sample.mp4", base64.StdEncoding.EncodeToString(appTestJPEG)); err != nil {
		t.Fatal(err)
	}
	if got := a.UpdateSettings(UISettings{Thumbnails: true}); !got.Thumbnails || len(a.store.ThumbStates()) != 1 {
		t.Fatalf("enabled settings = %+v", got)
	}
	if got := a.UpdateSettings(UISettings{Thumbnails: false}); got.Thumbnails || len(a.store.ThumbStates()) != 0 {
		t.Fatalf("disabled settings = %+v", got)
	}
	if _, err := os.Stat(a.store.ThumbDir()); !os.IsNotExist(err) {
		t.Fatalf("cache directory survived disabling: %v", err)
	}
	reopened, err := picker.OpenStore(a.store.Dir())
	if err != nil || reopened.Settings().Thumbnails {
		t.Fatalf("disabled setting not persisted: %v, %v", reopened, err)
	}
	info, err := NewApp(nil, nil, errors.New("unavailable")).Init()
	if err == nil || !info.Settings.Thumbnails || info.Settings.LibraryView != "list" {
		t.Fatalf("fallback settings = %+v, %v", info, err)
	}
}

func TestPruneThumbnailsKeepsCurrentAndOfflineVideos(t *testing.T) {
	a := licensedFixture(t, nil)
	root := a.store.Settings().MaterialRoot
	dir := filepath.Join(root, "开场")
	for _, name := range []string{"deleted.mp4", "modified.mp4"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("original"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	before, err := a.Videos("开场")
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range before {
		if err := a.SaveThumbnail("开场", v.Name, base64.StdEncoding.EncodeToString(appTestJPEG)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Remove(filepath.Join(dir, "deleted.mp4")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "modified.mp4"), []byte("new size and content"), 0o644); err != nil {
		t.Fatal(err)
	}
	if n := a.pruneThumbnails(); n != 2 {
		t.Fatalf("removed = %d", n)
	}
	states := a.store.ThumbStates()
	for _, v := range before {
		want := ""
		if v.Name == "sample.mp4" {
			want = "ready"
		}
		if states[v.Key] != want {
			t.Fatalf("state for %s = %q", v.Name, states[v.Key])
		}
	}
	for _, settings := range []picker.Settings{
		{MaterialRoot: filepath.Join(root, "missing"), Thumbnails: true},
		{MaterialRoot: "", Thumbnails: true},
		{MaterialRoot: root, Thumbnails: false},
	} {
		a.store.Update(func(s *picker.Settings) { s.MaterialRoot, s.Thumbnails = settings.MaterialRoot, settings.Thumbnails })
		if n := a.pruneThumbnails(); n != 0 || !reflect.DeepEqual(states, a.store.ThumbStates()) {
			t.Fatalf("offline/disabled prune = %d, %v", n, a.store.ThumbStates())
		}
	}
	if n := NewApp(nil, nil, nil).pruneThumbnails(); n != 0 {
		t.Fatalf("nil-store prune = %d", n)
	}
	// 切换根目录后原来的缓存名都失效。
	a.store.Update(func(s *picker.Settings) { s.MaterialRoot, s.Thumbnails = t.TempDir(), true })
	if n := a.pruneThumbnails(); n != 1 {
		t.Fatalf("root-change prune = %d", n)
	}
}

func TestThumbnailPruneSchedulingNeverBlocksState(t *testing.T) {
	a := licensedFixture(t, nil)
	a.thumbMu.Lock()
	done := make(chan State, 1)
	go func() { done <- a.State() }()
	select {
	case st := <-done:
		if st.MaterialError != "" {
			a.thumbMu.Unlock()
			t.Fatal(st.MaterialError)
		}
	case <-time.After(5 * time.Second):
		a.thumbMu.Unlock()
		t.Fatal("State waited for thumbnail pruning")
	}
	last := a.lastPrune.Load()
	if !a.pruneRunning.Load() || last == 0 {
		a.thumbMu.Unlock()
		t.Fatal("successful scan did not schedule pruning")
	}
	for i := 0; i < 5; i++ {
		a.scheduleThumbnailPrune()
	}
	if a.lastPrune.Load() != last {
		a.thumbMu.Unlock()
		t.Fatal("overlapping prune was scheduled")
	}
	a.thumbMu.Unlock()
	waitForImportIdle(t, func() bool { return !a.pruneRunning.Load() })
	a.scheduleThumbnailPrune()
	if a.pruneRunning.Load() || a.lastPrune.Load() != last {
		t.Fatal("pruning ignored the 20-second throttle")
	}
	a.lastPrune.Store(time.Now().Add(-21 * time.Second).UnixNano())
	a.scheduleThumbnailPrune()
	waitForImportIdle(t, func() bool { return !a.pruneRunning.Load() })
	if a.lastPrune.Load() <= last {
		t.Fatal("expired throttle did not allow another prune")
	}
}

func TestCreateCategoryAppBinding(t *testing.T) {
	a := licensedFixture(t, &testLicense{status: license.Status{State: license.StateActive}})
	st, err := a.CreateCategory("新分类")
	if err != nil || len(st.Categories) != 2 || st.Categories[1].Name != "新分类" {
		t.Fatalf("created category state = %+v, %v", st, err)
	}
	a.mu.Lock()
	a.busy = true
	a.mu.Unlock()
	st, err = a.CreateCategory("不能创建")
	if err == nil || err.Error() != "正在复制，请等待完成或取消后再操作" || !st.Busy || len(st.Categories) != 2 {
		t.Fatalf("busy category creation = %+v, %v", st, err)
	}
	if _, err := os.Stat(filepath.Join(st.MaterialRoot, "不能创建")); !os.IsNotExist(err) {
		t.Fatalf("busy creation changed disk: %v", err)
	}
}

func waitForImportIdle(t *testing.T, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ready() {
		if time.Now().After(deadline) {
			t.Fatal("background operation did not finish")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestAddVideosRefusalsAndBackgroundRun(t *testing.T) {
	a := licensedFixture(t, &testLicense{status: license.Status{State: license.StateActive}})
	if err := a.AddVideos("开场", nil); err == nil {
		t.Fatal("empty paths accepted")
	}
	a.busy = true
	if err := a.AddVideos("开场", []string{"unused"}); err == nil || err.Error() != "正在复制，请等待完成或取消后再操作" {
		t.Fatalf("busy import = %v", err)
	}
	a.busy = false
	path := filepath.Join(t.TempDir(), "import.mp4")
	if err := os.WriteFile(path, []byte("imported video"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := a.AddVideos("开场", []string{path}); err != nil {
		t.Fatal(err)
	}
	waitForImportIdle(t, func() bool { return a.idle() == nil })
	if b, err := os.ReadFile(filepath.Join(a.store.Settings().MaterialRoot, "开场", "import.mp4")); err != nil || string(b) != "imported video" {
		t.Fatalf("background import = %q, %v", b, err)
	}
}

func TestRunImportReportsCompletionAndClearsBusy(t *testing.T) {
	for _, mode := range []string{"success", "cancelled", "error", "quit"} {
		t.Run(mode, func(t *testing.T) {
			a := licensedFixture(t, nil)
			path := filepath.Join(t.TempDir(), "import.mp4")
			if err := os.WriteFile(path, []byte("imported video"), 0o644); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			a.busy, a.cancel = true, cancel
			if mode == "quit" {
				a.quitAfter = true
			}
			category := "开场"
			if mode == "error" {
				category = "missing"
			}
			var done []ImportDone
			var progress []picker.Progress
			a.runImport(ctx, cancel, a.store.Settings().MaterialRoot, category, []string{path}, func(event string, data any) {
				switch event {
				case "import-progress":
					progress = append(progress, data.(picker.Progress))
					if mode == "cancelled" {
						a.Cancel()
					}
				case "import-done":
					done = append(done, data.(ImportDone))
				default:
					t.Fatalf("unexpected event = %s", event)
				}
			})
			if a.busy || a.cancel != nil || ctx.Err() == nil {
				t.Fatal("import did not release busy state and context")
			}
			if mode == "quit" {
				if len(done) != 0 {
					t.Fatal("quit emitted completion")
				}
				return
			}
			if len(done) != 1 || done[0].Result.Added == nil || done[0].Result.Skipped == nil {
				t.Fatalf("completion events = %+v", done)
			}
			switch mode {
			case "success":
				if !done[0].OK || done[0].Cancelled || done[0].Error != "" || !reflect.DeepEqual(done[0].Result.Added, []string{"import.mp4"}) || len(progress) == 0 || progress[len(progress)-1].Done != progress[len(progress)-1].Total {
					t.Fatalf("successful import = %+v, %+v", done, progress)
				}
			case "cancelled":
				if done[0].OK || !done[0].Cancelled || done[0].Error != picker.ErrImportCancelled.Error() {
					t.Fatalf("cancelled import = %+v", done)
				}
			case "error":
				if done[0].OK || done[0].Cancelled || done[0].Error == "" {
					t.Fatalf("failed import = %+v", done)
				}
			}
		})
	}
}

func TestLibraryMutationsRequireLicense(t *testing.T) {
	if licenseBypass {
		t.Skip("development-only bypass")
	}
	a := licensedFixture(t, &testLicense{status: license.Status{State: license.StateInactive, Message: "TEST_LICENSE_REJECTED"}})
	if _, err := a.CreateCategory("new"); err == nil || err.Error() != "TEST_LICENSE_REJECTED" {
		t.Fatalf("unlicensed CreateCategory = %v", err)
	}
	if paths, err := a.ChooseVideos(); err == nil || err.Error() != "TEST_LICENSE_REJECTED" || paths == nil {
		t.Fatalf("unlicensed ChooseVideos = %v, %v", paths, err)
	}
	if err := a.AddVideos("开场", []string{"unused"}); err == nil || !strings.Contains(err.Error(), "TEST_LICENSE_REJECTED") || a.busy {
		t.Fatalf("unlicensed AddVideos = %v", err)
	}
}
