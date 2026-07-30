#Requires -Version 5.1
<#
.SYNOPSIS
  Fails when gofmt -l reports files that need formatting.

.DESCRIPTION
  gofmt -l exits 0 even when it reports improperly formatted files. This script
  prints any reported paths and exits 1; otherwise it exits 0.
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
