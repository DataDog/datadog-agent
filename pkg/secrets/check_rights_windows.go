// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018-present Datadog, Inc.

// +build secrets,windows

package secrets

import (
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

var (
	username = "ddagentuser"
)

// checkRights check that the given filename has access controls set only for
// Administrator, Local System and the datadog user.
func checkRights(filename string, allowGroupExec bool) error {
	// this function ignore `allowGroupExec` since it was design for the cluster-agent,
	// but the cluster-agent is not delivered for windows.
	if allowGroupExec {
		return fmt.Errorf("the option 'allowGroupExec=true' is not allowed on windows")
	}
	if _, err := os.Stat(filename); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("secretBackendCommand %s does not exist", filename)
		}
	}

	var fileDacl *winutil.Acl
	err := winutil.GetNamedSecurityInfo(filename,
		winutil.SE_FILE_OBJECT,
		winutil.DACL_SECURITY_INFORMATION,
		nil,
		nil,
		&fileDacl,
		nil,
		nil)
	if err != nil {
		return fmt.Errorf("could not query ACLs for %s: %s", filename, err)
	}

	var aclSizeInfo winutil.AclSizeInformation
	err = winutil.GetAclInformation(fileDacl, &aclSizeInfo, winutil.AclSizeInformationEnum)
	if err != nil {
		return fmt.Errorf("could not query ACLs for %s: %s", filename, err)
	}

	// create the sids that are acceptable to us (local system account and
	// administrators group)
	var localSystem *windows.SID
	err = windows.AllocateAndInitializeSid(&windows.SECURITY_NT_AUTHORITY,
		1, // local system has 1 valid subauth
		windows.SECURITY_LOCAL_SYSTEM_RID,
		0, 0, 0, 0, 0, 0, 0,
		&localSystem)
	if err != nil {
		return fmt.Errorf("could not query Local System SID: %s", err)
	}
	defer windows.FreeSid(localSystem)

	var administrators *windows.SID
	err = windows.AllocateAndInitializeSid(&windows.SECURITY_NT_AUTHORITY,
		2, // administrators group has 2 valid subauths
		windows.SECURITY_BUILTIN_DOMAIN_RID,
		windows.DOMAIN_ALIAS_RID_ADMINS,
		0, 0, 0, 0, 0, 0,
		&administrators)
	if err != nil {
		return fmt.Errorf("could not query Administrator SID: %s", err)
	}
	defer windows.FreeSid(administrators)

	secretuser, err := winutil.GetSidFromUser()
	if err != nil {
		return fmt.Errorf("could not get SID for current user: %s", err)
	}

	bSecretUserExplicitlyAllowed := false
	for i := uint32(0); i < aclSizeInfo.AceCount; i++ {
		var pAce *winutil.AccessAllowedAce
		if err := winutil.GetAce(fileDacl, i, &pAce); err != nil {
			return fmt.Errorf("Could not query a ACE on %s: %s", filename, err)
		}

		compareSid := (*windows.SID)(unsafe.Pointer(&pAce.SidStart))
		compareIsLocalSystem := windows.EqualSid(compareSid, localSystem)
		compareIsAdministrators := windows.EqualSid(compareSid, administrators)
		compareIsSecretUser := windows.EqualSid(compareSid, secretuser)

		if pAce.AceType == winutil.ACCESS_DENIED_ACE_TYPE {
			// if we're denying access to local system or administrators,
			// it's wrong. Otherwise, any explicit access denied is OK
			if compareIsLocalSystem || compareIsAdministrators || compareIsSecretUser {
				return fmt.Errorf("Invalid executable '%s': Can't deny access LOCAL_SYSTEM, Administrators or %s", filename, secretuser)
			}
			// otherwise, it's fine; deny access to whomever
		}
		if pAce.AceType == winutil.ACCESS_ALLOWED_ACE_TYPE {
			if !(compareIsLocalSystem || compareIsAdministrators || compareIsSecretUser) {
				return fmt.Errorf("Invalid executable '%s': other users/groups than LOCAL_SYSTEM, Administrators or %s have rights on it", filename, secretuser)
			}
			if compareIsSecretUser {
				bSecretUserExplicitlyAllowed = true
			}
		}
	}

	if !bSecretUserExplicitlyAllowed {
		// there was never an ACE explicitly allowing the secret user, so we can't use it
		return fmt.Errorf("'%s' user is not allowed to execute secretBackendCommand '%s'", secretuser, filename)
	}
	return nil
}
