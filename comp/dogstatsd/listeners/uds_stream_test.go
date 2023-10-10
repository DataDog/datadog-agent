// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// UDS won't work in windows

package listeners

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/packets"
)

func udsStreamListenerFactory(packetOut chan packets.Packets, manager *packets.PoolManager, cfg config.Component) (StatsdListener, error) {
	return NewUDSStreamListener(packetOut, manager, nil, cfg, nil)
}

func TestNewUDSStreamListener(t *testing.T) {
	testNewUDSListener(t, udsStreamListenerFactory, "unix")
}

func TestStartStopUDSStreamListener(t *testing.T) {
	testStartStopUDSListener(t, udsStreamListenerFactory, "unix")
}

func TestUDSStreamReceive(t *testing.T) {
	testUDSReceive(t, udsStreamListenerFactory, "unix")
}
