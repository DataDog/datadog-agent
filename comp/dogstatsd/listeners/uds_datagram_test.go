// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// UDS won't work in windows

package listeners

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/packets"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/pidmap"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

func udsDatagramListenerFactory(packetOut chan packets.Packets, manager *packets.PoolManager, cfg config.Component, pidMap pidmap.Component) (StatsdListener, error) {
	return NewUDSDatagramListener(packetOut, manager, nil, cfg, nil, optional.NewNoneOption[workloadmeta.Component](), pidMap)
}

func TestNewUDSDatagramListener(t *testing.T) {
	testNewUDSListener(t, udsDatagramListenerFactory, "unixgram")
}

func TestStartStopUDSDatagramListener(t *testing.T) {
	testStartStopUDSListener(t, udsDatagramListenerFactory, "unixgram")
}

func TestUDSDatagramReceive(t *testing.T) {
	socketPath := testSocketPath(t)

	mockConfig := map[string]interface{}{}
	mockConfig[socketPathConfKey("unixgram")] = socketPath
	mockConfig["dogstatsd_origin_detection"] = false

	var contents0 = []byte("daemon:666|g|#sometag1:somevalue1,sometag2:somevalue2")
	var contents1 = []byte("daemon:999|g|#sometag1:somevalue1")

	packetsChannel := make(chan packets.Packets)

	deps := fulfillDepsWithConfig(t, mockConfig)
	s, err := udsDatagramListenerFactory(packetsChannel, newPacketPoolManagerUDS(deps.Config), deps.Config, deps.PidMap)
	assert.Nil(t, err)
	assert.NotNil(t, s)

	s.Listen()
	defer s.Stop()
	conn, err := net.Dial("unixgram", socketPath)
	assert.Nil(t, err)
	defer conn.Close()

	conn.Write([]byte{})
	conn.Write(contents0)
	conn.Write(contents1)

	select {
	case pkts := <-packetsChannel:
		assert.Equal(t, 3, len(pkts))

		packet := pkts[0]
		assert.NotNil(t, packet)
		assert.Equal(t, packet.Contents, []byte{})
		assert.Equal(t, packet.Origin, "")
		assert.Equal(t, packet.Source, packets.UDS)

		packet = pkts[1]
		assert.NotNil(t, packet)
		assert.Equal(t, packet.Contents, contents0)
		assert.Equal(t, packet.Origin, "")
		assert.Equal(t, packet.Source, packets.UDS)

		packet = pkts[2]
		assert.NotNil(t, packet)
		assert.Equal(t, packet.Contents, contents1)
		assert.Equal(t, packet.Origin, "")
		assert.Equal(t, packet.Source, packets.UDS)

	case <-time.After(2 * time.Second):
		assert.FailNow(t, "Timeout on receive channel")
	}

}
