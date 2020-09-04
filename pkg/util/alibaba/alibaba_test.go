// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package alibaba

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config"
)

func TestGetHostname(t *testing.T) {
	expected := "i-rj9aql2pwopjn4sm24ix"
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
	assert.Equal(t, lastRequest.URL.Path, "/latest/meta-data/instance-id")
}

func TestGetNTPHosts(t *testing.T) {
	expectedHosts := []string{
		"ntp.cloud.aliyuncs.com", "ntp1.cloud.aliyuncs.com", "ntp2.cloud.aliyuncs.com", "ntp3.cloud.aliyuncs.com",
		"ntp4.cloud.aliyuncs.com", "ntp5.cloud.aliyuncs.com", "ntp6.cloud.aliyuncs.com", "ntp7.cloud.aliyuncs.com",
		"ntp8.cloud.aliyuncs.com", "ntp9.cloud.aliyuncs.com", "ntp10.cloud.aliyuncs.com", "ntp11.cloud.aliyuncs.com",
		"ntp12.cloud.aliyuncs.com",
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, "test")
	}))
	defer ts.Close()

	metadataURL = ts.URL
	config.Datadog.Set("cloud_provider_metadata", []string{"alibaba"})
	actualHosts := GetNTPHosts()

	assert.Equal(t, expectedHosts, actualHosts)
}
