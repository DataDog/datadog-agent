// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy

// Package trivy holds the scan components
package trivy

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/containerd/containerd/mount"
	"github.com/containerd/containerd/namespaces"

	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	cutil "github.com/DataDog/datadog-agent/pkg/util/containerd"
	containersimage "github.com/DataDog/datadog-agent/pkg/util/containers/image"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"

	"github.com/aquasecurity/trivy-db/pkg/db"
	"github.com/aquasecurity/trivy/pkg/fanal/analyzer"
	"github.com/aquasecurity/trivy/pkg/fanal/applier"
	"github.com/aquasecurity/trivy/pkg/fanal/artifact"
	image2 "github.com/aquasecurity/trivy/pkg/fanal/artifact/image"
	local2 "github.com/aquasecurity/trivy/pkg/fanal/artifact/local"
	ftypes "github.com/aquasecurity/trivy/pkg/fanal/types"
	"github.com/aquasecurity/trivy/pkg/sbom/cyclonedx"
	"github.com/aquasecurity/trivy/pkg/scanner"
	"github.com/aquasecurity/trivy/pkg/scanner/langpkg"
	"github.com/aquasecurity/trivy/pkg/scanner/local"
	"github.com/aquasecurity/trivy/pkg/scanner/ospkg"
	"github.com/aquasecurity/trivy/pkg/types"
	"github.com/aquasecurity/trivy/pkg/vulnerability"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/leases"
	"github.com/containerd/errdefs"
	"github.com/docker/docker/client"

	// This is required to load sqlite based RPM databases
	_ "modernc.org/sqlite"
)

const (
	cleanupTimeout = 30 * time.Second

	OSAnalyzers           = "os"                  // OSAnalyzers defines an OS analyzer
	LanguagesAnalyzers    = "languages"           // LanguagesAnalyzers defines a language analyzer
	SecretAnalyzers       = "secret"              // SecretAnalyzers defines a secret analyzer
	ConfigFileAnalyzers   = "config"              // ConfigFileAnalyzers defines a configuration file analyzer
	LicenseAnalyzers      = "license"             // LicenseAnalyzers defines a license analyzer
	TypeApkCommand        = "apk-command"         // TypeApkCommand defines a apk-command analyzer
	HistoryDockerfile     = "history-dockerfile"  // HistoryDockerfile defines a history-dockerfile analyzer
	TypeImageConfigSecret = "image-config-secret" // TypeImageConfigSecret defines a history-dockerfile analyzer
)

// ContainerdAccessor is a function that should return a containerd client
type ContainerdAccessor func() (cutil.ContainerdItf, error)

// collectorConfig allows to pass configuration
type collectorConfig struct {
	clearCacheOnClose bool
	maxCacheSize      int
	overlayFSSupport  bool
}

// Collector uses trivy to generate a SBOM
type Collector struct {
	config           collectorConfig
	cacheInitialized sync.Once
	persistentCache  CacheWithCleaner
	osScanner        ospkg.Scanner
	langScanner      langpkg.Scanner
	vulnClient       vulnerability.Client
	marshaler        cyclonedx.Marshaler
	wmeta            optional.Option[workloadmeta.Component]
}

var globalCollector *Collector

func getDefaultArtifactOption(root string, opts sbom.ScanOptions) artifact.Option {
	parallel := 1
	if opts.Fast {
		parallel = runtime.NumCPU()
	}

	option := artifact.Option{
		Offline:           true,
		NoProgress:        true,
		DisabledAnalyzers: DefaultDisabledCollectors(opts.Analyzers),
		Parallel:          parallel,
		SBOMSources:       []string{},
		DisabledHandlers:  DefaultDisabledHandlers(),
		WalkOption: artifact.WalkOption{
			ErrorCallback: func(_ string, err error) error {
				if errors.Is(err, fs.ErrPermission) || errors.Is(err, os.ErrNotExist) {
					return nil
				}
				return err
			},
		},
	}

	if len(opts.Analyzers) == 1 && opts.Analyzers[0] == OSAnalyzers {
		option.OnlyDirs = []string{
			"/etc/*",
			"/lib/apk/db/*",
			"/usr/lib/*",
			"/usr/lib/sysimage/rpm/*",
			"/var/lib/dpkg/**",
			"/var/lib/rpm/*",
		}
		if root != "" {
			// OnlyDirs is handled differently for image than for filesystem.
			// This needs to be fixed properly but in the meantime, use absolute
			// paths for fs and relative paths for images.
			for i := range option.OnlyDirs {
				option.OnlyDirs[i] = filepath.Join(root, option.OnlyDirs[i])
			}
		}
	}

	return option
}

// DefaultDisabledCollectors returns default disabled collectors
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
	if analyzersDisabled(TypeApkCommand) {
		disabledAnalyzers = append(disabledAnalyzers, analyzer.TypeApkCommand)
	}
	if analyzersDisabled(HistoryDockerfile) {
		disabledAnalyzers = append(disabledAnalyzers, analyzer.TypeHistoryDockerfile)
	}
	if analyzersDisabled(TypeImageConfigSecret) {
		disabledAnalyzers = append(disabledAnalyzers, analyzer.TypeImageConfigSecret)
	}
	disabledAnalyzers = append(disabledAnalyzers,
		analyzer.TypeExecutable,
		analyzer.TypeRedHatContentManifestType,
		analyzer.TypeRedHatDockerfileType,
		analyzer.TypeSBOM,
		analyzer.TypeUbuntuESM)
	return disabledAnalyzers
}

// DefaultDisabledHandlers returns default disabled handlers
func DefaultDisabledHandlers() []ftypes.HandlerType {
	return []ftypes.HandlerType{ftypes.UnpackagedPostHandler}
}

// NewCollector returns a new collector
func NewCollector(cfg config.Component, wmeta optional.Option[workloadmeta.Component]) (*Collector, error) {
	return &Collector{
		config: collectorConfig{
			clearCacheOnClose: cfg.GetBool("sbom.clear_cache_on_exit"),
			maxCacheSize:      cfg.GetInt("sbom.cache.max_disk_size"),
			overlayFSSupport:  cfg.GetBool("sbom.container_image.overlayfs_direct_scan"),
		},
		osScanner:   ospkg.NewScanner(),
		langScanner: langpkg.NewScanner(),
		vulnClient:  vulnerability.NewClient(db.Config{}),
		marshaler:   cyclonedx.NewMarshaler(""),
		wmeta:       wmeta,
	}, nil
}

// GetGlobalCollector gets the global collector
func GetGlobalCollector(cfg config.Component, wmeta optional.Option[workloadmeta.Component]) (*Collector, error) {
	if globalCollector != nil {
		return globalCollector, nil
	}

	collector, err := NewCollector(cfg, wmeta)
	if err != nil {
		return nil, err
	}

	globalCollector = collector
	return globalCollector, nil
}

// Close closes the collector
func (c *Collector) Close() error {
	if c.persistentCache == nil {
		return nil
	}

	if c.config.clearCacheOnClose {
		if err := c.persistentCache.Clear(); err != nil {
			return fmt.Errorf("error when clearing trivy persistentCache: %w", err)
		}
	}

	return c.persistentCache.Close()
}

// CleanCache cleans the persistentCache
func (c *Collector) CleanCache() error {
	if c.persistentCache != nil {
		return c.persistentCache.clean()
	}
	return nil
}

// getCache returns the persistentCache with the persistentCache Cleaner. It should initializes the persistentCache
// only once to avoid blocking the CLI with the `flock` file system.
func (c *Collector) getCache() (CacheWithCleaner, error) {
	var err error
	c.cacheInitialized.Do(func() {
		c.persistentCache, err = NewCustomBoltCache(
			c.wmeta,
			defaultCacheDir(),
			c.config.maxCacheSize,
		)
	})

	if err != nil {
		return nil, err
	}

	return c.persistentCache, nil
}

// ScanDockerImageFromGraphDriver scans a docker image directly from the graph driver
func (c *Collector) ScanDockerImageFromGraphDriver(ctx context.Context, imgMeta *workloadmeta.ContainerImageMetadata, client client.ImageAPIClient, scanOptions sbom.ScanOptions) (sbom.Report, error) {
	fanalImage, cleanup, err := convertDockerImage(ctx, client, imgMeta)
	if cleanup != nil {
		defer cleanup()
	}

	if err != nil {
		return nil, fmt.Errorf("unable to convert docker image, err: %w", err)
	}

	if fanalImage.inspect.GraphDriver.Name == "overlay2" {
		var layers []string
		if layerDirs, ok := fanalImage.inspect.GraphDriver.Data["LowerDir"]; ok {
			layers = append(layers, strings.Split(layerDirs, ":")...)
		}

		if layerDirs, ok := fanalImage.inspect.GraphDriver.Data["UpperDir"]; ok {
			layers = append(layers, strings.Split(layerDirs, ":")...)
		}

		if env.IsContainerized() {
			for i, layer := range layers {
				layers[i] = containersimage.SanitizeHostPath(layer)
			}
		}

		return c.scanOverlayFS(ctx, layers, imgMeta, scanOptions)
	}

	return nil, fmt.Errorf("unsupported graph driver: %s", fanalImage.inspect.GraphDriver.Name)
}

// ScanDockerImage scans a docker image by exporting it and scanning the tarball
func (c *Collector) ScanDockerImage(ctx context.Context, imgMeta *workloadmeta.ContainerImageMetadata, client client.ImageAPIClient, scanOptions sbom.ScanOptions) (sbom.Report, error) {
	fanalImage, cleanup, err := convertDockerImage(ctx, client, imgMeta)
	if cleanup != nil {
		defer cleanup()
	}

	if err != nil {
		return nil, fmt.Errorf("unable to convert docker image, err: %w", err)
	}

	return c.scanImage(ctx, fanalImage, imgMeta, scanOptions)
}

func (c *Collector) scanOverlayFS(ctx context.Context, layers []string, imgMeta *workloadmeta.ContainerImageMetadata, scanOptions sbom.ScanOptions) (sbom.Report, error) {
	log.Debugf("Generating SBOM for image %s using overlayfs %+v", imgMeta.ID, layers)
	overlayFsReader := NewFS(layers)
	report, err := c.scanFilesystem(ctx, overlayFsReader, "/", imgMeta, scanOptions)
	if err != nil {
		return nil, err
	}

	return report, nil
}

// ScanContainerdImageFromSnapshotter scans containerd image directly from the snapshotter
func (c *Collector) ScanContainerdImageFromSnapshotter(ctx context.Context, imgMeta *workloadmeta.ContainerImageMetadata, img containerd.Image, client cutil.ContainerdItf, scanOptions sbom.ScanOptions) (sbom.Report, error) {
	// Computing duration of containerd lease
	deadline, _ := ctx.Deadline()
	expiration := deadline.Sub(time.Now().Add(cleanupTimeout))
	clClient := client.RawClient()
	imageID := imgMeta.ID

	mounts, err := client.Mounts(ctx, expiration, imgMeta.Namespace, img)
	if err != nil {
		return nil, fmt.Errorf("unable to get mounts for image %s, err: %w", imgMeta.ID, err)
	}

	layers := extractLayersFromOverlayFSMounts(mounts)
	if len(layers) == 0 {
		return nil, fmt.Errorf("unable to extract layers from overlayfs mounts %+v for image %s", mounts, imgMeta.ID)
	}

	ctx = namespaces.WithNamespace(ctx, imgMeta.Namespace)
	// Adding a lease to cleanup dandling snaphots at expiration
	ctx, done, err := clClient.WithLease(ctx,
		leases.WithID(imageID),
		leases.WithExpiration(expiration),
		leases.WithLabels(map[string]string{
			"containerd.io/gc.ref.snapshot." + containerd.DefaultSnapshotter: imageID,
		}),
	)
	if err != nil && !errdefs.IsAlreadyExists(err) {
		return nil, fmt.Errorf("unable to get a lease, err: %w", err)
	}

	report, err := c.scanOverlayFS(ctx, layers, imgMeta, scanOptions)

	if err := done(ctx); err != nil {
		log.Warnf("Unable to cancel containerd lease with id: %s, err: %v", imageID, err)
	}

	return report, err
}

// ScanContainerdImage scans containerd image by exporting it and scanning the tarball
func (c *Collector) ScanContainerdImage(ctx context.Context, imgMeta *workloadmeta.ContainerImageMetadata, img containerd.Image, client cutil.ContainerdItf, scanOptions sbom.ScanOptions) (sbom.Report, error) {
	fanalImage, cleanup, err := convertContainerdImage(ctx, client.RawClient(), imgMeta, img)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		return nil, fmt.Errorf("unable to convert containerd image, err: %w", err)
	}

	return c.scanImage(ctx, fanalImage, imgMeta, scanOptions)
}

// ScanContainerdImageFromFilesystem scans containerd image from file-system
func (c *Collector) ScanContainerdImageFromFilesystem(ctx context.Context, imgMeta *workloadmeta.ContainerImageMetadata, img containerd.Image, client cutil.ContainerdItf, scanOptions sbom.ScanOptions) (sbom.Report, error) {
	//nolint:gosimple // TODO(CINT) Fix go simple linte
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

	return c.scanFilesystem(ctx, os.DirFS("/"), imagePath, imgMeta, scanOptions)
}

func (c *Collector) scanFilesystem(ctx context.Context, fsys fs.FS, path string, imgMeta *workloadmeta.ContainerImageMetadata, scanOptions sbom.ScanOptions) (sbom.Report, error) {
	// For filesystem scans, it is required to walk the filesystem to get the persistentCache key so caching does not add any value.
	// TODO: Cache directly the trivy report for container images
	cache := newMemoryCache()

	fsArtifact, err := local2.NewArtifact(fsys, path, cache, getDefaultArtifactOption(".", scanOptions))
	if err != nil {
		return nil, fmt.Errorf("unable to create artifact from fs, err: %w", err)
	}

	trivyReport, err := c.scan(ctx, fsArtifact, applier.NewApplier(cache), imgMeta, cache, false)
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
		id:        cache.blobID,
		marshaler: c.marshaler,
	}, nil
}

// ScanFilesystem scans file-system
func (c *Collector) ScanFilesystem(ctx context.Context, fsys fs.FS, path string, scanOptions sbom.ScanOptions) (sbom.Report, error) {
	return c.scanFilesystem(ctx, fsys, path, nil, scanOptions)
}

func (c *Collector) scan(ctx context.Context, artifact artifact.Artifact, applier applier.Applier, imgMeta *workloadmeta.ContainerImageMetadata, cache CacheWithCleaner, useCache bool) (*types.Report, error) {
	if useCache && imgMeta != nil && cache != nil {
		// The artifact reference is only needed to clean up the blobs after the scan.
		// It is re-generated from cached partial results during the scan.
		artifactReference, err := artifact.Inspect(ctx)
		if err != nil {
			return nil, err
		}
		cache.setKeysForEntity(imgMeta.EntityID.ID, append(artifactReference.BlobIDs, artifactReference.ID))
	}

	s := scanner.NewScanner(local.NewScanner(applier, c.osScanner, c.langScanner, c.vulnClient), artifact)
	trivyReport, err := s.ScanArtifact(ctx, types.ScanOptions{
		VulnType:            []string{},
		ScanRemovedPackages: false,
		ListAllPackages:     true,
	})
	if err != nil {
		return nil, err
	}

	return &trivyReport, nil
}

func (c *Collector) scanImage(ctx context.Context, fanalImage ftypes.Image, imgMeta *workloadmeta.ContainerImageMetadata, scanOptions sbom.ScanOptions) (sbom.Report, error) {
	cache, err := c.getCache()
	if err != nil {
		return nil, err
	}

	imageArtifact, err := image2.NewArtifact(fanalImage, cache, getDefaultArtifactOption("", scanOptions))
	if err != nil {
		return nil, fmt.Errorf("unable to create artifact from image, err: %w", err)
	}

	trivyReport, err := c.scan(ctx, imageArtifact, applier.NewApplier(cache), imgMeta, c.persistentCache, true)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal report to sbom format, err: %w", err)
	}

	return &Report{
		Report:    trivyReport,
		id:        trivyReport.Metadata.ImageID,
		marshaler: c.marshaler,
	}, nil
}

func extractLayersFromOverlayFSMounts(mounts []mount.Mount) []string {
	var layers []string
	for _, mount := range mounts {
		for _, opt := range mount.Options {
			for _, prefix := range []string{"upperdir=", "lowerdir="} {
				trimmedOpt := strings.TrimPrefix(opt, prefix)
				if trimmedOpt != opt {
					layers = append(layers, strings.Split(trimmedOpt, ":")...)
				}
			}
		}
	}
	return layers
}
