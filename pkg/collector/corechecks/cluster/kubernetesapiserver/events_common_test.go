// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver
// +build kubeapiserver

package kubernetesapiserver

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/stretchr/testify/assert"
)

func TestGetDDAlertType(t *testing.T) {
	tests := []struct {
		name    string
		k8sType string
		want    metrics.EventAlertType
	}{
		{
			name:    "normal",
			k8sType: "Normal",
			want:    metrics.EventAlertTypeInfo,
		},
		{
			name:    "warning",
			k8sType: "Warning",
			want:    metrics.EventAlertTypeWarning,
		},
		{
			name:    "unknown",
			k8sType: "Unknown",
			want:    metrics.EventAlertTypeInfo,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getDDAlertType(tt.k8sType)
			assert.Equal(t, got, tt.want)
		})
	}
}
