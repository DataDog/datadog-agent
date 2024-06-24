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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogResponseHandlerCallsNext(t *testing.T) {
	var testStatusCodes = []int{
		http.StatusContinue,
		http.StatusOK,
		http.StatusMovedPermanently,
		http.StatusBadRequest,
		http.StatusInternalServerError,
	}

	for _, code := range testStatusCodes {
		name := http.StatusText(code)
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "http://agent.host/test/", nil)

			rr := httptest.NewRecorder()
			nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(code)
			})

			handler := LogResponseHandler("TestServer")(nextHandler)
			handler.ServeHTTP(rr, req)

			assert.Equal(t, code, rr.Code)
		})
	}
}

func TestLogResponseHandlerLogging(t *testing.T) {
	serverName := "TestServer"

	testCases := []struct {
		statusCode  int
		method      string
		url         string
		remoteAddr  string
		duration    time.Duration
		stripPrefix string
	}{
		{
			statusCode: http.StatusContinue,
			method:     "GET",
			url:        "http://agent.host/test/",
			duration:   time.Nanosecond,
			remoteAddr: "myhost:1234",
		},
		{
			statusCode: http.StatusOK,
			method:     "POST",
			url:        "http://agent.host:8080/test/2",
			duration:   time.Microsecond,
			remoteAddr: "myotherhost:12345",
		},
		{
			statusCode:  http.StatusMovedPermanently,
			method:      "PUT",
			url:         "https://127.0.0.1/test/3",
			duration:    time.Millisecond,
			remoteAddr:  "anotherhost",
			stripPrefix: "/test",
		},
		{
			statusCode: http.StatusBadRequest,
			method:     "DELETE",
			url:        "http://127.0.0.1/test/4?myvalue=0&mysecret=qwertyuiop&myothervalue=1",
			duration:   time.Second,
			remoteAddr: "yetanotherhost",
		},
		{
			statusCode:  http.StatusInternalServerError,
			method:      "PATCH",
			url:         "https://localhost/test/5?secret=1234567890",
			duration:    500 * time.Millisecond,
			remoteAddr:  "lasthost",
			stripPrefix: "/test",
		},
	}

	for _, tt := range testCases {
		ttURL, err := url.Parse(tt.url)
		require.NoError(t, err)

		name := fmt.Sprintf(logFormat, serverName, tt.method, ttURL.Path, tt.remoteAddr, ttURL.Host, tt.duration, tt.statusCode)
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.url, nil)
			req.RemoteAddr = tt.remoteAddr

			var getLogFuncCalls int
			getLogFunc := func(int) logFunc {
				return func(format string, args ...interface{}) {
					getLogFuncCalls++

					require.Equal(t, 7, len(args))

					assert.EqualValues(t, serverName, args[0])
					assert.EqualValues(t, tt.method, args[1])
					assert.EqualValues(t, ttURL.Path, args[2])
					assert.EqualValues(t, tt.remoteAddr, args[3])
					assert.EqualValues(t, ttURL.Host, args[4])
					assert.LessOrEqual(t, tt.duration, args[5])
					assert.EqualValues(t, tt.statusCode, args[6])
				}
			}

			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(tt.duration)
				w.WriteHeader(tt.statusCode)
			})

			logHandler := logResponseHandler(serverName, getLogFunc)
			handler := http.StripPrefix(tt.stripPrefix, logHandler(next))

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			require.Equal(t, 1, getLogFuncCalls)
		})
	}
}
