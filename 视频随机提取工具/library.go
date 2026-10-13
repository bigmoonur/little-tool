package main

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"videopicker/internal/picker"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// SaveThumbnail 的缓存名由当前文件生成，避免界面写入其他视频的缓存。
func (a *App) SaveThumbnail(category, name, data string) error {
	a.thumbMu.Lock()
	defer a.thumbMu.Unlock()
	if a.store == nil {
		return errors.New("无法打开数据文件夹")
	}
	s := a.store.Settings()
	if !s.Thumbnails {
		return nil
	}
	path, err := picker.VideoPath(s.MaterialRoot, category, name)
	if err != nil {
		return err
	}
	fi, err := os.Lstat(path)
	if err != nil || !fi.Mode().IsRegular() {
		return errors.New("视频已经不存在了，请刷新素材库")
	}
	key := picker.ThumbKey(s.MaterialRoot, category, name, fi.Size(), fi.ModTime())
	if data == "" {
		return a.store.MarkNoThumb(key)
	}
	data = strings.TrimPrefix(data, "data:image/jpeg;base64,")
	decoded, err := io.ReadAll(io.LimitReader(base64.NewDecoder(base64.StdEncoding, strings.NewReader(data)), picker.MaxThumbBytes+1))
	if err != nil {
		return errors.New("缩略图数据不是有效的 base64")
	}
	return a.store.SaveThumb(key, decoded)
}

func (a *App) SetLibraryView(mode string) picker.Settings {
	return a.store.Update(func(s *picker.Settings) { s.LibraryView = mode })
}

func (a *App) pruneThumbnails() int {
	a.thumbMu.Lock()
	defer a.thumbMu.Unlock()
	if a.store == nil {
		return 0
	}
	s := a.store.Settings()
	if !s.Thumbnails || s.MaterialRoot == "" {
		return 0
	}
	keys, err := picker.LibraryKeys(s.MaterialRoot)
	if err != nil {
		return 0
	}
	return a.store.PruneThumbs(keys)
}

// 扫描只发起后台清理，忙碌或刚清理过时立即返回。
func (a *App) scheduleThumbnailPrune() {
	if a.store == nil || !a.pruneRunning.CompareAndSwap(false, true) {
		return
	}
	now := time.Now().UnixNano()
	if last := a.lastPrune.Load(); last != 0 && now-last < int64(20*time.Second) {
		a.pruneRunning.Store(false)
		return
	}
	a.lastPrune.Store(now)
	go func() {
		defer a.pruneRunning.Store(false)
		a.pruneThumbnails()
	}()
}

// CreateCategory 创建后返回刷新过的分类列表。
func (a *App) CreateCategory(name string) (State, error) {
	return a.CreateCategoryIn(name, "")
}

// ChooseVideos 取消时仍返回空列表，界面可以直接读取长度。
func (a *App) ChooseVideos() ([]string, error) {
	if err := a.requireLicense(); err != nil {
		return []string{}, err
	}
	patterns := make([]string, 0, len(picker.VideoExtensions))
	for ext, supported := range picker.VideoExtensions {
		if supported {
			patterns = append(patterns, "*"+ext)
		}
	}
	sort.Strings(patterns)
	paths, err := runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{
		Title:   "选择要添加的视频",
		Filters: []runtime.FileFilter{{DisplayName: "视频文件", Pattern: strings.Join(patterns, ";")}},
	})
	if paths == nil {
		paths = []string{}
	}
	return paths, err
}

type ImportDone struct {
	OK        bool                `json:"ok"`
	Cancelled bool                `json:"cancelled"`
	Error     string              `json:"error"`
	Result    picker.ImportResult `json:"result"`
}

// AddVideos 和抽取共用复制状态，取消及退出都等待回滚完成。
func (a *App) AddVideos(category string, paths []string) error {
	if err := a.requireLicense(); err != nil {
		return err
	}
	a.mu.Lock()
	if a.busy {
		a.mu.Unlock()
		return errors.New("正在复制，请等待完成或取消后再操作")
	}
	if len(paths) == 0 {
		a.mu.Unlock()
		return errors.New("请选择要添加的视频")
	}
	root := a.store.Settings().MaterialRoot
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel, a.busy = cancel, true
	a.mu.Unlock()
	go a.runImport(ctx, cancel, root, category, append([]string{}, paths...), func(event string, value any) {
		if a.ctx != nil {
			runtime.EventsEmit(a.ctx, event, value)
		}
	})
	return nil
}

func (a *App) runImport(ctx context.Context, cancel context.CancelFunc, root, category string, paths []string, emit func(string, any)) {
	var last time.Time
	res, err := picker.Import(ctx, root, category, paths, func(p picker.Progress) {
		if time.Since(last) < 80*time.Millisecond && p.Done < p.Total {
			return
		}
		last = time.Now()
		emit("import-progress", p)
	})
	done := ImportDone{OK: err == nil, Result: res}
	if err != nil {
		done.Cancelled = errors.Is(err, picker.ErrImportCancelled)
		done.Error = err.Error()
	}
	a.mu.Lock()
	a.busy, a.cancel = false, nil
	quit := a.quitAfter
	a.mu.Unlock()
	cancel()
	if quit {
		if a.ctx != nil {
			runtime.Quit(a.ctx)
		}
		return
	}
	emit("import-done", done)
}
