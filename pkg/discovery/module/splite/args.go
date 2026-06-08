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
	// ReexecArgs is the full system-probe invocation that system-probe-lite
	// re-execs into when it is asked to transition back to the full
	// system-probe. Empty means transition is disabled. Passed as trailing
	// arguments after a "--" separator.
	ReexecArgs []string

	// IPC connection parameters for the agent's remote-config gRPC endpoint.
	// When all are set, system-probe-lite polls remote config itself to learn
	// the Live Debugger toggle. Empty means remote-config polling is disabled.
	IPCAddress    string // --ipc-address
	IPCPort       string // --ipc-port
	AuthTokenPath string // --auth-token-path
	IPCCertPath   string // --ipc-cert-path
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
	if c.IPCAddress != "" {
		args = append(args, "--ipc-address", c.IPCAddress)
	}
	if c.IPCPort != "" {
		args = append(args, "--ipc-port", c.IPCPort)
	}
	if c.AuthTokenPath != "" {
		args = append(args, "--auth-token-path", c.AuthTokenPath)
	}
	if c.IPCCertPath != "" {
		args = append(args, "--ipc-cert-path", c.IPCCertPath)
	}
	if len(c.ReexecArgs) > 0 {
		args = append(args, "--")
		args = append(args, c.ReexecArgs...)
	}
	return args
}
