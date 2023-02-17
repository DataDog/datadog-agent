// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows
// +build windows

package eventmonitor

import (
	"net"
)

func (m *EventMonitor) getListener() (net.Listener, error) {
	var ln net.Listener
	var err error
	if ln, err = net.Listen("tcp", ":3335"); err != nil {
		return nil, err
	}
	return ln, nil
}

func (m *EventMonitor) init() error {
	return nil
}
