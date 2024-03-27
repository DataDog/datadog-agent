param (
    [string]$FilePath,
    [string]$FileContent
)

echo Filepath = $FilePath
echo FileContnet = $FileContent

$user = "ddagentuser"
$permissions = "Read", "ReadAndExecute"


# Create the file and get its permissions
$FileContent | Set-Content -Path $FilePath
$acl = Get-Acl -Path $FilePath

# Disable inheritance and remove all existing access rules
$acl.SetAccessRuleProtection($true, $false)
$acl.Access | ForEach-Object {
    $acl.RemoveAccessRule($_)
}

# Add the desired access rule for the specific user
foreach ($permission in $permissions) {
    $rule = New-Object System.Security.AccessControl.FileSystemAccessRule($user, $permission, "Allow")
    $acl.AddAccessRule($rule)
}

# Set the modified ACL on the file
Set-Acl -Path $FilePath -AclObject $acl