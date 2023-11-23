// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018-present Datadog, Inc.

//go:build secrets && windows

package secrets

import (
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
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
			return fmt.Errorf("secretBackendCommand '%s' does not exist", filename)
		}
		return fmt.Errorf("unable to check permissions for secretBackendCommand '%s': %s", filename, err)
	}

	fileDacl, err := getACL(filename)
	if err != nil {
		return fmt.Errorf("could not query ACLs for '%s': %s", filename, err)
	}

	var aclSizeInfo winutil.ACL_SIZE_INFORMATION
	err = winutil.GetAclInformation(fileDacl, &aclSizeInfo, winutil.AclSizeInformation)
	if err != nil {
		return fmt.Errorf("could not query ACLs for '%s': %s", filename, err)
	}

	// create the sids that are acceptable to us (local system account and
	// administrators group)
	localSystem, err := getLocalSystemSID()
	if err != nil {
		return fmt.Errorf("could not query Local System SID: %s", err)
	}
	defer windows.FreeSid(localSystem)

	administrators, err := getAdministratorsSID()
	if err != nil {
		return fmt.Errorf("could not query Administrator SID: %s", err)
	}
	defer windows.FreeSid(administrators)

	secretUser, err := getSecretUserSID()
	if err != nil {
		return err
	}

	bSecretUserExplicitlyAllowed := false
	for i := uint32(0); i < aclSizeInfo.AceCount; i++ {
		var pAce *winutil.ACCESS_ALLOWED_ACE
		if err := winutil.GetAce(fileDacl, i, &pAce); err != nil {
			return fmt.Errorf("could not query a ACE on '%s': %s", filename, err)
		}

		compareSid := (*windows.SID)(unsafe.Pointer(&pAce.SidStart))
		compareIsLocalSystem := windows.EqualSid(compareSid, localSystem)
		compareIsAdministrators := windows.EqualSid(compareSid, administrators)
		compareIsSecretUser := windows.EqualSid(compareSid, secretUser)

		if pAce.AceType == winutil.ACCESS_DENIED_ACE_TYPE {
			// if we're denying access to local system or administrators,
			// it's wrong. Otherwise, any explicit access denied is OK
			if compareIsLocalSystem || compareIsAdministrators || compareIsSecretUser {
				return fmt.Errorf("invalid executable '%s': explicit deny access for LOCAL_SYSTEM, Administrators or %s", filename, secretUser)
			}
			// otherwise, it's fine; deny access to whomever
		}
		if pAce.AceType == winutil.ACCESS_ALLOWED_ACE_TYPE {
			if !(compareIsLocalSystem || compareIsAdministrators || compareIsSecretUser) {
				return fmt.Errorf("invalid executable '%s': other users/groups than LOCAL_SYSTEM, Administrators or %s have rights on it", filename, secretUser)
			}
			if compareIsSecretUser {
				bSecretUserExplicitlyAllowed = true
			}
		}
	}

	if !bSecretUserExplicitlyAllowed {
		// there was never an ACE explicitly allowing the secret user, so we can't use it
		return fmt.Errorf("'%s' user is not allowed to execute secretBackendCommand '%s'", secretUser, filename)
	}
	return nil
}

// getACL retrieves the DACL for the file at filename path
func getACL(filename string) (*winutil.ACL, error) {
	var fileDacl *winutil.ACL
	err := winutil.GetNamedSecurityInfo(filename,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION,
		nil,
		nil,
		&fileDacl,
		nil,
		nil)

	return fileDacl, err
}

// getLocalSystemSID returns the SID of the Local System account
func getLocalSystemSID() (*windows.SID, error) {
	var localSystem *windows.SID
	err := windows.AllocateAndInitializeSid(&windows.SECURITY_NT_AUTHORITY,
		1, // local system has 1 valid subauth
		windows.SECURITY_LOCAL_SYSTEM_RID,
		0, 0, 0, 0, 0, 0, 0,
		&localSystem)

	return localSystem, err
}

// getAdministratorsSID returns the SID of the built-in Administrators group principal
func getAdministratorsSID() (*windows.SID, error) {
	var administrators *windows.SID
	err := windows.AllocateAndInitializeSid(&windows.SECURITY_NT_AUTHORITY,
		2, // administrators group has 2 valid subauths
		windows.SECURITY_BUILTIN_DOMAIN_RID,
		windows.DOMAIN_ALIAS_RID_ADMINS,
		0, 0, 0, 0, 0, 0,
		&administrators)
	return administrators, err
}

// getSecretUserSID returns the SID of the user running the secret backend
func getSecretUserSID() (*windows.SID, error) {
	localSystem, err := getLocalSystemSID()
	if err != nil {
		return nil, fmt.Errorf("could not query Local System SID: %s", err)
	}
	defer windows.FreeSid(localSystem)

	currentUser, err := winutil.GetSidFromUser()
	if err != nil {
		return nil, fmt.Errorf("could not get SID for current user: %s", err)
	}

	secretUser := currentUser

	elevated, err := winutil.IsProcessElevated()
	if err != nil {
		return nil, fmt.Errorf("unable to determine if running elevated: %s", err)
	}

	if elevated || currentUser.Equals(localSystem) {
		ddUser, err := getDDAgentUserSID()
		if err != nil {
			return nil, fmt.Errorf("could not resolve SID for ddagentuser user: %s", err)
		}
		secretUser = ddUser
	}
	return secretUser, nil
}

// getDDAgentUserSID returns the SID of the ddagentuser configured at installation time
var getDDAgentUserSID = func() (*windows.SID, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Datadog\Datadog Agent`, registry.QUERY_VALUE)
	if err != nil {
		return nil, fmt.Errorf("could not open installer registry key: %s", err)
	}
	defer k.Close()

	user, _, err := k.GetStringValue("installedUser")
	if err != nil {
		return nil, fmt.Errorf("could not read installedUser in registry: %s", err)
	}

	domain, _, err := k.GetStringValue("installedDomain")
	if err != nil {
		return nil, fmt.Errorf("could not read installedDomain in registry: %s", err)
	}

	if domain != "" {
		user = domain + `\` + user
	}

	sid, _, _, err := windows.LookupSID("", user)
	return sid, err
}
