<#
.SYNOPSIS
    Auto-Backup-Manager installer for Windows.

.DESCRIPTION
    Usage (elevated PowerShell):
        irm https://raw.githubusercontent.com/Humran13/Auto-Backup-Manager/main/install.ps1 | iex

    Safer alternative (recommended): download, read, then run:
        Invoke-WebRequest -Uri https://raw.githubusercontent.com/Humran13/Auto-Backup-Manager/main/install.ps1 -OutFile install.ps1
        Get-Content install.ps1 | more
        .\install.ps1

    Idempotent: re-running upgrades an existing install in place. Never
    touches C:\ProgramData\Auto-Backup-Manager\config.yaml, its secrets, or
    any backup repository. 'abm uninstall' only removes the scheduled task,
    never a backup repository.
#>
[CmdletBinding()]
param(
    [string]$ResticVersion = "0.19.1",
    [string]$RcloneVersion = "1.75.1",
    # AbmVersion pins an exact release tag (e.g. "v0.9.0-rc.1"), bypassing
    # RELEASE.json entirely. Leave as "auto" to resolve the project's
    # currently-approved release via RELEASE.json instead -- see
    # Resolve-AbmTag below for exactly why this project never queries
    # GitHub's own /releases/latest API endpoint.
    [string]$AbmVersion = "auto",
    # Overridable only so Install.Tests.ps1 can point this at a local
    # fixture file without touching the network; real installs should never
    # need to set this.
    [string]$ReleaseJsonUrl = "https://raw.githubusercontent.com/Humran13/Auto-Backup-Manager/main/RELEASE.json"
)

$ErrorActionPreference = "Stop"

function Write-Log($msg) { Write-Host "[abm-install] $msg" }
function Fail($msg) { Write-Error "[abm-install] ERROR: $msg"; exit 1 }

$Repo = "Humran13/Auto-Backup-Manager"
$BinDir = "C:\Program Files\Auto-Backup-Manager"
$ConfigDir = "C:\ProgramData\Auto-Backup-Manager"

# Resolve-AbmTag decides which release tag to install, in order:
#   1. $AbmVersion, if explicitly set to something other than "auto"
#   2. RELEASE.json's "version" field, fetched from this repo's own main
#      branch
# It deliberately never calls GitHub's /repos/{repo}/releases/latest API:
# that endpoint 404s outright for a repository with no stable release at
# all, and even once one exists, it never returns a prerelease -- exactly
# the failure a real Windows install hit (see docs/TROUBLESHOOTING.md). A
# repo-local metadata file sidesteps both problems and never depends on an
# ephemeral GitHub Actions artifact URL. Returns the resolved tag, or $null
# on failure -- callers must check for that and fail with their own clear
# message rather than exposing a raw API response.
# Get-VersionFromReleaseJson parses RELEASE.json's content (already fetched,
# as a string) and extracts its "version" field, or returns $null for
# malformed JSON or a missing field. Split out from Resolve-AbmTag so this
# parsing logic is unit-testable with a literal string, independent of
# however the content was fetched -- Invoke-RestMethod's HttpClient-based
# implementation on PowerShell 7+ doesn't support file:// URIs at all
# (unlike Windows PowerShell 5.1's legacy WebRequest-based one), which would
# otherwise make the happy-path untestable without a real HTTP server.
function Get-VersionFromReleaseJson($Json) {
    try {
        $release = $Json | ConvertFrom-Json
    } catch {
        return $null
    }
    if (-not $release.version) { return $null }
    return $release.version
}

function Resolve-AbmTag {
    if ($AbmVersion -ne "auto") {
        return $AbmVersion
    }
    try {
        $json = (Invoke-WebRequest -Uri $ReleaseJsonUrl -UseBasicParsing).Content
    } catch {
        return $null
    }
    return Get-VersionFromReleaseJson $json
}

# Get-VerifiedFile downloads $Url and its published checksum file, and
# refuses to continue if the checksum doesn't match -- an installer that
# fetches arbitrary binaries over HTTPS without verifying them is exactly
# the supply chain risk this project's own threat model calls out. Fails
# closed: a missing asset, a missing checksum entry, or a mismatch are all
# fatal, with no insecure fallback. (Defined at script scope, independent of
# $TmpDir, so Install.Tests.ps1 can exercise it in isolation.)
function Get-VerifiedFile($Url, $SumsUrl, $FileName, $DestDir) {
    Write-Log "downloading $FileName"
    $filePath = Join-Path $DestDir $FileName
    Invoke-WebRequest -Uri $Url -OutFile $filePath -UseBasicParsing
    $sumsPath = Join-Path $DestDir "SHA256SUMS"
    Invoke-WebRequest -Uri $SumsUrl -OutFile $sumsPath -UseBasicParsing

    $expectedLine = Select-String -Path $sumsPath -Pattern ([regex]::Escape($FileName)) | Select-Object -First 1
    if (-not $expectedLine) { Fail "no checksum entry found for $FileName" }
    $expected = ($expectedLine.Line -split '\s+')[0].ToLower()
    $actual = (Get-FileHash -Path $filePath -Algorithm SHA256).Hash.ToLower()
    if ($expected -ne $actual) {
        Fail "checksum mismatch for $FileName (expected $expected, got $actual)"
    }
    return $filePath
}

function Install-Restic($TmpDir) {
    $resticInstalled = $false
    if (Get-Command restic -ErrorAction SilentlyContinue) {
        $v = (& restic version) 2>$null
        if ($v -match [regex]::Escape($ResticVersion)) { $resticInstalled = $true }
    }
    if ($resticInstalled) {
        Write-Log "restic $ResticVersion already installed"
        return
    }
    Write-Log "installing restic $ResticVersion"
    $base = "restic_${ResticVersion}_windows_amd64"
    $url = "https://github.com/restic/restic/releases/download/v$ResticVersion/$base.zip"
    $sums = "https://github.com/restic/restic/releases/download/v$ResticVersion/SHA256SUMS"
    $zip = Get-VerifiedFile $url $sums "$base.zip" $TmpDir
    Expand-Archive -Path $zip -DestinationPath $TmpDir -Force
    Copy-Item (Join-Path $TmpDir "$base.exe") (Join-Path $BinDir "restic.exe") -Force
}

function Install-Rclone($TmpDir) {
    $rcloneInstalled = $false
    if (Get-Command rclone -ErrorAction SilentlyContinue) {
        $v = (& rclone version) 2>$null
        if ($v -match [regex]::Escape($RcloneVersion)) { $rcloneInstalled = $true }
    }
    if ($rcloneInstalled) {
        Write-Log "rclone $RcloneVersion already installed"
        return
    }
    Write-Log "installing rclone $RcloneVersion"
    $base = "rclone-v$RcloneVersion-windows-amd64"
    $url = "https://downloads.rclone.org/v$RcloneVersion/$base.zip"
    $sums = "https://downloads.rclone.org/v$RcloneVersion/SHA256SUMS"
    $zip = Get-VerifiedFile $url $sums "$base.zip" $TmpDir
    Expand-Archive -Path $zip -DestinationPath $TmpDir -Force
    Copy-Item (Join-Path $TmpDir "$base\rclone.exe") (Join-Path $BinDir "rclone.exe") -Force
}

function Install-Abm($TmpDir) {
    if ((Test-Path ".\cmd\abm\main.go") -and (Get-Command go -ErrorAction SilentlyContinue)) {
        Write-Log "building abm from local source"
        & go build -o (Join-Path $BinDir "abm.exe") .\cmd\abm
        if ($LASTEXITCODE -ne 0) { Fail "go build failed" }
        return
    }

    $tag = Resolve-AbmTag
    if (-not $tag) { Fail "No Auto-Backup-Manager release is available for this channel." }

    $abmPath = Join-Path $BinDir "abm.exe"
    if (Test-Path $abmPath) {
        $v = (& $abmPath --version) 2>$null
        if ($v -match [regex]::Escape($tag)) {
            Write-Log "Auto-Backup-Manager $tag already installed"
            return
        }
    }

    Write-Log "installing Auto-Backup-Manager $tag"
    $assetName = "abm_${tag}_windows_amd64.zip"
    $url = "https://github.com/$Repo/releases/download/$tag/$assetName"
    $sums = "https://github.com/$Repo/releases/download/$tag/checksums.txt"
    $zip = Get-VerifiedFile $url $sums $assetName $TmpDir
    Expand-Archive -Path $zip -DestinationPath $TmpDir -Force
    Copy-Item (Join-Path $TmpDir "abm.exe") $abmPath -Force
}

function Install-Main {
    $principal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
    if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
        Fail "this installer must be run from an elevated (Administrator) PowerShell prompt"
    }

    $TmpDir = Join-Path $env:TEMP ("abm-install-" + [guid]::NewGuid())
    New-Item -ItemType Directory -Path $TmpDir -Force | Out-Null
    try {
        New-Item -ItemType Directory -Force -Path $BinDir | Out-Null
        New-Item -ItemType Directory -Force -Path $ConfigDir, "$ConfigDir\secrets", "$ConfigDir\state", "$ConfigDir\locks", "$ConfigDir\dumps", "$ConfigDir\logs" | Out-Null

        # Lock down config/secrets to Administrators + SYSTEM only: these hold
        # restic repository passwords, database credentials, and rclone OAuth
        # tokens/S3/SFTP credentials, and must not be readable by ordinary
        # users on a shared machine. /inheritance:r drops inherited (more
        # permissive) ACEs from ProgramData before granting the explicit ones
        # below.
        & icacls $ConfigDir /inheritance:r | Out-Null
        & icacls $ConfigDir /grant:r "SYSTEM:(OI)(CI)F" "BUILTIN\Administrators:(OI)(CI)F" | Out-Null

        Install-Restic $TmpDir
        Install-Rclone $TmpDir
        Install-Abm $TmpDir

        # Add BinDir to the machine PATH if not already present.
        $machinePath = [Environment]::GetEnvironmentVariable("Path", "Machine")
        if ($machinePath -notlike "*$BinDir*") {
            [Environment]::SetEnvironmentVariable("Path", "$machinePath;$BinDir", "Machine")
            Write-Log "added $BinDir to the system PATH (restart your shell to pick it up)"
        }

        Write-Log "install complete."
        Write-Log ""
        Write-Log "Next steps (new elevated PowerShell window so PATH updates):"
        Write-Log "  abm setup"
        Write-Log "  abm storage providers"
        Write-Log "  abm storage add --provider <id> --name <name> ..."
        Write-Log "  abm job add --name my-job --source C:\Data --destination <name>"
        Write-Log "  abm backup now my-job"
        Write-Log "  abm schedule set"
        Write-Log ""
        Write-Log "Uninstalling later ('abm uninstall') removes only the scheduled"
        Write-Log "task; it never deletes config, secrets, or backup repositories."
    }
    finally {
        Remove-Item -Recurse -Force $TmpDir -ErrorAction SilentlyContinue
    }
}

# Guarded so Install.Tests.ps1 can dot-source this file (to unit-test
# Resolve-AbmTag/Get-VerifiedFile in isolation, with no elevation and no
# network access required for most cases) without running the real
# install -- every real side effect (elevation check, directory creation,
# ACLs, downloads) lives inside Install-Main, never at parse time.
if (-not $script:AbmInstallTesting) {
    Install-Main
}
