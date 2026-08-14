<#
.SYNOPSIS
    Install the ThreatOptic CLI on Windows.

.DESCRIPTION
    Downloads the release archive from GitHub, verifies its SHA-256 checksum
    against checksums.txt, extracts it, and adds the install directory to the
    current user's PATH.

        irm https://github.com/ThreatOptic/CLI/releases/latest/download/install.ps1 | iex

.PARAMETER Version
    Tag to install, for example v0.1.0. Defaults to the latest release.

.PARAMETER InstallDir
    Where to put threatoptic.exe. Defaults to
    %LOCALAPPDATA%\Programs\threatoptic.
#>
[CmdletBinding()]
param(
    [string]$Version,
    [string]$InstallDir = (Join-Path $env:LOCALAPPDATA 'Programs\threatoptic')
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$repo = 'ThreatOptic/CLI'
$binary = 'threatoptic'

if (-not $Version) {
    $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$repo/releases/latest" -UseBasicParsing
    $Version = $release.tag_name
}
$number = $Version.TrimStart('v')

# Only amd64 is published for Windows; arm64 machines run it under emulation.
# Must match archives.name_template in .goreleaser.yaml.
$archive = "${binary}_${number}_windows_amd64.zip"
$base = "https://github.com/$repo/releases/download/$Version"

$temp = Join-Path ([System.IO.Path]::GetTempPath()) "threatoptic-install-$([guid]::NewGuid())"
New-Item -ItemType Directory -Path $temp -Force | Out-Null

try {
    Write-Host "Downloading $binary $Version for windows/amd64..."
    $archivePath = Join-Path $temp $archive
    $checksumPath = Join-Path $temp 'checksums.txt'
    Invoke-WebRequest -Uri "$base/$archive" -OutFile $archivePath -UseBasicParsing
    Invoke-WebRequest -Uri "$base/checksums.txt" -OutFile $checksumPath -UseBasicParsing

    $line = Select-String -Path $checksumPath -Pattern ([regex]::Escape($archive)) | Select-Object -First 1
    if (-not $line) {
        throw "$archive is not listed in checksums.txt"
    }
    $expected = ($line.Line -split '\s+')[0]
    $actual = (Get-FileHash -Path $archivePath -Algorithm SHA256).Hash
    if ($expected -ne $actual) {
        throw "Checksum mismatch for ${archive}: expected $expected, got $actual"
    }

    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    Expand-Archive -Path $archivePath -DestinationPath $InstallDir -Force

    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    if ($userPath -notlike "*$InstallDir*") {
        [Environment]::SetEnvironmentVariable('Path', "$userPath;$InstallDir", 'User')
        Write-Host "Added $InstallDir to your PATH. Open a new terminal for it to take effect."
    }
    $env:Path = "$env:Path;$InstallDir"

    $exe = Join-Path $InstallDir "$binary.exe"
    Write-Host "Installed $binary $(& $exe version) to $exe"
    Write-Host "Next:  $binary auth login"
}
finally {
    Remove-Item -Path $temp -Recurse -Force -ErrorAction SilentlyContinue
}
