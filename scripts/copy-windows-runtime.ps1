param(
    [Parameter(Mandatory = $true)]
    [string]$BinaryPath
)

$projectRoot = Split-Path -Parent $PSScriptRoot
$binaryDirectory = Split-Path -Parent ([System.IO.Path]::GetFullPath($BinaryPath))
$runtimeSource = Join-Path $projectRoot "resources\background-removal\onnxruntime.dll"
$licensesSource = Join-Path $projectRoot "third_party\licenses"

Copy-Item -LiteralPath $runtimeSource -Destination (Join-Path $binaryDirectory "onnxruntime.dll") -Force
if (Test-Path -LiteralPath $licensesSource) {
    $licensesTarget = Join-Path $binaryDirectory "third_party\licenses"
    New-Item -ItemType Directory -Path $licensesTarget -Force | Out-Null
    Get-ChildItem -LiteralPath $licensesSource -File | ForEach-Object {
        Copy-Item -LiteralPath $_.FullName -Destination $licensesTarget -Force
    }
}

$noticesSource = Join-Path $projectRoot "THIRD_PARTY_NOTICES.md"
if (Test-Path -LiteralPath $noticesSource) {
    Copy-Item -LiteralPath $noticesSource -Destination (Join-Path $binaryDirectory "THIRD_PARTY_NOTICES.md") -Force
}
