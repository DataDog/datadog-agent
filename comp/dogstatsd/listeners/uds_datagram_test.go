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

func udsDatagramListenerFactory(packetOut chan packets.Packets, manager *packets.PoolManager, cfg config.Component) (StatsdListener, error) {
	return NewUDSDatagramListener(packetOut, manager, nil, cfg, nil)
}

func TestNewUDSDatagramListener(t *testing.T) {
	testNewUDSListener(t, udsDatagramListenerFactory, "unixgram")
}

func TestStartStopUDSDatagramListener(t *testing.T) {
	testStartStopUDSListener(t, udsDatagramListenerFactory, "unixgram")
}

func TestUDSDatagramReceive(t *testing.T) {
	testUDSReceive(t, udsDatagramListenerFactory, "unixgram")
}
