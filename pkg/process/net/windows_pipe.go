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

	"github.com/DataDog/datadog-agent/pkg/util/log"
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

// systemProbePipeName is the effective named pipe path for system probe
var systemProbePipeName = SystemProbePipeName

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
func NewSystemProbeListener(_ string) (*WindowsPipeListener, error) {
	// socketAddr not used

	namedPipe, err := newPipeListener(systemProbePipeName)
	if err != nil {
		return nil, fmt.Errorf("error named pipe %s : %s", systemProbePipeName, err)
	}

	return &WindowsPipeListener{namedPipe, systemProbePipeName}, nil
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
func DialSystemProbe() (net.Conn, error) {
	// Go clients do not immediately close (named pipe) connections when done,
	// they keep connections idle for a while.  Make sure the idle time
	// is not too high and the timeout is generous enough for pending connections.
	var timeout = time.Duration(30 * time.Second)

	namedPipe, err := winio.DialPipe(systemProbePipeName, &timeout)
	if err != nil {
		// This important error may not get reported upstream, making connection failures
		// very difficult to diagnose. Explicitly log the error here too for diagnostics.
		var namedPipeErr = fmt.Errorf("error connecting to named pipe %s : %s", systemProbePipeName, err)
		log.Errorf("%s", namedPipeErr.Error())
		return nil, namedPipeErr
	}

	return namedPipe, nil
}
