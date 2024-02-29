// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package traceroute

import "errors"

// WindowsTraceroute defines a structure for
// running traceroute from an agent running
// on Windows
type WindowsTraceroute struct {
	cfg Config
}

// New creates a new instance of WindowsTraceroute
// based on an input configuration
func New(cfg Config) *WindowsTraceroute {
	return &WindowsTraceroute{
		cfg: cfg,
	}
}

// Run executes a traceroute
func (w *WindowsTraceroute) Run() (NetworkPath, error) {
	// TODO: windows implementation
	return NetworkPath{}, errors.New("Not implemented")
}
