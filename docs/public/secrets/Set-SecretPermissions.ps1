<#
.SYNOPSIS

Sets the correct permissions on a file to be used with the Datadog Secret feature.

.DESCRIPTION

The script will use the `ddagentuser` stored in the registry, create a new `FileSecurity` object and:
- Set the Builtin\Administrators as the owner
- Set the Builtin\Administrators as the group
- Grant full access to LOCAL_SYSTEM
- Grant full access to the Builtin\Administrators
- Grant read access to the `ddagentuser`

It's a good idea to make a backup of the secrets executable before running this command.

.PARAMETER SecretBinaryPath

File path of the binary to update.

.INPUTS

None

.OUTPUTS

The `ddagentuser` SID, the new ACLs on the secret binary and whether or not the `secret` command considers the file permissions as valid.

.EXAMPLE

PS> .\Set-SecretPermissions -SecretBinaryPath C:\Example\Datadog\secrets\decrypt_secrets.exe
#>


[CmdletBinding(SupportsShouldProcess=$true)]
[CmdletBinding(DefaultParameterSetName='SecretBinaryPath')]
param(
    [Parameter(Mandatory=$true, ParameterSetName='SecretBinaryPath')]
    [string]$SecretBinaryPath = $null
)

$ddagentUserDomain = Get-ItemPropertyValue -Path 'HKLM:\SOFTWARE\Datadog\Datadog Agent' -Name 'installedDomain'
$ddagentUser = Get-ItemPropertyValue -Path 'HKLM:\SOFTWARE\Datadog\Datadog Agent' -Name 'installedUser'
$fullDdagentUserName = ("{0}\{1}" -f $ddagentUserDomain, $ddagentUser)
$ddagentUserSid = New-Object System.Security.Principal.SecurityIdentifier((New-Object System.Security.Principal.NTAccount($fullDdagentUserName)).Translate([System.Security.Principal.SecurityIdentifier]).Value)
Write-Host ("ddagentuser SID: {0}" -f $ddagentUserSid)
$builtInAdminSid = New-Object System.Security.Principal.SecurityIdentifier([System.Security.Principal.WellKnownSidType]::BuiltinAdministratorsSid, $null)
$localSystemSid = New-Object System.Security.Principal.SecurityIdentifier([System.Security.Principal.WellKnownSidType]::LocalSystemSid, $null)
$fileSecurity = New-Object System.Security.AccessControl.FileSecurity
$fileSecurity.SetAccessRuleProtection($true, $false)
$fileSecurity.SetOwner($builtInAdminSid)
$fileSecurity.SetGroup($builtInAdminSid)
$fileSecurity.AddAccessRule((New-Object System.Security.AccessControl.FileSystemAccessRule -ArgumentList ($ddagentUserSid, ([System.Security.AccessControl.FileSystemRights]::Read -bor [System.Security.AccessControl.FileSystemRights]::ExecuteFile), [System.Security.AccessControl.AccessControlType]::Allow)))
$fileSecurity.AddAccessRule((New-Object System.Security.AccessControl.FileSystemAccessRule -ArgumentList ($builtInAdminSid, [System.Security.AccessControl.FileSystemRights]::FullControl, [System.Security.AccessControl.AccessControlType]::Allow)))
$fileSecurity.AddAccessRule((New-Object System.Security.AccessControl.FileSystemAccessRule -ArgumentList ($localSystemSid, [System.Security.AccessControl.FileSystemRights]::FullControl, [System.Security.AccessControl.AccessControlType]::Allow)))
if ($pscmdlet.ShouldProcess($SecretBinaryPath, "SetAccessControl")) {
    [System.IO.File]::SetAccessControl($SecretBinaryPath, $fileSecurity)
}
try {
    $agentBinary = (Get-ItemPropertyValue -Path 'HKLM:\SOFTWARE\Datadog\Datadog Agent' -Name 'InstallPath') + "\bin\agent.exe"
    & $agentBinary secret
}
catch {
    icacls.exe $SecretBinaryPath
}
