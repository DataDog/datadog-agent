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
	configsync "github.com/DataDog/datadog-agent/comp/core/configsync/def"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
)

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

	overrides := map[string]interface{}{
		"agent_ipc.host":                    host,
		"agent_ipc.port":                    port,
		"agent_ipc.config_refresh_interval": 1,
	}
	cfg := config.NewMockWithOverrides(t, overrides)

	lc := compdef.NewTestLifecycle(t)
	comp, err := NewComponent(Requires{
		Lc:         lc,
		Config:     cfg,
		Log:        logmock.New(t),
		IPCClient:  ipcComp.GetClient(),
		SyncParams: configsync.Params{},
	})
	require.NoError(t, err)
	require.True(t, comp.(configSync).enabled)

	ctx := context.Background()
	require.NoError(t, lc.Start(ctx))
	t.Cleanup(func() { lc.Stop(ctx) })

	require.EventuallyWithT(t, func(t *assert.CollectT) {
		assert.Equal(t, "value1", cfg.Get("api_key"))
	}, 5*time.Second, 500*time.Millisecond)
}
