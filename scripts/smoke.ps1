$ErrorActionPreference = "Stop"

$RepoDir = Resolve-Path (Join-Path $PSScriptRoot "..")
$Tmp = Join-Path ([System.IO.Path]::GetTempPath()) ([System.Guid]::NewGuid().ToString())
$SavedEnv = @{}
foreach ($Name in @(
  "HOME",
  "USERPROFILE",
  "CODEX_SWITCH_CONFIG",
  "CODEX_SWITCH_CACHE",
  "CODEX_HOME",
  "GOPATH",
  "GOMODCACHE"
)) {
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

New-Item -ItemType Directory -Path $Tmp | Out-Null

try {
  $HomeDir = Join-Path $Tmp "home"
  $Config = Join-Path $Tmp "config.toml"
  $Cache = Join-Path $Tmp "cache"
  $Bin = Join-Path $Tmp "codex-switch.exe"
  New-Item -ItemType Directory -Force -Path (Join-Path $HomeDir ".codex\sessions") | Out-Null
  New-Item -ItemType Directory -Force -Path $Cache | Out-Null
  New-Item -ItemType File -Force -Path (Join-Path $HomeDir ".codex\config.toml") | Out-Null

  Push-Location $RepoDir
  try {
    go build -o $Bin .
  } finally {
    Pop-Location
  }

  $env:HOME = $HomeDir
  $env:USERPROFILE = $HomeDir
  $env:CODEX_SWITCH_CONFIG = $Config
  $env:CODEX_SWITCH_CACHE = $Cache

  & $Bin version | Out-Null
  & $Bin help | Out-Null

  $AccountsJson = & $Bin accounts --json
  if (-not ($AccountsJson -join "`n").Contains('"name": "default"')) {
    throw "accounts output did not include default"
  }

  $NewCmd = & $Bin new --account work --print -- --model gpt-test
  if (-not ($NewCmd -join "`n").Contains(".codex-work")) {
    throw "new --print did not include selected CODEX_HOME"
  }
  if (-not ($NewCmd -join "`n").Contains("--model gpt-test")) {
    throw "new --print did not preserve passthrough args"
  }
  if (Test-Path (Join-Path $HomeDir ".codex-work\config.toml")) {
    throw "new --print initialized account config"
  }
  if (Test-Path (Join-Path $HomeDir ".codex-work\sessions")) {
    throw "new --print initialized account sessions"
  }

  $ResumeCmd = & $Bin resume --account work --all --print
  $ResumeText = $ResumeCmd -join "`n"
  if (-not $ResumeText.Contains(".codex-work")) {
    throw "resume --print did not include selected CODEX_HOME"
  }
  if (-not $ResumeText.Contains("resume --all --include-non-interactive")) {
    throw "resume --all did not include expected Codex flags"
  }
  if (Test-Path (Join-Path $HomeDir ".codex-work\config.toml")) {
    throw "resume --print initialized account config"
  }
  if (Test-Path (Join-Path $HomeDir ".codex-work\sessions")) {
    throw "resume --print initialized account sessions"
  }

  $SessionID = "019d30aa-4798-7891-a56f-1f87a629e02c"
  $SessionCmd = & $Bin resume $SessionID --print
  $SessionText = $SessionCmd -join "`n"
  if (-not $SessionText.Contains("resume --cd")) {
    throw "resume positional session did not include resume command"
  }
  if (-not $SessionText.Contains($SessionID)) {
    throw "resume positional session did not pass through session id"
  }

  $AccountSessionCmd = & $Bin resume --account work --session $SessionID --print
  $AccountSessionText = $AccountSessionCmd -join "`n"
  if (-not $AccountSessionText.Contains(".codex-work")) {
    throw "resume --account --session did not include selected CODEX_HOME"
  }
  if (-not $AccountSessionText.Contains("resume --cd")) {
    throw "resume --account --session did not include resume command"
  }
  if (-not $AccountSessionText.Contains($SessionID)) {
    throw "resume --account --session did not pass through session id"
  }
  if (Test-Path (Join-Path $HomeDir ".codex-work\config.toml")) {
    throw "resume --account --session initialized account config"
  }
  if (Test-Path (Join-Path $HomeDir ".codex-work\sessions")) {
    throw "resume --account --session initialized account sessions"
  }

  $LoginCmd = & $Bin run work --print -- login
  $LoginText = $LoginCmd -join "`n"
  if (-not $LoginText.Contains(".codex-work")) {
    throw "run --print did not include selected CODEX_HOME"
  }
  if (-not $LoginText.Contains("codex login")) {
    throw "run --print did not preserve login command"
  }
  if (Test-Path (Join-Path $HomeDir ".codex-work\config.toml")) {
    throw "run --print initialized account config"
  }
  if (Test-Path (Join-Path $HomeDir ".codex-work\sessions")) {
    throw "run --print initialized account sessions"
  }

  & $Bin account add work2 | Out-Null
  if (-not (Select-String -Path $Config -Pattern "name = 'work2'" -Quiet)) {
    throw "account add did not write config"
  }
  if (Test-Path (Join-Path $HomeDir ".codex-work2\auth.json")) {
    throw "account add created auth.json"
  }

  & $Bin init-account workinit | Out-Null
  if (-not (Select-String -Path $Config -Pattern "name = 'workinit'" -Quiet)) {
    throw "init-account did not write config"
  }
  if (Test-Path (Join-Path $HomeDir ".codex-workinit\auth.json")) {
    throw "init-account created auth.json"
  }

  $FakeCodex = Join-Path $Tmp "codex.cmd"
  Set-Content -Path $FakeCodex -Value '@echo off' -Encoding Ascii
  Add-Content -Path $FakeCodex -Value 'echo %CODEX_HOME%> "%USERPROFILE%\launched-home.txt"' -Encoding Ascii
  $OldPath = $env:PATH
  $env:PATH = "$Tmp;$OldPath"
  try {
    & $Bin new --account work3 | Out-Null
  } finally {
    $env:PATH = $OldPath
  }
  $LaunchedHome = Get-Content (Join-Path $HomeDir "launched-home.txt") -Raw
  if (-not $LaunchedHome.Contains(".codex-work3")) {
    throw "launch did not use work3 CODEX_HOME"
  }
  if (-not (Test-Path (Join-Path $HomeDir ".codex-work3\config.toml"))) {
    throw "launch did not initialize shared config"
  }
  if (-not (Test-Path (Join-Path $HomeDir ".codex-work3\sessions"))) {
    throw "launch did not initialize shared sessions"
  }
  if (Test-Path (Join-Path $HomeDir ".codex-work3\auth.json")) {
    throw "launch created auth.json"
  }

  $env:CODEX_HOME = Join-Path $HomeDir ".codex"
  & $Bin skill install | Out-Null
  if (-not (Test-Path (Join-Path $HomeDir ".codex\skills\codex-switch\SKILL.md"))) {
    throw "skill install did not write bundled skill"
  }
  $InstalledSkill = Join-Path $HomeDir ".codex\skills\codex-switch\SKILL.md"
  if (-not ((Get-Content $InstalledSkill -Raw).Contains("codex-switch new --account NAME"))) {
    throw "skill install wrote stale or unexpected bundled skill"
  }
  if (-not ((Get-Content $InstalledSkill -Raw).Contains("codex-switch list --cwd ."))) {
    throw "skill install wrote bundled skill without scoped list guidance"
  }
  if (-not ((Get-Content $InstalledSkill -Raw).Contains("codex-switch resume --account NAME --session SESSION_ID"))) {
    throw "skill install wrote bundled skill without explicit session resume guidance"
  }
  Remove-Item Env:\CODEX_HOME -ErrorAction SilentlyContinue

  if ($env:RUN_INSTALL_SMOKE -eq "1") {
    $InstallGoPath = Join-Path $Tmp "gopath"
    $InstallGoModCache = Join-Path $Tmp "gomodcache"

    $env:GOPATH = $InstallGoPath
    $env:GOMODCACHE = $InstallGoModCache
    $env:CODEX_HOME = Join-Path $HomeDir ".codex"
    & (Join-Path $RepoDir "scripts\install.ps1") -FromSource -BinDir (Join-Path $Tmp "source-bin") -Skill | Out-Null
    if (-not (Test-Path (Join-Path $Tmp "source-bin\codex-switch.exe"))) {
      throw "install.ps1 did not install codex-switch.exe"
    }
    $StagingFiles = Get-ChildItem -Path (Join-Path $Tmp "source-bin") -Filter ".codex-switch-install-*.tmp" -ErrorAction SilentlyContinue
    if ($StagingFiles) {
      throw "install.ps1 left a temporary binary staging file"
    }
    if (-not (Test-Path (Join-Path $HomeDir ".codex\skills\codex-switch\SKILL.md"))) {
      throw "install.ps1 -Skill did not install bundled skill"
    }
    $InstalledSkill = Join-Path $HomeDir ".codex\skills\codex-switch\SKILL.md"
    if (-not ((Get-Content $InstalledSkill -Raw).Contains("codex-switch new --account NAME"))) {
      throw "install.ps1 -Skill wrote stale or unexpected bundled skill"
    }
    if (-not ((Get-Content $InstalledSkill -Raw).Contains("codex-switch list --cwd ."))) {
      throw "install.ps1 -Skill wrote bundled skill without scoped list guidance"
    }
    if (-not ((Get-Content $InstalledSkill -Raw).Contains("codex-switch resume --account NAME --session SESSION_ID"))) {
      throw "install.ps1 -Skill wrote bundled skill without explicit session resume guidance"
    }

    $AliasBin = Join-Path $Tmp "existing-alias-bin"
    New-Item -ItemType Directory -Force -Path $AliasBin | Out-Null
    $AliasPath = Join-Path $AliasBin "cs.exe"
    Set-Content -Path $AliasPath -Value "existing alias" -Encoding Ascii
    $env:GOPATH = $InstallGoPath
    $env:GOMODCACHE = $InstallGoModCache
    & (Join-Path $RepoDir "scripts\install.ps1") -FromSource -BinDir $AliasBin -Alias | Out-Null
    $AliasContent = (Get-Content $AliasPath -Raw).Trim()
    if ($AliasContent -ne "existing alias") {
      throw "install.ps1 -Alias overwrote an existing cs.exe"
    }

    $PathCollisionBin = Join-Path $Tmp "path-collision-bin"
    $PathCollisionTarget = Join-Path $Tmp "path-collision-target"
    New-Item -ItemType Directory -Force -Path $PathCollisionBin | Out-Null
    New-Item -ItemType Directory -Force -Path $PathCollisionTarget | Out-Null
    Set-Content -Path (Join-Path $PathCollisionBin "cs.cmd") -Value "@echo off`r`nexit /b 0" -Encoding Ascii
    $OldPath = $env:PATH
    try {
      $env:PATH = "$PathCollisionBin;$OldPath"
      $env:GOPATH = $InstallGoPath
      $env:GOMODCACHE = $InstallGoModCache
      & (Join-Path $RepoDir "scripts\install.ps1") -FromSource -BinDir $PathCollisionTarget -Alias | Out-Null
    } finally {
      $env:PATH = $OldPath
    }
    if (Test-Path (Join-Path $PathCollisionTarget "cs.exe")) {
      throw "install.ps1 -Alias installed cs.exe even though cs exists on PATH"
    }

    Push-Location $Tmp
    try {
      $env:GOPATH = $InstallGoPath
      $env:GOMODCACHE = $InstallGoModCache
      & (Join-Path $RepoDir "scripts\install.ps1") -FromSource -BinDir (Join-Path $Tmp "path-source-bin") | Out-Null
    } finally {
      Pop-Location
    }
    if (-not (Test-Path (Join-Path $Tmp "path-source-bin\codex-switch.exe"))) {
      throw "install.ps1 -FromSource did not install from script checkout path"
    }

    Push-Location $Tmp
    try {
      $env:GOPATH = $InstallGoPath
      $env:GOMODCACHE = $InstallGoModCache
      & (Join-Path $RepoDir "scripts\install.ps1") -FromSource -BinDir "relative-bin" | Out-Null
    } finally {
      Pop-Location
    }
    if (-not (Test-Path (Join-Path $Tmp "relative-bin\codex-switch.exe"))) {
      throw "install.ps1 -FromSource did not normalize relative -BinDir"
    }

    $SpacedBin = Join-Path $Tmp "source bin with spaces"
    $env:GOPATH = $InstallGoPath
    $env:GOMODCACHE = $InstallGoModCache
    & (Join-Path $RepoDir "scripts\install.ps1") -FromSource -BinDir $SpacedBin | Out-Null
    if (-not (Test-Path (Join-Path $SpacedBin "codex-switch.exe"))) {
      throw "install.ps1 -FromSource did not install into a path with spaces"
    }

    $DownloadProbe = Join-Path $Tmp "download-probe"
    New-Item -ItemType Directory -Force -Path $DownloadProbe | Out-Null
    $DownloadLog = Join-Path $DownloadProbe "urls.txt"
    function Invoke-WebRequest {
      param(
        [Parameter(Mandatory = $true)][string]$Uri,
        [Parameter(Mandatory = $true)][string]$OutFile
      )
      Add-Content -Path $DownloadLog -Value $Uri
      Set-Content -Path $OutFile -Value "not a release artifact" -Encoding Ascii
    }
    $AcceptedProbeDownloads = $true
    try {
      & (Join-Path $RepoDir "scripts\install.ps1") -FromRelease -Version "v1.2.3" -BinDir (Join-Path $DownloadProbe "install") | Out-Null
    } catch {
      $AcceptedProbeDownloads = $false
    }
    if ($AcceptedProbeDownloads) {
      throw "install.ps1 unexpectedly accepted probe downloads"
    }
    $DownloadText = Get-Content $DownloadLog -Raw
    if (-not $DownloadText.Contains("/releases/download/v1.2.3/codex-switch_windows_")) {
      throw "install.ps1 did not use the requested version for the release archive URL: $DownloadText"
    }
    if (-not $DownloadText.Contains("/releases/download/v1.2.3/checksums.txt")) {
      throw "install.ps1 did not use the requested version for checksums.txt: $DownloadText"
    }
  }

  Write-Host "smoke ok"
} finally {
  Restore-Environment
  Remove-Item -Recurse -Force $Tmp -ErrorAction SilentlyContinue
}
