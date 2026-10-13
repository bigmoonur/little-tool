package picker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func checkCopyNames(t *testing.T, p Plan) map[string]int {
	t.Helper()
	seenNames, seenSources := map[string]bool{}, map[string]int{}
	var size int64
	for _, it := range p.Items {
		original := filepath.Base(it.Source())
		if seenNames[fold(it.Name)] {
			t.Fatalf("duplicate destination name %q", it.Name)
		}
		seenNames[fold(it.Name)] = true
		if seenSources[it.Source()] == 0 {
			if it.Name != original || it.Origin != "" {
				t.Fatalf("first copy lost original name: %+v", it)
			}
		} else if it.Name == original || it.Origin != original {
			t.Fatalf("later copy has wrong name or origin: %+v", it)
		}
		seenSources[it.Source()]++
		size += it.Size
	}
	if p.Bytes != size {
		t.Fatalf("bytes = %d, want %d", p.Bytes, size)
	}
	return seenSources
}

func TestRepeatMultiplier(t *testing.T) {
	f := newFixture(t)
	picks := map[string]Pick{"开场": {Count: 2, Repeat: 4}, "产品": {Count: 1}, "不存在": {Repeat: -1}}
	p, err := Prepare(f.material, f.output, "第一批", picks, "", false, rng())
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Items) != 9 || p.Mode != ModeRepeat || !reflect.DeepEqual(p.Picks, map[string]Pick{"开场": {Count: 2, Repeat: 4}, "产品": {Count: 1, Repeat: 1}}) {
		t.Fatalf("plan parameters = %+v", p)
	}
	counts := checkCopyNames(t, p)
	if len(counts) != 3 {
		t.Fatalf("picked sources = %v", counts)
	}
	for path, count := range counts {
		want := p.Picks[filepath.Base(filepath.Dir(path))].Repeat
		if count != want {
			t.Fatalf("%s copied %d times, want %d", path, count, want)
		}
	}
	picks["开场"] = Pick{Count: 99}
	if p.Picks["开场"].Count != 2 {
		t.Fatal("plan parameters alias input")
	}
	for _, it := range p.Items {
		encoded, err := json.Marshal(it)
		if err != nil || bytes.Contains(encoded, []byte(`"origin"`)) != (it.Origin != "") {
			t.Fatalf("origin JSON = %s, %v", encoded, err)
		}
	}
}

func TestRandomRounds(t *testing.T) {
	f := newFixture(t)
	picks := map[string]Pick{"产品": {Count: 2, Repeat: 4}, "开场": {Count: 2, Repeat: 2}, "结尾": {Count: 1, Repeat: 3}}
	p, err := Prepare(f.material, f.output, "第一批", picks, ModeRandom, false, rng())
	if err != nil {
		t.Fatal(err)
	}
	if p.Mode != ModeRandom || len(p.Items) != 15 || !reflect.DeepEqual(p.Picks, picks) {
		t.Fatalf("random plan = %+v", p)
	}
	counts := checkCopyNames(t, p)
	if counts[filepath.Join(f.material, "结尾", "e1.mp4")] != 3 {
		t.Fatal("one-video category was not repeated in each active round")
	}
	index := 0
	productRounds := map[string]bool{}
	for round := 0; round < 4; round++ {
		for _, category := range []string{"产品", "开场", "结尾"} {
			pick := picks[category]
			if round >= pick.Repeat {
				continue
			}
			seen := map[string]bool{}
			var sources []string
			for i := 0; i < pick.Count; i++ {
				it := p.Items[index]
				index++
				if it.Category != category || seen[it.Source()] {
					t.Fatalf("round %d category %s is not distinct: %+v", round, category, it)
				}
				seen[it.Source()] = true
				sources = append(sources, filepath.Base(it.Source()))
			}
			if category == "产品" {
				// 比较集合，防止实现只重新排列同一批素材。
				if sources[0] > sources[1] {
					sources[0], sources[1] = sources[1], sources[0]
				}
				productRounds[strings.Join(sources, ",")] = true
			}
		}
	}
	if len(productRounds) < 2 || index != len(p.Items) {
		t.Fatalf("rounds did not redraw videos: %v", productRounds)
	}
}

func TestSuffixGrowthAndKeptNames(t *testing.T) {
	f := newFixture(t)
	picks := map[string]Pick{"结尾": {Count: 1, Repeat: 40}}
	first, err := Prepare(f.material, f.output, "第一批", picks, ModeRepeat, false, rng())
	if err != nil {
		t.Fatal(err)
	}
	// 占用同一种子原本会给第一个副本使用的名称，并以不同大小写保存。
	kept := strings.ToUpper(first.Items[1].Name)
	write(t, filepath.Join(f.output, "第一批", kept), "keep")
	p, err := Prepare(f.material, f.output, "第一批", picks, ModeRepeat, false, rng())
	if err != nil {
		t.Fatal(err)
	}
	checkCopyNames(t, p)
	one, two := 0, 0
	for _, it := range p.Items[1:] {
		if fold(it.Name) == fold(kept) {
			t.Fatal("copy collides with kept destination file")
		}
		suffix := strings.TrimSuffix(strings.TrimPrefix(it.Name, "e1_"), ".mp4")
		for _, ch := range suffix {
			if !strings.ContainsRune(suffixChars, ch) {
				t.Fatalf("invalid suffix %q", suffix)
			}
		}
		switch len(suffix) {
		case 1:
			if two > 0 {
				t.Fatal("returned to short suffixes after exhaustion")
			}
			one++
		case 2:
			two++
		default:
			t.Fatalf("unexpected suffix length: %q", suffix)
		}
	}
	if one != 35 || two != 4 {
		t.Fatalf("suffix lengths: one=%d two=%d", one, two)
	}
	if _, err := Execute(context.Background(), p, nil); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(p.Destination, kept)); err != nil || string(b) != "keep" {
		t.Fatalf("kept suffix changed: %q %v", b, err)
	}
}

func TestSuffixCannotTakeSelectedOriginal(t *testing.T) {
	f := newFixture(t)
	for _, ch := range suffixChars {
		write(t, filepath.Join(f.material, "结尾", "e1_"+string(ch)+".mp4"), "another source")
	}
	p, err := Prepare(f.material, f.output, "第一批", map[string]Pick{"结尾": {Count: 37, Repeat: 2}}, ModeRepeat, false, rng())
	if err != nil {
		t.Fatal(err)
	}
	counts := checkCopyNames(t, p)
	if len(counts) != 37 {
		t.Fatalf("sources = %v", counts)
	}
	for _, n := range counts {
		if n != 2 {
			t.Fatalf("copy count = %d", n)
		}
	}
}

func TestMultiplierAvoidance(t *testing.T) {
	for _, mode := range []string{ModeRepeat, ModeRandom} {
		t.Run(mode, func(t *testing.T) {
			f := newFixture(t)
			write(t, filepath.Join(f.material, "结尾", "P1.MP4"), "different source")
			write(t, filepath.Join(f.material, "结尾", "OLD.mp4"), "blocked source")
			write(t, filepath.Join(f.output, "第一批", "old.MP4"), "kept")
			picks := map[string]Pick{"产品": {Count: 4, Repeat: 2}, "结尾": {Count: 1, Repeat: 4}}
			p, err := Prepare(f.material, f.output, "第一批", picks, mode, false, rng())
			if err != nil {
				t.Fatal(err)
			}
			checkCopyNames(t, p)
			if p.Avoided != 2 || len(p.Items) != 12 {
				t.Fatalf("avoided=%d items=%d", p.Avoided, len(p.Items))
			}
			for _, it := range p.Items {
				if it.Category == "结尾" && filepath.Base(it.Source()) != "e1.mp4" {
					t.Fatalf("selected conflicting source %+v", it)
				}
			}
			picks["结尾"] = Pick{Count: 2, Repeat: 4}
			_, err = Prepare(f.material, f.output, "第一批", picks, mode, false, rng())
			if err == nil || !strings.Contains(err.Error(), "同名") {
				t.Fatalf("expected pool error, got %v", err)
			}
		})
	}
}

func TestCleanKeepsVideoNamedDirectories(t *testing.T) {
	f := newFixture(t)
	dest := filepath.Join(f.output, "第一批")
	write(t, filepath.Join(dest, "e1.mp4"), "old video")
	if err := os.Mkdir(filepath.Join(dest, "E1.MP4"), 0o755); err != nil {
		if errors.Is(err, os.ErrExist) {
			t.Skip("filesystem does not allow names differing only by case")
		}
		t.Fatal(err)
	}
	_, err := Prepare(f.material, f.output, "第一批", map[string]Pick{"结尾": {Count: 1, Repeat: 2}}, ModeRepeat, true, rng())
	if err == nil || !strings.Contains(err.Error(), "同名") {
		t.Fatalf("cleaning should not make a kept directory's name available: %v", err)
	}
}

func TestPickValidation(t *testing.T) {
	f := newFixture(t)
	for _, tt := range []struct {
		name, category, mode, message string
		pick                          Pick
	}{
		{"negative count", "结尾", "", "「结尾」的数量不能小于 0", Pick{Count: -1}},
		{"negative repeat", "结尾", "", "「结尾」的乘数至少为 1", Pick{Count: 1, Repeat: -1}},
		{"large repeat", "结尾", "", "「结尾」的乘数最多 99", Pick{Count: 1, Repeat: 100}},
		{"unknown mode", "结尾", "other", "不支持的乘数规则", Pick{Count: 1}},
		{"too many items", "产品", "", "一次最多复制 10000 个文件，请减小数量或乘数", Pick{Count: 102, Repeat: 99}},
		{"overflow", "产品", "", "一次最多复制 10000 个文件，请减小数量或乘数", Pick{Count: int(^uint(0) >> 1), Repeat: 99}},
		{"missing category", "不存在", "", "分类「不存在」已经不存在了，请检查素材根目录", Pick{Count: 1}},
		{"too many videos", "结尾", "", "「结尾」只有 1 个视频，不能抽取 2 个", Pick{Count: 2, Repeat: 2}},
		{"skipped", "不存在", "", "请至少为一个分类填写大于 0 的数量", Pick{Repeat: 100}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Prepare(f.material, f.output, "第一批", map[string]Pick{tt.category: tt.pick}, tt.mode, false, rng())
			var user *UserError
			if !errors.As(err, &user) || err.Error() != tt.message {
				t.Fatalf("error = %v, want %q", err, tt.message)
			}
		})
	}
	for i := 0; i < 100; i++ {
		write(t, filepath.Join(f.material, "大量", fmt.Sprintf("%03d.mp4", i)), "video")
	}
	picks := map[string]Pick{"大量": {Count: 100, Repeat: 99}, "产品": {Count: 2, Repeat: 50}}
	p, err := Prepare(f.material, f.output, "第一批", picks, ModeRandom, false, rng())
	if err != nil || len(p.Items) != MaxItems {
		t.Fatalf("exact limit rejected: items=%d err=%v", len(p.Items), err)
	}
	checkCopyNames(t, p)
	picks["产品"] = Pick{Count: 2, Repeat: 51}
	_, err = Prepare(f.material, f.output, "第一批", picks, ModeRepeat, false, rng())
	if err == nil || err.Error() != "一次最多复制 10000 个文件，请减小数量或乘数" {
		t.Fatalf("combined item limit = %v", err)
	}
}

func TestExecuteCopies(t *testing.T) {
	for _, mode := range []string{ModeRepeat, ModeRandom} {
		t.Run(mode, func(t *testing.T) {
			f := newFixture(t)
			picks := map[string]Pick{"产品": {Count: 2, Repeat: 3}, "结尾": {Count: 1, Repeat: 4}}
			p, err := Prepare(f.material, f.output, "第一批", picks, mode, false, rng())
			if err != nil {
				t.Fatal(err)
			}
			originals := map[string][]byte{}
			for _, it := range p.Items {
				b, err := os.ReadFile(it.Source())
				if err != nil {
					t.Fatal(err)
				}
				originals[it.Source()] = b
			}
			var last Progress
			res, err := Execute(context.Background(), p, func(pr Progress) { last = pr })
			if err != nil || len(names(p.Destination)) != len(p.Items) || !reflect.DeepEqual(res.Picks, picks) || res.Mode != mode {
				t.Fatalf("execute copies: %+v %v", res, err)
			}
			if last.Done != len(p.Items) || last.BytesDone != p.Bytes || last.BytesTotal != p.Bytes {
				t.Fatalf("progress = %+v", last)
			}
			for _, it := range p.Items {
				b, err := os.ReadFile(filepath.Join(p.Destination, it.Name))
				if err != nil || !bytes.Equal(b, originals[it.Source()]) {
					t.Fatalf("wrong contents for %s: %q %v", it.Name, b, err)
				}
				write(t, filepath.Join(p.Destination, it.Name), "changed copy")
			}
			for path, original := range originals {
				b, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(b, original) {
					t.Fatalf("source changed: %s %v", path, err)
				}
			}
			res.Picks["产品"] = Pick{Count: 999}
			res.Items[0].Name = "changed"
			if p.Picks["产品"].Count != 2 || p.Items[0].Name == "changed" || len(Leftovers(f.output)) != 0 {
				t.Fatal("result aliases plan or workspace was left behind")
			}
		})
	}
}

type cancelOnPublish struct {
	context.Context
	path   string
	cancel context.CancelFunc
}

func (c cancelOnPublish) Err() error {
	if _, err := os.Lstat(c.path); err == nil {
		c.cancel()
	}
	return c.Context.Err()
}

func TestCopiesRestoreDestinationOnCancelAndFailure(t *testing.T) {
	for _, scenario := range []string{"cancel staging", "cancel publishing", "changed copy source", "missing copy source"} {
		t.Run(scenario, func(t *testing.T) {
			f := newFixture(t)
			dest := filepath.Join(f.output, "第一批")
			write(t, filepath.Join(dest, "old.mp4"), "old video")
			write(t, filepath.Join(dest, "notes.txt"), "notes")
			p, err := Prepare(f.material, f.output, "第一批", map[string]Pick{"产品": {Count: 1, Repeat: 3}, "结尾": {Count: 1, Repeat: 2}}, ModeRepeat, true, rng())
			if err != nil {
				t.Fatal(err)
			}
			before, err := inventory(dest)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var runCtx context.Context = ctx
			if scenario == "cancel publishing" {
				runCtx = cancelOnPublish{ctx, filepath.Join(dest, p.Items[0].Name), cancel}
			}
			// 第一个素材已进入暂存时，操作另一个尚未复制的素材的后缀副本。
			source := p.Items[len(p.Items)-1].Source()
			_, err = Execute(runCtx, p, func(pr Progress) {
				if pr.Done != 1 || pr.BytesDone != p.Items[0].Size {
					return
				}
				switch scenario {
				case "cancel staging":
					cancel()
				case "changed copy source":
					write(t, source, "changed source content")
				case "missing copy source":
					if err := os.Remove(source); err != nil {
						t.Fatal(err)
					}
				}
			})
			if strings.HasPrefix(scenario, "cancel") {
				if !errors.Is(err, ErrCancelled) {
					t.Fatalf("cancel error = %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), filepath.Base(source)) {
				t.Fatalf("source error does not name original: %v", err)
			}
			after, inventoryErr := inventory(dest)
			if inventoryErr != nil || !sameInventory(before, after) || len(Leftovers(f.output)) != 0 {
				t.Fatalf("destination changed or workspace left: %v %v", names(dest), inventoryErr)
			}
			for name, want := range map[string]string{"old.mp4": "old video", "notes.txt": "notes"} {
				b, err := os.ReadFile(filepath.Join(dest, name))
				if err != nil || string(b) != want {
					t.Fatalf("%s was not restored: %q %v", name, b, err)
				}
			}
		})
	}
}

func TestRenamedSourceErrorsUseOriginal(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(fmt.Sprint(missing), func(t *testing.T) {
			f := newFixture(t)
			p, err := Prepare(f.material, f.output, "第一批", map[string]Pick{"结尾": {Count: 1, Repeat: 2}}, "", false, rand.New(rand.NewPCG(1, 2)))
			if err != nil {
				t.Fatal(err)
			}
			copy := p.Items[1]
			p.Items = p.Items[1:]
			if missing {
				if err := os.Remove(copy.Source()); err != nil {
					t.Fatal(err)
				}
			} else {
				write(t, copy.Source(), "different source")
			}
			_, err = Execute(context.Background(), p, nil)
			if err == nil || !strings.Contains(err.Error(), copy.Origin) || strings.Contains(err.Error(), copy.Name) {
				t.Fatalf("error = %v, original=%s copy=%s", err, copy.Origin, copy.Name)
			}
		})
	}
}

func TestRenamedSourceChangedDuringCopyUsesOriginal(t *testing.T) {
	f := newFixture(t)
	p, err := Prepare(f.material, f.output, "第一批", map[string]Pick{"结尾": {Count: 1, Repeat: 2}}, "", false, rng())
	if err != nil {
		t.Fatal(err)
	}
	copy := p.Items[1]
	_, err = Execute(context.Background(), p, func(pr Progress) {
		if pr.Done == 1 && pr.BytesDone == p.Items[0].Size {
			write(t, copy.Source(), "changed during renamed copy")
		}
	})
	if err == nil || !strings.Contains(err.Error(), copy.Origin) || strings.Contains(err.Error(), copy.Name) {
		t.Fatalf("copy error names renamed destination: %v", err)
	}
	if len(names(p.Destination)) != 0 || len(Leftovers(f.output)) != 0 {
		t.Fatal("changed source left published files or workspace")
	}
}
