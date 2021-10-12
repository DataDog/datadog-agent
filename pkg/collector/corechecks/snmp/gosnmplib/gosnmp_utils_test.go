package gosnmplib

import (
	"bufio"
	"bytes"
	"testing"

	"github.com/cihub/seelog"
	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func TestPacketAsStringIfLoglevel(t *testing.T) {
	tests := []struct {
		name        string
		packet      *gosnmp.SnmpPacket
		curLogLevel string
		logLevel    seelog.LogLevel
		expectedStr string
	}{
		{
			name: "same level as current log level",
			packet: &gosnmp.SnmpPacket{
				Variables: []gosnmp.SnmpPDU{
					{
						Name:  "1.3.6.1.2.1.1.2.0",
						Type:  gosnmp.ObjectIdentifier,
						Value: "1.3.6.1.4.1.3375.2.1.3.4.1",
					},
					{
						Name:  "1.3.6.1.2.1.1.3.0",
						Type:  gosnmp.Counter32,
						Value: 10,
					},
				},
			},
			curLogLevel: "debug",
			logLevel:    seelog.DebugLvl,
			expectedStr: "error=NoError(code:0, idx:0), values=[{\"oid\":\"1.3.6.1.2.1.1.2.0\",\"type\":\"ObjectIdentifier\",\"value\":\"1.3.6.1.4.1.3375.2.1.3.4.1\"},{\"oid\":\"1.3.6.1.2.1.1.3.0\",\"type\":\"Counter32\",\"value\":\"10\"}]",
		},
		{
			name: "current log is higher",
			packet: &gosnmp.SnmpPacket{
				Variables: []gosnmp.SnmpPDU{
					{
						Name:  "1.3.6.1.2.1.1.2.0",
						Type:  gosnmp.ObjectIdentifier,
						Value: "1.3.6.1.4.1.3375.2.1.3.4.1",
					},
				},
			},
			curLogLevel: "trace",
			logLevel:    seelog.DebugLvl,
			expectedStr: "error=NoError(code:0, idx:0), values=[{\"oid\":\"1.3.6.1.2.1.1.2.0\",\"type\":\"ObjectIdentifier\",\"value\":\"1.3.6.1.4.1.3375.2.1.3.4.1\"}]",
		},
		{
			name: "invalid ipaddr",
			packet: &gosnmp.SnmpPacket{
				Variables: []gosnmp.SnmpPDU{
					{
						Name:  "1.3.6.1.2.1.1.2.0",
						Type:  gosnmp.IPAddress,
						Value: 10,
					},
				},
			},
			curLogLevel: "debug",
			logLevel:    seelog.DebugLvl,
			expectedStr: "error=NoError(code:0, idx:0), values=[{\"oid\":\"1.3.6.1.2.1.1.2.0\",\"type\":\"IPAddress\",\"value\":\"10\",\"parse_err\":\"`oid 1.3.6.1.2.1.1.2.0: IPAddress should be string type but got int type: gosnmp.SnmpPDU{Name:\\\"1.3.6.1.2.1.1.2.0\\\", Type:0x40, Value:10}`\"}]",
		},
		{
			name: "not enough loglevel",
			packet: &gosnmp.SnmpPacket{
				Variables: []gosnmp.SnmpPDU{
					{
						Name:  "1.3.6.1.2.1.1.2.0",
						Type:  gosnmp.IPAddress,
						Value: 10,
					},
				},
			},
			curLogLevel: "debug",
			logLevel:    seelog.TraceLvl,
			expectedStr: "",
		},
		{
			name:        "nil packet loglevel",
			curLogLevel: "debug",
			logLevel:    seelog.DebugLvl,
			expectedStr: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b bytes.Buffer
			w := bufio.NewWriter(&b)
			l, err := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.TraceLvl, "[%LEVEL] %FuncShort: %Msg")
			require.NoError(t, err)
			log.SetupLogger(l, tt.curLogLevel)

			str := PacketAsStringIfLoglevel(tt.packet, tt.logLevel)
			assert.Equal(t, tt.expectedStr, str)
		})
	}
}
