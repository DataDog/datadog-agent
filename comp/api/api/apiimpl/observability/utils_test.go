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
	"github.com/gorilla/mux"
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

func TestCaptureRouteTemplateMiddleware(t *testing.T) {
	testCases := []struct {
		routeTemplate string
		requestPath   string
		expected      string
	}{
		{"/test", "/test", "/test"},
		{"/test/", "/test/", "/test/"},
		// Path parameters are returned as the template, not the concrete values.
		{"/test/{id}", "/test/1", "/test/{id}"},
		{"/test/{id}", "/test/2?arg=value", "/test/{id}"},
	}

	for _, tc := range testCases {
		t.Run(tc.requestPath, func(t *testing.T) {
			// Plant a capture in the context (normally done by telemetry middleware)
			var capturedTemplate string
			router := mux.NewRouter()
			router.Use(CaptureRouteTemplateMiddleware)
			router.HandleFunc(tc.routeTemplate, func(_ http.ResponseWriter, r *http.Request) {
				if capture, ok := r.Context().Value(routeCaptureKey{}).(*routeCapture); ok {
					capturedTemplate = capture.template
				}
			})

			r, capture := withRouteCapture(httptest.NewRequest("GET", "http://agent.host"+tc.requestPath, nil))
			router.ServeHTTP(httptest.NewRecorder(), r)

			assert.Equal(t, tc.expected, capture.template)
			assert.Equal(t, tc.expected, capturedTemplate)
		})
	}

	t.Run("with strip prefix - prefix is prepended to template", func(t *testing.T) {
		// Simulate http.StripPrefix("/agent", router): the router only sees the path after the prefix,
		// so the captured template must include the prefix to reflect the full request path.
		router := mux.NewRouter()
		router.Use(CaptureRouteTemplateMiddlewareWithPrefix("/agent"))
		router.HandleFunc("/{component}/status", func(_ http.ResponseWriter, _ *http.Request) {})

		// Mimic what http.StripPrefix does: strip "/agent" before handing off to the router.
		strippedReq := httptest.NewRequest("GET", "http://agent.host/trace-agent/status", nil)
		r, capture := withRouteCapture(strippedReq)
		router.ServeHTTP(httptest.NewRecorder(), r)

		assert.Equal(t, "/agent/{component}/status", capture.template)
	})

	t.Run("no matching route leaves capture empty", func(t *testing.T) {
		router := mux.NewRouter()
		router.Use(CaptureRouteTemplateMiddleware)
		r, capture := withRouteCapture(httptest.NewRequest("GET", "http://agent.host/no-such-route", nil))
		router.ServeHTTP(httptest.NewRecorder(), r)
		assert.Equal(t, "", capture.template)
	})

	t.Run("no capture in context is a no-op", func(t *testing.T) {
		// CaptureRouteTemplateMiddleware is safe to call without a capture in context
		router := mux.NewRouter()
		router.Use(CaptureRouteTemplateMiddleware)
		router.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		req := httptest.NewRequest("GET", "http://agent.host/test", nil)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
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
