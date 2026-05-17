param(
  [switch]$Alias,
  [switch]$Skill,
  [switch]$FromSource,
  [switch]$FromRelease,
  [string]$Version = "latest",
  [string]$BinDir = $env:BIN_DIR
)

$ErrorActionPreference = "Stop"
$Repo = "boan-anbo/codex-switch"

if (-not $BinDir) {
  $BinDir = Join-Path $HOME ".local\bin"
}

function Convert-BinDir {
  param([string]$Path)
  if ($Path -eq "~") {
    return $HOME
  }
  if ($Path.StartsWith("~/") -or $Path.StartsWith("~\")) {
    return (Join-Path $HOME $Path.Substring(2))
  }
  if ([System.IO.Path]::IsPathRooted($Path)) {
    return $Path
  }
  return Join-Path (Get-Location).Path $Path
}

$BinDir = Convert-BinDir $BinDir

function Test-Checkout {
  return [bool](Get-CheckoutDir)
}

function Get-CheckoutDir {
  $current = Get-Location
  $currentGoMod = Join-Path $current.Path "go.mod"
  if ((Test-Path $currentGoMod) -and (Select-String -Path $currentGoMod -Pattern "^module github.com/$Repo$" -Quiet)) {
    return $current.Path
  }
  if ($PSScriptRoot) {
    $candidate = Resolve-Path (Join-Path $PSScriptRoot "..") -ErrorAction SilentlyContinue
    if ($candidate) {
      $scriptGoMod = Join-Path $candidate.Path "go.mod"
      if ((Test-Path $scriptGoMod) -and (Select-String -Path $scriptGoMod -Pattern "^module github.com/$Repo$" -Quiet)) {
        return $candidate.Path
      }
    }
  }
  return $null
}

function Install-Alias {
  param([string]$Target)
  $aliasPath = Join-Path (Split-Path $Target -Parent) "cs.exe"
  if (Get-Command cs -ErrorAction SilentlyContinue) {
    Write-Warning "cs already exists; leaving it untouched"
  } elseif (Test-Path $aliasPath) {
    Write-Warning "$aliasPath already exists; leaving it untouched"
  } else {
    Copy-Item $Target $aliasPath
    Write-Host "installed cs.exe -> $Target"
  }
}

function Install-BundledSkill {
  param([string]$Target)
  & $Target skill install
}

function Install-Binary {
  param(
    [string]$Source,
    [string]$Target
  )
  $tmpTarget = Join-Path (Split-Path $Target -Parent) ".codex-switch-install-$([System.Guid]::NewGuid()).tmp"
  Remove-Item -Force $tmpTarget -ErrorAction SilentlyContinue
  try {
    Copy-Item $Source $tmpTarget
    Move-Item -Force $tmpTarget $Target
  } finally {
    Remove-Item -Force $tmpTarget -ErrorAction SilentlyContinue
  }
}

function Install-FromSource {
  if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    throw "go is required for -FromSource"
  }
  New-Item -ItemType Directory -Force -Path $BinDir | Out-Null
  $oldGoBin = $env:GOBIN
  try {
    $env:GOBIN = $BinDir
    $checkout = Get-CheckoutDir
    if ($checkout) {
      Push-Location $checkout
      try {
        go install .
      } finally {
        Pop-Location
      }
    } else {
      go install "github.com/$Repo@$Version"
    }
  } finally {
    if ($null -eq $oldGoBin) {
      Remove-Item Env:\GOBIN -ErrorAction SilentlyContinue
    } else {
      $env:GOBIN = $oldGoBin
    }
  }
  $target = Join-Path $BinDir "codex-switch.exe"
  if ($Alias) {
    Install-Alias $target
  }
  if ($Skill) {
    Install-BundledSkill $target
  }
  Write-Host "installed codex-switch -> $target"
}

function Get-ReleaseAsset {
  $arch = $env:PROCESSOR_ARCHITECTURE
  switch -Regex ($arch) {
    "ARM64" { $arch = "arm64"; break }
    "AMD64|x86_64" { $arch = "amd64"; break }
    default { throw "unsupported architecture: $arch" }
  }
  return "codex-switch_windows_$arch.zip"
}

function Get-ReleaseUrl {
  param([string]$Asset)
  if ($Version -eq "latest") {
    return "https://github.com/$Repo/releases/latest/download/$Asset"
  }
  return "https://github.com/$Repo/releases/download/$Version/$Asset"
}

function Test-ReleaseChecksum {
  param(
    [string]$Asset,
    [string]$Archive,
    [string]$Checksums
  )
  $line = Get-Content $Checksums | Where-Object {
    $parts = $_ -split "\s+"
    $parts.Count -ge 2 -and $parts[-1] -eq $Asset
  } | Select-Object -First 1
  if (-not $line) {
    throw "checksum entry missing for $Asset"
  }
  $expected = (($line -split "\s+")[0]).ToLowerInvariant()
  $actual = (Get-FileHash -Algorithm SHA256 -Path $Archive).Hash.ToLowerInvariant()
  if ($actual -ne $expected) {
    throw "checksum mismatch for $Asset; expected $expected, got $actual"
  }
}

function Install-FromRelease {
  $asset = Get-ReleaseAsset
  $tmp = Join-Path ([System.IO.Path]::GetTempPath()) ([System.Guid]::NewGuid().ToString())
  New-Item -ItemType Directory -Path $tmp | Out-Null
  try {
    $archive = Join-Path $tmp $asset
    $checksums = Join-Path $tmp "checksums.txt"
    if ($env:CODEX_SWITCH_RELEASE_DIR) {
      Copy-Item (Join-Path $env:CODEX_SWITCH_RELEASE_DIR $asset) $archive
      Copy-Item (Join-Path $env:CODEX_SWITCH_RELEASE_DIR "checksums.txt") $checksums
    } else {
      Invoke-WebRequest -Uri (Get-ReleaseUrl $asset) -OutFile $archive
      Invoke-WebRequest -Uri (Get-ReleaseUrl "checksums.txt") -OutFile $checksums
    }
    Test-ReleaseChecksum -Asset $asset -Archive $archive -Checksums $checksums
    Expand-Archive -Path $archive -DestinationPath $tmp
    New-Item -ItemType Directory -Force -Path $BinDir | Out-Null
    $target = Join-Path $BinDir "codex-switch.exe"
    Install-Binary (Join-Path $tmp "codex-switch.exe") $target
    if ($Alias) {
      Install-Alias $target
    }
    if ($Skill) {
      Install-BundledSkill $target
    }
    Write-Host "installed codex-switch -> $target"
  } finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
  }
}

if ($FromSource -and $FromRelease) {
  throw "choose only one of -FromSource or -FromRelease"
}

if ($FromSource) {
  Install-FromSource
} elseif ($FromRelease -or -not (Test-Checkout)) {
  Install-FromRelease
} else {
  Install-FromSource
}
