param(
  [string]$Destination = ".tools\ffmpeg",
  [string]$Url = "https://www.gyan.dev/ffmpeg/builds/ffmpeg-release-essentials.zip"
)

$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$dest = if ([System.IO.Path]::IsPathRooted($Destination)) {
  $Destination
} else {
  Join-Path $repoRoot $Destination
}
$dest = [System.IO.Path]::GetFullPath($dest)
$zip = Join-Path ([System.IO.Path]::GetTempPath()) "perfectpixel-ffmpeg.zip"
$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("perfectpixel-ffmpeg-" + [guid]::NewGuid().ToString("N"))

New-Item -ItemType Directory -Force -Path $dest | Out-Null

$existing = Get-ChildItem -Path $dest -Recurse -Filter ffmpeg.exe -ErrorAction SilentlyContinue | Select-Object -First 1
if ($existing) {
  Write-Host "ffmpeg already installed: $($existing.FullName)"
  exit 0
}

Write-Host "Downloading ffmpeg..."
Invoke-WebRequest -Uri $Url -OutFile $zip

Write-Host "Extracting ffmpeg..."
Expand-Archive -LiteralPath $zip -DestinationPath $tmp -Force

$ffmpeg = Get-ChildItem -Path $tmp -Recurse -Filter ffmpeg.exe | Select-Object -First 1
if (-not $ffmpeg) {
  throw "Downloaded archive did not contain ffmpeg.exe"
}

$targetBin = Join-Path $dest "bin"
New-Item -ItemType Directory -Force -Path $targetBin | Out-Null
Copy-Item -LiteralPath $ffmpeg.FullName -Destination (Join-Path $targetBin "ffmpeg.exe") -Force

$ffprobe = Get-ChildItem -Path $tmp -Recurse -Filter ffprobe.exe | Select-Object -First 1
if ($ffprobe) {
  Copy-Item -LiteralPath $ffprobe.FullName -Destination (Join-Path $targetBin "ffprobe.exe") -Force
}

Remove-Item -LiteralPath $tmp -Recurse -Force -ErrorAction SilentlyContinue
Remove-Item -LiteralPath $zip -Force -ErrorAction SilentlyContinue

$installed = Join-Path $targetBin "ffmpeg.exe"
Write-Host "ffmpeg installed: $installed"
& $installed -version | Select-Object -First 1
