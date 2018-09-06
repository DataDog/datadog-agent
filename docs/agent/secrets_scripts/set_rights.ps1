$file = $args[0]
$acl = Get-Acl $file

"Removing right inheritage"
$acl.SetAccessRuleProtection($true,$true)
$acl | Set-Acl

"Removing every rights outside Administrator, SYSTEM"
$acl = Get-Acl $file
$acl.Access | where { ($_.IdentityReference -ne 'NT AUTHORITY\SYSTEM') -and ($_.IdentityReference -ne 'BUILTIN\Administrators')} | ForEach-Object {
    $acl.RemoveAccessRule($_);
}

"Giving rights to datadog_secretuser"
$ddAcl = New-Object  system.security.accesscontrol.filesystemaccessrule("datadog_secretuser","FullControl","Allow")
$acl.SetAccessRule($ddAcl)

$acl | Set-Acl
