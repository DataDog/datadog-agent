// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package main

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"golang.org/x/exp/slices"

	cdx "github.com/CycloneDX/cyclonedx-go"

	"github.com/aquasecurity/trivy-db/pkg/db"
	"github.com/aquasecurity/trivy/pkg/fanal/analyzer"
	"github.com/aquasecurity/trivy/pkg/fanal/applier"
	"github.com/aquasecurity/trivy/pkg/fanal/artifact"
	trivyartifactlocal "github.com/aquasecurity/trivy/pkg/fanal/artifact/local"
	"github.com/aquasecurity/trivy/pkg/fanal/artifact/vm"
	"github.com/aquasecurity/trivy/pkg/fanal/cache"
	trivyhandler "github.com/aquasecurity/trivy/pkg/fanal/handler"
	ftypes "github.com/aquasecurity/trivy/pkg/fanal/types"
	"github.com/aquasecurity/trivy/pkg/fanal/walker"
	"github.com/aquasecurity/trivy/pkg/sbom/cyclonedx"
	trivyscanner "github.com/aquasecurity/trivy/pkg/scanner"
	"github.com/aquasecurity/trivy/pkg/scanner/langpkg"
	"github.com/aquasecurity/trivy/pkg/scanner/local"
	"github.com/aquasecurity/trivy/pkg/scanner/ospkg"
	"github.com/aquasecurity/trivy/pkg/types"
	"github.com/aquasecurity/trivy/pkg/vulnerability"
	"github.com/aws/aws-sdk-go-v2/service/ebs"
)

func init() {
	trivyhandler.DeregisterPostHandler(ftypes.UnpackagedPostHandler)
}

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
	var disabledAnalyzers []analyzer.Type
	for _, a := range trivyAnalyzersAll {
		if !slices.Contains(allowedAnalyzers, a) {
			disabledAnalyzers = append(disabledAnalyzers, a)
		}
	}
	return disabledAnalyzers
}

func launchScannerTrivyLocal(ctx context.Context, opts scannerOptions) (*cdx.BOM, error) {
	trivyCache := newMemoryCache()
	trivyArtifact, err := trivyartifactlocal.NewArtifact(opts.Root, trivyCache, artifact.Option{
		Offline:           true,
		NoProgress:        true,
		DisabledAnalyzers: getTrivyDisabledAnalyzers(analyzer.TypeOSes),
		Parallel:          1,
		SBOMSources:       []string{},
		DisabledHandlers:  []ftypes.HandlerType{ftypes.UnpackagedPostHandler},
		OnlyDirs: []string{
			filepath.Join(opts.Root, "etc/**"),
			filepath.Join(opts.Root, "var/lib/dpkg/**"),
			filepath.Join(opts.Root, "var/lib/rpm/**"),
			filepath.Join(opts.Root, "lib/apk/**"),
		},
		AWSRegion: opts.Scan.ARN.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("could not create local trivy artifact: %w", err)
	}

	return doTrivyScan(ctx, opts.Scan, trivyArtifact, trivyCache)
}

func launchScannerTrivyVM(ctx context.Context, opts scannerOptions) (*cdx.BOM, error) {
	assumedRole := opts.Scan.Roles[opts.Scan.ARN.AccountID]
	cfg, err := newAWSConfig(ctx, opts.Scan.ARN.Region, assumedRole)
	if err != nil {
		return nil, err
	}

	ebsclient := ebs.NewFromConfig(cfg)
	_, snapshotID, _ := getARNResource(*opts.SnapshotARN)
	trivyCache := newMemoryCache()
	onlyDirs := []string{
		"/etc/**",
		"/var/lib/dpkg/**",
		"/var/lib/rpm/**",
		"/lib/apk/**",
	}
	w := walker.NewVM(nil, nil, onlyDirs)
	target := "ebs:" + snapshotID
	trivyArtifact, err := vm.NewArtifact(target, trivyCache, w, artifact.Option{
		Offline:           true,
		NoProgress:        true,
		DisabledAnalyzers: getTrivyDisabledAnalyzers(analyzer.TypeOSes),
		Parallel:          1,
		SBOMSources:       []string{},
		DisabledHandlers:  []ftypes.HandlerType{ftypes.UnpackagedPostHandler},
		AWSRegion:         opts.Scan.ARN.Region,
	})
	if err != nil {
		return nil, err
	}

	trivyArtifactEBS := trivyArtifact.(*vm.EBS)
	trivyArtifactEBS.SetEBS(EBSClientWithWalk{ebsclient})
	return doTrivyScan(ctx, opts.Scan, trivyArtifact, trivyCache)
}

func launchScannerTrivyLambda(ctx context.Context, opts scannerOptions) (*cdx.BOM, error) {
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
		AWSRegion:         opts.Scan.ARN.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("unable to create artifact from fs: %w", err)
	}
	return doTrivyScan(ctx, opts.Scan, trivyArtifact, trivyCache)
}

func doTrivyScan(ctx context.Context, scan *scanTask, trivyArtifact artifact.Artifact, trivyCache cache.LocalArtifactCache) (*cdx.BOM, error) {
	trivyOSScanner := ospkg.NewScanner()
	trivyLangScanner := langpkg.NewScanner()
	trivyVulnClient := vulnerability.NewClient(db.Config{})
	trivyApplier := applier.NewApplier(trivyCache)
	trivyLocalScanner := local.NewScanner(trivyApplier, trivyOSScanner, trivyLangScanner, trivyVulnClient)
	trivyScanner := trivyscanner.NewScanner(trivyLocalScanner, trivyArtifact)

	log.Debugf("%s: trivy: starting scan", scan)
	trivyReport, err := trivyScanner.ScanArtifact(ctx, types.ScanOptions{
		VulnType:            []string{},
		Scanners:            types.Scanners{types.VulnerabilityScanner},
		ScanRemovedPackages: false,
		ListAllPackages:     true,
	})
	if err != nil {
		return nil, fmt.Errorf("trivy scan failed: %w", err)
	}
	log.Debugf("%s: trivy: scan of artifact finished successfully", scan)
	marshaler := cyclonedx.NewMarshaler("")
	cyclonedxBOM, err := marshaler.Marshal(trivyReport)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal report to sbom format: %w", err)
	}
	return cyclonedxBOM, nil
}
