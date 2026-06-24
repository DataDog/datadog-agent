// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy && (docker || containerd || crio)

package trivy

import (
	"testing"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	ftypes "github.com/aquasecurity/trivy/pkg/fanal/types"
	"github.com/stretchr/testify/assert"
)

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
