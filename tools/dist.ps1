#Requires -Version 5.1
<#
.SYNOPSIS
  ANSM 배포 산출물을 만든다.

.DESCRIPTION
  64비트와 32비트 실행 파일을 만들어 dist\win64, dist\win32 에 놓는다.
  원본 나씀의 배포 압축 파일과 같은 배치다.

  같은 커밋에서 몇 번을 돌려도 같은 바이트가 나와야 한다. 그래서

    * 버전과 빌드 일자를 저장소 이력에서 뽑는다. 손목시계를 보지 않는다.
    * -trimpath 로 빌드 기계의 경로를 지운다.
    * -buildvcs=false 로 작업 폴더가 깨끗한지 여부가 산출물을 바꾸지 않게 한다.
    * 리소스 오브젝트의 시각 도장은 0 이다(internal/rsrc).

  버전 문자열은 원본 version.cmd 와 같은 규칙이다. `git describe --tags --long`
  이 내는 "v2.24-101-g897c7ad" 에서 앞의 v 를 떼고 그대로 쓴다. 태그가 없으면
  internal/version 의 기본값을 그대로 둔다 — 이식의 바탕이 된 원본 스냅샷 값이다.

.PARAMETER Version
  버전 문자열을 직접 정한다. 비우면 저장소 이력에서 뽑는다.

.PARAMETER Date
  빌드 일자(YYYY-MM-DD)를 직접 정한다. 비우면 HEAD 커밋 날짜를 쓴다.

.PARAMETER OutDir
  산출물을 놓을 폴더. 기본은 dist.

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
    # 리소스에 실리는 버전도 같아야 한다. 파일 속성 창과 `ansm version` 이
    # 서로 다른 값을 말하면 어느 쪽이 맞는지 알 수 없다.
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
