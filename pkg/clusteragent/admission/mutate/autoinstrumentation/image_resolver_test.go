// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"encoding/json"
	"testing"

	rcclient "github.com/DataDog/datadog-agent/pkg/config/remote/client"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewImageResolver(t *testing.T) {
	t.Run("with_remote_config_client", func(t *testing.T) {
		// Mock client (nil is fine for this test)
		mockClient := (*rcclient.Client)(nil)
		resolver := NewImageResolver(mockClient)

		// Should return NoOpImageResolver since mockClient is nil
		_, ok := resolver.(*NoOpImageResolver)
		assert.True(t, ok, "Should return NoOpImageResolver when rcClient is nil")
	})

	t.Run("without_remote_config_client", func(t *testing.T) {
		resolver := NewImageResolver(nil)

		_, ok := resolver.(*NoOpImageResolver)
		assert.True(t, ok, "Should return NoOpImageResolver when rcClient is nil")
	})
}

func TestNoOpImageResolver(t *testing.T) {
	resolver := NewNoOpImageResolver()

	testCases := []struct {
		name           string
		imageRef       string
		expectedResult string
		expectedOK     bool
	}{
		{
			name:           "full_image_reference",
			imageRef:       "gcr.io/datadoghq/dd-lib-python-init:latest",
			expectedResult: "",
			expectedOK:     false,
		},
		{
			name:           "empty_image",
			imageRef:       "",
			expectedResult: "",
			expectedOK:     false,
		},
		{
			name:           "image_without_tag",
			imageRef:       "gcr.io/datadoghq/dd-lib-python-init",
			expectedResult: "",
			expectedOK:     false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, ok := resolver.Resolve(tc.imageRef)
			assert.Equal(t, tc.expectedResult, result)
			assert.Equal(t, tc.expectedOK, ok)
		})
	}
}

func TestParseImageReference(t *testing.T) {
	testCases := []struct {
		name         string
		imageRef     string
		expectedRepo string
		expectedTag  string
		expectError  bool
	}{
		{
			name:         "full_gcr_image",
			imageRef:     "gcr.io/datadoghq/dd-lib-python-init:latest",
			expectedRepo: "dd-lib-python-init",
			expectedTag:  "latest",
			expectError:  false,
		},
		{
			name:         "versioned_tag",
			imageRef:     "gcr.io/datadoghq/dd-lib-java-init:v1.2.3",
			expectedRepo: "dd-lib-java-init",
			expectedTag:  "v1.2.3",
			expectError:  false,
		},
		{
			name:         "simple_image",
			imageRef:     "dd-lib-python-init:latest",
			expectedRepo: "dd-lib-python-init",
			expectedTag:  "latest",
			expectError:  false,
		},
		{
			name:         "no_tag_separator",
			imageRef:     "gcr.io/datadoghq/dd-lib-python-init",
			expectedRepo: "",
			expectedTag:  "",
			expectError:  true,
		},
		{
			name:         "port_in_registry",
			imageRef:     "localhost:5000/my-image/test",
			expectedRepo: "",
			expectedTag:  "",
			expectError:  true,
		},
		{
			name:         "empty_image",
			imageRef:     "",
			expectedRepo: "",
			expectedTag:  "",
			expectError:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			repo, tag, err := parseImageReference(tc.imageRef)

			if tc.expectError {
				assert.Error(t, err)
				assert.Empty(t, repo)
				assert.Empty(t, tag)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedRepo, repo)
				assert.Equal(t, tc.expectedTag, tag)
			}
		})
	}
}

func TestRemoteConfigImageResolver_processUpdate(t *testing.T) {
	// Create resolver without remote config client for testing
	resolver := &RemoteConfigImageResolver{
		imageMappings: make(map[string]map[string]ResolvedImage),
	}

	// Test data: multiple repositories
	repo1Config := RepositoryConfig{
		RepositoryName: "dd-lib-python-init",
		RepositoryURL:  "gcr.io/datadoghq/dd-lib-python-init",
		Images: []ImageInfo{
			{
				Tag:              "latest",
				Digest:           "sha256:abc123",
				CanonicalVersion: "1.0.0",
			},
			{
				Tag:              "v3",
				Digest:           "sha256:def456",
				CanonicalVersion: "1.0.0",
			},
		},
	}

	repo2Config := RepositoryConfig{
		RepositoryName: "dd-lib-java-init",
		RepositoryURL:  "gcr.io/datadoghq/dd-lib-java-init",
		Images: []ImageInfo{
			{
				Tag:              "latest",
				Digest:           "sha256:ghi789",
				CanonicalVersion: "2.1.3",
			},
		},
	}

	// Marshal configs to JSON
	repo1JSON, err := json.Marshal(repo1Config)
	require.NoError(t, err)
	repo2JSON, err := json.Marshal(repo2Config)
	require.NoError(t, err)

	t.Run("multiple_repositories", func(t *testing.T) {
		update := map[string]state.RawConfig{
			"config1": {Config: repo1JSON},
			"config2": {Config: repo2JSON},
		}

		appliedStatuses := make(map[string]state.ApplyStatus)
		applyStateCallback := func(cfgPath string, status state.ApplyStatus) {
			appliedStatuses[cfgPath] = status
		}

		resolver.processUpdate(update, applyStateCallback)

		// Verify both repositories are in the cache
		assert.Len(t, resolver.imageMappings, 2)
		assert.Contains(t, resolver.imageMappings, "dd-lib-python-init")
		assert.Contains(t, resolver.imageMappings, "dd-lib-java-init")

		// Verify python repo content
		pythonRepo := resolver.imageMappings["dd-lib-python-init"]
		assert.Len(t, pythonRepo, 2)
		assert.Contains(t, pythonRepo, "latest")
		assert.Contains(t, pythonRepo, "v3")
		assert.Equal(t, "gcr.io/datadoghq/dd-lib-python-init@sha256:abc123", pythonRepo["latest"].FullImageRef)

		// Verify java repo content
		javaRepo := resolver.imageMappings["dd-lib-java-init"]
		assert.Len(t, javaRepo, 1)
		assert.Contains(t, javaRepo, "latest")
		assert.Equal(t, "gcr.io/datadoghq/dd-lib-java-init@sha256:ghi789", javaRepo["latest"].FullImageRef)

		// Verify apply statuses
		assert.Len(t, appliedStatuses, 2)
		assert.Equal(t, state.ApplyStateAcknowledged, appliedStatuses["config1"].State)
		assert.Equal(t, state.ApplyStateAcknowledged, appliedStatuses["config2"].State)
	})

	t.Run("empty_update_clears_cache", func(t *testing.T) {
		update := map[string]state.RawConfig{}

		appliedStatuses := make(map[string]state.ApplyStatus)
		applyStateCallback := func(cfgPath string, status state.ApplyStatus) {
			appliedStatuses[cfgPath] = status
		}

		resolver.processUpdate(update, applyStateCallback)

		// Cache should be empty
		assert.Len(t, resolver.imageMappings, 0)
		assert.Len(t, appliedStatuses, 0)
	})

	t.Run("invalid_json", func(t *testing.T) {
		update := map[string]state.RawConfig{
			"invalid_config": {Config: []byte("invalid json")},
		}

		appliedStatuses := make(map[string]state.ApplyStatus)
		applyStateCallback := func(cfgPath string, status state.ApplyStatus) {
			appliedStatuses[cfgPath] = status
		}

		resolver.processUpdate(update, applyStateCallback)

		// Cache should be empty
		assert.Len(t, resolver.imageMappings, 0)

		// Error status should be recorded
		assert.Len(t, appliedStatuses, 1)
		assert.Equal(t, state.ApplyStateError, appliedStatuses["invalid_config"].State)
		assert.Contains(t, appliedStatuses["invalid_config"].Error, "invalid character")
	})
}

func TestRemoteConfigImageResolver_Resolve(t *testing.T) {
	resolver := &RemoteConfigImageResolver{
		imageMappings: map[string]map[string]ResolvedImage{
			"dd-lib-python-init": {
				"latest": {
					FullImageRef:     "gcr.io/datadoghq/dd-lib-python-init@sha256:abc123",
					Digest:           "sha256:abc123",
					CanonicalVersion: "1.0.0",
				},
				"v3": {
					FullImageRef:     "gcr.io/datadoghq/dd-lib-python-init@sha256:def456",
					Digest:           "sha256:def456",
					CanonicalVersion: "1.0.0",
				},
			},
		},
	}

	testCases := []struct {
		name           string
		imageRef       string
		expectedResult string
		expectedOK     bool
	}{
		{
			name:           "successful_resolution_latest",
			imageRef:       "gcr.io/datadoghq/dd-lib-python-init:latest",
			expectedResult: "gcr.io/datadoghq/dd-lib-python-init@sha256:abc123",
			expectedOK:     true,
		},
		{
			name:           "successful_resolution_versioned",
			imageRef:       "gcr.io/datadoghq/dd-lib-python-init:v3",
			expectedResult: "gcr.io/datadoghq/dd-lib-python-init@sha256:def456",
			expectedOK:     true,
		},
		{
			name:           "non_existent_repository",
			imageRef:       "gcr.io/datadoghq/dd-lib-java-init:latest",
			expectedResult: "",
			expectedOK:     false,
		},
		{
			name:           "non_existent_tag",
			imageRef:       "gcr.io/datadoghq/dd-lib-python-init:v2",
			expectedResult: "",
			expectedOK:     false,
		},
		{
			name:           "invalid_image_format",
			imageRef:       "invalid-image",
			expectedResult: "",
			expectedOK:     false,
		},
		{
			name:           "empty_image",
			imageRef:       "",
			expectedResult: "",
			expectedOK:     false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, ok := resolver.Resolve(tc.imageRef)
			assert.Equal(t, tc.expectedResult, result)
			assert.Equal(t, tc.expectedOK, ok)
		})
	}

	// Test empty cache
	t.Run("empty_cache", func(t *testing.T) {
		emptyResolver := &RemoteConfigImageResolver{
			imageMappings: make(map[string]map[string]ResolvedImage),
		}
		result, ok := emptyResolver.Resolve("gcr.io/datadoghq/dd-lib-python-init:latest")
		assert.Equal(t, "", result)
		assert.Equal(t, false, ok)
	})
}

func TestImageParseError(t *testing.T) {
	err := &ImageParseError{
		imageRef: "invalid-image",
		reason:   "test reason",
	}

	expected := "failed to parse image reference 'invalid-image': test reason"
	assert.Equal(t, expected, err.Error())
}
