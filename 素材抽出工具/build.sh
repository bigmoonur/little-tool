#!/bin/sh
# ./build.sh [windows|mac] [--garble]：默认不使用 Garble，授权加固始终开启。
set -eu
cd "$(dirname "$0")"

export LANG=en_US.UTF-8 LC_ALL=en_US.UTF-8
export PATH="$PATH:$(go env GOPATH)/bin:/opt/homebrew/bin"

TARGET=windows
USE_GARBLE=0
for arg in "$@"; do
  case "$arg" in
    windows|mac) TARGET=$arg ;;
    --garble) USE_GARBLE=1 ;;
    *) echo "用法：./build.sh [windows|mac] [--garble]" >&2; exit 1 ;;
  esac
done
case "${GOFLAGS-}" in
  *devlicense*|*debug*) echo "发布构建不能使用开发授权或调试标记" >&2; exit 1 ;;
esac
if [ "$USE_GARBLE" = 1 ]; then
  command -v garble >/dev/null || { echo "请先安装 garble v0.18.0" >&2; exit 1; }
fi

VERSION=$(python3 -c "import json; print(json.load(open('wails.json'))['info']['productVersion'])")
PROGRAM=$(python3 -c "import json; print(json.load(open('wails.json'))['outputfilename'])")
(cd frontend && npm ci && npm run build)
go vet ./...
go test -race ./...
go -C ../shared/license vet ./...
go -C ../shared/license test -race ./...
# macOS 上直接执行 Wails production 测试需要该系统框架。
if [ "$(uname -s)" = Darwin ]; then
  CGO_LDFLAGS="${CGO_LDFLAGS:-} -framework UniformTypeIdentifiers" go test -tags 'production,devlicense' ./...
else
  go test -tags 'production,devlicense' ./...
fi

PLATFORM=windows/amd64
if [ "$TARGET" = mac ]; then PLATFORM=darwin/arm64; fi
set -- build -platform "$PLATFORM" -clean -trimpath -tags production -ldflags '-s -w'
if [ "$TARGET" = windows ]; then set -- "$@" -nsis; fi
if [ "$USE_GARBLE" = 1 ]; then set -- "$@" -obfuscated -garbleargs '-literals -seed=random'; fi
wails "$@"

mkdir -p dist
if [ "$TARGET" = mac ]; then
  rm -rf "dist/$PROGRAM.app"
  cp -R "build/bin/$PROGRAM.app" dist/
  echo "已输出 dist/$PROGRAM.app"
  exit 0
fi

OUT="dist/$PROGRAM-$VERSION-windows"
if [ "$USE_GARBLE" = 0 ]; then OUT="$OUT-no-garble"; fi
rm -rf "$OUT" "$OUT.zip"
mkdir -p "$OUT"
cp "build/bin/$PROGRAM-amd64-installer.exe" "$OUT/$PROGRAM-$VERSION-安装包.exe"
cp "build/bin/$PROGRAM.exe" "$OUT/$PROGRAM.exe"
{ printf '\357\273\277'; sed 's/$/\r/' 使用说明.md; } > "$OUT/使用说明.txt"
python3 - "$OUT" <<'PY'
import hashlib, os, sys, zipfile
src = sys.argv[1]
base = os.path.dirname(src)
with zipfile.ZipFile(src + ".zip", "w", zipfile.ZIP_DEFLATED) as z:
    for root, _, files in os.walk(src):
        for f in sorted(files):
            path = os.path.join(root, f)
            z.write(path, os.path.relpath(path, base))
with open(src + ".zip", "rb") as f:
    digest = hashlib.sha256(f.read()).hexdigest()
with open(src + ".zip.sha256", "w") as f:
    f.write(digest + "  " + os.path.basename(src) + ".zip\n")
PY
echo "已输出 $OUT 和 $OUT.zip"
