// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy && (docker || containerd || crio)

// Package trivy implement a simple overlayfs like filesystem to be able to
// scan through layered filesystems.
package trivy

import (
	"context"
	"errors"
	"fmt"
	"slices"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/ddtrivy"

	ftypes "github.com/aquasecurity/trivy/pkg/fanal/types"
)

// errLayerCountMismatch means the overlayfs mount (or the docker / crio
// equivalent) exposed a different number of layer paths than the image
// config has diff_ids. They are 1:1 by the OCI spec, so a divergence
// means an upstream invariant broke and we refuse to scan rather than
// guess a pairing.
var errLayerCountMismatch = errors.New("overlayfs mount layer count does not match image config")

// fakeContainer adapts a pre-paired set of LayerPaths into the
// ftypes.Container surface Trivy expects. It carries imgMeta separately
// so the CRI-O wrapper can read its History entries for ConfigFile().
type fakeContainer struct {
	imgMeta *workloadmeta.ContainerImageMetadata
	layers  []ftypes.LayerPath
}

// newFakeContainer wraps an already-paired LayerPath slice. The caller
// builds each {DiffID, Digest, Path} triple; this constructor does no
// further validation, so any desync here came from the caller.
func newFakeContainer(layers []ftypes.LayerPath, imgMeta *workloadmeta.ContainerImageMetadata) *fakeContainer {
	return &fakeContainer{imgMeta: imgMeta, layers: layers}
}

// diffIDs returns the DiffIDs of c's layers in image-config (bottom-up) order.
func (c *fakeContainer) diffIDs() []string {
	out := make([]string, len(c.layers))
	for i, l := range c.layers {
		out[i] = l.DiffID
	}
	return out
}

func (c *fakeContainer) LayerByDiffID(hash string) (ftypes.LayerPath, error) {
	for _, layer := range c.layers {
		if layer.DiffID == hash {
			return layer, nil
		}
	}
	return ftypes.LayerPath{}, errors.New("not found")
}

func (c *fakeContainer) LayerByDigest(hash string) (ftypes.LayerPath, error) {
	for _, layer := range c.layers {
		if layer.Digest == hash {
			return layer, nil
		}
	}
	return ftypes.LayerPath{}, errors.New("not found")
}

func (c *fakeContainer) Layers() []ftypes.LayerPath {
	return slices.Clone(c.layers)
}

func (c *Collector) scanOverlayFS(ctx context.Context, layers []string, ctr ftypes.Container, imgMeta *workloadmeta.ContainerImageMetadata, scanOptions sbom.ScanOptions) (*Report, error) {
	cache := newMemoryCache()

	log.Debugf("Generating SBOM for image %s using overlayfs %+v", imgMeta.ID, layers)

	artifactOptions := getDefaultArtifactOption(scanOptions)
	trivyReport, err := ddtrivy.ScanOverlays(ctx, artifactOptions, cache, ctr)
	if err != nil {
		return nil, fmt.Errorf("unable to scan overlayfs image, err: %w", err)
	}

	return c.buildReport(trivyReport, imgMeta.ID)
}
