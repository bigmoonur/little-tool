package picker

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type ImportSkip struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

type ImportResult struct {
	Category string       `json:"category"`
	Added    []string     `json:"added"`
	Skipped  []ImportSkip `json:"skipped"`
	Bytes    int64        `json:"bytes"`
}

var ErrImportCancelled = errors.New("已取消，这次没有添加任何视频")

// Import 只复制原文件，失败时撤销本次创建的文件。
func Import(ctx context.Context, root, category string, paths []string, progress func(Progress)) (res ImportResult, err error) {
	res = ImportResult{Category: category, Added: []string{}, Skipped: []ImportSkip{}}
	var created []string
	defer func() {
		if err == nil {
			return
		}
		for i := len(created) - 1; i >= 0; i-- {
			_ = os.Remove(created[i])
		}
		res.Added, res.Bytes = []string{}, 0
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			err = ErrImportCancelled
		} else {
			var ue *UserError
			if !errors.As(err, &ue) {
				err = userErr("添加失败，已撤销这次添加：%v", err)
			}
		}
	}()
	dir, err := CategoryDir(root, category)
	if err != nil {
		return res, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return res, err
	}
	names := make([]string, 0, len(entries)+len(paths))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	var accepted []video
	var total int64
	for _, path := range paths {
		name := filepath.Base(path)
		fi, statErr := os.Lstat(path)
		reason := ""
		switch {
		case statErr != nil || !isPlainFile(fi):
			reason = "不是文件"
		case !isVideo(name):
			reason = "不是支持的视频格式"
		case strings.EqualFold(absClean(filepath.Dir(path)), absClean(dir)):
			reason = "已经在这个分类里"
		default:
			for _, existing := range names {
				if strings.EqualFold(name, existing) {
					reason = "分类里已有同名文件"
					break
				}
			}
		}
		if reason != "" {
			res.Skipped = append(res.Skipped, ImportSkip{Name: name, Reason: reason})
			continue
		}
		accepted = append(accepted, video{name: name, path: path, size: fi.Size(), mod: fi.ModTime()})
		names = append(names, name)
		total += fi.Size()
	}
	if len(accepted) == 0 {
		return res, nil
	}
	if free, ok := freeSpace(dir); ok && free < uint64(total) {
		return res, userErr("素材磁盘剩余空间不足：需要 %s，只剩 %s", HumanBytes(total), HumanBytes(int64(free)))
	}
	var done int64
	for i, v := range accepted {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		before, err := os.Lstat(v.path)
		if err != nil {
			return res, err
		}
		if !isPlainFile(before) || before.Size() != v.size || !before.ModTime().Equal(v.mod) {
			return res, userErr("复制期间文件发生了变化，请重试：%s", v.name)
		}
		tmp, err := os.CreateTemp(dir, TempPrefix+"*.part")
		if err != nil {
			return res, err
		}
		created = append(created, tmp.Name())
		if err := tmp.Close(); err != nil {
			return res, err
		}
		hide(tmp.Name())
		report := func(n int64) {
			done += n
			if progress != nil {
				progress(Progress{Done: i, Total: len(accepted), Name: v.name, BytesDone: done, BytesTotal: total})
			}
		}
		report(0)
		if err := copyFile(ctx, v.path, tmp.Name(), os.O_WRONLY|os.O_TRUNC, report); err != nil {
			return res, err
		}
		if err := ctx.Err(); err != nil {
			return res, err
		}
		after, err := os.Lstat(v.path)
		st, statErr := os.Stat(tmp.Name())
		if err != nil || statErr != nil || !isPlainFile(after) || after.Size() != v.size || !after.ModTime().Equal(v.mod) || st.Size() != v.size {
			return res, userErr("复制期间文件发生了变化，请重试：%s", v.name)
		}
		if err := ctx.Err(); err != nil {
			return res, err
		}
		// 再查一次大小写冲突，外部程序可能在复制期间添加文件。
		current, err := os.ReadDir(dir)
		if err != nil {
			return res, err
		}
		for _, e := range current {
			if strings.EqualFold(e.Name(), v.name) {
				return res, userErr("分类里刚出现了同名文件：%s", v.name)
			}
		}
		// 硬链接会共享隐藏属性，发布前先恢复为可见文件。
		unhide(tmp.Name())
		target := filepath.Join(dir, v.name)
		if err := os.Link(tmp.Name(), target); err != nil {
			if errors.Is(err, fs.ErrExist) {
				return res, userErr("分类里刚出现了同名文件：%s", v.name)
			}
			// 独占创建成功后才记录归属，回滚不会删除其他程序的文件。
			out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
			if errors.Is(err, fs.ErrExist) {
				return res, userErr("分类里刚出现了同名文件：%s", v.name)
			}
			if err != nil {
				return res, err
			}
			created = append(created, target)
			if err := out.Close(); err != nil {
				return res, err
			}
			if err := copyFile(ctx, tmp.Name(), target, os.O_WRONLY|os.O_TRUNC, nil); err != nil {
				return res, err
			}
		} else {
			created = append(created, target)
		}
		if err := os.Remove(tmp.Name()); err != nil {
			return res, err
		}
		res.Added = append(res.Added, v.name)
		res.Bytes += v.size
	}
	if err := ctx.Err(); err != nil {
		return res, err
	}
	if progress != nil {
		progress(Progress{Done: len(accepted), Total: len(accepted), BytesDone: total, BytesTotal: total})
	}
	if err := ctx.Err(); err != nil {
		return res, err
	}
	return res, nil
}
