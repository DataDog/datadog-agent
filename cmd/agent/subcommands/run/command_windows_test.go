// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package run

import (
	"context"
	"testing"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	configsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
)

func TestStartAgentWithDefaults(t *testing.T) {
	fxutil.TestOneShot(t,
		func() {
			ctxChan := make(<-chan context.Context)
			_, err := StartAgentWithDefaults(ctxChan)
			require.NoError(t, err)
		})
}

func TestWindowsServiceRejectsPreparedRolloutBeforeStart(t *testing.T) {
	cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
		configsetup.ExperimentalNodeAgentRolloutEnabled: true,
	})
	started := false
	app := fx.New(
		fx.Provide(func() coreconfig.Component { return cfg }),
		fx.Invoke(validateWindowsPreparedRollout),
		fx.Invoke(func(lifecycle fx.Lifecycle) {
			lifecycle.Append(fx.Hook{OnStart: func(context.Context) error {
				started = true
				return nil
			}})
		}),
	)
	require.ErrorContains(t, app.Err(), "not supported")
	require.False(t, started)
}
