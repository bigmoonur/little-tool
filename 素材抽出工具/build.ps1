# Windows 本地或 GitHub Actions：检查、混淆编译、生成安装包和发布 ZIP。
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
Set-Location $PSScriptRoot

function Invoke-Checked {
    param([string]$Program, [string[]]$Arguments)
    & $Program @Arguments
    if ($LASTEXITCODE -ne 0) { throw "$Program failed with exit code $LASTEXITCODE" }
}

if ($env:GOFLAGS -match 'devlicense|debug') { throw 'Release builds cannot use development or debug flags' }
$env:PATH += ";$(go env GOPATH)\bin;${env:ProgramFiles(x86)}\NSIS"
Get-Command garble -ErrorAction Stop | Out-Null

Push-Location frontend
try {
    Invoke-Checked -Program npm -Arguments @('ci')
    Invoke-Checked -Program npm -Arguments @('run', 'build')
} finally { Pop-Location }

Invoke-Checked -Program go -Arguments @('vet', './...')
Invoke-Checked -Program go -Arguments @('test', '-race', './...')
# 即使误传开发免授权标签，正式版的编译期常量也必须仍为 false。
Invoke-Checked -Program go -Arguments @('test', '-tags', 'production,devlicense', './...')
Invoke-Checked -Program wails -Arguments @('build', '-platform', 'windows/amd64', '-nsis', '-clean', '-trimpath',
    '-tags', 'production', '-obfuscated', '-garbleargs', '-literals -seed=random', '-ldflags', '-s -w')

$version = (Get-Content -LiteralPath wails.json -Raw -Encoding utf8 | ConvertFrom-Json).info.productVersion
$name = "素材抽出-$version-windows"
$out = Join-Path dist $name
if (Test-Path -LiteralPath $out) { Remove-Item -LiteralPath $out -Recurse -Force }
New-Item -ItemType Directory -Path $out -Force | Out-Null
Copy-Item -LiteralPath 'build/bin/素材抽出.exe' -Destination $out
Copy-Item -LiteralPath 'build/bin/素材抽出-amd64-installer.exe' -Destination (Join-Path $out "素材抽出-$version-安装包.exe")
$text = [IO.File]::ReadAllText((Join-Path $PSScriptRoot '使用说明.md')) -replace '\r?\n', "`r`n"
[IO.File]::WriteAllText((Join-Path $PSScriptRoot "$out/使用说明.txt"), $text, [Text.UTF8Encoding]::new($true))

# .NET ZIP 使用 UTF-8 中文文件名，与 macOS 打包脚本一致。
Add-Type -AssemblyName System.IO.Compression.FileSystem
$zip = Join-Path $PSScriptRoot "dist/$name.zip"
if (Test-Path -LiteralPath $zip) { Remove-Item -LiteralPath $zip -Force }
[IO.Compression.ZipFile]::CreateFromDirectory((Join-Path $PSScriptRoot $out), $zip,
    [IO.Compression.CompressionLevel]::Optimal, $true, [Text.Encoding]::UTF8)
$hash = (Get-FileHash -LiteralPath $zip -Algorithm SHA256).Hash.ToLowerInvariant()
[IO.File]::WriteAllText("$zip.sha256", "$hash  $name.zip`n", [Text.Encoding]::UTF8)
Write-Host "Built $zip"
