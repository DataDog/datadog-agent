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
	"io"
	"io/fs"
	"os"
	"slices"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/ddtrivy"

	trivycache "github.com/aquasecurity/trivy/pkg/cache"
	"github.com/aquasecurity/trivy/pkg/fanal/artifact"
	ftypes "github.com/aquasecurity/trivy/pkg/fanal/types"
	"github.com/aquasecurity/trivy/pkg/types"
)

// errLayerCountMismatch means the overlayfs mount (or the docker / crio
// equivalent) exposed a different number of layer paths than the image
// config has diff_ids, as the view of a nydus image does with the layers
// nydusd merges into one directory. We refuse to scan rather than guess a
// pairing. A retry finds the same layout, so the error wraps
// sbom.ErrScanNotSupported and the scanner drops the request.
var errLayerCountMismatch = fmt.Errorf("%w: overlayfs mount layer count does not match image config", sbom.ErrScanNotSupported)

// errLayerUnreadable means the Agent cannot list the directory of an image
// layer, as when the container runtime's storage is missing from the Agent
// container or closed to the Agent's user. The walk of a layer skips what it
// cannot read, so the layer would scan as an empty one. A retry finds the
// same mounts, so the error wraps sbom.ErrScanNotSupported.
var errLayerUnreadable = fmt.Errorf("%w: cannot read image layer", sbom.ErrScanNotSupported)

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

// checkLayers returns an error for the first of layers whose directory cannot
// be opened and listed. The error wraps errLayerUnreadable when the directory
// is missing or closed to the Agent.
func checkLayers(layers []string) error {
	for _, layer := range layers {
		f, err := os.Open(layer)
		if err == nil {
			_, err = f.ReadDir(1)
			f.Close()
		}
		if err == nil || err == io.EOF {
			continue
		}
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrPermission) {
			return fmt.Errorf("%w: %w", errLayerUnreadable, err)
		}
		return fmt.Errorf("unable to read image layer: %w", err)
	}
	return nil
}

func (c *Collector) scanOverlayFS(ctx context.Context, layers []string, ctr ftypes.Container, imgMeta *workloadmeta.ContainerImageMetadata, scanOptions sbom.ScanOptions) (*Report, error) {
	log.Debugf("Generating SBOM for image %s using overlayfs %+v", imgMeta.ID, layers)

	if err := checkLayers(layers); err != nil {
		return nil, err
	}

	trivyReport, osFallback, err := scanWithOSFallback(ctx, imgMeta.ID, scanOptions, func(ctx context.Context, artifactOption artifact.Option) (*types.Report, error) {
		return ddtrivy.ScanOverlays(ctx, artifactOption, trivycache.NewMemoryCache(), ctr)
	})
	if err != nil {
		return nil, fmt.Errorf("unable to scan overlayfs image, err: %w", err)
	}

	return c.buildReport(trivyReport, imgMeta.ID, osFallback)
}
