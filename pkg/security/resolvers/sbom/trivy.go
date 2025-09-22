// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && trivy

// Package sbom holds sbom related files
package sbom

import (
	"context"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/sbom/collectors/host"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/sbom/types"
)

type trivyCollector struct {
	inner *host.Collector
}

func newTrivyCollector(c *config.RuntimeSecurityConfig) (*trivyCollector, error) {
	opts := sbom.ScanOptions{
		Analyzers: c.SBOMResolverAnalyzers,
	}
	inner, err := host.NewCollectorForCWS(pkgconfigsetup.SystemProbe(), opts)
	if err != nil {
		return nil, err
	}

	return &trivyCollector{inner: inner}, nil
}

func (tc *trivyCollector) ScanInstalledPackages(ctx context.Context, root string) ([]types.PackageWithInstalledFiles, error) {
	report, err := tc.inner.DirectScanForTrivyReport(ctx, root)
	if err != nil {
		return nil, err
	}

	var pkgs []types.PackageWithInstalledFiles
	for _, result := range report.Results {
		for _, resultPkg := range result.Packages {
			pkg := types.PackageWithInstalledFiles{
				Package: types.Package{
					Name:       resultPkg.Name,
					Version:    resultPkg.Version,
					Epoch:      resultPkg.Epoch,
					Release:    resultPkg.Release,
					SrcVersion: resultPkg.SrcVersion,
					SrcEpoch:   resultPkg.SrcEpoch,
					SrcRelease: resultPkg.SrcRelease,
				},
				InstalledFiles: resultPkg.InstalledFiles,
			}
			pkgs = append(pkgs, pkg)
		}
	}
	return pkgs, nil
}
