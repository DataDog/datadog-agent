// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"fmt"
	"net/http"
	"net/http/httptest"
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
			req, err := http.NewRequest("GET", "/test", nil)
			require.NoError(t, err)

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
		statusCode int
		method     string
		uri        string
		remoteAddr string
		duration   time.Duration
	}{
		{
			statusCode: http.StatusContinue,
			method:     "GET",
			uri:        "/test/",
			duration:   time.Nanosecond,
			remoteAddr: "myhost:1234",
		},
		{
			statusCode: http.StatusOK,
			method:     "POST",
			uri:        "/test/2",
			duration:   time.Microsecond,
			remoteAddr: "myotherhost:12345",
		},
		{
			statusCode: http.StatusMovedPermanently,
			method:     "PUT",
			uri:        "/test/3",
			duration:   time.Millisecond,
			remoteAddr: "anotherhost",
		},
		{
			statusCode: http.StatusBadRequest,
			method:     "DELETE",
			uri:        "/test/4",
			duration:   time.Second,
			remoteAddr: "yetanotherhost",
		},
		{
			statusCode: http.StatusInternalServerError,
			method:     "PATCH",
			uri:        "/test/5",
			duration:   500 * time.Millisecond,
			remoteAddr: "lasthost",
		},
	}

	for _, tt := range testCases {
		name := fmt.Sprintf(logFormat, serverName, tt.method, tt.uri, tt.remoteAddr, tt.duration, tt.statusCode)
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.uri, nil)
			req.RemoteAddr = tt.remoteAddr
			rr := httptest.NewRecorder()

			var getLogFuncCalls int
			getLogFunc := func(int) logFunc {
				return func(format string, args ...interface{}) {
					getLogFuncCalls++

					require.Equal(t, 6, len(args))

					assert.EqualValues(t, serverName, args[0])
					assert.EqualValues(t, tt.method, args[1])
					assert.EqualValues(t, tt.uri, args[2])
					assert.EqualValues(t, tt.remoteAddr, args[3])
					assert.LessOrEqual(t, tt.duration, args[4])
					assert.EqualValues(t, tt.statusCode, args[5])
				}
			}

			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(tt.duration)
				w.WriteHeader(tt.statusCode)
			})

			middleware := logResponseHandler(serverName, getLogFunc)
			handler := middleware(next)
			handler.ServeHTTP(rr, req)

			assert.Equal(t, 1, getLogFuncCalls)
		})
	}
}
