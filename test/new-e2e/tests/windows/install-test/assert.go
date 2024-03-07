// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installtest

import (
	"slices"
	"strings"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"

	windows "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"

	"github.com/stretchr/testify/assert"
	"testing"
)

// AssertInstalledUserInRegistry checks the registry for the installed user and domain
func AssertInstalledUserInRegistry(t *testing.T, host *components.RemoteHost, expecteddomain string, expectedusername string) bool {
	// check registry keys
	domain, username, err := windowsAgent.GetAgentUserFromRegistry(host)
	if !assert.NoError(t, err) {
		return false
	}
	username = strings.ToLower(username)
	expectedusername = strings.ToLower(expectedusername)
	// It's not a perfect test to be comparing the NetBIOS version of each domain, but the installer isn't
	// consistent with what it writes to the registry. On domain controllers, if the user exists then the domain part comes from the output
	// of LookupAccountName, which seems to consistently be a NetBIOS name. However, if the installer creates the account and a domain part wasn't
	// provided, then the FQDN is used and written to the registry.
	domain = windows.NameToNetBIOSName(domain)
	expecteddomain = windows.NameToNetBIOSName(expecteddomain)

	if !assert.Equal(t, expectedusername, username, "installedUser registry value should be %s", expectedusername) {
		return false
	}
	if !assert.Equal(t, expecteddomain, domain, "installedDomain registry value should be %s", expecteddomain) {
		return false
	}

	return true
}

// AssertAgentUserGroupMembership checks the agent user is a member of the expected groups
func AssertAgentUserGroupMembership(t *testing.T, host *components.RemoteHost, username string) bool {
	expectedGroups := []string{
		"Performance Log Users",
		"Event Log Readers",
		"Performance Monitor Users",
	}
	return AssertGroupMembership(t, host, username, expectedGroups)
}

// AssertGroupMembership asserts that the user is a member of the expected groups
func AssertGroupMembership(t *testing.T, host *components.RemoteHost, user string, expectedGroups []string) bool {
	hostInfo, err := windows.GetHostInfo(host)
	if !assert.NoError(t, err) {
		return false
	}
	userSid, err := windows.GetSIDForUser(host, user)
	if !assert.NoError(t, err) {
		return false
	}
	for _, g := range expectedGroups {
		// get members of group g
		var members []windows.SecurityIdentifier
		if hostInfo.IsDomainController() {
			// Domain controllers don't have local groups
			adMembers, err := windows.GetADGroupMembers(host, g)
			if !assert.NoError(t, err) {
				return false
			}
			for _, m := range adMembers {
				members = append(members, m)
			}
		} else {
			localMembers, err := windows.GetLocalGroupMembers(host, g)
			if !assert.NoError(t, err) {
				return false
			}
			for _, m := range localMembers {
				members = append(members, m)
			}
		}
		// check if user is in group
		assert.True(t, slices.ContainsFunc(members, func(s windows.SecurityIdentifier) bool {
			return strings.EqualFold(s.GetSID(), userSid)
		}), "user should be member of group %s", g)
	}
	return true
}

// AssertUserRights checks the user has the expected user rights
func AssertUserRights(t *testing.T, host *components.RemoteHost, username string) bool {
	expectedRights := []string{
		"SeServiceLogonRight",
		"SeDenyInteractiveLogonRight",
		"SeDenyNetworkLogonRight",
		"SeDenyRemoteInteractiveLogonRight",
	}
	actualRights, err := windows.GetUserRightsForUser(host, username)
	if !assert.NoError(t, err, "should get user rights") {
		return false
	}
	return assert.ElementsMatch(t, expectedRights, actualRights, "user %s should have user rights", username)
}
