// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOCIChartRegistryIndex(t *testing.T) {
	const ecr = "669783387624.dkr.ecr.us-east-1.amazonaws.com"
	tests := []struct {
		name       string
		chartRef   string
		registries string
		want       int
	}{
		{"chart on the only registry", "oci://" + ecr + "/ghcr/nvidia/chart/nvml-mock", ecr, 0},
		{"chart on the second registry", "oci://" + ecr + "/k8s/kueue/charts/kueue", "us-docker.pkg.dev," + ecr, 1},
		{"public chart", "oci://ghcr.io/nvidia/k8s-test-infra/chart/nvml-mock", ecr, -1},
		{"no registries configured", "oci://" + ecr + "/ghcr/chart", "", -1},
		{"not an OCI chart", ecr + "/ghcr/chart", ecr, -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ociChartRegistryIndex(tt.chartRef, tt.registries))
		})
	}
}
