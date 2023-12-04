package main

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"

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
	ftypes "github.com/aquasecurity/trivy/pkg/fanal/types"
	"github.com/aquasecurity/trivy/pkg/sbom/cyclonedx"
	"github.com/aquasecurity/trivy/pkg/scanner"
	"github.com/aquasecurity/trivy/pkg/scanner/local"
	"github.com/aquasecurity/trivy/pkg/types"
	"github.com/aquasecurity/trivy/pkg/vulnerability"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/ebs"
)

func launchScannerTrivyLocal(ctx context.Context, scan *scanTask, mountPoint string) (*sbommodel.SBOMEntity, error) {
	trivyDisabledAnalyzers := []analyzer.Type{analyzer.TypeSecret, analyzer.TypeLicenseFile}
	trivyDisabledAnalyzers = append(trivyDisabledAnalyzers, analyzer.TypeConfigFiles...)
	trivyDisabledAnalyzers = append(trivyDisabledAnalyzers, analyzer.TypeLanguages...)
	trivyCache := newMemoryCache()
	startTime := time.Now()
	trivyArtifact, err := trivyartifactlocal.NewArtifact(mountPoint, trivyCache, artifact.Option{
		Offline:           true,
		NoProgress:        true,
		DisabledAnalyzers: trivyDisabledAnalyzers,
		Slow:              false,
		SBOMSources:       []string{},
		DisabledHandlers:  []ftypes.HandlerType{ftypes.UnpackagedPostHandler},
		OnlyDirs: []string{
			filepath.Join(mountPoint, "etc"),
			filepath.Join(mountPoint, "var/lib/dpkg"),
			filepath.Join(mountPoint, "var/lib/rpm"),
			filepath.Join(mountPoint, "lib/apk"),
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
	return scanEBSSbomEntity(trivyReport, scan, duration)
}

func launchScannerTrivyVM(ctx context.Context, scan *scanTask, ebsclient *ebs.Client, snapshotARN arn.ARN) (*sbommodel.SBOMEntity, error) {
	trivyDisabledAnalyzers := []analyzer.Type{analyzer.TypeSecret, analyzer.TypeLicenseFile}
	trivyDisabledAnalyzers = append(trivyDisabledAnalyzers, analyzer.TypeConfigFiles...)
	trivyDisabledAnalyzers = append(trivyDisabledAnalyzers, analyzer.TypeLanguages...)
	startTime := time.Now()

	_, snapshotID, _ := getARNResource(snapshotARN)
	trivyCache := newMemoryCache()
	target := "ebs:" + snapshotID
	trivyArtifact, err := vm.NewArtifact(target, trivyCache, artifact.Option{
		Offline:           true,
		NoProgress:        true,
		DisabledAnalyzers: trivyDisabledAnalyzers,
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
	return scanEBSSbomEntity(trivyReport, scan, duration)
}

func launchScannerTrivyLambda(ctx context.Context, scan *scanTask, codePath string) (*sbommodel.SBOMEntity, error) {
	startTime := time.Now()

	trivyCache := newMemoryCache()
	trivyArtifact, err := trivyartifactlocal.NewArtifact(codePath, trivyCache, artifact.Option{
		Offline:           true,
		NoProgress:        true,
		DisabledAnalyzers: []analyzer.Type{},
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
	return scanLambdaSbomEntity(trivyReport, scan, duration)
}

func doTrivyScan(ctx context.Context, scan *scanTask, trivyArtifact artifact.Artifact, trivyCache cache.LocalArtifactCache) (*types.Report, error) {
	trivyDetector := ospkg.Detector{}
	trivyVulnClient := vulnerability.NewClient(db.Config{})
	trivyApplier := applier.NewApplier(trivyCache)
	trivyLocalScanner := local.NewScanner(trivyApplier, trivyDetector, trivyVulnClient)
	trivyScanner := scanner.NewScanner(trivyLocalScanner, trivyArtifact)

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
	return &trivyReport, nil
}

func scanEBSSbomEntity(trivyReport *types.Report, scan *scanTask, duration time.Duration) (*sbommodel.SBOMEntity, error) {
	marshaler := cyclonedx.NewMarshaler("")
	cyclonedxBOM, err := marshaler.Marshal(*trivyReport)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal report to sbom format: %w", err)
	}
	return &sbommodel.SBOMEntity{
		Status:             sbommodel.SBOMStatus_SUCCESS,
		Type:               sbommodel.SBOMSourceType_HOST_FILE_SYSTEM, // TODO: SBOMSourceType_EBS
		Id:                 scan.Hostname,
		InUse:              true,
		GeneratedAt:        timestamppb.New(time.Now()),
		GenerationDuration: convertDuration(duration),
		Hash:               "",
		Sbom: &sbommodel.SBOMEntity_Cyclonedx{
			Cyclonedx: convertBOM(cyclonedxBOM),
		},
	}, nil
}

func scanLambdaSbomEntity(trivyReport *types.Report, scan *scanTask, duration time.Duration) (*sbommodel.SBOMEntity, error) {
	marshaler := cyclonedx.NewMarshaler("")
	cyclonedxBOM, err := marshaler.Marshal(*trivyReport)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal report to sbom format: %w", err)
	}
	return &sbommodel.SBOMEntity{
		Status: sbommodel.SBOMStatus_SUCCESS,
		Type:   sbommodel.SBOMSourceType_CI_PIPELINE, // TODO: SBOMSourceType_LAMBDA
		Id:     scan.ARN.String(),
		InUse:  true,
		DdTags: []string{
			"git.repository_url:" + scan.ARN.String(),
			"git.branch:TODO", // XXX
		},
		GeneratedAt:        timestamppb.New(time.Now()),
		GenerationDuration: convertDuration(duration),
		Hash:               "",
		Sbom: &sbommodel.SBOMEntity_Cyclonedx{
			Cyclonedx: convertBOM(cyclonedxBOM),
		},
	}, nil
}
