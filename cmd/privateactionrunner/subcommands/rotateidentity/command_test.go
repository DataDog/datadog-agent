// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package rotateidentity

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/privateactionrunner/command"
	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/enrollment"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestRotateIdentityCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"rotate-identity"},
		run,
		func() {})
}

func TestRotateIdentityAlwaysEnrolls(t *testing.T) {
	cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
		"private_action_runner.enabled": true,
	})
	hostnameComp, _ := hostnamemock.NewMock("test-host")
	calls := 0

	err := rotateIdentity(context.Background(), logmock.New(t), cfg, hostnameComp, func(_ context.Context, _ log.Component, _ coreconfig.Component, agentID *enrollment.AgentIdentifier) (*enrollment.Result, error) {
		calls++
		assert.Equal(t, "test-host", agentID.Hostname)
		return &enrollment.Result{URN: "urn:dd:apps:on-prem-runner:us1:123:runner"}, nil
	})

	require.NoError(t, err)
	assert.Equal(t, 1, calls)
}

func TestRun_DisabledPAR(t *testing.T) {
	cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
		"private_action_runner.enabled": false,
	})
	hostnameComp, _ := hostnamemock.NewMock("test-host")

	err := run(logmock.New(t), cfg, hostnameComp)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "private_action_runner.enabled is false")
}
