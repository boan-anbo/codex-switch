$ErrorActionPreference = "Stop"

$RepoDir = Resolve-Path (Join-Path $PSScriptRoot "..")
$DistDir = Join-Path $RepoDir "dist"
$Arch = $env:PROCESSOR_ARCHITECTURE
$SavedEnv = @{}
foreach ($Name in @("CODEX_SWITCH_RELEASE_DIR", "BIN_DIR", "CODEX_HOME")) {
  $SavedEnv[$Name] = [Environment]::GetEnvironmentVariable($Name, "Process")
}

function Restore-Environment {
  foreach ($Name in $SavedEnv.Keys) {
    if ($null -eq $SavedEnv[$Name]) {
      Remove-Item "Env:\$Name" -ErrorAction SilentlyContinue
    } else {
      Set-Item -Path "Env:\$Name" -Value $SavedEnv[$Name]
    }
  }
}

switch -Regex ($Arch) {
  "ARM64" { $Arch = "arm64"; break }
  "AMD64|x86_64" { $Arch = "amd64"; break }
  default { throw "unsupported architecture: $Arch" }
}

$Archive = Get-ChildItem -Path $DistDir -Filter "codex-switch_windows_$Arch.zip" | Select-Object -First 1
if (-not $Archive) {
  throw "no Windows archive found for $Arch in $DistDir"
}

$Tmp = Join-Path ([System.IO.Path]::GetTempPath()) ([System.Guid]::NewGuid().ToString())
New-Item -ItemType Directory -Path $Tmp | Out-Null

try {
  Expand-Archive -Path $Archive.FullName -DestinationPath $Tmp

  foreach ($Path in @(
    "README.md",
    "LICENSE",
    "CHANGELOG.md",
    "SECURITY.md",
    "skills\codex-switch\SKILL.md",
    "codex-switch.exe"
  )) {
    $FullPath = Join-Path $Tmp $Path
    if (-not (Test-Path $FullPath)) {
      throw "missing archive file: $Path"
    }
  }
  $ArchiveSkill = Join-Path $Tmp "skills\codex-switch\SKILL.md"
  if (-not ((Get-Content $ArchiveSkill -Raw).Contains("codex-switch new --account NAME"))) {
    throw "archive skill is missing safe --account guidance"
  }
  if (-not ((Get-Content $ArchiveSkill -Raw).Contains("codex-switch list --cwd ."))) {
    throw "archive skill is missing scoped list guidance"
  }
  if (-not ((Get-Content $ArchiveSkill -Raw).Contains("codex-switch resume --account NAME --session SESSION_ID"))) {
    throw "archive skill is missing explicit session resume guidance"
  }

  $VersionOutput = & (Join-Path $Tmp "codex-switch.exe") version
  if (-not (($VersionOutput -join "`n").Contains("codex-switch "))) {
    throw "unexpected version output: $VersionOutput"
  }
  if (($VersionOutput -join "`n").Contains("0.1.0-dev")) {
    throw "release archive still reports dev version: $VersionOutput"
  }

  $InstallDir = Join-Path $Tmp "install"
  $SkillHome = Join-Path $Tmp "skill-home\.codex"
  $env:CODEX_SWITCH_RELEASE_DIR = $DistDir
  $env:BIN_DIR = "install\bin"
  $env:CODEX_HOME = $SkillHome
  $OldPath = $env:PATH
  try {
    $env:PATH = Join-Path $InstallDir "bin"
    Push-Location $Tmp
    try {
      & (Join-Path $RepoDir "scripts\install.ps1") -FromRelease -Alias -Skill | Out-Null
    } finally {
      Pop-Location
    }
  } finally {
    $env:PATH = $OldPath
  }

  $Installed = Join-Path (Join-Path $InstallDir "bin") "codex-switch.exe"
  if (-not (Test-Path $Installed)) {
    throw "installer did not write $Installed"
  }
  $StagingFiles = Get-ChildItem -Path (Join-Path $InstallDir "bin") -Filter ".codex-switch-install-*.tmp" -ErrorAction SilentlyContinue
  if ($StagingFiles) {
    throw "installer left a temporary binary staging file"
  }
  $Alias = Join-Path (Join-Path $InstallDir "bin") "cs.exe"
  if (-not (Test-Path $Alias)) {
    throw "installer -Alias did not write $Alias"
  }
  $InstalledSkill = Join-Path $SkillHome "skills\codex-switch\SKILL.md"
  if (-not (Test-Path $InstalledSkill)) {
    throw "installer -Skill did not write $InstalledSkill"
  }
  if (-not ((Get-Content $InstalledSkill -Raw).Contains("codex-switch new --account NAME"))) {
    throw "installed skill is missing safe --account guidance"
  }
  if (-not ((Get-Content $InstalledSkill -Raw).Contains("codex-switch list --cwd ."))) {
    throw "installed skill is missing scoped list guidance"
  }
  if (-not ((Get-Content $InstalledSkill -Raw).Contains("codex-switch resume --account NAME --session SESSION_ID"))) {
    throw "installed skill is missing explicit session resume guidance"
  }
  $InstalledVersionOutput = & $Installed version
  if (($InstalledVersionOutput -join "`n").Contains("0.1.0-dev")) {
    throw "installed release still reports dev version: $InstalledVersionOutput"
  }

  $SpacedReleaseBin = Join-Path $Tmp "release bin with spaces"
  $env:CODEX_SWITCH_RELEASE_DIR = $DistDir
  $env:BIN_DIR = $SpacedReleaseBin
  & (Join-Path $RepoDir "scripts\install.ps1") -FromRelease | Out-Null
  $SpacedInstalled = Join-Path $SpacedReleaseBin "codex-switch.exe"
  if (-not (Test-Path $SpacedInstalled)) {
    throw "install.ps1 -FromRelease did not install into a path with spaces"
  }
  $SpacedStagingFiles = Get-ChildItem -Path $SpacedReleaseBin -Filter ".codex-switch-install-*.tmp" -ErrorAction SilentlyContinue
  if ($SpacedStagingFiles) {
    throw "install.ps1 left a temporary binary staging file in a path with spaces"
  }

  $AliasCollisionDir = Join-Path $Tmp "alias-collision"
  $AliasCollisionBin = Join-Path $AliasCollisionDir "bin"
  New-Item -ItemType Directory -Force -Path $AliasCollisionBin | Out-Null
  $AliasCollisionPath = Join-Path $AliasCollisionBin "cs.exe"
  Set-Content -Path $AliasCollisionPath -Value "existing alias" -Encoding Ascii
  $env:CODEX_SWITCH_RELEASE_DIR = $DistDir
  $env:BIN_DIR = $AliasCollisionBin
  & (Join-Path $RepoDir "scripts\install.ps1") -FromRelease -Alias | Out-Null
  $AliasCollisionContent = (Get-Content $AliasCollisionPath -Raw).Trim()
  if ($AliasCollisionContent -ne "existing alias") {
    throw "install.ps1 -Alias overwrote an existing cs.exe"
  }

  $PathCollisionDir = Join-Path $Tmp "path-collision"
  $PathCollisionExisting = Join-Path $PathCollisionDir "existing"
  $PathCollisionInstall = Join-Path $PathCollisionDir "install"
  New-Item -ItemType Directory -Force -Path $PathCollisionExisting | Out-Null
  New-Item -ItemType Directory -Force -Path $PathCollisionInstall | Out-Null
  Set-Content -Path (Join-Path $PathCollisionExisting "cs.cmd") -Value "@echo off`r`nexit /b 0" -Encoding Ascii
  $OldPath = $env:PATH
  try {
    $env:PATH = "$PathCollisionExisting;$OldPath"
    $env:CODEX_SWITCH_RELEASE_DIR = $DistDir
    $env:BIN_DIR = $PathCollisionInstall
    & (Join-Path $RepoDir "scripts\install.ps1") -FromRelease -Alias | Out-Null
  } finally {
    $env:PATH = $OldPath
  }
  if (Test-Path (Join-Path $PathCollisionInstall "cs.exe")) {
    throw "install.ps1 -Alias installed cs.exe even though cs exists on PATH"
  }

  $BadRelease = Join-Path $Tmp "bad-release"
  New-Item -ItemType Directory -Path $BadRelease | Out-Null
  Copy-Item $Archive.FullName (Join-Path $BadRelease $Archive.Name)
  $BadHash = "0" * 64
  Get-Content (Join-Path $DistDir "checksums.txt") | ForEach-Object {
    $Parts = $_ -split "\s+"
    if ($Parts.Count -ge 2 -and $Parts[-1] -eq $Archive.Name) {
      "$BadHash  $($Archive.Name)"
    } else {
      $_
    }
  } | Set-Content -Path (Join-Path $BadRelease "checksums.txt")

  $env:CODEX_SWITCH_RELEASE_DIR = $BadRelease
  $env:BIN_DIR = Join-Path $Tmp "bad-install\bin"
  $AcceptedBadChecksum = $true
  try {
    & (Join-Path $RepoDir "scripts\install.ps1") -FromRelease | Out-Null
  } catch {
    $AcceptedBadChecksum = $false
  }
  if ($AcceptedBadChecksum) {
    throw "installer accepted archive with bad checksum"
  }

  Write-Host "archive smoke ok"
} finally {
  Restore-Environment
  Remove-Item -Recurse -Force $Tmp -ErrorAction SilentlyContinue
}
