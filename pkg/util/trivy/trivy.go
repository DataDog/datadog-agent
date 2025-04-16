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
	"math"
	"runtime"
	"slices"
	"sync"
	"syscall"

	"github.com/aquasecurity/trivy-db/pkg/db"
	"github.com/aquasecurity/trivy/pkg/fanal/analyzer"
	"github.com/aquasecurity/trivy/pkg/fanal/applier"
	"github.com/aquasecurity/trivy/pkg/fanal/artifact"
	local2 "github.com/aquasecurity/trivy/pkg/fanal/artifact/local"
	ftypes "github.com/aquasecurity/trivy/pkg/fanal/types"
	"github.com/aquasecurity/trivy/pkg/fanal/walker"
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
	uwalker "github.com/DataDog/datadog-agent/pkg/util/trivy/walker"
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
	clearCacheOnClose bool
	maxCacheSize      int
}

// Collector uses trivy to generate a SBOM
type Collector struct {
	config           collectorConfig
	cacheInitialized sync.Once
	persistentCache  CacheWithCleaner
	marshaler        cyclonedx.Marshaler
	wmeta            option.Option[workloadmeta.Component]

	osScanner   ospkg.Scanner
	langScanner langpkg.Scanner
	vulnClient  vulnerability.Client
}

var globalCollector *Collector

var trivyDefaultSkipDirs = []string{
	// already included in Trivy's defaultSkipDirs
	// "**/.git",
	// "proc",
	// "sys",
	// "dev",

	"**/.cargo/git/**",
}

func getDefaultArtifactOption(opts sbom.ScanOptions) artifact.Option {
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
		WalkerOption: walker.Option{
			ErrorCallback: func(_ string, err error) error {
				if errors.Is(err, fs.ErrPermission) || errors.Is(err, fs.ErrNotExist) {
					return nil
				}
				if errors.Is(err, syscall.ESRCH) {
					// ignore "no such process" errors when walking /proc/<pid>
					return nil
				}
				return err
			},
		},
	}

	option.WalkerOption.SkipDirs = trivyDefaultSkipDirs

	if looselyCompareAnalyzers(opts.Analyzers, []string{OSAnalyzers}) {
		option.WalkerOption.OnlyDirs = []string{
			"/etc/*",
			"/lib/apk/db/*",
			"/usr/lib/*",
			"/usr/lib/sysimage/rpm/*",
			"/var/lib/dpkg/**",
			"/var/lib/rpm/*",
			"/aarch64-bottlerocket-linux-gnu/sys-root/usr/lib/*",
			"/aarch64-bottlerocket-linux-gnu/sys-root/usr/share/bottlerocket/*",
			"/x86_64-bottlerocket-linux-gnu/sys-root/usr/lib/*",
			"/x86_64-bottlerocket-linux-gnu/sys-root/usr/share/bottlerocket/*",
		}
	} else if looselyCompareAnalyzers(opts.Analyzers, []string{OSAnalyzers, LanguagesAnalyzers}) {
		option.WalkerOption.SkipDirs = append(
			option.WalkerOption.SkipDirs,
			"/bin/**",
			"/boot/**",
			"/dev/**",
			"/media/**",
			"/mnt/**",
			"/proc/**",
			"/run/**",
			"/sbin/**",
			"/sys/**",
			"/tmp/**",
			"/usr/bin/**",
			"/usr/sbin/**",
			"/var/cache/**",
			"/var/lib/containerd/**",
			"/var/lib/containers/**",
			"/var/lib/docker/**",
			"/var/lib/libvirt/**",
			"/var/lib/snapd/**",
			"/var/log/**",
			"/var/run/**",
			"/var/tmp/**",
		)
	}

	if slices.Contains(opts.Analyzers, LanguagesAnalyzers) {
		option.FileChecksum = true
	}

	return option
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
			clearCacheOnClose: cfg.GetBool("sbom.clear_cache_on_exit"),
			maxCacheSize:      cfg.GetInt("sbom.cache.max_disk_size"),
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
			maxCacheSize: math.MaxInt,
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

type artifactWithType struct {
	inner     artifact.Artifact
	forceType artifact.Type
}

func (fa *artifactWithType) Inspect(ctx context.Context) (artifact.Reference, error) {
	ref, err := fa.inner.Inspect(ctx)
	ref.Type = fa.forceType
	return ref, err
}

func (fa *artifactWithType) Clean(ref artifact.Reference) error {
	return fa.inner.Clean(ref)
}

// ScanFilesystem scans the specified directory and logs detailed scan steps.
func (c *Collector) ScanFilesystem(ctx context.Context, path string, scanOptions sbom.ScanOptions, removeLayers bool) (sbom.Report, error) {
	// For filesystem scans, it is required to walk the filesystem to get the persistentCache key so caching does not add any value.
	// TODO: Cache directly the trivy report for container images
	cache := newMemoryCache()

	fsArtifact, err := local2.NewArtifact(path, cache, uwalker.NewFSWalker(), getDefaultArtifactOption(scanOptions))
	if err != nil {
		return nil, fmt.Errorf("unable to create artifact from fs, err: %w", err)
	}

	wrapper := &artifactWithType{
		inner:     fsArtifact,
		forceType: artifact.TypeContainerImage,
	}
	if removeLayers {
		wrapper.forceType = artifact.TypeFilesystem
	}

	trivyReport, err := c.scan(ctx, wrapper, applier.NewApplier(cache))
	if err != nil {
		return nil, fmt.Errorf("unable to marshal report to sbom format, err: %w", err)
	}

	return c.buildReport(trivyReport, cache.lastBlobID), nil
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

func (c *Collector) buildReport(trivyReport *types.Report, id string) *Report {
	log.Debugf("Found OS: %+v", trivyReport.Metadata.OS)
	pkgCount := 0
	for _, results := range trivyReport.Results {
		pkgCount += len(results.Packages)
	}
	log.Debugf("Found %d packages", pkgCount)

	return &Report{
		Report:    trivyReport,
		id:        id,
		marshaler: c.marshaler,
	}
}

func looselyCompareAnalyzers(given []string, against []string) bool {
	target := make(map[string]struct{}, len(against))
	for _, val := range against {
		target[val] = struct{}{}
	}

	validated := make(map[string]struct{})

	for _, val := range given {
		// if already validated, skip
		// this allows to support duplicated entries
		if _, ok := validated[val]; ok {
			continue
		}

		// if this value is not in
		if _, ok := target[val]; !ok {
			return false
		}
		delete(target, val)
		validated[val] = struct{}{}
	}

	return len(target) == 0
}
