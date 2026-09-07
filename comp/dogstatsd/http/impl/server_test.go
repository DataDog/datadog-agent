// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package httpimpl

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	telemetrymock "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	filterlistimpl "github.com/DataDog/datadog-agent/comp/filterlist/impl"
)

// startServer runs a server against the given config overrides and returns it
// once its http.Server has been built.
func startServer(t *testing.T, overrides map[string]interface{}) *server {
	t.Helper()

	overrides["dogstatsd_experimental_http.enabled"] = true
	overrides["dogstatsd_experimental_http.listen_address"] = "127.0.0.1:0"

	hn, _ := hostnamemock.NewMock("my-hostname")
	s := &server{
		config:     config.NewMockWithOverrides(t, overrides),
		log:        logmock.New(t),
		tagger:     newFakeTagger(t),
		hostname:   hn,
		filterList: filterlistimpl.NewNoopFilterList(),
		telemetry:  newTelemetryStore(telemetrymock.New(t)),
		out:        &drainingSerializer{},
	}

	ctx := context.Background()
	require.NoError(t, s.start(ctx))
	t.Cleanup(func() { require.NoError(t, s.stop(ctx)) })

	return s
}

func TestServerReadTimeout(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		s := startServer(t, map[string]interface{}{})
		require.NotZero(t, s.http.ReadTimeout)
	})

	t.Run("config", func(t *testing.T) {
		s := startServer(t, map[string]interface{}{
			"dogstatsd_experimental_http.read_timeout": "2500ms",
		})
		require.Equal(t, 2500*time.Millisecond, s.http.ReadTimeout)
	})

}
