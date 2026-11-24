[CmdletBinding()]
param(
    [string]$OutputPath = (Join-Path $PSScriptRoot '..\dist\test-results\pester.xml')
)

Set-StrictMode -Version Latest

$ErrorActionPreference = 'Stop'

Import-Module Pester -MinimumVersion 5.5.0

$outputDirectory = Split-Path -Parent $OutputPath
if (-not (Test-Path -LiteralPath $outputDirectory)) {
    New-Item -ItemType Directory -Path $outputDirectory | Out-Null
}

$configuration = New-PesterConfiguration
$configuration.Run.Path = Join-Path $PSScriptRoot 'guest'
$configuration.Run.Exit = $true
$configuration.Output.Verbosity = 'Detailed'
$configuration.TestResult.Enabled = $true
$configuration.TestResult.OutputFormat = 'JUnitXml'
$configuration.TestResult.OutputPath = $OutputPath
Invoke-Pester -Configuration $configuration
