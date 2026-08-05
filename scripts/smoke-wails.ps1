param(
  [int]$VitePort = 5190,
  [int]$WailsPort = 34115,
  [int]$TimeoutSeconds = 45,
  [switch]$SkipWails
)

$ErrorActionPreference = "Stop"

function Stop-TreeByPattern {
  param([string[]]$Patterns)
  $procs = Get-CimInstance Win32_Process | Where-Object {
    $cmd = $_.CommandLine
    if (-not $cmd) { return $false }
    foreach ($pattern in $Patterns) {
      if ($cmd -like $pattern) { return $true }
    }
    return $false
  }
  foreach ($proc in $procs) {
    Stop-Process -Id $proc.ProcessId -Force -ErrorAction SilentlyContinue
  }
}

function Wait-MotionPixel {
  param([string]$Url, [int]$TimeoutSeconds)
  $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
  $lastSeen = ""
  do {
    try {
      $resp = Invoke-WebRequest -UseBasicParsing -Uri $Url -TimeoutSec 3 -Headers @{
        Accept = "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"
      }
      $body = [string]$resp.Content
      $lastSeen = $body
      $hasTitle = $body.Contains("MotionPixel")
      $hasEntry = $body.Contains("src/main.tsx") -or $body.Contains("/assets/")
      if ($resp.StatusCode -eq 200 -and $hasTitle -and $hasEntry) {
        return $true
      }
    } catch {
      $lastSeen = $_.Exception.Message
    }
    Start-Sleep -Milliseconds 500
  } while ((Get-Date) -lt $deadline)
  if ($lastSeen.Length -gt 240) {
    $lastSeen = $lastSeen.Substring(0, 240)
  }
  Write-Host "Last response did not look like MotionPixel: $lastSeen"
  return $false
}

function Assert-MotionPixelNoAccept {
  param([string]$Url)
  try {
    $resp = Invoke-WebRequest -UseBasicParsing -Uri $Url -TimeoutSec 3
    $body = [string]$resp.Content
    if ($resp.StatusCode -eq 200 -and $body.Contains("MotionPixel") -and ($body.Contains("src/main.tsx") -or $body.Contains("/assets/"))) {
      return
    }
    throw "response did not look like MotionPixel"
  } catch {
    throw "No-Accept smoke failed for ${Url}: $($_.Exception.Message)"
  }
}

$root = Split-Path -Parent $PSScriptRoot
$frontend = Join-Path $root "frontend"
$viteOut = Join-Path $env:TEMP "motionpixel-vite-smoke.out.log"
$viteErr = Join-Path $env:TEMP "motionpixel-vite-smoke.err.log"
$wailsOut = Join-Path $env:TEMP "motionpixel-wails-smoke.out.log"
$wailsErr = Join-Path $env:TEMP "motionpixel-wails-smoke.err.log"
Remove-Item -LiteralPath $viteOut,$viteErr,$wailsOut,$wailsErr -ErrorAction SilentlyContinue

$npm = Get-Command npm.cmd -ErrorAction SilentlyContinue
if (-not $npm) {
  $npm = Get-Command npm -ErrorAction Stop
}

Write-Host "Starting Vite on 127.0.0.1:$VitePort..."
$vite = Start-Process -FilePath $npm.Source `
  -ArgumentList @("run","dev","--","--host","127.0.0.1","--port","$VitePort","--strictPort") `
  -WorkingDirectory $frontend `
  -RedirectStandardOutput $viteOut `
  -RedirectStandardError $viteErr `
  -WindowStyle Hidden `
  -PassThru

try {
  $viteURL = "http://127.0.0.1:$VitePort/"
  if (-not (Wait-MotionPixel -Url $viteURL -TimeoutSeconds $TimeoutSeconds)) {
    throw "Vite smoke failed: $viteURL did not return MotionPixel. See $viteOut and $viteErr"
  }
  Assert-MotionPixelNoAccept -Url $viteURL
  Write-Host "Vite smoke passed: $viteURL"

  if ($SkipWails) {
    return
  }

  $wails = Get-Command wails.exe -ErrorAction SilentlyContinue
  if (-not $wails) {
    $wails = Get-Command wails -ErrorAction Stop
  }
  Write-Host "Starting Wails dev server on localhost:$WailsPort..."
  $wailsProcess = Start-Process -FilePath $wails.Source `
    -ArgumentList @("dev","-browser","-s","-skipbindings","-frontenddevserverurl",$viteURL,"-devserver","localhost:$WailsPort","-noreload","-nogorebuild") `
    -WorkingDirectory $root `
    -RedirectStandardOutput $wailsOut `
    -RedirectStandardError $wailsErr `
    -WindowStyle Hidden `
    -PassThru
  try {
    $wailsURL = "http://localhost:$WailsPort/"
    if (-not (Wait-MotionPixel -Url $wailsURL -TimeoutSeconds $TimeoutSeconds)) {
      throw "Wails smoke failed: $wailsURL did not proxy MotionPixel. See $wailsOut and $wailsErr"
    }
    Assert-MotionPixelNoAccept -Url $wailsURL
    Write-Host "Wails smoke passed: $wailsURL"
  } finally {
    Stop-Process -Id $wailsProcess.Id -Force -ErrorAction SilentlyContinue
  }
} finally {
  Stop-Process -Id $vite.Id -Force -ErrorAction SilentlyContinue
  Stop-TreeByPattern @(
    "*$root\build\bin\motionpixel-dev.exe*",
    "*perfectpixel-studio\frontend\node_modules*",
    "*vite --host 127.0.0.1 --port $VitePort*",
    "*npm.cmd*run dev*$VitePort*"
  )
}
