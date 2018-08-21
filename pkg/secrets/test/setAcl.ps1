$file = $args[0]
$acl = Get-Acl $file

# remove right inherited 
$acl.SetAccessRuleProtection($true,$true)
$acl | Set-Acl

# removing unwanted rights
$acl = Get-Acl $file
$acl.Access | where { ($_.IdentityReference -ne 'NT AUTHORITY\SYSTEM') -and ($_.IdentityReference -ne 'BUILTIN\Administrators')} | ForEach-Object {
    $acl.RemoveAccessRule($_);
}

# adding ACL for datadog_secretuser
$ddAcl = New-Object  system.security.accesscontrol.filesystemaccessrule("datadog_secretuser","FullControl","Allow")
$acl.SetAccessRule($ddAcl)
$acl | Set-Acl