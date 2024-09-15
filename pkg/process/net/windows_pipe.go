// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package net

import (
	"fmt"
	"net"
	"time"

	"github.com/Microsoft/go-winio"
)

const (
	// winio seems to use a fixed buffer size 4096 for its client.
	namedPipeInputBufferSize  = int32(4096)
	namedPipeOutputBufferSize = int32(4096)
)

// WindowsPipeListener for communicating with Probe
type WindowsPipeListener struct {
	conn     net.Listener
	pipePath string
}

// activeSystemProbePipeName is the effective named pipe path for system probe
var activeSystemProbePipeName = SystemProbePipeName

// newPipeListener creates a standardized named pipe server and with hardened ACL
func newPipeListener(namedPipeName string) (net.Listener, error) {
	config := winio.PipeConfig{
		InputBufferSize:  namedPipeInputBufferSize,
		OutputBufferSize: namedPipeOutputBufferSize,
	}

	// TODO: Apply hardened ACL

	return winio.ListenPipe(namedPipeName, &config)
}

// NewSystemProbeListener sets up a named pipe listener for the system probe service.
func NewSystemProbeListener(_ string) (*WindowsPipeListener, error) {
	// socketAddr not used

	namedPipe, err := newPipeListener(activeSystemProbePipeName)
	if err != nil {
		return nil, fmt.Errorf("error named pipe %s : %s", activeSystemProbePipeName, err)
	}

	return &WindowsPipeListener{namedPipe, activeSystemProbePipeName}, nil
}

// GetListener will return underlying Listener's conn
func (wp *WindowsPipeListener) GetListener() net.Listener {
	return wp.conn
}

// Stop closes the WindowsPipeListener connection and stops listening
func (wp *WindowsPipeListener) Stop() {
	wp.conn.Close()
}

// DialSystemProbe connects to the system-probe service endpoint
func DialSystemProbe(_ string, _ string) (net.Conn, error) {
	// Unused netType and path

	var timeout = time.Duration(5 * time.Second)

	namedPipe, err := winio.DialPipe(activeSystemProbePipeName, &timeout)
	if err != nil {
		return nil, fmt.Errorf("error connecting to named pipe %s : %s", activeSystemProbePipeName, err)
	}

	return namedPipe, nil
}
