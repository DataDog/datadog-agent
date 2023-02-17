// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package eventmonitor

import (
	"net"
	"os"
)

func (m *EventMonitor) getListener() (net.Listener, error) {
	ln, err := net.Listen("unix", m.secconfig.SocketPath)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(m.secconfig.SocketPath, 0700); err != nil {
		return nil, err
	}
	return ln, nil
}

func (m *EventMonitor) init() error {
	// force socket cleanup of previous socket not cleanup

	os.Remove(m.secconfig.SocketPath)
}
