// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.
// +build !windows

package listeners

import (
	"errors"
)

// NamedPipeListener implements the StatsdListener interface for named pipe protocol.
type NamedPipeListener struct{}

// IsNamedPipeEndpoint detects if the endpoint has the named pipe prefix
func IsNamedPipeEndpoint(endpoint string) bool { return false }

// NewNamedPipeListener returns an named pipe Statsd listener
func NewNamedPipeListener(pipeName string, packetOut chan Packets, sharedPacketPool *PacketPool) (*NamedPipeListener, error) {
	return nil, errors.New("named pipe is only supported on Windows")
}

// Listen runs the intake loop. Should be called in its own goroutine
func (l *NamedPipeListener) Listen() {
}

// Stop closes the connection and stops listening
func (l *NamedPipeListener) Stop() {
}
