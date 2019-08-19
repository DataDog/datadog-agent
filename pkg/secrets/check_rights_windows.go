// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build secrets,windows

package secrets

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

func getSidForUser(username string) (sid *windows.SID, err error) {
	//
	// when getting the SID for the secret user, unlike above, we provide
	// the buffer. So this SID should *not* be passed to FreeSid() (the
	// way the other ones are. So much for API consistency
	//
	// also, *must* provide adequate buffer for the domain name, or the
	// function will fail (even though we aren't going to use it for anything)
	//
	var secretusersyscall *syscall.SID
	var cchRefDomain uint32
	var sidUse uint32
	var sidlen uint32
	var domainptr uint16

	// first call to get the sidbuf and domainbuf length
	err = syscall.LookupAccountName(nil, // local system lookup
		windows.StringToUTF16Ptr(username),
		secretusersyscall,
		&sidlen,
		&domainptr,
		&cchRefDomain,
		&sidUse)
	if err != error(syscall.ERROR_INSUFFICIENT_BUFFER) {
		// should never happen
		return nil, fmt.Errorf("could not query %s SID : %v", username, err)
	}

	sidbuf := make([]uint8, sidlen+1)
	domainbuf := make([]uint16, cchRefDomain+1)
	secretusersyscall = (*syscall.SID)(unsafe.Pointer(&sidbuf[0]))

	// second call to actually fetch the SID for username
	err = syscall.LookupAccountName(nil, // local system lookup
		windows.StringToUTF16Ptr(username),
		secretusersyscall,
		&sidlen,
		&domainbuf[0],
		&cchRefDomain,
		&sidUse)
	if err != nil {
		// should never happen
		return nil, fmt.Errorf("could not query %s SID: %s", username, err)
	}

	return (*windows.SID)(unsafe.Pointer(secretusersyscall)), nil
}

// checkRights check that the given filename has access controls set only for
// Administrator, Local System and the datadog user.
func checkRights(filename string) error {
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
	agent_proc, err := getSidForUser("NT Service\\datadogagent")
	if err != nil {
		agent_proc, err = windows.StringToSid("S-1-5-80-1780442038-2564740535-2014067642-3562800676-515077229")
		if err != nil {
			return fmt.Errorf("Failed to get SID for datadog agent service")
		}
		defer windows.FreeSid(agent_proc)
	}
	trace_proc, err := getSidForUser("NT Service\\datadog-trace-agent")
	if err != nil {
		trace_proc, err = windows.StringToSid("S-1-5-80-3626218227-2896763321-2052920590-1920844846-327269072")
		if err != nil {
			return fmt.Errorf("Failed to get SID for datadog trace agent service")
		}
		defer windows.FreeSid(trace_proc)
	}

	bAgentServiceAllowed := false
	bTraceServiceAllowed := false
	for i := uint32(0); i < aclSizeInfo.AceCount; i++ {
		var pAce *winutil.AccessAllowedAce
		if err := winutil.GetAce(fileDacl, i, &pAce); err != nil {
			return fmt.Errorf("Could not query a ACE on %s: %s", filename, err)
		}

		compareSid := (*windows.SID)(unsafe.Pointer(&pAce.SidStart))
		compareIsLocalSystem := windows.EqualSid(compareSid, localSystem)
		compareIsAdministrators := windows.EqualSid(compareSid, administrators)
		compareIsAgentService := windows.EqualSid(compareSid, agent_proc)
		compareIsTraceService := windows.EqualSid(compareSid, trace_proc)

		if pAce.AceType == winutil.ACCESS_DENIED_ACE_TYPE {
			// if we're denying access to local system or administrators,
			// it's wrong. Otherwise, any explicit access denied is OK
			if compareIsLocalSystem || compareIsAdministrators || compareIsAgentService || compareIsTraceService {
				return fmt.Errorf("Invalid executable '%s': Can't deny access LOCAL_SYSTEM, Administrators or agent services", filename)
			}
			// otherwise, it's fine; deny access to whomever
		}
		if pAce.AceType == winutil.ACCESS_ALLOWED_ACE_TYPE {
			if !(compareIsLocalSystem || compareIsAdministrators || compareIsAgentService || compareIsTraceService) {
				return fmt.Errorf("Invalid executable '%s': other users/groups than LOCAL_SYSTEM, Administrators or agent services have rights on it", filename)
			}
			if compareIsAgentService {
				bAgentServiceAllowed = true
			}
			if compareIsTraceService {
				bTraceServiceAllowed = true
			}
		}
	}
	if !bAgentServiceAllowed || !bTraceServiceAllowed {
		// there was never an ACE explicitly allowing the secret user, so we can't use it
		return fmt.Errorf("'%s' user is not allowed to execute secretBackendCommand '%s'", "service", filename)
	}
	return nil
}
