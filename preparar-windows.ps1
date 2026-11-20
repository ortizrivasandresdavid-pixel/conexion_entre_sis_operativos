$ErrorActionPreference = 'Stop'

$isAdmin = ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) {
    Write-Host "Solicitando permisos de Administrador..." -ForegroundColor Yellow
    Start-Process powershell.exe -Verb RunAs -ArgumentList "-NoProfile -ExecutionPolicy Bypass -NoExit -File `"$PSCommandPath`""
    exit
}

$root = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $root

function Refresh-EnvPath {
    $machinePath = [System.Environment]::GetEnvironmentVariable("Path", "Machine")
    $userPath = [System.Environment]::GetEnvironmentVariable("Path", "User")
    $env:Path = "$machinePath;$userPath"
}

function Find-Go {
    $cmd = Get-Command go -ErrorAction SilentlyContinue
    if ($cmd) { return $cmd.Source }
    $defaultGo = Join-Path $env:ProgramFiles 'Go\bin\go.exe'
    if (Test-Path $defaultGo) {
        $env:Path = "$(Split-Path $defaultGo);$env:Path"
        return $defaultGo
    }
    return $null
}

function Find-Gcc {
    $candidates = @(
        'C:\msys64\mingw64\bin\gcc.exe',
        'C:\msys64\ucrt64\bin\gcc.exe'
    )
    foreach ($c in $candidates) {
        if (Test-Path $c) { return $c }
    }
    $cmd = Get-Command gcc -ErrorAction SilentlyContinue
    if ($cmd) { return $cmd.Source }
    return $null
}

function Invoke-Winget {
    param([string[]]$Arguments)

    if (-not (Get-Command winget -ErrorAction SilentlyContinue)) {
        throw 'No se encontró winget. Instala App Installer desde Microsoft Store y vuelve a ejecutar este archivo.'
    }
    & winget @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "winget falló ($LASTEXITCODE): $($Arguments -join ' ')"
    }
}

function Get-GoVersion {
    param([string]$Executable)

    $output = & $Executable version
    if ($output -notmatch 'go version go(?<version>\d+\.\d+(?:\.\d+)?)') {
        throw "No se pudo interpretar la versión de Go: $output"
    }
    $version = $Matches.version
    if ($version -match '^\d+\.\d+$') {
        $version = "$version.0"
    }
    return [version]$version
}

$requiredGo = [version]'1.22.0'
$goModPath = Join-Path $root 'go.mod'
if (Test-Path $goModPath) {
    $goModLine = Select-String -Path $goModPath -Pattern '^go\s+(\d+\.\d+(?:\.\d+)?)' | Select-Object -First 1
    if ($goModLine) {
        $verStr = $goModLine.Matches.Groups[1].Value
        if ($verStr -match '^\d+\.\d+$') { $verStr = "$verStr.0" }
        $requiredGo = [version]$verStr
    }
}

Write-Host "[1/4] Verificando Go (mínimo requerido: $requiredGo)..." -ForegroundColor Cyan
$go = Find-Go
if (-not $go) {
    Write-Host "Instalando Go mediante winget..." -ForegroundColor Yellow
    Invoke-Winget @('install', '--id', 'GoLang.Go', '--exact', '--accept-source-agreements', '--accept-package-agreements')
    Refresh-EnvPath
    $go = Find-Go
}
if (-not $go) {
    throw "No se encontró Go después de instalarlo. Cierra esta ventana, abre una nueva terminal y vuelve a ejecutar."
}

$installedGo = Get-GoVersion $go
if ($installedGo -lt $requiredGo) {
    Write-Host "Actualizando Go..." -ForegroundColor Yellow
    Invoke-Winget @('upgrade', '--id', 'GoLang.Go', '--exact', '--accept-source-agreements', '--accept-package-agreements')
    Refresh-EnvPath
    $go = Find-Go
    if (-not $go) {
        throw 'No se encontró Go después de actualizarlo.'
    }
    $installedGo = Get-GoVersion $go
    if ($installedGo -lt $requiredGo) {
        throw "Se requiere Go $requiredGo o posterior; se encontró Go $installedGo."
    }
}
Write-Host "Usando: $(& $go version)" -ForegroundColor Green

Write-Host "[2/4] Verificando compilador C (GCC para Fyne)..." -ForegroundColor Cyan
$gcc = Find-Gcc
if (-not $gcc) {
    $pacman = 'C:\msys64\usr\bin\pacman.exe'
    if (-not (Test-Path $pacman)) {
        Write-Host "Instalando MSYS2..." -ForegroundColor Yellow
        Invoke-Winget @('install', '--id', 'MSYS2.MSYS2', '--exact', '--accept-source-agreements', '--accept-package-agreements')
        Refresh-EnvPath
    }
    if (Test-Path $pacman) {
        Write-Host "Instalando MinGW64 GCC con pacman..." -ForegroundColor Yellow
        & $pacman -S --needed --noconfirm mingw-w64-x86_64-gcc
        if ($LASTEXITCODE -ne 0) {
            throw 'No se pudo instalar GCC con MSYS2/pacman.'
        }
    }
    $gcc = Find-Gcc
}
if (-not $gcc) {
    throw "No se encontró gcc.exe. Instala MSYS2 en C:\msys64 y ejecuta: pacman -S mingw-w64-x86_64-gcc"
}
Write-Host "Usando GCC: $gcc" -ForegroundColor Green

Write-Host "[3/4] Verificando Tailscale..." -ForegroundColor Cyan
$tailscaleExe = Join-Path $env:ProgramFiles 'Tailscale\tailscale.exe'
if (-not (Get-Command tailscale -ErrorAction SilentlyContinue) -and -not (Test-Path $tailscaleExe)) {
    Invoke-Winget @('install', '--id', 'Tailscale.Tailscale', '--exact', '--accept-source-agreements', '--accept-package-agreements')
}

Write-Host "[4/4] Compilando cliente de consola y cliente gráfico (Fyne)..." -ForegroundColor Cyan
$env:CC = $gcc
$env:CGO_ENABLED = '1'
$env:Path = "$(Split-Path -Parent $gcc);$env:Path"

$dist = Join-Path $root 'dist\windows'
New-Item -ItemType Directory -Force -Path $dist | Out-Null

& $go mod download
if ($LASTEXITCODE -ne 0) { throw "Falló 'go mod download' en la raíz." }

if (Test-Path (Join-Path $root 'cliente')) {
    & $go build -o (Join-Path $dist 'cliente.exe') .\cliente
    if ($LASTEXITCODE -ne 0) { throw "Falló la compilación de cliente.exe." }
}

$guiDir = Join-Path $root 'src\opergui'
if (Test-Path $guiDir) {
    & $go -C $guiDir mod download
    if ($LASTEXITCODE -ne 0) { throw "Falló 'go mod download' en src\opergui." }

    & $go -C $guiDir build -o (Join-Path $dist 'cliente-gui.exe') .
    if ($LASTEXITCODE -ne 0) { throw "Falló la compilación de cliente-gui.exe." }
}

Write-Host ""
Write-Host "✅ ¡Proceso completado! Ejecutables creados en: $dist" -ForegroundColor Green
Get-ChildItem $dist | Format-Table Name, Length, LastWriteTime
