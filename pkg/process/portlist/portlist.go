// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package portlist

import (
	"errors"
	"runtime"
)

var (
	newOSImpl func(includeLocalhost bool) osImpl
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

// NewPoller initializes a new Poller.
func NewPoller(includeLocalhost bool) (*Poller, error) {
	if newOSImpl == nil {
		return nil, errors.New("poller not implemented on " + runtime.GOOS)
	}
	return &Poller{
		os: newOSImpl(includeLocalhost),
	}, nil
}

// osImpl is the OS-specific implementation of getting the open listening ports.
type osImpl interface {
	Close() error

	// ListeningPorts returns the list of listening ports. The Port struct should be
	// populated as completely as possible.
	ListeningPorts() ([]Port, error)
}

// Close closes the Poller.
func (p *Poller) Close() error {
	if p.os == nil {
		return nil
	}
	return p.os.Close()
}

// ListeningPorts returns the list of currently listening ports.
func (p *Poller) ListeningPorts() (List, error) {
	return p.os.ListeningPorts()
}
