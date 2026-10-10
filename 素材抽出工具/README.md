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
go install github.com/wailsapp/wails/v2/cmd/wails@v2.16.0
go install mvdan.cc/garble@v0.18.0
```

发布工具链固定为 Go 1.27.2、Wails 2.16.0、Garble 0.18.0；Node.js 使用 22 LTS。

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
- 每次受保护操作都在本地复核签名、有效期和离线截止时间，不额外发起网络请求；宽限期不会超过授权本身的到期时间。
- 收到明确拒绝或签名异常后清除旧的成功缓存，必须重新联网验证成功才能恢复；暂时断网、服务端 5xx 或限流仍可使用有效的离线缓存。
- 独立的 `license-state.bin` 记录已经收到的拒绝结果及最高已使用时间，Windows 使用当前用户 DPAPI 保护。进程内以服务器签名时间和单调时钟计时，重启后的明显时间回退需要联网验证。已有 `license.json` 自动兼容，不要求重新输入授权码。
- 抽取、复制、添加素材、导出等后端方法也会检查授权，绕过界面无效。
- 策略要求心跳时才发送心跳；本机登记被服务器释放后（例如心跳超时），下次启动用保存的授权码自动重新登记。卖家要收回授权时应在后台暂停（Suspend）授权，而不是只删除机器。

Keygen 后台需要的设置：

- Policy 的 Authentication Strategy 设为 `LICENSE` 或 `MIXED`，否则客户端无法登记机器。
- Max Machines 设为每个授权码可激活的电脑数。
- `PublicKey` 已填入账户的 Ed25519 公钥（Keygen 后台 Settings 页），所有服务器响应和离线缓存都会校验签名。更换公钥后运行下面的命令，确认能校验真实服务器的签名：

```bash
KEYGEN_LIVE=1 go test ./internal/license -run Live -v
```

开发时跳过授权：`wails dev -tags devlicense`。正式版使用编译期 `false` 常量，带 `production` 时即使误传 `devlicense` 也不能免授权。

## 打包

```bash
./build.sh
```

输出到 `dist/`：

- `素材抽出-版本号-windows/素材抽出-版本号-安装包.exe`：按当前用户安装，不需要管理员权限
- `素材抽出-版本号-windows/素材抽出.exe`：免安装版
- 同名 zip，直接发给客户

`./build.sh mac` 打包 macOS 版，用于在开发机上试用。

Windows 上用 PowerShell 7 执行 `./build.ps1`。两个脚本都会先执行检查和测试，再使用 Wails 的 Garble 混淆（含字符串混淆和每次构建的随机种子），移除调试符号及本机路径。保留崩溃诊断，不使用 UPX 或反调试行为，以兼顾稳定性和兼容性。

GitHub Actions 的 **Windows release build** 会在代码推送后自动执行，也支持手动运行。Windows runner 会运行授权回归测试（包括 DPAPI）、前端构建、混淆打包和程序启动检查，完成后在运行页的 **windows-release** artifact 中下载发布 ZIP 和 SHA-256 校验文件。ZIP 包含免安装 EXE、安装包和使用说明，保留 14 天；不会自动创建公开 Release 或发布给客户。启动截图另存为 **windows-startup-screenshot**，不放入发给客户的 ZIP。

本地授权检查和 DPAPI 都不能抵御完全控制本机的攻击者；保留七天离线使用意味着服务器暂停授权无法在断网期间立即传播。需要更强的服务端控制时，应另行设计服务端必需功能，不能靠加壳保证绝对防破解。

改版本号时同时修改 `wails.json` 的 `info.productVersion` 和 `app.go` 的 `AppVersion`。

## 发布前注意

- 没有代码签名时，Windows 首次运行会出现 SmartScreen 提示。购买代码签名证书后，在 `build/windows/installer/project.nsi` 末尾的 `!finalize` 处配置签名。
- 安装包脚本 `project.nsi` 带 UTF-8 BOM，编辑时不要去掉，否则中文会导致 makensis 报错。
- 数据保存在 `%APPDATA%\MaterialPicker\`（授权信息在其中的 `license.json`），exe 旁边有 `data` 文件夹时改用便携模式。
