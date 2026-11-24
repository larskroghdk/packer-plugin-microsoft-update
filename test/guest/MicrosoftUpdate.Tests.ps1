BeforeAll {
    Import-Module (Join-Path $PSScriptRoot 'TestHelpers.psm1') -Force
    $updateScriptPath = Get-GuestScriptPath -Name update
    . ([ScriptBlock]::Create((Get-ScriptFunctionDefinition -Path $updateScriptPath)))
    $script:InvariantCulture = [System.Globalization.CultureInfo]::InvariantCulture
    $script:RecordPrefix = '@@MU@@ '
    $script:TaskNamePrefix = 'packer-microsoft-update-'
    $script:MicrosoftUpdateServiceId = '7971f918-a847-4430-9279-4a52d1efe18d'
    $script:ClientApplicationId = 'packer-plugin-microsoft-update-tests'
    function New-FakeUpdate {
        param(
            [string]$Title = 'Security Update for Windows (KB5000001)',
            [int]$Type = 1,
            [bool]$BrowseOnly = $false
        )
        $installationBehavior = [pscustomobject]@{
            CanRequestUserInput = $false;
            RebootBehavior      = 1
        }
        return [pscustomobject]@{
            Title                    = $Title
            Identity                 = [pscustomobject]@{
                UpdateID       = 'ABCDEF01-2345-6789-ABCD-EF0123456789';
                RevisionNumber = 201
            }
            KBArticleIDs             = @('5000001')
            Categories               = @(
                [pscustomobject]@{
                    CategoryID = '0FA1201D-4330-4FA8-8AE9-B877473B6441';
                    Name       = 'Security Updates';
                    Type       = 'UpdateClassification'
                }
                [pscustomobject]@{
                    CategoryID = 'a3c2375d-0c8a-42f9-bce0-28333e198407';
                    Name       = 'Windows 10';
                    Type       = 'Product'
                }
            )
            Type                     = $Type
            BrowseOnly               = $BrowseOnly
            AutoSelectOnWebSites     = $true
            IsMandatory              = $false
            IsBeta                   = $false
            IsDownloaded             = $false
            InstallationBehavior     = $installationBehavior
            MaxDownloadSize          = [decimal]104857600
            MsrcSeverity             = 'Critical'
            LastDeploymentChangeTime = [datetime]'2026-11-11'
        }
    }
}

Describe 'ConvertTo-RecordJson' {
    It 'produces JSON that round-trips' {
        $value = @{
            text    = "quote `" backslash \ newline `n tab `t unicode æøå"
            number  = 42
            large   = [long]9007199254740993
            decimal = [decimal]1.5
            flag    = $true
            nothing = $null
            list    = @(1, 'two', @{ three = 3 })
            empty   = @()
        }
        $decoded = ConvertTo-RecordJson $value | ConvertFrom-Json
        $decoded.text | Should -BeExactly $value.text
        $decoded.number | Should -Be 42
        $decoded.decimal | Should -Be 1.5
        $decoded.flag | Should -BeTrue
        $decoded.nothing | Should -BeNullOrEmpty
        $decoded.list.Count | Should -Be 3
        $decoded.list[2].three | Should -Be 3
        @($decoded.empty).Count | Should -Be 0
    }
    It 'emits only ASCII characters' {
        $json = ConvertTo-RecordJson @{ title = 'Opdatering til Microsoft Office – sikkerhed' }
        [System.Text.Encoding]::ASCII.GetString([System.Text.Encoding]::ASCII.GetBytes($json)) | Should -BeExactly $json
    }
}

Describe 'Format-HResult' {
    It 'formats negative HRESULT values as unsigned hexadecimal' {
        Format-HResult (-2145124318) | Should -BeExactly '0x80240022'
        Format-HResult 0 | Should -BeExactly '0x00000000'
    }
}

Describe 'Read-Request' {
    It 'decodes base64 values and repeated names' {
        $path = Join-Path $TestDrive 'request.txt'
        $parameters = [ordered]@{
            action          = 'install';
            update_id       = @('a', 'b');
            search_criteria = "IsInstalled=0`r`nevil"
        }
        ConvertTo-RequestFile -Path $path -Parameters $parameters
        $request = Read-Request $path
        Get-RequestValue $request 'action' | Should -BeExactly 'install'
        Get-RequestValue $request 'missing' 'fallback' | Should -BeExactly 'fallback'
        Get-RequestValue $request 'search_criteria' | Should -BeExactly "IsInstalled=0`r`nevil"
        $ids = Get-RequestList $request 'update_id'
        $ids.Count | Should -Be 2
        (Get-RequestList $request 'missing').Count | Should -Be 0
    }
}

Describe 'ConvertTo-UpdateRecord' {
    It 'maps update metadata to stable identifiers' {
        $record = ConvertTo-UpdateRecord (New-FakeUpdate) @()
        $record.id | Should -BeExactly 'abcdef01-2345-6789-abcd-ef0123456789'
        $record.revision | Should -Be 201
        $record.kbs | Should -Be @('KB5000001')
        $record.categories[0].id | Should -BeExactly '0fa1201d-4330-4fa8-8ae9-b877473b6441'
        $record.type | Should -BeExactly 'software'
        $record.max_download_size | Should -Be 104857600
        $record.last_deployment_change | Should -BeExactly '2026-11-11'
        $record.ContainsKey('legacy_filter_match') | Should -BeFalse
    }
    It 'marks driver updates' {
        (ConvertTo-UpdateRecord (New-FakeUpdate -Type 2) @()).type | Should -BeExactly 'driver'
    }
    It 'tolerates missing optional properties' {
        $update = New-FakeUpdate
        $update.PSObject.Properties.Remove('BrowseOnly')
        $update.PSObject.Properties.Remove('LastDeploymentChangeTime')
        $record = ConvertTo-UpdateRecord $update @()
        $record.browse_only | Should -BeFalse
        $record.last_deployment_change | Should -BeExactly ''
    }
}

Describe 'Upstream filters' {
    It 'applies the first matching filter' {
        $filters = New-LegacyFilterList @('exclude:$_.Title -like ''*Preview*''', 'include:$true')
        Test-LegacyFilterList $filters (New-FakeUpdate -Title 'Cumulative Update Preview') | Should -BeFalse
        Test-LegacyFilterList $filters (New-FakeUpdate) | Should -BeTrue
    }
    It 'skips updates that match no filter' {
        $filters = New-LegacyFilterList @('include:$_.BrowseOnly')
        Test-LegacyFilterList $filters (New-FakeUpdate) | Should -BeFalse
    }
    It 'records the filter decision on the update record' {
        $filters = New-LegacyFilterList @('include:$true')
        (ConvertTo-UpdateRecord (New-FakeUpdate) $filters).legacy_filter_match | Should -BeTrue
    }
}

Describe 'Test-UpdateObjectValid' {
    It 'rejects update objects without expected members' {
        Test-UpdateObjectValid ([pscustomobject]@{ Title = 'broken' }) | Should -BeFalse
        Test-UpdateObjectValid (New-FakeUpdate) | Should -BeTrue
    }
}

Describe 'Get-OfflineInstallOutcome' {
    It 'maps <ExitCode> to <Result>' -ForEach @(
        @{ ExitCode = 0; Result = 'installed'; Reboot = $false }
        @{ ExitCode = 3010; Result = 'installed'; Reboot = $true }
        @{ ExitCode = 1641; Result = 'installed'; Reboot = $true }
        @{ ExitCode = 2359302; Result = 'already-installed'; Reboot = $false }
        @{ ExitCode = -2145124329; Result = 'not-applicable'; Reboot = $false }
        @{ ExitCode = -2146498530; Result = 'not-applicable'; Reboot = $false }
        @{ ExitCode = 1603; Result = 'failed'; Reboot = $false }
    ) {
        $outcome = Get-OfflineInstallOutcome $ExitCode
        $outcome.result | Should -BeExactly $Result
        $outcome.reboot_required | Should -Be $Reboot
    }
}

Describe 'Install-OfflinePackage' {
    It 'rejects a package whose checksum does not match' {
        $packagePath = Join-Path $TestDrive 'windows10.0-kb5000001-x64.msu'
        [System.IO.File]::WriteAllText($packagePath, 'not a real package')
        $outcome = Install-OfflinePackage $packagePath ('0' * 64)
        $outcome.result | Should -BeExactly 'rejected'
        $outcome.message | Should -Match 'checksum'
    }
    It 'rejects an unsigned package' {
        $packagePath = Join-Path $TestDrive 'windows10.0-kb5000002-x64.msu'
        [System.IO.File]::WriteAllText($packagePath, 'not a real package')
        $outcome = Install-OfflinePackage $packagePath ''
        $outcome.result | Should -BeExactly 'rejected'
        $outcome.signature_status | Should -Not -BeExactly 'Valid'
    }
    It 'rejects a missing package' {
        (Install-OfflinePackage (Join-Path $TestDrive 'missing.cab') '').result | Should -BeExactly 'rejected'
    }
    It 'computes SHA-256 like Get-FileHash' {
        $path = Join-Path $TestDrive 'hash.bin'
        [System.IO.File]::WriteAllText($path, 'hash me')
        Get-FileSha256 $path | Should -BeExactly (Get-FileHash -Algorithm SHA256 $path).Hash.ToLowerInvariant()
    }
}

Describe 'Get-PendingReboot' {
    It 'returns the documented structure' {
        $pendingReboot = Get-PendingReboot
        $pendingReboot.Keys | Should -Contain 'pending'
        $pendingReboot.pending | Should -BeOfType [bool]
        , $pendingReboot.reasons | Should -BeOfType [object[]]
        $pendingReboot.informational_reasons | Should -Not -Contain 'windows-update-agent'
    }
}

Describe 'Guest script process' {
    It 'reports the system capabilities for the detect action' {
        $result = Invoke-GuestScript -WorkingDirectory $TestDrive -Parameters ([ordered]@{ action = 'detect'; task_name = 'packer-microsoft-update-test-001' })
        $result.ExitCode | Should -Be 0
        $system = $result.Records | Where-Object { $_.type -eq 'system' }
        $system | Should -Not -BeNullOrEmpty
        $system.build | Should -BeGreaterThan 7600
        $system.powershell_version | Should -Match '^\d+\.\d+'
        $system.architecture | Should -BeIn @('AMD64', 'ARM64', 'x86')
        $system.PSObject.Properties.Name | Should -Not -Contain 'wsus_server'
        $result.Records[-1].type | Should -BeExactly 'done'
    }
    It 'reports pending restarts without waiting when the servicing timeout is zero' {
        $result = Invoke-GuestScript -WorkingDirectory $TestDrive -Parameters ([ordered]@{ action = 'pending-reboot'; servicing_timeout_seconds = '0' })
        $result.ExitCode | Should -Be 0
        ($result.Records | Where-Object { $_.type -eq 'pending-reboot' }).servicing_idle | Should -BeTrue
    }
    It 'reports an error record and exit code 1 for unknown actions' {
        $result = Invoke-GuestScript -WorkingDirectory $TestDrive -Parameters ([ordered]@{ action = 'format-disk' })
        $result.ExitCode | Should -Be 1
        $errorRecord = $result.Records | Where-Object { $_.type -eq 'error' }
        $errorRecord.message | Should -Match "Unknown action 'format-disk'"
        $errorRecord.hresult | Should -Match '^0x[0-9A-F]{8}$'
    }
    It 'writes every output line as a protocol record' {
        $result = Invoke-GuestScript -WorkingDirectory $TestDrive -Parameters ([ordered]@{ action = 'pending-reboot'; servicing_timeout_seconds = '0' })
        $result.Output | Where-Object { $_ -and -not $_.StartsWith('@@MU@@ ') } | Should -BeNullOrEmpty
    }
}
