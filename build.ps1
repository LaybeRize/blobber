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
$env:GOOS   = "windows"
$env:GOARCH = "amd64"

go build -mod=vendor -buildmode=c-shared -o "$OutputDir\$BinaryName.dll" .

if ($LASTEXITCODE -ne 0) {
    Write-Error "Windows build failed"
    exit 1
}
Write-Host "Windows build OK -> $OutputDir\$BinaryName.dll"
Write-Host ""