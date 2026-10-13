package picker

import (
	"bytes"
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

func tagStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func createTag(t *testing.T, s *Store, name string) string {
	t.Helper()
	id, err := s.CreateTag(name)
	if err != nil || id == "" {
		t.Fatalf("CreateTag(%q) = %q, %v", name, id, err)
	}
	return id
}

func tagError(t *testing.T, err error, want string) {
	t.Helper()
	var user *UserError
	if !errors.As(err, &user) || err.Error() != want {
		t.Fatalf("error = %v, want UserError %q", err, want)
	}
}

func TestTagNames(t *testing.T) {
	s := tagStore(t)
	id := createTag(t, s, " \tProduct\n\u3000 Video  ")
	other := createTag(t, s, strings.Repeat("签", 40))
	if got := s.Tags()[0].Name; got != "Product Video" {
		t.Fatalf("normalized name = %q", got)
	}
	for _, tc := range []struct {
		name, message string
	}{
		{"", "请填写标签名称"},
		{" \t\n\u3000", "请填写标签名称"},
		{strings.Repeat("签", 41), "标签名称最多 40 个字"},
		{" product \tVIDEO ", "已有同名标签「Product Video」"},
	} {
		t.Run(tc.message, func(t *testing.T) {
			before := s.Tags()
			gotID, err := s.CreateTag(tc.name)
			tagError(t, err, tc.message)
			if gotID != "" {
				t.Fatalf("rejected create returned ID %q", gotID)
			}
			tagError(t, s.RenameTag(other, tc.name), tc.message)
			if !reflect.DeepEqual(before, s.Tags()) {
				t.Fatal("invalid name changed tags")
			}
		})
	}
	if err := s.RenameTag(id, "  PRODUCT\tvideo  "); err != nil {
		t.Fatal(err)
	}
	if got := s.Tags()[0]; got.ID != id || got.Name != "PRODUCT video" {
		t.Fatalf("case-only rename = %+v", got)
	}
	if err := s.RenameTag(other, " \n新的  标签\t"); err != nil {
		t.Fatal(err)
	}
	if got := s.Tags()[1].Name; got != "新的 标签" {
		t.Fatalf("renamed name = %q", got)
	}
	for _, call := range []func() error{
		func() error { return s.RenameTag("missing", "名称") },
		func() error { return s.SetTagColor("missing", "rose") },
		func() error { return s.DeleteTag("missing") },
	} {
		tagError(t, call(), "标签不存在，可能已被删除")
	}
}

func TestTagColors(t *testing.T) {
	s := tagStore(t)
	for i := 0; i < len(TagColors)*2; i++ {
		createTag(t, s, fmt.Sprintf("标签 %d", i))
		if got := s.Tags()[i].Color; got != TagColors[i%len(TagColors)] {
			t.Fatalf("tag %d color = %q", i, got)
		}
	}
	id := s.Tags()[0].ID
	if err := s.SetTagColor(id, "teal"); err != nil {
		t.Fatal(err)
	}
	createTag(t, s, "补足最少的颜色")
	if got := s.Tags()[len(TagColors)*2].Color; got != "indigo" {
		t.Fatalf("least-used color = %q", got)
	}
	for _, color := range TagColors {
		if err := s.SetTagColor(id, color); err != nil {
			t.Fatal(err)
		}
		if s.Tags()[0].Color != color {
			t.Fatalf("color %q was not set", color)
		}
	}
	before := s.Tags()
	for _, color := range []string{"", "blue", "INDIGO", " rose "} {
		tagError(t, s.SetTagColor(id, color), "不支持的颜色")
		if !reflect.DeepEqual(before, s.Tags()) {
			t.Fatal("invalid color changed tags")
		}
	}
}

func TestCategoryTagsOrderAndRejection(t *testing.T) {
	s := tagStore(t)
	a, b, c := createTag(t, s, "一"), createTag(t, s, "二"), createTag(t, s, "三")
	input := []string{c, a, c, b, a}
	if err := s.SetCategoryTags("Category", input); err != nil {
		t.Fatal(err)
	}
	input[0] = "changed"
	want := []string{a, b, c}
	if got := s.CategoryTags("Category"); !reflect.DeepEqual(got, want) {
		t.Fatalf("ordered IDs = %v", got)
	}
	if err := s.SetCategoryTags("category", []string{b}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetCategoryTags(" Category ", []string{c}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s.CategoryTags("category"), []string{b}) || !reflect.DeepEqual(s.CategoryTags(" Category "), []string{c}) {
		t.Fatal("category folder names were folded or trimmed")
	}
	before, err := os.ReadFile(filepath.Join(s.Dir(), "tags.json"))
	if err != nil {
		t.Fatal(err)
	}
	tagError(t, s.SetCategoryTags("", []string{a}), "分类不存在")
	tagError(t, s.SetCategoryTags("Category", []string{b, "missing", a}), "所选标签已被删除，请重新选择")
	tagError(t, s.SetCategoryTags("new category", []string{a, "missing"}), "所选标签已被删除，请重新选择")
	after, err := os.ReadFile(filepath.Join(s.Dir(), "tags.json"))
	if err != nil || !bytes.Equal(before, after) || !reflect.DeepEqual(s.CategoryTags("Category"), want) {
		t.Fatalf("rejected assignment changed state: %v", err)
	}
	for _, empty := range [][]string{nil, {}} {
		if err := s.SetCategoryTags("Category", empty); err != nil {
			t.Fatal(err)
		}
		if got := s.CategoryTags("Category"); got == nil || len(got) != 0 {
			t.Fatalf("cleared assignment = %#v", got)
		}
		var data tagData
		if !readJSON(filepath.Join(s.Dir(), "tags.json"), &data) {
			t.Fatal("cannot read persisted tags")
		}
		if _, ok := data.Categories["Category"]; ok {
			t.Fatal("empty assignment was persisted")
		}
	}
}

func TestTagPersistenceAndDelete(t *testing.T) {
	s := tagStore(t)
	a, b := createTag(t, s, "一"), createTag(t, s, "二")
	if err := s.RenameTag(a, "重新命名"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTagColor(a, "rose"); err != nil {
		t.Fatal(err)
	}
	for name, ids := range map[string][]string{"未连接的分类": {b, a}, "仅一个标签": {a}, "保留标签": {b}} {
		if err := s.SetCategoryTags(name, ids); err != nil {
			t.Fatal(err)
		}
	}
	reopened, err := OpenStore(s.Dir())
	if err != nil || !reflect.DeepEqual(s.Tags(), reopened.Tags()) || !reflect.DeepEqual(reopened.CategoryTags("未连接的分类"), []string{a, b}) {
		t.Fatalf("reopened store lost tags: %v", err)
	}
	if err := reopened.DeleteTag(a); err != nil {
		t.Fatal(err)
	}
	if got := reopened.Tags(); len(got) != 1 || got[0].ID != b {
		t.Fatalf("tags after delete = %+v", got)
	}
	for _, name := range []string{"未连接的分类", "保留标签"} {
		if !reflect.DeepEqual(reopened.CategoryTags(name), []string{b}) {
			t.Fatalf("delete changed surviving assignment for %q", name)
		}
	}
	if got := reopened.CategoryTags("仅一个标签"); got == nil || len(got) != 0 {
		t.Fatalf("deleted ID still assigned: %#v", got)
	}
	var data tagData
	if !readJSON(filepath.Join(s.Dir(), "tags.json"), &data) {
		t.Fatal("cannot read persisted delete")
	}
	if _, ok := data.Categories["仅一个标签"]; ok {
		t.Fatal("delete left an empty category entry")
	}
	final, err := OpenStore(s.Dir())
	if err != nil || !reflect.DeepEqual(reopened.Tags(), final.Tags()) || !reflect.DeepEqual(final.CategoryTags("未连接的分类"), []string{b}) {
		t.Fatalf("delete was not persisted: %v", err)
	}
}

func TestLoadMissingCorruptAndNilTags(t *testing.T) {
	for name, content := range map[string]string{
		"missing": "", "corrupt": "{", "null": "null", "nil fields": `{"tags":null,"categories":null}`,
		"wrong type": `{"tags":[{"id":"partial","color":"rose"}],"categories":[]}`,
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if content != "" {
				write(t, filepath.Join(dir, "tags.json"), content)
			}
			s, err := OpenStore(dir)
			if err != nil {
				t.Fatal(err)
			}
			if got := s.Tags(); got == nil || len(got) != 0 {
				t.Fatalf("Tags() = %#v", got)
			}
			if got := s.CategoryTags("offline"); got == nil || len(got) != 0 {
				t.Fatalf("CategoryTags() = %#v", got)
			}
			id := createTag(t, s, "恢复可用")
			if err := s.SetCategoryTags("offline", []string{id}); err != nil {
				t.Fatal(err)
			}
			reopened, err := OpenStore(dir)
			if err != nil || !reflect.DeepEqual(s.Tags(), reopened.Tags()) || !reflect.DeepEqual(reopened.CategoryTags("offline"), []string{id}) {
				t.Fatalf("could not recover tags file: %v", err)
			}
		})
	}
}

func TestLoadOddTags(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "tags.json"), `{
		"tags":[
			{"id":"b","name":"first","color":"invalid"},
			{"id":"","name":"no ID","color":""},
			{"id":"b","name":"duplicate","color":"rose"},
			{"id":"a","name":"last","color":"violet"},
			{"id":"c","name":"fallback","color":"unknown"}
		],
		"categories":{"Offline":["a","missing","b","b","c",""],"offline":["a"],"nil":null,"unknown":["missing"]}
	}`)
	s, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	tags := s.Tags()
	if len(tags) != 4 || tags[0] != (Tag{ID: "b", Name: "first", Color: "indigo"}) || tags[1].ID == "" || tags[1].Color != "teal" || tags[2].ID != "a" || tags[2].Color != "violet" || tags[3].Color != "violet" {
		t.Fatalf("normalized tags = %+v", tags)
	}
	seen := map[string]bool{}
	for _, tag := range tags {
		if seen[tag.ID] {
			t.Fatalf("duplicate loaded ID %q", tag.ID)
		}
		seen[tag.ID] = true
	}
	if got := s.CategoryTags("Offline"); !reflect.DeepEqual(got, []string{"b", "a", "c"}) {
		t.Fatalf("normalized category IDs = %v", got)
	}
	if got := s.CategoryTags("offline"); !reflect.DeepEqual(got, []string{"a"}) {
		t.Fatalf("case-sensitive category lost = %v", got)
	}
	for _, name := range []string{"nil", "unknown", "missing"} {
		if got := s.CategoryTags(name); got == nil || len(got) != 0 {
			t.Fatalf("%q IDs = %#v", name, got)
		}
	}
	if err := s.SetCategoryTags("generated", []string{tags[1].ID}); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenStore(dir)
	if err != nil || !reflect.DeepEqual(tags, reopened.Tags()) || !reflect.DeepEqual(reopened.CategoryTags("generated"), []string{tags[1].ID}) {
		t.Fatalf("normalized tags were not persisted: %v", err)
	}
}

func TestTagSlicesDoNotAlias(t *testing.T) {
	s := tagStore(t)
	a, b := createTag(t, s, "一"), createTag(t, s, "二")
	if err := s.SetCategoryTags("分类", []string{b, a}); err != nil {
		t.Fatal(err)
	}
	tags, ids := s.Tags(), s.CategoryTags("分类")
	tags[0] = Tag{ID: "changed", Name: "changed", Color: "rose"}
	ids[0] = "changed"
	if s.Tags()[0].ID != a || !reflect.DeepEqual(s.CategoryTags("分类"), []string{a, b}) {
		t.Fatal("returned slices alias the store")
	}
	if err := s.RenameTag(a, "新名称"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetCategoryTags("分类", []string{b}); err != nil {
		t.Fatal(err)
	}
	if tags[1].ID != b || !reflect.DeepEqual(ids, []string{"changed", b}) {
		t.Fatal("store updates changed a previous snapshot")
	}
}

func TestTagSaveFailureKeepsState(t *testing.T) {
	s := tagStore(t)
	a, b := createTag(t, s, "一"), createTag(t, s, "二")
	if err := s.SetCategoryTags("分类", []string{a, b}); err != nil {
		t.Fatal(err)
	}
	beforeTags, beforeIDs := s.Tags(), s.CategoryTags("分类")
	beforeJSON, err := os.ReadFile(filepath.Join(s.Dir(), "tags.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(s.Dir(), "tags.json.tmp"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, call := range map[string]func() error{
		"create": func() error {
			id, err := s.CreateTag("新标签")
			if id != "" {
				t.Fatalf("failed save returned ID %q", id)
			}
			return err
		},
		"rename": func() error { return s.RenameTag(a, "新名称") },
		"color":  func() error { return s.SetTagColor(a, "rose") },
		"delete": func() error { return s.DeleteTag(a) },
		"assign": func() error { return s.SetCategoryTags("分类", []string{b}) },
	} {
		t.Run(name, func(t *testing.T) {
			err := call()
			var user *UserError
			if !errors.As(err, &user) || !strings.HasPrefix(err.Error(), "无法保存标签：") {
				t.Fatalf("save error = %v", err)
			}
			afterJSON, err := os.ReadFile(filepath.Join(s.Dir(), "tags.json"))
			if err != nil || !bytes.Equal(beforeJSON, afterJSON) || !reflect.DeepEqual(beforeTags, s.Tags()) || !reflect.DeepEqual(beforeIDs, s.CategoryTags("分类")) {
				t.Fatalf("failed save changed state: %v", err)
			}
		})
	}
}

func TestConcurrentTagUpdates(t *testing.T) {
	s := tagStore(t)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Go(func() {
			name := fmt.Sprintf("分类 %d", i)
			id, err := s.CreateTag(name)
			if err != nil {
				t.Error(err)
				return
			}
			if err := s.SetCategoryTags(name, []string{id, id}); err != nil {
				t.Error(err)
			}
			_ = s.Tags()
			if got := s.CategoryTags(name); !reflect.DeepEqual(got, []string{id}) {
				t.Errorf("concurrent assignment = %v", got)
			}
		})
	}
	wg.Wait()
	reopened, err := OpenStore(s.Dir())
	if err != nil || len(reopened.Tags()) != 16 {
		t.Fatalf("concurrent writes lost tags: %v", err)
	}
	for _, tag := range reopened.Tags() {
		if !reflect.DeepEqual(reopened.CategoryTags(tag.Name), []string{tag.ID}) {
			t.Fatalf("concurrent writes lost category %q", tag.Name)
		}
	}
}

func TestScannedCategoryTagsEncodeAsEmptyList(t *testing.T) {
	f := newFixture(t)
	cats, err := ScanCategories(f.material)
	if err != nil {
		t.Fatal(err)
	}
	for _, cat := range cats {
		encoded, err := json.Marshal(cat)
		if err != nil || cat.TagIDs == nil || !bytes.Contains(encoded, []byte(`"tagIds":[]`)) {
			t.Fatalf("scanned category JSON = %s, %v", encoded, err)
		}
	}
}
