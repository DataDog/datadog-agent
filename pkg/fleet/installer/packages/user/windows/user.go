// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package windowsuser offers an interface over user management on Windows
package windowsuser

/*
#include "lsa.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// ErrPrivateDataNotFound is returned when LSARetrievePrivateData returns STATUS_OBJECT_NAME_NOT_FOUND
var ErrPrivateDataNotFound = errors.New("private data not found")

// ValidateAgentUserRemoteUpdatePrerequisites validates the prerequisites for remote updates with the Agent user
//
// NOTE: This function must not be used to validate the Agent user prior to initial installation.
// That requires additional processing on the account name for handling of names that do not yet exist.
// Validation of initial installation is left to the MSI. We forward any MSI errors to the user.
//
// NOTE: This function is intended to be run only by the daemon service and its subprocesses running as LocalSystem.
// If this assumption changes, we must change how we validate gMSA accounts. See NetIsServiceAccount docs for details.
//
// Keep loosely in sync with the MSI ProcessUserCustomActions conditions. Noting the difference between
// fresh installs and remote updates noted above.
func ValidateAgentUserRemoteUpdatePrerequisites(userName string) error {
	if err := validateProcessContext(); err != nil {
		return err
	}

	// Sanity check expected username format.
	// We always store both parts in the registry so we should always have both here.
	if !strings.Contains(userName, `\`) {
		return fmt.Errorf("the provided account '%s' is not in the expected format domain\\username", userName)
	}

	// Check if the account exists
	// The account must already exist during remote updates.
	sid, _, err := lookupSID(userName)
	if err != nil {
		return fmt.Errorf("failed to lookup SID for account %s: %w. Please ensure the account exists", userName, err)
	}

	if IsSupportedWellKnownAccount(sid) {
		// no password is required for well known service accounts
		return nil
	}

	// Check if the account is a local account
	isLocalAccount, err := IsLocalAccount(sid)
	if err != nil {
		return fmt.Errorf("failed to check if account is a local account: %w", err)
	}
	if isLocalAccount {
		// no need to check if password exists.
		// The MSI will create a new password if needed.
		return nil
	}

	isServiceAccount, err := IsServiceAccount(sid)
	if err != nil {
		// We expect this function
		return fmt.Errorf("failed to check if account is a service account: %w. Please ensure the netlogon service is running and the domain controller is available", err)
	}
	if isServiceAccount {
		// no need to check if password exists.
		// gMSA accounts do not have passwords
		return nil
	} else if strings.HasSuffix(userName, "$") {
		return fmt.Errorf("The provided account '%s' ends with '$' but is not recognized as a valid gMSA account. Please ensure the username is correct and this host is a member of PrincipalsAllowedToRetrieveManagedPassword. If the account is a normal account, please reinstall the Agent with the password provided", userName)
	}

	// At this point, we assume the account is a domain account.
	// We need to check if the account has a password.
	passwordPresent, err := AgentUserPasswordPresent()
	if err != nil {
		return fmt.Errorf("failed to check if account has password: %w", err)
	}

	if !passwordPresent {
		return fmt.Errorf("the Agent user password is not available. The password is required for domain accounts. Please reinstall the Agent with the password provided")
	}

	return nil
}

func agentPasswordPrivateDataKey() string {
	// Keep in sync with MSI ConfigureUserCustomActions.AgentPasswordPrivateDataKey
	return "L$datadog_ddagentuser_password"
}

func getAgentUserPasswordFromLSA() (string, error) {
	key := agentPasswordPrivateDataKey()
	return retrievePrivateData(key)
}

func retrievePrivateData(key string) (string, error) {
	// Convert Go string to UTF-16
	keyUtf16, err := windows.UTF16PtrFromString(key)
	if err != nil {
		return "", fmt.Errorf("failed to convert key to UTF-16: %w", err)
	}

	// Call C function to retrieve private data
	var cResult unsafe.Pointer
	var cResultSize C.size_t
	s := C.retrieve_private_data(unsafe.Pointer(keyUtf16), &cResult, &cResultSize)
	if s != 0 {
		status := windows.NTStatus(s)
		if errors.Is(status, STATUS_OBJECT_NAME_NOT_FOUND) {
			return "", ErrPrivateDataNotFound
		}
		return "", fmt.Errorf("failed to retrieve private data from LSA: %w", status)
	}

	if cResult == nil {
		return "", nil
	}
	defer C.free_private_data(cResult, cResultSize)

	// Convert result back to Go string
	result := windows.UTF16PtrToString((*uint16)(cResult))
	return result, nil
}

// IsSupportedWellKnownAccount returns true if the account is a well known account that we support running the Agent as
//
// Current list: LocalSystem, LocalService, NetworkService
func IsSupportedWellKnownAccount(sid *windows.SID) bool {
	// First check the well known accounts that we support running the Agent as
	supportedWellKnownAccounts := []windows.WELL_KNOWN_SID_TYPE{
		windows.WinLocalSystemSid,
		windows.WinLocalServiceSid,
		windows.WinNetworkServiceSid,
	}
	for _, a := range supportedWellKnownAccounts {
		if sid.IsWellKnown(a) {
			return true
		}
	}
	return false
}

// IsServiceAccount returns true if the account is a service account.
//
// This function checks if the account is a well known account or a gMSA account.
//
// For implementation details and usage restrictions, see NetIsServiceAccount.
//
// https://learn.microsoft.com/en-us/windows-server/identity/ad-ds/manage/group-managed-service-accounts/group-managed-service-accounts/group-managed-service-accounts-overview
func IsServiceAccount(sid *windows.SID) (bool, error) {
	if err := validateProcessContext(); err != nil {
		return false, err
	}

	if sid == nil {
		return false, errors.New("sid is nil")
	}

	// Return true for well known accounts since they also don't have a password.
	// We should generally check this separately so it's more of a sanity check because
	// the naming conventions overlap and the check is cheap to perform.
	if IsSupportedWellKnownAccount(sid) {
		return true, nil
	}

	userName, domain, _, err := sid.LookupAccount("")
	if err != nil {
		return false, fmt.Errorf("failed to lookup account name for SID %s: %w", sid.String(), err)
	}

	r, err := NetIsServiceAccount(domain + `\` + userName)
	if err != nil {
		if errors.Is(err, RPC_NT_SERVER_UNAVAILABLE) {
			// Do not add punctuation after %w, the error message already contains it.
			err = fmt.Errorf("%w Please ensure the netlogon service is running and the domain controller is available", err)
		} else if errors.Is(err, windows.STATUS_OPEN_FAILED) {
			// Do not wrap the error message in the error string, it is too verbose and is unrelated to the actual issue
			err = fmt.Errorf("error 0x%X. Please ensure the netlogon service is running, the domain controller is available, and the current process has network credentials that will be accepted by the domain controller", windows.STATUS_OPEN_FAILED)
		}
		return false, fmt.Errorf("failed to check if account is a service account: %w", err)
	}
	return r, nil
}

// IsLocalAccount returns true if the account is a local account.
// This function compares the domain part of the account SID to the computer SID
//
// https://learn.microsoft.com/en-us/archive/blogs/aaron_margosis/machine-sids-and-domain-sids
func IsLocalAccount(sid *windows.SID) (bool, error) {
	if sid == nil {
		return false, errors.New("sid is nil")
	}

	// Get the domain SID for the account
	userDomainSid, err := GetWindowsAccountDomainSid(sid)
	if err != nil {
		if errors.Is(err, windows.ERROR_NON_ACCOUNT_SID) || errors.Is(err, windows.ERROR_NON_DOMAIN_SID) {
			// Can't be a domain account, is probably a container user or LocalSystem
			return false, nil
		}
		return false, fmt.Errorf("failed to get domain SID for account %s: %w", sid.String(), err)
	}

	// Get local computer name
	computerName, err := GetComputerName()
	if err != nil {
		return false, fmt.Errorf("failed to get local computer name: %w", err)
	}

	// Get the SID for the local host
	hostSid, _, err := lookupSID(computerName)
	if err != nil {
		return false, err
	}
	if hostSid == nil {
		return false, fmt.Errorf("could not get host SID")
	}

	// if the domain SID is different from the host SID, it's a domain account
	return userDomainSid.Equals(hostSid), nil
}

// AgentUserPasswordPresent returns true if the Agent user password is present in LSA.
func AgentUserPasswordPresent() (bool, error) {
	password, err := getAgentUserPasswordFromLSA()
	if err != nil {
		if errors.Is(err, ErrPrivateDataNotFound) {
			return false, nil
		}
		return false, err
	}
	return password != "", nil
}

// GetAgentUserNameFromRegistry returns the user name for the Agent, stored in the registry by the Agent MSI
func GetAgentUserNameFromRegistry() (string, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, "SOFTWARE\\Datadog\\Datadog Agent", registry.QUERY_VALUE)
	if err != nil {
		return "", err
	}
	defer k.Close()

	user, _, err := k.GetStringValue("installedUser")
	if err != nil {
		return "", fmt.Errorf("could not read installedUser in registry: %w", err)
	}

	domain, _, err := k.GetStringValue("installedDomain")
	if err != nil {
		return "", fmt.Errorf("could not read installedDomain in registry: %w", err)
	}

	if domain != "" {
		user = domain + `\` + user
	}

	return user, nil
}

func lookupSID(name string) (*windows.SID, string, error) {
	sid, domain, _, err := windows.LookupSID("", name)
	if err != nil {
		return nil, "", err
	}
	return sid, domain, nil
}

// validateProcessContext validates that the current process is running as LocalSystem
//
// Created as a variable so we can override it in unit tests.
var validateProcessContext = func() error {
	token := windows.GetCurrentProcessToken()
	// token is a pseudo token that does not need to be closed

	user, err := token.GetTokenUser()
	if err != nil {
		return fmt.Errorf("failed to get token user: %w", err)
	}

	if !user.User.Sid.IsWellKnown(windows.WinLocalSystemSid) {
		return fmt.Errorf("process is not running as LocalSystem")
	}

	return nil
}
