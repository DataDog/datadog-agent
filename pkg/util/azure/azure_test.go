// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package azure

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetHostname(t *testing.T) {
	expected := "5d33a910-a7a0-4443-9f01-6a807801b29b"
	var lastRequest *http.Request
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, expected)
		lastRequest = r
	}))
	defer ts.Close()
	metadataURL = ts.URL

	val, err := GetHostAlias()
	assert.Nil(t, err)
	assert.Equal(t, expected, val)
	assert.Equal(t, lastRequest.URL.Path, "/metadata/instance/compute/vmId")
	assert.Equal(t, lastRequest.URL.RawQuery, "api-version=2017-04-02&format=text")
}

func TestGetClusterName(t *testing.T) {
	apiResponse := "MC_aks-kenafeh_aks-kenafeh-eu_westeurope"
	expectedClusterName := "aks-kenafeh-eu"
	var lastRequest *http.Request
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, apiResponse)
		lastRequest = r
	}))
	defer ts.Close()
	metadataURL = ts.URL

	val, err := GetClusterName()
	assert.Nil(t, err)
	assert.Equal(t, expectedClusterName, val)
	assert.Equal(t, lastRequest.URL.Path, "/metadata/instance/compute/resourceGroupName")
	assert.Equal(t, lastRequest.URL.RawQuery, "api-version=2017-08-01&format=text")
}
