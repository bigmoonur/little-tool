package library

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
)

const (
	RepeatConsecutive = "consecutive"
	RepeatCycle       = "cycle"
	RepeatRandom      = "random"
)

func validRepeatMode(mode string) bool {
	return mode == "" || mode == RepeatConsecutive || mode == RepeatCycle || mode == RepeatRandom
}

type PickRequest struct {
	ListID string   `json:"listId"`
	Count  int      `json:"count"`
	Repeat int      `json:"repeat"`
	Mode   string   `json:"mode,omitempty"`
	TagIDs []string `json:"tagIds,omitempty"`
}

type PickPart struct {
	ListID string   `json:"listId"`
	Name   string   `json:"name"`
	Count  int      `json:"count"`
	Repeat int      `json:"repeat"`
	Mode   string   `json:"mode,omitempty"`
	TagIDs []string `json:"tagIds,omitempty"`
}

type ExtractResult struct {
	Text  string     `json:"text"`
	Total int        `json:"total"`
	Parts []PickPart `json:"parts"`
}

// Extract 按标签筛选后抽取。连续重复保留旧版行为；循环重复重用同一批；分轮随机每轮独立抽样。
func Extract(lists []List, reqs []PickRequest, r *rand.Rand) (ExtractResult, error) {
	byID := make(map[string]*List, len(lists))
	for i := range lists {
		byID[lists[i].ID] = &lists[i]
	}
	type job struct {
		list *List
		req  PickRequest
		pool []string
	}
	var jobs []job
	total := 0
	mode := ""
	for _, req := range reqs {
		if req.Count == 0 {
			continue
		}
		l, ok := byID[req.ListID]
		if !ok {
			return ExtractResult{}, userErr("有分类已被删除，请检查后重试")
		}
		if !validRepeatMode(req.Mode) {
			return ExtractResult{}, userErr("不支持的乘数规则")
		}
		if req.Mode == "" {
			req.Mode = RepeatConsecutive
		}
		if mode != "" && mode != req.Mode {
			return ExtractResult{}, userErr("一次抽取请使用相同的乘数规则")
		}
		mode = req.Mode
		if req.Count < 0 {
			return ExtractResult{}, userErr("「%s」的数量不能小于 0", l.Name)
		}
		if req.Repeat < 1 {
			return ExtractResult{}, userErr("「%s」的乘数至少为 1", l.Name)
		}
		pool := matchingItems(*l, req.TagIDs)
		if req.Repeat > 1 || mode == RepeatRandom {
			pool = dedupe(pool)
		}
		if req.Count > len(pool) {
			what := "文本"
			if req.Repeat > 1 || mode == RepeatRandom {
				what = "不同内容"
			}
			return ExtractResult{}, userErr("「%s」当前最多可抽取 %s 条%s", l.Name, thousands(len(pool)), what)
		}
		// 先除后乘，避免来自绑定的极大整数溢出。
		if req.Repeat > MaxOutputLines || req.Count > MaxOutputLines/req.Repeat {
			return ExtractResult{}, userErr("一次最多输出 %s 条，请减小数量或乘数", thousands(MaxOutputLines))
		}
		total += req.Count * req.Repeat
		if total > MaxOutputLines {
			return ExtractResult{}, userErr("合计最多输出 %s 条，请减小数量或乘数", thousands(MaxOutputLines))
		}
		jobs = append(jobs, job{l, req, pool})
	}
	if total == 0 {
		return ExtractResult{}, userErr("请至少为一个分类填写数量")
	}

	parts := make([]PickPart, 0, len(jobs))
	for _, j := range jobs {
		parts = append(parts, PickPart{ListID: j.list.ID, Name: j.list.Name, Count: j.req.Count, Repeat: j.req.Repeat, Mode: mode, TagIDs: slices.Clone(j.req.TagIDs)})
	}
	type selected struct {
		line   string
		repeat int
	}
	base := []selected{}
	if mode != RepeatRandom {
		for _, j := range jobs {
			for _, line := range sample(j.pool, j.req.Count, r) {
				base = append(base, selected{line, j.req.Repeat})
			}
		}
		r.Shuffle(len(base), func(a, b int) { base[a], base[b] = base[b], base[a] })
	}
	lines := make([]string, 0, total)
	switch mode {
	case RepeatConsecutive:
		for _, item := range base {
			for range item.repeat {
				lines = append(lines, item.line)
			}
		}
	case RepeatCycle:
		for round := 0; len(base) > 0; round++ {
			next := base[:0]
			for _, item := range base {
				lines = append(lines, item.line)
				if round+1 < item.repeat {
					next = append(next, item)
				}
			}
			base = next
		}
	case RepeatRandom:
		for round := 0; len(jobs) > 0; round++ {
			batch := []string{}
			next := jobs[:0]
			for _, j := range jobs {
				batch = append(batch, sample(j.pool, j.req.Count, r)...)
				if round+1 < j.req.Repeat {
					next = append(next, j)
				}
			}
			jobs = next
			r.Shuffle(len(batch), func(a, b int) { batch[a], batch[b] = batch[b], batch[a] })
			lines = append(lines, batch...)
		}
	}
	return ExtractResult{Text: strings.Join(lines, "\n"), Total: total, Parts: parts}, nil
}

// sample 不放回地随机取 n 个元素。
func sample(items []string, n int, r *rand.Rand) []string {
	// 小样本用稀疏交换表，分轮重抽无需每轮复制整个素材池。
	if n < len(items)/4 {
		swaps := make(map[int]int, n)
		at := func(i int) int {
			if v, ok := swaps[i]; ok {
				return v
			}
			return i
		}
		out := make([]string, n)
		for i := range n {
			j := i + r.IntN(len(items)-i)
			picked := at(j)
			swaps[j] = at(i)
			delete(swaps, i)
			out[i] = items[picked]
		}
		return out
	}
	idx := make([]int, len(items))
	for i := range idx {
		idx[i] = i
	}
	out := make([]string, n)
	for i := 0; i < n; i++ {
		j := i + r.IntN(len(idx)-i)
		idx[i], idx[j] = idx[j], idx[i]
		out[i] = items[idx[i]]
	}
	return out
}

func dedupe(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, it := range items {
		if _, ok := seen[it]; ok {
			continue
		}
		seen[it] = struct{}{}
		out = append(out, it)
	}
	return out
}

func thousands(n int) string {
	s := fmt.Sprint(n)
	if n < 0 {
		return "-" + thousands(-n)
	}
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// UserError 是可以直接展示给用户的错误。
type UserError struct{ Msg string }

func (e *UserError) Error() string { return e.Msg }

func userErr(format string, args ...any) error {
	return &UserError{Msg: fmt.Sprintf(format, args...)}
}
