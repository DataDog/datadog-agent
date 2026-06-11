// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

// Package splite provides shared CLI argument definitions for the system-probe-lite binary.
package splite

// Config holds the CLI arguments for the system-probe-lite binary.
// Socket is required; all other fields are optional (empty = omit).
type Config struct {
	Socket   string // required, maps to --socket
	LogLevel string // optional, maps to --log-level
	LogFile  string // optional, maps to --log-file
	PIDFile  string // optional, maps to --pid
}

// Args returns the command-line arguments for the system-probe-lite binary.
func (c *Config) Args() []string {
	args := []string{"run", "--socket", c.Socket}
	if c.LogLevel != "" {
		args = append(args, "--log-level", c.LogLevel)
	}
	if c.LogFile != "" {
		args = append(args, "--log-file", c.LogFile)
	}
	if c.PIDFile != "" {
		args = append(args, "--pid", c.PIDFile)
	}
	return args
}
