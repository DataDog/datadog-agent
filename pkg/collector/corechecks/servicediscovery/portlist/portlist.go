// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

// Package portlist provides functionality to fetch open ports in the current machine.
package portlist

import (
	"errors"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"path/filepath"
	"runtime"
)

var (
	newOSImpl func(cfg *config) osImpl
)

// Poller scans the systems for listening ports.
type Poller struct {
	// os, if non-nil, is an OS-specific implementation of the portlist getting
	// code. When non-nil, it's responsible for getting the complete list of
	// cached ports complete with the process name. That is, when set,
	// addProcesses is not used.
	// A nil values means we don't have code for getting the list on the current
	// operating system.
	os osImpl
}

type config struct {
	includeLocalhost bool
	procMountPoint   string
}

func newDefaultConfig() *config {
	return &config{
		includeLocalhost: false,
		procMountPoint:   "/proc",
	}
}

// NewPoller initializes a new Poller.
func NewPoller(opts ...Option) (*Poller, error) {
	if newOSImpl == nil {
		return nil, errors.New("poller not implemented on " + runtime.GOOS)
	}
	cfg := newDefaultConfig()
	for _, opt := range opts {
		opt(cfg)
	}
	return &Poller{
		os: newOSImpl(cfg),
	}, nil
}

// osImpl is the OS-specific implementation of getting the open listening ports.
type osImpl interface {
	Init()
	Close() error

	// OpenPorts returns the list of open ports. The Port struct should be
	// populated as completely as possible.
	OpenPorts() ([]Port, error)
}

// OpenPorts returns the list of currently listening ports.
func (p *Poller) OpenPorts() (List, error) {
	p.os.Init()
	defer func() {
		if err := p.os.Close(); err != nil {
			log.Warnf("failed to close port poller: %v", err)
		}
	}()
	return p.os.OpenPorts()
}

// Option is used to configure the Poller.
type Option func(cfg *config)

// WithIncludeLocalhost allows to include/exclude localhost ports (false by default).
func WithIncludeLocalhost(includeLocalhost bool) Option {
	return func(cfg *config) {
		cfg.includeLocalhost = includeLocalhost
	}
}

// WithProcMountPoint allows to change the proc filesystem mount point (this is used mainly in tests).
func WithProcMountPoint(mountPoint string) Option {
	return func(cfg *config) {
		cfg.procMountPoint = filepath.Clean(mountPoint)
	}
}
