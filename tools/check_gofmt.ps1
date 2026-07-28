#Requires -Version 5.1
<#
.SYNOPSIS
  gofmt -l 이 손댈 파일이 있으면 실패한다.

.DESCRIPTION
  gofmt -l 은 포맷이 깨진 파일이 있어도 종료 코드는 항상 0이라 그 자체로는
  CI 판정에 못 쓴다. 이 스크립트는 출력이 있으면 목록을 찍고 exit 1, 없으면
  exit 0 을 낸다.
#>
$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$root = Split-Path -Parent $PSScriptRoot
Push-Location $root
try {
  $files = & gofmt -l .
  if ($files) {
    $files | ForEach-Object { Write-Host $_ }
    exit 1
  }
  exit 0
}
finally {
  Pop-Location
}
