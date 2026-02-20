// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy

// Package trivy holds the scan components
package trivy

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"runtime"
	"slices"
	"sync"

	"github.com/aquasecurity/trivy-db/pkg/db"
	"github.com/aquasecurity/trivy/pkg/fanal/analyzer"
	"github.com/aquasecurity/trivy/pkg/fanal/applier"
	"github.com/aquasecurity/trivy/pkg/fanal/artifact"
	ftypes "github.com/aquasecurity/trivy/pkg/fanal/types"
	"github.com/aquasecurity/trivy/pkg/sbom/cyclonedx"
	"github.com/aquasecurity/trivy/pkg/scanner"
	"github.com/aquasecurity/trivy/pkg/scanner/langpkg"
	"github.com/aquasecurity/trivy/pkg/scanner/local"
	"github.com/aquasecurity/trivy/pkg/scanner/ospkg"
	"github.com/aquasecurity/trivy/pkg/types"
	"github.com/aquasecurity/trivy/pkg/vulnerability"

	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/DataDog/ddtrivy"
)

const (
	OSAnalyzers           = "os"                  // OSAnalyzers defines an OS analyzer
	LanguagesAnalyzers    = "languages"           // LanguagesAnalyzers defines a language analyzer
	SecretAnalyzers       = "secret"              // SecretAnalyzers defines a secret analyzer
	ConfigFileAnalyzers   = "config"              // ConfigFileAnalyzers defines a configuration file analyzer
	TypeApkCommand        = "apk-command"         // TypeApkCommand defines a apk-command analyzer
	HistoryDockerfile     = "history-dockerfile"  // HistoryDockerfile defines a history-dockerfile analyzer
	TypeImageConfigSecret = "image-config-secret" // TypeImageConfigSecret defines a history-dockerfile analyzer
)

// collectorConfig allows to pass configuration
type collectorConfig struct {
	cacheDir            string
	clearCacheOnClose   bool
	maxCacheSize        int
	computeDependencies bool
	simplifyBomRefs     bool
}

// Collector uses trivy to generate a SBOM
type Collector struct {
	config collectorConfig

	cacheInitialized   sync.Once
	persistentCache    CacheWithCleaner
	persistentCacheErr error

	marshaler cyclonedx.Marshaler
	wmeta     option.Option[workloadmeta.Component]

	osScanner   ospkg.Scanner
	langScanner langpkg.Scanner
	vulnClient  vulnerability.Client
}

var globalCollector *Collector

func getDefaultArtifactOption(scanOptions sbom.ScanOptions) artifact.Option {
	parallel := 1
	if scanOptions.Fast {
		parallel = runtime.NumCPU()
	}

	var artifactOption artifact.Option
	if len(scanOptions.Analyzers) == 1 && scanOptions.Analyzers[0] == OSAnalyzers {
		artifactOption = ddtrivy.TrivyOptionsOS(parallel)
	} else {
		artifactOption = ddtrivy.TrivyOptionsAll(parallel)
	}

	artifactOption.WalkerOption.OnlyDirs = append(artifactOption.WalkerOption.OnlyDirs, scanOptions.AdditionalDirs...)
	// agent specific config, needed so that we don't download the Java DB at runtime
	artifactOption.OfflineJar = true

	return artifactOption
}

// DefaultDisabledCollectors returns default disabled collectors
func DefaultDisabledCollectors(enabledAnalyzers []string) []analyzer.Type {
	analyzersDisabled := func(analyzers string) bool {
		return !slices.Contains(enabledAnalyzers, analyzers)
	}

	var disabledAnalyzers []analyzer.Type
	if analyzersDisabled(OSAnalyzers) {
		disabledAnalyzers = append(disabledAnalyzers, analyzer.TypeOSes...)
	}
	if analyzersDisabled(LanguagesAnalyzers) {
		disabledAnalyzers = append(disabledAnalyzers, analyzer.TypeLanguages...)
		disabledAnalyzers = append(disabledAnalyzers, analyzer.TypeIndividualPkgs...)
		disabledAnalyzers = append(disabledAnalyzers, analyzer.TypeExecutable)
	}
	if analyzersDisabled(SecretAnalyzers) {
		disabledAnalyzers = append(disabledAnalyzers, analyzer.TypeSecret)
	}
	if analyzersDisabled(ConfigFileAnalyzers) {
		disabledAnalyzers = append(disabledAnalyzers, analyzer.TypeConfigFiles...)
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
		analyzer.TypeRedHatContentManifestType,
		analyzer.TypeRedHatDockerfileType,
		analyzer.TypeSBOM,
		analyzer.TypeUbuntuESM,
		analyzer.TypeLicenseFile,
		analyzer.TypeRpmArchive,
	)

	return disabledAnalyzers
}

// DefaultDisabledHandlers returns default disabled handlers
func DefaultDisabledHandlers() []ftypes.HandlerType {
	return []ftypes.HandlerType{ftypes.UnpackagedPostHandler}
}

// NewCollector returns a new collector
func NewCollector(cfg config.Component, wmeta option.Option[workloadmeta.Component]) (*Collector, error) {
	return &Collector{
		config: collectorConfig{
			cacheDir:            cfg.GetString("sbom.cache_directory"),
			clearCacheOnClose:   cfg.GetBool("sbom.clear_cache_on_exit"),
			maxCacheSize:        cfg.GetInt("sbom.cache.max_disk_size"),
			computeDependencies: cfg.GetBool("sbom.compute_dependencies"),
			simplifyBomRefs:     cfg.GetBool("sbom.simplify_bom_refs"),
		},
		marshaler: cyclonedx.NewMarshaler(""),
		wmeta:     wmeta,

		osScanner:   ospkg.NewScanner(),
		langScanner: langpkg.NewScanner(),
		vulnClient:  vulnerability.NewClient(db.Config{}),
	}, nil
}

// NewCollectorForCLI returns a new collector, should be used only for sbomgen CLI
func NewCollectorForCLI() *Collector {
	return &Collector{
		config: collectorConfig{
			maxCacheSize:        math.MaxInt,
			computeDependencies: true,
		},
		marshaler: cyclonedx.NewMarshaler(""),

		osScanner:   ospkg.NewScanner(),
		langScanner: langpkg.NewScanner(),
		vulnClient:  vulnerability.NewClient(db.Config{}),
	}
}

// GetGlobalCollector gets the global collector
func GetGlobalCollector(cfg config.Component, wmeta option.Option[workloadmeta.Component]) (*Collector, error) {
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

// GetCache returns the persistentCache with the persistentCache Cleaner. It should initializes the persistentCache
// only once to avoid blocking the CLI with the `flock` file system.
func (c *Collector) GetCache() (CacheWithCleaner, error) {
	c.cacheInitialized.Do(func() {
		c.persistentCache, c.persistentCacheErr = NewCustomBoltCache(
			c.wmeta,
			c.config.cacheDir,
			c.config.maxCacheSize,
		)
	})

	return c.persistentCache, c.persistentCacheErr
}

// ScanFSTrivyReport scans the specified directory and logs detailed scan steps.
func (c *Collector) ScanFSTrivyReport(ctx context.Context, path string, scanOptions sbom.ScanOptions, removeLayers bool) (*types.Report, error) {
	// For filesystem scans, it is required to walk the filesystem to get the persistentCache key so caching does not add any value.
	// TODO: Cache directly the trivy report for container images
	cache := newMemoryCache()

	artifactOption := getDefaultArtifactOption(scanOptions)

	artifactType := artifact.TypeContainerImage
	if removeLayers {
		artifactType = artifact.TypeFilesystem
	}
	report, err := ddtrivy.ScanRootFS(ctx, artifactOption, cache, path, artifactType)
	if err != nil {
		return nil, fmt.Errorf("unable to scan rootfs, err: %w", err)
	}

	return report, nil
}

// ScanFilesystem scans the specified directory and logs detailed scan steps.
func (c *Collector) ScanFilesystem(ctx context.Context, path string, scanOptions sbom.ScanOptions, removeLayers bool) (*Report, error) {
	trivyReport, err := c.ScanFSTrivyReport(ctx, path, scanOptions, removeLayers)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal report to sbom format, err: %w", err)
	}

	hasher := sha256.New()
	encoder := json.NewEncoder(hasher)
	if err := encoder.Encode(trivyReport.Results); err != nil {
		return nil, fmt.Errorf("unable to compute hash for report: err: %w", err)
	}

	hash := "sha256:" + base64.StdEncoding.EncodeToString(hasher.Sum(nil))
	return c.buildReport(trivyReport, hash)
}

func (c *Collector) scan(ctx context.Context, artifact artifact.Artifact, applier applier.Applier) (*types.Report, error) {
	localScanner := local.NewScanner(applier, c.osScanner, c.langScanner, c.vulnClient)
	s := scanner.NewScanner(localScanner, artifact)

	trivyReport, err := s.ScanArtifact(ctx, types.ScanOptions{
		ScanRemovedPackages: false,
		PkgTypes:            types.PkgTypes,
		PkgRelationships:    ftypes.Relationships,
		Scanners:            types.Scanners{types.SBOMScanner},
	})
	if err != nil {
		return nil, err
	}

	return &trivyReport, nil
}

func (c *Collector) buildReport(trivyReport *types.Report, id string) (*Report, error) {
	log.Debugf("Found OS: %+v", trivyReport.Metadata.OS)
	pkgCount := 0
	for _, results := range trivyReport.Results {
		pkgCount += len(results.Packages)
	}
	log.Debugf("Found %d packages", pkgCount)

	return newReport(id, trivyReport, c.marshaler, reportOptions{
		dependencies:    c.config.computeDependencies,
		simplifyBomRefs: c.config.simplifyBomRefs,
	})
}
