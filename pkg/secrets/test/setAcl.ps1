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
(Get-Item $file).SetAccessControl($acl)


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

# adding ACL for ddagentuser
if ($addDDUser -eq $True) {
    $ddSid = New-Object Security.Principal.SecurityIdentifier("S-1-5-80-1780442038-2564740535-2014067642-3562800676-515077229")
    $ddAcl = New-Object  system.security.accesscontrol.filesystemaccessrule($ddSid, "FullControl","Allow")
    $acl.SetAccessRule($ddAcl)

    $traceSid = New-Object Security.Principal.SecurityIdentifier("S-1-5-80-3626218227-2896763321-2052920590-1920844846-327269072")
    $traceAcl = New-Object  system.security.accesscontrol.filesystemaccessrule($traceSid,"FullControl","Allow")
    $acl.SetAccessRule($traceAcl)
}
(Get-Item $file).SetAccessControl($acl)
