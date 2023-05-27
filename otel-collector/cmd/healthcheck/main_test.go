package main

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealthStatusHealthy(t *testing.T) {

	server := setUpMockCollector(t, "127.0.0.1:13133", http.StatusOK)
	defer server.Close()
	port := "13133"
	got, err := executeHealthCheck("127.0.0.1", &port, "/")
	expectedErrorString := "STATUS: 200"
	assert.Contains(t, got, expectedErrorString,
		fmt.Sprintf("Unexpected log message. Got %s but should contain %s", got, expectedErrorString))
	assert.NoError(t, err)

}

func TestHealthStatusUnhealthy(t *testing.T) {

	server := setUpMockCollector(t, "127.0.0.1:13133", http.StatusInternalServerError)
	defer server.Close()
	port := "13133"
	got, err := executeHealthCheck("127.0.0.1", &port, "/")
	expectedErrorString := "STATUS: 500"
	assert.Contains(t, err.Error(), expectedErrorString,
		fmt.Sprintf("Unexpected log message. Got %s but should contain %s", got, expectedErrorString))

}

func TestHealthStatusServerDown(t *testing.T) {

	server := setUpMockCollector(t, "127.0.0.1:13132", http.StatusInternalServerError)
	defer server.Close()
	port := "13133"
	got, err := executeHealthCheck("127.0.0.1", &port, "/")
	expectedErrorString := "unable to retrieve health status"
	assert.Contains(t, err.Error(), expectedErrorString,
		fmt.Sprintf("Unexpected log message. Got %s but should contain %s", got, expectedErrorString))

}

func setUpMockCollector(t *testing.T, healthCheckDefaultEndpoint string, statusCode int) *httptest.Server {
	l, err := net.Listen("tcp", healthCheckDefaultEndpoint)
	require.NoError(t, err)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(statusCode)
	}))
	server.Listener.Close()
	server.Listener = l
	server.Start()
	return server
}

func TestValidatePort(t *testing.T) {

	testCases := []struct {
		name           string
		port           string
		errorAssertion assert.ErrorAssertionFunc
	}{
		{
			name:           "WrongString",
			port:           "StringPort",
			errorAssertion: assert.Error,
		},
		{
			name:           "EmptyString",
			port:           "",
			errorAssertion: assert.Error,
		},
		{
			name:           "WrongPort",
			port:           "65536",
			errorAssertion: assert.Error,
		},
		{
			name:           "ValidPort",
			port:           "13133",
			errorAssertion: assert.NoError,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validatePort(tc.port)
			tc.errorAssertion(t, err)
		})
	}

}
