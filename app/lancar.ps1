param()

# Script de DESENVOLVIMENTO: compila e abre o app rapidamente. Compila para tz.exe
# (alvo do atalho da area de trabalho, ignorado pelo git) — NUNCA sobrescreve o
# entregavel dist\ThazzDraco.exe, que so o build.ps1 gera. Usa as MESMAS flags de
# release (-H windowsgui -s -w); antes buildava sem elas, entao o exe saia com
# console e sem strip.

$appDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$go      = "E:\CLAUDE AI\_toolchain\go\bin\go.exe"
$out     = "$appDir\tz.exe"

$env:GOTOOLCHAIN = "local"
Set-Location $appDir

if (-not (Test-Path "$appDir\resource.syso")) {
    Add-Type -AssemblyName System.Windows.Forms
    [System.Windows.Forms.MessageBox]::Show("resource.syso nao encontrado.`nVerifique se nao foi renomeado para .bak.", "ThazzDraco - Erro", "OK", "Error")
    exit 1
}

$result = & $go build -ldflags="-H windowsgui -s -w" -o $out . 2>&1
if ($LASTEXITCODE -ne 0) {
    Add-Type -AssemblyName System.Windows.Forms
    [System.Windows.Forms.MessageBox]::Show("Erro na compilacao:`n$result", "ThazzDraco - Erro de build", "OK", "Error")
    exit 1
}

Start-Process $out