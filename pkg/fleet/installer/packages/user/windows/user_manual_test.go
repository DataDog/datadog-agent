// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Add build tag to exclude these tests from the regular unit tests.
// These tests are intended for manual verification on real hosts and
// won't succeed in the CI where the Agent is not installed.
//go:build windows && manualtest

package windowsuser

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestAgentUserPassword gets the password for the agent user from the LSA.
//
// Test will fail for service accounts that have no password.
func TestAgentUserPassword(t *testing.T) {
	password, err := getAgentUserPasswordFromLSA()
	assert.NoError(t, err)
	fmt.Println("password: ", password)
}

// TestValidate runs ValidateAgentUserRemoteUpdatePrerequisites, expect it to succeed on any host with 7.66 or later installed.
func TestValidate(t *testing.T) {
	disableProcessContextValidation(t)

	user := getTestAgentUser(t)
	err := ValidateAgentUserRemoteUpdatePrerequisites(user)
	assert.NoError(t, err)
}

// TestAgentUser prints information about the agent user, mimicing ValidateAgentUserRemoteUpdatePrerequisites.
//
// isServiceAccount will return an error on non-domain joined hosts.
func TestAgentUser(t *testing.T) {
	disableProcessContextValidation(t)

	user := getTestAgentUser(t)
	fmt.Println("user: ", user)

	sid, domain, err := lookupSID(user)
	assert.NoError(t, err)
	fmt.Println("domain: ", domain)
	fmt.Println("sid: ", sid.String())

	hasPassword, err := AgentUserPasswordPresent()
	assert.NoError(t, err)
	fmt.Println("password in LSA:", hasPassword)

	isLocalAccount, err := IsLocalAccount(sid)
	assert.NoError(t, err)
	fmt.Println("is local account:", isLocalAccount)

	computerSid, err := getComputerSid()
	assert.NoError(t, err)
	fmt.Println("computer sid:", computerSid)

	accountDomainSid, err := GetWindowsAccountDomainSid(sid)
	assert.NoError(t, err)
	fmt.Println("windows account domain sid:", accountDomainSid)

	isServiceAccount, err := IsServiceAccount(sid)
	assert.NoError(t, err)
	fmt.Println("is service account:", isServiceAccount)

	msaInfo, err := NetQueryServiceAccount(user)
	assert.NoError(t, err)
	fmt.Println("msa info:", msaInfo)

	netIsServiceAccount, err := NetIsServiceAccount(user)
	assert.NoError(t, err)
	fmt.Println("NetIsServiceAccount:", netIsServiceAccount)
}
