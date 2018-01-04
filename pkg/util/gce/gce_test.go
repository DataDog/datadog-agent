// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

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
	assert.Equal(t, lastRequest.URL.Path, "/instance/hostname")
}

func TestGetHostAliases(t *testing.T) {
	lastRequests := []*http.Request{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		if r.URL.Path == "/instance/hostname" {
			io.WriteString(w, "gce-hostname.c.datadog-demo.internal")
		} else if r.URL.Path == "/project/project-id" {
			io.WriteString(w, "gce-project")
		} else {
			t.Fatalf("Unknown URL requested: %s", r.URL.Path)
		}
		lastRequests = append(lastRequests, r)
	}))
	defer ts.Close()
	metadataURL = ts.URL

	val, err := GetHostAlias()
	assert.Nil(t, err)
	assert.Equal(t, "gce-hostname.gce-project", val)
}
