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

function Get-RuleRights($rule) {
    $rights = 0
    if ($rule.FileSystemRights) {
        $rights = $rule.FileSystemRights
    } elseif ($rule.RegistryRights) {
        $rights = $rule.RegistryRights
    } else {
        throw "Could not determine rights for rule: $rule"
    }
    # Make sure to cast to int to get the real value from the Enum
    # https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.core/about/about_enum
    $rights = [int]$rights
    if ($rights -lt 0) {
        $rights = [uint32]::MaxValue + 1 + $rights
    }
    return [uint32]($rights -band 0xffffffffL)
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
            SDDL = $aclObject.Sddl
            AreAccessRulesProtected = $aclObject.AreAccessRulesProtected
            AreAuditRulesProtected = $aclObject.AreAuditRulesProtected
        }

        # Modify Access IdentityReferences and add to new ACL object
        foreach ($access in $aclObject.Access) {
            $modifiedRule = @{
                Rights = Get-RuleRights($access)
                AccessControlType = $access.AccessControlType
                Identity = @{
                    Name = Format-IdentityAsName($access.IdentityReference)
                    SID = Format-IdentityAsSID($access.IdentityReference)
                }
                IsInherited = $access.IsInherited
                InheritanceFlags = $access.InheritanceFlags
                PropagationFlags = $access.PropagationFlags
            }
            $newAclObject.Access += $modifiedRule
        }

        # Modify Audit IdentityReferences and add to new ACL object
        foreach ($audit in $aclObject.Audit) {
            $modifiedRule = @{
                Rights = Get-RuleRights($audit)
                AuditFlags = $audit.AuditFlags
                Identity = @{
                    Name = Format-IdentityAsName($audit.IdentityReference)
                    SID = Format-IdentityAsSID($audit.IdentityReference)
                }
                IsInherited = $audit.IsInherited
                InheritanceFlags = $audit.InheritanceFlags
                PropagationFlags = $audit.PropagationFlags
            }
            $newAclObject.Audit += $modifiedRule
        }

        # Convert new ACL object to JSON
        $newAclJson = $newAclObject | ConvertTo-Json -Depth 5

        # Output modified JSON
        Write-Output $newAclJson
    }
}

function ConvertTo-ServiceSecurityDTO {
    # process block to support pipeline input
    process {
        $sddl = $_
        $security = (ConvertFrom-SDDLString -Sddl $sddl).RawDescriptor
        $newObject = @{
            Access = @()
            Audit = @()
            SDDL = $sddl
        }

        # Modify Access IdentityReferences and add to new ACL object
        foreach ($access in $security.DiscretionaryAcl) {
            $modifiedRule = @{
                Rights = $access.AccessMask
                AccessControlType = $access.AceType
                Identity = @{
                    Name = Format-IdentityAsName($access.SecurityIdentifier)
                    SID = Format-IdentityAsSID($access.SecurityIdentifier)
                }
            }
            $newObject.Access += $modifiedRule
        }

        # Modify Audit IdentityReferences and add to new ACL object
        foreach ($audit in $security.SystemAcl) {
            $modifiedRule = @{
                Rights = $audit.AccessMask
                AuditFlags = $audit.AuditFlags
                Identity = @{
                    Name = Format-IdentityAsName($audit.SecurityIdentifier)
                    SID = Format-IdentityAsSID($audit.SecurityIdentifier)
                }
            }
            $newObject.Audit += $modifiedRule
        }

        # Convert new ACL object to JSON
        $newAclJson = $newObject | ConvertTo-Json -Depth 5

        # Output modified JSON
        Write-Output $newAclJson
    }
}

function GetServiceSDDL($serviceName) {
    Write-Output ((sc.exe sdshow $serviceName) -join "").Trim()
}