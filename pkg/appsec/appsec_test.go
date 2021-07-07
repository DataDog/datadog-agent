package appsec

import (
	"errors"
	"github.com/DataDog/datadog-agent/pkg/trace/api/apiutil"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
	"github.com/DataDog/datadog-agent/pkg/trace/test/testutil"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"testing"
)

func TestMetrics(t *testing.T) {
	t.Run("tags", func(t *testing.T) {
		for _, tc := range []struct {
			name         string
			request      http.Request
			expectedTags []string
		}{
			{
				name: "path only",
				request: http.Request{
					URL: &url.URL{
						Path: "/some/endpoint",
					},
				},
				expectedTags: []string{"path:/some/endpoint"},
			},
			{
				name: "path and content_type",
				request: http.Request{
					Header: map[string][]string{
						"Content-Type": {"application/json"},
					},
					URL: &url.URL{
						Path: "/some/endpoint",
					},
				},
				expectedTags: []string{"path:/some/endpoint", "content_type:application/json"},
			},
			{
				name: "path and payload_size",
				request: http.Request{
					URL: &url.URL{
						Path: "/some/endpoint",
					},
					Body: &apiutil.LimitedReader{Count: 1073741824},
				},
				expectedTags: []string{"path:/some/endpoint", "payload_size:1073741824"},
			},
			{
				name: "path, content_type and payload_size",
				request: http.Request{
					Header: map[string][]string{
						"Content-Type": {"application/json"},
					},
					URL: &url.URL{
						Path: "/some/endpoint",
					},
					Body: &apiutil.LimitedReader{Count: 1025},
				},
				expectedTags: []string{"path:/some/endpoint", "content_type:application/json", "payload_size:1025"},
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				require.Equal(t, tc.expectedTags, metricsTags(&tc.request))
			})
		}
	})

	t.Run("proxy metrics", func(t *testing.T) {
		stats := &testutil.TestStatsClient{}
		defer func(old metrics.StatsClient) { metrics.Client = old }(metrics.Client)
		metrics.Client = stats

		// Wrap the proxy with metrics
		proxy := &httputil.ReverseProxy{
			Director:     func(*http.Request) {},
			ErrorHandler: func(http.ResponseWriter, *http.Request, error) {},
		}
		handler := withMetrics(proxy)

		// Serve a fake request having everything we monitor
		req := &http.Request{
			Header: map[string][]string{
				"Content-Type": {"application/json"},
			},
			URL: &url.URL{
				Path: "/some/endpoint",
			},
			Body: &apiutil.LimitedReader{Count: 42025},
		}
		handler.ServeHTTP(httptest.NewRecorder(), req)

		tags := metricsTags(req)
		calls := stats.HistogramCalls
		require.Len(t, calls, 1)
		require.Equal(t, testutil.MetricsArgs{
			Name:  appSecRequestPayloadSizeMetricsID,
			Value: 42025,
			Tags:  tags,
			Rate:  1,
		}, calls[0])

		calls = stats.GaugeCalls
		require.Len(t, calls, 1)
		require.Equal(t, testutil.MetricsArgs{
			Name:  appSecRequestCountMetricsID,
			Value: 1,
			Tags:  tags,
			Rate:  1,
		}, calls[0])

		calls = stats.TimingCalls
		require.Len(t, calls, 1)
		require.Equal(t, appSecRequestDurationMetricsID, calls[0].Name)
		require.Equal(t, tags, calls[0].Tags)
		require.Equal(t, float64(1), calls[0].Rate)
		require.NotZero(t, calls[0].Value)

		// Test the proxy error handler with an error that is not monitored
		proxy.ErrorHandler(httptest.NewRecorder(), req, errors.New("an error occured"))
		calls = stats.CountCalls
		require.Len(t, calls, 0)

		// Test the proxy error handler with an error that is monitored
		proxy.ErrorHandler(httptest.NewRecorder(), req, apiutil.ErrLimitedReaderLimitReached)
		calls = stats.CountCalls
		require.Len(t, calls, 1)
		require.Equal(t, float64(1), calls[0].Value)
	})
}
