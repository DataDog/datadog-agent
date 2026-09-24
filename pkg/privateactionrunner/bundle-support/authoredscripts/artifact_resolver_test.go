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
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testPackageDigest = "ea7829a6ebdaa464eb4fbfff4c72e6e63176df58a430a4b0b8dfb66f0e57149c"

type testPackageMaterializer struct {
	mu                sync.Mutex
	materializationID string
	materializeErr    error
	materializes      int
}

func (m *testPackageMaterializer) MaterializationID() string {
	return m.materializationID
}

func (m *testPackageMaterializer) Materialize(_ context.Context, descriptor Descriptor, destination string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.materializes++
	if m.materializeErr != nil {
		return m.materializeErr
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

func (m *testPackageMaterializer) materializationCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.materializes
}

func TestNewArtifactResolver(t *testing.T) {
	t.Run("requires materializer", func(t *testing.T) {
		resolver, err := NewArtifactResolver(t.TempDir(), nil)
		require.ErrorContains(t, err, "materializer is required")
		assert.Nil(t, resolver)
	})

	t.Run("requires materialization ID", func(t *testing.T) {
		resolver, err := NewArtifactResolver(t.TempDir(), &testPackageMaterializer{})
		require.ErrorContains(t, err, "materialization ID is required")
		assert.Nil(t, resolver)
	})
}

func TestArtifactResolverResolveReusesValidArtifact(t *testing.T) {
	materializer := &testPackageMaterializer{materializationID: "test-linux-amd64"}
	cacheRoot := filepath.Join(t.TempDir(), "cache")
	resolver, err := NewArtifactResolver(cacheRoot, materializer)
	require.NoError(t, err)
	descriptor := testArtifactDescriptor()

	first, err := resolver.Resolve(context.Background(), descriptor)
	require.NoError(t, err)
	second, err := resolver.Resolve(context.Background(), descriptor)
	require.NoError(t, err)

	assert.Equal(t, first, second)
	assert.Equal(t, filepath.Join(cacheRoot, "artifacts", descriptor.Package, artifactDigestIDPrefix+descriptor.SHA256, materializer.materializationID), first.Directory)
	assert.Equal(t, 1, materializer.materializationCount())
}

func TestArtifactResolverResolveRepairsInvalidArtifact(t *testing.T) {
	materializer := &testPackageMaterializer{materializationID: "test-linux-amd64"}
	resolver, err := NewArtifactResolver(filepath.Join(t.TempDir(), "cache"), materializer)
	require.NoError(t, err)
	descriptor := testArtifactDescriptor()

	artifact, err := resolver.Resolve(context.Background(), descriptor)
	require.NoError(t, err)
	require.NoError(t, os.Remove(filepath.Join(artifact.Directory, scriptDirectory, "run.sh")))

	repaired, err := resolver.Resolve(context.Background(), descriptor)
	require.NoError(t, err)
	assert.Equal(t, artifact.Directory, repaired.Directory)
	assert.Equal(t, 2, materializer.materializationCount())
}

func TestArtifactResolverResolveTreatsFQNCaseInsensitively(t *testing.T) {
	materializer := &testPackageMaterializer{materializationID: "test-linux-amd64"}
	resolver, err := NewArtifactResolver(filepath.Join(t.TempDir(), "cache"), materializer)
	require.NoError(t, err)
	descriptor := testArtifactDescriptor()

	artifact, err := resolver.Resolve(context.Background(), descriptor)
	require.NoError(t, err)

	lowercaseDescriptor := descriptor
	lowercaseDescriptor.FQN = strings.ToLower(descriptor.FQN)
	resolved, err := resolver.Resolve(context.Background(), lowercaseDescriptor)
	require.NoError(t, err)
	assert.Equal(t, artifact, resolved)
	assert.Equal(t, 1, materializer.materializationCount())
}

func TestArtifactResolverResolveDoesNotPublishFailedMaterialization(t *testing.T) {
	materializeErr := errors.New("download failed")
	materializer := &testPackageMaterializer{materializationID: "test-linux-amd64", materializeErr: materializeErr}
	resolver, err := NewArtifactResolver(filepath.Join(t.TempDir(), "cache"), materializer)
	require.NoError(t, err)

	artifact, err := resolver.Resolve(context.Background(), testArtifactDescriptor())
	require.ErrorIs(t, err, materializeErr)
	assert.Empty(t, artifact)
}

func TestArtifactResolverResolveValidatesDescriptor(t *testing.T) {
	materializer := &testPackageMaterializer{materializationID: "test-linux-amd64"}
	resolver, err := NewArtifactResolver(filepath.Join(t.TempDir(), "cache"), materializer)
	require.NoError(t, err)

	_, err = resolver.Resolve(context.Background(), Descriptor{FQN: "com.datadoghq.authoredscripts.testAction"})
	require.ErrorContains(t, err, "package is required")
	assert.Equal(t, 0, materializer.materializationCount())
}

func testArtifactDescriptor() Descriptor {
	return Descriptor{
		FQN:     "com.datadoghq.authoredscripts.testAction",
		Package: "com.datadoghq.authoredscripts.testaction",
		Version: "1.2.3",
		URL:     "oci://registry.example.test/authored-script@sha256:" + testPackageDigest,
		SHA256:  testPackageDigest,
	}
}
