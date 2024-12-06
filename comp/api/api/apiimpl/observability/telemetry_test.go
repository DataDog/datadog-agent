// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observability

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestTelemetryMiddleware(t *testing.T) {
	testCases := []struct {
		method   string
		path     string
		code     int
		duration time.Duration
	}{
		{
			method:   http.MethodGet,
			path:     "/test/1",
			code:     http.StatusOK,
			duration: 0,
		},
		{
			method:   http.MethodPost,
			path:     "/test/2",
			code:     http.StatusInternalServerError,
			duration: time.Millisecond,
		},
		{
			method:   http.MethodHead,
			path:     "/test/3",
			code:     http.StatusNotFound,
			duration: time.Second,
		},
	}

	serverName := "test"
	for _, tc := range testCases {
		testName := fmt.Sprintf("%s %s %d %s", tc.method, tc.path, tc.code, tc.duration)
		t.Run(testName, func(t *testing.T) {
			clock := clock.NewMock()
			telemetry := fxutil.Test[telemetry.Mock](t, telemetryimpl.MockModule())
			tm := newTelemetryMiddlewareFactory(telemetry, clock)
			telemetryHandler := tm.Middleware(serverName)

			var tcHandler http.HandlerFunc = func(w http.ResponseWriter, _ *http.Request) {
				clock.Add(tc.duration)
				w.WriteHeader(tc.code)
			}
			server := httptest.NewServer(telemetryHandler(tcHandler))
			defer server.Close()

			url := url.URL{
				Scheme: "http",
				Host:   server.Listener.Addr().String(),
				Path:   tc.path,
			}

			req, err := http.NewRequest(tc.method, url.String(), nil)
			require.NoError(t, err)

			resp, err := server.Client().Do(req)
			require.NoError(t, err)
			resp.Body.Close()

			observabilityMetric, err := telemetry.GetHistogramMetric(MetricSubsystem, MetricName)
			require.NoError(t, err)

			require.Len(t, observabilityMetric, 1)

			metric := observabilityMetric[0]
			assert.EqualValues(t, tc.duration.Seconds(), metric.Value())

			labels := metric.Tags()

			expected := map[string]string{
				"servername":  serverName,
				"status_code": strconv.Itoa(tc.code),
				"method":      tc.method,
				"path":        tc.path,
			}
			assert.Equal(t, expected, labels)
		})
	}
}

func TestTelemetryMiddlewareDuration(t *testing.T) {
	telemetry := fxutil.Test[telemetry.Mock](t, telemetryimpl.MockModule())
	telemetryHandler := NewTelemetryMiddlewareFactory(telemetry).Middleware("test")

	var tcHandler http.HandlerFunc = func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}
	server := httptest.NewServer(telemetryHandler(tcHandler))
	defer server.Close()

	start := time.Now()

	resp, err := server.Client().Get(server.URL)
	require.NoError(t, err)
	resp.Body.Close()

	duration := time.Since(start).Milliseconds()
	require.LessOrEqual(t, duration, 100*time.Millisecond)
}

func TestTelemetryMiddlewareTwice(t *testing.T) {
	telemetry := fxutil.Test[telemetry.Mock](t, telemetryimpl.MockModule())
	tm := NewTelemetryMiddlewareFactory(telemetry)

	// test that we can create multiple middleware instances
	// Prometheus metrics can be registered only once, this test enforces that the metric
	// is not created in the Middleware itself
	_ = tm.Middleware("test1")
	_ = tm.Middleware("test2")
}
