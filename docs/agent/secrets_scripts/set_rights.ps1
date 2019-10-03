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

"Giving rights to datadogagent"
$ddAcl = New-Object  system.security.accesscontrol.filesystemaccessrule("NT Service\datadogagent","FullControl","Allow")
$acl.SetAccessRule($ddAcl)

"Giving rights to datadog-trace-agent"
$ddAcl = New-Object  system.security.accesscontrol.filesystemaccessrule("NT Service\datadog-trace-agent","FullControl","Allow")
$acl.SetAccessRule($ddAcl)

$acl | Set-Acl
