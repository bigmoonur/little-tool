package picker

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

var testJPEG = []byte{0xff, 0xd8, 0xff, 0xe0, 1, 2, 0xff, 0xd9}

func assertUserError(t *testing.T, err error) {
	t.Helper()
	var ue *UserError
	if !errors.As(err, &ue) {
		t.Fatalf("expected UserError, got %T %v", err, err)
	}
}

func TestThumbKeyAndLibraryKeys(t *testing.T) {
	root := t.TempDir()
	mod := time.Unix(1700000000, 123456789)
	key := ThumbKey(root, "分类", "a.mp4", 9, mod)
	if !validThumbKey(key) || key != ThumbKey(filepath.Join(root, "."), "分类", "a.mp4", 9, mod) {
		t.Fatalf("unstable key: %q", key)
	}
	for _, other := range []string{
		ThumbKey(filepath.Join(root, "other"), "分类", "a.mp4", 9, mod),
		ThumbKey(root, "其他", "a.mp4", 9, mod),
		ThumbKey(root, "分类", "b.mp4", 9, mod),
		ThumbKey(root, "分类", "a.mp4", 10, mod),
		ThumbKey(root, "分类", "a.mp4", 9, mod.Add(time.Nanosecond)),
		ThumbKey(root, "ab", "c.mp4", 9, mod),
		ThumbKey(root, "a", "bc.mp4", 9, mod),
	} {
		if !validThumbKey(other) || other == key {
			t.Fatalf("unchanged key: %q", other)
		}
	}
	if ThumbKey(root, "ab", "c", 9, mod) == ThumbKey(root, "a", "bc", 9, mod) {
		t.Fatal("components lack separators")
	}
	f := newFixture(t)
	path := filepath.Join(f.material, "开场", "o1.mp4")
	if err := os.Chtimes(path, mod, mod); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	vs, err := ListVideosWithKeys(f.material, "开场")
	if err != nil || vs[0].Key != ThumbKey(f.material, "开场", "o1.mp4", fi.Size(), fi.ModTime()) || vs[0].Thumb != "" {
		t.Fatalf("keyed videos = %+v, %v", vs, err)
	}
	plain, err := ListVideos(f.material, "开场")
	if err != nil || plain[0].Key != "" || plain[0].Thumb != "" {
		t.Fatalf("plain videos = %+v, %v", plain, err)
	}
	keys, err := LibraryKeys(f.material)
	if err != nil || len(keys) != 8 || !keys[vs[0].Key] {
		t.Fatalf("library keys = %v, %v", keys, err)
	}
	if keys, err := LibraryKeys(filepath.Join(f.material, "missing")); err == nil || keys != nil {
		t.Fatalf("missing library = %v, %v", keys, err)
	}
}

func TestThumbCacheValidationAndAtomicWrites(t *testing.T) {
	s, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	key := strings.Repeat("a", 40)
	for _, bad := range []string{"", "../x", strings.Repeat("a", 39), strings.Repeat("a", 41), strings.Repeat("A", 40), strings.Repeat("g", 40)} {
		assertUserError(t, s.SaveThumb(bad, testJPEG))
		assertUserError(t, s.MarkNoThumb(bad))
		if _, ok := s.ThumbPath(bad); ok {
			t.Fatalf("bad key accepted: %q", bad)
		}
	}
	for _, bad := range [][]byte{nil, {0xff, 0xd8}, {0xff, 0xd8, 0}, []byte("not JPEG"), append(testJPEG, make([]byte, MaxThumbBytes)...)} {
		assertUserError(t, s.SaveThumb(key, bad))
	}
	if _, ok := s.ThumbPath(key); ok {
		t.Fatal("validation wrote a cache file")
	}
	if err := s.SaveThumb(key, testJPEG); err != nil {
		t.Fatal(err)
	}
	replacement := append(append([]byte{}, testJPEG...), 3)
	if err := s.SaveThumb(key, replacement); err != nil {
		t.Fatal(err)
	}
	assertUserError(t, s.SaveThumb(key, []byte("bad")))
	path, ok := s.ThumbPath(key)
	b, err := os.ReadFile(path)
	if !ok || err != nil || !bytes.Equal(b, replacement) {
		t.Fatalf("atomic replacement = %q, %v, %v", b, ok, err)
	}
	// 目录占据目标位置，所有平台都能稳定触发发布失败。
	blocked := strings.Repeat("b", 40)
	if err := os.Mkdir(filepath.Join(s.ThumbDir(), blocked+".jpg"), 0o755); err != nil {
		t.Fatal(err)
	}
	assertUserError(t, s.SaveThumb(blocked, testJPEG))
	if got := names(s.ThumbDir()); !reflect.DeepEqual(got, []string{key + ".jpg", blocked + ".jpg"}) {
		t.Fatalf("temporary cache files left: %v", got)
	}
}

func TestThumbMarkersStatesPruneAndClear(t *testing.T) {
	s, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got := s.ThumbStates(); got == nil || len(got) != 0 || s.PruneThumbs(nil) != 0 {
		t.Fatalf("missing cache = %v", got)
	}
	if err := s.ClearThumbs(); err != nil {
		t.Fatal(err)
	}
	a, b, c, d := strings.Repeat("a", 40), strings.Repeat("b", 40), strings.Repeat("c", 40), strings.Repeat("d", 40)
	for _, key := range []string{a, b, c} {
		if err := s.MarkNoThumb(key); err != nil {
			t.Fatal(err)
		}
	}
	fi, err := os.Stat(filepath.Join(s.ThumbDir(), a+".none"))
	if err != nil || fi.Size() != 0 || s.ThumbStates()[a] != "none" {
		t.Fatalf("marker = %v %v", fi, err)
	}
	if err := s.SaveThumb(a, testJPEG); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkNoThumb(a); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.ThumbDir(), a+".none")); !os.IsNotExist(err) {
		t.Fatalf("marker survived SaveThumb: %v", err)
	}
	if err := s.SaveThumb(d, testJPEG); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(s.ThumbDir(), a+"-stray.tmp"), "partial")
	write(t, filepath.Join(s.ThumbDir(), "invalid.jpg"), "invalid")
	if got := s.ThumbStates(); !reflect.DeepEqual(got, map[string]string{a: "ready", b: "none", c: "none", d: "ready"}) {
		t.Fatalf("states = %v", got)
	}
	if n := s.PruneThumbs(map[string]bool{a: true, b: true}); n != 4 {
		t.Fatalf("pruned = %d", n)
	}
	if got := s.ThumbStates(); !reflect.DeepEqual(got, map[string]string{a: "ready", b: "none"}) {
		t.Fatalf("kept states = %v", got)
	}
	if err := s.ClearThumbs(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.ThumbDir()); !os.IsNotExist(err) || len(s.ThumbStates()) != 0 || s.PruneThumbs(nil) != 0 {
		t.Fatalf("cache survived clear: %v", err)
	}
	if err := s.ClearThumbs(); err != nil {
		t.Fatal(err)
	}
}

func TestThumbnailSettingsDefaultsAndView(t *testing.T) {
	for _, tc := range []struct {
		name, data, view string
		thumbnails       bool
	}{
		{"old", `{"theme":"dark"}`, "list", true},
		{"false", `{"thumbnails":false,"libraryView":"grid"}`, "grid", false},
		{"bad view", `{"libraryView":"GRID"}`, "list", true},
		{"null view", `{"libraryView":null}`, "list", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			write(t, filepath.Join(dir, "settings.json"), tc.data)
			s, err := OpenStore(dir)
			if err != nil || s.Settings().Thumbnails != tc.thumbnails || s.Settings().LibraryView != tc.view {
				t.Fatalf("settings = %+v, %v", s, err)
			}
			for _, mode := range []string{"grid", "list", "", "Grid", "other"} {
				want := "list"
				if mode == "grid" {
					want = "grid"
				}
				got := s.Update(func(st *Settings) { st.LibraryView = mode })
				if got.LibraryView != want || got.Thumbnails != tc.thumbnails {
					t.Fatalf("updated view %q = %+v", mode, got)
				}
				reopened, err := OpenStore(dir)
				if err != nil || reopened.Settings().LibraryView != want || reopened.Settings().Thumbnails != tc.thumbnails {
					t.Fatalf("reopened settings = %v, %v", reopened, err)
				}
			}
		})
	}
}
