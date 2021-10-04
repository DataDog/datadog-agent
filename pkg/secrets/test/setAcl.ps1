param(
    [Parameter(Mandatory=$True)]
    [string]$file,

    [Parameter(Mandatory=$True)]
    [bool]$removeAllUser = $False,

    [Parameter(Mandatory=$True)]
    [bool]$removeAdmin = $False,

    [Parameter(Mandatory=$True)]
    [bool]$removeLocalSystem = $False,

    [Parameter(Mandatory=$True)]
    [bool]$addDDUser = $False
)

# remove right inherited
$acl = Get-Acl $file
$acl.SetAccessRuleProtection($true,$true)
$acl | Set-Acl


$acl = Get-Acl $file

if ($removeAllUser -eq $True) {
    $acl.Access | Where-Object { ($_.IdentityReference -ne 'NT AUTHORITY\SYSTEM') -and ($_.IdentityReference -ne 'BUILTIN\Administrators')} | ForEach-Object {
        $acl.RemoveAccessRule($_);
    }
}

if ($removeAdmin -eq $True) {
    $acl.Access | Where-Object { ($_.IdentityReference -eq 'BUILTIN\Administrators') } | ForEach-Object {
        $acl.RemoveAccessRule($_);
    }
}

if ($removeLocalSystem -eq $True) {
    $acl.Access | Where-Object { ($_.IdentityReference -eq 'NT AUTHORITY\SYSTEM') } | ForEach-Object {
        $acl.RemoveAccessRule($_);
    }
}

# adding ACL for current user
if ($addDDUser -eq $True) {
    $ddCurrentUser = [System.Security.Principal.WindowsIdentity]::GetCurrent().Name
    $ddAcl = New-Object  system.security.accesscontrol.filesystemaccessrule($ddCurrentUser, "FullControl","Allow")
    $acl.SetAccessRule($ddAcl)
}

$acl | Set-Acl
