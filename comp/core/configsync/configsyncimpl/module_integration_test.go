// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package configsyncimpl

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/api/authtoken/fetchonlyimpl"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/configsync"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

func TestOptionalModule(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"key1": "value1"}`))
	}
	server := httptest.NewTLSServer(http.HandlerFunc(handler))
	t.Cleanup(server.Close)

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
	csopt := fxutil.Test[optional.Option[configsync.Component]](t, fx.Options(
		core.MockBundle(),
		fetchonlyimpl.Module(),
		OptionalModule(),
		fx.Populate(&cfg),
		fx.Replace(config.MockParams{Overrides: overrides}),
	))

	_, ok := csopt.Get()
	require.True(t, ok)

	require.EventuallyWithT(t, func(t *assert.CollectT) {
		assert.Equal(t, "value1", cfg.Get("key1"))
	}, 5*time.Second, 500*time.Millisecond)
}
