[CmdletBinding()]
param(
    [string[]]$Path = @('internal', 'test', 'examples'),

    [int]$MaximumAttempts = 3
)

Set-StrictMode -Version Latest

$ErrorActionPreference = 'Stop'

$repositoryRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$settingsPath = Join-Path $repositoryRoot 'PSScriptAnalyzerSettings.psd1'

Import-Module PSScriptAnalyzer -MinimumVersion 1.24.0

function Invoke-FileAnalysis {
    param(
        [string]$FilePath,
        [int]$AttemptLimit
    )
    for ($attempt = 1; $attempt -le $AttemptLimit; $attempt++) {
        try {
            return @(Invoke-ScriptAnalyzer -Path $FilePath -Settings $settingsPath -ErrorAction Stop)
        }
        catch {
            if ($_.Exception.GetBaseException() -isnot [System.NullReferenceException]) {
                throw
            }
            Write-Warning "PSScriptAnalyzer failed on $FilePath (attempt $attempt of $AttemptLimit): $($_.Exception.Message)"
        }
    }
    throw "PSScriptAnalyzer failed on $FilePath $AttemptLimit times."
}

$exitCode = 0
try {
    $files = foreach ($relativePath in $Path) {
        Get-ChildItem -Path (Join-Path $repositoryRoot $relativePath) -Recurse -File -Include '*.ps1', '*.psm1', '*.psd1'
    }
    $findings = foreach ($file in $files) {
        Invoke-FileAnalysis -FilePath $file.FullName -AttemptLimit $MaximumAttempts
    }
    $findings = @($findings)
    if ($findings.Count -gt 0) {
        $findings | Sort-Object ScriptPath, Line | Format-Table -AutoSize -Wrap ScriptName, Line, Severity, RuleName, Message | Out-String -Width 200 | Write-Output
        Write-Output "$($findings.Count) findings in $(@($files).Count) files."
        $exitCode = 1
    }
    else {
        Write-Output "No findings in $(@($files).Count) files."
    }
}
catch {
    Write-Error -ErrorRecord $_ -ErrorAction Continue
    $exitCode = 2
}
[System.Environment]::Exit($exitCode)
