param(
    [Parameter(Mandatory = $true)]
    [string]$RequestPath
)

Set-StrictMode -Version 2.0

$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$RecordPrefix = '@@MU@@ '
$MicrosoftUpdateServiceId = '7971f918-a847-4430-9279-4a52d1efe18d'
$ClientApplicationId = 'packer-plugin-microsoft-update'
$TaskNamePrefix = 'packer-microsoft-update-'
$InvariantCulture = [System.Globalization.CultureInfo]::InvariantCulture

function ConvertTo-JsonString {
    param([string]$Text)
    $builder = New-Object System.Text.StringBuilder
    [void]$builder.Append('"')
    foreach ($character in $Text.ToCharArray()) {
        $characterCode = [int]$character
        if ($characterCode -eq 34) {
            [void]$builder.Append('\"')
        }
        elseif ($characterCode -eq 92) {
            [void]$builder.Append('\\')
        }
        elseif ($characterCode -lt 32 -or $characterCode -gt 126) {
            [void]$builder.Append(('\u{0:x4}' -f $characterCode))
        }
        else {
            [void]$builder.Append($character)
        }
    }
    [void]$builder.Append('"')
    return $builder.ToString()
}

function ConvertTo-RecordJson {
    param($Value)
    if ($null -eq $Value) {
        return 'null'
    }
    if ($Value -is [bool]) {
        if ($Value) { return 'true' }
        return 'false'
    }
    if ($Value -is [string] -or $Value -is [char]) {
        return ConvertTo-JsonString ([string]$Value)
    }
    if ($Value -is [datetime]) {
        return ConvertTo-JsonString ($Value.ToUniversalTime().ToString('o', $InvariantCulture))
    }
    if ($Value -is [int] -or $Value -is [long] -or $Value -is [int16] -or $Value -is [byte] -or
        $Value -is [uint16] -or $Value -is [uint32] -or $Value -is [uint64] -or $Value -is [decimal] -or
        $Value -is [double] -or $Value -is [single]) {
        return [System.Convert]::ToString($Value, $InvariantCulture)
    }
    if ($Value -is [System.Collections.IDictionary]) {
        $members = New-Object System.Collections.ArrayList
        foreach ($key in $Value.Keys) {
            [void]$members.Add((ConvertTo-JsonString ([string]$key)) + ':' + (ConvertTo-RecordJson $Value[$key]))
        }
        return '{' + ($members.ToArray() -join ',') + '}'
    }
    if ($Value -is [System.Collections.IEnumerable]) {
        $items = New-Object System.Collections.ArrayList
        foreach ($item in $Value) {
            [void]$items.Add((ConvertTo-RecordJson $item))
        }
        return '[' + ($items.ToArray() -join ',') + ']'
    }
    return ConvertTo-JsonString ([string]$Value)
}

function Write-Record {
    param(
        [string]$Type,
        [hashtable]$Fields = @{}
    )
    $Fields['type'] = $Type
    [Console]::Out.WriteLine($RecordPrefix + (ConvertTo-RecordJson $Fields))
    [Console]::Out.Flush()
}

function Write-LogRecord {
    param(
        [string]$Level,
        [string]$Message
    )
    Write-Record 'log' @{ level = $Level; message = $Message }
}

function Write-Phase {
    param([string]$Name)
    Write-Record 'phase' @{ name = $Name }
}

function Format-HResult {
    param([int]$HResult)
    return '0x{0:X8}' -f $HResult
}

function Get-ErrorHResult {
    param($ErrorRecord)
    $exception = $ErrorRecord.Exception
    while ($exception -is [System.Management.Automation.MethodInvocationException] -and $null -ne $exception.InnerException) {
        $exception = $exception.InnerException
    }
    return [System.Runtime.InteropServices.Marshal]::GetHRForException($exception)
}

function Get-ErrorDetail {
    param($ErrorRecord)
    return @{
        hresult = Format-HResult (Get-ErrorHResult $ErrorRecord)
        message = [string]$ErrorRecord.Exception.Message
    }
}

function Read-Request {
    param([string]$Path)
    $request = @{}
    foreach ($line in [System.IO.File]::ReadAllLines($Path)) {
        if ($line.Length -eq 0) {
            continue
        }
        $separatorIndex = $line.IndexOf('=')
        $name = $line.Substring(0, $separatorIndex)
        $encodedValue = $line.Substring($separatorIndex + 1)
        $value = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String($encodedValue))
        if (-not $request.ContainsKey($name)) {
            $request[$name] = New-Object System.Collections.ArrayList
        }
        [void]$request[$name].Add($value)
    }
    return $request
}

function Get-RequestValue {
    param(
        [hashtable]$Request,
        [string]$Name,
        [string]$Default = ''
    )
    if ($Request.ContainsKey($Name)) {
        return [string]$Request[$Name][0]
    }
    return $Default
}

function Get-RequestList {
    param(
        [hashtable]$Request,
        [string]$Name
    )
    if ($Request.ContainsKey($Name)) {
        return , ([string[]]$Request[$Name].ToArray([string]))
    }
    return , ([string[]]@())
}

function Get-RegistryValue {
    param(
        [string]$Path,
        [string]$Name,
        $Default = $null
    )
    $item = Get-ItemProperty -Path $Path -ErrorAction SilentlyContinue
    if ($null -eq $item) {
        return $Default
    }
    $property = $item.PSObject.Properties[$Name]
    if ($null -eq $property -or $null -eq $property.Value) {
        return $Default
    }
    return $property.Value
}

function Get-ComPropertyValue {
    param(
        $ComObject,
        [string]$Name,
        $Default = $null
    )
    try {
        $value = $ComObject.$Name
        if ($null -eq $value) {
            return $Default
        }
        return $value
    }
    catch {
        return $Default
    }
}

function Test-ComMember {
    param(
        $ComObject,
        [string]$Name
    )
    return $null -ne ($ComObject | Get-Member -Name $Name -ErrorAction SilentlyContinue)
}

function Get-OperationResultName {
    param([int]$ResultCode)
    switch ($ResultCode) {
        0 { return 'not-started' }
        1 { return 'in-progress' }
        2 { return 'succeeded' }
        3 { return 'succeeded-with-errors' }
        4 { return 'failed' }
        5 { return 'aborted' }
    }
    return 'unknown'
}

function Get-ServiceStartMode {
    param([string]$Name)
    $service = Get-WmiObject -Class Win32_Service -Filter "Name='$Name'" -ErrorAction SilentlyContinue
    if ($null -eq $service) {
        return 'missing'
    }
    return [string]$service.StartMode
}

function Test-HotfixInstalled {
    param([string]$HotfixId)
    $hotfix = Get-WmiObject -Class Win32_QuickFixEngineering -Filter "HotFixID='$HotfixId'" -ErrorAction SilentlyContinue
    return $null -ne $hotfix
}

function Get-PendingReboot {
    $reasons = New-Object System.Collections.ArrayList
    $informationalReasons = New-Object System.Collections.ArrayList
    try {
        $systemInformation = New-Object -ComObject 'Microsoft.Update.SystemInfo'
        if ($systemInformation.RebootRequired) {
            [void]$reasons.Add('windows-update-agent')
        }
    }
    catch {
        [void]$informationalReasons.Add('windows-update-agent-unavailable')
    }
    $servicingPath = 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Component Based Servicing'
    if (Test-Path -Path "$servicingPath\RebootPending") {
        [void]$reasons.Add('component-based-servicing')
    }
    if (@(Get-ChildItem -Path "$servicingPath\PackagesPending" -ErrorAction SilentlyContinue).Count -gt 0) {
        [void]$reasons.Add('component-based-servicing-packages-pending')
    }
    if (Test-Path -Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\WindowsUpdate\Auto Update\RebootRequired') {
        [void]$reasons.Add('windows-update-auto-update')
    }
    $updateExeVolatile = Get-RegistryValue -Path 'HKLM:\SOFTWARE\Microsoft\Updates' -Name 'UpdateExeVolatile' -Default 0
    if ([int]$updateExeVolatile -ne 0) {
        [void]$reasons.Add('update-exe-volatile')
    }
    $sessionManagerPath = 'HKLM:\SYSTEM\CurrentControlSet\Control\Session Manager'
    if ($null -ne (Get-RegistryValue -Path $sessionManagerPath -Name 'PendingFileRenameOperations')) {
        [void]$informationalReasons.Add('pending-file-rename-operations')
    }
    return @{
        pending               = ($reasons.Count -gt 0)
        reasons               = $reasons.ToArray()
        informational_reasons = $informationalReasons.ToArray()
    }
}

function Wait-ServicingIdle {
    param(
        [int]$TimeoutSeconds,
        [int]$QuietSeconds = 15
    )
    if ($TimeoutSeconds -le 0) {
        return $true
    }
    Write-Phase 'servicing-wait'
    $stopwatch = [System.Diagnostics.Stopwatch]::StartNew()
    $quietSinceSeconds = -1
    while ($stopwatch.Elapsed.TotalSeconds -lt $TimeoutSeconds) {
        $servicingBusy = @(Get-Process -Name 'TiWorker' -ErrorAction SilentlyContinue).Count -gt 0
        if ($servicingBusy) {
            $quietSinceSeconds = -1
        }
        elseif ($quietSinceSeconds -lt 0) {
            $quietSinceSeconds = $stopwatch.Elapsed.TotalSeconds
        }
        elseif (($stopwatch.Elapsed.TotalSeconds - $quietSinceSeconds) -ge $QuietSeconds) {
            return $true
        }
        Start-Sleep -Seconds 2
    }
    return $false
}

function Remove-StaleTask {
    param([string]$CurrentTaskName)
    try {
        $scheduler = New-Object -ComObject 'Schedule.Service'
        $scheduler.Connect()
        $rootFolder = $scheduler.GetFolder('\')
        foreach ($task in @($rootFolder.GetTasks(1))) {
            $isOurTask = $task.Name.StartsWith($TaskNamePrefix, [System.StringComparison]::OrdinalIgnoreCase)
            $isRunning = $task.State -eq 4
            if ($isOurTask -and -not $isRunning -and $task.Name -ne $CurrentTaskName) {
                $rootFolder.DeleteTask($task.Name, 0)
                Write-LogRecord 'info' "Removed stale scheduled task $($task.Name)."
            }
        }
    }
    catch {
        Write-LogRecord 'warning' "Could not inspect scheduled tasks: $($_.Exception.Message)"
    }
}

function Get-SystemCapability {
    $operatingSystem = Get-WmiObject -Class Win32_OperatingSystem
    $currentVersionPath = 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion'
    $updatePolicyPath = 'HKLM:\SOFTWARE\Policies\Microsoft\Windows\WindowsUpdate'
    $osVersion = [System.Environment]::OSVersion.Version
    $productType = [int]$operatingSystem.ProductType
    $installationType = [string](Get-RegistryValue -Path $currentVersionPath -Name 'InstallationType' -Default '')
    if ($installationType.Length -eq 0) {
        if ($productType -eq 1) { $installationType = 'Client' } else { $installationType = 'Server' }
    }
    $displayVersion = [string](Get-RegistryValue -Path $currentVersionPath -Name 'DisplayVersion' -Default '')
    if ($displayVersion.Length -eq 0) {
        $displayVersion = [string](Get-RegistryValue -Path $currentVersionPath -Name 'ReleaseId' -Default '')
    }
    if ($displayVersion.Length -eq 0 -and $null -ne $operatingSystem.CSDVersion) {
        $displayVersion = [string]$operatingSystem.CSDVersion
    }
    $architecture = [string]$env:PROCESSOR_ARCHITEW6432
    if ($architecture.Length -eq 0) {
        $architecture = [string]$env:PROCESSOR_ARCHITECTURE
    }
    $agentVersion = ''
    try {
        $agentVersion = [string](New-Object -ComObject 'Microsoft.Update.AgentInfo').GetInfo('ProductVersionString')
    }
    catch {
        $agentVersion = 'unavailable'
    }
    $microsoftUpdateRegistered = $false
    $supportsAddService2 = $false
    try {
        $serviceManager = New-Object -ComObject 'Microsoft.Update.ServiceManager'
        $supportsAddService2 = Test-ComMember -ComObject $serviceManager -Name 'AddService2'
        foreach ($service in $serviceManager.Services) {
            if ($service.ServiceID -eq $MicrosoftUpdateServiceId) {
                $microsoftUpdateRegistered = $true
            }
        }
    }
    catch {
        Write-LogRecord 'warning' "Could not query the Windows Update service manager: $($_.Exception.Message)"
    }
    $wsusServer = [string](Get-RegistryValue -Path $updatePolicyPath -Name 'WUServer' -Default '')
    $useWsusServer = [int](Get-RegistryValue -Path "$updatePolicyPath\AU" -Name 'UseWUServer' -Default 0)
    $systemDrive = Get-WmiObject -Class Win32_LogicalDisk -Filter "DeviceID='$env:SystemDrive'" -ErrorAction SilentlyContinue
    $systemDriveFreeBytes = [long]0
    if ($null -ne $systemDrive) {
        $systemDriveFreeBytes = [long]$systemDrive.FreeSpace
    }
    $updateServiceStartMode = Get-ServiceStartMode 'wuauserv'
    $missingPrerequisites = New-Object System.Collections.ArrayList
    if ($updateServiceStartMode -eq 'Disabled') {
        [void]$missingPrerequisites.Add('The Windows Update service (wuauserv) is disabled.')
    }
    if ((Get-ServiceStartMode 'BITS') -eq 'Disabled') {
        [void]$missingPrerequisites.Add('The Background Intelligent Transfer Service (BITS) is disabled.')
    }
    if ($osVersion.Major -eq 6 -and $osVersion.Minor -eq 1) {
        if (-not (Test-HotfixInstalled 'KB4490628')) {
            [void]$missingPrerequisites.Add('KB4490628 (servicing stack update required for SHA-2 signed updates) is not installed.')
        }
        if (-not (Test-HotfixInstalled 'KB4474419')) {
            [void]$missingPrerequisites.Add('KB4474419 (SHA-2 code signing support) is not installed.')
        }
    }
    if ($systemDriveFreeBytes -gt 0 -and $systemDriveFreeBytes -lt 10GB) {
        [void]$missingPrerequisites.Add('The system drive has less than 10 GB of free space.')
    }
    $lifecycleNotice = ''
    if ($osVersion.Major -eq 6 -and $osVersion.Minor -le 3) {
        $lifecycleNotice = 'This Windows release is past the end of support. Microsoft publishes further updates only to systems with an Extended Security Updates license; the plugin neither checks nor bypasses ESU licensing.'
    }
    return @{
        caption                        = ([string]$operatingSystem.Caption).Trim()
        version                        = [string]$operatingSystem.Version
        build                          = [int](Get-RegistryValue -Path $currentVersionPath -Name 'CurrentBuildNumber' -Default $operatingSystem.BuildNumber)
        update_build_revision          = [int](Get-RegistryValue -Path $currentVersionPath -Name 'UBR' -Default 0)
        display_version                = $displayVersion
        edition_id                     = [string](Get-RegistryValue -Path $currentVersionPath -Name 'EditionID' -Default '')
        installation_type              = $installationType
        product_type                   = $productType
        architecture                   = $architecture
        language                       = [System.Globalization.CultureInfo]::InstalledUICulture.Name
        powershell_version             = $PSVersionTable.PSVersion.ToString()
        clr_version                    = $PSVersionTable.CLRVersion.ToString()
        windows_update_agent_version   = $agentVersion
        insider_branch                 = [string](Get-RegistryValue -Path 'HKLM:\SOFTWARE\Microsoft\WindowsSelfHost\Applicability' -Name 'BranchName' -Default '')
        wsus_configured                = ($wsusServer.Length -gt 0 -and $useWsusServer -eq 1)
        windows_update_access_disabled = ([int](Get-RegistryValue -Path $updatePolicyPath -Name 'DisableWindowsUpdateAccess' -Default 0) -eq 1)
        internet_locations_disabled    = ([int](Get-RegistryValue -Path $updatePolicyPath -Name 'DoNotConnectToWindowsUpdateInternetLocations' -Default 0) -eq 1)
        microsoft_update_registered    = $microsoftUpdateRegistered
        supports_add_service2          = $supportsAddService2
        update_service_start_mode      = $updateServiceStartMode
        system_drive_free_bytes        = $systemDriveFreeBytes
        lifecycle_notice               = $lifecycleNotice
        missing_prerequisites          = $missingPrerequisites.ToArray()
        pending_reboot                 = (Get-PendingReboot)
        boot_time                      = [string]$operatingSystem.LastBootUpTime
    }
}

function New-UpdateSession {
    $session = New-Object -ComObject 'Microsoft.Update.Session'
    $session.ClientApplicationID = $ClientApplicationId
    return $session
}

function New-UpdateSearcher {
    param(
        $Session,
        [string]$Source
    )
    $searcher = $Session.CreateUpdateSearcher()
    switch ($Source) {
        'default' { $searcher.ServerSelection = 0 }
        'wsus' { $searcher.ServerSelection = 1 }
        'windows-update' { $searcher.ServerSelection = 2 }
        'microsoft-update' {
            $searcher.ServerSelection = 3
            $searcher.ServiceID = $MicrosoftUpdateServiceId
        }
        default { throw "Unknown update source '$Source'." }
    }
    return $searcher
}

function New-LegacyFilterList {
    param([string[]]$FilterDefinitions)
    $filters = New-Object System.Collections.ArrayList
    foreach ($definition in $FilterDefinitions) {
        $action, $expression = $definition -split ':', 2
        [void]$filters.Add(@{
                Action     = $action
                Expression = [ScriptBlock]::Create($expression)
            })
    }
    return , $filters
}

function Test-LegacyFilterList {
    param(
        $Filters,
        $Update
    )
    foreach ($filter in $Filters) {
        if (Where-Object -InputObject $Update $filter.Expression) {
            return $filter.Action -eq 'include'
        }
    }
    return $false
}

function ConvertTo-UpdateRecord {
    param(
        $Update,
        $LegacyFilters
    )
    $knowledgeBaseIds = New-Object System.Collections.ArrayList
    foreach ($articleId in $Update.KBArticleIDs) {
        [void]$knowledgeBaseIds.Add('KB' + $articleId)
    }
    $categories = New-Object System.Collections.ArrayList
    foreach ($category in $Update.Categories) {
        [void]$categories.Add(@{
                id   = ([string]$category.CategoryID).ToLowerInvariant()
                name = [string]$category.Name
                type = [string]$category.Type
            })
    }
    $updateType = 'software'
    if ([int]$Update.Type -eq 2) {
        $updateType = 'driver'
    }
    $lastDeploymentChange = ''
    $deploymentChangeTime = Get-ComPropertyValue -ComObject $Update -Name 'LastDeploymentChangeTime'
    if ($null -ne $deploymentChangeTime) {
        $lastDeploymentChange = ([datetime]$deploymentChangeTime).ToString('yyyy-MM-dd', $InvariantCulture)
    }
    $installationBehavior = Get-ComPropertyValue -ComObject $Update -Name 'InstallationBehavior'
    $record = @{
        id                       = ([string]$Update.Identity.UpdateID).ToLowerInvariant()
        revision                 = [int]$Update.Identity.RevisionNumber
        title                    = [string]$Update.Title
        kbs                      = $knowledgeBaseIds.ToArray()
        categories               = $categories.ToArray()
        type                     = $updateType
        browse_only              = [bool](Get-ComPropertyValue -ComObject $Update -Name 'BrowseOnly' -Default $false)
        auto_select_on_web_sites = [bool](Get-ComPropertyValue -ComObject $Update -Name 'AutoSelectOnWebSites' -Default $false)
        is_mandatory             = [bool](Get-ComPropertyValue -ComObject $Update -Name 'IsMandatory' -Default $false)
        is_beta                  = [bool](Get-ComPropertyValue -ComObject $Update -Name 'IsBeta' -Default $false)
        is_downloaded            = [bool](Get-ComPropertyValue -ComObject $Update -Name 'IsDownloaded' -Default $false)
        can_request_user_input   = [bool](Get-ComPropertyValue -ComObject $installationBehavior -Name 'CanRequestUserInput' -Default $false)
        reboot_behavior          = [int](Get-ComPropertyValue -ComObject $installationBehavior -Name 'RebootBehavior' -Default 0)
        max_download_size        = [long](Get-ComPropertyValue -ComObject $Update -Name 'MaxDownloadSize' -Default 0)
        msrc_severity            = [string](Get-ComPropertyValue -ComObject $Update -Name 'MsrcSeverity' -Default '')
        last_deployment_change   = $lastDeploymentChange
    }
    if ($LegacyFilters.Count -gt 0) {
        $record['legacy_filter_match'] = Test-LegacyFilterList $LegacyFilters $Update
    }
    return $record
}

function Test-UpdateObjectValid {
    param($Update)
    foreach ($propertyName in @('Title', 'Identity', 'MaxDownloadSize', 'InstallationBehavior')) {
        if (-not (Test-ComMember -ComObject $Update -Name $propertyName)) {
            return $false
        }
    }
    return $true
}

function Invoke-Search {
    param(
        [hashtable]$Request,
        [switch]$EmitUpdates
    )
    $source = Get-RequestValue -Request $Request -Name 'source' -Default 'default'
    $criteria = Get-RequestValue -Request $Request -Name 'search_criteria' -Default 'IsInstalled=0 and IsHidden=0'
    $session = New-UpdateSession
    $searcher = New-UpdateSearcher $session $source
    Write-Phase 'search'
    Write-LogRecord 'info' "Searching for updates (source $source; criteria $criteria)..."
    try {
        $searchResult = $searcher.Search($criteria)
    }
    catch {
        $errorFields = Get-ErrorDetail $_
        $errorFields['result_code'] = 4
        $errorFields['result'] = 'failed'
        $errorFields['update_count'] = 0
        Write-Record 'search' $errorFields
        return $null
    }
    $resultCode = [int]$searchResult.ResultCode
    $updateCount = [int]$searchResult.Updates.Count
    if ($EmitUpdates) {
        $legacyFilters = New-LegacyFilterList (Get-RequestList -Request $Request -Name 'legacy_filter')
        for ($index = 0; $index -lt $updateCount; $index++) {
            $update = $searchResult.Updates.Item($index)
            if (-not (Test-UpdateObjectValid $update)) {
                Write-Record 'search' @{
                    result_code  = 4
                    result       = 'failed'
                    update_count = $updateCount
                    hresult      = Format-HResult (-2145124353)
                    message      = 'The Windows Update Agent returned an incomplete update object. This is a known defect of some preview builds of the Windows Update Agent; install the latest cumulative update or rebuild from newer media.'
                }
                return $null
            }
            Write-Record 'update' (ConvertTo-UpdateRecord $update $legacyFilters)
        }
    }
    Write-Record 'search' @{
        result_code  = $resultCode
        result       = Get-OperationResultName $resultCode
        update_count = $updateCount
        hresult      = Format-HResult 0
    }
    return @{ Session = $session; Result = $searchResult }
}

function Set-DownloadPriority {
    param($Downloader)
    foreach ($priority in @(4, 3)) {
        try {
            $Downloader.Priority = $priority
            return
        }
        catch {
            continue
        }
    }
}

function Invoke-Install {
    param([hashtable]$Request)
    $requestedIds = @{}
    foreach ($updateId in (Get-RequestList -Request $Request -Name 'update_id')) {
        $requestedIds[$updateId.ToLowerInvariant()] = $true
    }
    $search = Invoke-Search $Request
    if ($null -eq $search) {
        return
    }
    $session = $search.Session
    $foundIds = @{}
    $selectedUpdates = New-Object -ComObject 'Microsoft.Update.UpdateColl'
    for ($index = 0; $index -lt $search.Result.Updates.Count; $index++) {
        $update = $search.Result.Updates.Item($index)
        $updateId = ([string]$update.Identity.UpdateID).ToLowerInvariant()
        if (-not $requestedIds.ContainsKey($updateId) -or $foundIds.ContainsKey($updateId)) {
            continue
        }
        $foundIds[$updateId] = $true
        if (-not $update.EulaAccepted) {
            $update.AcceptEula()
        }
        [void]$selectedUpdates.Add($update)
    }
    foreach ($updateId in $requestedIds.Keys) {
        if (-not $foundIds.ContainsKey($updateId)) {
            Write-Record 'update-result' @{ id = $updateId; phase = 'search'; result = 'not-found'; result_code = 0; hresult = Format-HResult 0; reboot_required = $false }
        }
    }
    if ($selectedUpdates.Count -eq 0) {
        Write-Record 'install' @{ result_code = 0; result = 'not-started'; hresult = Format-HResult 0; reboot_required = $false }
        return
    }
    $updatesToDownload = New-Object -ComObject 'Microsoft.Update.UpdateColl'
    $totalDownloadBytes = [long]0
    for ($index = 0; $index -lt $selectedUpdates.Count; $index++) {
        $update = $selectedUpdates.Item($index)
        if (-not $update.IsDownloaded) {
            [void]$updatesToDownload.Add($update)
            $totalDownloadBytes += [long]$update.MaxDownloadSize
        }
    }
    if ($updatesToDownload.Count -gt 0) {
        Write-Phase 'download'
        Write-LogRecord 'info' ('Downloading {0} updates ({1:N1} MB)...' -f $updatesToDownload.Count, ($totalDownloadBytes / 1MB))
        $downloader = $session.CreateUpdateDownloader()
        $downloader.ClientApplicationID = $ClientApplicationId
        Set-DownloadPriority $downloader
        $downloader.Updates = $updatesToDownload
        try {
            $downloadResult = $downloader.Download()
            for ($index = 0; $index -lt $updatesToDownload.Count; $index++) {
                $updateResult = $downloadResult.GetUpdateResult($index)
                $resultCode = [int]$updateResult.ResultCode
                Write-Record 'update-result' @{
                    id              = ([string]$updatesToDownload.Item($index).Identity.UpdateID).ToLowerInvariant()
                    phase           = 'download'
                    result_code     = $resultCode
                    result          = Get-OperationResultName $resultCode
                    hresult         = Format-HResult $updateResult.HResult
                    reboot_required = $false
                }
            }
        }
        catch {
            $errorFields = Get-ErrorDetail $_
            for ($index = 0; $index -lt $updatesToDownload.Count; $index++) {
                Write-Record 'update-result' @{
                    id              = ([string]$updatesToDownload.Item($index).Identity.UpdateID).ToLowerInvariant()
                    phase           = 'download'
                    result_code     = 4
                    result          = 'failed'
                    hresult         = $errorFields.hresult
                    reboot_required = $false
                }
            }
            Write-LogRecord 'warning' "Download failed: $($errorFields.message)"
        }
    }
    $updatesToInstall = New-Object -ComObject 'Microsoft.Update.UpdateColl'
    for ($index = 0; $index -lt $selectedUpdates.Count; $index++) {
        $update = $selectedUpdates.Item($index)
        if ($update.IsDownloaded) {
            [void]$updatesToInstall.Add($update)
        }
    }
    if ($updatesToInstall.Count -eq 0) {
        Write-Record 'install' @{ result_code = 4; result = 'failed'; hresult = Format-HResult (-2145124318); reboot_required = $false }
        return
    }
    Write-Phase 'install'
    Write-LogRecord 'info' "Installing $($updatesToInstall.Count) updates..."
    $installer = $session.CreateUpdateInstaller()
    $installer.ClientApplicationID = $ClientApplicationId
    try { $installer.ForceQuiet = $true } catch { Write-LogRecord 'debug' 'ForceQuiet is not supported by this Windows Update Agent.' }
    try { $installer.AllowSourcePrompts = $false } catch { Write-LogRecord 'debug' 'AllowSourcePrompts is not supported by this Windows Update Agent.' }
    if ([bool](Get-ComPropertyValue -ComObject $installer -Name 'RebootRequiredBeforeInstallation' -Default $false)) {
        Write-Record 'install' @{ result_code = 0; result = 'not-started'; hresult = Format-HResult (-2147021879); reboot_required = $true }
        return
    }
    $installer.Updates = $updatesToInstall
    try {
        $installResult = $installer.Install()
    }
    catch {
        $errorFields = Get-ErrorDetail $_
        for ($index = 0; $index -lt $updatesToInstall.Count; $index++) {
            Write-Record 'update-result' @{
                id              = ([string]$updatesToInstall.Item($index).Identity.UpdateID).ToLowerInvariant()
                phase           = 'install'
                result_code     = 4
                result          = 'failed'
                hresult         = $errorFields.hresult
                reboot_required = $false
            }
        }
        Write-Record 'install' @{ result_code = 4; result = 'failed'; hresult = $errorFields.hresult; message = $errorFields.message; reboot_required = (Get-PendingReboot).pending }
        return
    }
    for ($index = 0; $index -lt $updatesToInstall.Count; $index++) {
        $updateResult = $installResult.GetUpdateResult($index)
        $resultCode = [int]$updateResult.ResultCode
        Write-Record 'update-result' @{
            id              = ([string]$updatesToInstall.Item($index).Identity.UpdateID).ToLowerInvariant()
            phase           = 'install'
            result_code     = $resultCode
            result          = Get-OperationResultName $resultCode
            hresult         = Format-HResult $updateResult.HResult
            reboot_required = [bool]$updateResult.RebootRequired
        }
    }
    $installResultCode = [int]$installResult.ResultCode
    $rebootRequired = [bool]$installResult.RebootRequired -or (Get-PendingReboot).pending
    Write-Record 'install' @{
        result_code     = $installResultCode
        result          = Get-OperationResultName $installResultCode
        hresult         = Format-HResult $installResult.HResult
        reboot_required = $rebootRequired
    }
}

function Register-MicrosoftUpdate {
    $serviceManager = New-Object -ComObject 'Microsoft.Update.ServiceManager'
    $serviceManager.ClientApplicationID = $ClientApplicationId
    foreach ($service in $serviceManager.Services) {
        if ($service.ServiceID -eq $MicrosoftUpdateServiceId) {
            Write-Record 'registration' @{ state = 'registered'; changed = $false; hresult = Format-HResult 0 }
            return
        }
    }
    if (-not (Test-ComMember -ComObject $serviceManager -Name 'AddService2')) {
        Write-Record 'registration' @{
            state   = 'unsupported'
            changed = $false
            hresult = Format-HResult 0
            message = 'This Windows Update Agent does not provide IUpdateServiceManager2.AddService2; update the Windows Update Agent or use update_source = "windows-update".'
        }
        return
    }
    try {
        $registration = $serviceManager.AddService2($MicrosoftUpdateServiceId, 7, '')
        $registrationState = 'not-registered'
        switch ([int]$registration.RegistrationState) {
            2 { $registrationState = 'pending' }
            3 { $registrationState = 'registered' }
        }
        Write-Record 'registration' @{ state = $registrationState; changed = $true; hresult = Format-HResult 0 }
    }
    catch {
        $errorFields = Get-ErrorDetail $_
        $errorFields['state'] = 'failed'
        $errorFields['changed'] = $false
        Write-Record 'registration' $errorFields
    }
}

function Get-FileSha256 {
    param([string]$Path)
    $hashAlgorithm = New-Object System.Security.Cryptography.SHA256Managed
    $stream = [System.IO.File]::OpenRead($Path)
    try {
        $hashBytes = $hashAlgorithm.ComputeHash($stream)
    }
    finally {
        $stream.Dispose()
    }
    return ([System.BitConverter]::ToString($hashBytes) -replace '-', '').ToLowerInvariant()
}

function Invoke-NativeInstaller {
    param(
        [string]$FilePath,
        [string]$Arguments
    )
    $startInfo = New-Object System.Diagnostics.ProcessStartInfo
    $startInfo.FileName = $FilePath
    $startInfo.Arguments = $Arguments
    $startInfo.UseShellExecute = $false
    $startInfo.CreateNoWindow = $true
    $process = [System.Diagnostics.Process]::Start($startInfo)
    $process.WaitForExit()
    return $process.ExitCode
}

function Get-OfflineInstallOutcome {
    param([int]$ExitCode)
    switch ($ExitCode) {
        0 { return @{ result = 'installed'; reboot_required = $false } }
        3010 { return @{ result = 'installed'; reboot_required = $true } }
        1641 { return @{ result = 'installed'; reboot_required = $true } }
        2359302 { return @{ result = 'already-installed'; reboot_required = $false } }
        -2145124329 { return @{ result = 'not-applicable'; reboot_required = $false } }
        -2146498530 { return @{ result = 'not-applicable'; reboot_required = $false } }
    }
    return @{ result = 'failed'; reboot_required = $false }
}

function Install-OfflinePackage {
    param(
        [string]$Path,
        [string]$ExpectedSha256
    )
    $fullPath = [System.IO.Path]::GetFullPath($Path)
    $packageName = [System.IO.Path]::GetFileName($fullPath)
    $outcome = @{ name = $packageName; sha256 = ''; signature_status = ''; signer = ''; result = 'rejected'; exit_code = 0; reboot_required = $false }
    if (-not (Test-Path -LiteralPath $fullPath -PathType Leaf)) {
        $outcome['message'] = "The package $fullPath does not exist."
        return $outcome
    }
    $outcome['sha256'] = Get-FileSha256 $fullPath
    if ($ExpectedSha256.Length -gt 0 -and $outcome['sha256'] -ne $ExpectedSha256.ToLowerInvariant()) {
        $outcome['message'] = "The SHA-256 checksum of $packageName does not match the configured value."
        return $outcome
    }
    $signature = Get-AuthenticodeSignature -FilePath $fullPath
    $outcome['signature_status'] = [string]$signature.Status
    if ($null -ne $signature.SignerCertificate) {
        $outcome['signer'] = [string]$signature.SignerCertificate.Subject
    }
    if ($outcome['signature_status'] -ne 'Valid') {
        $outcome['message'] = "The Authenticode signature of $packageName is not valid ($($outcome['signature_status']))."
        return $outcome
    }
    if ($outcome['signer'] -notmatch '(^|,\s*)O=Microsoft Corporation(,|$)') {
        $outcome['message'] = "$packageName is not signed by Microsoft Corporation."
        return $outcome
    }
    $extension = [System.IO.Path]::GetExtension($fullPath).ToLowerInvariant()
    $systemDirectory = Join-Path $env:SystemRoot 'System32'
    if ($extension -eq '.msu') {
        $exitCode = Invoke-NativeInstaller (Join-Path $systemDirectory 'wusa.exe') ('"{0}" /quiet /norestart' -f $fullPath)
    }
    elseif ($extension -eq '.cab') {
        $exitCode = Invoke-NativeInstaller (Join-Path $systemDirectory 'Dism.exe') ('/Online /Add-Package /PackagePath:"{0}" /Quiet /NoRestart' -f $fullPath)
    }
    else {
        $outcome['message'] = "$packageName is not a .msu or .cab package."
        return $outcome
    }
    $installOutcome = Get-OfflineInstallOutcome $exitCode
    $outcome['exit_code'] = $exitCode
    $outcome['result'] = $installOutcome.result
    $outcome['reboot_required'] = $installOutcome.reboot_required
    return $outcome
}

function Invoke-OfflineInstall {
    param([hashtable]$Request)
    Write-Phase 'offline'
    foreach ($packageDefinition in (Get-RequestList -Request $Request -Name 'offline_package')) {
        $path, $expectedSha256 = $packageDefinition -split '\|', 2
        if ($null -eq $expectedSha256) {
            $expectedSha256 = ''
        }
        Write-LogRecord 'info' "Installing offline package $([System.IO.Path]::GetFileName($path))..."
        Write-Record 'offline-result' (Install-OfflinePackage $path $expectedSha256)
    }
    Write-Record 'pending-reboot' (Get-PendingReboot)
}

try {
    $request = Read-Request $RequestPath
    $action = Get-RequestValue -Request $request -Name 'action'
    switch ($action) {
        'detect' {
            Remove-StaleTask (Get-RequestValue -Request $request -Name 'task_name')
            Write-Record 'system' (Get-SystemCapability)
        }
        'register-microsoft-update' {
            Register-MicrosoftUpdate
        }
        'search' {
            [void](Invoke-Search $request -EmitUpdates)
        }
        'install' {
            Invoke-Install $request
        }
        'offline-install' {
            Invoke-OfflineInstall $request
        }
        'pending-reboot' {
            $servicingTimeoutSeconds = [int](Get-RequestValue -Request $request -Name 'servicing_timeout_seconds' -Default '0')
            $servicingIdle = Wait-ServicingIdle $servicingTimeoutSeconds
            $pendingReboot = Get-PendingReboot
            $pendingReboot['servicing_idle'] = $servicingIdle
            Write-Record 'pending-reboot' $pendingReboot
        }
        default {
            throw "Unknown action '$action'."
        }
    }
    Write-Record 'done' @{ action = $action }
    exit 0
}
catch {
    $errorFields = Get-ErrorDetail $_
    $errorFields['position'] = [string]$_.InvocationInfo.PositionMessage
    Write-Record 'error' $errorFields
    exit 1
}
