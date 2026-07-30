#Requires -Version 5.1
<#
.SYNOPSIS
  Builds ANSM distribution artifacts.

.DESCRIPTION
  Builds 64-bit and 32-bit executables under dist\win64 and dist\win32,
  matching the layout of the NSSM distribution archive.

  Repeated runs from the same commit must produce identical bytes:

    * Version and build date come from repository history, not the wall clock.
    * -trimpath removes build-machine paths.
    * -buildvcs=false prevents worktree cleanliness from changing the output.
    * Resource-object timestamps are zero (internal/rsrc).

  Version strings follow NSSM version.cmd rules. The leading v is removed from
  values such as "v2.24-101-g897c7ad" returned by git describe --tags --long.
  If no tag is available, internal/version keeps the source-snapshot default.

.PARAMETER Version
  Explicit version string. When omitted, derive it from repository history.

.PARAMETER Date
  Explicit build date in YYYY-MM-DD format. When omitted, use the HEAD commit date.

.PARAMETER OutDir
  Output directory. The default is dist.

.EXAMPLE
  pwsh tools\dist.ps1
#>
[CmdletBinding()]
param(
  [string]$Version,
  [string]$Date,
  [string]$OutDir = "dist"
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$root = Split-Path -Parent $PSScriptRoot
Push-Location $root
try {
  function Invoke-Git([string[]]$GitArgs) {
    $output = & git @GitArgs 2>$null
    if ($LASTEXITCODE -ne 0) { return $null }
    return ($output | Select-Object -First 1)
  }

  if (-not $Version) {
    $described = Invoke-Git @("describe", "--tags", "--long")
    if ($described) { $Version = $described -replace '^v', '' }
  }
  if (-not $Date) {
    $committed = Invoke-Git @("show", "-s", "--format=%cs", "HEAD")
    if ($committed) { $Date = $committed }
  }

  $ldflags = @("-s", "-w")
  if ($Version) { $ldflags += "-X", "ansm/internal/version.Number=$Version" }
  if ($Date) { $ldflags += "-X", "ansm/internal/version.BuildDate=$Date" }

  if ($Version) {
    # Keep the resource version and nsm version output identical.
    $mkrsrc = @("run", "./tools/mkrsrc")
    if ($Date) { $mkrsrc += "-date", $Date }
    $mkrsrc += "-version", $Version, "-icon", "resources/nssm.ico"
    & go @mkrsrc
    if ($LASTEXITCODE -ne 0) { throw "mkrsrc failed" }
  }

  Write-Host "ansm $(if ($Version) { $Version } else { '(default version)' }) $(if ($Date) { $Date } else { '' })"

  $targets = @(
    @{ Arch = "amd64"; Dir = "win64" },
    @{ Arch = "386"; Dir = "win32" }
  )
  $made = @()
  foreach ($target in $targets) {
    $dir = Join-Path $OutDir $target.Dir
    New-Item -ItemType Directory -Force -Path $dir | Out-Null
    $exe = Join-Path $dir "ansm.exe"

    $env:GOOS = "windows"
    $env:GOARCH = $target.Arch
    & go build -trimpath -buildvcs=false -ldflags ($ldflags -join " ") -o $exe ./cmd/ansm
    if ($LASTEXITCODE -ne 0) { throw "build for $($target.Arch) failed" }
    $made += $exe
  }
  Remove-Item Env:GOOS, Env:GOARCH -ErrorAction SilentlyContinue

  foreach ($exe in $made) {
    $hash = (Get-FileHash -Algorithm SHA256 $exe).Hash.ToLower()
    $size = (Get-Item $exe).Length
    Write-Host ("{0,-24} {1,10} bytes  sha256 {2}" -f $exe, $size, $hash)
  }
}
finally {
  Pop-Location
}
