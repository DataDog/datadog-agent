// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package awsimds

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/dmi"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
)

func testDeps(hostname string) issues.ModuleDeps {
	hn, _ := hostnamemock.NewMock(hostnamemock.MockHostname(hostname))
	return issues.ModuleDeps{Hostname: hn}
}

// testModule builds the module directly, bypassing NewModule's environment gate.
func testModule(hostname string) *awsIMDSModule {
	return newModule(testDeps(hostname))
}

func TestInstanceIssueID(t *testing.T) {
	m := testModule("host-a")
	id := m.instanceIssueID()
	assert.True(t, strings.HasPrefix(id, IssueID+":"))
	assert.Len(t, id, len(IssueID)+1+16)
	assert.Equal(t, id, testModule("host-a").instanceIssueID())
	assert.NotEqual(t, id, testModule("host-b").instanceIssueID())
}

// TestNewModule_RegistrationGate verifies the module registers only in a container on AWS.
func TestNewModule_RegistrationGate(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("ec2_use_dmi", true)
	deps := testDeps("host")

	t.Run("aws and containerized registers", func(t *testing.T) {
		t.Setenv("DOCKER_DD_AGENT", "true")
		dmi.SetupMock(t, "", "", "", ec2.DMIBoardVendor)
		assert.NotNil(t, NewModule(deps))
	})

	t.Run("not containerized declines", func(t *testing.T) {
		t.Setenv("DOCKER_DD_AGENT", "")
		dmi.SetupMock(t, "", "", "", ec2.DMIBoardVendor)
		assert.Nil(t, NewModule(deps))
	})

	t.Run("not aws declines", func(t *testing.T) {
		t.Setenv("DOCKER_DD_AGENT", "true")
		dmi.SetupMock(t, "", "", "", "not AWS")
		assert.Nil(t, NewModule(deps))
	})
}
