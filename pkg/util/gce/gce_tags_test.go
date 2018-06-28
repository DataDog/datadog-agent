// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build gce

package gce

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

func TestGetHostTags(t *testing.T) {
	lastRequests := []*http.Request{}
	content, err := ioutil.ReadFile("test/gce_metadata.json")
	if err != nil {
		assert.Fail(t, fmt.Sprintf("Error getting test data: %v", err))
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.String(), "/?recursive=true")
		assert.Equal(t, "Google", r.Header.Get("Metadata-Flavor"))
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, string(content))
		lastRequests = append(lastRequests, r)
	}))
	defer ts.Close()
	metadataURL = ts.URL

	tags, err := GetTags()
	if err != nil {
		assert.Fail(t, fmt.Sprintf("Error getting tags: %v", err))
	}

	expectedTags := []string{
		"tag",
		"zone:us-east1-b",
		"instance-type:n1-standard-1",
		"internal-hostname:dd-test.c.datadog-dd-test.internal",
		"instance-id:1111111111111111111",
		"project:test-project",
		"numeric_project_id:111111111111",
		"cluster-location:us-east1-b",
		"cluster-name:test-cluster",
		"created-by:projects/111111111111/zones/us-east1-b/instanceGroupManagers/gke-test-cluster-default-pool-0012834b-grp",
		"gci-ensure-gke-docker:true",
		"gci-update-strategy:update_disabled",
		"google-compute-enable-pcid:true",
		"instance-template:projects/111111111111/global/instanceTemplates/gke-test-cluster-default-pool-0012834b",
	}
	require.Len(t, tags, len(expectedTags))
	for _, tag := range tags {
		assert.Contains(t, expectedTags, tag)
	}
}
