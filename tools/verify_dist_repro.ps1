#Requires -Version 5.1
<#
.SYNOPSIS
  tools\dist.ps1 이 같은 커밋에서 두 번 돌아도 같은 바이트를 내는지 확인한다.

.DESCRIPTION
  win64, win32 ansm.exe 를 각각 두 번 만들어 SHA256 을 비교한다. 하나라도
  다르면 exit 1, 모두 같으면 exit 0. 산출물은 임시 폴더에 만들고 끝나면
  지운다 — 저장소의 dist\ 는 건드리지 않는다.

.PARAMETER OutDir
  두 회차의 산출물을 놓을 부모 폴더. 비우면 FLOWGATE_TEST_SCRATCH 환경
  변수(있으면) 아래, 없으면 임시 폴더 아래에 만든다.
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
  # dist.ps1 은 pwsh 로 돌리는 것을 전제로 한다(스크립트 안내 예시와 동일). Windows
  # PowerShell 5.1 은 git describe 가 태그 없이 실패할 때 2>$null 로 죽인 에러를
  # 그대로 종료 예외로 올려 dist.ps1 을 깨뜨린다.
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
