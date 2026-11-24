BeforeDiscovery {
    $script:scriptFiles = @(
        @{ Name = 'microsoft-update.ps1'; Path = Join-Path $PSScriptRoot '..\..\internal\guest\scripts\microsoft-update.ps1' }
        @{ Name = 'elevated-runner.ps1'; Path = Join-Path $PSScriptRoot '..\..\internal\guest\scripts\elevated-runner.ps1' }
    )
}

Describe 'Windows PowerShell 2.0 compatibility of <Name>' -ForEach $scriptFiles {
    BeforeAll {
        Import-Module (Join-Path $PSScriptRoot 'TestHelpers.psm1') -Force
        $script:source = Get-RenderedScriptSource -Path (Resolve-Path $Path).Path
        $tokens = $null
        $parseErrors = $null
        $syntaxTree = [System.Management.Automation.Language.Parser]::ParseInput($source, [ref]$tokens, [ref]$parseErrors)
        $script:commandNames = @($syntaxTree.FindAll(
                {
                    param($node)
                    $node -is [System.Management.Automation.Language.CommandAst]
                }, $true) | ForEach-Object {
                $_.GetCommandName()
            } | Where-Object { $_ }
        )
        $script:typeNames = @($syntaxTree.FindAll(
                {
                    param($node)
                    $node -is [System.Management.Automation.Language.TypeExpressionAst] -or $node -is [System.Management.Automation.Language.ConvertExpressionAst]
                }, $true) | ForEach-Object {
                if ($_ -is [System.Management.Automation.Language.TypeExpressionAst]) {
                    $_.TypeName.FullName
                }
                else {
                    $_.Type.TypeName.FullName
                }
            }
        )
        $script:memberNames = @($syntaxTree.FindAll(
                {
                    param($node)
                    $node -is [System.Management.Automation.Language.InvokeMemberExpressionAst]
                }, $true) | ForEach-Object {
                $_.Member.Extent.Text
            }
        )
        $script:operators = @($tokens | Where-Object { $_.Text -match '^-[ci]?(not)?in$' } | ForEach-Object { $_.Text })
        $script:variableNames = @($tokens | Where-Object { $_.Kind -eq 'Variable' } | ForEach-Object { $_.Name })
    }
    It 'parses without errors' {
        $parseErrors | Should -BeNullOrEmpty
    }
    It 'uses no cmdlets introduced after PowerShell 2.0' {
        $newerCommands = @(
            'ConvertTo-Json',
            'ConvertFrom-Json',
            'Get-CimInstance',
            'Invoke-CimMethod',
            'Get-FileHash',
            'Invoke-RestMethod',
            'Invoke-WebRequest',
            'Get-ScheduledTask',
            'Register-ScheduledTask',
            'Start-ScheduledTask',
            'Unregister-ScheduledTask',
            'Get-WindowsUpdateLog',
            'Get-HotFix'
        )
        $commandNames | Where-Object { $newerCommands -contains $_ } | Should -BeNullOrEmpty
    }
    It 'uses no type accelerators introduced after PowerShell 2.0' {
        $typeNames | Where-Object { $_ -in 'ordered', 'pscustomobject', 'PSCustomObject' } | Should -BeNullOrEmpty
    }
    It 'does not call constructors with ::new' {
        $memberNames | Where-Object { $_ -eq 'new' } | Should -BeNullOrEmpty
    }
    It 'uses no -in or -notin operators' {
        $operators | Should -BeNullOrEmpty
    }
    It 'does not use $PSItem or $PSScriptRoot' {
        $variableNames | Where-Object { $_ -in 'PSItem', 'PSScriptRoot' } | Should -BeNullOrEmpty
    }
    It 'does not use the .Where() or .ForEach() methods' {
        $memberNames | Where-Object { $_ -in 'Where', 'ForEach' } | Should -BeNullOrEmpty
    }
    It 'does not use -Raw, -Depth, or -NoNewline parameters' {
        $tokens | Where-Object { $_.Kind -eq 'Parameter' -and $_.Text -in '-Raw', '-Depth', '-NoNewline' } | Should -BeNullOrEmpty
    }
    It 'passes PSScriptAnalyzer with the repository settings' -Skip:(-not (Get-Module -ListAvailable PSScriptAnalyzer)) {
        $settingsPath = Join-Path $PSScriptRoot '..\..\PSScriptAnalyzerSettings.psd1'
        $findings = $null
        foreach ($attempt in 1..3) {
            try {
                $findings = Invoke-ScriptAnalyzer -ScriptDefinition $source -Settings $settingsPath -ErrorAction Stop
                break
            }
            catch {
                if ($attempt -eq 3 -or $_.Exception.GetBaseException() -isnot [System.NullReferenceException]) {
                    throw
                }
            }
        }
        $findings | ForEach-Object { "$($_.RuleName) line $($_.Line): $($_.Message)" } | Should -BeNullOrEmpty
    }
}

Describe 'Windows PowerShell 2.0 engine' {
    It 'loads microsoft-update.ps1 functions in the version 2 engine' -Skip:(-not (Test-Path 'HKLM:\SOFTWARE\Microsoft\PowerShell\1\PowerShellEngine') -or -not (Get-ItemProperty 'HKLM:\SOFTWARE\Microsoft\PowerShell\1\PowerShellEngine' -ErrorAction SilentlyContinue | Where-Object { $_.PSObject.Properties['PowerShellVersion'] -and $_.PowerShellVersion -like '2.*' })) {
        $scriptPath = (Resolve-Path (Join-Path $PSScriptRoot '..\..\internal\guest\scripts\microsoft-update.ps1')).Path
        $output = & powershell.exe -Version 2 -NoProfile -Command "`$null = [System.Management.Automation.PSParser]::Tokenize((Get-Content '$scriptPath' | Out-String), [ref]`$null); `$PSVersionTable.PSVersion.Major"
        $output | Select-Object -Last 1 | Should -Be '2'
    }
}
