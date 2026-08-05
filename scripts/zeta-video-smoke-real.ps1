param(
  [switch]$ListModels,
  [string]$Models = "@dynamic-video",
  [string]$Case = "auto",
  [string]$ModelFilter = "",
  [int]$Seconds = 4,
  [int]$FPS = 12,
  [string]$PollTimeout = "20m",
  [string]$PollInterval = "8s",
  [int]$MaxCases = 0,
  [string]$Out = "",
  [switch]$DryRun,
  [switch]$ConfirmCost
)

$ErrorActionPreference = "Stop"

$repo = Split-Path -Parent $PSScriptRoot
Push-Location $repo
try {
  if (-not $Out) {
    $stamp = Get-Date -Format "yyyyMMdd-HHmmss"
    $Out = ".codex/zeta-video-smoke-real-$stamp"
  }

  $env:GOCACHE = Join-Path (Get-Location) ".gocache"

  $cmdArgs = @(
    "./cmd/ppzeta-video-smoke",
    "-out", $Out,
    "-seconds", "$Seconds",
    "-fps", "$FPS",
    "-poll-timeout", $PollTimeout,
    "-poll-interval", $PollInterval
  )

  if ($ListModels) {
    $cmdArgs += "-list-models"
  } else {
    $cmdArgs += @("-models", $Models, "-case", $Case)
  }

  if ($ModelFilter) {
    $cmdArgs += @("-model-filter", $ModelFilter)
  }
  if ($MaxCases -gt 0) {
    $cmdArgs += @("-max-cases", "$MaxCases")
  }
  if ($DryRun) {
    $cmdArgs += "-dry-run"
  }
  if ($ConfirmCost) {
    $cmdArgs += "-confirm-cost"
  }

  go run @cmdArgs
} finally {
  Pop-Location
}
