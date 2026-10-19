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
	hostname "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/stretchr/testify/require"
)

type lifecycle struct{ hooks []compdef.Hook }

func (l *lifecycle) Append(h compdef.Hook) { l.hooks = append(l.hooks, h) }

type waitingHostname struct {
	hostname.Component
	entered, done chan struct{}
}

func (h *waitingHostname) Get(ctx context.Context) (string, error) {
	close(h.entered)
	<-ctx.Done()
	close(h.done)
	return "", ctx.Err()
}

func TestLifecycleDoesNotWaitForPrerequisites(t *testing.T) {
	t.Setenv("DD_PAR_PHONE_HOME_POC", "true")
	cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{"private_action_runner.enabled": true})
	lc := &lifecycle{}
	host := &waitingHostname{entered: make(chan struct{}), done: make(chan struct{})}
	result := NewComponent(Requires{Config: cfg, Lifecycle: lc, Hostname: host, Log: logmock.New(t)})
	require.NotNil(t, result.Comp)
	if runtime.GOOS == "windows" {
		require.Empty(t, lc.hooks)
		require.Nil(t, result.Comp.Status())
		return
	}
	require.Len(t, lc.hooks, 1)
	require.NoError(t, lc.hooks[0].OnStart(context.Background()))
	select {
	case <-host.entered:
	case <-time.After(time.Second):
		t.Fatal("hostname worker did not start")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, lc.hooks[0].OnStop(ctx))
	select {
	case <-host.done:
	default:
		t.Fatal("hostname work was not canceled")
	}
}

func TestOptOutDoesNotRegisterBackgroundWork(t *testing.T) {
	t.Setenv("DD_PAR_PHONE_HOME_POC", "false")
	lc := &lifecycle{}
	cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{"private_action_runner.enabled": true})
	result := NewComponent(Requires{Config: cfg, Lifecycle: lc})
	require.Empty(t, lc.hooks)
	require.Nil(t, result.Comp.Status())
}
