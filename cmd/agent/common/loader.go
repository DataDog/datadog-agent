// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/DataDog/datadog-agent/cmd/agent/common/path"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/sbom/scanner"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// GetWorkloadmetaInit provides the InitHelper for workloadmeta so it can be injected as a Param
// at workloadmeta comp fx injection.
func GetWorkloadmetaInit() workloadmeta.InitHelper {
	return func(ctx context.Context, wm workloadmeta.Component, cfg config.Component) error {
		// SBOM scanner needs to be called here as initialization is required prior to the
		// catalog getting instantiated and initialized.
		sbomScanner, err := scanner.CreateGlobalScanner(cfg, optional.NewOption(wm))
		if err != nil {
			return fmt.Errorf("failed to create SBOM scanner: %s", err)
		} else if sbomScanner != nil {
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
		filepath.Join(path.GetDistPath(), "conf.d"),
		"",
	}

	// TODO: (components) - This is a temporary fix to start the autodiscovery component in CLI mode (agent flare and diagnose in forcelocal checks)
	// because the autodiscovery component is not started by the agent automatically. Probably we can start it inside
	// fx lifecycle and remove this call.
	if !ac.IsStarted() {
		ac.Start()
	}
	setupAutoDiscovery(confSearchPaths, wmeta, ac)
}
