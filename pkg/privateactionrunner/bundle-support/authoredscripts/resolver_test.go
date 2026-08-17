// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package authoredscripts

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolve_SelectsHighestVersion(t *testing.T) {
	cacheRoot := t.TempDir()
	writeCachedPackage(t, cacheRoot, testFQN, testDigest("a"), "1.9.0")
	writeCachedPackage(t, cacheRoot, testFQN, testDigest("b"), "1.10.0")
	writeCachedPackage(t, cacheRoot, testFQN, testDigest("c"), "1.2.0")

	digest, err := NewCacheResolver(cacheRoot).Resolve(testFQN)

	require.NoError(t, err)
	assert.Equal(t, testDigest("b"), digest)
}

func TestResolve_PrefersReleaseOverPrerelease(t *testing.T) {
	cacheRoot := t.TempDir()
	writeCachedPackage(t, cacheRoot, testFQN, testDigest("a"), "2.0.0-rc1")
	writeCachedPackage(t, cacheRoot, testFQN, testDigest("b"), "2.0.0")

	digest, err := NewCacheResolver(cacheRoot).Resolve(testFQN)

	require.NoError(t, err)
	assert.Equal(t, testDigest("b"), digest)
}

func TestResolve_IsStableWhenVersionsAreEqual(t *testing.T) {
	cacheRoot := t.TempDir()
	writeCachedPackage(t, cacheRoot, testFQN, testDigest("a"), "1.0.0")
	writeCachedPackage(t, cacheRoot, testFQN, testDigest("f"), "1.0.0")

	resolver := NewCacheResolver(cacheRoot)
	first, err := resolver.Resolve(testFQN)
	require.NoError(t, err)
	second, err := resolver.Resolve(testFQN)
	require.NoError(t, err)

	assert.Equal(t, first, second)
	assert.Equal(t, testDigest("f"), first)
}

func TestResolve_IgnoresPackageMissingCompletionMarker(t *testing.T) {
	cacheRoot := t.TempDir()
	writeCachedPackage(t, cacheRoot, testFQN, testDigest("a"), "1.0.0")
	incomplete := writeCachedPackage(t, cacheRoot, testFQN, testDigest("b"), "2.0.0")
	require.NoError(t, os.Remove(filepath.Join(incomplete, completionMarker)))

	digest, err := NewCacheResolver(cacheRoot).Resolve(testFQN)

	require.NoError(t, err)
	assert.Equal(t, testDigest("a"), digest)
}

func TestResolve_IgnoresPackageForAnotherPlatform(t *testing.T) {
	cacheRoot := t.TempDir()
	writeCachedPackage(t, cacheRoot, testFQN, testDigest("a"), "1.0.0")
	foreign := packageDirectory(cacheRoot, testFQN, testDigest("b"), "solaris-s390x")
	require.NoError(t, os.MkdirAll(foreign, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(foreign, completionMarker), nil, 0o644))

	digest, err := NewCacheResolver(cacheRoot).Resolve(testFQN)

	require.NoError(t, err)
	assert.Equal(t, testDigest("a"), digest)
}

func TestResolve_IgnoresPackageDeclaringAnotherAction(t *testing.T) {
	cacheRoot := t.TempDir()
	directory := writeCachedPackage(t, cacheRoot, testFQN, testDigest("a"), "2.0.0")
	require.NoError(t, os.WriteFile(filepath.Join(directory, scriptDirectory, manifestFile),
		[]byte(packageManifest("com.datadoghq.authoredscripts.echo", "2.0.0", nil)), 0o644))
	writeCachedPackage(t, cacheRoot, testFQN, testDigest("b"), "1.0.0")

	digest, err := NewCacheResolver(cacheRoot).Resolve(testFQN)

	require.NoError(t, err)
	assert.Equal(t, testDigest("b"), digest)
}

func TestResolve_IgnoresPackageWithUnparseableVersion(t *testing.T) {
	cacheRoot := t.TempDir()
	directory := writeCachedPackage(t, cacheRoot, testFQN, testDigest("a"), "1.0.0")
	require.NoError(t, os.WriteFile(filepath.Join(directory, scriptDirectory, manifestFile),
		[]byte(packageManifest(testFQN, "not-a-version", nil)), 0o644))
	writeCachedPackage(t, cacheRoot, testFQN, testDigest("b"), "0.0.1")

	digest, err := NewCacheResolver(cacheRoot).Resolve(testFQN)

	require.NoError(t, err)
	assert.Equal(t, testDigest("b"), digest)
}

func TestResolve_IgnoresEntriesThatAreNotDigests(t *testing.T) {
	cacheRoot := t.TempDir()
	digestsDirectory := packageDigestsDirectory(cacheRoot, testFQN)
	require.NoError(t, os.MkdirAll(filepath.Join(digestsDirectory, "staging"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(digestsDirectory, testDigest("a")), nil, 0o644))
	writeCachedPackage(t, cacheRoot, testFQN, testDigest("b"), "1.0.0")

	digest, err := NewCacheResolver(cacheRoot).Resolve(testFQN)

	require.NoError(t, err)
	assert.Equal(t, testDigest("b"), digest)
}

func TestResolve_ReportsNotCachedWhenActionIsUnknown(t *testing.T) {
	_, err := NewCacheResolver(t.TempDir()).Resolve(testFQN)

	assert.ErrorIs(t, err, ErrNotCached)
}

func TestResolve_ReportsNotCachedWhenNoPackageIsUsable(t *testing.T) {
	cacheRoot := t.TempDir()
	directory := writeCachedPackage(t, cacheRoot, testFQN, testDigest("a"), "1.0.0")
	require.NoError(t, os.Remove(filepath.Join(directory, completionMarker)))

	_, err := NewCacheResolver(cacheRoot).Resolve(testFQN)

	assert.ErrorIs(t, err, ErrNotCached)
}

func TestResolve_RejectsInvalidActionName(t *testing.T) {
	_, err := NewCacheResolver(t.TempDir()).Resolve("../../etc")

	assert.ErrorContains(t, err, "is not a valid authored-script action name")
}
