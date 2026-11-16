// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy && (docker || containerd || crio)

package trivy

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	ftypes "github.com/aquasecurity/trivy/pkg/fanal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckLayers(t *testing.T) {
	empty := t.TempDir()
	full := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(full, "os-release"), []byte("ID=alpine\n"), 0o644))
	missing := filepath.Join(t.TempDir(), "missing")

	assert.NoError(t, checkLayers([]string{empty, full}))

	err := checkLayers([]string{full, missing})
	assert.ErrorIs(t, err, ErrLayerUnreadable)
	assert.ErrorIs(t, err, sbom.ErrScanNotSupported)
	assert.ErrorIs(t, err, fs.ErrNotExist)
	assert.ErrorContains(t, err, missing)

	// Any other failure, here a layer that is a file, stays retryable.
	file := filepath.Join(full, "os-release")
	err = checkLayers([]string{file})
	assert.ErrorContains(t, err, file)
	assert.NotErrorIs(t, err, sbom.ErrScanNotSupported)
}

func TestCheckLayersPermission(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads a directory whatever its mode")
	}
	closed := t.TempDir()
	require.NoError(t, os.Chmod(closed, 0))
	t.Cleanup(func() { assert.NoError(t, os.Chmod(closed, 0o700)) })

	err := checkLayers([]string{closed})
	assert.ErrorIs(t, err, ErrLayerUnreadable)
	assert.ErrorIs(t, err, fs.ErrPermission)
}

func TestFakeContainerLookups(t *testing.T) {
	const snapshotsBase = "/host/root/var/lib/containerd/io.containerd.snapshotter.v1.nydus/snapshots"
	diffIDs := []string{
		"sha256:d543b8cad89e3428ac8852a13cb2dbfaf55b1e10fd95a9753e51faf393d60e81",
		"sha256:7b1349c98b6929b220fd1ed89c7255c6a8a7557c448cf7fda8dcfbad37fa4549",
		"sha256:1608fb7c693049b6edc16af0542d923cd8542f7039bfca161600e9142055dcfb",
	}
	digests := []string{
		"sha256:1111111111111111111111111111111111111111111111111111111111111111",
		"sha256:2222222222222222222222222222222222222222222222222222222222222222",
		"sha256:3333333333333333333333333333333333333333333333333333333333333333",
	}
	paths := []string{
		snapshotsBase + "/134/fs",
		snapshotsBase + "/135/fs",
		snapshotsBase + "/136/fs",
	}

	layers := make([]ftypes.LayerPath, len(diffIDs))
	for i := range diffIDs {
		layers[i] = ftypes.LayerPath{DiffID: diffIDs[i], Digest: digests[i], Path: paths[i]}
	}
	c := newFakeContainer(layers, &workloadmeta.ContainerImageMetadata{})

	for i, id := range diffIDs {
		got, err := c.LayerByDiffID(id)
		if !assert.NoErrorf(t, err, "LayerByDiffID(%s)", id) {
			continue
		}
		assert.Equal(t, paths[i], got.Path)
		assert.Equal(t, digests[i], got.Digest)
	}

	for i, d := range digests {
		got, err := c.LayerByDigest(d)
		if !assert.NoErrorf(t, err, "LayerByDigest(%s)", d) {
			continue
		}
		assert.Equal(t, diffIDs[i], got.DiffID)
	}

	_, err := c.LayerByDiffID("sha256:doesnotexist")
	assert.Error(t, err)
}
