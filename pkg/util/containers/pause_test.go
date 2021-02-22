// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsPauseContainer(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
		want   bool
	}{
		{
			name: "docker & crio pause container",
			labels: map[string]string{
				"io.kubernetes.container.name": "POD",
				"io.kubernetes.pod.name":       "coredns-66bff467f8-qjf22",
				"io.kubernetes.pod.namespace":  "kube-system",
				"io.kubernetes.pod.uid":        "0adb6ccb-2024-43de-b5a2-6fb0c7f3dc5f",
				"k8s-app":                      "kube-dns",
				"pod-template-hash":            "66bff467f8",
			},
			want: true,
		},
		{
			name: "docker & crio regular container",
			labels: map[string]string{
				"io.kubernetes.container.name": "storage-provisioner",
				"io.kubernetes.pod.name":       "storage-provisioner",
				"io.kubernetes.pod.namespace":  "kube-system",
				"io.kubernetes.pod.uid":        "72e546dc-6977-430e-8e7d-99b61c45eab7",
			},
			want: false,
		},
		{
			name: "containerd pause container",
			labels: map[string]string{
				"controller-revision-hash":    "c8bb659c5",
				"io.kubernetes.pod.name":      "kube-proxy-f5dvk",
				"io.kubernetes.pod.namespace": "kube-system",
				"io.kubernetes.pod.uid":       "180db9cf-e5c0-4d5f-9bbe-505df6c63791",
				"k8s-app":                     "kube-proxy",
				"pod-template-generation":     "1",
			},
			want: true,
		},
		{
			name: "containerd regular container",
			labels: map[string]string{
				"io.kubernetes.container.name": "redis-ctr",
				"io.kubernetes.pod.name":       "redis",
				"io.kubernetes.pod.namespace":  "default",
				"io.kubernetes.pod.uid":        "c83e3e11-263c-4467-9efd-bcb6a314a1f8",
			},
			want: false,
		},
		{
			name:   "empty labels",
			labels: map[string]string{},
			want:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsPauseContainer(tt.labels))
		})
	}
}
