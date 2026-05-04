// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package installinfo exposes install info and version history for the agent.
package installinfo

// team: agent-configuration

// InstallInfo contains metadata on how the Agent was installed.
type InstallInfo struct {
	Tool             string `json:"tool" yaml:"tool"`
	ToolVersion      string `json:"tool_version" yaml:"tool_version"`
	InstallerVersion string `json:"installer_version" yaml:"installer_version"`
}

// Component is the component type.
type Component interface {
	// Get returns information about how the Agent was installed.
	// A runtime override (written via the /install-info POST endpoint) takes precedence
	// over env vars and the install_info file.
	Get() (*InstallInfo, error)
}
