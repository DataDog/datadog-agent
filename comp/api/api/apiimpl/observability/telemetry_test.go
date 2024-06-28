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
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	dto "github.com/prometheus/client_model/go"
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
			registry := telemetry.GetRegistry()

			var tcHandler http.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
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

			metricFamilies, err := registry.Gather()
			require.NoError(t, err)

			expectedMetricName := fmt.Sprintf("%s__%s", MetricSubsystem, MetricName)
			idx := slices.IndexFunc(metricFamilies, func(e *dto.MetricFamily) bool {
				return e.GetName() == expectedMetricName
			})
			require.NotEqual(t, -1, idx, "API telemetry metric not found")

			telemetryMetricFamily := metricFamilies[idx]
			require.Equal(t, dto.MetricType_HISTOGRAM, telemetryMetricFamily.GetType())

			metrics := telemetryMetricFamily.GetMetric()
			require.Len(t, metrics, 1)

			metric := metrics[0]
			histogram := metric.GetHistogram()
			assert.EqualValues(t, 1, histogram.GetSampleCount())
			assert.EqualValues(t, tc.duration.Seconds(), histogram.GetSampleSum())

			labels := metric.GetLabel()
			// labels are not necessarily in the order they were declared
			// so we use a map to compare them
			labelMap := make(map[string]string, len(labels))
			for _, label := range labels {
				labelMap[label.GetName()] = label.GetValue()
			}

			expected := map[string]string{
				"servername":  serverName,
				"status_code": strconv.Itoa(tc.code),
				"method":      tc.method,
				"path":        tc.path,
			}
			assert.Equal(t, expected, labelMap)
		})
	}
}

func TestTelemetryMiddlewareDuration(t *testing.T) {
	telemetry := fxutil.Test[telemetry.Mock](t, telemetryimpl.MockModule())
	telemetryHandler := NewTelemetryMiddlewareFactory(telemetry).Middleware("test")

	var tcHandler http.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
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
