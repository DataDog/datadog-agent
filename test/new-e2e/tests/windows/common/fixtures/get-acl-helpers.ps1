# Function to translate identity to SID
function Format-IdentityAsSID($identity) {
    if ($identity -is [System.Security.Principal.SecurityIdentifier]) {
        return $identity.Value
    }
    try {
        # Try it directly, first
        if ($identity -is [System.Security.Principal.NTAccount]) {
            return $identity.Translate([System.Security.Principal.SecurityIdentifier]).Value
        }
        if ($identity -is [String]) {
            return (New-Object System.Security.Principal.NTAccount($identity)).Translate([System.Security.Principal.SecurityIdentifier]).Value
        }
    } catch {
        # Some names (e.g. APPLICATION PACKAGE AUTHORITY\\ALL RESTRICTED APPLICATION PACKAGES) only work if the latter half is provided
        $name = $identity
        if ($name -is [System.Security.Principal.NTAccount]) {
            $name = $identity.Value
        }
        $name = $name.Split('\')[-1]
        return (New-Object System.Security.Principal.NTAccount($name)).Translate([System.Security.Principal.SecurityIdentifier]).Value
        # We must either return a value or throw an error, all accounts/identities have SIDs
    }
}

# Function to translate identity to name
function Format-IdentityAsName($identity) {
    if ($identity -is [String]) {
        return $identity
    }
    if ($identity -is [System.Security.Principal.NTAccount]) {
        return $identity.Value
    }
    try {
        return $identity.Translate([System.Security.Principal.NTAccount]).Value
    } catch {
        # Don't throw an error if we can't resolve the name, some SIDs don't have names
        return ""
    }
}

# Retrieves the ACL for $path and returns it as a JSON object that can be unmarshalled by the test
function ConvertTo-ACLDTO {
    # process block to support pipeline input
    process {
        $aclObject = $_
        # Create a new object to store modified ACL
        $newAclObject = @{
            Owner = @{
                Name = Format-IdentityAsName($aclObject.Owner)
                SID = Format-IdentityAsSID($aclObject.Owner)
            }
            Group = @{
                Name = Format-IdentityAsName($aclObject.Group)
                SID = Format-IdentityAsSID($aclObject.Group)
            }
            Access = @()
            Audit = @()
            Sddl = $aclObject.Sddl
            AreAccessRulesProtected = $aclObject.AreAccessRulesProtected
            AreAuditRulesProtected = $aclObject.AreAuditRulesProtected
        }

        # Modify Access IdentityReferences and add to new ACL object
        foreach ($access in $aclObject.Access) {
            $accessSid = Format-IdentityAsSID($access.IdentityReference)
            $accessName = Format-IdentityAsName($access.IdentityReference)
            $modifiedAccess = @{
                Rights = $access.FileSystemRights
                AccessControlType = $access.AccessControlType
                IdentityReference = @{
                    Name = $accessName
                    SID = $accessSid
                }
                IsInherited = $access.IsInherited
                InheritanceFlags = $access.InheritanceFlags
                PropagationFlags = $access.PropagationFlags
            }
            $newAclObject.Access += $modifiedAccess
        }

        # Modify Audit IdentityReferences and add to new ACL object
        foreach ($audit in $aclObject.Audit) {
            $auditSid = Format-IdentityAsSID($audit.IdentityReference.Value)
            $auditName = Format-IdentityAsName($audit.IdentityReference.Value)
            $modifiedAccess = @{
                Rights = $audit.FileSystemRights
                AuditFlags = $audit.AuditFlags
                IdentityReference = @{
                    Name = $auditName
                    SID = $auditSid
                }
                IsInherited = $audit.IsInherited
                InheritanceFlags = $audit.InheritanceFlags
                PropagationFlags = $audit.PropagationFlags
            }
            $newAclObject.Audit += $modifiedAccess
        }

        # Convert new ACL object to JSON
        $newAclJson = $newAclObject | ConvertTo-Json -Depth 5

        # Output modified JSON
        Write-Output $newAclJson
    }
}
