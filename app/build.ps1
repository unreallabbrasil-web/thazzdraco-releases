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

Write-Host "[1/5] Gerando recurso (icone + manifesto)..." -ForegroundColor Cyan
python resgen.py

Write-Host "[2/5] Compilando ThazzDraco.exe..." -ForegroundColor Cyan
New-Item -ItemType Directory -Force -Path dist | Out-Null
& $go build -ldflags="-H windowsgui -s -w" -o dist\ThazzDraco.exe .
if ($LASTEXITCODE -ne 0) { throw "build falhou" }

$size = [math]::Round((Get-Item dist\ThazzDraco.exe).Length / 1MB, 1)
Write-Host "[3/5] OK -> dist\ThazzDraco.exe ($size MB)" -ForegroundColor Green

# ---------------------------------------------------------------------------
#  Assinatura do release. O app so instala atualizacao assinada pela chave
#  privada correspondente a que esta embutida no binario — sem isso, quem
#  controlar o repositorio de releases executa codigo como admin na maquina de
#  todo cliente. A chave privada fica FORA do repositorio.
# ---------------------------------------------------------------------------
Write-Host "[4/5] Assinando o release..." -ForegroundColor Cyan
$chave = $env:THAZZDRACO_UPDATE_KEY
if (-not $chave) { $chave = "E:\CLAUDE AI\_chaves\thazzdraco-update.key" }
if (Test-Path $chave) {
    & $go run ./cmd/assinar -assinar "$app\dist\ThazzDraco.exe" -chave $chave
    if ($LASTEXITCODE -ne 0) { throw "assinatura falhou" }
    Write-Host "OK -> dist\ThazzDraco.exe.sig (publique os DOIS no release)" -ForegroundColor Green
} else {
    Write-Host "AVISO: chave privada nao encontrada em $chave" -ForegroundColor Yellow
    Write-Host "       O exe compilou, mas NAO esta assinado. Publicar sem o .sig" -ForegroundColor Yellow
    Write-Host "       faz o app dos clientes RECUSAR a atualizacao (o que esta certo)." -ForegroundColor Yellow
    Write-Host "       Defina THAZZDRACO_UPDATE_KEY se a chave estiver em outro lugar." -ForegroundColor Yellow
}

# ---------------------------------------------------------------------------
#  tz.exe e o alvo do atalho da area de trabalho — e o binario que a gente
#  realmente abre no dia a dia. Sem esta copia, o build vai para dist\ e a gente
#  continua testando a versao ANTERIOR sem perceber. Foi exatamente assim que a
#  v5.0.0 foi "testada" rodando um binario 4.4.3.
# ---------------------------------------------------------------------------
Write-Host "[5/5] Atualizando tz.exe (alvo do atalho)..." -ForegroundColor Cyan
try {
    Copy-Item "$app\dist\ThazzDraco.exe" "$app\tz.exe" -Force -ErrorAction Stop
    Write-Host "OK -> tz.exe atualizado" -ForegroundColor Green
} catch {
    # Windows trava o arquivo do processo em execucao. Nao e motivo para derrubar
    # o build — mas TEM que aparecer, senao volta o problema de testar o antigo.
    Write-Host "AVISO: nao deu para atualizar o tz.exe (o app esta aberto?)." -ForegroundColor Yellow
    Write-Host "       Feche o ThazzDraco e rode de novo, ou copie na mao:" -ForegroundColor Yellow
    Write-Host "       Copy-Item dist\ThazzDraco.exe tz.exe -Force" -ForegroundColor Yellow
    Write-Host "       Ate la o atalho continua abrindo a versao ANTERIOR." -ForegroundColor Yellow
}

Write-Host "Executavel unico, portatil, sem dependencias. Copie para o pendrive." -ForegroundColor Gray
