// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build kubeapiserver

// Package util contains utility functions for workload metadata collectors
package util

import (
	"testing"

	"github.com/stretchr/testify/assert"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

func TestGenerateKubeMetadataEntityID(t *testing.T) {
	tests := []struct {
		name         string
		namespace    string
		resourceType string
		resourceName string
		expectedID   workloadmeta.KubeMetadataEntityID
	}{
		{
			name:         "namespace scoped resource",
			namespace:    "default",
			resourceType: "deployments",
			resourceName: "app",
			expectedID:   "deployments/default/app",
		},
		{
			name:         "cluster scoped resource",
			namespace:    "",
			resourceType: "nodes",
			resourceName: "foo-node",
			expectedID:   GenerateKubeMetadataEntityID("nodes", "", "foo-node"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			assert.Equal(tt, GenerateKubeMetadataEntityID(test.resourceType, test.namespace, test.resourceName), test.expectedID)
		})
	}

}

func TestParseKubeMetadataEntityID(t *testing.T) {
	tests := []struct {
		name              string
		entityID          workloadmeta.KubeMetadataEntityID
		expectedName      string
		expectedNamespace string
		expectedResource  string
		expectError       bool
	}{
		{
			name:              "namespace scoped resource",
			expectedNamespace: "default",
			expectedResource:  "deployments",
			expectedName:      "app",
			entityID:          "deployments/default/app",
			expectError:       false,
		},
		{
			name:              "cluster scoped resource",
			expectedNamespace: "",
			expectedResource:  "nodes",
			expectedName:      "foo-node",
			entityID:          GenerateKubeMetadataEntityID("nodes", "", "foo-node"),
			expectError:       false,
		},
		{
			name:              "malformatted id",
			expectedNamespace: "",
			expectedResource:  "",
			expectedName:      "",
			entityID:          "mal//formatted//id",
			expectError:       true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			resource, namespace, name, err := ParseKubeMetadataEntityID(test.entityID)
			if test.expectError {
				assert.Error(tt, err)
			} else {
				assert.NoError(tt, err)
			}

			assert.Equal(tt, name, test.expectedName)
			assert.Equal(tt, namespace, test.expectedNamespace)
			assert.Equal(tt, resource, test.expectedResource)
		})
	}

}
