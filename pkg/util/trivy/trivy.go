// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trivy

import (
	"context"
	"fmt"

	cyclonedxgo "github.com/CycloneDX/cyclonedx-go"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	"github.com/aquasecurity/trivy-db/pkg/db"
	"github.com/aquasecurity/trivy/pkg/commands/operation"
	"github.com/aquasecurity/trivy/pkg/detector/ospkg"
	"github.com/aquasecurity/trivy/pkg/fanal/analyzer"
	"github.com/aquasecurity/trivy/pkg/fanal/applier"
	"github.com/aquasecurity/trivy/pkg/fanal/artifact"
	image2 "github.com/aquasecurity/trivy/pkg/fanal/artifact/image"
	"github.com/aquasecurity/trivy/pkg/fanal/cache"
	ftypes "github.com/aquasecurity/trivy/pkg/fanal/types"
	"github.com/aquasecurity/trivy/pkg/flag"
	"github.com/aquasecurity/trivy/pkg/sbom/cyclonedx"
	"github.com/aquasecurity/trivy/pkg/scanner"
	"github.com/aquasecurity/trivy/pkg/scanner/local"
	"github.com/aquasecurity/trivy/pkg/types"
	"github.com/aquasecurity/trivy/pkg/vulnerability"
	"github.com/containerd/containerd"
)

// Collector interface
type Collector interface {
	ScanContainerdImage(ctx context.Context, imageMeta *workloadmeta.ContainerImageMetadata, img containerd.Image) (*cyclonedxgo.BOM, error)
}

// CollectorConfig allows to pass configuration
type CollectorConfig struct {
	ArtifactCache      cache.ArtifactCache
	LocalArtifactCache cache.LocalArtifactCache
	ArtifactOption     artifact.Option

	ContainerdAccessor func() (*containerd.Client, error)
}

// Collector uses trivy to generate a SBOM
type collector struct {
	config     CollectorConfig
	applier    local.Applier
	detector   local.OspkgDetector
	dbConfig   db.Config
	vulnClient vulnerability.Client
	marshaler  *cyclonedx.Marshaler
}

// DefaultCollectorConfig returns a default collector configuration
// However, accessors still need to be filled in externally
func DefaultCollectorConfig() (CollectorConfig, error) {
	cache, err := operation.NewCache(flag.CacheOptions{CacheBackend: "fs"})
	if err != nil {
		return CollectorConfig{}, err
	}

	return CollectorConfig{
		ArtifactCache:      cache,
		LocalArtifactCache: cache,
		ArtifactOption: artifact.Option{
			Offline:           true,
			NoProgress:        true,
			DisabledAnalyzers: DefaultDisabledCollectors(),
			Slow:              true,
			SBOMSources:       []string{},
			DisabledHandlers:  DefaultDisabledHandlers(),
		},
	}, nil
}

func DefaultDisabledCollectors() []analyzer.Type {
	disabledAnalyzers := make([]analyzer.Type, 0)
	disabledAnalyzers = append(disabledAnalyzers, analyzer.TypeLanguages...)
	disabledAnalyzers = append(disabledAnalyzers, analyzer.TypeSecret)
	disabledAnalyzers = append(disabledAnalyzers, analyzer.TypeConfigFiles...)
	disabledAnalyzers = append(disabledAnalyzers, analyzer.TypeLicenseFile)
	return disabledAnalyzers
}

func DefaultDisabledHandlers() []ftypes.HandlerType {
	return []ftypes.HandlerType{ftypes.UnpackagedPostHandler}
}

func NewCollector(collectorConfig CollectorConfig) (Collector, error) {
	dbConfig := db.Config{}

	return &collector{
		config:     collectorConfig,
		applier:    applier.NewApplier(collectorConfig.LocalArtifactCache),
		detector:   ospkg.Detector{},
		dbConfig:   dbConfig,
		vulnClient: vulnerability.NewClient(dbConfig),
		marshaler:  cyclonedx.NewMarshaler(""),
	}, nil
}

func (c *collector) ScanContainerdImage(ctx context.Context, imgMeta *workloadmeta.ContainerImageMetadata, img containerd.Image) (*cyclonedxgo.BOM, error) {
	client, err := c.config.ContainerdAccessor()
	if err != nil {
		return nil, fmt.Errorf("unable to access containerd client, err: %w", err)
	}

	fanalImage, cleanup, err := convertContainerdImage(ctx, client, imgMeta, img)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		return nil, fmt.Errorf("unable to convert containerd image, err: %w", err)
	}

	bom, err := c.scan(ctx, fanalImage)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal report to sbom format, err: %w", err)
	}

	return bom, nil
}

func (c *collector) scan(ctx context.Context, image ftypes.Image) (*cyclonedxgo.BOM, error) {
	artifact, err := image2.NewArtifact(image, c.config.ArtifactCache, c.config.ArtifactOption)
	if err != nil {
		return nil, err
	}

	s := scanner.NewScanner(local.NewScanner(c.applier, c.detector, c.vulnClient), artifact)
	report, err := s.ScanArtifact(ctx, types.ScanOptions{
		VulnType:            []string{},
		SecurityChecks:      []string{},
		ScanRemovedPackages: false,
		ListAllPackages:     true,
	})
	if err != nil {
		return nil, err
	}

	return c.marshaler.Marshal(report)
}
