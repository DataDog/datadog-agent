// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package traps

import (
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/snmp/traps"
)

func TestTrapsShouldReceiveMessages(t *testing.T) {
	inputChan := make(traps.PacketsChannel, 1)
	outputChan := make(chan *message.Message)
	tailer := NewTailer(config.NewLogSource("test", &config.LogsConfig{}), inputChan, outputChan)
	tailer.Start()

	p := &traps.SnmpPacket{
		Content: &gosnmp.SnmpPacket{
			Version:   gosnmp.Version2c,
			Community: "public",
			Variables: traps.NetSNMPExampleHeartbeatNotificationVariables,
		},
		Addr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1620},
	}

	inputChan <- p

	var msg *message.Message
	select {
	case msg = <-outputChan:
		break
	case <-time.After(1 * time.Second):
		t.Error("Message not received")
		return
	}

	assert.Equal(t, message.StatusInfo, msg.GetStatus())
	assert.Equal(t, format(t, p), msg.Content)
	assert.Equal(t, traps.GetTags(p), msg.Origin.Tags())

	close(inputChan)
	tailer.WaitFlush()
}

func format(t *testing.T, p *traps.SnmpPacket) []byte {
	data, err := traps.FormatPacketToJSON(p)
	assert.NoError(t, err)
	content, err := json.Marshal(data)
	assert.NoError(t, err)
	return content
}
