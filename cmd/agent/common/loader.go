// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	config "github.com/DataDog/datadog-agent/comp/core/config/def"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/sbom/scanner"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
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

// LoadComponents configures several common Agent components:
// tagger, collector, scheduler and autodiscovery
func LoadComponents(_ secrets.Component, wmeta workloadmeta.Component, ac autodiscovery.Component, confdPath string) {
	confSearchPaths := []string{
		confdPath,
		filepath.Join(defaultpaths.GetDistPath(), "conf.d"),
		"",
	}

	setupAutoDiscovery(confSearchPaths, wmeta, ac)
}
