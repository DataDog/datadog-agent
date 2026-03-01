// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package configsyncimpl

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// testLifecycle is a simple lifecycle implementation for testing
type testLifecycle struct {
	hooks []compdef.Hook
}

func (l *testLifecycle) Append(h compdef.Hook) {
	l.hooks = append(l.hooks, h)
}

func (l *testLifecycle) Start(ctx context.Context) error {
	for _, h := range l.hooks {
		if h.OnStart != nil {
			if err := h.OnStart(ctx); err != nil {
				return err
			}
		}
	}
	return nil
}

func (l *testLifecycle) Stop(ctx context.Context) error {
	for i := len(l.hooks) - 1; i >= 0; i-- {
		if l.hooks[i].OnStop != nil {
			if err := l.hooks[i].OnStop(ctx); err != nil {
				return err
			}
		}
	}
	return nil
}

func TestOptionalModule(t *testing.T) {
	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"api_key": "value1"}`))
	}

	ipcComp := ipcmock.New(t)
	server := ipcComp.NewMockServer(http.HandlerFunc(handler))

	url, err := url.Parse(server.URL)
	require.NoError(t, err)

	host, port, err := net.SplitHostPort(url.Host)
	require.NoError(t, err)

	// Use makeDeps to build base dependencies, then override config with specific values
	deps := makeDeps(t)
	deps.Config.Set("agent_ipc.host", host, pkgconfigmodel.SourceFile)
	deps.Config.Set("agent_ipc.port", port, pkgconfigmodel.SourceFile)
	deps.Config.Set("agent_ipc.config_refresh_interval", 1, pkgconfigmodel.SourceFile)
	deps.IPCClient = ipcComp.GetClient()

	// Override lifecycle with our test implementation
	lc := &testLifecycle{}
	deps.Lc = lc

	provides, err := NewComponent(deps)
	require.NoError(t, err)
	require.True(t, provides.Comp.(configSync).enabled)

	// Start the component hooks
	require.NoError(t, lc.Start(context.Background()))
	t.Cleanup(func() { _ = lc.Stop(context.Background()) })

	var cfg config.Component = deps.Config
	require.EventuallyWithT(t, func(t *assert.CollectT) {
		assert.Equal(t, "value1", cfg.Get("api_key"))
	}, 5*time.Second, 500*time.Millisecond)
}
