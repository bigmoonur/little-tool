# Windows 本地或 GitHub Actions：检查、编译、生成安装包和发布 ZIP。
param([switch]$WithObfuscation, [switch]$WithoutObfuscation)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
Set-Location $PSScriptRoot

function Invoke-Checked {
    param([string]$Program, [string[]]$Arguments)
    & $Program @Arguments
    if ($LASTEXITCODE -ne 0) { throw "$Program failed with exit code $LASTEXITCODE" }
}

if ($WithObfuscation -and $WithoutObfuscation) { throw 'Choose only one obfuscation option' }
$useGarble = [bool]$WithObfuscation
if ($env:GOFLAGS -match 'devlicense|debug') { throw 'Release builds cannot use development or debug flags' }
$env:PATH += ";$(go env GOPATH)\bin;${env:ProgramFiles(x86)}\NSIS"
if ($useGarble) { Get-Command garble -ErrorAction Stop | Out-Null }

Push-Location frontend
try {
    Invoke-Checked -Program npm -Arguments @('ci')
    Invoke-Checked -Program npm -Arguments @('run', 'build')
} finally { Pop-Location }

Invoke-Checked -Program go -Arguments @('vet', './...')
Invoke-Checked -Program go -Arguments @('test', '-race', './...')
Invoke-Checked -Program go -Arguments @('-C', '../shared/license', 'vet', './...')
Invoke-Checked -Program go -Arguments @('-C', '../shared/license', 'test', '-race', './...')
# 即使误传开发免授权标签，正式版的编译期常量也必须仍为 false。
Invoke-Checked -Program go -Arguments @('test', '-tags', 'production,devlicense', './...')
$wailsArguments = @('build', '-platform', 'windows/amd64', '-nsis', '-clean', '-trimpath',
    '-tags', 'production', '-ldflags', '-s -w')
if ($useGarble) {
    $wailsArguments += @('-obfuscated', '-garbleargs', '-literals -seed=random')
}
Write-Host "Garble enabled: $useGarble"
Invoke-Checked -Program wails -Arguments $wailsArguments

$config = Get-Content -LiteralPath wails.json -Raw -Encoding utf8 | ConvertFrom-Json
$version = $config.info.productVersion
$program = $config.outputfilename
$name = "$program-$version-windows"
if (-not $useGarble) { $name += '-no-garble' }
$out = Join-Path dist $name
if (Test-Path -LiteralPath $out) { Remove-Item -LiteralPath $out -Recurse -Force }
New-Item -ItemType Directory -Path $out -Force | Out-Null
Copy-Item -LiteralPath "build/bin/$program.exe" -Destination $out
Copy-Item -LiteralPath "build/bin/$program-amd64-installer.exe" -Destination (Join-Path $out "$program-$version-安装包.exe")
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
$sourceCommit = git rev-parse HEAD
if ($LASTEXITCODE -ne 0) { throw 'Could not identify the source commit' }
[ordered]@{
    Version = $version
    SourceCommit = $sourceCommit
    GarbleEnabled = $useGarble
    WailsArguments = $wailsArguments
    Package = "$name.zip"
    SHA256 = $hash
} | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath 'dist/build-info.json' -Encoding utf8
Write-Host "Built $zip"
