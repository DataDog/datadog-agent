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

func TestWrapWithRouteTemplate(t *testing.T) {
	for _, template := range []string{"/test", "/test/", "/test/{id}"} {
		t.Run(template, func(t *testing.T) {
			var capturedTemplate string
			mux := http.NewServeMux()
			WrapWithRouteTemplate(mux, "GET", template, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				if capture, ok := r.Context().Value(routeCaptureKey{}).(*routeCapture); ok {
					capturedTemplate = capture.template
				}
			}))

			r, capture := withRouteCapture(httptest.NewRequest("GET", "http://agent.host"+template, nil))
			mux.ServeHTTP(httptest.NewRecorder(), r)

			assert.Equal(t, template, capture.template)
			assert.Equal(t, template, capturedTemplate)
		})
	}

	t.Run("no capture in context is a no-op", func(t *testing.T) {
		called := false
		mux := http.NewServeMux()
		WrapWithRouteTemplate(mux, "GET", "/test", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			called = true
			w.WriteHeader(http.StatusOK)
		}))
		req := httptest.NewRequest("GET", "http://agent.host/test", nil)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		assert.True(t, called)
		assert.Equal(t, http.StatusOK, rr.Code)
	})
}

func TestMountWithPrefix(t *testing.T) {
	t.Run("prepends prefix to template set by inner handler", func(t *testing.T) {
		inner := http.NewServeMux()
		WrapWithRouteTemplate(inner, "GET", "/{path}", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		r, capture := withRouteCapture(httptest.NewRequest("GET", "http://host/config/v1/log_level", nil))
		MountWithPrefix("/config/v1", inner).ServeHTTP(httptest.NewRecorder(), r)

		assert.Equal(t, "/config/v1/{path}", capture.template)
	})

	t.Run("no-op when inner sets no template", func(t *testing.T) {
		inner := http.NewServeMux()
		inner.HandleFunc("GET /plain", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

		r, capture := withRouteCapture(httptest.NewRequest("GET", "http://host/prefix/plain", nil))
		MountWithPrefix("/prefix", inner).ServeHTTP(httptest.NewRecorder(), r)

		assert.Equal(t, "", capture.template)
	})
}

func TestExtractStatusHandler(t *testing.T) {
	// can't test with status code 1xx since they are not final responses
	testCases := []int{
		http.StatusOK, http.StatusMovedPermanently, http.StatusNotFound, http.StatusInternalServerError,
	}

	for _, tcStatus := range testCases {
		t.Run(http.StatusText(tcStatus), func(t *testing.T) {
			var statusCode int
			statusMiddleware := extractStatusCodeHandler(&statusCode)

			handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tcStatus)
			})

			server := httptest.NewServer(statusMiddleware(handler))
			defer server.Close()

			resp, err := server.Client().Get(server.URL)
			require.NoError(t, err)
			resp.Body.Close()

			assert.Equal(t, tcStatus, statusCode)
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

		handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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
