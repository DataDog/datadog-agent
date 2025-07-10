// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package configsyncimpl

import (
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/configsync"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestOptionalModule(t *testing.T) {
	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"key1": "value1"}`))
	}

	ipcComp := ipcmock.New(t)
	server := ipcComp.NewMockServer(http.HandlerFunc(handler))

	url, err := url.Parse(server.URL)
	require.NoError(t, err)

	host, port, err := net.SplitHostPort(url.Host)
	require.NoError(t, err)

	var cfg config.Component
	overrides := map[string]interface{}{
		"agent_ipc.host":                    host,
		"agent_ipc.port":                    port,
		"agent_ipc.config_refresh_interval": 1,
	}
	comp := fxutil.Test[configsync.Component](t, fx.Options(
		config.MockModule(),
		fx.Supply(log.Params{}),
		fx.Provide(func(t testing.TB) log.Component { return logmock.New(t) }),
		telemetryimpl.MockModule(),
		fx.Provide(func() ipc.Component { return ipcComp }),
		fx.Provide(func(ipcComp ipc.Component) ipc.HTTPClient { return ipcComp.GetClient() }),
		Module(Params{}),
		fx.Populate(&cfg),
		fx.Replace(config.MockParams{Overrides: overrides}),
	))
	require.True(t, comp.(configSync).enabled)

	require.EventuallyWithT(t, func(t *assert.CollectT) {
		assert.Equal(t, "value1", cfg.Get("key1"))
	}, 5*time.Second, 500*time.Millisecond)
}
