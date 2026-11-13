// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseImageReference(t *testing.T) {

	tests := []struct {
		fullImagePath string
		expectedTag   string
		expectedImage string
	}{
		{
			fullImagePath: "nginx",
			expectedImage: "nginx",
			expectedTag:   "latest",
		},
		{
			fullImagePath: "nginx:latest",
			expectedImage: "nginx",
			expectedTag:   "latest",
		},
		{
			fullImagePath: "example.com:5000/myimage",
			expectedImage: "example.com:5000/myimage",
			expectedTag:   "latest",
		},
		{
			fullImagePath: "example.com:5000/myimage:1.0",
			expectedImage: "example.com:5000/myimage",
			expectedTag:   "1.0",
		},
		{
			fullImagePath: "example.com:5000/myimage:",
			expectedImage: "example.com:5000/myimage",
			expectedTag:   "latest",
		},
		{
			fullImagePath: "example.com/datadog/agent-dev:abcdefg",
			expectedImage: "example.com/datadog/agent-dev",
			expectedTag:   "abcdefg",
		},
	}

	for _, test := range tests {
		t.Run(test.fullImagePath, func(t *testing.T) {
			imagePath, tag := ParseImageReference(test.fullImagePath)
			assert.Equal(t, test.expectedImage, imagePath)
			assert.Equal(t, test.expectedTag, tag)
		})
	}
}

func TestKubernetesImageRegistry(t *testing.T) {
	for _, tt := range []struct {
		name, registry, want string
	}{
		{"public fallback", "", "registry.k8s.io"},
		{"cache", "cache.example.com", "cache.example.com/k8s"},
		{"multiple credentials", "cache.example.com,another.example.com", "cache.example.com/k8s"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, KubernetesImageRegistry(tt.registry))
		})
	}
}
