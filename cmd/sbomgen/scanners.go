// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && trivy

package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/util/trivy"
)

func runScanFS(path string, analyzers []string, fast bool, removeLayers bool) error {
	collector := trivy.NewCollectorForCLI()

	ctx := context.Background()
	report, err := collector.ScanFilesystem(ctx, path, sbom.ScanOptions{
		Analyzers: analyzers,
		Fast:      fast,
	}, removeLayers)
	if err != nil {
		return err
	}

	return outputReport(report)
}

func outputReport(report sbom.Report) error {
	bom := report.ToCycloneDX()
	bomJSON, err := json.MarshalIndent(bom, "", "  ")
	if err != nil {
		return err
	}

	fmt.Printf("sbom: %+v\n", string(bomJSON))
	return nil
}
