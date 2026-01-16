// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package libraryinjection_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/libraryinjection"
)

func TestNewLibraryImageFromFullRef(t *testing.T) {
	tests := []struct {
		name             string
		fullRef          string
		canonicalVersion string
		expectedName     string
		expectedRegistry string
		expectedVersion  string
	}{
		{
			name:             "full image with tag",
			fullRef:          "gcr.io/datadoghq/dd-lib-java-init:1.2.3",
			canonicalVersion: "1.2.3",
			expectedName:     "dd-lib-java-init",
			expectedRegistry: "gcr.io/datadoghq",
			expectedVersion:  "1.2.3",
		},
		{
			name:             "full image with digest",
			fullRef:          "gcr.io/datadoghq/dd-lib-java-init@sha256:abc123",
			canonicalVersion: "1.2.3",
			expectedName:     "dd-lib-java-init",
			expectedRegistry: "gcr.io/datadoghq",
			expectedVersion:  "sha256:abc123",
		},
		{
			name:             "image without registry",
			fullRef:          "dd-lib-java-init:1.2.3",
			canonicalVersion: "",
			expectedName:     "dd-lib-java-init",
			expectedRegistry: "",
			expectedVersion:  "1.2.3",
		},
		{
			name:             "image without version",
			fullRef:          "gcr.io/datadoghq/dd-lib-java-init",
			canonicalVersion: "",
			expectedName:     "dd-lib-java-init",
			expectedRegistry: "gcr.io/datadoghq",
			expectedVersion:  "",
		},
		{
			name:             "simple image name only",
			fullRef:          "dd-lib-java-init",
			canonicalVersion: "",
			expectedName:     "dd-lib-java-init",
			expectedRegistry: "",
			expectedVersion:  "",
		},
		{
			name:             "registry with port",
			fullRef:          "localhost:5000/dd-lib-java-init:1.2.3",
			canonicalVersion: "",
			expectedName:     "dd-lib-java-init",
			expectedRegistry: "localhost:5000",
			expectedVersion:  "1.2.3",
		},
		{
			name:             "multi-level namespace",
			fullRef:          "gcr.io/project/team/dd-lib-java-init:1.2.3",
			canonicalVersion: "",
			expectedName:     "dd-lib-java-init",
			expectedRegistry: "gcr.io/project/team",
			expectedVersion:  "1.2.3",
		},
		{
			name:             "docker hub user namespace",
			fullRef:          "foo/bar:1.0",
			canonicalVersion: "",
			expectedName:     "bar",
			expectedRegistry: "foo",
			expectedVersion:  "1.0",
		},
		{
			name:             "docker.io/library is preserved",
			fullRef:          "docker.io/library/nginx:latest",
			canonicalVersion: "",
			expectedName:     "nginx",
			expectedRegistry: "docker.io/library",
			expectedVersion:  "latest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			img := libraryinjection.NewLibraryImageFromFullRef(tt.fullRef, tt.canonicalVersion)

			assert.Equal(t, tt.expectedName, img.Name)
			assert.Equal(t, tt.expectedRegistry, img.Registry)
			assert.Equal(t, tt.expectedVersion, img.Version)
			assert.Equal(t, tt.canonicalVersion, img.CanonicalVersion)
		})
	}
}

func TestLibraryImage_FullRef(t *testing.T) {
	tests := []struct {
		name     string
		img      libraryinjection.LibraryImage
		expected string
	}{
		{
			name: "all fields set",
			img: libraryinjection.LibraryImage{
				Name:     "dd-lib-java-init",
				Registry: "gcr.io/datadoghq",
				Version:  "1.2.3",
			},
			expected: "gcr.io/datadoghq/dd-lib-java-init:1.2.3",
		},
		{
			name: "no version",
			img: libraryinjection.LibraryImage{
				Name:     "dd-lib-java-init",
				Registry: "gcr.io/datadoghq",
			},
			expected: "gcr.io/datadoghq/dd-lib-java-init",
		},
		{
			name: "no registry",
			img: libraryinjection.LibraryImage{
				Name:    "dd-lib-java-init",
				Version: "1.2.3",
			},
			expected: "dd-lib-java-init:1.2.3",
		},
		{
			name: "name only",
			img: libraryinjection.LibraryImage{
				Name: "dd-lib-java-init",
			},
			expected: "dd-lib-java-init",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.img.FullRef())
		})
	}
}

func TestLibraryImage_RoundTrip(t *testing.T) {
	// Test that parsing a full ref and then calling FullRef returns the original
	testCases := []string{
		"gcr.io/datadoghq/dd-lib-java-init:1.2.3",
		"localhost:5000/dd-lib-java-init:latest",
		"dd-lib-java-init:1.2.3",
		"foo/bar:1.0",
		"docker.io/library/nginx:latest",
		"gcr.io/datadoghq/dd-lib-java-init@sha256:abc123",
	}

	for _, fullRef := range testCases {
		t.Run(fullRef, func(t *testing.T) {
			img := libraryinjection.NewLibraryImageFromFullRef(fullRef, "")
			assert.Equal(t, fullRef, img.FullRef())
		})
	}
}
