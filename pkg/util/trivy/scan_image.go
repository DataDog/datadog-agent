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
	image2 "github.com/aquasecurity/trivy/pkg/fanal/artifact/image"
	ftypes "github.com/aquasecurity/trivy/pkg/fanal/types"

	"github.com/DataDog/datadog-agent/pkg/sbom"
)

func (c *Collector) scanImage(ctx context.Context, fanalImage ftypes.Image, scanOptions sbom.ScanOptions) (*Report, error) {
	cache := newMemoryCache()

	imageArtifact, err := image2.NewArtifact(fanalImage, cache, getDefaultArtifactOption(scanOptions))
	if err != nil {
		return nil, fmt.Errorf("unable to create artifact from image, err: %w", err)
	}

	trivyReport, err := c.scan(ctx, imageArtifact, applier.NewApplier(cache))
	if err != nil {
		return nil, fmt.Errorf("unable to marshal report to sbom format, err: %w", err)
	}

	return c.buildReport(trivyReport, trivyReport.Metadata.ImageID)
}
