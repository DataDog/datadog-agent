// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package tracers is a collection of helpers to fetch tracer configurations from the disk.
package tracers

import (
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/comp/core/config"
)

// ApplicationMonitoringConfig fetches the `application_monitoring.yaml` configurations from the disk.
func ApplicationMonitoringConfig(config config.Reader) (string, error) {
	return readConfigFile(config, "application_monitoring.yaml")
}

// ApplicationMonitoringConfigFleet fetches the `application_monitoring.yaml` configurations from the disk.
func ApplicationMonitoringConfigFleet(config config.Reader) (string, error) {
	return readConfigFile(config, "managed/datadog-agent/stable/application_monitoring.yaml")
}

func readConfigFile(config config.Reader, subpath string) (string, error) {
	configDir := filepath.Dir(config.ConfigFileUsed())
	path := filepath.Join(configDir, subpath)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "", nil // file not found, return empty YAML
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(contents), nil
}
