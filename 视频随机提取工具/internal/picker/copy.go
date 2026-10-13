package picker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ErrCancelled 表示用户取消了复制，目标文件夹已恢复原样。
var ErrCancelled = errors.New("已取消，目标文件夹没有任何变化")

// Progress 是复制进度。
type Progress struct {
	Done       int    `json:"done"`
	Total      int    `json:"total"`
	Name       string `json:"name"`
	BytesDone  int64  `json:"bytesDone"`
	BytesTotal int64  `json:"bytesTotal"`
}

// Result 是复制完成后的结果。
type Result struct {
	Destination string          `json:"destination"`
	Folder      string          `json:"folder"`
	Items       []Item          `json:"items"`
	Removed     int             `json:"removed"`
	Avoided     int             `json:"avoided"`
	Warning     string          `json:"warning"`
	Picks       map[string]Pick `json:"picks"`
	Mode        string          `json:"mode"`
}

type entryInfo struct {
	mode fs.FileMode
	size int64
	mod  time.Time
}

// inventory 记录目标文件夹的直接内容，用于发现复制期间被其他程序改动。
func inventory(dir string) (map[string]entryInfo, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make(map[string]entryInfo, len(entries))
	for _, e := range entries {
		fi, err := os.Lstat(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		out[e.Name()] = entryInfo{fi.Mode(), fi.Size(), fi.ModTime()}
	}
	return out, nil
}

func sameInventory(a, b map[string]entryInfo) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		w, ok := b[k]
		if !ok || w.mode != v.mode || w.size != v.size || !w.mod.Equal(v.mod) {
			return false
		}
	}
	return true
}

// copyFile 复制单个文件，每块之间检查是否取消，并保留修改时间。
func copyFile(ctx context.Context, src, dst string, flag int, onBytes func(int64)) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, flag, 0o644)
	if err != nil {
		return err
	}
	buf := make([]byte, 1<<20)
	for {
		if err := ctx.Err(); err != nil {
			out.Close()
			return err
		}
		n, rerr := in.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				out.Close()
				return werr
			}
			if onBytes != nil {
				onBytes(int64(n))
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			out.Close()
			return rerr
		}
	}
	if err := out.Close(); err != nil {
		return err
	}
	if fi, err := in.Stat(); err == nil {
		_ = os.Chtimes(dst, fi.ModTime(), fi.ModTime())
	}
	return nil
}

// Execute 先把全部视频复制到目标根目录里的临时文件夹，确认无误后再一次性放入目标文件夹。
// 复制失败或取消时撤销本次放入的视频并恢复被清理的旧视频；原素材始终不变。
func Execute(ctx context.Context, p Plan, progress func(Progress)) (res Result, err error) {
	if err := CheckSeparate(p.MaterialRoot, p.OutputRoot); err != nil {
		return Result{}, err
	}
	dest, err := ResolveDestination(p.OutputRoot, p.Folder)
	if err != nil {
		return Result{}, err
	}
	if len(p.Items) == 0 {
		return Result{}, userErr("没有要复制的视频")
	}
	seen := map[string]bool{}
	for _, it := range p.Items {
		fi, err := os.Lstat(it.source)
		if err != nil || !isPlainFile(fi) || !sourceUnder(p.MaterialRoot, it.source) {
			return Result{}, userErr("素材已不存在或已移动：%s / %s", it.Category, it.originalName())
		}
		if fi.Size() != it.Size || !fi.ModTime().Equal(it.mod) {
			return Result{}, userErr("素材发生了变化，请重新抽取：%s", it.originalName())
		}
		if !isVideo(it.Name) {
			return Result{}, userErr("不是支持的视频文件：%s", it.Name)
		}
		if seen[fold(it.Name)] {
			return Result{}, userErr("抽中的视频有同名冲突：%s", it.Name)
		}
		seen[fold(it.Name)] = true
	}

	// 重新确认要清理的旧视频和同名冲突，抽取到确认之间目标文件夹可能有变化。
	var old []video
	if p.Clean {
		if old, err = listVideos(dest); err != nil {
			return Result{}, userErr("无法读取目标文件夹：%v", err)
		}
	}
	removable := map[string]bool{}
	for _, v := range old {
		removable[v.name] = true
	}
	before, err := inventory(dest)
	if err != nil {
		return Result{}, userErr("无法读取目标文件夹：%v", err)
	}
	for name := range before {
		if seen[fold(name)] && !removable[name] {
			return Result{}, userErr("目标文件夹里已有同名的「%s」，为避免覆盖已停止。请重新抽取", name)
		}
	}

	var total int64
	for _, it := range p.Items {
		total += it.Size
	}
	if free, ok := freeSpace(p.OutputRoot); ok && free < uint64(total) {
		return Result{}, userErr("目标磁盘剩余空间不足：需要 %s，只剩 %s", HumanBytes(total), HumanBytes(int64(free)))
	}

	workspace, err := os.MkdirTemp(p.OutputRoot, TempPrefix)
	if err != nil {
		return Result{}, userErr("无法在目标根目录中创建临时文件夹：%v", err)
	}
	hide(workspace)
	staged := filepath.Join(workspace, "new")
	backup := filepath.Join(workspace, "old")
	if err := os.Mkdir(staged, 0o755); err != nil {
		os.RemoveAll(workspace)
		return Result{}, userErr("无法创建临时文件夹：%v", err)
	}
	if err := os.Mkdir(backup, 0o755); err != nil {
		os.RemoveAll(workspace)
		return Result{}, userErr("无法创建临时文件夹：%v", err)
	}

	var published []string
	type moved struct{ saved, original string }
	var backedUp []moved
	keepWorkspace := false

	defer func() {
		if err != nil {
			var rollbackErrs []string
			for i := len(published) - 1; i >= 0; i-- {
				if e := os.Remove(published[i]); e != nil && !errors.Is(e, fs.ErrNotExist) {
					rollbackErrs = append(rollbackErrs, e.Error())
				}
			}
			for i := len(backedUp) - 1; i >= 0; i-- {
				m := backedUp[i]
				if _, e := os.Lstat(m.original); e == nil {
					rollbackErrs = append(rollbackErrs, "恢复位置被占用："+filepath.Base(m.original))
					continue
				}
				if e := os.Rename(m.saved, m.original); e != nil {
					rollbackErrs = append(rollbackErrs, e.Error())
				}
			}
			if len(rollbackErrs) > 0 {
				keepWorkspace = true
				err = userErr("复制失败，部分文件没能自动恢复。旧视频保存在：\n%s\n\n%s", backup, strings.Join(rollbackErrs, "\n"))
			} else if errors.Is(err, context.Canceled) {
				err = ErrCancelled
			} else {
				var ue *UserError
				if !errors.As(err, &ue) {
					err = userErr("复制失败，已撤销本次复制并恢复原样：%v", err)
				}
			}
		}
		if !keepWorkspace {
			_ = os.RemoveAll(workspace)
			if _, e := os.Stat(workspace); e == nil && err == nil {
				res.Warning = "临时文件夹没能删除，可在关闭处理程序后手动删除：" + workspace
			}
		}
	}()

	// 第一阶段：复制到临时文件夹。
	var done int64
	for i, it := range p.Items {
		srcBefore, err := os.Lstat(it.source)
		if err != nil || !isPlainFile(srcBefore) || !sourceUnder(p.MaterialRoot, it.source) {
			return Result{}, userErr("素材已不存在：%s / %s", it.Category, it.originalName())
		}
		if srcBefore.Size() != it.Size || !srcBefore.ModTime().Equal(it.mod) {
			return Result{}, userErr("复制期间素材发生了变化，请重试：%s", it.originalName())
		}
		report := func(n int64) {
			done += n
			if progress != nil {
				progress(Progress{Done: i, Total: len(p.Items), Name: it.Name, BytesDone: done, BytesTotal: total})
			}
		}
		report(0)
		target := filepath.Join(staged, it.Name)
		if err := copyFile(ctx, it.source, target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, report); err != nil {
			return Result{}, err
		}
		after, err := os.Stat(it.source)
		st, serr := os.Stat(target)
		if err != nil || serr != nil || after.Size() != srcBefore.Size() || !after.ModTime().Equal(srcBefore.ModTime()) || st.Size() != srcBefore.Size() {
			return Result{}, userErr("复制期间素材发生了变化，请重试：%s", it.originalName())
		}
	}
	if progress != nil {
		progress(Progress{Done: len(p.Items), Total: len(p.Items), BytesDone: total, BytesTotal: total})
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}

	// 第二阶段：确认目标文件夹没被改动，再清理旧视频、放入新视频。
	if _, err := ResolveDestination(p.OutputRoot, p.Folder); err != nil {
		return Result{}, userErr("复制期间目标文件夹发生了变化，本次没有改动目标文件夹")
	}
	if now, err := inventory(dest); err != nil || !sameInventory(before, now) {
		return Result{}, userErr("复制期间目标文件夹被其他程序改动了。请先停止使用这个文件夹的程序，再重试")
	}
	for _, v := range old {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		saved := filepath.Join(backup, v.name)
		if err := os.Rename(v.path, saved); err != nil {
			return Result{}, userErr("无法清理旧视频「%s」：%v", v.name, err)
		}
		backedUp = append(backedUp, moved{saved, v.path})
	}
	for _, it := range p.Items {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		src := filepath.Join(staged, it.Name)
		target := filepath.Join(dest, it.Name)
		// 硬链接只连接临时副本，原素材始终独立；不支持硬链接的磁盘（如 exFAT）改为独占创建并复制。
		if err := os.Link(src, target); err != nil {
			if errors.Is(err, fs.ErrExist) {
				return Result{}, userErr("目标文件夹里刚出现了同名文件：%s", it.Name)
			}
			published = append(published, target)
			if err := copyFile(ctx, src, target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, nil); err != nil {
				if errors.Is(err, fs.ErrExist) {
					published = published[:len(published)-1]
					return Result{}, userErr("目标文件夹里刚出现了同名文件：%s", it.Name)
				}
				return Result{}, err
			}
			continue
		}
		published = append(published, target)
	}

	return Result{
		Destination: dest,
		Folder:      p.Folder,
		Items:       append([]Item{}, p.Items...),
		Removed:     len(old),
		Avoided:     p.Avoided,
		Picks:       clonePicks(p.Picks),
		Mode:        p.Mode,
	}, nil
}

func (it Item) originalName() string {
	if it.Origin != "" {
		return it.Origin
	}
	return it.Name
}

// HumanBytes 把字节数写成 1.2 GB 这样的形式。
func HumanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}
