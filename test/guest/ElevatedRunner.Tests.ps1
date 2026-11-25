BeforeAll {
    Import-Module (Join-Path $PSScriptRoot 'TestHelpers.psm1') -Force
    . ([ScriptBlock]::Create((Get-ScriptFunctionDefinition -Path (Get-GuestScriptPath -Name runner) -Name 'Send-LogOutput')))
}

Describe 'Send-LogOutput' {
    BeforeEach {
        $logPath = Join-Path $TestDrive 'task.log'
        Remove-Item -LiteralPath $logPath -ErrorAction SilentlyContinue
        $script:writer = $null
    }
    AfterEach {
        if ($writer) {
            $writer.Dispose()
        }
    }
    It 'returns the offset unchanged when the log does not exist' {
        Send-LogOutput -Path $logPath -Offset 0 | Should -Be 0
    }
    It 'forwards only complete lines until the final drain' {
        $stream = New-Object System.IO.FileStream($logPath, 'Create', 'Write', 'ReadWrite')
        $writer = New-Object System.IO.StreamWriter($stream)
        $writer.Write("first line`r`nsecond par")
        $writer.Flush()
        $output = & { Send-LogOutput -Path $logPath -Offset 0 } 6>&1
        $offset = $output | Select-Object -Last 1
        $offset | Should -Be ("first line`r`n").Length
        $writer.Write("tial`r`n")
        $writer.Flush()
        $offset = Send-LogOutput -Path $logPath -Offset $offset
        $offset | Should -Be ("first line`r`nsecond partial`r`n").Length
        $writer.Write('unterminated')
        $writer.Flush()
        $finalOffset = Send-LogOutput -Path $logPath -Offset $offset -Final
        $finalOffset | Should -Be ("first line`r`nsecond partial`r`nunterminated").Length
    }
    It 'reads a log that another process keeps open for writing' {
        $stream = New-Object System.IO.FileStream($logPath, 'Create', 'Write', 'Read')
        $writer = New-Object System.IO.StreamWriter($stream)
        $writer.Write("locked`n")
        $writer.Flush()
        { Send-LogOutput -Path $logPath -Offset 0 } | Should -Not -Throw
    }
}

Describe 'Runner template' {
    It 'renders every placeholder through psquote or a literal' {
        $source = [System.IO.File]::ReadAllText((Get-GuestScriptPath -Name runner))
        $placeholders = [regex]::Matches($source, '<%\s*(.*?)\s*%>') | ForEach-Object { $_.Groups[1].Value }
        $placeholders | Where-Object { $_ -notmatch '^(psquote \.\w+|if \.Password|else|end)$' } | Should -BeNullOrEmpty
    }
    It 'clears the password variable after registering the task' {
        $source = [System.IO.File]::ReadAllText((Get-GuestScriptPath -Name runner))
        $source | Should -Match 'RegisterTaskDefinition[^\r\n]+\r?\n\s+\$taskPassword = \$null'
    }
    It 'deletes the scheduled task in a finally block' {
        $source = [System.IO.File]::ReadAllText((Get-GuestScriptPath -Name runner))
        $source | Should -Match '(?s)finally\s*\{.*DeleteTask'
    }
}
