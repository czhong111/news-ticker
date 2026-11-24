$ErrorActionPreference = 'Stop'
$exe = Join-Path $PSScriptRoot 'ticker.exe'
if (-not (Test-Path -LiteralPath $exe)) {
    throw "找不到 $exe，请先运行 build.bat。"
}
Start-Process -FilePath $exe
