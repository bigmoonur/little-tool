package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync"
	"time"

	"materialpicker/internal/library"
	"materialpicker/internal/license"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const AppVersion = "1.1.8"

type App struct {
	ctx     context.Context
	store   *library.Store
	lic     *license.Manager
	initErr error

	closeMu    sync.Mutex
	allowClose bool
	unsaved    bool
	closeTimer *time.Timer
}

func NewApp(store *library.Store, lic *license.Manager, initErr error) *App {
	return &App{store: store, lic: lic, initErr: initErr}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	if a.lic != nil && !licenseBypass {
		a.lic.Start(func(st license.Status) { runtime.EventsEmit(ctx, "license-changed", st) })
	}
}

// ---------- 授权 ----------

// LicenseStatus 等待启动时的联网验证完成后返回授权状态。
func (a *App) LicenseStatus() license.Status {
	switch {
	case licenseBypass:
		return license.Status{State: license.StateActive, Masked: "开发模式"}
	case a.lic == nil:
		return license.Status{State: license.StateInactive, Message: "无法读取数据文件夹，不能保存授权信息"}
	}
	return a.lic.Wait()
}

func (a *App) ActivateLicense(key string) (license.Status, error) {
	if a.lic == nil {
		return a.LicenseStatus(), errors.New("无法读取数据文件夹，不能保存授权信息")
	}
	return a.lic.Activate(key)
}

// RefreshLicense 重新联网验证已保存的授权码。
func (a *App) RefreshLicense() license.Status {
	if a.lic == nil || licenseBypass {
		return a.LicenseStatus()
	}
	return a.lic.Refresh()
}

// DeactivateLicense 解除本机授权，可以把授权转移到其他电脑。
func (a *App) DeactivateLicense() (license.Status, error) {
	if a.lic == nil || licenseBypass {
		return a.LicenseStatus(), nil
	}
	return a.lic.Deactivate()
}

// requireLicense 在后端再检查一次授权，界面被绕过时核心功能也不可用。
func (a *App) requireLicense() error {
	if licenseBypass {
		return nil
	}
	if a.lic != nil {
		st := a.lic.Status()
		if st.State == license.StateActive {
			return nil
		}
		if st.Message != "" {
			return errors.New(st.Message)
		}
	}
	return errors.New("程序尚未激活，请先输入授权码")
}

// beforeClose 没有未保存的修改时立即关闭；有的话交给界面询问是否保存。
func (a *App) beforeClose(ctx context.Context) bool {
	a.closeMu.Lock()
	if a.allowClose || !a.unsaved || a.store == nil {
		a.closeMu.Unlock()
		a.saveWindow()
		return false
	}
	runtime.EventsEmit(ctx, "close-requested")
	// 界面会立即调用 HoldClose 并弹出询问；界面无响应时仍然关闭，避免窗口关不掉。
	if a.closeTimer != nil {
		a.closeTimer.Stop()
	}
	a.closeTimer = time.AfterFunc(1500*time.Millisecond, a.ConfirmClose)
	a.closeMu.Unlock()
	return true
}

// SetUnsaved 由界面在编辑器出现或清除未保存的修改时调用。
func (a *App) SetUnsaved(v bool) {
	a.closeMu.Lock()
	a.unsaved = v
	a.closeMu.Unlock()
}

// HoldClose 由界面调用：有未保存的修改，界面会自己询问用户。
func (a *App) HoldClose() {
	a.closeMu.Lock()
	defer a.closeMu.Unlock()
	if a.closeTimer != nil {
		a.closeTimer.Stop()
		a.closeTimer = nil
	}
}

// ConfirmClose 关闭程序。
func (a *App) ConfirmClose() {
	a.closeMu.Lock()
	if a.closeTimer != nil {
		a.closeTimer.Stop()
		a.closeTimer = nil
	}
	a.allowClose = true
	a.closeMu.Unlock()
	runtime.Quit(a.ctx)
}

func (a *App) saveWindow() {
	if a.store == nil || a.ctx == nil {
		return
	}
	if runtime.WindowIsMinimised(a.ctx) {
		return
	}
	w := library.WindowState{Maximised: runtime.WindowIsMaximised(a.ctx)}
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
	// Windows 下短暂置顶，把窗口带到最前面。
	if a.store != nil && !a.store.Settings().AlwaysOnTop {
		runtime.WindowSetAlwaysOnTop(a.ctx, true)
		runtime.WindowSetAlwaysOnTop(a.ctx, false)
	}
}

// ---------- 初始化 ----------

type InitInfo struct {
	Snapshot library.Snapshot `json:"snapshot"`
	Notice   string           `json:"notice"`
	Version  string           `json:"version"`
	Platform string           `json:"platform"`
	MaxLines int              `json:"maxLines"`
}

func (a *App) Init() (InitInfo, error) {
	if a.initErr != nil {
		return InitInfo{}, errors.New("无法打开数据文件夹：" + a.initErr.Error())
	}
	notice := a.store.Notice
	a.store.Notice = ""
	return InitInfo{
		Snapshot: a.store.Snapshot(),
		Notice:   notice,
		Version:  AppVersion,
		Platform: goruntime.GOOS,
		MaxLines: library.MaxOutputLines,
	}, nil
}

func (a *App) Snapshot() library.Snapshot { return a.store.Snapshot() }

func (a *App) ListText(id string) (string, error) { return a.store.ListText(id) }

func (a *App) Undo() (library.Snapshot, error) {
	if err := a.requireLicense(); err != nil {
		return library.Snapshot{}, err
	}
	return a.store.Undo()
}

// ---------- 分组 ----------

type Created struct {
	Snapshot library.Snapshot `json:"snapshot"`
	ID       string           `json:"id"`
}

func (a *App) CreateGroup(name string) (Created, error) {
	if err := a.requireLicense(); err != nil {
		return Created{}, err
	}
	snap, id, err := a.store.CreateGroup(name)
	return Created{snap, id}, err
}

func (a *App) RenameGroup(id, name string) (library.Snapshot, error) {
	if err := a.requireLicense(); err != nil {
		return library.Snapshot{}, err
	}
	return a.store.RenameGroup(id, name)
}

func (a *App) SetGroupColor(id, color string) (library.Snapshot, error) {
	if err := a.requireLicense(); err != nil {
		return library.Snapshot{}, err
	}
	return a.store.SetGroupColor(id, color)
}

func (a *App) SetGroupCollapsed(id string, collapsed bool) library.Snapshot {
	return a.store.SetGroupCollapsed(id, collapsed)
}

func (a *App) MoveGroup(id string, index int) (library.Snapshot, error) {
	if err := a.requireLicense(); err != nil {
		return library.Snapshot{}, err
	}
	return a.store.MoveGroup(id, index)
}

func (a *App) DeleteGroup(id string, deleteLists bool) (library.Snapshot, error) {
	if err := a.requireLicense(); err != nil {
		return library.Snapshot{}, err
	}
	return a.store.DeleteGroup(id, deleteLists)
}

// ---------- 分类 ----------

func (a *App) CreateList(name, groupID string) (Created, error) {
	if err := a.requireLicense(); err != nil {
		return Created{}, err
	}
	snap, id, err := a.store.CreateList(name, groupID)
	return Created{snap, id}, err
}

func (a *App) RenameList(id, name string) (library.Snapshot, error) {
	if err := a.requireLicense(); err != nil {
		return library.Snapshot{}, err
	}
	return a.store.RenameList(id, name)
}

func (a *App) MoveList(id, groupID, beforeID string) (library.Snapshot, error) {
	if err := a.requireLicense(); err != nil {
		return library.Snapshot{}, err
	}
	return a.store.MoveList(id, groupID, beforeID)
}

func (a *App) DeleteList(id string) (library.Snapshot, error) {
	if err := a.requireLicense(); err != nil {
		return library.Snapshot{}, err
	}
	return a.store.DeleteList(id)
}

func (a *App) SetListText(id, text string) (library.Snapshot, error) {
	if err := a.requireLicense(); err != nil {
		return library.Snapshot{}, err
	}
	return a.store.SetListText(id, text)
}

type Deduped struct {
	Snapshot library.Snapshot `json:"snapshot"`
	Removed  int              `json:"removed"`
}

func (a *App) DedupeList(id string) (Deduped, error) {
	if err := a.requireLicense(); err != nil {
		return Deduped{}, err
	}
	snap, n, err := a.store.DedupeList(id)
	return Deduped{snap, n}, err
}

// ---------- 导入 ----------

func (a *App) PreviewImport(plan library.ImportPlan) library.ImportReport {
	return a.store.PreviewImport(plan)
}

type Imported struct {
	Snapshot library.Snapshot     `json:"snapshot"`
	Report   library.ImportReport `json:"report"`
}

func (a *App) ApplyImport(plan library.ImportPlan) (Imported, error) {
	if err := a.requireLicense(); err != nil {
		return Imported{Snapshot: a.store.Snapshot()}, err
	}
	snap, rep, err := a.store.ApplyImport(plan)
	return Imported{snap, rep}, err
}

// ---------- 抽取 ----------

type Extracted struct {
	Result    library.ExtractResult `json:"result"`
	Copied    bool                  `json:"copied"`
	HistoryID string                `json:"historyId"`
	Time      int64                 `json:"time"`
}

func (a *App) Extract(reqs []library.PickRequest) (Extracted, error) {
	if err := a.requireLicense(); err != nil {
		return Extracted{}, err
	}
	res, err := a.store.Extract(reqs)
	if err != nil {
		return Extracted{}, err
	}
	copied := a.copy(res.Text) == nil
	entry := a.store.AddHistory(res)
	return Extracted{Result: res, Copied: copied, HistoryID: entry.ID, Time: entry.Time}, nil
}

// CopyText 写入剪贴板。
func (a *App) CopyText(text string) error {
	if err := a.requireLicense(); err != nil {
		return err
	}
	if err := a.copy(text); err != nil {
		return errors.New("剪贴板正被其他程序占用，请稍后再试")
	}
	return nil
}

// copy 写系统剪贴板并重试，容忍其他进程短暂占用剪贴板。
func (a *App) copy(text string) error {
	var err error
	for range 15 {
		if err = runtime.ClipboardSetText(a.ctx, text); err == nil {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return err
}

func (a *App) SaveDraft(draft map[string]library.Pick) { a.store.SaveDraft(draft) }

// ---------- 方案 ----------

func (a *App) SavePreset(name string, picks map[string]library.Pick) (Created, error) {
	if err := a.requireLicense(); err != nil {
		return Created{}, err
	}
	snap, id, err := a.store.SavePreset(name, picks)
	return Created{snap, id}, err
}

func (a *App) RenamePreset(id, name string) (library.Snapshot, error) {
	if err := a.requireLicense(); err != nil {
		return library.Snapshot{}, err
	}
	return a.store.RenamePreset(id, name)
}

func (a *App) DeletePreset(id string) (library.Snapshot, error) {
	if err := a.requireLicense(); err != nil {
		return library.Snapshot{}, err
	}
	return a.store.DeletePreset(id)
}

// ---------- 记录 ----------

func (a *App) History() []library.HistoryEntry { return a.store.History() }

func (a *App) HistoryText(id string) (string, error) { return a.store.HistoryText(id) }

func (a *App) ClearHistory() error { return a.store.ClearHistory() }

// ---------- 设置 ----------

func (a *App) UpdateSettings(s library.Settings) (library.Snapshot, error) {
	prev := a.store.Settings()
	snap, err := a.store.UpdateSettings(s)
	if err == nil && prev.AlwaysOnTop != snap.Settings.AlwaysOnTop {
		runtime.WindowSetAlwaysOnTop(a.ctx, snap.Settings.AlwaysOnTop)
	}
	return snap, err
}

// ---------- 文件 ----------

var unsafeFileChars = strings.NewReplacer(`\`, "_", "/", "_", ":", "_", "*", "_", "?", "_", `"`, "_", "<", "_", ">", "_", "|", "_")

func lineEnding(text string) string {
	if goruntime.GOOS == "windows" {
		return strings.ReplaceAll(text, "\n", "\r\n")
	}
	return text
}

// SaveTextFile 让用户选择位置保存文本，取消时返回空字符串。
func (a *App) SaveTextFile(defaultName, text string) (string, error) {
	if err := a.requireLicense(); err != nil {
		return "", err
	}
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "保存为 TXT",
		DefaultFilename: unsafeFileChars.Replace(defaultName) + ".txt",
		Filters:         []runtime.FileFilter{{DisplayName: "文本文件 (*.txt)", Pattern: "*.txt"}},
	})
	if err != nil || path == "" {
		return "", err
	}
	if !strings.EqualFold(filepath.Ext(path), ".txt") {
		path += ".txt"
	}
	if err := os.WriteFile(path, []byte(lineEnding(text)+lineEnding("\n")), 0o644); err != nil {
		return "", errors.New("保存失败：" + err.Error())
	}
	return path, nil
}

func (a *App) ExportList(id string) (string, error) {
	text, err := a.store.ListText(id)
	if err != nil {
		return "", err
	}
	return a.SaveTextFile(a.store.ListName(id), text)
}

func (a *App) ExportBackup() (string, error) {
	if err := a.requireLicense(); err != nil {
		return "", err
	}
	b, err := a.store.ExportData()
	if err != nil {
		return "", err
	}
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "导出备份",
		DefaultFilename: "素材抽出备份-" + time.Now().Format("20060102") + ".json",
		Filters:         []runtime.FileFilter{{DisplayName: "素材抽出备份 (*.json)", Pattern: "*.json"}},
	})
	if err != nil || path == "" {
		return "", err
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return "", errors.New("导出失败：" + err.Error())
	}
	return path, nil
}

type Restored struct {
	Snapshot library.Snapshot `json:"snapshot"`
	Restored bool             `json:"restored"`
}

func (a *App) RestoreBackup() (Restored, error) {
	if err := a.requireLicense(); err != nil {
		return Restored{Snapshot: a.store.Snapshot()}, err
	}
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:   "选择备份文件",
		Filters: []runtime.FileFilter{{DisplayName: "素材抽出备份 (*.json)", Pattern: "*.json"}},
	})
	if err != nil || path == "" {
		return Restored{Snapshot: a.store.Snapshot()}, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return Restored{Snapshot: a.store.Snapshot()}, errors.New("无法读取备份文件：" + err.Error())
	}
	snap, err := a.store.RestoreData(b)
	return Restored{snap, err == nil}, err
}

func (a *App) OpenDataFolder() error {
	dir := a.store.Dir()
	var cmd *exec.Cmd
	switch goruntime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", dir)
	case "darwin":
		cmd = exec.Command("open", dir)
	default:
		cmd = exec.Command("xdg-open", dir)
	}
	// explorer 即使成功也可能返回非零退出码，这里只关心能否启动。
	return cmd.Start()
}
