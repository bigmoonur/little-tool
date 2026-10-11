package main

import (
	"errors"

	"github.com/bigmoonur/little-tool/shared/license"
)

// 两款工具共用同一实现。接口便于测试后端是否在每次核心操作前读取最新状态。
type licenseManager interface {
	Start(func(license.Status))
	Wait() license.Status
	Status() license.Status
	Refresh() license.Status
	Activate(string) (license.Status, error)
	Deactivate() (license.Status, error)
}

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

func (a *App) RefreshLicense() license.Status {
	if a.lic == nil || licenseBypass {
		return a.LicenseStatus()
	}
	return a.lic.Refresh()
}

func (a *App) DeactivateLicense() (license.Status, error) {
	if a.lic == nil || licenseBypass {
		return a.LicenseStatus(), nil
	}
	// 已经开始的复制允许完成或取消；不能在复制途中主动解除授权。
	if err := a.idle(); err != nil {
		return a.lic.Status(), err
	}
	return a.lic.Deactivate()
}

// requireLicense 在后端检查签名、有效期和离线截止时间，绕过界面不能使用核心功能。
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
