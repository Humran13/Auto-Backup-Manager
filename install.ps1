#Requires -RunAsAdministrator
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
    [string]$AbmVersion = "latest"
)

$ErrorActionPreference = "Stop"

function Write-Log($msg) { Write-Host "[abm-install] $msg" }
function Fail($msg) { Write-Error "[abm-install] ERROR: $msg"; exit 1 }

$Repo = "Humran13/Auto-Backup-Manager"
$BinDir = "C:\Program Files\Auto-Backup-Manager"
$ConfigDir = "C:\ProgramData\Auto-Backup-Manager"

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
    # tokens/S3/SFTP credentials, and must not be readable by ordinary users
    # on a shared machine. /inheritance:r drops inherited (more permissive)
    # ACEs from ProgramData before granting the explicit ones below.
    & icacls $ConfigDir /inheritance:r | Out-Null
    & icacls $ConfigDir /grant:r "SYSTEM:(OI)(CI)F" "BUILTIN\Administrators:(OI)(CI)F" | Out-Null

    function Get-VerifiedFile($Url, $SumsUrl, $FileName) {
        Write-Log "downloading $FileName"
        $filePath = Join-Path $TmpDir $FileName
        Invoke-WebRequest -Uri $Url -OutFile $filePath -UseBasicParsing
        $sumsPath = Join-Path $TmpDir "SHA256SUMS"
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

    # --- restic ---
    $resticInstalled = $false
    if (Get-Command restic -ErrorAction SilentlyContinue) {
        $v = (& restic version) 2>$null
        if ($v -match [regex]::Escape($ResticVersion)) { $resticInstalled = $true }
    }
    if (-not $resticInstalled) {
        Write-Log "installing restic $ResticVersion"
        $base = "restic_${ResticVersion}_windows_amd64"
        $url = "https://github.com/restic/restic/releases/download/v$ResticVersion/$base.zip"
        $sums = "https://github.com/restic/restic/releases/download/v$ResticVersion/SHA256SUMS"
        $zip = Get-VerifiedFile $url $sums "$base.zip"
        Expand-Archive -Path $zip -DestinationPath $TmpDir -Force
        Copy-Item (Join-Path $TmpDir "$base.exe") (Join-Path $BinDir "restic.exe") -Force
    } else {
        Write-Log "restic $ResticVersion already installed"
    }

    # --- rclone ---
    $rcloneInstalled = $false
    if (Get-Command rclone -ErrorAction SilentlyContinue) {
        $v = (& rclone version) 2>$null
        if ($v -match [regex]::Escape($RcloneVersion)) { $rcloneInstalled = $true }
    }
    if (-not $rcloneInstalled) {
        Write-Log "installing rclone $RcloneVersion"
        $base = "rclone-v$RcloneVersion-windows-amd64"
        $url = "https://downloads.rclone.org/v$RcloneVersion/$base.zip"
        $sums = "https://downloads.rclone.org/v$RcloneVersion/SHA256SUMS"
        $zip = Get-VerifiedFile $url $sums "$base.zip"
        Expand-Archive -Path $zip -DestinationPath $TmpDir -Force
        Copy-Item (Join-Path $TmpDir "$base\rclone.exe") (Join-Path $BinDir "rclone.exe") -Force
    } else {
        Write-Log "rclone $RcloneVersion already installed"
    }

    # --- abm itself ---
    if ((Test-Path ".\cmd\abm\main.go") -and (Get-Command go -ErrorAction SilentlyContinue)) {
        Write-Log "building abm from local source"
        & go build -o (Join-Path $BinDir "abm.exe") .\cmd\abm
        if ($LASTEXITCODE -ne 0) { Fail "go build failed" }
    } else {
        if ($AbmVersion -eq "latest") {
            $apiUrl = "https://api.github.com/repos/$Repo/releases/latest"
        } else {
            $apiUrl = "https://api.github.com/repos/$Repo/releases/tags/$AbmVersion"
        }
        $release = Invoke-RestMethod -Uri $apiUrl -UseBasicParsing
        $tag = $release.tag_name
        if (-not $tag) { Fail "no published Auto-Backup-Manager release found; clone the repo and re-run this script from its root to build from source instead" }

        Write-Log "installing Auto-Backup-Manager $tag"
        $assetName = "abm_${tag}_windows_amd64.zip"
        $url = "https://github.com/$Repo/releases/download/$tag/$assetName"
        $sums = "https://github.com/$Repo/releases/download/$tag/checksums.txt"
        $zip = Get-VerifiedFile $url $sums $assetName
        Expand-Archive -Path $zip -DestinationPath $TmpDir -Force
        Copy-Item (Join-Path $TmpDir "abm.exe") (Join-Path $BinDir "abm.exe") -Force
    }

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
    Write-Log "  abm storage add --type <s3|sftp|local|...> ..."
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
