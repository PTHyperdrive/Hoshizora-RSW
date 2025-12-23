# Build p2pnode as Windows DLL
# Requires: Go 1.21+ with CGO enabled, MinGW-w64 (for Windows DLL compilation)
#
# Install MinGW-w64: choco install mingw
# Or download from: https://www.mingw-w64.org/downloads/
#
# ============================================================================
# IMPORTANT: Windows Defender may flag Go DLL builds as false positive
# Run this script as Administrator FIRST to add an exclusion:
#
#   Set-ExecutionPolicy Bypass -Scope Process -Force
#   .\build-dll.ps1 -AddExclusion
#
# Then build normally:
#   .\build-dll.ps1
# ============================================================================

param(
    [switch]$AddExclusion
)

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$goNodeDir = $scriptDir

# Add Windows Defender exclusion
if ($AddExclusion) {
    Write-Host "Adding Windows Defender exclusion for: $goNodeDir" -ForegroundColor Yellow
    try {
        Add-MpPreference -ExclusionPath $goNodeDir -ErrorAction Stop
        Write-Host "[SUCCESS] Exclusion added. You can now build the DLL." -ForegroundColor Green
        Write-Host "Run: .\build-dll.ps1" -ForegroundColor Cyan
    }
    catch {
        Write-Host "[ERROR] Failed to add exclusion. Make sure you run as Administrator." -ForegroundColor Red
        Write-Host "  Right-click PowerShell -> Run as Administrator" -ForegroundColor Yellow
    }
    exit
}

Write-Host "Building p2pnode.dll..." -ForegroundColor Cyan

# Set environment for CGO
$env:CGO_ENABLED = "1"

# Check for GCC
$gcc = Get-Command gcc -ErrorAction SilentlyContinue
if (-not $gcc) {
    Write-Host "[ERROR] GCC not found. Install MinGW-w64:" -ForegroundColor Red
    Write-Host "  choco install mingw" -ForegroundColor Yellow
    Write-Host "  or download from https://www.mingw-w64.org/" -ForegroundColor Yellow
    exit 1
}

Write-Host "Using GCC: $($gcc.Path)" -ForegroundColor Green

# Build the DLL with dll tag
Write-Host "Compiling with -tags=dll -buildmode=c-shared..." -ForegroundColor Gray
go build -tags=dll -buildmode=c-shared -o p2pnode.dll .

if ($LASTEXITCODE -eq 0) {
    Write-Host "[SUCCESS] Built p2pnode.dll" -ForegroundColor Green
    
    # Display generated files
    $dll = Get-Item "p2pnode.dll" -ErrorAction SilentlyContinue
    $header = Get-Item "p2pnode.h" -ErrorAction SilentlyContinue
    
    if ($dll) {
        Write-Host "  DLL: $($dll.FullName) ($([Math]::Round($dll.Length / 1MB, 2)) MB)" -ForegroundColor White
    }
    if ($header) {
        Write-Host "  Header: $($header.FullName)" -ForegroundColor White
    }

    # Copy to Hoshizora output
    $hoshizoraDir = Join-Path $scriptDir "..\Hoshizora\bin\Debug\net8.0-windows"
    if (Test-Path $hoshizoraDir) {
        Copy-Item "p2pnode.dll" $hoshizoraDir -Force
        Write-Host "  Copied to: $hoshizoraDir" -ForegroundColor Cyan
    }
}
else {
    Write-Host "[ERROR] Build failed with exit code $LASTEXITCODE" -ForegroundColor Red
    Write-Host ""
    Write-Host "If blocked by Windows Defender, run as Administrator:" -ForegroundColor Yellow
    Write-Host "  .\build-dll.ps1 -AddExclusion" -ForegroundColor Cyan
    exit $LASTEXITCODE
}
