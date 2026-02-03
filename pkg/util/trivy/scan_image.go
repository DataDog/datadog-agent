// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy && (docker || containerd || crio)

// Package trivy holds the scan components
package trivy

import (
	"context"
	"fmt"

	"github.com/aquasecurity/trivy/pkg/fanal/applier"
	"github.com/aquasecurity/trivy/pkg/fanal/artifact"
	image2 "github.com/aquasecurity/trivy/pkg/fanal/artifact/image"
	ftypes "github.com/aquasecurity/trivy/pkg/fanal/types"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/sbom"
)

func (c *Collector) fixupCacheKeyForImgMeta(ctx context.Context, artifact artifact.Artifact, imgMeta *workloadmeta.ContainerImageMetadata, cache CacheWithCleaner) error {
	// The artifact reference is only needed to clean up the blobs after the scan.
	// It is re-generated from cached partial results during the scan.
	artifactReference, err := artifact.Inspect(ctx)
	if err != nil {
		return err
	}

	cache.setKeysForEntity(imgMeta.EntityID.ID, append(artifactReference.BlobIDs, artifactReference.ID))
	return nil
}

func (c *Collector) scanImage(ctx context.Context, fanalImage ftypes.Image, imgMeta *workloadmeta.ContainerImageMetadata, scanOptions sbom.ScanOptions) (*Report, error) {
	cache, err := c.GetCache()
	if err != nil {
		return nil, err
	}

	if cache == nil {
		return nil, fmt.Errorf("cache is not available")
	}

	imageArtifact, err := image2.NewArtifact(fanalImage, cache, getDefaultArtifactOption(scanOptions))
	if err != nil {
		return nil, fmt.Errorf("unable to create artifact from image, err: %w", err)
	}

	if err := c.fixupCacheKeyForImgMeta(ctx, imageArtifact, imgMeta, cache); err != nil {
		return nil, fmt.Errorf("unable to fixup cache key for image, err: %w", err)
	}

	trivyReport, err := c.scan(ctx, imageArtifact, applier.NewApplier(cache))
	if err != nil {
		return nil, fmt.Errorf("unable to marshal report to sbom format, err: %w", err)
	}

	return c.buildReport(trivyReport, trivyReport.Metadata.ImageID)
}
