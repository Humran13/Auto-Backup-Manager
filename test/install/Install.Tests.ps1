<#
.SYNOPSIS
    Regression tests for install.ps1's release-version resolution.

.DESCRIPTION
    These exist specifically to prevent a repeat of the real failure found
    during manual Windows testing: a public install command silently
    depending on GitHub's /releases/latest API, which 404s for a repo with
    no stable release and never returns a prerelease even once one exists.

    Run with:
        pwsh -File test/install/Install.Tests.ps1
    (or powershell.exe on Windows PowerShell 5.1)

    No elevation and no network access are required for the resolution
    tests: $script:AbmInstallTesting stops install.ps1 from running its real
    Install-Main when dot-sourced, so this just defines functions to call
    directly.
#>

$ErrorActionPreference = "Stop"
$RepoRoot = Resolve-Path (Join-Path $PSScriptRoot "..\..")
$InstallScript = Join-Path $RepoRoot "install.ps1"

$script:failures = 0
function Write-Pass($name) { Write-Host "ok   - $name" }
function Write-Fail($name, $detail) { Write-Host "FAIL - $name ($detail)"; $script:failures++ }

function Invoke-Case($name, [scriptblock]$body) {
    try {
        if (& $body) { Write-Pass $name } else { Write-Fail $name "assertion returned false" }
    } catch {
        Write-Fail $name $_.Exception.Message
    }
}

# Load install.ps1's functions without running the real install.
$script:AbmInstallTesting = $true
. $InstallScript

$FixtureDir = Join-Path ([System.IO.Path]::GetTempPath()) ("abm-install-test-" + [guid]::NewGuid())
New-Item -ItemType Directory -Path $FixtureDir -Force | Out-Null

# Write-FixtureFile avoids Set-Content's BOM: Windows PowerShell 5.1's
# "-Encoding utf8" always prepends a UTF-8 BOM, which silently breaks
# Invoke-RestMethod's JSON parsing (found while writing this very test) --
# .NET's UTF8Encoding($false) writes plain UTF-8 with no BOM on both
# Windows PowerShell 5.1 and pwsh 7+.
function Write-FixtureFile($Path, $Content) {
    [System.IO.File]::WriteAllText($Path, $Content, (New-Object System.Text.UTF8Encoding $false))
}

try {
    $releaseJsonPath = Join-Path $FixtureDir "release.json"
    Write-FixtureFile $releaseJsonPath '{"channel":"rc","version":"v0.9.0-rc.1","prerelease":true,"note":"test fixture"}'

    $malformedJsonPath = Join-Path $FixtureDir "malformed.json"
    Write-FixtureFile $malformedJsonPath '{ this is not valid json'

    $noVersionJsonPath = Join-Path $FixtureDir "no-version-field.json"
    Write-FixtureFile $noVersionJsonPath '{"channel":"rc"}'

    function ToFileUrl($path) { "file:///" + ($path -replace '\\', '/') }

    Invoke-Case "explicit AbmVersion bypasses RELEASE.json (no network)" {
        $AbmVersion = "v9.9.9"
        $ReleaseJsonUrl = "file:///does/not/exist"
        (Resolve-AbmTag) -eq "v9.9.9"
    }

    Invoke-Case "RELEASE.json resolution returns its version field" {
        $AbmVersion = "auto"
        $ReleaseJsonUrl = ToFileUrl $releaseJsonPath
        (Resolve-AbmTag) -eq "v0.9.0-rc.1"
    }

    Invoke-Case "install.ps1 never depends on GitHub's /releases/latest" {
        $nonCommentLines = Get-Content $InstallScript | Where-Object { $_ -notmatch '^\s*#' }
        -not ($nonCommentLines -match "releases/latest")
    }

    Invoke-Case "malformed RELEASE.json resolves to null, not a crash" {
        $AbmVersion = "auto"
        $ReleaseJsonUrl = ToFileUrl $malformedJsonPath
        $null -eq (Resolve-AbmTag)
    }

    Invoke-Case "RELEASE.json missing the version field resolves to null" {
        $AbmVersion = "auto"
        $ReleaseJsonUrl = ToFileUrl $noVersionJsonPath
        $null -eq (Resolve-AbmTag)
    }

    Invoke-Case "unreachable RELEASE.json resolves to null" {
        $AbmVersion = "auto"
        $ReleaseJsonUrl = "file:///no/such/path.json"
        $null -eq (Resolve-AbmTag)
    }

    Invoke-Case "the real repo RELEASE.json is valid and resolves to a tag" {
        $AbmVersion = "auto"
        $ReleaseJsonUrl = ToFileUrl (Join-Path $RepoRoot "RELEASE.json")
        (Resolve-AbmTag) -like "v*"
    }

    Invoke-Case "Install-Abm fails with the controlled message, never a raw API dump" {
        $AbmVersion = "auto"
        $ReleaseJsonUrl = "file:///no/such/path.json"
        $threw = $false
        $message = ""
        try {
            Install-Abm $FixtureDir
        } catch {
            $threw = $true
            $message = $_.Exception.Message
        }
        $threw -and ($message -like "*No Auto-Backup-Manager release is available for this channel.*")
    }
}
finally {
    Remove-Item -Recurse -Force $FixtureDir -ErrorAction SilentlyContinue
}

Write-Host ""
if ($script:failures -gt 0) {
    Write-Host "$($script:failures) test(s) FAILED"
    exit 1
}
Write-Host "all install.ps1 regression tests passed"
