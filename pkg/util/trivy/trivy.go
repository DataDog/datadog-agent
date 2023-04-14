// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy
// +build trivy

package trivy

import (
	"context"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	containerdUtil "github.com/DataDog/datadog-agent/pkg/util/containerd"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"

	"github.com/aquasecurity/trivy-db/pkg/db"
	"github.com/aquasecurity/trivy/pkg/detector/ospkg"
	"github.com/aquasecurity/trivy/pkg/fanal/analyzer"
	"github.com/aquasecurity/trivy/pkg/fanal/applier"
	"github.com/aquasecurity/trivy/pkg/fanal/artifact"
	image2 "github.com/aquasecurity/trivy/pkg/fanal/artifact/image"
	local2 "github.com/aquasecurity/trivy/pkg/fanal/artifact/local"
	"github.com/aquasecurity/trivy/pkg/fanal/cache"
	ftypes "github.com/aquasecurity/trivy/pkg/fanal/types"
	"github.com/aquasecurity/trivy/pkg/sbom/cyclonedx"
	"github.com/aquasecurity/trivy/pkg/scanner"
	"github.com/aquasecurity/trivy/pkg/scanner/local"
	"github.com/aquasecurity/trivy/pkg/types"
	"github.com/aquasecurity/trivy/pkg/vulnerability"
	"github.com/containerd/containerd"
)

const (
	cleanupTimeout      = 30 * time.Second
	OSAnalyzers         = "os"
	LanguagesAnalyzers  = "languages"
	SecretAnalyzers     = "secret"
	ConfigFileAnalyzers = "config"
	LicenseAnalyzers    = "license"
)

// CollectorConfig allows to pass configuration
type CollectorConfig struct {
	ArtifactOption     artifact.Option
	CacheProvider      CacheProvider
	ClearCacheOnClose  bool
	ContainerdAccessor func() (containerdUtil.ContainerdItf, error)
}

// Collector uses trivy to generate a SBOM
type collector struct {
	config       CollectorConfig
	cache        cache.Cache
	cacheCleaner CacheCleaner
	applier      local.Applier
	detector     local.OspkgDetector
	dbConfig     db.Config
	vulnClient   vulnerability.Client
	marshaler    *cyclonedx.Marshaler
}

// DefaultCollectorConfig returns a default collector configuration
// However, accessors still need to be filled in externally
func DefaultCollectorConfig(enabledAnalyzers []string, cacheLocation string) CollectorConfig {
	collectorConfig := CollectorConfig{
		ArtifactOption: artifact.Option{
			Offline:           true,
			NoProgress:        true,
			DisabledAnalyzers: DefaultDisabledCollectors(enabledAnalyzers),
			Slow:              true,
			SBOMSources:       []string{},
			DisabledHandlers:  DefaultDisabledHandlers(),
		},
		ClearCacheOnClose: true,
	}

	collectorConfig.CacheProvider = cacheProvider(cacheLocation, config.Datadog.GetBool("container_image_collection.sbom.use_custom_cache"))

	if len(enabledAnalyzers) == 1 && enabledAnalyzers[0] == OSAnalyzers {
		collectorConfig.ArtifactOption.OnlyDirs = []string{"etc", "var/lib/dpkg", "var/lib/rpm", "lib/apk"}
	}

	return collectorConfig
}

func cacheProvider(cacheLocation string, useCustomCache bool) func() (cache.Cache, CacheCleaner, error) {
	if useCustomCache {
		return func() (cache.Cache, CacheCleaner, error) {
			return NewCustomBoltCache(
				cacheLocation,
				config.Datadog.GetInt("container_image_collection.sbom.custom_cache_max_cache_entries"),
				config.Datadog.GetInt("container_image_collection.sbom.custom_cache_max_disk_size"),
			)
		}
	}

	return func() (cache.Cache, CacheCleaner, error) {
		return NewBoltCache(cacheLocation)
	}
}

func DefaultDisabledCollectors(enabledAnalyzers []string) []analyzer.Type {
	sort.Strings(enabledAnalyzers)
	analyzersDisabled := func(analyzers string) bool {
		index := sort.SearchStrings(enabledAnalyzers, analyzers)
		return index >= len(enabledAnalyzers) || enabledAnalyzers[index] != analyzers
	}

	var disabledAnalyzers []analyzer.Type
	if analyzersDisabled(OSAnalyzers) {
		disabledAnalyzers = append(disabledAnalyzers, analyzer.TypeOSes...)
	}
	if analyzersDisabled(LanguagesAnalyzers) {
		disabledAnalyzers = append(disabledAnalyzers, analyzer.TypeLanguages...)
	}
	if analyzersDisabled(SecretAnalyzers) {
		disabledAnalyzers = append(disabledAnalyzers, analyzer.TypeSecret)
	}
	if analyzersDisabled(ConfigFileAnalyzers) {
		disabledAnalyzers = append(disabledAnalyzers, analyzer.TypeConfigFiles...)
	}
	if analyzersDisabled(LicenseAnalyzers) {
		disabledAnalyzers = append(disabledAnalyzers, analyzer.TypeLicenseFile)
	}

	return disabledAnalyzers
}

func DefaultDisabledHandlers() []ftypes.HandlerType {
	return []ftypes.HandlerType{ftypes.UnpackagedPostHandler}
}

func NewCollector(collectorConfig CollectorConfig) (Collector, error) {
	dbConfig := db.Config{}
	fanalCache, cacheCleaner, err := collectorConfig.CacheProvider()
	if err != nil {
		return nil, err
	}

	return &collector{
		config:       collectorConfig,
		cache:        fanalCache,
		cacheCleaner: cacheCleaner,
		applier:      applier.NewApplier(fanalCache),
		detector:     ospkg.Detector{},
		dbConfig:     dbConfig,
		vulnClient:   vulnerability.NewClient(dbConfig),
		marshaler:    cyclonedx.NewMarshaler(""),
	}, nil
}

func (c *collector) Close() error {
	if c.config.ClearCacheOnClose {
		if err := c.cache.Clear(); err != nil {
			return fmt.Errorf("error when clearing trivy cache: %w", err)
		}
	}

	return c.cache.Close()
}

func (c *collector) GetCacheCleaner() CacheCleaner {
	return c.cacheCleaner
}

func (c *collector) ScanContainerdImage(ctx context.Context, imgMeta *workloadmeta.ContainerImageMetadata, img containerd.Image) (Report, error) {
	client, err := c.config.ContainerdAccessor()
	if err != nil {
		return nil, fmt.Errorf("unable to access containerd client, err: %w", err)
	}

	fanalImage, cleanup, err := convertContainerdImage(ctx, client.RawClient(), imgMeta, img)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		return nil, fmt.Errorf("unable to convert containerd image, err: %w", err)
	}

	imageArtifact, err := image2.NewArtifact(fanalImage, c.cache, c.config.ArtifactOption)
	if err != nil {
		return nil, fmt.Errorf("unable to create artifact from image, err: %w", err)
	}

	bom, err := c.scan(ctx, imageArtifact, imgMeta)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal report to sbom format, err: %w", err)
	}

	return bom, nil
}

func (c *collector) ScanContainerdImageFromFilesystem(ctx context.Context, imgMeta *workloadmeta.ContainerImageMetadata, img containerd.Image) (Report, error) {
	client, err := c.config.ContainerdAccessor()
	if err != nil {
		return nil, fmt.Errorf("unable to access containerd client, err: %w", err)
	}

	imagePath, err := os.MkdirTemp(os.TempDir(), fmt.Sprintf("containerd-image-*"))
	if err != nil {
		return nil, fmt.Errorf("unable to create temp dir, err: %w", err)
	}
	defer func() {
		err := os.RemoveAll(imagePath)
		if err != nil {
			log.Errorf("Unable to remove temp dir: %s, err: %v", imagePath, err)
		}
	}()

	// Computing duration of containerd lease
	deadline, _ := ctx.Deadline()
	expiration := deadline.Sub(time.Now().Add(cleanupTimeout))

	cleanUp, err := client.MountImage(ctx, expiration, imgMeta.Namespace, img, imagePath)
	if err != nil {
		return nil, fmt.Errorf("unable to mount containerd image, err: %w", err)
	}

	defer func() {
		cleanUpContext, cleanUpContextCancel := context.WithTimeout(context.Background(), cleanupTimeout)
		err := cleanUp(cleanUpContext)
		cleanUpContextCancel()
		if err != nil {
			log.Errorf("Unable to clean up mounted image, err: %v", err)
		}
	}()

	return c.scanFilesystem(ctx, imagePath, imgMeta)
}

func (c *collector) scanFilesystem(ctx context.Context, path string, imgMeta *workloadmeta.ContainerImageMetadata) (Report, error) {
	fsArtifact, err := local2.NewArtifact(path, c.cache, c.config.ArtifactOption)
	if err != nil {
		return nil, fmt.Errorf("unable to create artifact from fs, err: %w", err)
	}

	bom, err := c.scan(ctx, fsArtifact, imgMeta)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal report to sbom format, err: %w", err)
	}

	return bom, nil
}

func (c *collector) ScanFilesystem(ctx context.Context, path string) (Report, error) {
	return c.scanFilesystem(ctx, path, nil)
}

func (c *collector) scan(ctx context.Context, artifact artifact.Artifact, imgMeta *workloadmeta.ContainerImageMetadata) (Report, error) {
	artifactReference, err := artifact.Inspect(ctx) // called by the scanner as well
	if err != nil {
		return nil, err
	}
	if imgMeta != nil {
		c.cacheCleaner.setKeysForEntity(imgMeta.EntityID.ID, append(artifactReference.BlobIDs, artifactReference.ID))
	}
	s := scanner.NewScanner(local.NewScanner(c.applier, c.detector, c.vulnClient), artifact)
	trivyReport, err := s.ScanArtifact(ctx, types.ScanOptions{
		VulnType:            []string{},
		SecurityChecks:      []string{},
		ScanRemovedPackages: false,
		ListAllPackages:     true,
	})
	if err != nil {
		return nil, err
	}

	return &TrivyReport{
		Report:    trivyReport,
		marshaler: c.marshaler,
	}, nil
}
