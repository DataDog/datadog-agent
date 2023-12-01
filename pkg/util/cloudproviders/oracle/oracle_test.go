// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package oracle

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

func TestGetHostAliases(t *testing.T) {
	holdValue := config.Datadog.Get("cloud_provider_metadata")
	defer config.Datadog.SetWithoutSource("cloud_provider_metadata", holdValue)
	config.Datadog.SetWithoutSource("cloud_provider_metadata", []string{"oracle"})

	ctx := context.Background()
	expected := "ocid1.instance.oc1.iad.anuwcljte6cuweqcz7sarpn43hst2kaaaxbbbccbaaa6vpd66tvcyhgiifsq"
	var lastRequest *http.Request
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		io.WriteString(w, expected)
		lastRequest = r
	}))
	defer ts.Close()

	defer func(url string) { metadataURL = url }(metadataURL)
	metadataURL = ts.URL

	aliases, err := GetHostAliases(ctx)
	assert.Nil(t, err)
	require.Len(t, aliases, 1)
	assert.Equal(t, expected, aliases[0])
	assert.Equal(t, lastRequest.URL.Path, "/opc/v2/instance/id")
	assert.Equal(t, lastRequest.Header.Get("Authorization"), "Bearer Oracle")
}

func TestGetNTPHosts(t *testing.T) {
	holdValue := config.Datadog.Get("cloud_provider_metadata")
	defer config.Datadog.SetWithoutSource("cloud_provider_metadata", holdValue)
	config.Datadog.SetWithoutSource("cloud_provider_metadata", []string{"oracle"})

	ctx := context.Background()
	expectedHosts := []string{"169.254.169.254"}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		io.WriteString(w, "test")
	}))
	defer ts.Close()

	defer func(url string) { metadataURL = url }(metadataURL)
	metadataURL = ts.URL

	actualHosts := GetNTPHosts(ctx)
	assert.Equal(t, expectedHosts, actualHosts)
}
