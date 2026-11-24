@{
    Scenarios = @(
        @{
            Name            = 'windows-11-microsoft-update'
            OperatingSystem = 'Windows 11 Enterprise 25H2'
            BaseVmName      = 'mu-base-windows-11'
            Example         = 'microsoft-update'
            ExpectedStatus  = @('updated')
            ExpectedSource  = 'microsoft-update'
            MinimumPasses   = 1
        }
        @{
            Name               = 'windows-11-fully-updated'
            OperatingSystem    = 'Windows 11 Enterprise 25H2 (already updated)'
            BaseVmName         = 'mu-base-windows-11-updated'
            Example            = 'basic'
            ExpectedStatus     = @('updated')
            ExpectedSource     = 'windows-update'
            ExpectedMaxReboots = 0
        }
        @{
            Name            = 'windows-10-ltsc-basic'
            OperatingSystem = 'Windows 10 Enterprise LTSC 2021'
            BaseVmName      = 'mu-base-windows-10-ltsc'
            Example         = 'basic'
            ExpectedStatus  = @('updated')
            ExpectedSource  = 'windows-update'
        }
        @{
            Name            = 'windows-server-2025-core'
            OperatingSystem = 'Windows Server 2025 Datacenter (Server Core)'
            BaseVmName      = 'mu-base-windows-server-2025-core'
            Example         = 'windows-server-modern'
            ExpectedStatus  = @('updated')
            ExpectedSource  = 'microsoft-update'
        }
        @{
            Name            = 'windows-server-2022-desktop'
            OperatingSystem = 'Windows Server 2022 Datacenter (Desktop Experience)'
            BaseVmName      = 'mu-base-windows-server-2022'
            Example         = 'all-updates'
            ExpectedStatus  = @('updated')
            ExpectedSource  = 'microsoft-update'
        }
        @{
            Name            = 'windows-server-2019-json-report'
            OperatingSystem = 'Windows Server 2019 Datacenter'
            BaseVmName      = 'mu-base-windows-server-2019'
            Example         = 'json-reporting'
            ExpectedStatus  = @('updated', 'partial')
            ExpectedSource  = 'windows-update'
        }
        @{
            Name            = 'windows-server-2016-upstream-filters'
            OperatingSystem = 'Windows Server 2016 Datacenter'
            BaseVmName      = 'mu-base-windows-server-2016'
            Example         = 'upstream-filters'
            ExpectedStatus  = @('updated')
            ExpectedSource  = 'windows-update'
        }
        @{
            Name            = 'windows-server-2012-r2'
            OperatingSystem = 'Windows Server 2012 R2 Datacenter'
            BaseVmName      = 'mu-base-windows-server-2012-r2'
            Example         = 'windows-server-2012'
            ExpectedStatus  = @('updated', 'partial', 'unsupported')
            ExpectedSource  = 'microsoft-update'
        }
        @{
            Name            = 'windows-server-2012'
            OperatingSystem = 'Windows Server 2012 Datacenter'
            BaseVmName      = 'mu-base-windows-server-2012'
            Example         = 'windows-server-2012'
            ExpectedStatus  = @('updated', 'partial', 'unsupported')
            ExpectedSource  = 'microsoft-update'
        }
        @{
            Name            = 'windows-8-1'
            OperatingSystem = 'Windows 8.1 Enterprise'
            BaseVmName      = 'mu-base-windows-8-1'
            Example         = 'basic'
            ExpectedStatus  = @('updated', 'partial', 'unsupported')
            ExpectedSource  = 'windows-update'
        }
        @{
            Name            = 'windows-7-sp1'
            OperatingSystem = 'Windows 7 Enterprise SP1'
            BaseVmName      = 'mu-base-windows-7'
            Example         = 'basic'
            ExpectedStatus  = @('updated', 'partial', 'unsupported')
            ExpectedSource  = 'windows-update'
        }
        @{
            Name             = 'wsus'
            OperatingSystem  = 'Windows Server 2022 Datacenter'
            BaseVmName       = 'mu-base-windows-server-2022'
            Example          = 'wsus'
            ExpectedStatus   = @('updated')
            ExpectedSource   = 'wsus'
            RequiresVariable = 'wsus_server_url'
        }
        @{
            Name             = 'offline-packages'
            OperatingSystem  = 'Windows Server 2022 Datacenter'
            BaseVmName       = 'mu-base-windows-server-2022'
            Example          = 'offline'
            ExpectedStatus   = @('updated')
            ExpectedSource   = 'offline'
            RequiresVariable = 'offline_package_paths'
        }
        @{
            Name            = 'kb-exclusion'
            OperatingSystem = 'Windows 11 Enterprise 25H2'
            BaseVmName      = 'mu-base-windows-11'
            Example         = 'kb-exclude'
            ExpectedStatus  = @('updated')
            ExpectedSource  = 'windows-update'
        }
    )
}
