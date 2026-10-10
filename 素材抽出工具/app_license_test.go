package main

import (
	"reflect"
	"testing"

	"materialpicker/internal/library"
)

func TestUnlicensedMutationsCannotChangeUserData(t *testing.T) {
	if licenseBypass {
		t.Skip("development-only bypass")
	}
	store, err := library.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a := NewApp(store, nil, nil)
	before := store.Snapshot()
	cases := map[string]func() error{
		"create group":   func() error { _, e := a.CreateGroup("test"); return e },
		"rename group":   func() error { _, e := a.RenameGroup("id", "test"); return e },
		"group color":    func() error { _, e := a.SetGroupColor("id", "blue"); return e },
		"move group":     func() error { _, e := a.MoveGroup("id", 0); return e },
		"delete group":   func() error { _, e := a.DeleteGroup("id", true); return e },
		"create list":    func() error { _, e := a.CreateList("test", ""); return e },
		"rename list":    func() error { _, e := a.RenameList("id", "test"); return e },
		"move list":      func() error { _, e := a.MoveList("id", "", ""); return e },
		"delete list":    func() error { _, e := a.DeleteList("id"); return e },
		"edit text":      func() error { _, e := a.SetListText("id", "text"); return e },
		"dedupe":         func() error { _, e := a.DedupeList("id"); return e },
		"undo":           func() error { _, e := a.Undo(); return e },
		"import":         func() error { _, e := a.ApplyImport(library.ImportPlan{}); return e },
		"extract":        func() error { _, e := a.Extract(nil); return e },
		"copy":           func() error { return a.CopyText("text") },
		"save preset":    func() error { _, e := a.SavePreset("test", nil); return e },
		"rename preset":  func() error { _, e := a.RenamePreset("id", "test"); return e },
		"delete preset":  func() error { _, e := a.DeletePreset("id"); return e },
		"save file":      func() error { _, e := a.SaveTextFile("test", "text"); return e },
		"export backup":  func() error { _, e := a.ExportBackup(); return e },
		"restore backup": func() error { _, e := a.RestoreBackup(); return e },
	}
	for name, call := range cases {
		t.Run(name, func(t *testing.T) {
			if err := call(); err == nil {
				t.Fatal("unlicensed mutation succeeded")
			}
			if !reflect.DeepEqual(before, store.Snapshot()) {
				t.Fatal("unlicensed mutation changed user data")
			}
		})
	}
	// 用户自己的素材仍可读取，授权失效不会删除或加密这些数据。
	if _, err := a.Init(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, a.Snapshot()) {
		t.Fatal("user data is inaccessible")
	}
}
