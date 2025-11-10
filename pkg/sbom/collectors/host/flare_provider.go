// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package host

import (
	"context"
	"encoding/json"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/pkg/sbom/scanner"
)

// FlareProvider generates a host SBOM and adds it to the flare.
type FlareProvider struct {
	Config config.Component
}

// ProvideFlare generates a host SBOM and adds it to the flare.
func (p *FlareProvider) ProvideFlare(fb flaretypes.FlareBuilder) error {
	if !p.Config.GetBool("sbom.host.enabled") {
		return nil
	}

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

	cycloneDX := scanResult.Report.ToCycloneDX()
	jsonContent, err := json.MarshalIndent(cycloneDX, "", "  ")
	if err != nil {
		return err
	}

	return fb.AddFileWithoutScrubbing("host-sbom.json", jsonContent)
}
