param(
    [Parameter(Mandatory=$true)][string[]] $EventLogs
)

function Set-RegAccessFor {
    Param(
        [Parameter(Mandatory=$true)][System.Security.Principal.SecurityIdentifier]$ddsid,
        [Parameter(Mandatory=$true)][string]$logName
    )

    $reg_acl = Get-Acl HKLM:\System\CurrentControlSet\Services\EventLog\$logName
    $accessrule = New-Object System.Security.AccessControl.RegistryAccessRule($ddsid,"ReadKey", "Allow")
    $reg_acl.SetAccessRule($accessrule)
    $reg_acl | Set-Acl HKLM:\System\CurrentControlSet\Services\EventLog\$logName
}

function Set-ChannelAccessFor {
	
    Param(
	[Parameter(Mandatory=$true)][string]$sidstring,
        [Parameter(Mandatory=$true)][string]$logName
    )

    [string]$channel_access = & wevtutil gl $logName | Select-String "channelAccess:"
    $old_sddl = ($channel_access -split ':',2)[1].Trim()
    $new_sddl = "$old_sddl(A;;0x7;;;$sidstring)"
    & wevtutil sl security /ca:$new_sddl
}

[string]$service_sid_out = & sc.exe showsid datadogagent | Select-String "SERVICE SID"
$service_sid = $service_sid_out.split(":")[1].Trim()

[System.Security.Principal.SecurityIdentifier]$ddsid = $service_sid

foreach ($log in $EventLogs) {
    Set-RegAccessFor -ddsid $ddsid -logName $log
    Set-ChannelAccessFor -sidstring $service_sid -logName $log
}
Write-Host -ForegroundColor Green "Done setting permissions"