package library

import "slices"

const (
	ImportNew     = "new"     // 新建分类
	ImportAppend  = "append"  // 追加到已有分类
	ImportReplace = "replace" // 替换已有分类的全部内容
)

// ImportItem 是一份待添加的文本（一个 TXT 文件或一段粘贴内容）及其去向。
type ImportItem struct {
	Source   string `json:"source"`
	Text     string `json:"text"`
	Mode     string `json:"mode"`
	TargetID string `json:"targetId"`
	NewName  string `json:"newName"`
	GroupID  string `json:"groupId"`
}

type ImportPlan struct {
	Items  []ImportItem `json:"items"`
	Dedupe bool         `json:"dedupe"`
}

type ImportStat struct {
	Source     string `json:"source"`
	Target     string `json:"target"`
	Mode       string `json:"mode"`
	Lines      int    `json:"lines"`
	Added      int    `json:"added"`
	Duplicates int    `json:"duplicates"`
	Removed    int    `json:"removed"`
	Error      string `json:"error"`
}

type ImportReport struct {
	Items      []ImportStat `json:"items"`
	Added      int          `json:"added"`
	Duplicates int          `json:"duplicates"`
	Removed    int          `json:"removed"`
	Created    int          `json:"created"`
	Touched    int          `json:"touched"`
	OK         bool         `json:"ok"`
	Error      string       `json:"error"`
}

// planImport 在 d.Lists 的副本上模拟导入，返回导入后的分类列表和统计。
// 原有分类的 Items 不会被修改，因此同一函数既用于预览也用于实际导入。
func planImport(d *Data, plan ImportPlan, now int64) ([]List, ImportReport) {
	lists := slices.Clone(d.Lists)
	rep := ImportReport{Items: make([]ImportStat, 0, len(plan.Items))}
	groupOK := map[string]bool{"": true}
	for _, g := range d.Groups {
		groupOK[g.ID] = true
	}
	touched := map[string]bool{}

	findByID := func(id string) int {
		for i := range lists {
			if lists[i].ID == id {
				return i
			}
		}
		return -1
	}
	nameTaken := func(name string) bool {
		for i := range lists {
			if sameName(lists[i].Name, name) {
				return true
			}
		}
		return false
	}

	hasError := false
	for _, it := range plan.Items {
		st := ImportStat{Source: it.Source, Mode: it.Mode}
		incoming, _ := SplitLines(it.Text)
		st.Lines = len(incoming)

		fail := func(err error) {
			st.Error = err.Error()
			hasError = true
		}

		switch it.Mode {
		case ImportNew:
			name, err := CleanName(it.NewName, "分类")
			st.Target = name
			switch {
			case err != nil:
				fail(err)
			case nameTaken(name):
				fail(userErr("已有同名分类「%s」", name))
			case !groupOK[it.GroupID]:
				fail(userErr("所选分组已被删除"))
			}
			if st.Error != "" {
				break
			}
			items := incoming
			if plan.Dedupe {
				items = dedupe(incoming)
			}
			st.Added = len(items)
			st.Duplicates = len(incoming) - len(items)
			l := List{ID: NewID(), Name: name, GroupID: it.GroupID, UpdatedAt: now}
			l.setItems(items)
			lists = append(lists, l)
			rep.Created++
			touched[l.ID] = true

		case ImportAppend, ImportReplace:
			i := findByID(it.TargetID)
			if i < 0 {
				fail(userErr("目标分类不存在，可能已被删除"))
				break
			}
			l := lists[i]
			st.Target = l.Name
			if st.Lines == 0 {
				// 没有内容时不改动分类，替换模式下也不会清空原有内容。
				break
			}
			var base []string
			if it.Mode == ImportReplace {
				st.Removed = len(l.Items)
				base = []string{}
			} else {
				base = slices.Clip(l.Items)
			}
			added := incoming
			if plan.Dedupe {
				seen := make(map[string]struct{}, len(base)+len(incoming))
				for _, s := range base {
					seen[s] = struct{}{}
				}
				added = make([]string, 0, len(incoming))
				for _, s := range incoming {
					if _, ok := seen[s]; ok {
						continue
					}
					seen[s] = struct{}{}
					added = append(added, s)
				}
			}
			st.Added = len(added)
			st.Duplicates = len(incoming) - len(added)
			l.setItems(append(base, added...))
			l.UpdatedAt = max(l.UpdatedAt+1, now)
			lists[i] = l
			touched[l.ID] = true

		default:
			fail(userErr("未知的添加方式"))
		}

		rep.Added += st.Added
		rep.Duplicates += st.Duplicates
		rep.Removed += st.Removed
		rep.Items = append(rep.Items, st)
	}

	rep.Touched = len(touched)
	switch {
	case len(plan.Items) == 0:
		rep.Error = "没有要添加的内容"
	case hasError:
		rep.Error = "部分内容需要调整后才能添加"
	case rep.Added == 0 && rep.Created == 0 && rep.Removed == 0:
		if rep.Duplicates > 0 {
			rep.Error = "所有内容都已存在，没有新内容可添加"
		} else {
			rep.Error = "没有可添加的内容"
		}
	default:
		rep.OK = true
	}
	return lists, rep
}
