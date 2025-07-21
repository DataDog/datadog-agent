// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package windowsuser

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIsWellKnownAccount tests that go can lookup the SID for well known accounts, and that we recognize them as such.
func TestIsWellKnownAccount(t *testing.T) {
	disableProcessContextValidation(t)

	names := []string{
		"NT AUTHORITY\\SYSTEM",
		"NT AUTHORITY\\LOCAL SERVICE",
		"NT AUTHORITY\\NETWORK SERVICE",
	}

	for _, name := range names {
		sid, _, err := lookupSID(name)
		assert.NoError(t, err)
		assert.True(t, IsSupportedWellKnownAccount(sid), "expected %s to be a well known account", name)
		// IsServiceAccount should return true for well known accounts since they also don't have a password
		// we generally check this separately so it's more of a sanity check.
		isServiceAccount, err := IsServiceAccount(sid)
		assert.NoError(t, err)
		assert.True(t, isServiceAccount, "expected %s to be a service account", name)
		err = ValidateAgentUserRemoteUpdatePrerequisites(name)
		assert.NoError(t, err, "validate should succeed for well known service accounts")
	}
}

func TestAgentUserIsLocalAccount(t *testing.T) {
	if !runningInCI() {
		// ddagentuser is created by CI tests so we can make some assumptions about it in that environment,
		// but not outside of it.
		t.Skip("skipping test outside of CI")
	}
	// Agent user created in Invoke-UnitTests.ps1
	agentUser := getTestAgentUser(t)
	sid, _, err := lookupSID(agentUser)
	assert.NoError(t, err, "expected %s to be a valid account", agentUser)

	isLocalAccount, err := IsLocalAccount(sid)
	assert.NoError(t, err)
	assert.True(t, isLocalAccount, "expected %s to be a local account", agentUser)

	// We don't expect the CI unit test environment to have the password configured in the LSA
	passwordPresent, err := AgentUserPasswordPresent()
	assert.NoError(t, err, "not found should return false, not an error")
	assert.False(t, passwordPresent, "expected %s to not have a password", agentUser)
}

func TestNonExistingUser(t *testing.T) {
	disableProcessContextValidation(t)

	user := `.\non-existing-user`
	err := ValidateAgentUserRemoteUpdatePrerequisites(user)
	assert.ErrorContains(t, err, "Please ensure the account exists")

	user = `non-existing-user`
	err = ValidateAgentUserRemoteUpdatePrerequisites(user)
	assert.ErrorContains(t, err, "not in the expected format domain\\username")
}

func runningInCI() bool {
	return os.Getenv("CI") != ""
}

func getTestAgentUser(t *testing.T) string {
	var err error
	user := os.Getenv("DD_AGENT_USER_NAME")
	if user != "" {
		return user
	}

	if runningInCI() {
		return fmt.Sprintf("%s\\%s", os.Getenv("COMPUTERNAME"), os.Getenv("DD_AGENT_USER_NAME"))
	}

	user, err = GetAgentUserNameFromRegistry()
	require.NoError(t, err, "failed to get agent user from registry, please set DD_AGENT_USER_NAME")

	return user
}

// disableProcessContextValidation is a helper function to disable the process context validation in unit tests.
func disableProcessContextValidation(t *testing.T) {
	oldValidateProcessContext := validateProcessContext
	validateProcessContext = func() error {
		return nil
	}
	t.Cleanup(func() {
		validateProcessContext = oldValidateProcessContext
	})
}
