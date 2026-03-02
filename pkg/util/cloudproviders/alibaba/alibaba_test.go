// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package alibaba

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestGetHostname(t *testing.T) {
	ctx := context.Background()
	expected := "i-rj9aql2pwopjn4sm24ix"
	var lastRequest *http.Request
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, expected)
		lastRequest = r
	}))
	defer ts.Close()
	metadataURL = ts.URL

	aliases, err := GetHostAliases(ctx)
	assert.NoError(t, err)
	require.Len(t, aliases, 1)
	assert.Equal(t, expected, aliases[0])
	assert.Equal(t, lastRequest.URL.Path, "/latest/meta-data/instance-id")
}

func TestGetNTPHosts(t *testing.T) {
	cfg := configmock.New(t)
	ctx := context.Background()
	expectedHosts := []string{
		"ntp.aliyun.com", "ntp1.aliyun.com", "ntp2.aliyun.com", "ntp3.aliyun.com",
		"ntp4.aliyun.com", "ntp5.aliyun.com", "ntp6.aliyun.com", "ntp7.aliyun.com",
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, "test")
	}))
	defer ts.Close()

	metadataURL = ts.URL
	cfg.SetWithoutSource("cloud_provider_metadata", []string{"alibaba"})
	actualHosts := GetNTPHosts(ctx)

	assert.Equal(t, expectedHosts, actualHosts)
}

func TestIsRunningOnNotRunning(t *testing.T) {
	cfg := configmock.New(t)
	ctx := context.Background()
	// Point to a server that returns an error
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	metadataURL = ts.URL
	cfg.SetWithoutSource("cloud_provider_metadata", []string{"alibaba"})
	// Reset the fetcher to force re-fetch
	instanceIDFetcher.Reset()

	result := IsRunningOn(ctx)
	assert.False(t, result)
}

func TestGetNTPHostsNotRunning(t *testing.T) {
	cfg := configmock.New(t)
	ctx := context.Background()
	// Point to a server that returns an error
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	metadataURL = ts.URL
	cfg.SetWithoutSource("cloud_provider_metadata", []string{"alibaba"})
	// Reset the fetcher to force re-fetch
	instanceIDFetcher.Reset()

	hosts := GetNTPHosts(ctx)
	assert.Nil(t, hosts)
}

func TestGetHostAliasesDisabledProvider(t *testing.T) {
	cfg := configmock.New(t)
	ctx := context.Background()

	// Disable alibaba cloud provider
	cfg.SetWithoutSource("cloud_provider_metadata", []string{})
	// Reset the fetcher to force re-fetch
	instanceIDFetcher.Reset()

	_, err := GetHostAliases(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cloud provider is disabled")
}

func TestGetHostAliasesResponseTooLong(t *testing.T) {
	cfg := configmock.New(t)
	ctx := context.Background()

	// Create a response longer than the max hostname size
	longResponse := "i-" + strings.Repeat("x", 300)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, longResponse)
	}))
	defer ts.Close()

	metadataURL = ts.URL
	cfg.SetWithoutSource("cloud_provider_metadata", []string{"alibaba"})
	cfg.SetWithoutSource("metadata_endpoints_max_hostname_size", 256)
	// Reset the fetcher to force re-fetch
	instanceIDFetcher.Reset()

	_, err := GetHostAliases(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "length")
}
