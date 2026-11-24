[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidateSet(
        'basic', 'microsoft-update', 'all-updates', 'wsus', 'windows-server-2012',
        'windows-server-modern', 'kb-include', 'kb-exclude', 'offline', 'advanced-filtering',
        'upstream-filters', 'json-reporting'
    )]
    [string]$Example,

    [ValidateSet('Initialize', 'Validate', 'Build')]
    [string]$Action = 'Build',

    [string]$BaseVmName,

    [string]$SwitchName = 'Default Switch',

    [pscredential]$Credential,

    [string]$PluginPath,

    [string]$VariableFile,

    [string]$LogDirectory = (Join-Path $PSScriptRoot 'logs')
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function Assert-Command {
    param([string]$Name)
    if (-not (Get-Command -Name $Name -ErrorAction SilentlyContinue)) {
        throw "$Name was not found on PATH."
    }
}

function Assert-HyperVAccess {
    $currentIdentity = [System.Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = New-Object System.Security.Principal.WindowsPrincipal($currentIdentity)
    $isAdministrator = $principal.IsInRole([System.Security.Principal.WindowsBuiltInRole]::Administrator)
    $isHyperVAdministrator = $principal.IsInRole((New-Object System.Security.Principal.SecurityIdentifier('S-1-5-32-578')))
    if (-not ($isAdministrator -or $isHyperVAdministrator)) {
        throw 'Building with Hyper-V requires an elevated session or membership of the Hyper-V Administrators group.'
    }
    if (-not (Get-VM -Name $BaseVmName -ErrorAction SilentlyContinue)) {
        throw "The base VM '$BaseVmName' does not exist."
    }
}

function Invoke-Packer {
    param([string[]]$Arguments)
    Write-Information -MessageData "packer $($Arguments -join ' ')" -InformationAction Continue
    & packer @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "packer $($Arguments[0]) failed with exit code $LASTEXITCODE."
    }
}

Assert-Command -Name 'packer'
$env:CHECKPOINT_DISABLE = '1'

if ($PluginPath) {
    Invoke-Packer -Arguments @('plugins', 'install', '--path', (Resolve-Path $PluginPath).Path, 'github.com/larskroghdk/microsoft-update')
}

Invoke-Packer -Arguments @('init', $PSScriptRoot)
if ($Action -eq 'Initialize') {
    return
}

if ($Action -eq 'Build') {
    if (-not $BaseVmName) {
        throw 'BaseVmName is required to build an example.'
    }
    Assert-HyperVAccess
}

if ($Action -eq 'Build' -and -not $Credential -and -not $env:PKR_VAR_winrm_password) {
    $Credential = Get-Credential -Message 'WinRM credential of the base VM' -UserName 'Administrator'
}

$previousPassword = $env:PKR_VAR_winrm_password
try {
    $packerArguments = New-Object System.Collections.Generic.List[string]
    if ($Credential) {
        $env:PKR_VAR_winrm_password = $Credential.GetNetworkCredential().Password
        $packerArguments.Add("-var=winrm_username=$($Credential.UserName)")
    }
    if ($BaseVmName) {
        $packerArguments.Add("-var=clone_from_vm_name=$BaseVmName")
    }
    else {
        $packerArguments.Add('-var=clone_from_vm_name=validation-only')
    }
    $packerArguments.Add("-var=switch_name=$SwitchName")
    if ($VariableFile) {
        $packerArguments.Add("-var-file=$((Resolve-Path $VariableFile).Path)")
    }

    if ($Action -eq 'Validate') {
        Invoke-Packer -Arguments (@('validate', "-only=$Example.null.validation") + $packerArguments + @($PSScriptRoot))
        return
    }

    if (-not (Test-Path -LiteralPath $LogDirectory)) {
        New-Item -ItemType Directory -Path $LogDirectory | Out-Null
    }
    $env:PACKER_LOG = '1'
    $env:PACKER_LOG_PATH = Join-Path $LogDirectory ('{0}-{1:yyyyMMdd-HHmmss}.log' -f $Example, (Get-Date))
    Invoke-Packer -Arguments (@('build', '-on-error=abort', '-force', "-only=$Example.hyperv-vmcx.windows") + $packerArguments + @($PSScriptRoot))
    Write-Information -MessageData "Packer log: $env:PACKER_LOG_PATH" -InformationAction Continue
}
finally {
    $env:PKR_VAR_winrm_password = $previousPassword
    Remove-Item Env:\PACKER_LOG, Env:\PACKER_LOG_PATH -ErrorAction SilentlyContinue
}
