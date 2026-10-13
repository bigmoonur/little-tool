package picker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCreateCategory(t *testing.T) {
	root := t.TempDir()
	if err := CreateCategory(root, "NewCategory"); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(filepath.Join(root, "NewCategory")); err != nil || !fi.IsDir() {
		t.Fatalf("category = %v, %v", fi, err)
	}
	if err := CreateCategory(root, "newcategory"); err == nil || err.Error() != "「NewCategory」已经存在，可以直接选择它或换一个名称" {
		t.Fatalf("duplicate = %v", err)
	}
	write(t, filepath.Join(root, "FileName"), "existing entry")
	if err := CreateCategory(root, "filename"); err == nil || err.Error() != "「FileName」已经存在，可以直接选择它或换一个名称" {
		t.Fatalf("file duplicate = %v", err)
	}
	for _, bad := range []string{"", ".", "..", "a/b", `a\b`, "CON", "x.", TempPrefix + "x"} {
		assertUserError(t, CreateCategory(root, bad))
	}
	assertUserError(t, CreateCategory("", "name"))
	assertUserError(t, CreateCategory(filepath.Join(root, "missing"), "name"))
	if got := names(root); !reflect.DeepEqual(got, []string{"FileName", "NewCategory"}) {
		t.Fatalf("unexpected entries = %v", got)
	}
}

func importFixture(t *testing.T) (root, dir, source string) {
	t.Helper()
	base := t.TempDir()
	root, source = filepath.Join(base, "素材"), filepath.Join(base, "来源")
	dir = filepath.Join(root, "分类")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	return
}

func TestImportContentsTimesSourcesAndProgress(t *testing.T) {
	root, dir, source := importFixture(t)
	paths := []string{filepath.Join(source, "first.mp4"), filepath.Join(source, "second.MOV")}
	mod := time.Unix(1700000000, 123456700)
	for _, path := range paths {
		write(t, path, "source-"+filepath.Base(path))
		if err := os.Chtimes(path, mod, mod); err != nil {
			t.Fatal(err)
		}
	}
	var updates []Progress
	res, err := Import(context.Background(), root, "分类", paths, func(p Progress) { updates = append(updates, p) })
	if err != nil || !reflect.DeepEqual(res.Added, []string{"first.mp4", "second.MOV"}) || res.Skipped == nil || len(res.Skipped) != 0 || res.Category != "分类" {
		t.Fatalf("import = %+v, %v", res, err)
	}
	var total int64
	for _, path := range paths {
		want := "source-" + filepath.Base(path)
		b, err := os.ReadFile(filepath.Join(dir, filepath.Base(path)))
		if err != nil || string(b) != want {
			t.Fatalf("copied content = %q, %v", b, err)
		}
		original, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		copy, err := os.Stat(filepath.Join(dir, filepath.Base(path)))
		if err != nil || !copy.ModTime().Equal(original.ModTime()) || copy.Size() != original.Size() {
			t.Fatalf("copied metadata = %v, %v", copy, err)
		}
		total += original.Size()
		write(t, filepath.Join(dir, filepath.Base(path)), "changed copy")
		b, err = os.ReadFile(path)
		if err != nil || string(b) != want {
			t.Fatalf("source changed = %q, %v", b, err)
		}
		after, err := os.Stat(path)
		if err != nil || !after.ModTime().Equal(original.ModTime()) {
			t.Fatalf("source timestamp changed = %v, %v", after, err)
		}
	}
	last := updates[len(updates)-1]
	if res.Bytes != total || last.Done != 2 || last.Total != 2 || last.BytesDone != total || last.BytesTotal != total {
		t.Fatalf("result/progress = %+v, %+v", res, updates)
	}
	for _, p := range updates[:len(updates)-1] {
		if p.Done < 0 || p.Done > 1 || p.Name != res.Added[p.Done] || p.Total != 2 || p.BytesTotal != total {
			t.Fatalf("copy progress = %+v", p)
		}
	}
}

func TestImportSkipsAndNeverOverwrites(t *testing.T) {
	root, dir, source := importFixture(t)
	write(t, filepath.Join(dir, "existing.mp4"), "keep")
	write(t, filepath.Join(dir, "in-category.mp4"), "already here")
	paths := []string{
		filepath.Join(source, "missing.mp4"), filepath.Join(source, "directory.mp4"),
		filepath.Join(source, "notes.txt"), filepath.Join(dir, "in-category.mp4"),
		filepath.Join(source, "EXISTING.MP4"), filepath.Join(source, "accepted.mp4"),
		filepath.Join(source, "other", "ACCEPTED.MP4"),
	}
	if err := os.Mkdir(paths[1], 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{paths[2], paths[4], paths[5], paths[6]} {
		write(t, path, "new")
	}
	res, err := Import(context.Background(), root, "分类", paths, nil)
	wantSkipped := []ImportSkip{
		{Name: "missing.mp4", Reason: "不是文件"}, {Name: "directory.mp4", Reason: "不是文件"},
		{Name: "notes.txt", Reason: "不是支持的视频格式"}, {Name: "in-category.mp4", Reason: "已经在这个分类里"},
		{Name: "EXISTING.MP4", Reason: "分类里已有同名文件"}, {Name: "ACCEPTED.MP4", Reason: "分类里已有同名文件"},
	}
	if err != nil || !reflect.DeepEqual(res.Added, []string{"accepted.mp4"}) || !reflect.DeepEqual(res.Skipped, wantSkipped) || res.Bytes != 3 {
		t.Fatalf("import skips = %+v, %v", res, err)
	}
	if b, err := os.ReadFile(filepath.Join(dir, "existing.mp4")); err != nil || string(b) != "keep" {
		t.Fatalf("existing file changed = %q, %v", b, err)
	}
	for _, paths := range [][]string{nil, {filepath.Join(source, "missing.mp4"), filepath.Join(dir, "accepted.mp4")}} {
		res, err := Import(context.Background(), root, "分类", paths, nil)
		if err != nil || res.Added == nil || res.Skipped == nil || len(res.Added) != 0 || res.Bytes != 0 {
			t.Fatalf("empty import = %+v, %v", res, err)
		}
	}
	for _, category := range []string{"", "missing", "..", filepath.Join("分类", "child")} {
		_, err := Import(context.Background(), root, category, paths, nil)
		assertUserError(t, err)
	}
}

func TestImportRollbackOnCancellationAndFailure(t *testing.T) {
	for _, mode := range []string{"cancel", "copy failure", "changed source"} {
		t.Run(mode, func(t *testing.T) {
			root, dir, source := importFixture(t)
			write(t, filepath.Join(dir, "keep.txt"), "untouched")
			first, second := filepath.Join(source, "first.mp4"), filepath.Join(source, "second.mp4")
			write(t, first, "small")
			write(t, second, strings.Repeat("x", 3<<20))
			before, err := inventory(dir)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			triggered := false
			res, err := Import(ctx, root, "分类", []string{first, second}, func(p Progress) {
				if p.Done != 1 || triggered {
					return
				}
				switch mode {
				case "cancel":
					if p.BytesDone < int64(len("small"))+(1<<20) {
						return
					}
					cancel()
				case "copy failure":
					if err := os.Remove(second); err != nil {
						t.Fatal(err)
					}
				case "changed source":
					write(t, second, "changed")
				}
				triggered = true
			})
			if !triggered || err == nil || res.Added == nil || len(res.Added) != 0 || res.Bytes != 0 {
				t.Fatalf("failed import = %+v, %v, triggered=%v", res, err, triggered)
			}
			if mode == "cancel" {
				if !errors.Is(err, ErrImportCancelled) || err.Error() != "已取消，这次没有添加任何视频" {
					t.Fatalf("cancel error = %v", err)
				}
			} else {
				assertUserError(t, err)
				if mode == "changed source" && err.Error() != "复制期间文件发生了变化，请重试：second.mp4" {
					t.Fatalf("change error = %v", err)
				}
				if mode == "copy failure" && !strings.HasPrefix(err.Error(), "添加失败，已撤销这次添加：") {
					t.Fatalf("copy failure error = %v", err)
				}
			}
			after, invErr := inventory(dir)
			if invErr != nil || !sameInventory(before, after) {
				t.Fatalf("rollback changed directory: before=%v after=%v, %v", before, after, invErr)
			}
		})
	}
}

func TestImportConcurrentNameConflictKeepsExternalFile(t *testing.T) {
	root, dir, source := importFixture(t)
	path := filepath.Join(source, "video.mp4")
	write(t, path, "source")
	_, err := Import(context.Background(), root, "分类", []string{path}, func(p Progress) {
		if p.Done == 0 && p.BytesDone == 0 {
			write(t, filepath.Join(dir, "VIDEO.MP4"), "external")
		}
	})
	if err == nil || err.Error() != "分类里刚出现了同名文件：video.mp4" {
		t.Fatalf("conflict = %v", err)
	}
	if got := names(dir); !reflect.DeepEqual(got, []string{"VIDEO.MP4"}) {
		t.Fatalf("rollback removed external file or left temporary file: %v", got)
	}
	if b, err := os.ReadFile(filepath.Join(dir, "VIDEO.MP4")); err != nil || string(b) != "external" {
		t.Fatalf("external file changed: %q, %v", b, err)
	}
}
