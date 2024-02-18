// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package net

import (
	"net"

	"github.com/Microsoft/go-winio"
)

// WindowsPipeListener for communicating with Probe
type WindowsPipeListener struct {
	conn     net.Listener
	pipePath string
}

const maxGRPCServerMessage = 100 * 1024 * 1024

// NewListener sets up a TCP listener for now, will eventually be a named pipe
func NewListener(_ string) (*WindowsPipeListener, error) {
	pc := winio.PipeConfig{
		InputBufferSize:  int32(maxGRPCServerMessage),
		OutputBufferSize: int32(maxGRPCServerMessage),
	}
	l, err := winio.ListenPipe(`\\.\pipe\datadog-system-probe`, &pc)
	return &WindowsPipeListener{l, "path"}, err
}

// GetListener will return underlying Listener's conn
func (wp *WindowsPipeListener) GetListener() net.Listener {
	return wp.conn
}

// Stop closes the WindowsPipeListener connection and stops listening
func (wp *WindowsPipeListener) Stop() {
	wp.conn.Close()
}
