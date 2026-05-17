param(
  [switch]$Release
)

$ErrorActionPreference = "Stop"

$RepoDir = Resolve-Path (Join-Path $PSScriptRoot "..")
$StaticcheckVersion = "v0.7.0"
$ShfmtVersion = "v3.12.0"
$GovulncheckVersion = "v1.3.0"

function Invoke-Step {
  param(
    [Parameter(Mandatory = $true)][string]$Name,
    [Parameter(Mandatory = $true)][scriptblock]$Command
  )

  Write-Host ""
  Write-Host "==> $Name"
  & $Command
}

function Test-GoModTidy {
  $Tmp = Join-Path ([System.IO.Path]::GetTempPath()) ([System.Guid]::NewGuid().ToString())
  New-Item -ItemType Directory -Path $Tmp | Out-Null
  $OriginalGoMod = Join-Path $Tmp "go.mod"
  $OriginalGoSum = Join-Path $Tmp "go.sum"
  Copy-Item (Join-Path $RepoDir "go.mod") $OriginalGoMod
  Copy-Item (Join-Path $RepoDir "go.sum") $OriginalGoSum
  $TidyComplete = $false
  $restore = {
    Copy-Item $OriginalGoMod (Join-Path $RepoDir "go.mod") -Force
    Copy-Item $OriginalGoSum (Join-Path $RepoDir "go.sum") -Force
  }
  try {
    try {
      Invoke-Step "go mod tidy" {
        go mod tidy
      }
    } catch {
      & $restore
      throw
    }
    $GoModChanged = -not ((Get-Content (Join-Path $RepoDir "go.mod") -Raw) -ceq (Get-Content $OriginalGoMod -Raw))
    $GoSumChanged = -not ((Get-Content (Join-Path $RepoDir "go.sum") -Raw) -ceq (Get-Content $OriginalGoSum -Raw))
    if ($GoModChanged -or $GoSumChanged) {
      & $restore
      throw "go.mod or go.sum is not tidy; run go mod tidy"
    }
    $TidyComplete = $true
  } finally {
    if (-not $TidyComplete) {
      & $restore
    }
    Remove-Item -Recurse -Force $Tmp -ErrorAction SilentlyContinue
  }
}

Push-Location $RepoDir
try {
  Invoke-Step "go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12" {
    go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12
  }
  $Sh = Get-Command sh -ErrorAction SilentlyContinue
  if ($Sh) {
    foreach ($Script in Get-ChildItem -Path (Join-Path $RepoDir "scripts") -Filter "*.sh") {
      $ScriptPath = $Script.FullName
      Invoke-Step "sh -n $ScriptPath" {
        & $Sh.Source -n $ScriptPath
      }
    }
  } else {
    Write-Host ""
    Write-Host "==> sh -n scripts/*.sh"
    Write-Host "skipped: sh not found"
  }
  $ShellScripts = Get-ChildItem -Path (Join-Path $RepoDir "scripts") -Filter "*.sh" | ForEach-Object { $_.FullName }
  Invoke-Step "go run mvdan.cc/sh/v3/cmd/shfmt@$ShfmtVersion -d -i 2 -ci scripts/*.sh" {
    go run "mvdan.cc/sh/v3/cmd/shfmt@$ShfmtVersion" -d -i 2 -ci @ShellScripts
  }
  Invoke-Step "gofmt -l ." {
    $Files = gofmt -l .
    if ($Files) {
      $Files
      throw "Run gofmt before committing."
    }
  }
  Test-GoModTidy
  Invoke-Step "go test ./..." {
    go test ./...
  }
  Invoke-Step "go vet ./..." {
    go vet ./...
  }
  Invoke-Step "go run honnef.co/go/tools/cmd/staticcheck@$StaticcheckVersion ./..." {
    go run "honnef.co/go/tools/cmd/staticcheck@$StaticcheckVersion" ./...
  }
  Invoke-Step "go run golang.org/x/vuln/cmd/govulncheck@$GovulncheckVersion ./..." {
    go run "golang.org/x/vuln/cmd/govulncheck@$GovulncheckVersion" ./...
  }
  Invoke-Step "RUN_INSTALL_SMOKE=1 ./scripts/smoke.ps1" {
    $oldRunInstallSmoke = [Environment]::GetEnvironmentVariable("RUN_INSTALL_SMOKE", "Process")
    $env:RUN_INSTALL_SMOKE = "1"
    try {
      ./scripts/smoke.ps1
    } finally {
      if ($null -eq $oldRunInstallSmoke) {
        Remove-Item Env:\RUN_INSTALL_SMOKE -ErrorAction SilentlyContinue
      } else {
        $env:RUN_INSTALL_SMOKE = $oldRunInstallSmoke
      }
    }
  }
  Invoke-Step "go run github.com/goreleaser/goreleaser/v2@v2.15.4 check" {
    go run github.com/goreleaser/goreleaser/v2@v2.15.4 check
  }

  if ($Release) {
    try {
      Invoke-Step "go run github.com/goreleaser/goreleaser/v2@v2.15.4 release --snapshot --clean" {
        go run github.com/goreleaser/goreleaser/v2@v2.15.4 release --snapshot --clean
      }
      Invoke-Step "./scripts/archive-smoke.ps1" {
        ./scripts/archive-smoke.ps1
      }
    } finally {
      Remove-Item -Recurse -Force (Join-Path $RepoDir "dist") -ErrorAction SilentlyContinue
    }
  }

  Write-Host ""
  Write-Host "verify ok"
} finally {
  Pop-Location
}
