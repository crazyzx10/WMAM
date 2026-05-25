param(
  [ValidateSet("current", "windows-amd64", "linux-amd64", "all")]
  [string]$Target = "current"
)

$ErrorActionPreference = "Stop"

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$goApp = Join-Path $repoRoot "go-app"
$dist = Join-Path $repoRoot "dist"
$env:GOCACHE = Join-Path $goApp ".gocache"
$env:GOPATH = Join-Path $goApp ".gopath"
$env:GOMODCACHE = Join-Path $goApp ".gomodcache"

New-Item -ItemType Directory -Path $env:GOCACHE -Force | Out-Null
New-Item -ItemType Directory -Path $env:GOPATH -Force | Out-Null
New-Item -ItemType Directory -Path $env:GOMODCACHE -Force | Out-Null

function Build-Frontend {
  Push-Location (Join-Path $goApp "web")
  try {
    npm run build
    if ($LASTEXITCODE -ne 0) {
      throw "Frontend build failed with exit code $LASTEXITCODE"
    }
  } finally {
    Pop-Location
  }
}

function Copy-CommonFiles {
  param([string]$OutDir)

  Copy-Item (Join-Path $goApp "config.yaml.example") (Join-Path $OutDir "config.yaml.example") -Force
  Copy-Item (Join-Path $repoRoot "README.md") (Join-Path $OutDir "README.md") -Force
  Copy-Item (Join-Path $repoRoot "WMAM-Deployment-Guide.md") (Join-Path $OutDir "WMAM-Deployment-Guide.md") -Force
}

function Build-GoTarget {
  param(
    [string]$Name,
    [string]$GOOS,
    [string]$GOARCH,
    [string]$BinaryName
  )

  $outDir = Join-Path $dist "wmam-$Name"
  New-Item -ItemType Directory -Path $outDir -Force | Out-Null

  $previousGOOS = $env:GOOS
  $previousGOARCH = $env:GOARCH
  try {
    $env:GOOS = $GOOS
    $env:GOARCH = $GOARCH
    Push-Location $goApp
    try {
      go build -trimpath -o (Join-Path $outDir $BinaryName) .
      if ($LASTEXITCODE -ne 0) {
        throw "Go build failed with exit code $LASTEXITCODE"
      }
    } finally {
      Pop-Location
    }
  } finally {
    $env:GOOS = $previousGOOS
    $env:GOARCH = $previousGOARCH
  }

  Copy-CommonFiles -OutDir $outDir
  Write-Host "Built $Name -> $outDir"
}

Build-Frontend
New-Item -ItemType Directory -Path $dist -Force | Out-Null

if ($Target -eq "current") {
  if ($IsWindows -or $env:OS -eq "Windows_NT") {
    Build-GoTarget -Name "windows-amd64" -GOOS "windows" -GOARCH "amd64" -BinaryName "wmam-server.exe"
  } else {
    Build-GoTarget -Name "linux-amd64" -GOOS "linux" -GOARCH "amd64" -BinaryName "wmam-server"
  }
} elseif ($Target -eq "windows-amd64") {
  Build-GoTarget -Name "windows-amd64" -GOOS "windows" -GOARCH "amd64" -BinaryName "wmam-server.exe"
} elseif ($Target -eq "linux-amd64") {
  Build-GoTarget -Name "linux-amd64" -GOOS "linux" -GOARCH "amd64" -BinaryName "wmam-server"
} else {
  Build-GoTarget -Name "windows-amd64" -GOOS "windows" -GOARCH "amd64" -BinaryName "wmam-server.exe"
  Build-GoTarget -Name "linux-amd64" -GOOS "linux" -GOARCH "amd64" -BinaryName "wmam-server"
}
