import subprocess
from pathlib import Path
import os
import sys

SIGN_SCRIPT = r"""
$DllPath = "{DLL_PATH}"

# Check if we already have a dev cert
$cert = Get-ChildItem "Cert:\CurrentUser\My" | Where-Object { $_.Subject -eq "CN=BlobberDllCert" } | Select-Object -First 1

if (-not $cert) {
    Write-Host "Creating new self-signed certificate..."
    $cert = New-SelfSignedCertificate `
        -Subject "CN=BlobberDllCert" `
        -Type CodeSigning `
        -CertStoreLocation "Cert:\CurrentUser\My" `
        -HashAlgorithm SHA256

    $certPath = "$env:TEMP\blobberdllcert.cer"
    Export-Certificate -Cert $cert -FilePath $certPath | Out-Null

    Import-Certificate -FilePath $certPath -CertStoreLocation "Cert:\CurrentUser\TrustedPublisher" | Out-Null
    Import-Certificate -FilePath $certPath -CertStoreLocation "Cert:\CurrentUser\Root" | Out-Null

    Remove-Item $certPath
    Write-Host "Certificate created and trusted."
} else {
    Write-Host "Reusing existing BlobberDllCert certificate."
}

$result = Set-AuthenticodeSignature `
    -FilePath $DllPath `
    -Certificate $cert `
    -HashAlgorithm SHA256

if ($result.Status -eq "Valid") {
    Write-Host "Successfully signed: $DllPath"
    exit 0
} else {
    Write-Host "Signing failed: $($result.StatusMessage)"
    exit 1
}
"""

def sign_dll(dll_path: str) -> bool:
    path = Path(dll_path)
    if not path.exists():
        print(f"Error: {dll_path} does not exist")
        return False

    result = subprocess.run(
        [
            "powershell",
            "-NoProfile",
            "-ExecutionPolicy", "Bypass",
            "-Command", SIGN_SCRIPT.replace("{DLL_PATH}", str(path.resolve()).replace(os.sep, "/")),
        ],
        capture_output=True,
        text=True
    )

    print(result.stdout.strip())
    if result.returncode != 0:
        print(f"Error: {result.stderr.strip()}", file=sys.stderr)
        return False
    return True