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
	"runtime"
	"slices"

	"github.com/aquasecurity/trivy-db/pkg/db"
	"github.com/aquasecurity/trivy/pkg/fanal/analyzer"
	"github.com/aquasecurity/trivy/pkg/fanal/applier"
	"github.com/aquasecurity/trivy/pkg/fanal/artifact"
	ftypes "github.com/aquasecurity/trivy/pkg/fanal/types"
	"github.com/aquasecurity/trivy/pkg/sbom/cyclonedx"
	"github.com/aquasecurity/trivy/pkg/scan"
	"github.com/aquasecurity/trivy/pkg/scan/langpkg"
	"github.com/aquasecurity/trivy/pkg/scan/local"
	"github.com/aquasecurity/trivy/pkg/scan/ospkg"
	"github.com/aquasecurity/trivy/pkg/types"
	"github.com/aquasecurity/trivy/pkg/vulnerability"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
	computeDependencies bool
	simplifyBomRefs     bool
}

// Collector uses trivy to generate a SBOM
type Collector struct {
	config collectorConfig

	marshaler cyclonedx.Marshaler

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
func NewCollector(cfg config.Component) (*Collector, error) {
	return &Collector{
		config: collectorConfig{
			computeDependencies: cfg.GetBool("sbom.compute_dependencies"),
			simplifyBomRefs:     cfg.GetBool("sbom.simplify_bom_refs"),
		},
		marshaler: cyclonedx.NewMarshaler(""),

		osScanner:   ospkg.NewScanner(),
		langScanner: langpkg.NewScanner(),
		vulnClient:  vulnerability.NewClient(db.Config{}),
	}, nil
}

// NewCollectorForCLI returns a new collector, should be used only for sbomgen CLI
func NewCollectorForCLI() *Collector {
	return &Collector{
		config: collectorConfig{
			computeDependencies: true,
		},
		marshaler: cyclonedx.NewMarshaler(""),

		osScanner:   ospkg.NewScanner(),
		langScanner: langpkg.NewScanner(),
		vulnClient:  vulnerability.NewClient(db.Config{}),
	}
}

// GetGlobalCollector gets the global collector
func GetGlobalCollector(cfg config.Component) (*Collector, error) {
	if globalCollector != nil {
		return globalCollector, nil
	}

	collector, err := NewCollector(cfg)
	if err != nil {
		return nil, err
	}

	globalCollector = collector
	return globalCollector, nil
}

// Close closes the collector. The cache is in-memory and per scan, so there is
// nothing to release here.
func (c *Collector) Close() error {
	return nil
}

// ScanFSTrivyReport scans the specified directory and logs detailed scan steps.
func (c *Collector) ScanFSTrivyReport(ctx context.Context, path string, scanOptions sbom.ScanOptions, removeLayers bool) (*types.Report, error) {
	cache := newMemoryCache()

	artifactOption := getDefaultArtifactOption(scanOptions)

	artifactType := ftypes.TypeContainerImage
	if removeLayers {
		artifactType = ftypes.TypeFilesystem
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
	localScanner := local.NewService(applier, c.osScanner, c.langScanner, c.vulnClient)
	s := scan.NewService(localScanner, artifact)

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
