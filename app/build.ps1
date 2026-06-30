# ============================================================================
#  ThazzDraco Optimizer - build.ps1
#  Compila o executavel unico portatil (dist\ThazzDraco.exe) com icone do
#  dragao + manifesto requireAdministrator (escudo UAC) + sem console.
#  Usa o toolchain Go portatil em E:\CLAUDE AI\_toolchain (nao polui o sistema).
# ============================================================================
$ErrorActionPreference = "Stop"
$app = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $app

$env:GOROOT     = "E:\CLAUDE AI\_toolchain\go"
$env:GOPATH     = "E:\CLAUDE AI\_toolchain\gopath"
$env:GOCACHE    = "E:\CLAUDE AI\_toolchain\gocache"
$env:GOTOOLCHAIN = "local"
$go = "$env:GOROOT\bin\go.exe"

Write-Host "[1/3] Gerando recurso (icone + manifesto)..." -ForegroundColor Cyan
python resgen.py

Write-Host "[2/3] Compilando ThazzDraco.exe..." -ForegroundColor Cyan
New-Item -ItemType Directory -Force -Path dist | Out-Null
& $go build -ldflags="-H windowsgui -s -w" -o dist\ThazzDraco.exe .
if ($LASTEXITCODE -ne 0) { throw "build falhou" }

$size = [math]::Round((Get-Item dist\ThazzDraco.exe).Length / 1MB, 1)
Write-Host "[3/3] OK -> dist\ThazzDraco.exe ($size MB)" -ForegroundColor Green
Write-Host "Executavel unico, portatil, sem dependencias. Copie para o pendrive." -ForegroundColor Gray
