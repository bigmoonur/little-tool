# 素材抽出 · 开发说明

面向 Windows 用户的桌面工具，在 macOS 上开发和打包。用户使用说明见 [使用说明.md](使用说明.md)。

- 后端：Go + [Wails v2](https://wails.io)，界面运行在系统 WebView2 中，打包后是单个 exe（约 12 MB）
- 前端：React + TypeScript + Vite，没有使用 UI 组件库，样式都在 `frontend/src/styles.css`

## 目录

```text
main.go                    窗口配置、数据目录
app.go                     暴露给界面的方法（剪贴板、文件对话框、关闭确认）
internal/license/          Keygen 联网授权（激活、验证、离线宽限期、心跳、签名校验）
internal/library/          素材库核心逻辑，可单独测试
  model.go                 数据结构、文本拆行
  extract.go               抽取算法（与旧版 素材抽出.pyw 规则一致）
  importer.go              添加素材：新建 / 追加 / 替换、去重统计
  store.go                 读写 library.json、自动备份、撤销、记录、方案
  library_test.go
frontend/src/
  App.tsx                  外壳：顶栏、页面切换、拖入文件、关闭确认、快捷键
  views/                   抽取页、素材库（侧栏 + 编辑器）、记录页
  dialogs/                 添加素材、设置、方案管理
  ui/                      弹窗、菜单、提示条、回车键图标
build/                     图标、Windows 版本信息、NSIS 安装包脚本
build.sh                   打包脚本
```

## 环境

```bash
brew install go makensis
```

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

Node.js 需要 20 以上。

## 开发

```bash
wails dev
```

会打开一个原生窗口，并在 http://localhost:34115 提供可以调用 Go 方法的浏览器版本。

两个只用于开发的环境变量：

- `MATERIAL_PICKER_DATA=/某个目录`：使用指定的数据目录，避免改动真实数据
- `MATERIAL_PICKER_HIDDEN=1`：不显示原生窗口，只在浏览器里调试

## 测试

```bash
go test ./...
```

```bash
cd frontend && npx tsc --noEmit
```

## 授权（Keygen）

配置在 `internal/license/config.go`：Account、Product、Policy ID，以及 `PublicKey`。

- 客户端只用授权码认证（`Authorization: License <key>`），程序里没有管理员或产品令牌。
- 验证时限定本产品、本策略和本机指纹（Windows MachineGuid 的哈希），并带随机数防止重放。
- 启动时验证，运行中每 6 小时复查；联网失败时凭上次验证结果离线使用 7 天。
- 抽取、复制、添加素材、导出等后端方法也会检查授权，绕过界面无效。
- 策略要求心跳时才发送心跳；本机登记被服务器释放后（例如心跳超时），下次启动用保存的授权码自动重新登记。卖家要收回授权时应在后台暂停（Suspend）授权，而不是只删除机器。

Keygen 后台需要的设置：

- Policy 的 Authentication Strategy 设为 `LICENSE` 或 `MIXED`，否则客户端无法登记机器。
- Max Machines 设为每个授权码可激活的电脑数。
- `PublicKey` 已填入账户的 Ed25519 公钥（Keygen 后台 Settings 页），所有服务器响应和离线缓存都会校验签名。更换公钥后运行下面的命令，确认能校验真实服务器的签名：

```bash
KEYGEN_LIVE=1 go test ./internal/license -run Live -v
```

开发时跳过授权：`wails dev -tags devlicense`。发布构建不带这个标记，`devlicense.go` 不会被编译进去。

## 打包

```bash
./build.sh
```

输出到 `dist/`：

- `素材抽出-版本号-windows/素材抽出-版本号-安装包.exe`：按当前用户安装，不需要管理员权限
- `素材抽出-版本号-windows/素材抽出.exe`：免安装版
- 同名 zip，直接发给客户

`./build.sh mac` 打包 macOS 版，用于在开发机上试用。

改版本号时同时修改 `wails.json` 的 `info.productVersion` 和 `app.go` 的 `AppVersion`。

## 发布前注意

- 没有代码签名时，Windows 首次运行会出现 SmartScreen 提示。购买代码签名证书后，在 `build/windows/installer/project.nsi` 末尾的 `!finalize` 处配置签名。
- 安装包脚本 `project.nsi` 带 UTF-8 BOM，编辑时不要去掉，否则中文会导致 makensis 报错。
- 数据保存在 `%APPDATA%\MaterialPicker\`（授权信息在其中的 `license.json`），exe 旁边有 `data` 文件夹时改用便携模式。
