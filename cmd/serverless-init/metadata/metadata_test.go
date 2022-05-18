// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metadata

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDefaultConfig(t *testing.T) {
	assert.Equal(t, 300*time.Millisecond, GetDefaultConfig().timeout)
	assert.Equal(t, "http://metadata.google.internal/computeMetadata/v1/instance/id", GetDefaultConfig().url)
}

func TestGetContainerMalformedUrl(t *testing.T) {
	testConfig := &MetadataConfig{
		timeout: 1 * time.Millisecond,
		url:     string([]byte("\u007F")),
	}
	assert.Equal(t, "unknown-id", GetContainerId(testConfig))
}

func TestGetContainerTimeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(200)
	}))
	defer ts.Close()
	testConfig := &MetadataConfig{
		timeout: 1 * time.Millisecond,
		url:     ts.URL,
	}
	assert.Equal(t, "unknown-id", GetContainerId(testConfig))
}

func TestGetContainerOK(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("1234"))
	}))
	defer ts.Close()
	testConfig := &MetadataConfig{
		timeout: 1 * time.Millisecond,
		url:     ts.URL,
	}
	assert.Equal(t, "1234", GetContainerId(testConfig))
}
