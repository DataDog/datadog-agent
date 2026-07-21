// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded"
)

const parProcmgrConfigName = "datadog-agent-action.yaml"

// writePARProcmgrConfig writes the dd-procmgr config for the Private Action Runner into the package
// processes.d. When present, the datadog-agent-action.service systemd unit is inhibited by its
// ConditionPathExists=! gate. No-op when the privateactionrunner binary is not shipped.
func writePARProcmgrConfig(installRoot string) error {
	parBinaryPath := filepath.Join(installRoot, "embedded", "bin", "privateactionrunner")
	if _, err := os.Stat(parBinaryPath); err != nil {
		return nil
	}
	processesDir := filepath.Join(installRoot, "processes.d")
	config := strings.ReplaceAll(embedded.PARProcessConfig, "/opt/datadog-agent", installRoot)
	if err := os.MkdirAll(processesDir, 0755); err != nil {
		return fmt.Errorf("failed to write PAR procmgr config: %w", err)
	}
	path := filepath.Join(processesDir, parProcmgrConfigName)
	return os.WriteFile(path, []byte(config), 0644)
}

func removePARProcmgrConfig(installRoot string) error {
	path := filepath.Join(installRoot, "processes.d", parProcmgrConfigName)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
