// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy

// Package trivy implement a simple overlayfs like filesystem to be able to
// scan through layered filesystems.
package trivy

import (
	"context"
	"errors"
	"fmt"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/aquasecurity/trivy/pkg/fanal/applier"
	local "github.com/aquasecurity/trivy/pkg/fanal/artifact/container"
	ftypes "github.com/aquasecurity/trivy/pkg/fanal/types"
	v1 "github.com/google/go-containerregistry/pkg/v1"
)

type fakeContainer struct {
	layerIDs   []string
	imgMeta    *workloadmeta.ContainerImageMetadata
	layerPaths []string
}

func (c *fakeContainer) LayerByDiffID(hash string) (ftypes.LayerPath, error) {
	for i, layer := range c.layerIDs {
		diffID, _ := v1.NewHash(layer)
		if diffID.String() == hash {
			return ftypes.LayerPath{
				DiffID: diffID.String(),
				Path:   c.layerPaths[i],
				Digest: c.imgMeta.Layers[i].Digest,
			}, nil
		}
	}
	return ftypes.LayerPath{}, errors.New("not found")
}

func (c *fakeContainer) LayerByDigest(hash string) (ftypes.LayerPath, error) {
	for i, layer := range c.layerIDs {
		diffID, _ := v1.NewHash(layer)
		if hash == c.imgMeta.Layers[i].Digest {
			return ftypes.LayerPath{
				DiffID: diffID.String(),
				Path:   c.layerPaths[i],
				Digest: c.imgMeta.Layers[i].Digest,
			}, nil
		}
	}
	return ftypes.LayerPath{}, errors.New("not found")
}

func (c *fakeContainer) Layers() (layers []ftypes.LayerPath) {
	for i, layer := range c.layerIDs {
		diffID, _ := v1.NewHash(layer)
		layers = append(layers, ftypes.LayerPath{
			DiffID: diffID.String(),
			Path:   c.layerPaths[i],
			Digest: c.imgMeta.Layers[i].Digest,
		})
	}

	return layers
}

func (c *Collector) scanOverlayFS(ctx context.Context, layers []string, ctr ftypes.Container, imgMeta *workloadmeta.ContainerImageMetadata, scanOptions sbom.ScanOptions) (sbom.Report, error) {
	cache, err := c.getCache()
	if err != nil {
		return nil, err
	}

	if cache == nil {
		return nil, errors.New("failed to get cache for scan")
	}

	containerArtifact, err := local.NewArtifact(ctr, cache, NewFSWalker(), getDefaultArtifactOption(scanOptions))
	if err != nil {
		return nil, err
	}

	log.Debugf("Generating SBOM for image %s using overlayfs %+v", imgMeta.ID, layers)

	trivyReport, err := c.scan(ctx, containerArtifact, applier.NewApplier(cache), imgMeta, cache, false)
	if err != nil {
		if imgMeta != nil {
			return nil, fmt.Errorf("unable to marshal report to sbom format for image %s, err: %w", imgMeta.ID, err)
		}
		return nil, fmt.Errorf("unable to marshal report to sbom format, err: %w", err)
	}

	log.Debugf("Found OS: %+v", trivyReport.Metadata.OS)
	pkgCount := 0
	for _, results := range trivyReport.Results {
		pkgCount += len(results.Packages)
	}
	log.Debugf("Found %d packages", pkgCount)

	return &Report{
		Report:    trivyReport,
		id:        imgMeta.ID,
		marshaler: c.marshaler,
	}, nil
}
