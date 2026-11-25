Set-StrictMode -Version Latest

$RepositoryRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$GuestScriptDirectory = Join-Path $RepositoryRoot 'internal\guest\scripts'
$UpdateScriptPath = Join-Path $GuestScriptDirectory 'microsoft-update.ps1'
$RunnerScriptPath = Join-Path $GuestScriptDirectory 'elevated-runner.ps1'

function Get-GuestScriptPath {
    <#
    .SYNOPSIS
    Returns the path of an embedded guest script, the update script or the elevated runner.
    #>
    param(
        [ValidateSet(
            'update',
            'runner'
        )]
        [string]$Name
    )
    if ($Name -eq 'runner') {
        return $RunnerScriptPath
    }
    return $UpdateScriptPath
}

function Get-RenderedScriptSource {
    <#
    .SYNOPSIS
    Reads a guest script and replaces its Go template placeholders so that PowerShell can parse it.
    #>
    param(
        [string]$Path
    )
    $source = [System.IO.File]::ReadAllText($Path)
    $source = $source -replace '<%\s*if \.\w+\s*%>.*?<%\s*else\s*%>(.*?)<%\s*end\s*%>', '$1'
    return $source -replace '<%\s*psquote \.\w+\s*%>', "'template-value'"
}

function Get-ScriptFunctionDefinition {
    <#
    .SYNOPSIS
    Returns the source of the named functions of a guest script, or of all its functions.
    #>
    param(
        [string]$Path,
        [string[]]$Name
    )
    $source = Get-RenderedScriptSource -Path $Path
    $tokens = $null
    $parseErrors = $null
    $syntaxTree = [System.Management.Automation.Language.Parser]::ParseInput($source, [ref]$tokens, [ref]$parseErrors)
    $functions = $syntaxTree.FindAll({ param($node) $node -is [System.Management.Automation.Language.FunctionDefinitionAst] }, $true)
    $selected = $functions | Where-Object { -not $Name -or $Name -contains $_.Name }
    return ($selected | ForEach-Object { $_.Extent.Text }) -join [Environment]::NewLine
}

function ConvertTo-RequestFile {
    <#
    .SYNOPSIS
    Writes a request file in the format the guest script reads: one name=base64(value) line per value.
    #>
    param(
        [string]$Path,
        [System.Collections.Specialized.OrderedDictionary]$Parameters
    )
    $lines = foreach ($name in $Parameters.Keys) {
        foreach ($value in @($Parameters[$name])) {
            '{0}={1}' -f $name, [Convert]::ToBase64String([System.Text.Encoding]::UTF8.GetBytes([string]$value))
        }
    }
    [System.IO.File]::WriteAllLines($Path, [string[]]$lines)
}

function Invoke-GuestScript {
    <#
    .SYNOPSIS
    Runs the update script with a request file and returns the records it writes.
    #>
    param(
        [System.Collections.Specialized.OrderedDictionary]$Parameters,
        [string]$WorkingDirectory
    )
    $requestPath = Join-Path $WorkingDirectory 'request.txt'
    ConvertTo-RequestFile -Path $requestPath -Parameters $Parameters
    $powerShellPath = Join-Path $env:SystemRoot 'System32\WindowsPowerShell\v1.0\powershell.exe'
    $output = & $powerShellPath -NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File $UpdateScriptPath -RequestPath $requestPath 2>&1
    $records = foreach ($line in $output) {
        $text = [string]$line
        if ($text.StartsWith('@@MU@@ ')) {
            $text.Substring(7) | ConvertFrom-Json
        }
    }
    return [pscustomobject]@{
        ExitCode = $LASTEXITCODE
        Records  = @($records)
        Output   = @($output | ForEach-Object { [string]$_ })
    }
}

Export-ModuleMember -Function Get-GuestScriptPath, Get-RenderedScriptSource, Get-ScriptFunctionDefinition, ConvertTo-RequestFile, Invoke-GuestScript
