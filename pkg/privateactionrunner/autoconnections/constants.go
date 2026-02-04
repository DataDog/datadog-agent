// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package autoconnections

import "path/filepath"

const (
	PrivateActionRunnerBaseDir = "/etc/privateactionrunner"
	ScriptConfigFileName       = "script-config.yaml"
	ConfigDirPermissions       = 0755 // rwxr-xr-x
	ConfigFilePermissions      = 0640 // rw-r-----
)

// GetScriptConfigPath returns the full path for the script configuration file
// Returns: "/etc/privateactionrunner/script-config.yaml"
func GetScriptConfigPath() string {
	return filepath.Join(PrivateActionRunnerBaseDir, ScriptConfigFileName)
}
