# IPAlpha setup for Windows (PowerShell 5.1+). Downloads the ipalpha tool and runs its setup
# (tools via winget, repositories, .env files, AI). Safe to run again.
#   irm https://raw.githubusercontent.com/ipalpha-dev/.github/master/install.ps1 | iex
$ErrorActionPreference = 'Stop'
[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

$base = if ($env:IPALPHA_RELEASE_URL) { $env:IPALPHA_RELEASE_URL } else { 'https://github.com/ipalpha-dev/.github/releases/latest/download' }
$arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64' -or $env:PROCESSOR_ARCHITEW6432 -eq 'ARM64') { 'arm64' } else { 'amd64' }
$asset = "ipalpha-windows-$arch.exe"

$tmp = Join-Path ([IO.Path]::GetTempPath()) ("ipalpha-install-" + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $tmp | Out-Null
try {
    Write-Host ""
    Write-Host "IPAlpha - baixando a ferramenta / downloading the tool ($asset)" -ForegroundColor Cyan
    $exe = Join-Path $tmp 'ipalpha.exe'
    $sums = Join-Path $tmp 'checksums.txt'
    try {
        $ProgressPreference = 'SilentlyContinue'
        Invoke-WebRequest -UseBasicParsing -Uri "$base/$asset" -OutFile $exe
        Invoke-WebRequest -UseBasicParsing -Uri "$base/checksums.txt" -OutFile $sums
    } catch {
        Write-Host "Falha no download / download failed: $base/$asset" -ForegroundColor Red
        Write-Host "Confira a internet e tente de novo / check the connection and try again." -ForegroundColor Red
        exit 1
    }
    $want = (Get-Content $sums | Where-Object { $_ -match " $([regex]::Escape($asset))$" } | ForEach-Object { ($_ -split '\s+')[0] }) | Select-Object -First 1
    $got = (Get-FileHash -Algorithm SHA256 $exe).Hash.ToLower()
    if (-not $want -or $want.ToLower() -ne $got) {
        Write-Host "Arquivo corrompido (checksum) / corrupted download (checksum)" -ForegroundColor Red
        exit 1
    }
    # Windows Terminal and PowerShell 7 show colors and the full-screen panel best.
    & $exe setup @args
    exit $LASTEXITCODE
} finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}
