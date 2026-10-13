package picker

import (
	"math/rand/v2"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Pick struct {
	Count  int `json:"count"`
	Repeat int `json:"repeat"`
}

const (
	ModeRepeat = "repeat"
	ModeRandom = "random"
)

const MaxRepeat = 99
const MaxItems = 10000

// Item 是一个被抽中的视频。
type Item struct {
	Category string `json:"category"`
	Name     string `json:"name"`
	Origin   string `json:"origin,omitempty"`
	Size     int64  `json:"size"`
	source   string
	mod      time.Time
}

// Plan 是一次抽取的结果，确认后交给 Execute 复制。
type Plan struct {
	MaterialRoot string          `json:"materialRoot"`
	OutputRoot   string          `json:"outputRoot"`
	Folder       string          `json:"folder"`
	Destination  string          `json:"destination"`
	Clean        bool            `json:"clean"`
	Items        []Item          `json:"items"`
	Old          []string        `json:"old"`     // 复制前要清理的旧视频
	Bytes        int64           `json:"bytes"`   // 本次复制的总大小
	Avoided      int             `json:"avoided"` // 因同名而避开的视频数
	Mode         string          `json:"mode"`
	Picks        map[string]Pick `json:"picks"`
}

func pickMode(mode string) (string, error) {
	if mode == "" {
		mode = ModeRepeat
	}
	if mode != ModeRepeat && mode != ModeRandom {
		return "", userErr("不支持的乘数规则")
	}
	return mode, nil
}

func clonePicks(picks map[string]Pick) map[string]Pick {
	out := make(map[string]Pick, len(picks))
	for name, pick := range picks {
		out[name] = pick
	}
	return out
}

// Prepare 按数量和乘数抽取；名称归属源文件，额外副本使用不冲突的后缀。
func Prepare(materialRoot, outputRoot, folder string, picks map[string]Pick, mode string, clean bool, r *rand.Rand) (Plan, error) {
	mode, err := pickMode(mode)
	if err != nil {
		return Plan{}, err
	}
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
	used := map[string]Pick{}
	names := make([]string, 0, len(picks))
	for name := range picks {
		names = append(names, name)
	}
	sort.Strings(names)
	total, rounds := 0, 1
	for _, name := range names {
		pick := picks[name]
		if pick.Count < 0 {
			return Plan{}, userErr("「%s」的数量不能小于 0", name)
		}
		if pick.Count == 0 {
			continue
		}
		if !exists[name] {
			return Plan{}, userErr("分类「%s」已经不存在了，请检查素材根目录", name)
		}
		if pick.Repeat < 0 {
			return Plan{}, userErr("「%s」的乘数至少为 1", name)
		}
		if pick.Repeat == 0 {
			pick.Repeat = 1
		}
		if pick.Repeat > MaxRepeat {
			return Plan{}, userErr("「%s」的乘数最多 %d", name, MaxRepeat)
		}
		// 先除后乘，极大的输入也不能绕过总数限制。
		if pick.Count > (MaxItems-total)/pick.Repeat {
			return Plan{}, userErr("一次最多复制 %d 个文件，请减小数量或乘数", MaxItems)
		}
		total += pick.Count * pick.Repeat
		rounds = max(rounds, pick.Repeat)
		used[name] = pick
	}

	p := Plan{MaterialRoot: materialRoot, OutputRoot: outputRoot, Folder: folder, Destination: dest, Clean: clean, Mode: mode, Picks: used}
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
		removable[n] = true
	}
	entries, err := os.ReadDir(dest)
	if err != nil {
		return Plan{}, userErr("无法读取目标文件夹：%v", err)
	}
	taken := map[string]string{}
	for _, e := range entries {
		if !removable[e.Name()] {
			taken[fold(e.Name())] = ""
		}
	}

	avoided := map[string]bool{}
	picked := map[string]bool{}
	suffixes := map[string]*suffixPool{}
	appendVideo := func(category string, v video) {
		it := Item{Category: category, Name: v.name, Size: v.size, source: v.path, mod: v.mod}
		if picked[v.path] {
			it.Name = copyName(v.name, taken, suffixes, r)
			it.Origin = v.name
		}
		picked[v.path] = true
		taken[fold(it.Name)] = v.path
		p.Items = append(p.Items, it)
		p.Bytes += v.size
	}
	selectVideos := func(name string) ([]video, error) {
		want := used[name].Count
		all := files[name]
		if want > len(all) {
			return nil, userErr("「%s」只有 %d 个视频，不能抽取 %d 个", name, len(all), want)
		}
		var pool []video
		for _, v := range all {
			if owner, ok := taken[fold(v.name)]; !ok || owner == v.path {
				pool = append(pool, v)
			} else {
				avoided[v.path] = true
			}
		}
		r.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
		// 同一分类在大小写敏感的磁盘上也可能存在同名视频。
		unique := make([]video, 0, len(pool))
		seen := map[string]bool{}
		for _, v := range pool {
			if !seen[fold(v.name)] {
				unique = append(unique, v)
				seen[fold(v.name)] = true
			}
		}
		if want > len(unique) {
			return nil, userErr("「%s」有 %d 个视频与目标文件夹或其他分类抽中的视频同名，最多还能抽 %d 个。可以换一个文件夹，或勾选清空旧视频",
				name, len(all)-len(unique), len(unique))
		}
		selected := unique[:want]
		// 先保留本轮原名，避免某个副本后缀占用另一个刚抽中视频的原名。
		for _, v := range selected {
			taken[fold(v.name)] = v.path
		}
		for _, v := range pool {
			if owner, ok := taken[fold(v.name)]; ok && owner != v.path {
				avoided[v.path] = true
			}
		}
		return selected, nil
	}
	if mode == ModeRepeat {
		rounds = 1
	}
	for round := 0; round < rounds; round++ {
		for _, name := range names {
			pick, ok := used[name]
			if !ok || pick.Repeat <= round {
				continue
			}
			selected, err := selectVideos(name)
			if err != nil {
				return Plan{}, err
			}
			for _, v := range selected {
				copies := 1
				if mode == ModeRepeat {
					copies = pick.Repeat
				}
				for copy := 0; copy < copies; copy++ {
					appendVideo(name, v)
				}
			}
		}
	}
	if len(p.Items) == 0 {
		return Plan{}, userErr("请至少为一个分类填写大于 0 的数量")
	}
	p.Avoided = len(avoided)
	return p, nil
}

const suffixChars = "abcdefghijklmnopqrstuvwxyz0123456789"

type suffixPool struct {
	length int
	left   []int
}

func copyName(name string, taken map[string]string, pools map[string]*suffixPool, r *rand.Rand) string {
	key := fold(name)
	pool := pools[key]
	if pool == nil {
		pool = &suffixPool{}
		pools[key] = pool
	}
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for {
		if len(pool.left) == 0 {
			pool.length++
			n := 1
			for i := 0; i < pool.length; i++ {
				n *= len(suffixChars)
			}
			pool.left = make([]int, n)
			for i := range pool.left {
				pool.left[i] = i
			}
		}
		// 无放回地尝试随机后缀，名称耗尽时一定能进入下一长度。
		i := r.IntN(len(pool.left))
		n := pool.left[i]
		pool.left[i] = pool.left[len(pool.left)-1]
		pool.left = pool.left[:len(pool.left)-1]
		suffix := make([]byte, pool.length)
		for j := len(suffix) - 1; j >= 0; j-- {
			suffix[j] = suffixChars[n%len(suffixChars)]
			n /= len(suffixChars)
		}
		candidate := stem + "_" + string(suffix) + ext
		if _, ok := taken[fold(candidate)]; !ok {
			return candidate
		}
	}
}

func (it Item) Source() string { return it.source }

// sourceUnder 确认素材仍在素材根目录内。
func sourceUnder(root, path string) bool {
	return within(absClean(root), absClean(filepath.Dir(path)))
}
