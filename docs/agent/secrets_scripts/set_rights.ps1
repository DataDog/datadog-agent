$file = $args[0]
$acl = Get-Acl $file

"Removing right inheritance"
$acl.SetAccessRuleProtection($true,$true)
$acl | Set-Acl

"Removing every rights outside Administrator, SYSTEM"
$acl = Get-Acl $file
$acl.Access | where { ($_.IdentityReference -ne 'NT AUTHORITY\SYSTEM') -and ($_.IdentityReference -ne 'BUILTIN\Administrators')} | ForEach-Object {
    $acl.RemoveAccessRule($_);
}

"Giving rights to ddagentuser"
$ddAcl = New-Object  system.security.accesscontrol.filesystemaccessrule("ddagentuser","FullControl","Allow")
$acl.SetAccessRule($ddAcl)

$acl | Set-Acl
