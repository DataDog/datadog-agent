// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package traps

import (
	"encoding/json"
	"net"
	"testing"

	"github.com/soniah/gosnmp"

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
			Variables: []gosnmp.SnmpPDU{
				// sysUpTime
				{Name: "1.3.6.1.2.1.1.3", Type: gosnmp.TimeTicks, Value: uint32(1000)},
				// snmpTrapOID
				{Name: "1.3.6.1.6.3.1.1.4.1", Type: gosnmp.OctetString, Value: "1.3.6.1.4.1.8072.2.3.0.1"},
				// heartBeatRate
				{Name: "1.3.6.1.4.1.8072.2.3.2.1", Type: gosnmp.Integer, Value: 1024},
				// heartBeatName
				{Name: "1.3.6.1.4.1.8072.2.3.2.2", Type: gosnmp.OctetString, Value: "test"},
			},
		},
		Addr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1620},
	}

	inputChan <- p
	msg := <-outputChan

	assert.Equal(t, message.StatusInfo, msg.GetStatus())
	assert.Equal(t, format(t, p), msg.Content)
	assert.Equal(t, traps.GetTags(p), msg.Origin.Tags())

	close(inputChan)
	tailer.Stop()
}

func format(t *testing.T, p *traps.SnmpPacket) []byte {
	data, err := traps.FormatJSON(p)
	assert.NoError(t, err)
	content, err := json.Marshal(data)
	assert.NoError(t, err)
	return content
}
