// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetTagValue(t *testing.T) {
	tests := []struct {
		name     string
		tagName  string
		tags     []string
		expected string
	}{
		{
			name:     "tag found",
			tagName:  "env",
			tags:     []string{"env:production", "service:web"},
			expected: "production",
		},
		{
			name:     "tag not found",
			tagName:  "version",
			tags:     []string{"env:production", "service:web"},
			expected: "",
		},
		{
			name:     "empty tags",
			tagName:  "env",
			tags:     []string{},
			expected: "",
		},
		{
			name:     "tag without colon",
			tagName:  "env",
			tags:     []string{"invalid_tag", "env:staging"},
			expected: "staging",
		},
		{
			name:     "multiple colons in value",
			tagName:  "url",
			tags:     []string{"url:http://example.com:8080"},
			expected: "http://example.com:8080",
		},
		{
			name:     "empty value",
			tagName:  "env",
			tags:     []string{"env:"},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetTagValue(tt.tagName, tt.tags)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetTagName(t *testing.T) {
	tests := []struct {
		name     string
		tag      string
		expected string
	}{
		{
			name:     "valid tag",
			tag:      "env:production",
			expected: "env",
		},
		{
			name:     "no colon",
			tag:      "invalid",
			expected: "",
		},
		{
			name:     "empty string",
			tag:      "",
			expected: "",
		},
		{
			name:     "multiple colons",
			tag:      "url:http://example.com:8080",
			expected: "url",
		},
		{
			name:     "colon only",
			tag:      ":",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetTagName(tt.tag)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetContainerFilterTags(t *testing.T) {
	tests := []struct {
		name              string
		tags              []string
		expectedContainer string
		expectedImage     string
		expectedNamespace string
	}{
		{
			name:              "all tags present",
			tags:              []string{"container_name:web", "image_name:nginx", "kube_namespace:default"},
			expectedContainer: "web",
			expectedImage:     "nginx",
			expectedNamespace: "default",
		},
		{
			name:              "some tags missing",
			tags:              []string{"container_name:web", "other:value"},
			expectedContainer: "web",
			expectedImage:     "",
			expectedNamespace: "",
		},
		{
			name:              "no tags",
			tags:              []string{},
			expectedContainer: "",
			expectedImage:     "",
			expectedNamespace: "",
		},
		{
			name:              "other tags only",
			tags:              []string{"env:prod", "service:api"},
			expectedContainer: "",
			expectedImage:     "",
			expectedNamespace: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			container, image, namespace := GetContainerFilterTags(tt.tags)
			assert.Equal(t, tt.expectedContainer, container)
			assert.Equal(t, tt.expectedImage, image)
			assert.Equal(t, tt.expectedNamespace, namespace)
		})
	}
}
