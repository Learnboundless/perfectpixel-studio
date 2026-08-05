param(
  [decimal]$BudgetCny = 30,
  [switch]$ConfirmCost,
  [string]$Out = ""
)

$ErrorActionPreference = "Stop"

$costPlan = @(
  [pscustomobject]@{ Name = "prompt_optimization_image"; Provider = "volcengine"; Model = "configured text model"; EstimatedCostCny = 0.25 },
  [pscustomobject]@{ Name = "prompt_optimization_ui"; Provider = "volcengine"; Model = "configured text model"; EstimatedCostCny = 0.25 },
  [pscustomobject]@{ Name = "text_to_image"; Provider = "zetatechs"; Model = "gpt-image-2"; EstimatedCostCny = 2.00 },
  [pscustomobject]@{ Name = "reference_to_ui"; Provider = "zetatechs"; Model = "gpt-image-2"; EstimatedCostCny = 2.00 },
  [pscustomobject]@{ Name = "masked_ui_edit"; Provider = "zetatechs"; Model = "gpt-image-2"; EstimatedCostCny = 2.00 },
  [pscustomobject]@{ Name = "multimodal_ui_analysis"; Provider = "zetatechs"; Model = "gpt-4o-mini"; EstimatedCostCny = 1.00 },
  [pscustomobject]@{ Name = "text_to_video_4s_480p"; Provider = "volcengine"; Model = "doubao-seedance-2-0-mini-260615"; EstimatedCostCny = 8.00 },
  [pscustomobject]@{ Name = "image_to_video_4s_480p"; Provider = "volcengine"; Model = "doubao-seedance-2-0-mini-260615"; EstimatedCostCny = 8.00 }
)

$estimatedTotal = [decimal](($costPlan | Measure-Object -Property EstimatedCostCny -Sum).Sum)
$costPlan | Format-Table -AutoSize
Write-Host ("Highest estimated total: {0:N2} CNY" -f $estimatedTotal)
Write-Host ("Authorized budget: {0:N2} CNY; hard creation gate: 25.00 CNY; absolute authorization ceiling: 30.00 CNY" -f $BudgetCny)

if ($BudgetCny -le 0 -or $BudgetCny -gt 30) {
  throw "BudgetCny must be greater than 0 and no more than 30."
}
if ($estimatedTotal -gt 25) {
  throw "Refusing paid creation because the highest estimated total exceeds 25 CNY."
}
if ($estimatedTotal -gt $BudgetCny) {
  throw "Refusing paid creation because the highest estimated total exceeds the authorized budget."
}
if (-not $ConfirmCost) {
  Write-Host "Dry run only. Add -ConfirmCost to execute the paid whitelist through GenerationJob APIs."
  return
}

$root = Split-Path -Parent $PSScriptRoot
$stamp = Get-Date -Format "yyyyMMdd-HHmmss"
if ([string]::IsNullOrWhiteSpace($Out)) {
  $Out = Join-Path $root ".codex\toolchain-acceptance\$stamp"
}
$resolvedOut = [System.IO.Path]::GetFullPath($Out)
$workspaceRoot = [System.IO.Path]::GetFullPath($root)
if (-not $resolvedOut.StartsWith($workspaceRoot, [System.StringComparison]::OrdinalIgnoreCase)) {
  throw "Acceptance output must stay inside the repository workspace."
}
New-Item -ItemType Directory -Path $resolvedOut -Force | Out-Null

$ffmpegBin = Join-Path $root ".tools\ffmpeg\bin"
if (-not (Test-Path -LiteralPath (Join-Path $ffmpegBin "ffprobe.exe")) -or -not (Test-Path -LiteralPath (Join-Path $ffmpegBin "ffmpeg.exe"))) {
  throw "ffmpeg and ffprobe are required in .tools/ffmpeg/bin before any paid calls."
}
$env:PATH = "$ffmpegBin;$env:PATH"
$env:PP_TOOLCHAIN_REAL_ACCEPTANCE = "1"
$env:PP_TOOLCHAIN_ACCEPTANCE_BUDGET_CNY = ([string]$BudgetCny)
$env:PP_TOOLCHAIN_ACCEPTANCE_ROOT = Join-Path $resolvedOut "toolchain"
$env:PP_TOOLCHAIN_ACCEPTANCE_REPORT = Join-Path $resolvedOut "acceptance-report.json"

Write-Host "Paid acceptance output: $resolvedOut"
go test -count=1 -run '^TestRealToolchainAcceptance$' -v .
if ($LASTEXITCODE -ne 0) {
  throw "Real toolchain acceptance failed. Later cases stop automatically after the first failure."
}
Write-Host "Acceptance report: $env:PP_TOOLCHAIN_ACCEPTANCE_REPORT"
