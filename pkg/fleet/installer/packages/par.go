// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package packages

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/service/systemd"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup/config"
)

// configurePARDefault covers native packages as well as OCI installs. On a
// fresh native Linux install, the shell installer still needs to populate the
// configuration from its example; do not create an incomplete datadog.yaml.
func configurePARDefault() error {
	e := env.FromEnv()
	return configurePARDefaultAt(paths.AgentConfigDir, runtime.GOOS, e.ProcessManagerEnabled, e.FIPSMode, systemd.IsRunning)
}

func configurePARDefaultAt(dir, goos string, procmgr, fips bool, systemdRunning func() (bool, error)) error {
	if !procmgr || fips || (goos != "linux" && goos != "windows") {
		return nil
	}
	path := filepath.Join(dir, "datadog.yaml")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		path += ".example"
	} else if err != nil {
		return err
	}
	// Neither configuration nor template exists for some package flavors.
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	enabled, err := config.PARInstallerDefault(path, goos, procmgr, fips, systemdRunning)
	if err != nil {
		return err
	}
	if enabled == nil {
		return nil
	}
	var cfg config.DatadogConfig
	cfg.PrivateActionRunner.Enabled = enabled
	if err := config.WriteConfig(path, cfg, 0640, true); err != nil {
		return fmt.Errorf("write PAR installer default: %w", err)
	}
	return nil
}
