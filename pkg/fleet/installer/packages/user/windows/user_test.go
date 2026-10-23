// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package windowsuser

import (
	"fmt"
	"os"
	"os/exec"
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
		// This test creates a user in the machine running the test as a side-effect,
		// so we only run it on CI.
		t.Skip("skipping test outside of CI")
	}

	agentUser, err := createTestAgentUser("ddagentuser-test")
	require.NoError(t, err, "failed to create user for tests: %w", err)

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

// createTestAgentUser creates a local user for use inside CI tests
func createTestAgentUser(username string) (string, error) {
	const script = `
$ErrorActionPreference = 'Stop'
$username = $env:DD_TEST_AGENT_USERNAME

$existingUser = Get-LocalUser | Where-Object { $_.Name -eq $username }
if (-not $existingUser) {
    $password = ConvertTo-SecureString 'dummyPW_:-gch6Rejae9' -AsPlainText -Force
    New-LocalUser -Name $username -Description 'Test user for the secrets feature on windows.' -Password $password | Out-Null
}
`

	cmd := exec.Command(
		"powershell.exe",
		"-NoProfile",
		"-NonInteractive",
		"-Command", script,
	)
	cmd.Env = append(os.Environ(), "DD_TEST_AGENT_USERNAME="+username)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("ensure local test agent user %q exists: %w\n%s", username, err, output)
	}
	return fmt.Sprintf("%s\\%s", os.Getenv("COMPUTERNAME"), username), nil
}

// disableProcessContextValidation disables validateProcessContext for the duration of the test.
func disableProcessContextValidation(t *testing.T) {
	oldValidateProcessContext := validateProcessContext
	validateProcessContext = func() error {
		return nil
	}
	t.Cleanup(func() {
		validateProcessContext = oldValidateProcessContext
	})
}
