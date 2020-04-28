// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package azure

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mockMetadataRequest(t *testing.T) *httptest.Server {
	lastRequests := []*http.Request{}
	content, err := ioutil.ReadFile("test/azure_metadata.json")
	if err != nil {
		assert.Fail(t, fmt.Sprintf("Error getting test data: %v", err))
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.String(), "/metadata/instance/compute?api-version=2017-08-01")
		assert.Equal(t, "true", r.Header.Get("Metadata"))
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, string(content))
		lastRequests = append(lastRequests, r)
	}))
	metadataURL = ts.URL
	return ts
}

func TestGetHostTags(t *testing.T) {
	server := mockMetadataRequest(t)
	defer server.Close()
	tags, err := GetTags()
	if err != nil {
		assert.Fail(t, fmt.Sprintf("Error getting tags: %v", err))
	}

	expectedTags := []string{
		"vm-id:13f56399-bd52-4150-9748-7190aae1ff21",
		"zone:1",
		"vm-size:Standard_A1_v2",
		"resource-group:myrg",
	}
	require.Len(t, tags, len(expectedTags))
	for _, tag := range tags {
		assert.Contains(t, expectedTags, tag)
	}
}
