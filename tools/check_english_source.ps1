#Requires -Version 5.1
<#
.SYNOPSIS
  Fails when tracked source text contains Hangul syllables.

.DESCRIPTION
  Scans tracked source files that use known text formats. Git metadata, build
  caches, distribution artifacts, and binary resources are outside this source
  text check. Every match is reported as path:line, and any match causes exit 1.
#>
[CmdletBinding()]
param()

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$root = Split-Path -Parent $PSScriptRoot
Push-Location $root
try {
  $textExtensions = @(
    ".go", ".md", ".ps1", ".mc", ".mod", ".sum", ".json", ".yaml", ".yml",
    ".txt", ".cmd", ".bat", ".rc"
  )
  $hangulSyllable = '[\uAC00-\uD7A3]'
  $matches = 0

  $tracked = @(& git ls-files; & git ls-files --others --exclude-standard) | Sort-Object -Unique
  if ($LASTEXITCODE -ne 0) { throw "git ls-files failed" }

  foreach ($path in $tracked) {
    $leaf = Split-Path -Leaf $path
    $extension = [IO.Path]::GetExtension($path).ToLowerInvariant()
    if ($leaf -ne ".gitignore" -and $extension -notin $textExtensions) { continue }

    $lineNumber = 0
    foreach ($line in Get-Content -LiteralPath $path -Encoding UTF8) {
      $lineNumber++
      if ($line -match $hangulSyllable) {
        Write-Host "$path`:$lineNumber`:$line"
        $matches++
      }
    }
  }

  if ($matches -ne 0) {
    Write-Error "Found $matches Hangul-containing source lines."
    exit 1
  }

  Write-Host "OK: tracked source text contains no Hangul syllables."
  exit 0
}
finally {
  Pop-Location
}
