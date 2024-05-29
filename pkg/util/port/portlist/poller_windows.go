// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package portlist

import (
	"errors"
)

// ErrNotImplemented is the "not implemented" error given by `gopsutil` when an
// OS doesn't support and API. Unfortunately it's in an internal package so
// we can't import it so we'll copy it here.
var ErrNotImplemented = errors.New("not implemented yet")

// init initializes the Poller by ensuring it has an underlying
func (p *Poller) init() {
	p.os = newWindowsOSImpl(p.IncludeLocalhost)
}

type WindowsOSImpl struct {
	includeLocalhost bool
}

func newWindowsOSImpl(includeLocalhost bool) osImpl {
	return &WindowsOSImpl{
		includeLocalhost: includeLocalhost,
	}
}

func (im *WindowsOSImpl) AppendListeningPorts(_ []Port) ([]Port, error) {
	return nil, ErrNotImplemented
}

func (*WindowsOSImpl) Close() error { return ErrNotImplemented }
