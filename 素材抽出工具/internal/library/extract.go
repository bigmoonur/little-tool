package library

import (
	"fmt"
	"math/rand/v2"
	"strings"
)

type PickRequest struct {
	ListID string `json:"listId"`
	Count  int    `json:"count"`
	Repeat int    `json:"repeat"`
}

type PickPart struct {
	ListID string `json:"listId"`
	Name   string `json:"name"`
	Count  int    `json:"count"`
	Repeat int    `json:"repeat"`
}

type ExtractResult struct {
	Text  string     `json:"text"`
	Total int        `json:"total"`
	Parts []PickPart `json:"parts"`
}

type group struct {
	line   string
	repeat int
}

// Extract 按请求从各分类抽取素材。
//
// 普通数量按行抽取；重复次数大于 1 时先按内容去重，确保抽中的内容各不相同。
// 所有分类抽中的内容先按组混合打乱，再展开重复次数，同一组的重复行保持连续。
func Extract(lists []List, reqs []PickRequest, r *rand.Rand) (ExtractResult, error) {
	byID := make(map[string]*List, len(lists))
	for i := range lists {
		byID[lists[i].ID] = &lists[i]
	}

	type job struct {
		list *List
		req  PickRequest
	}
	var jobs []job
	total := 0
	for _, req := range reqs {
		if req.Count == 0 {
			continue
		}
		l, ok := byID[req.ListID]
		if !ok {
			return ExtractResult{}, userErr("有分类已被删除，请检查后重试")
		}
		if req.Count < 0 {
			return ExtractResult{}, userErr("「%s」的数量不能小于 0", l.Name)
		}
		if req.Repeat < 1 {
			return ExtractResult{}, userErr("「%s」的重复次数至少为 1", l.Name)
		}
		if req.Repeat > 1 {
			if req.Count > l.unique {
				return ExtractResult{}, userErr("「%s」最多可抽取 %s 条不同内容", l.Name, thousands(l.unique))
			}
		} else if req.Count > len(l.Items) {
			return ExtractResult{}, userErr("「%s」最多可抽取 %s 条", l.Name, thousands(len(l.Items)))
		}
		if req.Repeat > MaxOutputLines || req.Count*req.Repeat > MaxOutputLines {
			return ExtractResult{}, userErr("一次最多输出 %s 条，请减小数量或重复次数", thousands(MaxOutputLines))
		}
		total += req.Count * req.Repeat
		if total > MaxOutputLines {
			return ExtractResult{}, userErr("合计最多输出 %s 条，请减小数量或重复次数", thousands(MaxOutputLines))
		}
		jobs = append(jobs, job{l, req})
	}
	if total == 0 {
		return ExtractResult{}, userErr("请至少为一个分类填写数量")
	}

	var groups []group
	parts := make([]PickPart, 0, len(jobs))
	for _, j := range jobs {
		pool := j.list.Items
		if j.req.Repeat > 1 {
			pool = dedupe(pool)
		}
		for _, line := range sample(pool, j.req.Count, r) {
			groups = append(groups, group{line, j.req.Repeat})
		}
		parts = append(parts, PickPart{
			ListID: j.list.ID,
			Name:   j.list.Name,
			Count:  j.req.Count,
			Repeat: j.req.Repeat,
		})
	}

	r.Shuffle(len(groups), func(a, b int) { groups[a], groups[b] = groups[b], groups[a] })

	var sb strings.Builder
	for i, g := range groups {
		for k := 0; k < g.repeat; k++ {
			if i > 0 || k > 0 {
				sb.WriteByte('\n')
			}
			sb.WriteString(g.line)
		}
	}
	return ExtractResult{Text: sb.String(), Total: total, Parts: parts}, nil
}

// sample 不放回地随机取 n 个元素。
func sample(items []string, n int, r *rand.Rand) []string {
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
