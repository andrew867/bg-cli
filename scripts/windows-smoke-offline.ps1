# Offline Windows smoke for bg-cli analytics RC.
# Usage: .\scripts\windows-smoke-offline.ps1 [-Exe .\bg-cli.exe]
param(
  [string]$Exe = "bg-cli"
)

$ErrorActionPreference = "Stop"
$failed = 0
function Run-Step([string]$name, [scriptblock]$block) {
  Write-Host "==> $name"
  try {
    & $block
    if ($LASTEXITCODE -ne 0 -and $null -ne $LASTEXITCODE) { throw "exit $LASTEXITCODE" }
    Write-Host "PASS $name"
  } catch {
    Write-Host "FAIL $name : $_"
    $script:failed++
  }
}

Run-Step "version" { & $Exe version }
Run-Step "help analytics" { & $Exe account hatch report --help | Out-Null; & $Exe releases list --help | Out-Null }
Run-Step "forge butler analyze" {
  & $Exe account forge analyze --observations .\testdata\fixtures\forge-observations-butler-2026-11-06.json --pretty | Out-Null
}
Run-Step "forge observed-mixed" {
  & $Exe account forge optimize --observations .\testdata\fixtures\forge-observations-mixed-shards.json --mode observed-mixed --objective min-shards --pretty | Out-Null
}
Run-Step "forge mixed shard-budget 63" {
  & $Exe account forge optimize --observations .\testdata\fixtures\forge-observations-mixed-shards.json --mode observed-mixed --shard-budget 63 --objective min-shards --pretty | Out-Null
}

if ($failed -gt 0) {
  Write-Host "SMOKE FAILED ($failed)"
  exit 1
}
Write-Host "SMOKE OK"
exit 0
