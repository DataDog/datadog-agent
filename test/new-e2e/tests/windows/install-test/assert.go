// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installtest

import (
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
