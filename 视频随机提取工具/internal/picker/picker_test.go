package picker

import (
	"context"
	"errors"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

type fixture struct {
	material, output string
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// newFixture 建立：素材根目录/开场(3)、产品(4)、结尾(1)，目标根目录/第一批。
func newFixture(t *testing.T) fixture {
	base := t.TempDir()
	f := fixture{material: filepath.Join(base, "素材"), output: filepath.Join(base, "输出")}
	for _, n := range []string{"o1.mp4", "o2.MOV", "o3.mkv"} {
		write(t, filepath.Join(f.material, "开场", n), "open-"+n)
	}
	for _, n := range []string{"p1.mp4", "p2.mp4", "p3.mp4", "p4.mp4"} {
		write(t, filepath.Join(f.material, "产品", n), "product-"+n)
	}
	write(t, filepath.Join(f.material, "结尾", "e1.mp4"), "end")
	write(t, filepath.Join(f.material, "开场", "说明.txt"), "not a video")
	write(t, filepath.Join(f.material, "开场", "子目录", "deep.mp4"), "nested")
	write(t, filepath.Join(f.material, "根目录视频.mp4"), "root video")
	if err := os.MkdirAll(filepath.Join(f.output, "第一批"), 0o755); err != nil {
		t.Fatal(err)
	}
	return f
}

func rng() *rand.Rand { return rand.New(rand.NewPCG(7, 9)) }

func names(dir string) []string {
	entries, _ := os.ReadDir(dir)
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return out
}

func TestScanOnlyDirectVideos(t *testing.T) {
	f := newFixture(t)
	os.MkdirAll(filepath.Join(f.material, TempPrefix+"x"), 0o755)
	cats, err := ScanCategories(f.material)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	for _, c := range cats {
		got[c.Name] = c.Count
	}
	if len(got) != 3 || got["开场"] != 3 || got["产品"] != 4 || got["结尾"] != 1 {
		t.Fatalf("categories = %v", got)
	}
}

func TestCopyExactCountsAndOriginalsUntouched(t *testing.T) {
	f := newFixture(t)
	p, err := Prepare(f.material, f.output, "第一批", map[string]Pick{"开场": {Count: 2}, "产品": {Count: 3}, "结尾": {Count: 0}}, "", false, rng())
	if err != nil {
		t.Fatal(err)
	}
	var last Progress
	res, err := Execute(context.Background(), p, func(pr Progress) { last = pr })
	if err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(f.output, "第一批")
	if got := names(dest); len(got) != 5 || len(res.Items) != 5 {
		t.Fatalf("dest = %v", got)
	}
	if last.Done != 5 || last.BytesDone != last.BytesTotal {
		t.Fatalf("progress = %+v", last)
	}
	// 修改副本不影响原素材。
	for _, it := range res.Items {
		os.WriteFile(filepath.Join(dest, it.Name), []byte("changed"), 0o644)
		b, _ := os.ReadFile(it.Source())
		if string(b) == "changed" {
			t.Fatal("original changed through the copy")
		}
	}
	if got := Leftovers(f.output); len(got) != 0 {
		t.Fatalf("workspace left: %v", got)
	}
}

func TestCountsValidation(t *testing.T) {
	f := newFixture(t)
	if _, err := Prepare(f.material, f.output, "第一批", map[string]Pick{"结尾": {Count: 2}}, "", false, rng()); err == nil {
		t.Fatal("expected error when count exceeds videos")
	}
	if _, err := Prepare(f.material, f.output, "第一批", map[string]Pick{}, "", false, rng()); err == nil {
		t.Fatal("expected error for empty request")
	}
	if _, err := Prepare(f.material, f.output, "第一批", map[string]Pick{"不存在": {Count: 1}}, "", false, rng()); err == nil {
		t.Fatal("expected error for missing category")
	}
}

func TestSameNamesAreAvoided(t *testing.T) {
	f := newFixture(t)
	// 结尾里也有一个 p1.mp4（大小写不同），抽全部产品时必须避开它。
	write(t, filepath.Join(f.material, "结尾", "P1.MP4"), "dup")
	for seed := uint64(0); seed < 20; seed++ {
		p, err := Prepare(f.material, f.output, "第一批", map[string]Pick{"产品": {Count: 4}, "结尾": {Count: 1}}, "", false, rand.New(rand.NewPCG(seed, 1)))
		if err != nil {
			t.Fatal(err)
		}
		seen := map[string]bool{}
		for _, it := range p.Items {
			if seen[strings.ToLower(it.Name)] {
				t.Fatalf("duplicate name %s", it.Name)
			}
			seen[strings.ToLower(it.Name)] = true
		}
	}
	// 结尾两个都要时，必然与产品冲突。
	if _, err := Prepare(f.material, f.output, "第一批", map[string]Pick{"产品": {Count: 4}, "结尾": {Count: 2}}, "", false, rng()); err == nil {
		t.Fatal("expected not-enough-videos error")
	}
}

func TestExistingFilesNeverOverwritten(t *testing.T) {
	f := newFixture(t)
	dest := filepath.Join(f.output, "第一批")
	write(t, filepath.Join(dest, "E1.mp4"), "keep me")
	if _, err := Prepare(f.material, f.output, "第一批", map[string]Pick{"结尾": {Count: 1}}, "", false, rng()); err == nil {
		t.Fatal("only video conflicts with existing file: expected error")
	}
	p, err := Prepare(f.material, f.output, "第一批", map[string]Pick{"开场": {Count: 3}}, "", false, rng())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Execute(context.Background(), p, nil); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dest, "E1.mp4")); string(b) != "keep me" {
		t.Fatal("existing file was touched")
	}
}

func TestCleanReplacesOnlyDirectVideos(t *testing.T) {
	f := newFixture(t)
	dest := filepath.Join(f.output, "第一批")
	write(t, filepath.Join(dest, "old1.mp4"), "old")
	write(t, filepath.Join(dest, "e1.mp4"), "old with same name")
	write(t, filepath.Join(dest, "notes.txt"), "keep")
	write(t, filepath.Join(dest, "sub", "inner.mp4"), "keep")
	p, err := Prepare(f.material, f.output, "第一批", map[string]Pick{"结尾": {Count: 1}}, "", true, rng())
	if err != nil {
		t.Fatal(err)
	}
	res, err := Execute(context.Background(), p, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Removed != 2 {
		t.Fatalf("removed = %d", res.Removed)
	}
	if got := strings.Join(names(dest), ","); got != "e1.mp4,notes.txt,sub" {
		t.Fatalf("dest = %s", got)
	}
	if b, _ := os.ReadFile(filepath.Join(dest, "e1.mp4")); string(b) != "end" {
		t.Fatal("new video not in place")
	}
}

func TestCancelKeepsDestinationUnchanged(t *testing.T) {
	f := newFixture(t)
	dest := filepath.Join(f.output, "第一批")
	write(t, filepath.Join(dest, "old.mp4"), "old")
	p, err := Prepare(f.material, f.output, "第一批", map[string]Pick{"产品": {Count: 4}}, "", true, rng())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	_, err = Execute(ctx, p, func(pr Progress) {
		if pr.Done == 1 {
			cancel()
		}
	})
	if !errors.Is(err, ErrCancelled) {
		t.Fatalf("err = %v", err)
	}
	if got := strings.Join(names(dest), ","); got != "old.mp4" {
		t.Fatalf("dest changed: %s", got)
	}
	if len(Leftovers(f.output)) != 0 {
		t.Fatal("workspace left after cancel")
	}
}

func TestDestinationChangedDuringCopyAborts(t *testing.T) {
	f := newFixture(t)
	dest := filepath.Join(f.output, "第一批")
	write(t, filepath.Join(dest, "old.mp4"), "old")
	p, _ := Prepare(f.material, f.output, "第一批", map[string]Pick{"产品": {Count: 2}}, "", true, rng())
	_, err := Execute(context.Background(), p, func(pr Progress) {
		if pr.Done == 1 {
			write(t, filepath.Join(dest, "external.txt"), "from another program")
		}
	})
	if err == nil || !strings.Contains(err.Error(), "改动") {
		t.Fatalf("err = %v", err)
	}
	if got := strings.Join(names(dest), ","); got != "external.txt,old.mp4" {
		t.Fatalf("dest = %s", got)
	}
}

func TestRootsAndFolders(t *testing.T) {
	f := newFixture(t)
	if err := CheckSeparate(f.material, filepath.Join(f.material, "开场")); err == nil {
		t.Fatal("nested roots must be rejected")
	}
	if err := CheckSeparate(f.output, f.output); err == nil {
		t.Fatal("same roots must be rejected")
	}
	if _, err := ResolveDestination(f.output, "不存在"); err == nil {
		t.Fatal("missing destination must be rejected")
	}
	if _, err := ResolveDestination(f.output, "../素材"); err == nil {
		t.Fatal("destination outside root must be rejected")
	}
	for _, bad := range []string{"", "a/b", "CON", "lpt1.txt", "x.", "a?", TempPrefix + "1"} {
		if ValidateFolderName(bad) == nil {
			t.Fatalf("%q should be invalid", bad)
		}
	}
	if err := CreateFolder(f.output, "第二批"); err != nil {
		t.Fatal(err)
	}
	if err := CreateFolder(f.output, "第一批"); err == nil {
		t.Fatal("duplicate folder must be rejected")
	}
	dests, _ := ListDestinations(f.output)
	if len(dests) != 2 {
		t.Fatalf("destinations = %+v", dests)
	}
	got, err := ResolveAccountDestination(f.output, map[string]string{"account": "a"}, func(map[string]string) (string, error) { return "第二批", nil })
	if err != nil || filepath.Base(got) != "第二批" {
		t.Fatalf("account resolver: %v %s", err, got)
	}
}

// 没有记录时必须返回 []，返回 null 会让记录页在渲染时出错、整个界面变成空白。
func TestEmptyHistoryIsEmptyList(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "history.json"), "null")
	s, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if h := s.History(); h == nil || len(h) != 0 {
		t.Fatalf("History() = %#v", h)
	}
	s2, _ := OpenStore(t.TempDir())
	if h := s2.History(); h == nil {
		t.Fatal("fresh store returned nil history")
	}
}

func TestLibraryListsOnlyRealCategoriesAndVideos(t *testing.T) {
	f := newFixture(t)
	vs, err := ListVideos(f.material, "开场")
	if err != nil || len(vs) != 3 || vs[0].Name != "o1.mp4" || vs[1].Name != "o2.MOV" || vs[0].Size != int64(len("open-o1.mp4")) || vs[0].Modified == 0 {
		t.Fatalf("videos = %+v %v", vs, err)
	}
	if err := os.MkdirAll(filepath.Join(f.material, "空的"), 0o755); err != nil {
		t.Fatal(err)
	}
	if vs, err := ListVideos(f.material, "空的"); err != nil || vs == nil || len(vs) != 0 {
		t.Fatalf("empty category = %#v %v", vs, err)
	}
	// 只认素材根目录下现有的分类和分类里现有的视频，路径拼出来的一律不认。
	for _, category := range []string{"", ".", "..", "不存在", "开场/子目录", filepath.Join("开场", "子目录"), filepath.Join("..", "输出"), f.output} {
		if _, err := ListVideos(f.material, category); err == nil {
			t.Fatalf("category %q was accepted", category)
		}
		if _, err := CategoryDir(f.material, category); err == nil {
			t.Fatalf("category dir %q was accepted", category)
		}
	}
	path, err := VideoPath(f.material, "开场", "o2.MOV")
	if err != nil || path != filepath.Join(f.material, "开场", "o2.MOV") {
		t.Fatalf("video path = %q %v", path, err)
	}
	for _, name := range []string{"", "说明.txt", "子目录", filepath.Join("子目录", "deep.mp4"), filepath.Join("..", "产品", "p1.mp4"), "o9.mp4"} {
		if _, err := VideoPath(f.material, "开场", name); err == nil {
			t.Fatalf("video %q was accepted", name)
		}
	}
	if _, err := ListVideos("", "开场"); err == nil {
		t.Fatal("missing root was accepted")
	}
}
