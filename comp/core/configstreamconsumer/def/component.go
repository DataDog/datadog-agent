// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package configstreamconsumer implements a component that consumes config streams from the core agent.
//
// team: agent-configuration
package configstreamconsumer

import "time"

// Params for the configstreamconsumer component.
type Params struct {
	// ClientName identifies this remote agent (e.g. "system-probe"). Required.
	ClientName string
	// CLIConfigPath is the binary's resolved -config / --cfgpath (file or dir).
	CLIConfigPath string
	// ReadyTimeout caps NewComponent's wait for the first snapshot. Defaults to 60s.
	ReadyTimeout time.Duration
}

// Option mutates Params.
type Option func(*Params)

// WithReadyTimeout overrides the default first-snapshot timeout.
func WithReadyTimeout(d time.Duration) Option {
	return func(p *Params) { p.ReadyTimeout = d }
}

// NewParams returns Params with the required fields set; apply Options for the rest.
func NewParams(clientName, cliConfigPath string, opts ...Option) Params {
	p := Params{
		ClientName:    clientName,
		CLIConfigPath: cliConfigPath,
	}
	for _, opt := range opts {
		opt(&p)
	}
	return p
}

// Component is the config stream consumer. IsActive is true once the initial snapshot
// has been applied to the global config builder.
type Component interface {
	IsActive() bool
}
