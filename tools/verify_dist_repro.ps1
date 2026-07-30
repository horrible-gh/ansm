#Requires -Version 5.1
<#
.SYNOPSIS
  Verifies that tools\dist.ps1 produces identical bytes twice from one commit.

.DESCRIPTION
  Builds win64 and win32 ansm.exe twice and compares SHA-256 values. The script
  exits 1 on any mismatch and 0 when both targets match. Artifacts are created
  in a temporary directory and removed afterward; the repository dist\ tree is
  not changed.

.PARAMETER OutDir
  Parent directory for both runs. When omitted, use FLOWGATE_TEST_SCRATCH when
  available, otherwise use the system temporary directory.
#>
[CmdletBinding()]
param(
  [string]$OutDir
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$root = Split-Path -Parent $PSScriptRoot
if (-not $OutDir) {
  $base = if ($env:FLOWGATE_TEST_SCRATCH) { $env:FLOWGATE_TEST_SCRATCH } else { $env:TEMP }
  $OutDir = Join-Path $base "ansm-dist-repro"
}

$run1 = Join-Path $OutDir "run1"
$run2 = Join-Path $OutDir "run2"
Remove-Item -Recurse -Force $OutDir -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force -Path $OutDir | Out-Null

try {
  # Run dist.ps1 with pwsh, matching its documented invocation. Windows
  # PowerShell 5.1 can promote redirected git errors into terminating errors.
  & pwsh -NoProfile -File (Join-Path $root "tools\dist.ps1") -OutDir $run1
  if ($LASTEXITCODE -ne 0) { throw "first dist.ps1 run failed" }
  & pwsh -NoProfile -File (Join-Path $root "tools\dist.ps1") -OutDir $run2
  if ($LASTEXITCODE -ne 0) { throw "second dist.ps1 run failed" }

  $targets = @("win64\ansm.exe", "win32\ansm.exe")
  $mismatch = $false
  foreach ($target in $targets) {
    $hash1 = (Get-FileHash -Algorithm SHA256 (Join-Path $run1 $target)).Hash
    $hash2 = (Get-FileHash -Algorithm SHA256 (Join-Path $run2 $target)).Hash
    if ($hash1 -ne $hash2) {
      Write-Host "MISMATCH $target : $hash1 vs $hash2"
      $mismatch = $true
    } else {
      Write-Host "OK $target sha256 $hash1"
    }
  }
  if ($mismatch) { exit 1 }
  exit 0
}
finally {
  Remove-Item -Recurse -Force $OutDir -ErrorAction SilentlyContinue
}
