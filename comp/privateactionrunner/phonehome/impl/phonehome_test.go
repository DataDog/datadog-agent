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
	"github.com/stretchr/testify/require"
)

type lifecycle struct{ hooks []compdef.Hook }

func (l *lifecycle) Append(h compdef.Hook) { l.hooks = append(l.hooks, h) }

func TestLifecycleDoesNotWaitForPrerequisites(t *testing.T) {
	t.Setenv("DD_PAR_PHONE_HOME_POC", "true")
	cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{"private_action_runner.enabled": true})
	lc := &lifecycle{}
	require.NotNil(t, NewComponent(Requires{Config: cfg, Lifecycle: lc}).Comp)
	if runtime.GOOS == "windows" {
		require.Empty(t, lc.hooks)
		return
	}
	require.Len(t, lc.hooks, 1)
	require.NoError(t, lc.hooks[0].OnStart(context.Background()))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, lc.hooks[0].OnStop(ctx))
}

func TestOptOutDoesNotRegisterBackgroundWork(t *testing.T) {
	t.Setenv("DD_PAR_PHONE_HOME_POC", "false")
	lc := &lifecycle{}
	cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{"private_action_runner.enabled": true})
	component := NewComponent(Requires{Config: cfg, Lifecycle: lc}).Comp
	require.NotNil(t, component)
	require.Nil(t, component.Status())
	require.Empty(t, lc.hooks)
}
