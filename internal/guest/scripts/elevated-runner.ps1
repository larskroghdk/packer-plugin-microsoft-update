Set-StrictMode -Version 2.0

$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

Remove-Item -LiteralPath $MyInvocation.MyCommand.Path -Force -ErrorAction SilentlyContinue

$taskName = <% psquote .TaskName %>
$scriptPath = <% psquote .ScriptPath %>
$requestPath = <% psquote .RequestPath %>
$logPath = <% psquote .LogPath %>
$taskUser = <% psquote .Username %>
$taskPassword = <% if .Password %><% psquote .Password %><% else %>$null<% end %>
$taskLogonType = <% if .Password %>1<% else %>5<% end %>
$heartbeatInterval = [System.TimeSpan]::FromSeconds(60)
$taskStateRunning = 4
$taskHasNotRunResult = 267011

function Send-LogOutput {
    param(
        [string]$Path,
        [long]$Offset,
        [switch]$Final
    )
    if (-not (Test-Path -LiteralPath $Path)) {
        return $Offset
    }
    $stream = New-Object System.IO.FileStream($Path, [System.IO.FileMode]::Open, [System.IO.FileAccess]::Read, [System.IO.FileShare]::ReadWrite)
    try {
        if ($stream.Length -le $Offset) {
            return $Offset
        }
        [void]$stream.Seek($Offset, [System.IO.SeekOrigin]::Begin)
        $buffer = New-Object byte[] ([int]($stream.Length - $Offset))
        $bytesRead = $stream.Read($buffer, 0, $buffer.Length)
        $completeLength = $bytesRead
        if (-not $Final) {
            $completeLength = 0
            for ($index = $bytesRead - 1; $index -ge 0; $index--) {
                if ($buffer[$index] -eq 10) {
                    $completeLength = $index + 1
                    break
                }
            }
        }
        if ($completeLength -eq 0) {
            return $Offset
        }
        $text = [System.Text.Encoding]::Default.GetString($buffer, 0, $completeLength)
        foreach ($line in ($text -split "`r?`n")) {
            if ($line.Length -gt 0) {
                [Console]::Out.WriteLine($line)
            }
        }
        [Console]::Out.Flush()
        return $Offset + $completeLength
    }
    finally {
        $stream.Dispose()
    }
}

function Write-Heartbeat {
    $processorLoad = (Get-WmiObject -Class Win32_Processor | Measure-Object -Property LoadPercentage -Average).Average
    $operatingSystem = Get-WmiObject -Class Win32_OperatingSystem
    $memoryUsedPercent = [int](100 * (1 - $operatingSystem.FreePhysicalMemory / $operatingSystem.TotalVisibleMemorySize))
    [Console]::Out.WriteLine(('@@MU@@ {{"type":"heartbeat","cpu_percent":{0},"memory_percent":{1}}}' -f [int]$processorLoad, $memoryUsedPercent))
    [Console]::Out.Flush()
}

$powerShellPath = Join-Path $env:SystemRoot 'System32\WindowsPowerShell\v1.0\powershell.exe'
$taskArguments = '/c ""{0}" -NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "{1}" -RequestPath "{2}" > "{3}" 2>&1"' -f $powerShellPath, $scriptPath, $requestPath, $logPath
$scheduler = New-Object -ComObject 'Schedule.Service'
$scheduler.Connect()
$rootFolder = $scheduler.GetFolder('\')
$taskDefinition = $scheduler.NewTask(0)
$taskDefinition.RegistrationInfo.Description = 'packer-plugin-microsoft-update guest operation'
$taskDefinition.Principal.RunLevel = 1
$taskDefinition.Settings.Enabled = $true
$taskDefinition.Settings.AllowDemandStart = $true
$taskDefinition.Settings.DisallowStartIfOnBatteries = $false
$taskDefinition.Settings.StopIfGoingOnBatteries = $false
$taskDefinition.Settings.MultipleInstances = 2
$taskDefinition.Settings.Priority = 4
$taskDefinition.Settings.ExecutionTimeLimit = <% psquote .ExecutionTimeLimit %>
$taskAction = $taskDefinition.Actions.Create(0)
$taskAction.Path = Join-Path $env:SystemRoot 'System32\cmd.exe'
$taskAction.Arguments = $taskArguments
$exitCode = 1
try {
    $registeredTask = $rootFolder.RegisterTaskDefinition($taskName, $taskDefinition, 6, $taskUser, $taskPassword, $taskLogonType, $null)
    $taskPassword = $null
    [void]$registeredTask.Run($null)
    $startStopwatch = [System.Diagnostics.Stopwatch]::StartNew()
    while ($registeredTask.State -ne $taskStateRunning -and $registeredTask.LastTaskResult -eq $taskHasNotRunResult) {
        if ($startStopwatch.Elapsed.TotalSeconds -gt 120) {
            throw "The scheduled task $taskName did not start within 120 seconds."
        }
        Start-Sleep -Milliseconds 500
    }
    $logOffset = [long]0
    $heartbeatStopwatch = [System.Diagnostics.Stopwatch]::StartNew()
    while ($registeredTask.State -eq $taskStateRunning) {
        Start-Sleep -Seconds 1
        $logOffset = Send-LogOutput -Path $logPath -Offset $logOffset
        if ($heartbeatStopwatch.Elapsed -ge $heartbeatInterval) {
            Write-Heartbeat
            $heartbeatStopwatch = [System.Diagnostics.Stopwatch]::StartNew()
        }
    }
    [void](Send-LogOutput -Path $logPath -Offset $logOffset -Final)
    $exitCode = [int]$registeredTask.LastTaskResult
}
finally {
    try { $rootFolder.DeleteTask($taskName, 0) } catch { [Console]::Out.WriteLine("Could not delete scheduled task ${taskName}: $($_.Exception.Message)") }
    Remove-Item -LiteralPath $logPath -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $requestPath -Force -ErrorAction SilentlyContinue
}
[Console]::Out.WriteLine(('@@MU-RUNNER@@ {{"exit_code":{0}}}' -f $exitCode))
exit 0
