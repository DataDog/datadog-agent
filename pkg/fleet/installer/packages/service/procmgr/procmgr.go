// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package procmgr provides helpers for managing dd-procmgrd process configs.
package procmgr

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/service/systemd"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	procmgrdUnit      = "datadog-agent-procmgrd.service"
	markerPath        = "/etc/datadog-agent/.procmgr-ddot-enabled"
	agentInstallDebRp = "/opt/datadog-agent"
)

// WriteConfig writes a procmgr YAML config file to destDir/configName.
func WriteConfig(destDir, configName, content string) error {
	dest := filepath.Join(destDir, configName)
	if err := os.WriteFile(dest, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write procmgr config to %s: %w", dest, err)
	}
	writeMarker()
	return nil
}

// RemoveConfig removes a procmgr YAML config file from destDir.
func RemoveConfig(destDir, configName string) {
	dest := filepath.Join(destDir, configName)
	if err := os.Remove(dest); err != nil && !os.IsNotExist(err) {
		log.Warnf("failed to remove procmgr config %s: %s", dest, err)
	}
}

// RestartDaemon restarts the dd-procmgrd systemd unit so it reloads its
// config directory and picks up newly added or removed process definitions.
func RestartDaemon(ctx context.Context) error {
	return systemd.RestartUnit(ctx, procmgrdUnit)
}

// writeMarker persists the opt-in marker so future upgrades preserve
// procmgr management without requiring the env var again.
func writeMarker() {
	if err := os.WriteFile(markerPath, []byte{}, 0644); err != nil {
		log.Warnf("failed to write procmgr opt-in marker %s: %s", markerPath, err)
	}
}
