// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package scanners implements the different scanners that can be launched by
// our agentless scanner.
package scanners

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/agentless/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"golang.org/x/exp/slices"

	cdx "github.com/CycloneDX/cyclonedx-go"

	"github.com/aquasecurity/trivy-db/pkg/db"
	"github.com/aquasecurity/trivy/pkg/fanal/analyzer"
	"github.com/aquasecurity/trivy/pkg/fanal/applier"
	"github.com/aquasecurity/trivy/pkg/fanal/artifact"
	trivyartifactlocal "github.com/aquasecurity/trivy/pkg/fanal/artifact/local"
	"github.com/aquasecurity/trivy/pkg/fanal/cache"
	trivyhandler "github.com/aquasecurity/trivy/pkg/fanal/handler"
	ftypes "github.com/aquasecurity/trivy/pkg/fanal/types"
	"github.com/aquasecurity/trivy/pkg/javadb"
	"github.com/aquasecurity/trivy/pkg/sbom/cyclonedx"
	trivyscanner "github.com/aquasecurity/trivy/pkg/scanner"
	"github.com/aquasecurity/trivy/pkg/scanner/langpkg"
	"github.com/aquasecurity/trivy/pkg/scanner/local"
	"github.com/aquasecurity/trivy/pkg/scanner/ospkg"
	trivytypes "github.com/aquasecurity/trivy/pkg/types"
	"github.com/aquasecurity/trivy/pkg/vulnerability"
)

const (
	tyivyCacheDir                = "/tmp/trivy"
	trivyDefaultJavaDBRepository = "ghcr.io/aquasecurity/trivy-java-db"
)

var trivyInitOnce sync.Once

func getTrivyDisabledAnalyzers(allowedAnalyzers []analyzer.Type) []analyzer.Type {
	var trivyAnalyzersAll = []analyzer.Type{}
	trivyAnalyzersAll = append(trivyAnalyzersAll, analyzer.TypeOSes...)
	trivyAnalyzersAll = append(trivyAnalyzersAll, analyzer.TypeLanguages...)
	trivyAnalyzersAll = append(trivyAnalyzersAll, analyzer.TypeLockfiles...)
	trivyAnalyzersAll = append(trivyAnalyzersAll, analyzer.TypeIndividualPkgs...)
	trivyAnalyzersAll = append(trivyAnalyzersAll, analyzer.TypeConfigFiles...)
	trivyAnalyzersAll = append(trivyAnalyzersAll, analyzer.TypeLicenseFile)
	trivyAnalyzersAll = append(trivyAnalyzersAll, analyzer.TypeSecret)
	trivyAnalyzersAll = append(trivyAnalyzersAll, analyzer.TypeRedHatContentManifestType)
	trivyAnalyzersAll = append(trivyAnalyzersAll, analyzer.TypeRedHatDockerfileType)
	trivyAnalyzersAll = append(trivyAnalyzersAll, analyzer.TypeApkCommand)
	trivyAnalyzersAll = append(trivyAnalyzersAll, analyzer.TypeHistoryDockerfile)
	trivyAnalyzersAll = append(trivyAnalyzersAll, analyzer.TypeImageConfigSecret)

	var disabledAnalyzers []analyzer.Type
	for _, a := range trivyAnalyzersAll {
		if !slices.Contains(allowedAnalyzers, a) {
			disabledAnalyzers = append(disabledAnalyzers, a)
		}
	}
	return disabledAnalyzers
}

// LaunchTrivyHost launches a trivy scan on a directory.
func LaunchTrivyHost(ctx context.Context, opts types.ScannerOptions) (*cdx.BOM, error) {
	trivyCache := newMemoryCache()
	trivyArtifact, err := trivyartifactlocal.NewArtifact(opts.Root, trivyCache, artifact.Option{
		Offline:           true,
		NoProgress:        true,
		DisabledAnalyzers: getTrivyDisabledAnalyzers(analyzer.TypeOSes),
		Parallel:          1,
		SBOMSources:       []string{},
		DisabledHandlers:  []ftypes.HandlerType{ftypes.UnpackagedPostHandler},
		OnlyDirs: []string{
			filepath.Join(opts.Root, "etc/*"),
			filepath.Join(opts.Root, "usr/lib/*"),
			filepath.Join(opts.Root, "var/lib/dpkg/*"),
			filepath.Join(opts.Root, "var/lib/rpm/*"),
			filepath.Join(opts.Root, "usr/lib/sysimage/rpm/*"),
			filepath.Join(opts.Root, "lib/apk/*"),
		},
		AWSRegion: opts.Scan.TargetID.Region(),
	})
	if err != nil {
		return nil, fmt.Errorf("could not create local trivy artifact: %w", err)
	}
	return doTrivyScan(ctx, opts.Scan, trivyArtifact, trivyCache)
}

// LaunchTrivyApp launches a trivy scan on a directory for application SBOMs.
func LaunchTrivyApp(ctx context.Context, opts types.ScannerOptions) (*cdx.BOM, error) {
	var allowedAnalyzers []analyzer.Type
	allowedAnalyzers = append(allowedAnalyzers, analyzer.TypeLanguages...)
	allowedAnalyzers = append(allowedAnalyzers, analyzer.TypeLockfiles...)
	allowedAnalyzers = append(allowedAnalyzers, analyzer.TypeIndividualPkgs...)
	trivyCache := newMemoryCache()
	trivyArtifact, err := trivyartifactlocal.NewArtifact(opts.Root, trivyCache, artifact.Option{
		Offline:           true,
		NoProgress:        true,
		DisabledAnalyzers: getTrivyDisabledAnalyzers(allowedAnalyzers),
		Parallel:          1,
		SBOMSources:       []string{},
		DisabledHandlers:  []ftypes.HandlerType{ftypes.UnpackagedPostHandler},
		AWSRegion:         opts.Scan.TargetID.Region(),
	})
	if err != nil {
		return nil, fmt.Errorf("unable to create artifact from fs: %w", err)
	}
	return doTrivyScan(ctx, opts.Scan, trivyArtifact, trivyCache)
}

func doTrivyScan(ctx context.Context, scan *types.ScanTask, trivyArtifact artifact.Artifact, trivyCache cache.LocalArtifactCache) (*cdx.BOM, error) {
	trivyInitOnce.Do(func() {
		// Init the JavaDB required for trivy application scans
		javadb.Init(tyivyCacheDir, trivyDefaultJavaDBRepository, false, false, ftypes.RegistryOptions{})

		// Making sure the Unpackaged post handler relying on external DBs is
		// deregistered
		trivyhandler.DeregisterPostHandler(ftypes.UnpackagedPostHandler)
	})

	trivyOSScanner := ospkg.NewScanner()
	trivyLangScanner := langpkg.NewScanner()
	trivyVulnClient := vulnerability.NewClient(db.Config{})
	trivyApplier := applier.NewApplier(trivyCache)
	trivyLocalScanner := local.NewScanner(trivyApplier, trivyOSScanner, trivyLangScanner, trivyVulnClient)
	trivyScanner := trivyscanner.NewScanner(trivyLocalScanner, trivyArtifact)

	log.Debugf("%s: trivy: starting scan", scan)
	trivyReport, err := trivyScanner.ScanArtifact(ctx, trivytypes.ScanOptions{
		VulnType:            []string{},
		Scanners:            trivytypes.Scanners{trivytypes.VulnerabilityScanner},
		ScanRemovedPackages: false,
		ListAllPackages:     true,
	})
	if err != nil {
		return nil, fmt.Errorf("trivy scan failed: %w", err)
	}
	log.Debugf("%s: trivy: scan of artifact finished successfully", scan)
	marshaler := cyclonedx.NewMarshaler("")
	cyclonedxBOM, err := marshaler.MarshalReport(ctx, trivyReport)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal report to sbom format: %w", err)
	}
	return cyclonedxBOM, nil
}
