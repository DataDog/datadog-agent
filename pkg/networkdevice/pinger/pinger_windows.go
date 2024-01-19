//go:build windows

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pinger

// WindowsPinger is a structure for pinging
// hosts in Windows
type WindowsPinger struct {
	cfg Config
}

// New creates a WindowsPinger using the passed in
// config
func New(cfg Config) (Pinger, error) {
	if !cfg.UseRawSocket {
		return nil, ErrUDPSocketUnsupported
	}
	return &WindowsPinger{
		cfg: cfg,
	}, nil
}

// Ping takes a host sends ICMP ping
func (p *WindowsPinger) Ping(host string) (*Result, error) {
	// We set privileged to true, per pro-bing's docs
	// but it's not actually privileged
	// https://github.com/prometheus-community/pro-bing#windows
	return RunPing(&p.cfg, host)
}
