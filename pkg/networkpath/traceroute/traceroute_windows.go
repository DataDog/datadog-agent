// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package traceroute

import "errors"

type WindowsTraceroute struct {
	cfg Config
}

func New(cfg Config) *WindowsTraceroute {
	return &WindowsTraceroute{
		cfg: cfg,
	}
}

func (w *WindowsTraceroute) Run() (NetworkPath, error) {
	// TODO: windows implementation
	return NetworkPath{}, errors.New("Not implemented")
}
