// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package client

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAuth(t *testing.T) {
	timeNow = mockTimeNow

	mux := http.NewServeMux()

	loginHandler := newHandler(func(w http.ResponseWriter, r *http.Request, calls int32) {
		r.ParseForm()

		username := r.Form.Get("j_username")
		password := r.Form.Get("j_password")

		require.Equal(t, "username", username)
		require.Equal(t, "password", password)
		w.WriteHeader(http.StatusOK)
	})

	tokenHandler := newHandler(func(w http.ResponseWriter, r *http.Request, calls int32) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("testtoken"))
	})

	endpointHandler := newHandler(func(w http.ResponseWriter, r *http.Request, calls int32) {
		token := r.Header.Get("X-XSRF-TOKEN")

		// Ensure token is correctly passed as header
		require.Equal(t, "testtoken", token)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("{}"))
	})

	mux.HandleFunc("/j_security_check", loginHandler.Func)
	mux.HandleFunc("/dataservice/client/token", tokenHandler.Func)
	mux.HandleFunc("/dataservice/device", endpointHandler.Func)

	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := NewClient(serverURL(server), "username", "password", true)
	require.NoError(t, err)

	_, err = client.GetDevices()
	require.NoError(t, err)

	require.Equal(t, "testtoken", client.token, "Token should be set correctly")
	require.Equal(t, int64(946688400), client.tokenExpiry.Unix(), "Token expiry should be set correctly")

	// Ensure login endpoint has been called 1 times
	require.Equal(t, 1, loginHandler.numberOfCalls())
	// Ensure token endpoint has been called 1 times
	require.Equal(t, 1, loginHandler.numberOfCalls())

	// Re-call GetDevices and ensure auth is not re-called
	_, err = client.GetDevices()
	require.NoError(t, err)

	// Ensure login endpoint has been called 1 times
	require.Equal(t, 1, loginHandler.numberOfCalls())
	// Ensure token endpoint has been called 1 times
	require.Equal(t, 1, loginHandler.numberOfCalls())

	// Fast-forward 1h01 and ensure token is correctly expired
	timeNow = func() time.Time {
		return mockTimeNow().Add(time.Hour + time.Minute)
	}

	// Re-call GetDevices and ensure auth is re-called
	_, err = client.GetDevices()
	require.NoError(t, err)

	// Ensure login endpoint has been called 2 times
	require.Equal(t, 2, loginHandler.numberOfCalls())
	// Ensure token endpoint has been called 2 times
	require.Equal(t, 2, loginHandler.numberOfCalls())
}

func TestClearAuth(t *testing.T) {
	client, err := NewClient("test", "testuser", "testpass", false)
	require.NoError(t, err)

	client.token = "testtoken"
	client.clearAuth()
	require.Equal(t, "", client.token)
}

func TestIsAuthenticated(t *testing.T) {
	tests := []struct {
		headers  map[string]string
		expected bool
	}{
		{
			headers:  map[string]string{"content-type": "text/html"},
			expected: false,
		},
		{
			headers: map[string]string{
				"content-type": "text/html;charset=UTF-8",
			},
			expected: false,
		},
		{
			headers: map[string]string{
				"content-type": "application/json",
			},
			expected: true,
		},
		{
			headers:  map[string]string{},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			headers := make(http.Header)
			for key, value := range tt.headers {
				headers.Add(key, value)
			}
			require.Equal(t, tt.expected, isAuthenticated(headers))
		})
	}
}
