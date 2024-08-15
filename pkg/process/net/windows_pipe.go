// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package net

import (
	"net"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
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

// Create a standardized named pipe server and with hardened ACL
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

	namedPipe, err := newPipeListener(SystemProbePipeName)
	if err != nil {
		log.Errorf("error creating named pipe %s: %s", SystemProbePipeName, err)
		return nil, err
	}

	return &WindowsPipeListener{namedPipe, SystemProbePipeName}, err
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

	namedPipe, err := winio.DialPipe(SystemProbePipeName, &timeout)
	if err != nil {
		log.Errorf("error connecting to named pipe %s: %s", SystemProbePipeName, err)
	}

	return namedPipe, err
}
