// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build kubeapiserver

// Package util contains utility functions for image metadata collection
package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateKubeMetadataEntityID(t *testing.T) {
	tests := []struct {
		name         string
		namespace    string
		resourceType string
		resourceName string
		expectedID   string
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
			expectedID:   "nodes//foo-node",
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
		entityID          string
		expectedName      string
		expectedNamespace string
		expectedResource  string
	}{
		{
			name:              "namespace scoped resource",
			expectedNamespace: "default",
			expectedResource:  "deployments",
			expectedName:      "app",
			entityID:          "deployments/default/app",
		},
		{
			name:              "cluster scoped resource",
			expectedNamespace: "",
			expectedResource:  "nodes",
			expectedName:      "foo-node",
			entityID:          "nodes//foo-node",
		},
		{
			name:              "malformatted id",
			expectedNamespace: "",
			expectedResource:  "",
			expectedName:      "",
			entityID:          "mal//formatted//id",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			resource, namespace, name := ParseKubeMetadataEntityID(test.entityID)
			assert.Equal(tt, test.expectedName, name)
			assert.Equal(tt, test.expectedNamespace, namespace)
			assert.Equal(tt, test.expectedResource, resource)
		})
	}

}
