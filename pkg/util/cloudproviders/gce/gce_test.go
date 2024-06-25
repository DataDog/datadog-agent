// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package gce

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config"
)

func reset() {
	hostnameFetcher.Reset()
	nameFetcher.Reset()
	clusterNameFetcher.Reset()
	publicIPv4Fetcher.Reset()
	networkIDFetcher.Reset()
}

func TestGetHostname(t *testing.T) {
	reset()
	ctx := context.Background()
	expected := "gke-cluster-massi-agent59-default-pool-6087cc76-9cfa"
	var lastRequest *http.Request
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, expected)
		lastRequest = r
	}))
	defer ts.Close()
	metadataURL = ts.URL

	val, err := GetHostname(ctx)
	assert.NoError(t, err)
	assert.Equal(t, expected, val)
	assert.Equal(t, "/instance/hostname", lastRequest.URL.Path)
}

func TestGetHostnameEmptyBody(t *testing.T) {
	reset()
	ctx := context.Background()
	var lastRequest *http.Request
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		lastRequest = r
	}))
	defer ts.Close()
	metadataURL = ts.URL

	val, err := GetHostname(ctx)
	assert.Error(t, err)
	assert.Empty(t, val)
	assert.Equal(t, "/instance/hostname", lastRequest.URL.Path)
}

func TestGetHostAliases(t *testing.T) {
	reset()
	ctx := context.Background()
	lastRequests := []*http.Request{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		switch path := r.URL.Path; path {
		case "/instance/hostname":
			io.WriteString(w, "gce-custom-hostname.custom-domain.gce-project")
		case "/instance/name":
			io.WriteString(w, "gce-instance-name")
		case "/project/project-id":
			io.WriteString(w, "gce-project")
		default:
			t.Fatalf("Unknown URL requested: %s", path)
		}
		lastRequests = append(lastRequests, r)
	}))
	defer ts.Close()
	metadataURL = ts.URL

	val, err := GetHostAliases(ctx)
	assert.NoError(t, err)
	assert.Equal(t, []string{"gce-custom-hostname.custom-domain.gce-project", "gce-instance-name.gce-project"}, val)
}

func TestGetHostAliasesInstanceNameError(t *testing.T) {
	reset()
	ctx := context.Background()
	lastRequests := []*http.Request{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		switch path := r.URL.Path; path {
		case "/instance/hostname":
			io.WriteString(w, "gce-custom-hostname.custom-domain.gce-project")
		case "/instance/name":
			w.WriteHeader(http.StatusNotFound)
		case "/project/project-id":
			io.WriteString(w, "gce-project")
		default:
			t.Fatalf("Unknown URL requested: %s", path)
		}
		lastRequests = append(lastRequests, r)
	}))
	defer ts.Close()
	metadataURL = ts.URL

	val, err := GetHostAliases(ctx)
	assert.NoError(t, err)
	assert.Equal(t, []string{"gce-custom-hostname.custom-domain.gce-project", "gce-custom-hostname.gce-project"}, val)
}

func TestGetClusterName(t *testing.T) {
	reset()
	ctx := context.Background()
	expected := "test-cluster-name"
	var lastRequest *http.Request
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, expected)
		lastRequest = r
	}))
	defer ts.Close()
	metadataURL = ts.URL

	val, err := GetClusterName(ctx)
	assert.NoError(t, err)
	assert.Equal(t, expected, val)
	assert.Equal(t, "/instance/attributes/cluster-name", lastRequest.URL.Path)
}

func TestGetPublicIPv4(t *testing.T) {
	reset()
	ctx := context.Background()
	expected := "10.0.0.2"
	var lastRequest *http.Request
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, expected)
		lastRequest = r
	}))
	defer ts.Close()
	metadataURL = ts.URL

	val, err := GetPublicIPv4(ctx)
	assert.NoError(t, err)
	assert.Equal(t, expected, val)
	assert.Equal(t, "/instance/network-interfaces/0/access-configs/0/external-ip", lastRequest.URL.Path)
}

func TestGetNetwork(t *testing.T) {
	reset()
	ctx := context.Background()
	expected := "projects/123456789/networks/my-network-name"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		switch r.RequestURI {
		case "/instance/network-interfaces/":
			io.WriteString(w, "0/\n")
		case "/instance/network-interfaces/0/network":
			io.WriteString(w, expected)
		default:
			t.Errorf("unexpected request %s", r.RequestURI)
		}
	}))
	defer ts.Close()
	metadataURL = ts.URL

	val, err := GetNetworkID(ctx)
	assert.NoError(t, err)
	assert.Equal(t, expected, val)
}

func TestGetNetworkNoInferface(t *testing.T) {
	reset()
	ctx := context.Background()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, "")
	}))
	defer ts.Close()
	metadataURL = ts.URL

	_, err := GetNetworkID(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty response body")
}

func TestGetNetworkMultipleVPC(t *testing.T) {
	reset()
	ctx := context.Background()
	vpc := "projects/123456789/networks/my-network-name"
	vpcOther := "projects/123456789/networks/my-other-name"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		switch r.RequestURI {
		case "/instance/network-interfaces/":
			io.WriteString(w, "0/\n")
			io.WriteString(w, "1/\n")
		case "/instance/network-interfaces/0/network":
			io.WriteString(w, vpc)
		case "/instance/network-interfaces/1/network":
			io.WriteString(w, vpcOther)
		default:
			t.Errorf("unexpected request %s", r.RequestURI)
		}
	}))
	defer ts.Close()
	metadataURL = ts.URL

	_, err := GetNetworkID(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "more than one network interface")
}

func TestGetNTPHosts(t *testing.T) {
	ctx := context.Background()
	expectedHosts := []string{"metadata.google.internal"}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, "test")
	}))
	defer ts.Close()

	metadataURL = ts.URL
	config.Datadog().SetWithoutSource("cloud_provider_metadata", []string{"gcp"})
	actualHosts := GetNTPHosts(ctx)

	assert.Equal(t, expectedHosts, actualHosts)
}
