package picker

import (
	"math/rand/v2"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Item 是一个被抽中的视频。
type Item struct {
	Category string `json:"category"`
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	source   string
	mod      time.Time
}

// Plan 是一次抽取的结果，确认后交给 Execute 复制。
type Plan struct {
	MaterialRoot string   `json:"materialRoot"`
	OutputRoot   string   `json:"outputRoot"`
	Folder       string   `json:"folder"`
	Destination  string   `json:"destination"`
	Clean        bool     `json:"clean"`
	Items        []Item   `json:"items"`
	Old          []string `json:"old"`     // 复制前要清理的旧视频
	Bytes        int64    `json:"bytes"`   // 本次复制的总大小
	Avoided      int      `json:"avoided"` // 因同名而避开的视频数
}

// Prepare 按数量从各分类随机抽取视频。
//
// 同一批视频要以原名放进同一个文件夹，所以会自动避开：与其他分类已抽中视频同名的、
// 与目标文件夹里会保留下来的文件同名的。可用视频不够时报错，不会覆盖任何文件。
func Prepare(materialRoot, outputRoot, folder string, counts map[string]int, clean bool, r *rand.Rand) (Plan, error) {
	if err := CheckSeparate(materialRoot, outputRoot); err != nil {
		return Plan{}, err
	}
	dest, err := ResolveDestination(outputRoot, folder)
	if err != nil {
		return Plan{}, err
	}
	cats, files, err := scan(materialRoot)
	if err != nil {
		return Plan{}, err
	}
	exists := map[string]bool{}
	for _, c := range cats {
		exists[c.Name] = true
	}
	for name, n := range counts {
		if n < 0 {
			return Plan{}, userErr("「%s」的数量不能小于 0", name)
		}
		if n > 0 && !exists[name] {
			return Plan{}, userErr("分类「%s」已经不存在了，请检查素材根目录", name)
		}
	}

	p := Plan{MaterialRoot: materialRoot, OutputRoot: outputRoot, Folder: folder, Destination: dest, Clean: clean}
	if clean {
		old, err := listVideos(dest)
		if err != nil {
			return Plan{}, userErr("无法读取目标文件夹：%v", err)
		}
		for _, v := range old {
			p.Old = append(p.Old, v.name)
		}
	}
	removable := map[string]bool{}
	for _, n := range p.Old {
		removable[fold(n)] = true
	}
	entries, err := os.ReadDir(dest)
	if err != nil {
		return Plan{}, userErr("无法读取目标文件夹：%v", err)
	}
	taken := map[string]bool{}
	for _, e := range entries {
		if !removable[fold(e.Name())] {
			taken[fold(e.Name())] = true
		}
	}

	names := make([]string, 0, len(counts))
	for name, n := range counts {
		if n > 0 {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		want := counts[name]
		all := files[name]
		if want > len(all) {
			return Plan{}, userErr("「%s」只有 %d 个视频，不能抽取 %d 个", name, len(all), want)
		}
		var pool []video
		for _, v := range all {
			if !taken[fold(v.name)] {
				pool = append(pool, v)
			}
		}
		if want > len(pool) {
			return Plan{}, userErr("「%s」有 %d 个视频与目标文件夹或其他分类抽中的视频同名，最多还能抽 %d 个。可以换一个文件夹，或勾选清空旧视频",
				name, len(all)-len(pool), len(pool))
		}
		p.Avoided += len(all) - len(pool)
		r.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
		for _, v := range pool[:want] {
			taken[fold(v.name)] = true
			p.Items = append(p.Items, Item{Category: name, Name: v.name, Size: v.size, source: v.path, mod: v.mod})
			p.Bytes += v.size
		}
	}
	if len(p.Items) == 0 {
		return Plan{}, userErr("请至少为一个分类填写大于 0 的数量")
	}
	return p, nil
}

func (it Item) Source() string { return it.source }

// sourceUnder 确认素材仍在素材根目录内。
func sourceUnder(root, path string) bool {
	return within(absClean(root), absClean(filepath.Dir(path)))
}
