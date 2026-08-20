// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
)

func TestGetEndpointURL(t *testing.T) {
	endpoint := config.NewEndpoint("key", "keyPath", "localhost", 8080, "/prefix/url", false)
	url := GetEndpointURL(endpoint, "test")
	assert.Equal(t, "http://localhost:8080/prefix/url/test", url)
}

func TestGetEndpointURLSSL(t *testing.T) {
	endpoint := config.NewEndpoint("key", "keyPath", "localhost", 8080, "/prefix/url", true)
	url := GetEndpointURL(endpoint, "test")
	assert.Equal(t, "https://localhost:8080/prefix/url/test", url)
}

func TestGetEndpointURLHostPort(t *testing.T) {
	endpoint := config.NewEndpoint("key", "keyPath", "localhost", 8080, config.EmptyPathPrefix, false)
	url := GetEndpointURL(endpoint, "test")
	assert.Equal(t, "http://localhost:8080/test", url)
}

func TestGetEndpointURLHostOnlySSL(t *testing.T) {
	endpoint := config.NewEndpoint("key", "keyPath", "localhost", 0, config.EmptyPathPrefix, true)
	url := GetEndpointURL(endpoint, "test")
	assert.Equal(t, "https://localhost:443/test", url)
}

func TestGetEndpointURLHostOnlyNoSSL(t *testing.T) {
	endpoint := config.NewEndpoint("key", "keyPath", "localhost", 0, config.EmptyPathPrefix, false)
	url := GetEndpointURL(endpoint, "test")
	assert.Equal(t, "http://localhost:80/test", url)
}

func TestGetEndpointURLIPv6(t *testing.T) {
	endpoint := config.NewEndpoint("key", "keyPath", "fd38::1", 8080, "/prefix/url", false)
	url := GetEndpointURL(endpoint, "test")
	assert.Equal(t, "http://[fd38::1]:8080/prefix/url/test", url)
}

func TestGetEndpointURLIPv6Bracketed(t *testing.T) {
	endpoint := config.NewEndpoint("key", "keyPath", "[fd38::1]", 8080, "/prefix/url", false)
	url := GetEndpointURL(endpoint, "test")
	assert.Equal(t, "http://[fd38::1]:8080/prefix/url/test", url)
}

// TestGetEndpointURLHostWithEmbeddedPort mirrors chart-generated `additional_endpoints`
// entries where the port is baked into the host field ("cws-intake.<site>.:443") and no
// separate port is set. The embedded port must be honored instead of double-appended.
func TestGetEndpointURLHostWithEmbeddedPort(t *testing.T) {
	endpoint := config.NewEndpoint("key", "keyPath", "cws-intake.datadoghq.com.:443", 0, config.EmptyPathPrefix, true)
	url := GetEndpointURL(endpoint, "api/v2/secdump")
	assert.Equal(t, "https://cws-intake.datadoghq.com.:443/api/v2/secdump", url)

	// The prod failure was http.NewRequest rejecting the malformed
	// "https://[cws-intake.datadoghq.com.:443]:443/..." URL. Assert the produced URL
	// is actually accepted by the same call the forwarder makes.
	_, err := http.NewRequest("POST", url, nil)
	require.NoError(t, err)
}

// TestGetEndpointURLHostWithEmbeddedPortExplicitWins ensures an explicitly configured
// port takes precedence over a port embedded in the host string.
func TestGetEndpointURLHostWithEmbeddedPortExplicitWins(t *testing.T) {
	endpoint := config.NewEndpoint("key", "keyPath", "cws-intake.datadoghq.com.:443", 8443, config.EmptyPathPrefix, true)
	url := GetEndpointURL(endpoint, "api/v2/secdump")
	assert.Equal(t, "https://cws-intake.datadoghq.com.:8443/api/v2/secdump", url)
}

func TestGetEndpointURLIPv6WithEmbeddedPort(t *testing.T) {
	endpoint := config.NewEndpoint("key", "keyPath", "[fd38::1]:8080", 0, config.EmptyPathPrefix, false)
	url := GetEndpointURL(endpoint, "test")
	assert.Equal(t, "http://[fd38::1]:8080/test", url)
}
