// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package gce

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetHostname(t *testing.T) {
	expected := "gke-cluster-massi-agent59-default-pool-6087cc76-9cfa"
	var lastRequest *http.Request
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, expected)
		lastRequest = r
	}))
	defer ts.Close()
	metadataURL = ts.URL

	val, err := GetHostname()
	assert.Nil(t, err)
	assert.Equal(t, expected, val)
	assert.Equal(t, "/instance/hostname", lastRequest.URL.Path)
}

func TestGetHostnameEmptyBody(t *testing.T) {
	var lastRequest *http.Request
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		lastRequest = r
	}))
	defer ts.Close()
	metadataURL = ts.URL

	val, err := GetHostname()
	assert.Error(t, err)
	assert.Empty(t, val)
	assert.Equal(t, "/instance/hostname", lastRequest.URL.Path)
}

func TestGetHostAliases(t *testing.T) {
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

	val, err := GetHostAlias()
	assert.Nil(t, err)
	assert.Equal(t, "gce-instance-name.gce-project", val)
}

func TestGetClusterName(t *testing.T) {
	expected := "test-cluster-name"
	var lastRequest *http.Request
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, expected)
		lastRequest = r
	}))
	defer ts.Close()
	metadataURL = ts.URL

	val, err := GetClusterName()
	assert.Nil(t, err)
	assert.Equal(t, expected, val)
	assert.Equal(t, "/instance/attributes/cluster-name", lastRequest.URL.Path)
}
