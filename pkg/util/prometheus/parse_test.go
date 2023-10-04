// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package prometheus

import (
	"testing"
)

func TestParseMetrics(t *testing.T) {

	mockOpenmetricsData := `
	grpc_server_msg_received_total{grpc_method="PullImage",grpc_service="runtime.v1.ImageService",grpc_type="unary"} 0
	grpc_server_msg_received_total{grpc_method="PullImage",grpc_service="runtime.v1alpha2.ImageService",grpc_type="unary"} 16631
	grpc_server_msg_sent_total{grpc_method="PullImage",grpc_service="runtime.v1.ImageService",grpc_type="unary"} 0
	grpc_server_msg_sent_total{grpc_method="PullImage",grpc_service="runtime.v1alpha2.ImageService",grpc_type="unary"} 72
	grpc_server_started_total{grpc_method="PullImage",grpc_service="runtime.v1.ImageService",grpc_type="unary"} 0
	grpc_server_started_total{grpc_method="PullImage",grpc_service="runtime.v1alpha2.ImageService",grpc_type="unary"} 16631
	`

	parsedMetrics, err := ParseMetrics([]byte(mockOpenmetricsData))

	if err != nil {
		t.Errorf("parsing metrics failed with %s", err)
	}

	expectedNumberOfMetrics := 6
	actualNumberOfMetrics := parsedMetrics.Len()

	if actualNumberOfMetrics != expectedNumberOfMetrics {
		t.Errorf("expected %d reported metrics, got %d reported metrics", expectedNumberOfMetrics, actualNumberOfMetrics)
	}

}
