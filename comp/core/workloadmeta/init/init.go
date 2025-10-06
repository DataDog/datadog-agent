// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package init provides GetWorkloadmetaInit
package init

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/sbom/scanner"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// GetWorkloadmetaInit provides the InitHelper for workloadmeta so it can be injected as a Param
// at workloadmeta comp fx injection.
func GetWorkloadmetaInit() workloadmeta.InitHelper {
	return func(ctx context.Context, wm workloadmeta.Component, cfg config.Component) error {
		// SBOM scanner needs to be called here as initialization is required prior to the
		// catalog getting instantiated and initialized.
		if cfg.GetBool("sbom.host.enabled") || cfg.GetBool("sbom.container_image.enabled") {
			sbomScanner, err := scanner.CreateGlobalScanner(cfg, option.New(wm))
			if err != nil {
				return fmt.Errorf("failed to create SBOM scanner: %s", err)
			}

			sbomScanner.Start(ctx)
		}

		return nil
	}
}
