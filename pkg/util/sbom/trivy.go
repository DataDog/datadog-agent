// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sbom

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/aquasecurity/trivy-db/pkg/db"
	"github.com/aquasecurity/trivy/pkg/commands/operation"
	"github.com/aquasecurity/trivy/pkg/detector/ospkg"
	"github.com/aquasecurity/trivy/pkg/fanal/analyzer"
	"github.com/aquasecurity/trivy/pkg/fanal/applier"
	"github.com/aquasecurity/trivy/pkg/fanal/artifact"
	local2 "github.com/aquasecurity/trivy/pkg/fanal/artifact/local"
	"github.com/aquasecurity/trivy/pkg/fanal/cache"
	"github.com/aquasecurity/trivy/pkg/flag"
	pkgReport "github.com/aquasecurity/trivy/pkg/report"
	"github.com/aquasecurity/trivy/pkg/result"
	"github.com/aquasecurity/trivy/pkg/rpc/client"
	"github.com/aquasecurity/trivy/pkg/scanner"
	"github.com/aquasecurity/trivy/pkg/scanner/local"
	"github.com/aquasecurity/trivy/pkg/types"
	"github.com/aquasecurity/trivy/pkg/utils"
	"github.com/aquasecurity/trivy/pkg/vulnerability"
	"golang.org/x/xerrors"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// TrivyCollector uses trivy to generate a SBOM
type TrivyCollector struct {
	cache cache.Cache
}

// InitializeScanner defines the initialize function signature of scanner
type InitializeScanner func(context.Context, ScannerConfig) (scanner.Scanner, func(), error)

type ScannerConfig struct {
	// e.g. image name and file path
	Target string

	// Cache
	ArtifactCache      cache.ArtifactCache
	LocalArtifactCache cache.Cache

	// Client/Server options
	RemoteOption client.ScannerOption

	// Artifact options
	ArtifactOption artifact.Option
}

// filesystemStandaloneScanner initializes a filesystem scanner in standalone mode
func (c *TrivyCollector) filesystemStandaloneScanner(ctx context.Context, conf ScannerConfig) (scanner.Scanner, func(), error) {
	s, cleanup, err := c.initializeFilesystemScanner(ctx, conf.Target, conf.ArtifactCache, conf.LocalArtifactCache, conf.ArtifactOption)
	if err != nil {
		return scanner.Scanner{}, func() {}, xerrors.Errorf("unable to initialize a filesystem scanner: %w", err)
	}
	return s, cleanup, nil
}

func (c *TrivyCollector) initScannerConfig(opts flag.Options) (ScannerConfig, types.ScanOptions, error) {
	target := opts.Target
	if opts.Input != "" {
		target = opts.Input
	}

	scanOptions := types.ScanOptions{
		ScanRemovedPackages: opts.ScanRemovedPkgs, // this is valid only for 'image' subcommand
		ListAllPackages:     opts.ListAllPkgs,
		LicenseCategories:   opts.LicenseCategories,
	}

	return ScannerConfig{
		Target:             target,
		ArtifactCache:      c.cache,
		LocalArtifactCache: c.cache,
		ArtifactOption: artifact.Option{
			DisabledAnalyzers: c.disabledAnalyzers(opts),
			SkipFiles:         opts.SkipFiles,
			SkipDirs:          opts.SkipDirs,
			OnlyDirs:          opts.OnlyDirs,
			Offline:           opts.OfflineScan,
			NoProgress:        opts.NoProgress || opts.Quiet,
		},
	}, scanOptions, nil
}

func (c *TrivyCollector) disabledAnalyzers(opts flag.Options) []analyzer.Type {
	analyzers := opts.DisabledAnalyzers
	analyzers = append(analyzers, analyzer.TypeLanguages...)
	analyzers = append(analyzers, analyzer.TypeSecret)
	analyzers = append(analyzers, analyzer.TypeConfigFiles...)
	analyzers = append(analyzers, analyzer.TypeLicenseFile)
	return analyzers
}

func (c *TrivyCollector) scan(ctx context.Context, opts flag.Options, initializeScanner InitializeScanner) (types.Report, error) {
	scannerConfig, scanOptions, err := c.initScannerConfig(opts)
	if err != nil {
		return types.Report{}, err
	}

	s, cleanup, err := initializeScanner(ctx, scannerConfig)
	if err != nil {
		return types.Report{}, xerrors.Errorf("unable to initialize a scanner: %w", err)
	}
	defer cleanup()

	report, err := s.ScanArtifact(ctx, scanOptions)
	if err != nil {
		return types.Report{}, xerrors.Errorf("scan failed: %w", err)
	}
	return report, nil
}

func (c *TrivyCollector) scanArtifact(ctx context.Context, opts flag.Options, initializeScanner InitializeScanner) (types.Report, error) {
	report, err := c.scan(ctx, opts, initializeScanner)
	if err != nil {
		return types.Report{}, xerrors.Errorf("scan error: %w", err)
	}

	return report, nil
}

func (c *TrivyCollector) initializeFilesystemScanner(ctx context.Context, path string, artifactCache cache.ArtifactCache, localArtifactCache cache.Cache, artifactOption artifact.Option) (scanner.Scanner, func(), error) {
	applierApplier := applier.NewApplier(localArtifactCache)
	detector := ospkg.Detector{}
	config := db.Config{}
	client := vulnerability.NewClient(config)
	localScanner := local.NewScanner(applierApplier, detector, client)
	artifactArtifact, err := local2.NewArtifact(path, artifactCache, artifactOption)
	if err != nil {
		return scanner.Scanner{}, nil, err
	}
	scannerScanner := scanner.NewScanner(localScanner, artifactArtifact)
	return scannerScanner, func() {
	}, nil
}

func (c *TrivyCollector) filterReport(ctx context.Context, opts flag.Options, report types.Report) (types.Report, error) {
	results := report.Results

	// Filter results
	for i := range results {
		err := result.Filter(ctx, &results[i], opts.Severities, opts.IgnoreUnfixed, opts.IncludeNonFailures,
			opts.IgnoreFile, opts.IgnorePolicy, opts.IgnoredLicenses)
		if err != nil {
			return types.Report{}, xerrors.Errorf("unable to filter vulnerabilities: %w", err)
		}
	}

	return report, nil
}

func (c *TrivyCollector) report(opts flag.Options, report types.Report) error {
	if err := pkgReport.Write(report, pkgReport.Option{
		AppVersion:         opts.AppVersion,
		Format:             opts.Format,
		Output:             opts.Output,
		Tree:               opts.DependencyTree,
		Severities:         opts.Severities,
		OutputTemplate:     opts.Template,
		IncludeNonFailures: opts.IncludeNonFailures,
		Trace:              opts.Trace,
	}); err != nil {
		return xerrors.Errorf("unable to write results: %w", err)
	}

	return nil
}

// ScanRootfs generates a SBOM from a filesystem
func (c *TrivyCollector) ScanRootfs(ctx context.Context, root string) (*types.Report, error) {
	reportFlagGroup := flag.NewReportFlagGroup()
	fsFlags := &flag.Flags{
		ReportFlagGroup: reportFlagGroup,
		ScanFlagGroup:   flag.NewScanFlagGroup()}
	globalFlags := flag.NewGlobalFlagGroup()

	opts, err := fsFlags.ToOptions("", []string{root}, globalFlags, os.Stdout)
	if err != nil {
		return nil, err
	}

	opts.Format = "table"
	opts.Timeout = 60 * time.Second
	opts.ListAllPkgs = true
	opts.OnlyDirs = []string{"/etc", "/var/lib/dpkg", "/var/lib/rpm", "/lib/apk"}

	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	defer func() {
		if errors.Is(err, context.DeadlineExceeded) {
			log.Warnf("increase --timeout value")
		}
	}()

	report, err := c.scanArtifact(ctx, opts, c.filesystemStandaloneScanner)
	if err != nil {
		return nil, fmt.Errorf("rootfs scan error: %w", err)
	}

	report, err = c.filterReport(ctx, opts, report)
	if err != nil {
		return nil, fmt.Errorf("filter error: %w", err)
	}

	if err = c.report(opts, report); err != nil {
		return nil, fmt.Errorf("report error: %w", err)
	}

	jsonContent, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return nil, err
	}
	fmt.Println(string(jsonContent))

	return &report, nil
}

// NewTrivyCollector returns a new trivy SBOM collector
func NewTrivyCollector() (*TrivyCollector, error) {
	utils.SetCacheDir(utils.DefaultCacheDir())
	cacheClient, err := operation.NewCache(flag.CacheOptions{})
	if err != nil {
		return nil, err
	}

	return &TrivyCollector{
		cache: cacheClient,
	}, nil
}
