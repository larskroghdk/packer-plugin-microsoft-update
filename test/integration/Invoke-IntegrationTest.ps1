[CmdletBinding()]
param(
    [string[]]$Scenario = @('all'),
    [string]$SwitchName = 'Default Switch',
    [string]$VariableFile,
    [string]$ResultDirectory = (Join-Path $PSScriptRoot 'results')
)

Set-StrictMode -Version Latest

$ErrorActionPreference = 'Stop'

$repositoryRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$examplesDirectory = Join-Path $repositoryRoot 'examples'
$reportDirectory = Join-Path $examplesDirectory 'artifacts'
$scenarioCatalog = Import-PowerShellDataFile -Path (Join-Path $PSScriptRoot 'scenarios.psd1')

function Get-SelectedScenario {
    param([string[]]$RequestedName)
    $requested = @($RequestedName | ForEach-Object { $_.Trim() } | Where-Object { $_ })
    if ($requested -contains 'all') {
        return $scenarioCatalog.Scenarios
    }
    foreach ($name in $requested) {
        $match = $scenarioCatalog.Scenarios | Where-Object { $_.Name -eq $name }
        if (-not $match) {
            throw "Unknown scenario '$name'. Known scenarios: $(($scenarioCatalog.Scenarios | ForEach-Object { $_.Name }) -join ', ')"
        }
        $match
    }
}

function Build-Plugin {
    $pluginPath = Join-Path $repositoryRoot 'dist\packer-plugin-microsoft-update.exe'
    & go build -trimpath -o $pluginPath $repositoryRoot
    if ($LASTEXITCODE -ne 0) {
        throw 'go build failed.'
    }
    return $pluginPath
}

function Test-Report {
    param(
        [hashtable]$Definition,
        [string]$ReportPath
    )
    $failures = New-Object System.Collections.Generic.List[string]
    if (-not (Test-Path -LiteralPath $ReportPath)) {
        $failures.Add("The report $ReportPath was not written.")
        return $failures
    }
    $report = Get-Content -LiteralPath $ReportPath -Raw | ConvertFrom-Json
    if ($report.schema_version -notlike '1.*') {
        $failures.Add("Unexpected schema version $($report.schema_version).")
    }
    if ($Definition.ExpectedStatus -notcontains $report.status) {
        $failures.Add("Status $($report.status) is not one of $($Definition.ExpectedStatus -join ', '): $($report.status_reason)")
    }
    if ($report.source.effective -ne $Definition.ExpectedSource) {
        $failures.Add("Effective source $($report.source.effective), expected $($Definition.ExpectedSource).")
    }
    if ($Definition.ContainsKey('ExpectedMaxReboots') -and $report.reboots -gt $Definition.ExpectedMaxReboots) {
        $failures.Add("Restarted $($report.reboots) times, expected at most $($Definition.ExpectedMaxReboots).")
    }
    if ($Definition.ContainsKey('MinimumPasses') -and $report.passes -lt $Definition.MinimumPasses) {
        $failures.Add("Ran $($report.passes) passes, expected at least $($Definition.MinimumPasses).")
    }
    $counted = @($report.updates | Where-Object { $_.state -eq 'installed' }).Count
    if ($counted -ne $report.summary.installed) {
        $failures.Add("Summary reports $($report.summary.installed) installed updates, the update list contains $counted.")
    }
    if ($report.status -eq 'updated' -and ($report.summary.remaining -gt 0 -or $report.summary.failed -gt 0)) {
        $failures.Add('Status is updated although updates remain or failed.')
    }
    return $failures
}

if (-not (Test-Path -LiteralPath $ResultDirectory)) {
    New-Item -ItemType Directory -Path $ResultDirectory | Out-Null
}

$pluginPath = Build-Plugin
$results = New-Object System.Collections.Generic.List[object]
$pluginInstalled = $false

foreach ($definition in Get-SelectedScenario -RequestedName $Scenario) {
    $startedAt = Get-Date
    $result = [ordered]@{
        scenario         = $definition.Name
        operating_system = $definition.OperatingSystem
        example          = $definition.Example
        started_at       = $startedAt.ToUniversalTime().ToString('o')
        status           = 'not-run'
        failures         = @()
        report           = $null
    }
    try {
        if (-not (Get-VM -Name $definition.BaseVmName -ErrorAction SilentlyContinue)) {
            $result.status = 'skipped'
            $result.failures = @("Base VM $($definition.BaseVmName) does not exist.")
            continue
        }
        if ($definition.ContainsKey('RequiresVariable') -and -not $VariableFile) {
            $result.status = 'skipped'
            $result.failures = @("Scenario requires the $($definition.RequiresVariable) variable in -VariableFile.")
            continue
        }
        $reportPath = Join-Path $reportDirectory "$($definition.Example).json"
        Get-ChildItem -Path $reportDirectory -Filter "$($definition.Example)*.json" -ErrorAction SilentlyContinue | Remove-Item -Force
        $exampleArguments = @{
            Example      = $definition.Example
            BaseVmName   = $definition.BaseVmName
            SwitchName   = $SwitchName
            LogDirectory = Join-Path $ResultDirectory 'logs'
        }
        if (-not $pluginInstalled) {
            $exampleArguments.PluginPath = $pluginPath
            $pluginInstalled = $true
        }
        if ($VariableFile) {
            $exampleArguments.VariableFile = $VariableFile
        }
        try {
            & (Join-Path $examplesDirectory 'Invoke-Example.ps1') @exampleArguments
        }
        catch {
            Write-Warning "Packer reported: $($_.Exception.Message)"
        }
        $reportFile = Get-ChildItem -Path $reportDirectory -Filter "$($definition.Example)*.json" -ErrorAction SilentlyContinue | Sort-Object LastWriteTime -Descending | Select-Object -First 1
        if ($reportFile) {
            $reportPath = $reportFile.FullName
            $result.report = Get-Content -LiteralPath $reportPath -Raw | ConvertFrom-Json
        }
        $failures = @(Test-Report -Definition $definition -ReportPath $reportPath)
        $result.failures = $failures
        if ($failures.Count -eq 0) {
            $result.status = 'passed'
        }
        else {
            $result.status = 'failed'
        }
    }
    catch {
        $result.status = 'failed'
        $result.failures = @($_.Exception.Message)
    }
    finally {
        $result.duration_seconds = [int]((Get-Date) - $startedAt).TotalSeconds
        $results.Add([pscustomobject]$result)
        $resultPath = Join-Path $ResultDirectory "$($definition.Name).json"
        [pscustomobject]$result | ConvertTo-Json -Depth 20 | Set-Content -LiteralPath $resultPath -Encoding utf8
        Write-Output ('{0,-40} {1}' -f $definition.Name, $result.status)
    }
}

$summaryPath = Join-Path $ResultDirectory 'summary.json'
$results | Select-Object scenario, operating_system, example, status, duration_seconds, failures | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath $summaryPath -Encoding utf8
Write-Output "Results: $summaryPath"
if (@($results | Where-Object { $_.status -eq 'failed' }).Count -gt 0) {
    exit 1
}
