// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package djm contains data-jobs-monitoring installation logic
package djm

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"cloud.google.com/go/compute/metadata"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup/common"
	"github.com/stretchr/testify/assert"
)

type DynamicRoundTripper struct {
	Handler func(req *http.Request) (*http.Response, error)
}

func (d *DynamicRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return d.Handler(req)
}

func TestSetupDataproc(t *testing.T) {

	mockResponses := map[string]string{
		"/computeMetadata/v1/instance/attributes/dataproc-cluster-uuid": "test-cluster-uuid",
		"/computeMetadata/v1/instance/attributes/dataproc-role":         "Master",
		"/computeMetadata/v1/instance/attributes/dataproc-cluster-name": "test-cluster-name",
	}

	mockRoundTripper := &DynamicRoundTripper{
		Handler: func(req *http.Request) (*http.Response, error) {
			if value, found := mockResponses[req.URL.Path]; found {
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(strings.NewReader(value)),
					Header:     make(http.Header),
				}, nil
			}
			return &http.Response{
				StatusCode: 404,
				Body:       io.NopCloser(strings.NewReader("")),
				Header:     make(http.Header),
			}, nil
		},
	}

	mockHTTPClient := &http.Client{Transport: mockRoundTripper}

	// Create a metadata client with the mocked HTTP client
	mockMetadataClient := metadata.NewClient(mockHTTPClient)

	tests := []struct {
		name     string
		wantTags []string
	}{
		{
			name: "master node",
			wantTags: []string{
				"data_workload_monitoring_trial:true",
				"cluster_id:test-cluster-uuid",
				"dataproc_cluster_id:test-cluster-uuid",
				"cluster_name:test-cluster-name",
				"is_master_node:true",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			span, _ := telemetry.StartSpanFromContext(context.Background(), "test")
			s := &common.Setup{
				Span: span,
				Ctx:  context.Background(),
			}

			_, _, err := setupCommonDataprocHostTags(s, mockMetadataClient)
			assert.Nil(t, err)
			assert.ElementsMatch(t, tt.wantTags, s.Config.DatadogYAML.Tags)
		})
	}
}
