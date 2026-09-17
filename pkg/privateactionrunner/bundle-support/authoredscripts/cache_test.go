// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package authoredscripts

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testPackageDigest = "ea7829a6ebdaa464eb4fbfff4c72e6e63176df58a430a4b0b8dfb66f0e57149c"

type testPackageSource struct {
	mu       sync.Mutex
	variant  string
	fetchErr error
	fetches  int
}

func (s *testPackageSource) Variant() string {
	return s.variant
}

func (s *testPackageSource) Fetch(_ context.Context, descriptor Descriptor, destination string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fetches++
	if s.fetchErr != nil {
		return s.fetchErr
	}

	scriptDir := filepath.Join(destination, scriptDirectory)
	if err := os.MkdirAll(scriptDir, 0o700); err != nil {
		return err
	}
	manifest := fmt.Sprintf(`{
  "schema-version": "v1",
  "version": %q,
  "fqn": %q,
  "command": {"entrypoint": "run.sh"}
}
`, descriptor.Version, descriptor.FQN)
	if err := os.WriteFile(filepath.Join(scriptDir, manifestFile), []byte(manifest), 0o600); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(scriptDir, "run.sh"), []byte("#!/bin/sh\n"), 0o700)
}

func (s *testPackageSource) fetchCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.fetches
}

func TestNewPackageCache(t *testing.T) {
	t.Run("requires source", func(t *testing.T) {
		cache, err := NewPackageCache(t.TempDir(), nil)
		require.ErrorContains(t, err, "source is required")
		assert.Nil(t, cache)
	})

	t.Run("requires source variant", func(t *testing.T) {
		cache, err := NewPackageCache(t.TempDir(), &testPackageSource{})
		require.ErrorContains(t, err, "variant is required")
		assert.Nil(t, cache)
	})
}

func TestPackageCacheResolveReusesValidArtifact(t *testing.T) {
	source := &testPackageSource{variant: "test-linux-amd64"}
	cache, err := NewPackageCache(filepath.Join(t.TempDir(), "cache"), source)
	require.NoError(t, err)
	descriptor := testCacheDescriptor()

	first, err := cache.Resolve(context.Background(), descriptor)
	require.NoError(t, err)
	second, err := cache.Resolve(context.Background(), descriptor)
	require.NoError(t, err)

	assert.Equal(t, first, second)
	assert.Equal(t, 1, source.fetchCount())
}

func TestPackageCacheResolveRepairsInvalidArtifact(t *testing.T) {
	source := &testPackageSource{variant: "test-linux-amd64"}
	cache, err := NewPackageCache(filepath.Join(t.TempDir(), "cache"), source)
	require.NoError(t, err)
	descriptor := testCacheDescriptor()

	artifact, err := cache.Resolve(context.Background(), descriptor)
	require.NoError(t, err)
	require.NoError(t, os.Remove(filepath.Join(artifact.Directory, scriptDirectory, "run.sh")))

	repaired, err := cache.Resolve(context.Background(), descriptor)
	require.NoError(t, err)
	assert.Equal(t, artifact.Directory, repaired.Directory)
	assert.Equal(t, 2, source.fetchCount())
}

func TestPackageCacheResolveDoesNotPublishFailedFetch(t *testing.T) {
	fetchErr := errors.New("download failed")
	source := &testPackageSource{variant: "test-linux-amd64", fetchErr: fetchErr}
	cache, err := NewPackageCache(filepath.Join(t.TempDir(), "cache"), source)
	require.NoError(t, err)

	artifact, err := cache.Resolve(context.Background(), testCacheDescriptor())
	require.ErrorIs(t, err, fetchErr)
	assert.Empty(t, artifact)
}

func TestPackageCacheResolveValidatesDescriptor(t *testing.T) {
	source := &testPackageSource{variant: "test-linux-amd64"}
	cache, err := NewPackageCache(filepath.Join(t.TempDir(), "cache"), source)
	require.NoError(t, err)

	_, err = cache.Resolve(context.Background(), Descriptor{FQN: "com.datadoghq.authoredscripts.testAction"})
	require.ErrorContains(t, err, "package is required")
	assert.Equal(t, 0, source.fetchCount())
}

func testCacheDescriptor() Descriptor {
	return Descriptor{
		FQN:     "com.datadoghq.authoredscripts.testAction",
		Package: "com.datadoghq.authoredscripts.testaction",
		Version: "1.2.3",
		URL:     "oci://registry.example.test/authored-script@sha256:" + testPackageDigest,
		SHA256:  testPackageDigest,
	}
}
