[CmdletBinding()]
param(
    [string]$Example = 'basic',
    [ValidateSet(
        'Validate',
        'Build'
    )]
    [string]$Action = 'Validate',
    [string]$BaseVmName,
    [string]$SwitchName = 'Default Switch',
    [pscredential]$Credential,
    [string]$VariableFile
)

Set-StrictMode -Version Latest

$ErrorActionPreference = 'Stop'

$repositoryRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$testRoot = Join-Path $repositoryRoot 'dist\test'
$pluginPath = Join-Path $repositoryRoot ('dist\packer-plugin-microsoft-update' + (& go env GOEXE))

if (Test-Path -LiteralPath $testRoot) {
    Remove-Item -LiteralPath $testRoot -Recurse -Force
}
New-Item -ItemType Directory -Path (Join-Path $testRoot 'plugins') -Force | Out-Null

& go build -trimpath -o $pluginPath $repositoryRoot
if ($LASTEXITCODE -ne 0) {
    throw 'go build failed.'
}

$isolatedEnvironment = @{
    CHECKPOINT_DISABLE = '1'
    PACKER_CONFIG_DIR  = $testRoot
    PACKER_PLUGIN_PATH = Join-Path $testRoot 'plugins'
}
$previousEnvironment = @{}
foreach ($name in $isolatedEnvironment.Keys) {
    $previousEnvironment[$name] = [System.Environment]::GetEnvironmentVariable($name)
    [System.Environment]::SetEnvironmentVariable($name, $isolatedEnvironment[$name])
}
try {
    $exampleArguments = @{
        Example      = $Example
        Action       = $Action
        SwitchName   = $SwitchName
        PluginPath   = $pluginPath
        LogDirectory = $testRoot
    }
    if ($BaseVmName) {
        $exampleArguments['BaseVmName'] = $BaseVmName
    }
    if ($Credential) {
        $exampleArguments['Credential'] = $Credential
    }
    if ($VariableFile) {
        $exampleArguments['VariableFile'] = $VariableFile
    }
    & (Join-Path $repositoryRoot 'examples\Invoke-Example.ps1') @exampleArguments
}
finally {
    foreach ($name in $previousEnvironment.Keys) {
        [System.Environment]::SetEnvironmentVariable($name, $previousEnvironment[$name])
    }
}
