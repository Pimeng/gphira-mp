#!/usr/bin/env pwsh
#Requires -Version 5.1

param(
    [Parameter(Position = 0, ValueFromRemainingArguments = $true)]
    [string[]]$Targets = @('help')
)

$AppName    = 'gphira-mp'
$BenchName  = 'bench'
$BinDir     = 'build\bin'
$DistDir    = 'build\dist'

$Version = git describe --tags --always --dirty 2>$null
if (-not $Version) { $Version = 'dev' }

$LDFLAGS = "-s -w -X github.com/Pimeng/gphira-mp-next/internal/version.version=$Version"
$GOFLAGS = '-trimpath'

function Show-Help {
    Write-Host 'Usage: .\make.ps1 [target ...]'
    Write-Host ''
    Write-Host 'Targets:'
    Write-Host '  help      Show this help message'
    Write-Host '  server    Build server binary'
    Write-Host '  bench     Build benchmark binary'
    Write-Host '  build     Build server + bench'
    Write-Host '  run       Build and run server with example config'
    Write-Host '  test      Run all tests'
    Write-Host '  fmt       Format Go source files'
    Write-Host '  vet       Run go vet'
    Write-Host '  clean     Remove build artifacts'
    Write-Host '  dist      Build release binaries for current platform'
    Write-Host '  version   Show current version'
}

function Build-Server {
    New-Item -ItemType Directory -Force -Path $BinDir | Out-Null
    $cmd = "go build $GOFLAGS -ldflags `"$LDFLAGS`" -o `"$BinDir\$AppName.exe`" ./cmd/server"
    Invoke-Expression $cmd
    if ($LASTEXITCODE -ne 0) { exit 1 }
    Write-Host "[OK] server => $BinDir\$AppName.exe"
}

function Build-Bench {
    New-Item -ItemType Directory -Force -Path $BinDir | Out-Null
    $cmd = "go build $GOFLAGS -ldflags `"$LDFLAGS`" -o `"$BinDir\$BenchName.exe`" ./cmd/bench"
    Invoke-Expression $cmd
    if ($LASTEXITCODE -ne 0) { exit 1 }
    Write-Host "[OK] bench => $BinDir\$BenchName.exe"
}

function Build-All {
    Build-Server
    Build-Bench
}

function Start-Run {
    Build-Server
    if ($LASTEXITCODE -ne 0) { exit 1 }
    & "$BinDir\$AppName.exe" --config server_config.example.yml
}

function Start-Test {
    go test -v -count=1 ./...
}

function Start-Fmt {
    go fmt ./...
}

function Start-Vet {
    go vet ./...
}

function Start-Clean {
    if (Test-Path 'build') {
        Remove-Item -Recurse -Force 'build'
    }
    Write-Host '[OK] cleaned'
}

function Build-Dist {
    New-Item -ItemType Directory -Force -Path $DistDir | Out-Null
    $cmd1 = "go build $GOFLAGS -ldflags `"$LDFLAGS`" -o `"$DistDir\$AppName-windows-amd64.exe`" ./cmd/server"
    Invoke-Expression $cmd1
    $cmd2 = "go build $GOFLAGS -ldflags `"$LDFLAGS`" -o `"$DistDir\$BenchName-windows-amd64.exe`" ./cmd/bench"
    Invoke-Expression $cmd2
    Write-Host "[OK] dist => $DistDir"
}

function Show-Version {
    Write-Host $Version
}

foreach ($Target in $Targets) {
    switch ($Target.ToLower()) {
        'help'    { Show-Help }
        'server'  { Build-Server }
        'bench'   { Build-Bench }
        'build'   { Build-All }
        'run'     { Start-Run }
        'test'    { Start-Test }
        'fmt'     { Start-Fmt }
        'vet'     { Start-Vet }
        'clean'   { Start-Clean }
        'dist'    { Build-Dist }
        'version' { Show-Version }
        default   { Write-Host "Unknown target: $_"; Show-Help; exit 1 }
    }
}
