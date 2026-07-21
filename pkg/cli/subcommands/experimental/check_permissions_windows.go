// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package experimental

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

// checkFilePermissions checks that datadog.yaml has secure Windows permissions.
//
// Two checks are performed, mirroring the approach used elsewhere in the codebase:
//
//  1. Owner check (mirrors checkOwner in pkg/util/filesystem/permission_windows.go,
//     PR #46678): the file must be owned by SYSTEM, Administrators, or ddagentuser.
//     A file owned by any other principal is suspicious — the config likely ended
//     up with incorrect ownership.
//
//  2. DACL check (mirrors CheckRights in pkg/util/filesystem/rights_windows.go,
//     adapted for a config file rather than a secret backend executable):
//     - Any ACCESS_ALLOWED ACE for a principal outside {SYSTEM, Administrators,
//     ddagentuser} means a potentially untrusted user can read the API key.
//     - Any ACCESS_DENIED ACE that blocks SYSTEM, Administrators, or ddagentuser
//     would prevent the Agent from reading its own config.
//
// Unlike CheckRights, ddagentuser is NOT required to be explicitly present in the
// DACL — an Admins-only config is equally valid for a config file (whereas for a
// secret backend executable the agent user must be explicitly allowed to execute it).
func checkFilePermissions(path string) (bool, error) {
	// --- Build the allowed SID set ---
	//
	// SIDs from AllocateAndInitializeSid are C-allocated and must be freed.
	// SIDs from winutil.GetDDAgentUserSID (which uses windows.LookupSID
	// internally) are Go-managed and do not need FreeSid.

	systemSID, err := winutil.GetLocalSystemSID()
	if err != nil {
		return false, fmt.Errorf("cannot get SYSTEM SID: %w", err)
	}
	defer windows.FreeSid(systemSID)

	adminsSID, err := getAdministratorsSID()
	if err != nil {
		return false, fmt.Errorf("cannot get Administrators SID: %w", err)
	}
	defer windows.FreeSid(adminsSID)

	// ddagentuser is best-effort: if the Agent service is not registered (e.g.
	// on a development machine or non-standard install) we proceed without it.
	// In that case a legitimately fine config may trigger a spurious warning,
	// but that is preferable to skipping the check entirely.
	ddagentSID, _ := winutil.GetDDAgentUserSID()

	isAllowed := func(sid *windows.SID) bool {
		if windows.EqualSid(sid, systemSID) || windows.EqualSid(sid, adminsSID) {
			return true
		}
		return ddagentSID != nil && windows.EqualSid(sid, ddagentSID)
	}

	// --- Single syscall: retrieve both owner and DACL ---
	//
	// Combining OWNER_SECURITY_INFORMATION and DACL_SECURITY_INFORMATION into
	// one GetNamedSecurityInfo call is more efficient than the two separate
	// calls made in checkOwner and getACL respectively.
	var ownerSID *windows.SID
	var dacl *winutil.ACL
	if err := winutil.GetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION,
		&ownerSID, nil, &dacl, nil, nil,
	); err != nil {
		return false, fmt.Errorf("cannot read file security descriptor: %w", err)
	}

	// --- Check 1: owner ---
	//
	// Mirrors checkOwner from pkg/util/filesystem/permission_windows.go (PR #46678).
	if !isAllowed(ownerSID) {
		return false, fmt.Errorf("config file is owned by %s — expected SYSTEM, Administrators, or ddagentuser", ownerSID.String())
	}

	// --- Check 2: DACL ---
	//
	// Mirrors CheckRights from pkg/util/filesystem/rights_windows.go, adapted
	// for a config file.
	var aclInfo winutil.ACL_SIZE_INFORMATION
	if err := winutil.GetAclInformation(dacl, &aclInfo, winutil.AclSizeInformation); err != nil {
		return false, fmt.Errorf("cannot get ACL information: %w", err)
	}

	for i := uint32(0); i < aclInfo.AceCount; i++ {
		var ace *winutil.ACCESS_ALLOWED_ACE
		if err := winutil.GetAce(dacl, i, &ace); err != nil {
			continue
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))

		switch ace.AceType {
		case winutil.ACCESS_ALLOWED_ACE_TYPE:
			// Any principal outside the allowed set with an explicit ALLOW
			// entry can read the file — and therefore the API key.
			if !isAllowed(sid) {
				return false, fmt.Errorf(
					"config file grants access to %s — only SYSTEM, Administrators, and ddagentuser should have access",
					sid.String())
			}

		case winutil.ACCESS_DENIED_ACE_TYPE:
			// An explicit DENY against one of the allowed principals would
			// prevent the Agent from reading its own config file.
			// (Denying any other principal is fine and is intentionally ignored.)
			if isAllowed(sid) {
				return false, fmt.Errorf(
					"config file explicitly denies access to %s — the Agent cannot read its own config",
					sid.String())
			}
		}
	}

	return true, nil
}

// getAdministratorsSID returns the SID of the built-in Administrators group
// (S-1-5-32-544). The returned SID must be freed by the caller with
// windows.FreeSid. This mirrors the unexported helper of the same name in
// pkg/util/filesystem/rights_windows.go.
func getAdministratorsSID() (*windows.SID, error) {
	var sid *windows.SID
	err := windows.AllocateAndInitializeSid(
		&windows.SECURITY_NT_AUTHORITY,
		2,
		windows.SECURITY_BUILTIN_DOMAIN_RID,
		windows.DOMAIN_ALIAS_RID_ADMINS,
		0, 0, 0, 0, 0, 0,
		&sid)
	return sid, err
}
