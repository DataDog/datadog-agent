// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package traceroute

type MacTraceroute struct {
	cfg Config
}

func New(cfg Config) *MacTraceroute {
	return &MacTraceroute{
		cfg: cfg,
	}
}

func (m *MacTraceroute) Run() (NetworkPath, error) {
	// TODO: mac implementation, can we get this no system-probe or root access?
	// To test: we probably can, but maybe without modifying
	// the dublin library
	return RunTraceroute(m.cfg)
}
