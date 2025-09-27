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
    } elseif ($rule.PipeAccessRights) {
        $rights = $rule.PipeAccessRights
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

function Get-PipeSecurity($pipename) {
    # split the pipe name to get the server name
    # https://learn.microsoft.com/en-us/windows/win32/ipc/pipe-names
    $parts = $pipename -split "\\"
    if ($parts.Length -eq 1) {
        # pipename
        $pipename = $parts[0]
        $server = "."
    } elseif ($parts.Length -eq 5) {
        # \\.\pipe\pipename
        $server = $parts[2]
        $pipename = $parts[4]
    } else {
        throw "Invalid pipe name: $pipename"
    }
    # have to connect to pipe to get security info
    $pipe = New-Object System.IO.Pipes.NamedPipeClientStream($server, $pipename, [System.IO.Pipes.PipeDirection]::In)
    $pipe.Connect()
    try {
        if (Get-Member -InputObject $pipe -Name "GetAccessControl" -Membertype Methods) {
            # This method works on PS5
            return $pipe.GetAccessControl()
        } else {
            # https://github.com/PowerShell/PowerShell/issues/23962
            # PS7/.NET moved security related methods into extensions, which must be called directly
            # in PowerShell
            $ac = [System.IO.Pipes.PipesAclExtensions]::GetAccessControl($pipe)
            # Unfortunately the extension doesn't have the properties, so fetch them ourselves
            return @{
                Owner = $ac.GetOwner([System.Security.Principal.SecurityIdentifier])
                Group = $ac.GetGroup([System.Security.Principal.SecurityIdentifier])
                Access = $ac.GetAccessRules($true, $true, [System.Security.Principal.SecurityIdentifier])
                Audit = $ac.GetAuditRules($true, $true, [System.Security.Principal.SecurityIdentifier])
                AreAccessRulesProtected = $ac.AreAccessRulesProtected
                AreAuditRulesProtected = $ac.AreAuditRulesProtected
                Sddl = $ac.GetSecurityDescriptorSddlForm([System.Security.AccessControl.AccessControlSections]::All)
            }
        }
    } finally {
        $pipe.Close()
    }
}

function GetServiceSDDL($serviceName) {
    Write-Output ((sc.exe sdshow $serviceName) -join "").Trim()
}

# Function to check if a file or directory is world-writable
# World-writable means accessible by Everyone (S-1-1-0), Users (S-1-5-32-545), or Authenticated Users (S-1-5-11) groups
function Test-IsWorldWritable($path) {
    try {
        $acl = Get-Acl -Path $path -ErrorAction Stop

        # Check each access rule
        foreach ($rule in $acl.Access) {
            # Only check Allow rules
            if ($rule.AccessControlType -ne "Allow") {
                continue
            }

            # Get the SID for the identity
            $sid = ""
            try {
                if ($rule.IdentityReference -is [System.Security.Principal.SecurityIdentifier]) {
                    $sid = $rule.IdentityReference.Value
                } else {
                    $sid = $rule.IdentityReference.Translate([System.Security.Principal.SecurityIdentifier]).Value
                }
            } catch {
                # If we can't translate, skip this rule
                continue
            }

            # Check if this is Everyone, Users, or Authenticated Users group
            if ($sid -eq "S-1-1-0" -or $sid -eq "S-1-5-32-545" -or $sid -eq "S-1-5-11") {
                # Check if the rule grants write permissions
                $writeRights = @()

                # For file system rights
                if ($rule.FileSystemRights) {
                    $rights = [int]$rule.FileSystemRights
                    # Check for various write permissions
                    # FILE_WRITE_DATA (0x2), FILE_APPEND_DATA (0x4), FILE_WRITE_ATTRIBUTES (0x100), FILE_WRITE_EA (0x10)
                    # DELETE (0x10000), WRITE_DAC (0x40000), WRITE_OWNER (0x80000)
                    # FileWrite (0x116), FileModify (0x301BF), FileFullControl (0x1F01FF)
                    if (($rights -band 0x2) -or      # FILE_WRITE_DATA
                        ($rights -band 0x4) -or      # FILE_APPEND_DATA
                        ($rights -band 0x10) -or     # FILE_WRITE_EA
                        ($rights -band 0x100) -or    # FILE_WRITE_ATTRIBUTES
                        ($rights -band 0x10000) -or  # DELETE
                        ($rights -band 0x40000) -or  # WRITE_DAC
                        ($rights -band 0x80000)) {   # WRITE_OWNER
                        return $true
                    }
                }
            }
        }

        return $false
    } catch {
        # If we can't read the ACL, assume it's not world-writable
        return $false
    }
}

# Function to check a file or directory for world-writable files
# If input is a file, checks only that file
# If input is a directory, recursively checks all files and subdirectories
function Find-WorldWritableFiles($rootPath) {
    $worldWritableFiles = @()

    try {
        # Check if the path exists
        if (-not (Test-Path -Path $rootPath)) {
            return $worldWritableFiles
        }

        # Get the item to determine if it's a file or directory
        $item = Get-Item -Path $rootPath -Force -ErrorAction Stop

        if ($item.PSIsContainer) {
            # It's a directory - check the directory itself and all contents
            if (Test-IsWorldWritable -path $rootPath) {
                $worldWritableFiles += $rootPath
            }

            # Recursively check all files and subdirectories
            Get-ChildItem -Path $rootPath -Recurse -Force -ErrorAction SilentlyContinue | ForEach-Object {
                if (Test-IsWorldWritable -path $_.FullName) {
                    $worldWritableFiles += $_.FullName
                }
            }
        } else {
            # It's a file - check only this file
            if (Test-IsWorldWritable -path $rootPath) {
                $worldWritableFiles += $rootPath
            }
        }
    } catch {
        # If we can't access the path, return what we have so far
    }

    return $worldWritableFiles
}

# Function to check multiple files or directories and return results as a list
function Find-WorldWritableFilesInPaths($paths) {
    $results = @()

    foreach ($path in $paths) {
        if (Test-Path -Path $path) {
            $worldWritableFiles = Find-WorldWritableFiles -rootPath $path
            $results += $worldWritableFiles
        }
    }

    return $results
}
