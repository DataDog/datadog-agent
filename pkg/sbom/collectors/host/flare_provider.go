// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package host

import (
	"context"
	"encoding/json"
	"time"

	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"

	"github.com/DataDog/datadog-agent/pkg/sbom/scanner"
)

// FlareProvider generates a host SBOM and adds it to the flare.
func FlareProvider(fb flaretypes.FlareBuilder) error {
	globalScanner := scanner.GetGlobalScanner()
	if globalScanner == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	scanRequest := NewHostScanRequest()
	scanResult := globalScanner.PerformScan(ctx, scanRequest, globalScanner.GetCollector(scanRequest.Collector()))
	if scanResult.Error != nil {
		return scanResult.Error
	}

	cycloneDX, err := scanResult.Report.ToCycloneDX()
	if err != nil {
		return err
	}

	jsonContent, err := json.MarshalIndent(cycloneDX, "", "  ")
	if err != nil {
		return err
	}

	return fb.AddFile("host-sbom.json", jsonContent)
}
