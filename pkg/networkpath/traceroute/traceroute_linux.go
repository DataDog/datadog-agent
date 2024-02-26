// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package traceroute

import "errors"

type LinuxTraceroute struct {
	cfg Config
}

func New(cfg Config) *LinuxTraceroute {
	return &WindowsTraceroute{
		cfg: cfg,
	}
}

func (l *LinuxTraceroute) Run() (NetworkPath, error) {
	// TODO: linux implementation
	return NetworkPath{}, errors.New("Not implemented")
}
