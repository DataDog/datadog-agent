// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build kubeapiserver

package kubemetadata

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateKubeMetadataEntityID(t *testing.T) {
	tests := []struct {
		name         string
		group        string
		namespace    string
		resourceType string
		resourceName string
		expectedID   EntityID
	}{
		{
			name:         "namespace scoped resource",
			group:        "apps",
			namespace:    "default",
			resourceType: "deployments",
			resourceName: "app",
			expectedID:   "apps/deployments/default/app",
		},
		{
			name:         "cluster scoped resource",
			group:        "",
			namespace:    "",
			resourceType: "nodes",
			resourceName: "foo-node",
			expectedID:   "/nodes//foo-node",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			assert.Equal(tt, test.expectedID, GenerateKubeMetadataEntityID(test.group, test.resourceType, test.namespace, test.resourceName))
		})
	}

}

func TestParseKubeMetadataEntityID(t *testing.T) {
	tests := []struct {
		name              string
		entityID          EntityID
		expectedGroup     string
		expectedName      string
		expectedNamespace string
		expectedResource  string
		expectError       bool
	}{
		{
			name:              "namespace scoped resource",
			expectedGroup:     "apps",
			expectedNamespace: "default",
			expectedResource:  "deployments",
			expectedName:      "app",
			entityID:          "apps/deployments/default/app",
			expectError:       false,
		},
		{
			name:              "cluster scoped resource",
			expectedGroup:     "",
			expectedNamespace: "",
			expectedResource:  "nodes",
			expectedName:      "foo-node",
			entityID:          GenerateKubeMetadataEntityID("", "nodes", "", "foo-node"),
			expectError:       false,
		},
		{
			name:              "malformatted id",
			expectedGroup:     "",
			expectedNamespace: "",
			expectedResource:  "",
			expectedName:      "",
			entityID:          "mal//formatted//id",
			expectError:       true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			group, resource, namespace, name, err := ParseKubeMetadataEntityID(test.entityID)
			if test.expectError {
				assert.Error(tt, err)
			} else {
				assert.NoError(tt, err)
			}

			assert.Equal(tt, group, test.expectedGroup)
			assert.Equal(tt, name, test.expectedName)
			assert.Equal(tt, namespace, test.expectedNamespace)
			assert.Equal(tt, resource, test.expectedResource)
		})
	}

}
