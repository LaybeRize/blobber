param(
    [string]$OutputDir = ".\build",
    [string]$BinaryName = "blobber"
)

$ErrorActionPreference = "Stop"

# ── Resolve paths ─────────────────────────────────────────────────────────────
$ProjectDir = (Get-Location).Path
$OutputDir  = (New-Item -ItemType Directory -Force -Path $OutputDir).FullName

Write-Host "Project dir:    $ProjectDir"
Write-Host "Output dir:     $OutputDir"
Write-Host ""

# ── Build Windows DLL ─────────────────────────────────────────────────────────
Write-Host "Building Windows DLL..."
$env:GOOS        = "windows"
$env:GOARCH      = "amd64"
$env:CGO_ENABLED = 1

go build -buildmode=c-shared -o "$OutputDir\$BinaryName.dll" .

if ($LASTEXITCODE -ne 0) {
    Write-Error "Windows build for DLL failed"
    exit 1
}

Write-Host "Building Windows EXE..."

go build -o "$OutputDir\$BinaryName.exe" -tags cli .

if ($LASTEXITCODE -ne 0) {
    Write-Error "Windows build for EXE failed"
    exit 1
}

Write-Host "Windows build DLL -> $OutputDir\$BinaryName.dll"
Write-Host "Windows build EXE -> $OutputDir\$BinaryName.exe"
Write-Host ""