// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package net

import (
	"fmt"
	"net"

	"github.com/Microsoft/go-winio"
)

const (
	// Buffer sizes for the system probe named pipe.
	// The sizes are advisory, Windows can adjust them, but should be small enough to preserve
	// the nonpaged pool.
	namedPipeInputBufferSize  = int32(4096)
	namedPipeOutputBufferSize = int32(4096)

	// DACL for the system probe named pipe.
	// SE_DACL_PROTECTED (P), SE_DACL_AUTO_INHERITED (AI)
	// Allow Everyone (WD)
	// nolint:revive // TODO: Hardened DACL and ensure the datadogagent run-as user is allowed.
	namedPipeSecurityDescriptor = "D:PAI(A;;FA;;;WD)"
)

// WindowsPipeListener for communicating with Probe
type WindowsPipeListener struct {
	conn     net.Listener
	pipePath string
}

// systemProbePipSecurityDescriptor has the effective DACL for the system probe named pipe.
var systemProbePipSecurityDescriptor = namedPipeSecurityDescriptor

// newPipeListener creates a standardized named pipe server and with hardened ACL
func newPipeListener(namedPipeName string) (net.Listener, error) {
	// The DACL must allow the run-as user of datadogagent.
	config := winio.PipeConfig{
		SecurityDescriptor: systemProbePipSecurityDescriptor,
		InputBufferSize:    namedPipeInputBufferSize,
		OutputBufferSize:   namedPipeOutputBufferSize,
	}

	// winio specifies virtually unlimited number of named pipe instances but is limited by
	// the nonpaged pool.
	return winio.ListenPipe(namedPipeName, &config)
}

// NewSystemProbeListener sets up a named pipe listener for the system probe service.
func NewSystemProbeListener(namedPipePath string) (*WindowsPipeListener, error) {
	namedPipe, err := newPipeListener(namedPipePath)
	if err != nil {
		return nil, fmt.Errorf("error named pipe %s : %s", namedPipePath, err)
	}

	return &WindowsPipeListener{namedPipe, namedPipePath}, nil
}

// GetListener will return underlying Listener's conn
func (wp *WindowsPipeListener) GetListener() net.Listener {
	return wp.conn
}

// Stop closes the WindowsPipeListener connection and stops listening
func (wp *WindowsPipeListener) Stop() {
	wp.conn.Close()
}
