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

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/ddtrivy"

	ftypes "github.com/aquasecurity/trivy/pkg/fanal/types"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/samber/lo"
)

type fakeContainer struct {
	layerIDs   []string
	imgMeta    *workloadmeta.ContainerImageMetadata
	layerPaths []string
	layers     []ftypes.LayerPath
}

// layerIDs and layerPaths must both be in image-config order (bottom-up);
// paths from overlay mount syntax must be reversed by the caller, else each
// DiffID is paired with the wrong layer and trivy's DiffID-keyed blob cache
// gets poisoned. Empty-layer history markers in imgMeta.Layers are filtered
// to keep the i-th non-empty entry aligned with layerIDs[i].
func newFakeContainer(layerPaths []string, imgMeta *workloadmeta.ContainerImageMetadata, layerIDs []string) (*fakeContainer, error) {
	imageLayers := lo.Filter(imgMeta.Layers, func(layer workloadmeta.ContainerImageLayer, _ int) bool {
		return layer.Digest != ""
	})
	// Path / DiffID alignment must be exact: these are what trivy looks up.
	// imageLayers feeds the optional Digest annotation; tolerate the Docker
	// fallback in layersFromDockerHistoryAndInspect that can emit more
	// non-empty entries than RootFS.Layers.
	if len(layerIDs) != len(layerPaths) || len(layerIDs) > len(imageLayers) {
		return nil, fmt.Errorf("mismatch count for layer IDs, paths and image layers (ids=%d, paths=%d, layers=%d)",
			len(layerIDs), len(layerPaths), len(imageLayers))
	}

	log.Debugf("create fake container with paths=%v", layerPaths)

	layers := make([]ftypes.LayerPath, len(layerIDs))
	for i, id := range layerIDs {
		diffID, _ := v1.NewHash(id)
		layers[i] = ftypes.LayerPath{
			DiffID: diffID.String(),
			Path:   layerPaths[i],
			Digest: imageLayers[i].Digest,
		}
	}

	return &fakeContainer{
		layerIDs:   layerIDs,
		imgMeta:    imgMeta,
		layerPaths: layerPaths,
		layers:     layers,
	}, nil
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
	return c.layers
}

func (c *Collector) scanOverlayFS(ctx context.Context, layers []string, ctr ftypes.Container, imgMeta *workloadmeta.ContainerImageMetadata, scanOptions sbom.ScanOptions) (*Report, error) {
	var cache CacheWithCleaner
	if pkgconfigsetup.Datadog().GetBool("sbom.container_image.overlayfs_disable_cache") {
		cache = newMemoryCache()
	} else {
		globalCache, err := c.GetCache()
		if err != nil {
			return nil, err
		}
		cache = globalCache
	}

	if cache == nil {
		return nil, errors.New("failed to get cache for scan")
	}

	log.Debugf("Generating SBOM for image %s using overlayfs %+v", imgMeta.ID, layers)

	artifactOptions := getDefaultArtifactOption(scanOptions)
	trivyReport, err := ddtrivy.ScanOverlays(ctx, artifactOptions, cache, ctr)
	if err != nil {
		return nil, fmt.Errorf("unable to scan overlayfs image, err: %w", err)
	}

	return c.buildReport(trivyReport, imgMeta.ID)
}
