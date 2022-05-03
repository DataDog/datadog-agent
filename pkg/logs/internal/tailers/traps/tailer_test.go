// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package traps

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/snmp/traps"
)

// NoOpOIDResolver is a dummy OIDResolver implementation that is unable to get any Trap or Variable metadata.
type NoOpOIDResolver struct{}

// GetTrapMetadata always return an error in this OIDResolver implementation
func (or NoOpOIDResolver) GetTrapMetadata(trapOID string) (traps.TrapMetadata, error) {
	return traps.TrapMetadata{}, fmt.Errorf("trap OID %s is not defined", trapOID)
}

// GetVariableMetadata always return an error in this OIDResolver implementation
func (or NoOpOIDResolver) GetVariableMetadata(trapOID string, varOID string) (traps.VariableMetadata, error) {
	return traps.VariableMetadata{}, fmt.Errorf("trap OID %s is not defined", trapOID)
}

func TestTrapsShouldReceiveMessages(t *testing.T) {
	inputChan := make(traps.PacketsChannel, 1)
	outputChan := make(chan *message.Message)
	tailer, err := NewTailer(&NoOpOIDResolver{}, config.NewLogSource("test", &config.LogsConfig{}), inputChan, outputChan)
	require.NoError(t, err)
	tailer.Start()

	p := &traps.SnmpPacket{
		Content: &gosnmp.SnmpPacket{
			Version:   gosnmp.Version2c,
			Community: "public",
			Variables: traps.NetSNMPExampleHeartbeatNotification.Variables,
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

	formattedPacket := format(t, p)
	assert.Equal(t, message.StatusInfo, msg.GetStatus())
	assert.Equal(t, formattedPacket, msg.Content)
	assert.Equal(t, []string{
		"snmp_version:2",
		"device_namespace:default",
		"snmp_device:127.0.0.1",
	}, msg.Origin.Tags())

	close(inputChan)
	tailer.WaitFlush()
}

func format(t *testing.T, p *traps.SnmpPacket) []byte {
	formatter, err := traps.NewJSONFormatter(NoOpOIDResolver{})
	require.NoError(t, err)
	formattedPacket, err := formatter.FormatPacket(p)
	assert.NoError(t, err)
	return formattedPacket
}
