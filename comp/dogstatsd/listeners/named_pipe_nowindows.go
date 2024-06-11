// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build !windows

package listeners

import (
	"errors"

	"github.com/DataDog/datadog-agent/comp/dogstatsd/packets"
	replay "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/def"
	"github.com/DataDog/datadog-agent/pkg/config"
)

// NamedPipeListener implements the StatsdListener interface for named pipe protocol.
type NamedPipeListener struct{}

// NewNamedPipeListener returns an named pipe Statsd listener
//
//nolint:revive // TODO(AML) Fix revive linter
func NewNamedPipeListener(pipeName string, packetOut chan packets.Packets,
	sharedPacketPoolManager *packets.PoolManager, cfg config.Reader, capture replay.Component, telemetryStore *TelemetryStore) (*NamedPipeListener, error) { //nolint:revive // TODO fix revive unused-parameter

	return nil, errors.New("named pipe is only supported on Windows")
}

// Listen runs the intake loop. Should be called in its own goroutine
func (l *NamedPipeListener) Listen() {
}

// Stop closes the connection and stops listening
func (l *NamedPipeListener) Stop() {
}
