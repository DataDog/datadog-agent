// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package autoconnections

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

//go:embed script-config.yaml
var defaultScriptConfig []byte

// ConfigWriter handles creation of script configuration files
type ConfigWriter struct {
	BaseDir string
}

func NewDefaultConfigWriter() ConfigWriter {
	return ConfigWriter{BaseDir: PrivateActionRunnerBaseDir}
}

// EnsureScriptBundleConfig creates the script bundle configuration file if it doesn't exist
// Returns true if file was created, false if it already existed
func (w ConfigWriter) EnsureScriptBundleConfig() (bool, error) {
	configPath := filepath.Join(w.BaseDir, ScriptConfigFileName)

	// Check if file already exists
	if _, err := os.Stat(configPath); err == nil {
		log.Debugf("Config file already exists: %s", configPath)
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("failed to check config file %s: %w", configPath, err)
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(w.BaseDir, ConfigDirPermissions); err != nil {
		return false, fmt.Errorf("failed to create config directory %s: %w", w.BaseDir, err)
	}

	// Write the embedded static config file
	if err := os.WriteFile(configPath, defaultScriptConfig, ConfigFilePermissions); err != nil {
		return false, fmt.Errorf("failed to write config file %s: %w", configPath, err)
	}

	log.Infof("Created script config file: %s", configPath)
	return true, nil
}
