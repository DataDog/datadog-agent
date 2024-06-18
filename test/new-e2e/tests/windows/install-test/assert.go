// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installtest

import (
	"fmt"
	"slices"
	"strings"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"

	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common"
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

// RequireAgentRunningWithNoErrors checks the agent is running with no errors
func RequireAgentRunningWithNoErrors(t *testing.T, client *common.TestClient) {
	common.CheckAgentBehaviour(t, client)
}

// getExpectedSignedFilesForAgentMajorVersion returns the list of files that should be signed for the given agent major version
// as relative paths from the agent install directory.
func getExpectedSignedFilesForAgentMajorVersion(majorVersion string) []string {
	// all executables should be signed
	return getExpectedExecutablesForAgentMajorVersion(majorVersion)
}

// getExpectedConfigFiles returns the list of config files that should be present in the agent config directory,
// as relative paths from the agent config directory.
func getExpectedConfigFiles() []string {
	return []string{
		`datadog.yaml`,
		`system-probe.yaml`,
		`security-agent.yaml`,
		`runtime-security.d\default.policy`,
		`conf.d\win32_event_log.d\profiles\dd_security_events_high.yaml`,
		`conf.d\win32_event_log.d\profiles\dd_security_events_low.yaml`,
	}
}

// getExpectedBinFilesForAgentMajorVersion returns the list of files that should be present in the agent install directory,
// as relative paths from the agent install directory.
func getExpectedBinFilesForAgentMajorVersion(majorVersion string) []string {
	py3 := shortPythonVersion(common.ExpectedPythonVersion3)
	paths := []string{
		// user binaries
		`bin\agent.exe`,
		`bin\agent\ddtray.exe`,
		`bin\agent\trace-agent.exe`,
		`bin\agent\process-agent.exe`,
		`bin\agent\security-agent.exe`,
		`bin\agent\system-probe.exe`,
		// drivers
		`bin\agent\driver\ddnpm.sys`,
		`bin\agent\driver\ddnpm.inf`,
		`bin\agent\driver\ddnpm.cat`,
		`bin\agent\driver\ddprocmon.sys`,
		`bin\agent\driver\ddprocmon.inf`,
		`bin\agent\driver\ddprocmon.cat`,
		// python3
		`bin\libdatadog-agent-three.dll`,
		`embedded3\python.exe`,
		`embedded3\pythonw.exe`,
		fmt.Sprintf(`embedded3\python%s.dll`, py3),
	}
	if ExpectPython2Installed(majorVersion) {
		py2 := shortPythonVersion(common.ExpectedPythonVersion2)
		paths = append(paths, []string{
			`bin\libdatadog-agent-two.dll`,
			`embedded2\python.exe`,
			`embedded2\pythonw.exe`,
			fmt.Sprintf(`embedded2\python%s.dll`, py2),
		}...)
	}
	return paths
}

// getExpectedExecutablesForAgentMajorVersion returns the list of executables that should be present in the agent install directory,
// as relative paths from the agent install directory.
func getExpectedExecutablesForAgentMajorVersion(majorVersion string) []string {
	r := getExpectedBinFilesForAgentMajorVersion(majorVersion)
	// keep only items ending in dll, exe, or sys
	var executables []string
	for _, f := range r {
		if strings.HasSuffix(f, ".dll") || strings.HasSuffix(f, ".exe") || strings.HasSuffix(f, ".sys") {
			executables = append(executables, f)
		}
	}
	return executables
}

// shortPythonVersion returns the short version of the provided Python version. (e.g. 3.7.3 -> 37)
func shortPythonVersion(version string) string {
	return strings.Join(strings.Split(version, ".")[0:2], "")
}

// ExpectPython2Installed returns true if the provided agent major version is expected
// to contain an embedded Python2.
func ExpectPython2Installed(majorVersion string) bool {
	return majorVersion == "6"
}

// getBaseConfigRootSecurity returns the base security settings for the config root
//   - SYSTEM full control, owner and group
//   - Administrators full control
//   - protected (inheritance disabled)
func getBaseConfigRootSecurity() (windows.ObjectSecurity, error) {
	// SYSTEM and Administrators have full control
	return windows.NewProtectedSecurityInfo(
		windows.GetIdentityForSID(windows.LocalSystemSID),
		windows.GetIdentityForSID(windows.LocalSystemSID),
		[]windows.AccessRule{
			windows.NewExplicitAccessRuleWithFlags(
				windows.GetIdentityForSID(windows.LocalSystemSID),
				windows.FileFullControl,
				windows.AccessControlTypeAllow,
				windows.InheritanceFlagsContainer|windows.InheritanceFlagsObject,
				windows.PropagationFlagsNone,
			),
			windows.NewExplicitAccessRuleWithFlags(
				windows.GetIdentityForSID(windows.AdministratorsSID),
				windows.FileFullControl,
				windows.AccessControlTypeAllow,
				windows.InheritanceFlagsContainer|windows.InheritanceFlagsObject,
				windows.PropagationFlagsNone,
			),
		},
	), nil
}

// getBaseInheritedConfigFileSecurity returns the base security settings for a config file that inherits permissions from the config root
//   - (inherited) SYSTEM full control, owner and group
//   - (inherited) Administrators full control
func getBaseInheritedConfigFileSecurity() (windows.ObjectSecurity, error) {
	// SYSTEM and Administrators have full control
	return windows.NewInheritSecurityInfo(
		windows.GetIdentityForSID(windows.LocalSystemSID),
		windows.GetIdentityForSID(windows.LocalSystemSID),
		[]windows.AccessRule{
			windows.NewInheritedAccessRule(
				windows.GetIdentityForSID(windows.LocalSystemSID),
				windows.FileFullControl,
				windows.AccessControlTypeAllow,
			),
			windows.NewInheritedAccessRule(
				windows.GetIdentityForSID(windows.AdministratorsSID),
				windows.FileFullControl,
				windows.AccessControlTypeAllow,
			),
		},
	), nil
}

// getBaseInheritedConfigDirSecurity returns the base security settings for a config dir that inherits permissions from the config root
//   - (inherited) SYSTEM full control, owner and group
//   - (inherited) Administrators full control
func getBaseInheritedConfigDirSecurity() (windows.ObjectSecurity, error) {
	// SYSTEM and Administrators have full control
	return windows.NewInheritSecurityInfo(
		windows.GetIdentityForSID(windows.LocalSystemSID),
		windows.GetIdentityForSID(windows.LocalSystemSID),
		[]windows.AccessRule{
			windows.NewInheritedAccessRuleWithFlags(
				windows.GetIdentityForSID(windows.LocalSystemSID),
				windows.FileFullControl,
				windows.AccessControlTypeAllow,
				windows.InheritanceFlagsContainer|windows.InheritanceFlagsObject,
				windows.PropagationFlagsNone,
			),
			windows.NewInheritedAccessRuleWithFlags(
				windows.GetIdentityForSID(windows.AdministratorsSID),
				windows.FileFullControl,
				windows.AccessControlTypeAllow,
				windows.InheritanceFlagsContainer|windows.InheritanceFlagsObject,
				windows.PropagationFlagsNone,
			),
		},
	), nil
}
