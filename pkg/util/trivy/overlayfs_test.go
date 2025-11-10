// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy && (docker || containerd || crio)

package trivy

import (
	"testing"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/stretchr/testify/assert"
)

func TestFakeContainer(t *testing.T) {
	tests := []struct {
		name               string
		layersPath         []string
		imageMeta          *workloadmeta.ContainerImageMetadata
		layerIDs           []string
		requestedLayerDiff string
		expectedLayerPath  string
	}{
		{
			layersPath: []string{
				"/host/root/var/lib/containerd/io.containerd.snapshotter.v1.nydus/snapshots/148/fs",
				"/host/root/var/lib/containerd/io.containerd.snapshotter.v1.nydus/snapshots/147/fs",
				"/host/root/var/lib/containerd/io.containerd.snapshotter.v1.nydus/snapshots/146/fs",
				"/host/root/var/lib/containerd/io.containerd.snapshotter.v1.nydus/snapshots/145/fs",
				"/host/root/var/lib/containerd/io.containerd.snapshotter.v1.nydus/snapshots/144/fs",
				"/host/root/var/lib/containerd/io.containerd.snapshotter.v1.nydus/snapshots/143/fs",
				"/host/root/var/lib/containerd/io.containerd.snapshotter.v1.nydus/snapshots/142/fs",
				"/host/root/var/lib/containerd/io.containerd.snapshotter.v1.nydus/snapshots/141/fs",
				"/host/root/var/lib/containerd/io.containerd.snapshotter.v1.nydus/snapshots/140/fs",
				"/host/root/var/lib/containerd/io.containerd.snapshotter.v1.nydus/snapshots/139/fs",
				"/host/root/var/lib/containerd/io.containerd.snapshotter.v1.nydus/snapshots/138/fs",
				"/host/root/var/lib/containerd/io.containerd.snapshotter.v1.nydus/snapshots/137/fs",
				"/host/root/var/lib/containerd/io.containerd.snapshotter.v1.nydus/snapshots/136/fs",
				"/host/root/var/lib/containerd/io.containerd.snapshotter.v1.nydus/snapshots/135/fs",
				"/host/root/var/lib/containerd/io.containerd.snapshotter.v1.nydus/snapshots/134/fs",
			},
			imageMeta: &workloadmeta.ContainerImageMetadata{
				Layers: []workloadmeta.ContainerImageLayer{
					{
						Digest: "", // empty layer
					},
					{
						Digest: "sha256:d543b8cad89e3428ac8852a13cb2dbfaf55b1e10fd95a9753e51faf393d60e81",
					},
					{
						Digest: "sha256:7b1349c98b6929b220fd1ed89c7255c6a8a7557c448cf7fda8dcfbad37fa4549",
					},
					{
						Digest: "sha256:1608fb7c693049b6edc16af0542d923cd8542f7039bfca161600e9142055dcfb",
					},
					{
						Digest: "sha256:3c516036f954ffb308f505c5bbea2099110f84d3f7b76940e4eade80691194b7",
					},
					{
						Digest: "sha256:a2ff3eeada006822f19570505fa99db3e6410addac86394fba4ba9304230cfeb",
					},
					{
						Digest: "sha256:20bde7516cdbeccc4625f1691140d63ed8e3fc46a0f593163b48020de88b8c3e",
					},
					{
						Digest: "sha256:b7d0da6041ab2b567cab384374652b82beddd560b7d59540463a7ce3e600274c",
					},
					{
						Digest: "sha256:9b33b108c42d01575feac410e94f500e01cf2552df064a019a7b27184d8371ff",
					},
					{
						Digest: "sha256:24586912362b7671f6b5e8e1a78194f340653f1df0376cd415e09832b2be40fb",
					},
					{
						Digest: "sha256:384eaa6554ab5ccf7d8462698e5b86b98014aa1c8c8a7d8457f8854148941aad",
					},
					{
						Digest: "sha256:1a651b5627b77ee65c117e801b759506cc6f4c907a0784aa0e0280e001386df8",
					},
					{
						Digest: "sha256:0c11dc029b3c6cd8b02788f49f0d0a9bbc353cebb900869fd401e74f2e0ac8fd",
					},
					{
						Digest: "sha256:f8a6696ef6df5fb297d4f1cd3ef877c836a3e269b566b415f47cf3cdf5b14c97",
					},
					{
						Digest: "sha256:7c6bc12b66c3f9059ff7c6665f75916c2533e829476ff0ec351850f9d3f32233",
					},
					{
						Digest: "sha256:873b790faaaaa245c52fba066b70c5a7b3d427d2068d72756738c7741b4b7031",
					},
				},
			},
			layerIDs: []string{
				"sha256:d543b8cad89e3428ac8852a13cb2dbfaf55b1e10fd95a9753e51faf393d60e81",
				"sha256:7b1349c98b6929b220fd1ed89c7255c6a8a7557c448cf7fda8dcfbad37fa4549",
				"sha256:1608fb7c693049b6edc16af0542d923cd8542f7039bfca161600e9142055dcfb",
				"sha256:3c516036f954ffb308f505c5bbea2099110f84d3f7b76940e4eade80691194b7",
				"sha256:a2ff3eeada006822f19570505fa99db3e6410addac86394fba4ba9304230cfeb",
				"sha256:20bde7516cdbeccc4625f1691140d63ed8e3fc46a0f593163b48020de88b8c3e",
				"sha256:b7d0da6041ab2b567cab384374652b82beddd560b7d59540463a7ce3e600274c",
				"sha256:9b33b108c42d01575feac410e94f500e01cf2552df064a019a7b27184d8371ff",
				"sha256:24586912362b7671f6b5e8e1a78194f340653f1df0376cd415e09832b2be40fb",
				"sha256:384eaa6554ab5ccf7d8462698e5b86b98014aa1c8c8a7d8457f8854148941aad",
				"sha256:1a651b5627b77ee65c117e801b759506cc6f4c907a0784aa0e0280e001386df8",
				"sha256:0c11dc029b3c6cd8b02788f49f0d0a9bbc353cebb900869fd401e74f2e0ac8fd",
				"sha256:f8a6696ef6df5fb297d4f1cd3ef877c836a3e269b566b415f47cf3cdf5b14c97",
				"sha256:7c6bc12b66c3f9059ff7c6665f75916c2533e829476ff0ec351850f9d3f32233",
				"sha256:873b790faaaaa245c52fba066b70c5a7b3d427d2068d72756738c7741b4b7031",
			},
			requestedLayerDiff: "sha256:873b790faaaaa245c52fba066b70c5a7b3d427d2068d72756738c7741b4b7031",
			expectedLayerPath:  "/host/root/var/lib/containerd/io.containerd.snapshotter.v1.nydus/snapshots/134/fs",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fakeContainer, err := newFakeContainer(test.layersPath, test.imageMeta, test.layerIDs)
			if err != nil {
				t.Error(err)
			}

			layerPath, err := fakeContainer.LayerByDiffID(test.requestedLayerDiff)
			if err != nil {
				t.Error(err)
			}

			assert.Equal(t, layerPath.Path, test.expectedLayerPath)
		})
	}
}
