// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package rotateidentity

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/privateactionrunner/command"
	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname"
	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestRotateIdentityCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"rotate-identity"},
		run,
		func() {})
}

type failingHostname struct {
	hostname.Component
	calls int
	err   error
}

func (h *failingHostname) Get(context.Context) (string, error) {
	h.calls++
	return "", h.err
}

func TestRun_ExplicitRotationRespectsOptOut(t *testing.T) {
	for _, source := range []model.Source{model.SourceDefault, model.SourceFile, model.SourceEnvVar, model.SourceFleetPolicies, model.SourceCLI} {
		for _, enabled := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/enabled=%t", source, enabled), func(t *testing.T) {
				if source == model.SourceEnvVar {
					t.Setenv("DD_PRIVATE_ACTION_RUNNER_ENABLED", fmt.Sprint(enabled))
				}
				cfg := coreconfig.NewMock(t)
				if source != model.SourceEnvVar {
					cfg.Set("private_action_runner.enabled", enabled, source)
				}
				// Stop before enrollment or storage; this test must never rotate real credentials.
				host := &failingHostname{err: errors.New("hostname unavailable")}
				err := run(logmock.New(t), cfg, host)
				if source != model.SourceDefault && !enabled {
					require.ErrorContains(t, err, "private_action_runner.enabled is false")
					require.Zero(t, host.calls)
				} else {
					require.ErrorIs(t, err, host.err)
					require.Equal(t, 1, host.calls)
				}
			})
		}
	}
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
