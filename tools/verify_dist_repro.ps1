#Requires -Version 5.1
<#
.SYNOPSIS
  Verifies tools\dist.ps1 reproducibility and Git-metadata fallback behavior.

.DESCRIPTION
  Builds win64 and win32 ansm.exe twice from one commit and compares SHA-256
  values. It also creates disposable Git repositories to verify that dist.ps1
  completes when git describe cannot reach a tag, including a tagless repository
  and a depth-1 --no-tags clone.

  Windows PowerShell 5.1 is exercised when powershell.exe is available. PowerShell
  7 is exercised when pwsh is available. Missing optional hosts are reported as
  SKIP rather than failing the verification. Nested dist.ps1 hosts are launched
  with Start-Process and separated stdout/stderr files so Windows PowerShell 5.1
  cannot promote native stderr into a terminating NativeCommandError in this
  verification harness itself.

  All scenario repositories and artifacts live under temporary directories and
  are removed afterward; the repository dist\ tree and Git state are not changed.

.PARAMETER OutDir
  Parent directory for the reproducibility pair. When omitted, use
  FLOWGATE_TEST_SCRATCH when available, otherwise the system temporary directory.
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
$scenarioRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("ansm-dist-scenarios-" + [Guid]::NewGuid().ToString("N"))

function Resolve-Runner([string]$Name) {
  $cmd = Get-Command $Name -ErrorAction SilentlyContinue
  if ($cmd) { return $cmd.Source }
  return $null
}

function Invoke-GitProcess(
  [string]$WorkingDirectory,
  [string[]]$GitArgs,
  [switch]$AllowFailure
) {
  $stdoutPath = [System.IO.Path]::GetTempFileName()
  $stderrPath = [System.IO.Path]::GetTempFileName()
  try {
    $process = Start-Process `
      -FilePath "git" `
      -ArgumentList $GitArgs `
      -WorkingDirectory $WorkingDirectory `
      -NoNewWindow `
      -Wait `
      -PassThru `
      -RedirectStandardOutput $stdoutPath `
      -RedirectStandardError $stderrPath

    $stdout = @(Get-Content -LiteralPath $stdoutPath -ErrorAction SilentlyContinue)
    $stderr = @(Get-Content -LiteralPath $stderrPath -ErrorAction SilentlyContinue)
    if ($process.ExitCode -ne 0 -and -not $AllowFailure) {
      throw "git $($GitArgs -join ' ') failed with exit $($process.ExitCode): $($stderr -join ' ')"
    }
    return @{
      ExitCode = $process.ExitCode
      Stdout = $stdout
      Stderr = $stderr
    }
  }
  finally {
    Remove-Item -LiteralPath $stdoutPath, $stderrPath -Force -ErrorAction SilentlyContinue
  }
}

function Invoke-Dist(
  [string]$Runner,
  [string]$RepoRoot,
  [string]$Destination
) {
  $distScript = Join-Path $RepoRoot "tools\dist.ps1"
  $stdoutPath = [System.IO.Path]::GetTempFileName()
  $stderrPath = [System.IO.Path]::GetTempFileName()
  try {
    $args = @(
      "-NoProfile",
      "-ExecutionPolicy", "Bypass",
      "-File", $distScript,
      "-OutDir", $Destination
    )

    $process = Start-Process `
      -FilePath $Runner `
      -ArgumentList $args `
      -WorkingDirectory $RepoRoot `
      -NoNewWindow `
      -Wait `
      -PassThru `
      -RedirectStandardOutput $stdoutPath `
      -RedirectStandardError $stderrPath

    $stdout = @(Get-Content -LiteralPath $stdoutPath -ErrorAction SilentlyContinue)
    $stderr = @(Get-Content -LiteralPath $stderrPath -ErrorAction SilentlyContinue)

    foreach ($line in $stdout) { Write-Host $line }
    foreach ($line in $stderr) { Write-Host $line }

    if ($process.ExitCode -ne 0) {
      throw "$Runner dist.ps1 failed with exit $($process.ExitCode) in $RepoRoot"
    }
    return $stdout
  }
  finally {
    Remove-Item -LiteralPath $stdoutPath, $stderrPath -Force -ErrorAction SilentlyContinue
  }
}

function Assert-DefaultVersion([object[]]$Output, [string]$Scenario, [string]$Runner) {
  $text = ($Output | ForEach-Object { "$_" }) -join "`n"
  if ($text -notmatch [regex]::Escape("(default version)")) {
    throw "$Scenario under $Runner did not use the snapshot default version"
  }
  Write-Host "OK $Scenario under $Runner : git describe fallback used"
}

function Compare-ArtifactPair([string]$First, [string]$Second) {
  $targets = @("win64\ansm.exe", "win32\ansm.exe")
  $mismatch = $false
  foreach ($target in $targets) {
    $hash1 = (Get-FileHash -Algorithm SHA256 (Join-Path $First $target)).Hash
    $hash2 = (Get-FileHash -Algorithm SHA256 (Join-Path $Second $target)).Hash
    if ($hash1 -ne $hash2) {
      Write-Host "MISMATCH $target : $hash1 vs $hash2"
      $mismatch = $true
    } else {
      Write-Host "OK $target sha256 $hash1"
    }
  }
  if ($mismatch) { throw "distribution artifacts are not reproducible" }
}

function Copy-SourceTree([string]$Source, [string]$Destination) {
  New-Item -ItemType Directory -Force -Path $Destination | Out-Null
  Get-ChildItem -LiteralPath $Source -Force |
    Where-Object { $_.Name -notin @(".git", "dist") } |
    ForEach-Object {
      Copy-Item -LiteralPath $_.FullName -Destination $Destination -Recurse -Force
    }
}

function Initialize-Repository([string]$RepoRoot) {
  Invoke-GitProcess $RepoRoot @("init") | Out-Null
  Invoke-GitProcess $RepoRoot @("config", "user.name", "ANSM Dist Test") | Out-Null
  Invoke-GitProcess $RepoRoot @("config", "user.email", "ansm-dist@example.invalid") | Out-Null
  Invoke-GitProcess $RepoRoot @("add", "-A") | Out-Null
  Invoke-GitProcess $RepoRoot @("commit", "-m", "baseline") | Out-Null
}

function New-TaglessRepository([string]$Destination) {
  Copy-SourceTree $root $Destination
  Initialize-Repository $Destination

  $describe = Invoke-GitProcess $Destination @("describe", "--tags", "--long") -AllowFailure
  if ($describe.ExitCode -eq 0) {
    throw "tagless scenario unexpectedly has a describable tag"
  }
}

function New-ShallowNoTagRepository([string]$SourceRepo, [string]$Destination) {
  Copy-SourceTree $root $SourceRepo
  Initialize-Repository $SourceRepo
  Invoke-GitProcess $SourceRepo @("tag", "v0.0-test") | Out-Null

  Add-Content -LiteralPath (Join-Path $SourceRepo "README.md") -Value "`n<!-- dist repro shallow head -->"
  Invoke-GitProcess $SourceRepo @("add", "README.md") | Out-Null
  Invoke-GitProcess $SourceRepo @("commit", "-m", "head") | Out-Null

  $sourcePath = (Resolve-Path $SourceRepo).Path + [System.IO.Path]::DirectorySeparatorChar
  $sourceUri = (New-Object -TypeName System.Uri -ArgumentList $sourcePath).AbsoluteUri
  Invoke-GitProcess $scenarioRoot @("clone", "--depth", "1", "--no-tags", $sourceUri, "shallow") | Out-Null

  $describe = Invoke-GitProcess $Destination @("describe", "--tags", "--long") -AllowFailure
  if ($describe.ExitCode -eq 0) {
    throw "shallow scenario unexpectedly has a reachable describable tag"
  }
}

Remove-Item -Recurse -Force $OutDir -ErrorAction SilentlyContinue
Remove-Item -Recurse -Force $scenarioRoot -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force -Path $OutDir | Out-Null
New-Item -ItemType Directory -Force -Path $scenarioRoot | Out-Null

try {
  $pwsh = Resolve-Runner "pwsh"
  $windowsPowerShell = Resolve-Runner "powershell.exe"

  if (-not $pwsh -and -not $windowsPowerShell) {
    throw "Neither pwsh nor powershell.exe is available"
  }

  # Preserve the original reproducibility check. Prefer the documented PowerShell 7
  # path, falling back to Windows PowerShell 5.1 when pwsh is not installed.
  $primaryRunner = if ($pwsh) { $pwsh } else { $windowsPowerShell }
  Write-Host "== reproducibility: $primaryRunner =="
  Invoke-Dist $primaryRunner $root $run1 | Out-Null
  Invoke-Dist $primaryRunner $root $run2 | Out-Null
  Compare-ArtifactPair $run1 $run2

  # Build disposable repositories whose git describe command is guaranteed to fail.
  $taglessRepo = Join-Path $scenarioRoot "tagless"
  $shallowSource = Join-Path $scenarioRoot "shallow-source"
  $shallowRepo = Join-Path $scenarioRoot "shallow"
  New-TaglessRepository $taglessRepo
  New-ShallowNoTagRepository $shallowSource $shallowRepo

  $fallbackRunners = @()
  if ($windowsPowerShell) {
    $fallbackRunners += $windowsPowerShell
  } else {
    Write-Host "SKIP Windows PowerShell 5.1: powershell.exe not available"
  }
  if ($pwsh) {
    $fallbackRunners += $pwsh
  } else {
    Write-Host "SKIP PowerShell 7: pwsh not available"
  }

  foreach ($runner in $fallbackRunners) {
    $runnerName = Split-Path -Leaf $runner

    $taglessOut = Join-Path $scenarioRoot ("out-tagless-" + $runnerName)
    Write-Host "== tagless fallback: $runner =="
    $taglessOutput = @(Invoke-Dist $runner $taglessRepo $taglessOut)
    Assert-DefaultVersion $taglessOutput "tagless repository" $runner

    $shallowOut = Join-Path $scenarioRoot ("out-shallow-" + $runnerName)
    Write-Host "== shallow/no-tags fallback: $runner =="
    $shallowOutput = @(Invoke-Dist $runner $shallowRepo $shallowOut)
    Assert-DefaultVersion $shallowOutput "shallow --no-tags clone" $runner
  }

  Write-Host "ALL DIST REPRO/FALLBACK CHECKS PASSED"
  exit 0
}
finally {
  Remove-Item -Recurse -Force $OutDir -ErrorAction SilentlyContinue
  Remove-Item -Recurse -Force $scenarioRoot -ErrorAction SilentlyContinue
}
