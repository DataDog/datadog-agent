// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Add build tag to exclude these tests from the regular unit tests.
// These tests are intended for manual verification on real hosts and
// won't succeed in the CI where the Agent is not installed.
//go:build manualtest

package windowsuser

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAgentUserPassword(t *testing.T) {
	password, err := getAgentUserPasswordFromLSA()
	assert.NoError(t, err)
	fmt.Println("password: ", password)
}

func TestValidate(t *testing.T) {
	disableProcessContextValidation(t)

	user := getTestAgentUser(t)
	err := ValidateAgentUserRemoteUpdatePrerequisites(user)
	assert.NoError(t, err)
}

func TestAgentUser(t *testing.T) {
	disableProcessContextValidation(t)

	user := getTestAgentUser(t)
	fmt.Println("user: ", user)

	sid, domain, err := lookupSID(user)
	assert.NoError(t, err)
	fmt.Println("domain: ", domain)
	fmt.Println("sid: ", sid.String())

	isLocalAccount, err := IsLocalAccount(sid)
	assert.NoError(t, err)
	fmt.Println("is local account:", isLocalAccount)

	isServiceAccount, err := IsServiceAccount(sid)
	assert.NoError(t, err)
	fmt.Println("is service account:", isServiceAccount)
}
