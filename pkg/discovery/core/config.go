// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

// Package core provides the core functionality for service discovery.
package core

const (
	// MaxCommLen is maximum command name length to process when checking for non-reportable commands,
	// is one byte less (excludes end of line) than the maximum of /proc/<pid>/comm
	// defined in https://man7.org/linux/man-pages/man5/proc.5.html.
	MaxCommLen = 15
)

// DiscoveryConfig holds the configuration for service discovery.
type DiscoveryConfig struct {
}

// NewConfig creates a new DiscoveryConfig with default values.
func NewConfig() *DiscoveryConfig {
	return &DiscoveryConfig{}
}
