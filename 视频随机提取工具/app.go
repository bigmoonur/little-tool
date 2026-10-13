package main

import (
	"context"
	"errors"
	"math/rand/v2"
	"os"
	"os/exec"
	goruntime "runtime"
	"sync"
	"sync/atomic"
	"time"

	"videopicker/internal/picker"

	"github.com/bigmoonur/little-tool/shared/license"
	"github.com/bigmoonur/little-tool/shared/license/applock"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const AppVersion = "2.0.3"

type App struct {
	ctx     context.Context
	store   *picker.Store
	lic     licenseManager
	initErr error
	rng     *rand.Rand
	lock    *applock.Lock
	lockMu  sync.Mutex

	mu        sync.Mutex
	pending   *picker.Plan
	pendingID string
	cancel    context.CancelFunc
	busy      bool
	quitAfter bool

	thumbMu      sync.Mutex
	pruneRunning atomic.Bool
	lastPrune    atomic.Int64
}

func NewApp(store *picker.Store, lic licenseManager, initErr error) *App {
	a := &App{store: store, lic: lic, initErr: initErr, rng: rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))}
	if initErr == nil && store != nil {
		a.lock = applock.Open(store.Dir())
	}
	return a
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.scheduleThumbnailPrune()
	if a.lic != nil && !licenseBypass {
		a.lic.Start(func(st license.Status) { runtime.EventsEmit(ctx, "license-changed", st) })
	}
}

// beforeClose 空闲时立即关闭；复制中先让界面确认是否取消。
func (a *App) beforeClose(ctx context.Context) bool {
	a.mu.Lock()
	busy, quitting := a.busy, a.quitAfter
	a.mu.Unlock()
	if busy && !quitting {
		runtime.EventsEmit(ctx, "close-while-busy")
		return true
	}
	if busy {
		return true
	}
	a.saveWindow()
	return false
}

// CancelAndQuit 取消复制，恢复目标文件夹后退出。
func (a *App) CancelAndQuit() {
	a.mu.Lock()
	a.quitAfter = true
	cancel, busy := a.cancel, a.busy
	a.mu.Unlock()
	if !busy {
		runtime.Quit(a.ctx)
		return
	}
	if cancel != nil {
		cancel()
	}
}

func (a *App) saveWindow() {
	if a.store == nil || a.ctx == nil || runtime.WindowIsMinimised(a.ctx) {
		return
	}
	w := picker.WindowState{Maximised: runtime.WindowIsMaximised(a.ctx)}
	if prev := a.store.Window(); prev != nil {
		w.Width, w.Height = prev.Width, prev.Height
	}
	if !w.Maximised {
		w.Width, w.Height = runtime.WindowGetSize(a.ctx)
	}
	a.store.SaveWindow(w)
}

func (a *App) onSecondInstance() {
	runtime.WindowUnminimise(a.ctx)
	runtime.WindowShow(a.ctx)
	if a.store != nil && !a.store.Settings().AlwaysOnTop {
		runtime.WindowSetAlwaysOnTop(a.ctx, true)
		runtime.WindowSetAlwaysOnTop(a.ctx, false)
	}
}

// ---------- 状态 ----------

type InitInfo struct {
	Settings picker.Settings `json:"settings"`
	Version  string          `json:"version"`
	Platform string          `json:"platform"`
	Lock     applock.Status  `json:"lock"`
}

func (a *App) Init() (InitInfo, error) {
	if a.initErr != nil {
		return InitInfo{Settings: picker.Settings{Draft: map[string]int{}, DraftRepeat: map[string]int{}, Mode: picker.ModeConsecutive, Thumbnails: true, LibraryView: "list", AfterCopy: "stay"}, Lock: a.LockStatus()}, errors.New("无法打开数据文件夹：" + a.initErr.Error())
	}
	return InitInfo{Settings: a.store.Settings(), Version: AppVersion, Platform: goruntime.GOOS, Lock: a.LockStatus()}, nil
}

// State 是界面需要的文件夹信息，每次都重新扫描磁盘。
type State struct {
	MaterialRoot  string               `json:"materialRoot"`
	OutputRoot    string               `json:"outputRoot"`
	Folder        string               `json:"folder"`
	Categories    []picker.Category    `json:"categories"`
	Groups        []picker.Group       `json:"groups"`
	Tags          []picker.Tag         `json:"tags"`
	Presets       []picker.Preset      `json:"presets"`
	Destinations  []picker.Destination `json:"destinations"`
	MaterialError string               `json:"materialError"`
	OutputError   string               `json:"outputError"`
	Leftovers     []string             `json:"leftovers"`
	Busy          bool                 `json:"busy"`
}

func (a *App) State() State {
	if a.store == nil {
		return State{Categories: []picker.Category{}, Groups: []picker.Group{}, Tags: []picker.Tag{}, Presets: []picker.Preset{}, Destinations: []picker.Destination{}, Leftovers: []string{}}
	}
	s := a.store.Settings()
	st := State{MaterialRoot: s.MaterialRoot, OutputRoot: s.OutputRoot, Folder: s.Folder,
		Categories: []picker.Category{}, Groups: a.store.Groups(), Tags: a.store.Tags(), Presets: a.store.Presets(), Destinations: []picker.Destination{}, Leftovers: []string{}}
	if s.MaterialRoot != "" {
		if cats, err := a.store.ScanCategories(s.MaterialRoot); err != nil {
			st.MaterialError = err.Error()
		} else {
			st.Categories = a.store.ArrangeCategories(cats)
			a.scheduleThumbnailPrune()
		}
	}
	if s.OutputRoot != "" {
		if dests, err := picker.ListDestinations(s.OutputRoot); err != nil {
			st.OutputError = err.Error()
		} else {
			st.Destinations = dests
			if l := picker.Leftovers(s.OutputRoot); l != nil {
				st.Leftovers = l
			}
		}
	}
	found := false
	for _, d := range st.Destinations {
		if d.Name == st.Folder {
			found = true
		}
	}
	if !found {
		st.Folder = ""
	}
	a.mu.Lock()
	st.Busy = a.busy
	a.mu.Unlock()
	return st
}

type PresetSaved struct {
	State State  `json:"state"`
	ID    string `json:"id"`
}

func (a *App) SavePreset(name string, picks map[string]picker.Pick, mode string) (PresetSaved, error) {
	if err := a.requireLicense(); err != nil {
		return PresetSaved{State: a.State()}, err
	}
	id, err := a.store.SavePreset(name, picks, mode)
	return PresetSaved{State: a.State(), ID: id}, err
}

func (a *App) RenamePreset(id, name string) (State, error) {
	if err := a.requireLicense(); err != nil {
		return a.State(), err
	}
	err := a.store.RenamePreset(id, name)
	return a.State(), err
}

func (a *App) DeletePreset(id string) (State, error) {
	if err := a.requireLicense(); err != nil {
		return a.State(), err
	}
	err := a.store.DeletePreset(id)
	return a.State(), err
}

type TagCreated struct {
	State State  `json:"state"`
	ID    string `json:"id"`
}

func (a *App) CreateTag(name string) (TagCreated, error) {
	if err := a.requireLicense(); err != nil {
		return TagCreated{State: a.State()}, err
	}
	id, err := a.store.CreateTag(name)
	return TagCreated{State: a.State(), ID: id}, err
}

func (a *App) RenameTag(id, name string) (State, error) {
	if err := a.requireLicense(); err != nil {
		return a.State(), err
	}
	err := a.store.RenameTag(id, name)
	return a.State(), err
}

func (a *App) SetTagColor(id, color string) (State, error) {
	if err := a.requireLicense(); err != nil {
		return a.State(), err
	}
	err := a.store.SetTagColor(id, color)
	return a.State(), err
}

func (a *App) DeleteTag(id string) (State, error) {
	if err := a.requireLicense(); err != nil {
		return a.State(), err
	}
	err := a.store.DeleteTag(id)
	return a.State(), err
}

func (a *App) SetCategoryTags(category string, tagIDs []string) (State, error) {
	if err := a.requireLicense(); err != nil {
		return a.State(), err
	}
	err := a.store.SetCategoryTags(category, tagIDs)
	return a.State(), err
}

func (a *App) chooseDir(title, current string) string {
	opts := runtime.OpenDialogOptions{Title: title, CanCreateDirectories: true}
	if fi, err := os.Stat(current); err == nil && fi.IsDir() {
		opts.DefaultDirectory = current
	}
	path, _ := runtime.OpenDirectoryDialog(a.ctx, opts)
	return path
}

// ChooseMaterialRoot 选择素材根目录：里面的每个子文件夹是一个分类。
func (a *App) ChooseMaterialRoot() (State, error) {
	if err := a.requireLicense(); err != nil {
		return a.State(), err
	}
	if err := a.idle(); err != nil {
		return a.State(), err
	}
	s := a.store.Settings()
	path := a.chooseDir("选择素材根目录（里面的每个子文件夹是一个分类）", s.MaterialRoot)
	if path == "" {
		return a.State(), nil
	}
	if err := picker.CheckSeparate(path, s.OutputRoot); err != nil {
		return a.State(), err
	}
	if _, err := picker.ScanCategories(path); err != nil {
		return a.State(), err
	}
	a.store.Update(func(st *picker.Settings) {
		if st.MaterialRoot != path {
			st.Draft = map[string]int{}
			st.DraftRepeat = map[string]int{}
			st.Mode = picker.ModeConsecutive
		}
		st.MaterialRoot = path
	})
	return a.State(), nil
}

// ChooseOutputRoot 选择目标根目录：抽到的视频放进它下面的一个子文件夹。
func (a *App) ChooseOutputRoot() (State, error) {
	if err := a.requireLicense(); err != nil {
		return a.State(), err
	}
	if err := a.idle(); err != nil {
		return a.State(), err
	}
	s := a.store.Settings()
	path := a.chooseDir("选择目标根目录（抽到的视频会放进它下面的一个文件夹）", s.OutputRoot)
	if path == "" {
		return a.State(), nil
	}
	if err := picker.CheckSeparate(s.MaterialRoot, path); err != nil {
		return a.State(), err
	}
	if _, err := picker.ListDestinations(path); err != nil {
		return a.State(), err
	}
	a.store.Update(func(st *picker.Settings) {
		if st.OutputRoot != path {
			st.Folder = ""
		}
		st.OutputRoot = path
	})
	return a.State(), nil
}

func (a *App) SetFolder(name string) (State, error) {
	if err := a.requireLicense(); err != nil {
		return a.State(), err
	}
	if err := a.idle(); err != nil {
		return a.State(), err
	}
	a.store.Update(func(st *picker.Settings) { st.Folder = name })
	return a.State(), nil
}

// CreateFolder 在目标根目录下新建文件夹并选中它。
func (a *App) CreateFolder(name string) (State, error) {
	if err := a.requireLicense(); err != nil {
		return a.State(), err
	}
	if err := a.idle(); err != nil {
		return a.State(), err
	}
	s := a.store.Settings()
	if err := picker.CreateFolder(s.OutputRoot, name); err != nil {
		return a.State(), err
	}
	// 已通过授权检查，创建成功后直接选中，避免在两步之间到期留下未选中的文件夹。
	a.store.Update(func(st *picker.Settings) { st.Folder = name })
	return a.State(), nil
}

// ---------- 抽取与复制 ----------

func (a *App) idle() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.busy {
		return errors.New("正在复制，请等待完成或取消后再操作")
	}
	return nil
}

type PlanSummary struct {
	ID          string        `json:"id"`
	Count       int           `json:"count"`
	Bytes       int64         `json:"bytes"`
	Old         int           `json:"old"`
	Destination string        `json:"destination"`
	Folder      string        `json:"folder"`
	Items       []picker.Item `json:"items"`
}

// Prepare 随机抽取一批视频但不复制；需要清理旧视频时由界面确认后再调用 Start。
func (a *App) Prepare(picks map[string]picker.Pick, mode string, clean bool) (PlanSummary, error) {
	if err := a.requireLicense(); err != nil {
		return PlanSummary{}, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.busy {
		return PlanSummary{}, userErr("正在复制，请等待完成或取消后再操作")
	}
	s := a.store.Settings()
	p, err := a.store.Prepare(s.MaterialRoot, s.OutputRoot, s.Folder, picks, mode, clean, a.rng)
	if err != nil {
		return PlanSummary{}, err
	}
	id := time.Now().Format("150405.000000")
	a.pending, a.pendingID = &p, id
	return PlanSummary{ID: id, Count: len(p.Items), Bytes: p.Bytes, Old: len(p.Old), Destination: p.Destination, Folder: p.Folder, Items: p.Items}, nil
}

type Done struct {
	OK        bool                `json:"ok"`
	Cancelled bool                `json:"cancelled"`
	Error     string              `json:"error"`
	Result    picker.Result       `json:"result"`
	Entry     picker.HistoryEntry `json:"entry"`
}

// Start 开始复制 Prepare 抽好的那一批。进度和结果通过事件发给界面。
func (a *App) Start(id string) error {
	// 确认清空旧视频期间授权可能已失效，因此开始复制前必须再次检查。
	if err := a.requireLicense(); err != nil {
		return err
	}
	a.mu.Lock()
	if a.busy {
		a.mu.Unlock()
		return errors.New("正在复制")
	}
	if a.pending == nil || a.pendingID != id {
		a.mu.Unlock()
		return errors.New("这批视频已失效，请重新抽取")
	}
	plan := *a.pending
	a.pending = nil
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel, a.busy = cancel, true
	a.mu.Unlock()

	go func() {
		var last time.Time
		res, err := picker.Execute(ctx, plan, func(p picker.Progress) {
			if time.Since(last) < 80*time.Millisecond && p.Done < p.Total {
				return
			}
			last = time.Now()
			runtime.EventsEmit(a.ctx, "copy-progress", p)
		})
		done := Done{OK: err == nil, Result: res}
		switch {
		case errors.Is(err, picker.ErrCancelled):
			done.Cancelled = true
			done.Error = err.Error()
		case err != nil:
			done.Error = err.Error()
		default:
			done.Entry = a.store.AddHistory(res)
		}
		a.mu.Lock()
		a.busy, a.cancel = false, nil
		quit := a.quitAfter
		a.mu.Unlock()
		cancel()
		if quit {
			runtime.Quit(a.ctx)
			return
		}
		runtime.EventsEmit(a.ctx, "copy-done", done)
		a.afterSuccessfulCopy(done.OK, res.Destination, openPath, func() { runtime.WindowMinimise(a.ctx) }, func() { runtime.Quit(a.ctx) })
	}()
	return nil
}

// Cancel 取消正在进行的复制，目标文件夹会恢复原样。
func (a *App) Cancel() {
	a.mu.Lock()
	cancel := a.cancel
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// ---------- 记录、设置、文件夹 ----------

func (a *App) History() []picker.HistoryEntry { return a.store.History() }

func (a *App) ClearHistory() error {
	if err := a.requireLicense(); err != nil {
		return err
	}
	return a.store.ClearHistory()
}

type UISettings struct {
	AlwaysOnTop   bool   `json:"alwaysOnTop"`
	OpenAfter     bool   `json:"openAfter"`
	AfterCopy     string `json:"afterCopy"`
	Theme         string `json:"theme"`
	RememberDraft bool   `json:"rememberDraft"`
	Thumbnails    bool   `json:"thumbnails"`
}

func (a *App) UpdateSettings(u UISettings) picker.Settings {
	a.thumbMu.Lock()
	prev := a.store.Settings()
	s := a.store.Update(func(st *picker.Settings) {
		st.AlwaysOnTop, st.OpenAfter, st.RememberDraft = u.AlwaysOnTop, u.OpenAfter, u.RememberDraft
		st.Thumbnails = u.Thumbnails
		st.AfterCopy = u.AfterCopy
		switch u.Theme {
		case "system", "light", "dark":
			st.Theme = u.Theme
		}
	})
	if prev.Thumbnails && !s.Thumbnails {
		_ = a.store.ClearThumbs()
	}
	a.thumbMu.Unlock()
	if prev.AlwaysOnTop != s.AlwaysOnTop && a.ctx != nil {
		runtime.WindowSetAlwaysOnTop(a.ctx, s.AlwaysOnTop)
	}
	return s
}

func (a *App) SaveDraft(counts map[string]int, repeats map[string]int, mode string) {
	a.store.Update(func(st *picker.Settings) {
		if st.RememberDraft {
			st.Draft, st.DraftRepeat, st.Mode = counts, repeats, mode
		}
	})
}

func openPath(path string) error {
	var cmd *exec.Cmd
	switch goruntime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", path)
	case "darwin":
		cmd = exec.Command("open", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	return cmd.Start()
}

// OpenFolder 在资源管理器中打开文件夹。
func (a *App) OpenFolder(path string) error {
	fi, err := os.Stat(path)
	if err != nil || !fi.IsDir() {
		return errors.New("这个文件夹已经不存在了")
	}
	return openPath(path)
}

func (a *App) OpenDataFolder() error { return openPath(a.store.Dir()) }

// ---------- 素材库 ----------

// Videos 列出一个分类里的视频，供素材库查看。
func (a *App) Videos(category string) ([]picker.VideoFile, error) {
	if a.store == nil {
		return []picker.VideoFile{}, errors.New("无法打开数据文件夹")
	}
	s := a.store.Settings()
	var videos []picker.VideoFile
	var err error
	if s.Thumbnails {
		videos, err = picker.ListVideosWithKeys(s.MaterialRoot, category)
		states := a.store.ThumbStates()
		for i := range videos {
			videos[i].Thumb = states[videos[i].Key]
		}
	} else {
		videos, err = picker.ListVideos(s.MaterialRoot, category)
	}
	if videos == nil {
		videos = []picker.VideoFile{}
	}
	return videos, err
}

// OpenCategory 在资源管理器中打开分类的文件夹。
func (a *App) OpenCategory(category string) error {
	dir, err := picker.CategoryDir(a.store.Settings().MaterialRoot, category)
	if err != nil {
		return err
	}
	return openPath(dir)
}

// OpenVideo 用系统默认的播放器打开素材库里的一个视频。
func (a *App) OpenVideo(category, name string) error {
	path, err := picker.VideoPath(a.store.Settings().MaterialRoot, category, name)
	if err != nil {
		return err
	}
	return openPath(path)
}
