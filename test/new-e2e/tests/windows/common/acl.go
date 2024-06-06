// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package common

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"

	"testing"

	"github.com/stretchr/testify/assert"
)

//revive:disable:var-naming These names are intended to match the Windows API names

// SECURITY_DESCRIPTOR_CONTROL flags
//
// https://learn.microsoft.com/en-us/windows/win32/secauthz/security-descriptor-control
const (
	DELETE                   = 0x00010000
	READ_CONTROL             = 0x00020000
	WRITE_DAC                = 0x00040000
	WRITE_OWNER              = 0x00080000
	SYNCHRONIZE              = 0x00100000
	STANDARD_RIGHTS_REQUIRED = 0x000F0000
	STANDARD_RIGHTS_READ     = READ_CONTROL
	STANDARD_RIGHTS_WRITE    = READ_CONTROL
	STANDARD_RIGHTS_EXECUTE  = READ_CONTROL
	STANDARD_RIGHTS_ALL      = 0x001F0000
	SPECIFIC_RIGHTS_ALL      = 0x0000FFFF
	ACCESS_SYSTEM_SECURITY   = 0x01000000
	MAXIMUM_ALLOWED          = 0x02000000
	GENERIC_READ             = 0x80000000
	GENERIC_WRITE            = 0x40000000
	GENERIC_EXECUTE          = 0x20000000
	GENERIC_ALL              = 0x10000000
)

// Filesystem access rights
//
// https://learn.microsoft.com/en-us/windows/win32/wmisdk/file-and-directory-access-rights-constants
// https://learn.microsoft.com/en-us/windows/win32/fileio/file-security-and-access-rights
const (
	FILE_READ_DATA        = 0x00000001
	FILE_READ_ATTRIBUTES  = 0x00000080
	FILE_READ_EA          = 0x00000008
	FILE_WRITE_DATA       = 0x00000002
	FILE_WRITE_ATTRIBUTES = 0x00000100
	FILE_WRITE_EA         = 0x00000010
	FILE_APPEND_DATA      = 0x00000004
	FILE_EXECUTE          = 0x00000020

	FILE_GENERIC_READ    = STANDARD_RIGHTS_READ | FILE_READ_DATA | FILE_READ_ATTRIBUTES | FILE_READ_EA | SYNCHRONIZE
	FILE_GENERIC_WRITE   = STANDARD_RIGHTS_WRITE | FILE_WRITE_DATA | FILE_WRITE_ATTRIBUTES | FILE_WRITE_EA | FILE_APPEND_DATA | SYNCHRONIZE
	FILE_GENERIC_EXECUTE = STANDARD_RIGHTS_EXECUTE | FILE_READ_ATTRIBUTES | FILE_EXECUTE | SYNCHRONIZE

	FILE_LIST_DIRECTORY   = FILE_READ_DATA
	FILE_CREATE_FILES     = FILE_WRITE_DATA
	FILE_ADD_SUBDIRECTORY = FILE_APPEND_DATA
	FILE_TRAVERSE         = FILE_EXECUTE
	FILE_DELETE_CHILD     = 0x00000040
)

// Filesystem access rights
//
// https://learn.microsoft.com/en-us/dotnet/api/system.security.accesscontrol.filesystemrights
const (
	ChangePermissions            = WRITE_DAC
	ReadPermissions              = READ_CONTROL
	TakeOwnership                = WRITE_OWNER
	DeleteSubdirectoriesAndFiles = FILE_DELETE_CHILD

	// FileFullControl = 0x001F01FF
	FileFullControl = SYNCHRONIZE | TakeOwnership | ChangePermissions | ReadPermissions | DELETE | FILE_WRITE_ATTRIBUTES | FILE_READ_ATTRIBUTES | DeleteSubdirectoriesAndFiles | FILE_TRAVERSE | FILE_WRITE_EA | FILE_READ_EA | FILE_ADD_SUBDIRECTORY | FILE_CREATE_FILES | FILE_LIST_DIRECTORY
	// FileRead = 0x00020089
	FileRead = ReadPermissions | FILE_READ_ATTRIBUTES | FILE_READ_EA | FILE_LIST_DIRECTORY
	// ReadAndExecute = // 0x000200A9
	FileReadAndExecute = FileRead | FILE_TRAVERSE
	// Write = 0x00000116
	FileWrite = FILE_WRITE_ATTRIBUTES | FILE_WRITE_EA | FILE_ADD_SUBDIRECTORY | FILE_CREATE_FILES
	// Modify = 0x000301BF
	FileModify = FileWrite | FileReadAndExecute | DELETE
)

// Registry access rights
//
// https://learn.microsoft.com/en-us/windows/win32/sysinfo/registry-key-security-and-access-rights
const (
	KEY_CREATE_LINK        = 0x0020
	KEY_CREATE_SUB_KEY     = 0x0004
	KEY_ENUMERATE_SUB_KEYS = 0x0008
	KEY_EXECUTE            = KEY_READ
	KEY_NOTIFY             = 0x0010
	KEY_QUERY_VALUE        = 0x0001
	KEY_READ               = STANDARD_RIGHTS_READ | KEY_QUERY_VALUE | KEY_ENUMERATE_SUB_KEYS | KEY_NOTIFY
	KEY_SET_VALUE          = 0x0002
	KEY_WRITE              = STANDARD_RIGHTS_WRITE | KEY_SET_VALUE | KEY_CREATE_SUB_KEY
	KEY_ALL_ACCESS         = STANDARD_RIGHTS_REQUIRED | KEY_QUERY_VALUE | KEY_SET_VALUE | KEY_CREATE_SUB_KEY | KEY_ENUMERATE_SUB_KEYS | KEY_NOTIFY | KEY_CREATE_LINK
)

// Registry access rights
//
// https://learn.microsoft.com/en-us/dotnet/api/system.security.accesscontrol.registryrights
const (
	// RegistryFullControl = 0xF003F
	RegistryFullControl = TakeOwnership | ChangePermissions | ReadPermissions | DELETE | KEY_CREATE_LINK | KEY_NOTIFY | KEY_ENUMERATE_SUB_KEYS | KEY_CREATE_SUB_KEY | KEY_SET_VALUE | KEY_QUERY_VALUE
)

// Inheritance flags
//
// https://learn.microsoft.com/en-us/dotnet/api/system.security.accesscontrol.inheritanceflags
const (
	InheritanceFlagsNone      = 0
	InheritanceFlagsContainer = 1
	InheritanceFlagsObject    = 2
)

// Propagation flags
//
// https://learn.microsoft.com/en-us/dotnet/api/system.security.accesscontrol.propagationflags
const (
	PropagationFlagsNone        = 0
	PropagationFlagsInherit     = 1
	PropagationFlagsNoPropagate = 2
)

// Access control types
//
// https://learn.microsoft.com/en-us/dotnet/api/system.security.accesscontrol.accesscontroltype
const (
	AccessControlTypeAllow = 0
	AccessControlTypeDeny  = 1
)

//revive:enable:var-naming

// AuthorizationRuleWithRights is an interface for an authorization rule with rights
type AuthorizationRuleWithRights interface {
	GetAuthorizationRule() AuthorizationRule
	GetRights() int
}

// AuthorizationRule represents the identity and inheritance flags for a Windows ACE
//
// https://learn.microsoft.com/en-us/dotnet/api/system.security.accesscontrol.authorizationrule
type AuthorizationRule struct {
	Identity         Identity
	InheritanceFlags int
	PropagationFlags int
	IsInherited      bool
}

// AccessRule represents a Windows access rule ACE
//
// https://learn.microsoft.com/en-us/dotnet/api/system.security.accesscontrol.accessrule
type AccessRule struct {
	AuthorizationRule
	Rights            int
	AccessControlType int
}

// AuditRule represents Windows audit rule ACE
//
// https://learn.microsoft.com/en-us/dotnet/api/system.security.accesscontrol.auditrule
type AuditRule struct {
	AuthorizationRule
	Rights     int
	AuditFlags int
}

// ObjectSecurity represents the security information for a Windows Object (e.g. file, directory, registry key)
//
// https://learn.microsoft.com/en-us/dotnet/api/system.security.accesscontrol.nativeobjectsecurity
type ObjectSecurity struct {
	Owner                   Identity
	Group                   Identity
	Access                  []AccessRule
	Audit                   []AuditRule
	SDDL                    string `json:"Sddl"`
	AreAccessRulesProtected bool
	AreAuditRulesProtected  bool
}

// GetAuthorizationRule returns the authorization rule, used to satisfy interfces when embedding in other structs
func (r AuthorizationRule) GetAuthorizationRule() AuthorizationRule {
	return r
}

// Equal returns true if the rules are equal.
//
// See Identity.Equal for more information on how it is compared.
func (r AuthorizationRule) Equal(other AuthorizationRule) bool {
	return r.InheritanceFlags == other.InheritanceFlags &&
		r.PropagationFlags == other.PropagationFlags &&
		r.IsInherited == other.IsInherited &&
		r.Identity.Equal(other.Identity)
}

// Equal returns true if the rules are equal.
func (r AuditRule) Equal(other AuditRule) bool {
	return r.Rights == other.Rights &&
		r.AuditFlags == other.AuditFlags &&
		r.AuthorizationRule.Equal(other.AuthorizationRule)
}

// IsSuccess returns true if the audit rule is a success rule
func (r AuditRule) IsSuccess() bool {
	return r.AuditFlags == 0
}

// IsFailure returns true if the audit rule is a failure rule
func (r AuditRule) IsFailure() bool {
	return r.AuditFlags == 1
}

// GetRights returns the rights for the audit rule
func (r AuditRule) GetRights() int {
	return r.Rights
}

// Equal returns true if the rules are equal.
func (r AccessRule) Equal(other AccessRule) bool {
	return r.Rights == other.Rights &&
		r.AccessControlType == other.AccessControlType &&
		r.AuthorizationRule.Equal(other.AuthorizationRule)
}

// IsAllow returns true if the access rule is an allow rule
func (r AccessRule) IsAllow() bool {
	return r.AccessControlType == AccessControlTypeAllow
}

// IsDeny returns true if the access rule is a deny rule
func (r AccessRule) IsDeny() bool {
	return r.AccessControlType == AccessControlTypeDeny
}

// GetRights returns the rights for the access rule
func (r AccessRule) GetRights() int {
	return r.Rights
}

// NewExplicitAccessRule creates a new explicit AccessRule
//
// Flags default to no inheritance, no no propagation
func NewExplicitAccessRule(identity Identity, rights int, accessControlType int) AccessRule {
	return NewExplicitAccessRuleWithFlags(identity, rights, accessControlType, InheritanceFlagsNone, PropagationFlagsNone)
}

// NewInheritedAccessRule creates a new inherited AccessRule
func NewInheritedAccessRule(identity Identity, rights int, accessControlType int) AccessRule {
	return NewInheritedAccessRuleWithFlags(identity, rights, accessControlType, InheritanceFlagsNone, PropagationFlagsNone)
}

// NewExplicitAccessRuleWithFlags creates a new AccessRule with the given flags
func NewExplicitAccessRuleWithFlags(identity Identity, rights int, accessControlType int, inheritanceFlags int, propagationFlags int) AccessRule {
	return newAccessRuleWithFlags(identity, rights, accessControlType, inheritanceFlags, propagationFlags, false)
}

// NewInheritedAccessRuleWithFlags creates a new inherited AccessRule with the given flags
func NewInheritedAccessRuleWithFlags(identity Identity, rights int, accessControlType int, inheritanceFlags int, propagationFlags int) AccessRule {
	return newAccessRuleWithFlags(identity, rights, accessControlType, inheritanceFlags, propagationFlags, true)
}

func newAccessRuleWithFlags(identity Identity, rights int, accessControlType int, inheritanceFlags int, propagationFlags int, isInherited bool) AccessRule {
	return AccessRule{
		AuthorizationRule: AuthorizationRule{
			Identity:         identity,
			InheritanceFlags: inheritanceFlags,
			PropagationFlags: propagationFlags,
			IsInherited:      isInherited,
		},
		Rights:            rights,
		AccessControlType: accessControlType,
	}
}

//go:embed fixtures/get-acl-helpers.ps1
var aclHelpersPs1Fixture []byte
var aclHelpersPath = `C:\Windows\Temp\acl_helpers.ps1`

func placeACLHelpers(host *components.RemoteHost) error {
	if exists, _ := host.FileExists(aclHelpersPath); exists {
		return nil
	}
	_, err := host.WriteFile(aclHelpersPath, aclHelpersPs1Fixture)
	return err
}

// GetSecurityInfoForPath returns the security information for the given path using Get-ACL
//   - Example file path: C:\Windows\Temp\file.txt
//   - Example registry path: HKLM:\SOFTWARE\Datadog
//
// https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.security/get-acl
func GetSecurityInfoForPath(host *components.RemoteHost, path string) (ObjectSecurity, error) {
	var s ObjectSecurity

	err := placeACLHelpers(host)
	if err != nil {
		return s, err
	}

	// Get the ACL information
	cmd := fmt.Sprintf(`. %s; Get-Acl -Audit -Path '%s' | ConvertTo-ACLDTO`, aclHelpersPath, path)
	output, err := host.Execute(cmd)
	if err != nil {
		return s, err
	}

	err = json.Unmarshal([]byte(output), &s)
	if err != nil {
		return s, fmt.Errorf("failed to unmarshal ACL information: %w\n%s", err, output)
	}

	return s, nil
}

// NewProtectedSecurityInfo creates a new ObjectSecurity with protected access rules (i.e. inheritance is disabled)
func NewProtectedSecurityInfo(owner Identity, group Identity, access []AccessRule) ObjectSecurity {
	return ObjectSecurity{
		Owner:                   owner,
		Group:                   group,
		Access:                  access,
		AreAccessRulesProtected: true,
	}
}

// NewInheritSecurityInfo creates a new ObjectSecurity that can inherit access rules
func NewInheritSecurityInfo(owner Identity, group Identity, access []AccessRule) ObjectSecurity {
	return ObjectSecurity{
		Owner:                   owner,
		Group:                   group,
		Access:                  access,
		AreAccessRulesProtected: false,
	}
}

// ContainsRuleForIdentity returns true if the list contains a rule for the given identity
func ContainsRuleForIdentity[T AuthorizationRuleWithRights](acl []T, identity Identity) bool {
	return slices.ContainsFunc(acl, func(rule T) bool {
		return rule.GetAuthorizationRule().Identity.Equal(identity)
	})
}

// AssertEqualAccessSecurity asserts that the access control settings for the expected and actual are equal.
//
// Compares the owner, group, and access rules. Note that the order of the access rules is relevant when
// multiple rules apply to the same identity, but since this is not relevant for our use cases, we do not
// enforce the order of the rules in this function.
func AssertEqualAccessSecurity(t *testing.T, path string, expected, actual ObjectSecurity) {
	t.Helper()

	AssertEqualableElementsMatch(t, expected.Access, actual.Access, "%s access rules should match", path)
	assert.Equal(t, expected.Owner, actual.Owner, "%s owner should match", path)
	assert.Equal(t, expected.Group, actual.Group, "%s group should match", path)
	assert.Equal(t, expected.AreAccessRulesProtected, actual.AreAccessRulesProtected, "%s access rules protection should match", path)
}
