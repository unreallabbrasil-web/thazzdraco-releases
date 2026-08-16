# ============================================================================
#  ThazzDraco Optimizer - publicar.ps1
#
#  UM comando: compila, assina, publica no GitHub e os clientes passam a ver
#  "nova versao disponivel" dentro do proprio app.
#
#      .\publicar.ps1 5.0.0
#      .\publicar.ps1 5.0.1 -Notas "Corrige X e Y"
#
#  O que ele NAO faz de proposito: publicar sem conferir. Antes de subir, ele
#  verifica a assinatura com a mesma chave publica embutida no binario. Se o
#  arquivo nao passar aqui, ele tambem nao passaria na maquina do cliente — e
#  publicar um release que todo mundo recusa e pior que nao publicar.
# ============================================================================
param(
    [Parameter(Mandatory = $true, Position = 0)]
    [string]$Versao,
    [string]$Notas = "",
    [switch]$Simular   # mostra tudo que faria, sem publicar nada
)

$ErrorActionPreference = "Stop"
$app = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $app
$repo = "unreallabbrasil-web/thazzdraco-releases"

function Passo($n, $txt) { Write-Host "[$n] $txt" -ForegroundColor Cyan }
function Ok($txt)   { Write-Host "    OK   $txt" -ForegroundColor Green }
function Aviso($txt){ Write-Host "    AVISO $txt" -ForegroundColor Yellow }
function Erro($txt) { Write-Host "    ERRO $txt" -ForegroundColor Red }

# ---------------------------------------------------------------- 1. checagens
Passo 1 "Conferindo antes de mexer em qualquer coisa..."

if ($Versao -notmatch '^\d+\.\d+\.\d+$') { throw "versao precisa ser no formato X.Y.Z (ex.: 5.0.0)" }

if (-not (Get-Command gh -ErrorAction SilentlyContinue)) { throw "gh (GitHub CLI) nao encontrado" }
$auth = gh auth status 2>&1 | Out-String
if ($auth -notmatch "Logged in") { throw "gh nao esta autenticado - rode: gh auth login" }
Ok "gh autenticado"

# a versao precisa ser MAIOR que a ultima publicada, senao o app dos clientes
# compara e conclui que nao ha nada novo (isNewer em updater.go)
$ultima = (gh release list --repo $repo --limit 1 2>$null | Select-Object -First 1) -split "`t" | Select-Object -Index 2
if ($ultima) {
    $u = [version]($ultima -replace '^v','')
    $n = [version]$Versao
    if ($n -le $u) { throw "a versao $Versao nao e maior que a ultima publicada ($ultima) - os clientes nao veriam atualizacao nenhuma" }
    Ok "versao $Versao e maior que a publicada ($ultima)"
}

$sujo = git status --porcelain -- . 2>$null
if ($sujo) { Aviso "ha mudancas nao commitadas em app/ - o release sairia de codigo que nao esta no git" }
$naoEnviados = git log --oneline "@{u}.." 2>$null
if ($naoEnviados) { Aviso "ha commits locais nao enviados - o codigo deste release nao esta no GitHub" }

# ------------------------------------------------------------------ 2. versao
Passo 2 "Gravando a versao $Versao..."
if (-not $Simular) { Set-Content -Path "$app\VERSION" -Value $Versao -Encoding ASCII -NoNewline }
Ok "VERSION = $Versao"

# ------------------------------------------------------------------- 3. build
Passo 3 "Compilando e assinando..."
if ($Simular) {
    Ok "(simulacao) build.ps1 nao foi executado"
} else {
    & "$app\build.ps1"
    if ($LASTEXITCODE -ne 0) { throw "build falhou" }
}

$exe = "$app\dist\ThazzDraco.exe"
$sig = "$exe.sig"
if (-not (Test-Path $exe)) { throw "nao achei $exe" }
if (-not (Test-Path $sig)) {
    Erro "o release NAO esta assinado."
    Erro "Sem o .sig, o app dos clientes RECUSA a atualizacao (e esta certo em recusar)."
    throw "assine antes de publicar - veja docs/DISTRIBUICAO-CONFIANCA.md"
}
Ok ("exe: {0:N1} MB" -f ((Get-Item $exe).Length / 1MB))

# --------------------------------------------------------------- 4. conferir
# Verifica com a MESMA chave publica embutida no binario. Se falhar aqui,
# falharia na maquina do cliente.
Passo 4 "Conferindo a assinatura como o cliente faria..."
$env:GOROOT = "E:\CLAUDE AI\_toolchain\go"
$env:GOPATH = "E:\CLAUDE AI\_toolchain\gopath"
$env:GOCACHE = "E:\CLAUDE AI\_toolchain\gocache"
$env:GOTOOLCHAIN = "local"
$conf = & "$env:GOROOT\bin\go.exe" run ./cmd/assinar -conferir $exe 2>&1 | Out-String
if ($LASTEXITCODE -ne 0) { Erro $conf.Trim(); throw "a assinatura nao confere - NAO publicado" }
Ok $conf.Trim()

# ----------------------------------------------------------------- 5. publicar
Passo 5 "Publicando o release v$Versao..."
if (-not $Notas) {
    $Notas = "Nova versao do ThazzDraco Optimizer.`n`nO proprio app avisa e instala sozinho - basta clicar em atualizar."
}
if ($Simular) {
    Write-Host ""
    Write-Host "  SIMULACAO - nada foi publicado. O comando real seria:" -ForegroundColor Yellow
    Write-Host "  gh release create v$Versao `"$exe`" `"$sig`" --repo $repo --title `"ThazzDraco Optimizer v$Versao`"" -ForegroundColor Gray
    Write-Host ""
    return
}
gh release create "v$Versao" $exe $sig --repo $repo --title "ThazzDraco Optimizer v$Versao" --notes $Notas
if ($LASTEXITCODE -ne 0) { throw "gh release create falhou" }

# --------------------------------------------------------------- 6. confirmar
Passo 6 "Conferindo o que ficou publicado..."
$assets = gh release view "v$Versao" --repo $repo --json assets --jq '.assets[].name' 2>$null
$temExe = $assets -contains "ThazzDraco.exe"
$temSig = $assets -contains "ThazzDraco.exe.sig"
if ($temExe) { Ok "ThazzDraco.exe publicado" } else { Erro "ThazzDraco.exe NAO subiu" }
if ($temSig) { Ok "ThazzDraco.exe.sig publicado" } else { Erro "ThazzDraco.exe.sig NAO subiu - os clientes vao recusar a atualizacao" }

Write-Host ""
if ($temExe -and $temSig) {
    Write-Host "PUBLICADO: v$Versao esta no ar." -ForegroundColor Green
    Write-Host "Os clientes veem o aviso ao abrir o app (ou em ate 4 horas, se estiver aberto)." -ForegroundColor Gray
} else {
    Write-Host "PUBLICACAO INCOMPLETA - confira em https://github.com/$repo/releases" -ForegroundColor Red
}
