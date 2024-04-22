// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package traceroute

// MacTraceroute defines a structure for
// running traceroute from an agent running
// on macOS
type MacTraceroute struct {
	cfg Config
}

// New creates a new instance of MacTraceroute
// based on an input configuration
func New(cfg Config) *MacTraceroute {
	return &MacTraceroute{
		cfg: cfg,
	}
}

// Run executes a traceroute
func (m *MacTraceroute) Run() (NetworkPath, error) {
	// TODO: mac implementation, can we get this no system-probe or root access?
	// To test: we probably can, but maybe not without modifying
	// the library we currently use
	return RunTraceroute(m.cfg)
}
