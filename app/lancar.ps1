param()

$appDir = "E:\CLAUDE AI\OPTM\app"
$go     = "E:\CLAUDE AI\_toolchain\go\bin\go.exe"
$out    = "$appDir\dist\ThazzDraco.exe"

Set-Location $appDir

if (-not (Test-Path "$appDir\resource.syso")) {
    Add-Type -AssemblyName System.Windows.Forms
    [System.Windows.Forms.MessageBox]::Show("resource.syso nao encontrado.`nVerifique se nao foi renomeado para .bak.", "ThazzDraco - Erro", "OK", "Error")
    exit 1
}

$result = & $go build -o $out . 2>&1
if ($LASTEXITCODE -ne 0) {
    Add-Type -AssemblyName System.Windows.Forms
    [System.Windows.Forms.MessageBox]::Show("Erro na compilacao:`n$result", "ThazzDraco - Erro de build", "OK", "Error")
    exit 1
}

Start-Process $out