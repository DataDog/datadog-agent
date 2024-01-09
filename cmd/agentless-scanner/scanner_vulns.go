// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package main

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"golang.org/x/exp/slices"

	sbommodel "github.com/DataDog/agent-payload/v5/sbom"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/aquasecurity/trivy-db/pkg/db"
	"github.com/aquasecurity/trivy/pkg/detector/ospkg"
	"github.com/aquasecurity/trivy/pkg/fanal/analyzer"
	"github.com/aquasecurity/trivy/pkg/fanal/applier"
	"github.com/aquasecurity/trivy/pkg/fanal/artifact"
	trivyartifactlocal "github.com/aquasecurity/trivy/pkg/fanal/artifact/local"
	"github.com/aquasecurity/trivy/pkg/fanal/artifact/vm"
	"github.com/aquasecurity/trivy/pkg/fanal/cache"
	trivyhandler "github.com/aquasecurity/trivy/pkg/fanal/handler"
	ftypes "github.com/aquasecurity/trivy/pkg/fanal/types"
	"github.com/aquasecurity/trivy/pkg/sbom/cyclonedx"
	trivyscanner "github.com/aquasecurity/trivy/pkg/scanner"
	"github.com/aquasecurity/trivy/pkg/scanner/local"
	"github.com/aquasecurity/trivy/pkg/types"
	"github.com/aquasecurity/trivy/pkg/vulnerability"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
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

func launchScannerTrivy(ctx context.Context, opts scannerOptions) (*sbommodel.SBOMEntity, error) {
	switch opts.Mode {
	case "local":
		return launchScannerTrivyLocal(ctx, opts.Scan, opts.Root, opts.EntityType, opts.EntityID, opts.EntityTags)
	case "vm":
		return launchScannerTrivyVM(ctx, opts.Scan, *opts.SnapshotARN, opts.EntityType, opts.EntityID, opts.EntityTags)
	case "lambda":
		return launchScannerTrivyLambda(ctx, opts.Scan, opts.Root, opts.EntityType, opts.EntityID, opts.EntityTags)
	default:
		return nil, fmt.Errorf("unknown vuln scanner mode %q", opts.Mode)
	}
}

func launchScannerTrivyLocal(ctx context.Context, scan *scanTask, root string, entityType sbommodel.SBOMSourceType, entityID string, entityTags []string) (*sbommodel.SBOMEntity, error) {
	trivyCache := newMemoryCache()
	startTime := time.Now()
	trivyArtifact, err := trivyartifactlocal.NewArtifact(root, trivyCache, artifact.Option{
		Offline:           true,
		NoProgress:        true,
		DisabledAnalyzers: getTrivyDisabledAnalyzers(analyzer.TypeOSes),
		Slow:              false,
		SBOMSources:       []string{},
		DisabledHandlers:  []ftypes.HandlerType{ftypes.UnpackagedPostHandler},
		OnlyDirs: []string{
			filepath.Join(root, "etc"),
			filepath.Join(root, "var/lib/dpkg"),
			filepath.Join(root, "var/lib/rpm"),
			filepath.Join(root, "lib/apk"),
		},
		AWSRegion: scan.ARN.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("could not create local trivy artifact: %w", err)
	}

	trivyReport, err := doTrivyScan(ctx, scan, trivyArtifact, trivyCache)
	if err != nil {
		return nil, err
	}
	duration := time.Since(startTime)
	return scanSbomEntity(
		trivyReport,
		scan,
		duration,
		entityType,
		entityID,
		entityTags)
}

func launchScannerTrivyVM(ctx context.Context, scan *scanTask, snapshotARN arn.ARN, entityType sbommodel.SBOMSourceType, entityID string, entityTags []string) (*sbommodel.SBOMEntity, error) {
	assumedRole := scan.Roles[scan.ARN.AccountID]
	cfg, err := newAWSConfig(ctx, scan.ARN.Region, assumedRole)
	if err != nil {
		return nil, err
	}

	ebsclient := ebs.NewFromConfig(cfg)
	startTime := time.Now()

	_, snapshotID, _ := getARNResource(snapshotARN)
	trivyCache := newMemoryCache()
	target := "ebs:" + snapshotID
	trivyArtifact, err := vm.NewArtifact(target, trivyCache, artifact.Option{
		Offline:           true,
		NoProgress:        true,
		DisabledAnalyzers: getTrivyDisabledAnalyzers(analyzer.TypeOSes),
		Slow:              false,
		SBOMSources:       []string{},
		DisabledHandlers:  []ftypes.HandlerType{ftypes.UnpackagedPostHandler},
		OnlyDirs:          []string{"etc", "var/lib/dpkg", "var/lib/rpm", "lib/apk"},
		AWSRegion:         scan.ARN.Region,
	})
	if err != nil {
		return nil, err
	}

	trivyArtifactEBS := trivyArtifact.(*vm.EBS)
	trivyArtifactEBS.SetEBS(EBSClientWithWalk{ebsclient})
	trivyReport, err := doTrivyScan(ctx, scan, trivyArtifact, trivyCache)
	if err != nil {
		return nil, err
	}
	duration := time.Since(startTime)
	return scanSbomEntity(
		trivyReport,
		scan,
		duration,
		entityType,
		entityID,
		entityTags)
}

func launchScannerTrivyLambda(ctx context.Context, scan *scanTask, root string, entityType sbommodel.SBOMSourceType, entityID string, entityTags []string) (*sbommodel.SBOMEntity, error) {
	startTime := time.Now()

	var allowedAnalyzers []analyzer.Type
	allowedAnalyzers = append(allowedAnalyzers, analyzer.TypeLanguages...)
	allowedAnalyzers = append(allowedAnalyzers, analyzer.TypeLockfiles...)
	allowedAnalyzers = append(allowedAnalyzers, analyzer.TypeIndividualPkgs...)
	trivyCache := newMemoryCache()
	trivyArtifact, err := trivyartifactlocal.NewArtifact(root, trivyCache, artifact.Option{
		Offline:           true,
		NoProgress:        true,
		DisabledAnalyzers: getTrivyDisabledAnalyzers(allowedAnalyzers),
		Slow:              true,
		SBOMSources:       []string{},
		AWSRegion:         scan.ARN.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("unable to create artifact from fs: %w", err)
	}
	trivyReport, err := doTrivyScan(ctx, scan, trivyArtifact, trivyCache)
	if err != nil {
		return nil, err
	}
	duration := time.Since(startTime)
	return scanSbomEntity(
		trivyReport,
		scan,
		duration,
		entityType,
		entityID,
		entityTags,
	)
}

func doTrivyScan(ctx context.Context, scan *scanTask, trivyArtifact artifact.Artifact, trivyCache cache.LocalArtifactCache) (*types.Report, error) {
	trivyDetector := ospkg.Detector{}
	trivyVulnClient := vulnerability.NewClient(db.Config{})
	trivyApplier := applier.NewApplier(trivyCache)
	trivyLocalScanner := local.NewScanner(trivyApplier, trivyDetector, trivyVulnClient)
	trivyScanner := trivyscanner.NewScanner(trivyLocalScanner, trivyArtifact)

	log.Debugf("trivy: starting scan of artifact %s", scan)
	trivyReport, err := trivyScanner.ScanArtifact(ctx, types.ScanOptions{
		VulnType:            []string{},
		SecurityChecks:      []string{},
		ScanRemovedPackages: false,
		ListAllPackages:     true,
	})
	if err != nil {
		return nil, fmt.Errorf("trivy scan failed: %w", err)
	}
	log.Debugf("trivy: scan of artifact %s finished successfully", scan)
	return &trivyReport, nil
}

func scanSbomEntity(trivyReport *types.Report, scan *scanTask, duration time.Duration, entityType sbommodel.SBOMSourceType, entityID string, entityTags []string) (*sbommodel.SBOMEntity, error) {
	marshaler := cyclonedx.NewMarshaler("")
	cyclonedxBOM, err := marshaler.Marshal(*trivyReport)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal report to sbom format: %w", err)
	}
	return &sbommodel.SBOMEntity{
		Status: sbommodel.SBOMStatus_SUCCESS,
		Type:   entityType,
		Id:     entityID,
		InUse:  true,
		DdTags: append([]string{
			fmt.Sprintf("region:%s", scan.ARN.Region),
			fmt.Sprintf("account_id:%s", scan.ARN.AccountID),
		}, entityTags...),
		GeneratedAt:        timestamppb.New(time.Now()),
		GenerationDuration: convertDuration(duration),
		Hash:               "",
		Sbom: &sbommodel.SBOMEntity_Cyclonedx{
			Cyclonedx: convertBOM(cyclonedxBOM),
		},
	}, nil
}
