// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package phonehomeimpl

import (
	"context"
	"runtime"
	"testing"
	"time"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/deployment"
	"github.com/stretchr/testify/require"
)

type lifecycle struct{ hooks []compdef.Hook }

func (l *lifecycle) Append(h compdef.Hook) { l.hooks = append(l.hooks, h) }

func TestLifecycleDoesNotWaitForPrerequisites(t *testing.T) {
	cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{"private_action_runner.enabled": true})
	lc := &lifecycle{}
	require.NotNil(t, NewComponent(Requires{Config: cfg, Lifecycle: lc}).Comp)
	mode, err := deployment.Resolve(cfg)
	require.NoError(t, err)
	if runtime.GOOS == "windows" || mode != deployment.Split {
		require.Empty(t, lc.hooks)
		return
	}
	require.Len(t, lc.hooks, 1)
	require.NoError(t, lc.hooks[0].OnStart(context.Background()))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, lc.hooks[0].OnStop(ctx))
}

func TestMonolithicPARDoesNotRegisterBackgroundWork(t *testing.T) {
	for _, disabledSetting := range []string{"process_manager.enabled", "private_action_runner.split_enabled"} {
		t.Run(disabledSetting, func(t *testing.T) {
			cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
				"private_action_runner.enabled":       true,
				"private_action_runner.split_enabled": true,
				"process_manager.enabled":             true,
			})
			cfg.SetInTest(disabledSetting, false)
			lc := &lifecycle{}
			require.NotNil(t, NewComponent(Requires{Config: cfg, Lifecycle: lc}).Comp)
			require.Empty(t, lc.hooks, "monolithic startup must not create RC/AAA background work")
		})
	}
}

func TestDisabledPARDoesNotRegisterBackgroundWork(t *testing.T) {
	lc := &lifecycle{}
	cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{"private_action_runner.enabled": false})
	require.NotNil(t, NewComponent(Requires{Config: cfg, Lifecycle: lc}).Comp)
	require.Empty(t, lc.hooks)
}
