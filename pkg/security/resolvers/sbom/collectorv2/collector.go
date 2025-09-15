// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && trivy

// Package collectorv2 holds sbom related files
package collectorv2

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/aquasecurity/trivy/pkg/types"
)

// OSScanner is responsible for scanning the host OS for packages
type OSScanner struct {
	scanners []actualScanner
}

type actualScanner interface {
	ListPackages(ctx context.Context, root string) (types.Result, error)
}

// NewOSScanner returns a new OSScanner
func NewOSScanner() *OSScanner {
	return &OSScanner{
		scanners: []actualScanner{
			&dpkgScanner{},
		},
	}
}

// DirectScanForTrivyReport scans the given rootfs and returns a trivy report
func (s *OSScanner) DirectScanForTrivyReport(ctx context.Context, root string) (*types.Report, error) {
	report := &types.Report{}
	for _, scanner := range s.scanners {
		result, err := scanner.ListPackages(ctx, root)
		if err != nil {
			seclog.Errorf("failed to list packages with dpkg scanner: %v", err)
		}
		report.Results = append(report.Results, result)
	}
	return report, nil
}
