// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package traceroute

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
)

// MacTraceroute defines a structure for
// running traceroute from an agent running
// on macOS
type MacTraceroute struct {
	cfg    Config
	runner *Runner
}

// New creates a new instance of MacTraceroute
// based on an input configuration
func New(cfg Config) (*MacTraceroute, error) {
	runner, err := NewRunner()
	if err != nil {
		return nil, err
	}

	return &MacTraceroute{
		cfg:    cfg,
		runner: runner,
	}, nil
}

// Run executes a traceroute
func (m *MacTraceroute) Run(ctx context.Context) (payload.NetworkPath, error) {
	// TODO: mac implementation, can we get this no system-probe or root access?
	// To test: we probably can, but maybe not without modifying
	// the library we currently use
	return m.runner.RunTraceroute(ctx, m.cfg)
}
