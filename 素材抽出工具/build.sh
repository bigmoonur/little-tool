#!/bin/sh
# 在 macOS 上打包发布文件。
#   ./build.sh          打包 Windows 版（免安装 exe + 安装包），输出到 dist/
#   ./build.sh mac      打包 macOS 版，用于在开发机上试用
set -e
cd "$(dirname "$0")"

# makensis 需要 UTF-8 环境才能读取中文的产品名称。
export LANG=en_US.UTF-8 LC_ALL=en_US.UTF-8
export PATH="$PATH:$(go env GOPATH)/bin:/opt/homebrew/bin"

VERSION=$(python3 -c "import json; print(json.load(open('wails.json'))['info']['productVersion'])")

go vet ./...
go test ./...

if [ "$1" = "mac" ]; then
  wails build -platform darwin/arm64 -clean -trimpath
  mkdir -p dist
  rm -rf "dist/素材抽出.app"
  cp -R "build/bin/素材抽出.app" dist/
  echo "已输出 dist/素材抽出.app"
  exit 0
fi

wails build -platform windows/amd64 -nsis -clean -trimpath

OUT="dist/素材抽出-$VERSION-windows"
rm -rf "$OUT" "$OUT.zip"
mkdir -p "$OUT"
cp "build/bin/素材抽出-amd64-installer.exe" "$OUT/素材抽出-$VERSION-安装包.exe"
cp "build/bin/素材抽出.exe" "$OUT/素材抽出.exe"
# 带 BOM、CRLF 换行，旧版记事本也能正常显示。
{ printf '\357\273\277'; sed 's/$/\r/' 使用说明.md; } > "$OUT/使用说明.txt"
# 用 Python 打包：会给中文文件名设置 UTF-8 标记，Windows 解压时不会乱码。
python3 - "$OUT" <<'PY'
import os, sys, zipfile
src = sys.argv[1]
base = os.path.dirname(src)
with zipfile.ZipFile(src + ".zip", "w", zipfile.ZIP_DEFLATED) as z:
    for root, _, files in os.walk(src):
        for f in sorted(files):
            path = os.path.join(root, f)
            z.write(path, os.path.relpath(path, base))
PY
echo "已输出 $OUT 和 $OUT.zip"
