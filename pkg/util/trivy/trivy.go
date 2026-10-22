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
	"time"

	"github.com/aquasecurity/trivy-db/pkg/db"
	trivycache "github.com/aquasecurity/trivy/pkg/cache"
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
	v1 "github.com/google/go-containerregistry/pkg/v1"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/ddtrivy"
)

const (
	OSAnalyzers        = "os"        // OSAnalyzers defines an OS analyzer
	LanguagesAnalyzers = "languages" // LanguagesAnalyzers defines a language analyzer
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
	if scanOptions.OSOnly() {
		artifactOption = ddtrivy.TrivyOptionsOS(parallel)
	} else {
		artifactOption = ddtrivy.TrivyOptionsAll(parallel)
		// OnlyDirs filters the static paths an OS-only scan walks, so only
		// application scans take the additional directories.
		artifactOption.WalkerOption.OnlyDirs = append(artifactOption.WalkerOption.OnlyDirs, scanOptions.AdditionalDirs...)
	}

	// agent specific config, needed so that we don't download the Java DB at runtime
	artifactOption.OfflineJar = true

	return artifactOption
}

// scanWithOSFallback runs scan with the artifact options for scanOptions and
// reports whether the report lists the OS packages alone in place of the
// application scan. Without a deadline, ddtrivy's walk and analyzer timeouts
// bound the scan. With one, the deadline alone does. A scan with application
// analyzers then leaves the last sbom.OSFallbackTimeout before the deadline in
// reserve. When it runs out of its own time, scanWithOSFallback runs scan
// again with the OS analyzers alone in the reserve, so target still gets its
// OS packages. With the reserve or less left, as in a flare, the application
// scan and the OS scan get half of that time each.
func scanWithOSFallback(ctx context.Context, target string, scanOptions sbom.ScanOptions, scan func(context.Context, artifact.Option) (*types.Report, error)) (report *types.Report, osFallback bool, err error) {
	deadline, ok := ctx.Deadline()
	if !ok {
		report, err = scan(ctx, getDefaultArtifactOption(scanOptions))
		return report, false, err
	}

	deadlineOption := func(o sbom.ScanOptions) artifact.Option {
		artifactOption := getDefaultArtifactOption(o)
		artifactOption.AnalyzerTimeout = 0
		artifactOption.PostAnalyzerTimeout = 0
		return artifactOption
	}
	if scanOptions.OSOnly() {
		report, err = scan(ctx, deadlineOption(scanOptions))
		return report, false, err
	}

	reserve := sbom.OSFallbackTimeout
	if left := time.Until(deadline); left <= reserve {
		reserve = left / 2
	}
	appCtx, cancel := context.WithDeadline(ctx, deadline.Add(-reserve))
	defer cancel()
	report, err = scan(appCtx, deadlineOption(scanOptions))
	// Key on the deadline: a scan cut short can fail with an error that
	// wraps no deadline error.
	if err == nil || appCtx.Err() != context.DeadlineExceeded || ctx.Err() != nil {
		return report, false, err
	}

	log.Warnf("SBOM scan of %s with analyzers %v ran out of time, scanning OS packages only: %v", target, scanOptions.Analyzers, err)
	osOptions := scanOptions
	osOptions.Analyzers = []string{OSAnalyzers}
	report, err = scan(ctx, deadlineOption(osOptions))
	return report, err == nil, err
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

// scanFSTrivyReport scans path and reports whether the report lists the OS
// packages alone because the application scan ran out of time.
func (c *Collector) scanFSTrivyReport(ctx context.Context, path string, scanOptions sbom.ScanOptions, removeLayers bool) (*types.Report, bool, error) {
	artifactType := ftypes.TypeContainerImage
	if removeLayers {
		artifactType = ftypes.TypeFilesystem
	}
	report, osFallback, err := scanWithOSFallback(ctx, path, scanOptions, func(ctx context.Context, artifactOption artifact.Option) (*types.Report, error) {
		return ddtrivy.ScanRootFS(ctx, artifactOption, trivycache.NewMemoryCache(), path, artifactType)
	})
	if err != nil {
		return nil, false, fmt.Errorf("unable to scan rootfs, err: %w", err)
	}

	return report, osFallback, nil
}

// ScanFilesystem scans the specified directory and logs detailed scan steps.
func (c *Collector) ScanFilesystem(ctx context.Context, path string, scanOptions sbom.ScanOptions, removeLayers bool) (*Report, error) {
	return c.scanFilesystem(ctx, path, scanOptions, removeLayers, time.Time{})
}

// scanFilesystem scans path like ScanFilesystem and records imageCreated as
// the build time of the image mounted there.
func (c *Collector) scanFilesystem(ctx context.Context, path string, scanOptions sbom.ScanOptions, removeLayers bool, imageCreated time.Time) (*Report, error) {
	trivyReport, osFallback, err := c.scanFSTrivyReport(ctx, path, scanOptions, removeLayers)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal report to sbom format, err: %w", err)
	}
	trivyReport.Metadata.ImageConfig.Created = v1.Time{Time: imageCreated}

	hash, err := reportHash(trivyReport.Results, osFallback)
	if err != nil {
		return nil, err
	}
	return c.buildReport(trivyReport, hash, osFallback)
}

// reportHash returns the ID of a filesystem report listing results. The sbom
// check sends a heartbeat in place of a host SBOM with the ID it sent last. A
// fallback and a full scan of a host whose packages all come from the OS list
// the same results, so osFallback goes into the ID for the full scan to
// replace the fallback.
func reportHash(results types.Results, osFallback bool) (string, error) {
	hasher := sha256.New()
	if err := json.NewEncoder(hasher).Encode(results); err != nil {
		return "", fmt.Errorf("unable to compute hash for report: err: %w", err)
	}
	if osFallback {
		hasher.Write([]byte(sbom.OSFallbackProperty))
	}
	return "sha256:" + base64.StdEncoding.EncodeToString(hasher.Sum(nil)), nil
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

// buildReport turns trivyReport into a Report with id. osFallback marks a
// report that lists the OS packages alone because the application scan ran out
// of time.
func (c *Collector) buildReport(trivyReport *types.Report, id string, osFallback bool) (*Report, error) {
	log.Debugf("Found OS: %+v", trivyReport.Metadata.OS)
	pkgCount := 0
	for _, results := range trivyReport.Results {
		pkgCount += len(results.Packages)
	}
	log.Debugf("Found %d packages", pkgCount)

	return newReport(id, trivyReport, c.marshaler, reportOptions{
		dependencies:    c.config.computeDependencies,
		simplifyBomRefs: c.config.simplifyBomRefs,
		osFallback:      osFallback,
	})
}
