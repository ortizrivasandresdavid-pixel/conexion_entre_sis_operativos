#Requires -RunAsAdministrator

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $MyInvocation.MyCommand.Path
$msysRoot = 'C:\msys64'
$gcc = Join-Path $msysRoot 'ucrt64\bin\gcc.exe'
$goBin = Join-Path $env:ProgramFiles 'Go\bin'
$requiredGo = [version]'1.27.3'

function Invoke-Winget {
    param([string[]]$Arguments)

    & winget @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "winget falló ($LASTEXITCODE): $($Arguments -join ' ')"
    }
}

function Get-GoExecutable {
    $candidate = Get-Command go -ErrorAction SilentlyContinue
    if ($candidate) {
        return $candidate.Source
    }

    $installedGo = Join-Path $goBin 'go.exe'
    if (Test-Path $installedGo) {
        $env:Path = "$goBin;$env:Path"
        return $installedGo
    }

    return $null
}

if (-not (Get-Command winget -ErrorAction SilentlyContinue)) {
    throw 'No se encontró winget. Instala App Installer desde Microsoft Store y vuelve a ejecutar este archivo.'
}

if (-not (Get-GoExecutable)) {
    Invoke-Winget @('install', '--id', 'GoLang.Go', '--exact', '--accept-source-agreements', '--accept-package-agreements')
    $env:Path = "$goBin;$env:Path"
}

$go = Get-GoExecutable
if (-not $go) {
    throw 'No se encontró Go después de instalarlo. Cierra y vuelve a abrir PowerShell, y ejecuta el archivo otra vez.'
}

$goOutput = & $go version
if ($goOutput -notmatch 'go version go(?<version>\d+\.\d+\.\d+)') {
    throw "No se pudo interpretar la versión de Go: $goOutput"
}
$installedGoVersion = [version]$Matches.version
if ($installedGoVersion -lt $requiredGo) {
    & winget upgrade --id GoLang.Go --exact --accept-source-agreements --accept-package-agreements
    $env:Path = "$goBin;$env:Path"
    $go = Get-GoExecutable
    $goOutput = & $go version
    if ($goOutput -notmatch 'go version go(?<version>\d+\.\d+\.\d+)') {
        throw "No se pudo interpretar la versión de Go: $goOutput"
    }
    $installedGoVersion = [version]$Matches.version
    if ($installedGoVersion -lt $requiredGo) {
        throw "Se requiere Go $requiredGo o posterior; se encontró $installedGoVersion. Actualiza Go y vuelve a ejecutar."
    }
}

$tailscaleExe = Join-Path $env:ProgramFiles 'Tailscale\tailscale.exe'
if (-not (Get-Command tailscale -ErrorAction SilentlyContinue) -and -not (Test-Path $tailscaleExe)) {
    Invoke-Winget @('install', '--id', 'Tailscale.Tailscale', '--exact', '--accept-source-agreements', '--accept-package-agreements')
}

if (-not (Test-Path $gcc)) {
    if (-not (Test-Path (Join-Path $msysRoot 'usr\bin\pacman.exe'))) {
        Invoke-Winget @('install', '--id', 'MSYS2.MSYS2', '--exact', '--accept-source-agreements', '--accept-package-agreements')
    }
    $pacman = Join-Path $msysRoot 'usr\bin\pacman.exe'
    if (-not (Test-Path $pacman)) {
        throw "No se encontró MSYS2 en $msysRoot. Instálalo allí o ajusta la ruta en preparar-windows.ps1."
    }
    & $pacman -S --needed --noconfirm mingw-w64-ucrt-x86_64-gcc
    if ($LASTEXITCODE -ne 0) {
        throw 'No se pudo instalar GCC con MSYS2/pacman.'
    }
}

$env:CC = $gcc
$env:CGO_ENABLED = '1'
$env:Path = "$(Split-Path -Parent $gcc);$env:Path"
$dist = Join-Path $root 'dist\windows'
New-Item -ItemType Directory -Force -Path $dist | Out-Null

Push-Location $root
try {
    & $go mod download
    if ($LASTEXITCODE -ne 0) {
        throw 'No se pudieron descargar las dependencias de Go del cliente.'
    }
    & $go build -o (Join-Path $dist 'cliente.exe') .\cliente
    if ($LASTEXITCODE -ne 0) {
        throw 'Falló la compilación del cliente de consola.'
    }

    & $go -C .\src\opergui mod download
    if ($LASTEXITCODE -ne 0) {
        throw 'No se pudieron descargar las dependencias de Go de la interfaz gráfica.'
    }
    & $go -C .\src\opergui build -o ..\..\dist\windows\cliente-gui.exe .
    if ($LASTEXITCODE -ne 0) {
        throw 'Falló la compilación del cliente gráfico.'
    }
}
finally {
    Pop-Location
}

Write-Host ''
Write-Host 'Listo. Ejecutables creados en dist\windows:'
Write-Host '  cliente.exe      (consola)'
Write-Host '  cliente-gui.exe  (interfaz gráfica)'
Write-Host 'Inicia sesión en Tailscale en Windows antes de conectarte.'
