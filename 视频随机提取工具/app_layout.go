package main

import (
	"fmt"

	"videopicker/internal/picker"
)

func userErr(format string, args ...any) error {
	return &picker.UserError{Msg: fmt.Sprintf(format, args...)}
}

// 与开始复制共用锁，避免检查空闲后任务恰好启动。
func (a *App) changeLibrary(licensed bool, fn func() error) error {
	if licensed {
		if err := a.requireLicense(); err != nil {
			return err
		}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.busy {
		return userErr("正在复制，请等待完成或取消后再操作")
	}
	return fn()
}

type GroupCreated struct {
	State State  `json:"state"`
	ID    string `json:"id"`
}

func (a *App) CreateGroup(name string) (GroupCreated, error) {
	var id string
	err := a.changeLibrary(true, func() error {
		var err error
		id, err = a.store.CreateGroup(name)
		return err
	})
	return GroupCreated{State: a.State(), ID: id}, err
}

func (a *App) RenameGroup(id, name string) (State, error) {
	err := a.changeLibrary(true, func() error { return a.store.RenameGroup(id, name) })
	return a.State(), err
}

func (a *App) SetGroupColor(id, color string) (State, error) {
	err := a.changeLibrary(true, func() error { return a.store.SetGroupColor(id, color) })
	return a.State(), err
}

func (a *App) SetGroupCollapsed(id string, collapsed bool) (State, error) {
	err := a.changeLibrary(false, func() error { return a.store.SetGroupCollapsed(id, collapsed) })
	return a.State(), err
}

func (a *App) DeleteGroup(id string) (State, error) {
	err := a.changeLibrary(true, func() error { return a.store.DeleteGroup(id) })
	return a.State(), err
}

func (a *App) MoveGroup(id, beforeID string) (State, error) {
	err := a.changeLibrary(true, func() error { return a.store.MoveGroup(id, beforeID) })
	return a.State(), err
}

func (a *App) MoveCategory(name, groupID, beforeName string) (State, error) {
	err := a.changeLibrary(true, func() error {
		cats, err := a.store.ScanCategories(a.store.Settings().MaterialRoot)
		if err != nil {
			return err
		}
		a.store.ArrangeCategories(cats)
		return a.store.MoveCategory(name, groupID, beforeName)
	})
	return a.State(), err
}

func (a *App) CreateCategoryIn(name, groupID string) (State, error) {
	err := a.changeLibrary(true, func() error {
		root := a.store.Settings().MaterialRoot
		cats, err := a.store.ScanCategories(root)
		if err != nil {
			return err
		}
		a.store.ArrangeCategories(cats)
		return a.store.CreateCategoryIn(root, name, groupID)
	})
	return a.State(), err
}

func (a *App) RenameCategory(oldName, newName string) (State, error) {
	err := a.changeLibrary(true, func() error {
		if err := a.store.RenameCategory(a.store.Settings().MaterialRoot, oldName, newName); err != nil {
			return err
		}
		// 确认过的抽取计划仍使用旧路径，需要重新抽取。
		a.pending, a.pendingID = nil, ""
		return nil
	})
	return a.State(), err
}

type CategoryDeleted struct {
	State   State `json:"state"`
	Removed int   `json:"removed"`
	Kept    bool  `json:"kept"`
}

func (a *App) DeleteCategory(name string, withVideos bool) (CategoryDeleted, error) {
	var deleted picker.CategoryDeletion
	err := a.changeLibrary(true, func() error {
		var err error
		deleted, err = a.store.DeleteCategory(a.store.Settings().MaterialRoot, name, withVideos)
		a.pending, a.pendingID = nil, ""
		return err
	})
	return CategoryDeleted{State: a.State(), Removed: deleted.Removed, Kept: deleted.Kept}, err
}

// 添加视频不经过这里；只有抽取复制成功后才处理窗口。
func (a *App) afterSuccessfulCopy(ok bool, destination string, open func(string) error, minimize, quit func()) {
	if !ok {
		return
	}
	s := a.store.Settings()
	if s.OpenAfter {
		_ = open(destination)
	}
	switch s.AfterCopy {
	case "minimize":
		minimize()
	case "quit":
		quit()
	}
}
