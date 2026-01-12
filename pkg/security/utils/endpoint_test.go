// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"

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
