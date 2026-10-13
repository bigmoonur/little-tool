# 视频分发工具 · 开发说明

与「链接抽取工具」同一套技术和设计语言的 Windows 桌面工具，在 macOS 上开发和打包。用户使用说明见 [使用说明.md](使用说明.md)。

- 后端：Go + [Wails v2](https://wails.io)，打包后是单个 exe
- 前端：React + TypeScript + Vite；弹窗、提示条、回车键图标与「链接抽取工具」共用同一份实现，样式沿用同一套色彩和组件

## 目录

```text
main.go                    窗口配置、数据目录
app.go                     暴露给界面的方法：选择文件夹、抽取、复制进度与取消、记录、设置
internal/picker/
  fs.go                    扫描分类和目标文件夹、文件夹名称规则、两个根目录相互独立、账号接口预留
  plan.go                  随机抽取，自动避开同名视频
  copy.go                  暂存复制 → 检查 → 一次性放入；失败或取消时回滚
  disk_*.go                剩余空间、隐藏临时文件夹（按平台）
  store.go                 settings.json、history.json、window.json
  tags.go                  标签（名称、颜色）和每个分类贴的标签，保存在 tags.json
  picker_test.go
frontend/src/
  App.tsx                  外壳：顶栏、复制状态、自动刷新、关闭确认
  views/                   首次使用、抽取页、记录页
  dialogs/                 设置、标签管理和贴标签
  ui/                      弹窗、小面板、提示条、标签、回车键图标、标志
../shared/license/        两款工具共用的 Keygen 授权和安全回归测试
license.go                 授权接口与后端操作检查
build.sh / build.ps1        相同发布策略：默认无 Garble、production、移除路径和符号
```

## 与旧版的对应

旧版（Python / Tkinter）的规则全部保留：分类只读直接子文件夹里的视频、两个根目录相互独立、Windows 文件夹名称规则、同名不覆盖、清空旧视频只动直接的视频文件、先暂存再提交、失败回滚、复制期间检测目标文件夹改动、剩余空间检查、记住文件夹。

改进：同名视频自动避开而不是报错；按字节显示进度，取消立即生效；回到窗口自动刷新；复制记录；残留临时文件夹提示；临时文件夹在 Windows 上设为隐藏。

旧版预留的账号接口对应 `picker.ResolveAccountDestination`：以后提供“账号信息 → 文件夹名称”的函数即可接入，目前界面仍由用户手动选择文件夹。

## 开发

```bash
wails dev
# 本地界面开发才可以跳过授权
wails dev -tags devlicense
```

只用于开发的环境变量：`VIDEO_PICKER_DATA=目录` 使用指定的数据目录；`VIDEO_PICKER_HIDDEN=1` 不显示原生窗口，只在 http://localhost:34115 里调试。

## 测试

```bash
go test -race ./...
go -C ../shared/license test -race ./...
```

```bash
cd frontend && npx tsc --noEmit
```

## 授权（Keygen）

与链接抽取工具共用 [`../shared/license`](../shared/license/README.md)，Account、Product、Policy ID、公钥和机器指纹相同。同一授权码在同一电脑上共用一个设备名额；各自的缓存保存在各自的数据目录，首次使用分别输入一次授权码。

- 启动时联网验证，运行中每 24 小时复核；只有后台要求心跳时才按后台期限发送心跳。
- 每分钟和每次核心操作都做本地检查；签名有效的缓存最多允许离线 7 天，且不超过授权到期。
- 签名校验、明确拒绝后禁止离线恢复、防时钟回拨和 Windows 当前用户 DPAPI 状态保护与文本工具共用同一实现。
- 选择路径、选择或创建目标文件夹、抽取、开始复制、清空记录都在 Go 后端检查授权；确认清空旧视频后开始复制时再次检查。
- 已开始的复制允许完成或取消，重新验证覆盖层保留数量、任务状态和事件监听。取消、退出、查看数据及保存草稿不依赖授权。
- 换电脑前在两款工具中都解除本机授权，避免另一款用保存的授权码自动重新登记。


## 打包

```bash
./build.sh
```

Windows 也可执行 `./build.ps1`。默认无 Garble，始终使用 `production`、`-trimpath` 和 `-s -w`，并检查误传 `devlicense` 也不能免授权。需要主动比较混淆构建时才使用 `./build.ps1 -WithObfuscation` 或 `./build.sh --garble`。

输出到 `dist/`（默认包名带 `-no-garble`）：安装包（按当前用户安装，不需要管理员权限）、免安装 exe、使用说明，以及同名 zip。`./build.sh mac` 打包 macOS 版用于试用。

改版本号时同时修改 `wails.json` 的 `info.productVersion` 和 `app.go` 的 `AppVersion`。

## 发布前注意

- 没有代码签名时，Windows 首次运行会出现 SmartScreen 提示。
- 安装包脚本 `project.nsi` 带 UTF-8 BOM，编辑时不要去掉。
- 数据保存在 `%APPDATA%\VideoRandomPicker\`，exe 旁边有 `data` 文件夹时改用便携模式。
