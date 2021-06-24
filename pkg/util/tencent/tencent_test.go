// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tencent

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestGetInstanceID(t *testing.T) {
	ctx := context.Background()
	holdValue := config.Datadog.Get("cloud_provider_metadata")
	defer config.Datadog.Set("cloud_provider_metadata", holdValue)
	config.Datadog.Set("cloud_provider_metadata", []string{"tencent"})

	expected := "ins-nad6bga0"
	var lastRequest *http.Request
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, expected)
		lastRequest = r
	}))
	defer ts.Close()
	metadataURL = ts.URL

	val, err := GetInstanceID(ctx)
	assert.Nil(t, err)
	assert.Equal(t, expected, val)
	assert.Equal(t, lastRequest.URL.Path, "/meta-data/instance-id")
}

func TestGetNTPHosts(t *testing.T) {
	ctx := context.Background()
	expectedHosts := []string{"ntpupdate.tencentyun.com"}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, "test")
	}))
	defer ts.Close()

	metadataURL = ts.URL
	config.Datadog.Set("cloud_provider_metadata", []string{"tencent"})
	actualHosts := GetNTPHosts(ctx)

	assert.Equal(t, expectedHosts, actualHosts)
}
