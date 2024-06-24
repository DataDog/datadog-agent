// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observability

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractPath(t *testing.T) {
	testCases := []struct {
		path     string
		expected string
	}{
		{"/", "/"},
		{"/test", "/test"},
		{"/test/", "/test/"},
		{"/test/1", "/test/1"},
		{"/test/1?arg=value", "/test/1"},
	}

	for _, tc := range testCases {
		t.Run(tc.path, func(t *testing.T) {
			req := httptest.NewRequest("GET", "http://agent.host"+tc.path, nil)
			assert.Equal(t, tc.expected, extractPath(req))
		})
	}

	t.Run("invalid url", func(t *testing.T) {
		req := http.Request{RequestURI: "http://agent.host/invalidurl" + string([]byte{0x7f})}
		assert.Equal(t, "<invalid url>", extractPath(&req))
	})
}

func TestExtractStatusHandler(t *testing.T) {
	// can't test with status code 1xx since they are not final responses
	testCases := []int{
		http.StatusOK, http.StatusMovedPermanently, http.StatusNotFound, http.StatusInternalServerError,
	}

	for _, tcStatus := range testCases {
		t.Run(http.StatusText(tcStatus), func(t *testing.T) {
			var status int
			statusMiddleware := extractStatusHandler(&status)

			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tcStatus)
			})

			server := httptest.NewServer(statusMiddleware(handler))
			defer server.Close()

			resp, err := server.Client().Get(server.URL)
			require.NoError(t, err)
			resp.Body.Close()

			assert.Equal(t, tcStatus, resp.StatusCode)
		})
	}
}

func TestTimeHandler(t *testing.T) {
	testCases := []time.Duration{
		0, time.Nanosecond, time.Microsecond, time.Millisecond, time.Second, time.Minute, time.Hour,
	}

	for _, tcDuration := range testCases {
		clock := clock.NewMock()
		var duration time.Duration
		timeMiddleware := timeHandler(clock, &duration)

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			clock.Add(tcDuration)
			w.WriteHeader(http.StatusOK)
		})

		server := httptest.NewServer(timeMiddleware(handler))
		defer server.Close()

		resp, err := server.Client().Get(server.URL)
		require.NoError(t, err)
		resp.Body.Close()

		assert.Equal(t, tcDuration, duration)
	}
}
