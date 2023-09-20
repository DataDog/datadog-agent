// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package utils offers a number of high level helpers to work with the configuration
package utils

import (
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/config"
)

// ConfFileDirectory returns the absolute path to the folder containing the config
// file used to populate the registry
func ConfFileDirectory(c config.ConfigReader) string {
	return filepath.Dir(c.ConfigFileUsed())
}
