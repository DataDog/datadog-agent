// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package network

import (
	"slices"
	"syscall"
	"testing"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/network/driver"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/types"
	"github.com/DataDog/datadog-agent/pkg/process/util"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var englishOut = `
Protocol tcp Dynamic Port Range
---------------------------------
Start Port      : 49152
Number of Ports : 16384`

//nolint:misspell // misspell only handles english
var frenchOut = `
Plage de ports dynamique du protocole tcp
---------------------------------
Port de démarrage   : 49152
Nombre de ports     : 16384
`

func TestNetshParse(t *testing.T) {
	t.Run("english", func(t *testing.T) {
		low, hi, err := parseNetshOutput(englishOut)
		require.NoError(t, err)
		assert.Equal(t, uint16(49152), low)
		assert.Equal(t, uint16(65535), hi)
	})
	t.Run("french", func(t *testing.T) {
		low, hi, err := parseNetshOutput(frenchOut)
		require.NoError(t, err)
		assert.Equal(t, uint16(49152), low)
		assert.Equal(t, uint16(65535), hi)
	})
}

func TestKeyTuplesFromConn(t *testing.T) {
	sourceAddress := util.AddressFromString("1.2.3.4")
	sourcePort := uint16(1234)
	destinationAddress := util.AddressFromString("5.6.7.8")
	destinationPort := uint16(5678)

	connectionStats := ConnectionStats{ConnectionTuple: ConnectionTuple{
		Source: sourceAddress,
		SPort:  sourcePort,
		Dest:   destinationAddress,
		DPort:  destinationPort,
	}}
	keyTuples := ConnectionKeysFromConnectionStats(connectionStats)

	assert.Len(t, keyTuples, 1, "Expected different number of key tuples")
	assert.True(t, slices.ContainsFunc(keyTuples, func(keyTuple types.ConnectionKey) bool {
		sourceAddressLow, sourceAddressHigh := util.ToLowHigh(sourceAddress)
		destinationAddressLow, destinationAddressHigh := util.ToLowHigh(destinationAddress)
		return (keyTuple.SrcIPLow == sourceAddressLow) && (keyTuple.SrcIPHigh == sourceAddressHigh) &&
			(keyTuple.DstIPLow == destinationAddressLow) && (keyTuple.DstIPHigh == destinationAddressHigh) &&
			(keyTuple.SrcPort == sourcePort) && (keyTuple.DstPort == destinationPort)
	}), "Missing original connection")

}

func TestFlowToConnStatWithIPv6TcpMonotonic(t *testing.T) {
	flow := &driver.PerFlowData{
		FlowHandle:               1,
		FlowCookie:               2,
		ProcessId:                3,
		AddressFamily:            syscall.AF_INET6,
		Protocol:                 syscall.IPPROTO_TCP,
		Flags:                    driver.FlowClosedMask | (driver.FlowDirectionInbound << driver.FlowDirectionBits),
		PacketsOut:               10,
		MonotonicSentBytes:       11,
		TransportBytesOut:        12,
		PacketsIn:                13,
		MonotonicRecvBytes:       14,
		TransportBytesIn:         15,
		Timestamp:                16,
		LocalPort:                17,
		RemotePort:               18,
		ClassificationStatus:     driver.ClassificationClassified,
		ClassifyRequest:          driver.ClassificationRequestTLS,
		ClassifyResponse:         21,
		HttpUpgradeToH2Requested: 22,
		HttpUpgradeToH2Accepted:  23,
		Tls_versions_offered:     0x1 | 0x2 | 0x4,
		Tls_version_chosen:       0x0303,
		Tls_alpn_requested:       26,
		Tls_alpn_chosen:          27,
		Tls_cipher_suite:         0x1301,
	}

	tcpData := &driver.TCPFlowData{
		IRTT:             100,
		SRTT:             101,
		RttVariance:      102,
		RetransmitCount:  103,
		ConnectionStatus: uint32(driver.ConnectionStatusACKRST),
	}

	for i := 0; i < 16; i++ {
		flow.LocalAddress[i] = byte(i + 1)
		flow.RemoteAddress[i] = byte(i + 101)
	}

	flowData := (*driver.TCPFlowData)(unsafe.Pointer(&flow.Protocol_u[0]))
	flowData.IRTT = tcpData.IRTT
	flowData.SRTT = tcpData.SRTT
	flowData.RttVariance = tcpData.RttVariance
	flowData.RetransmitCount = tcpData.RetransmitCount
	flowData.ConnectionStatus = tcpData.ConnectionStatus

	cs := &ConnectionStats{}
	FlowToConnStat(cs, flow, true)

	assert.Equal(t, cs.Source, convertV6Addr(flow.LocalAddress))
	assert.Equal(t, cs.Dest, convertV6Addr(flow.RemoteAddress))
	assert.Equal(t, cs.Monotonic.SentBytes, flow.MonotonicSentBytes)
	assert.Equal(t, cs.Monotonic.RecvBytes, flow.MonotonicRecvBytes)
	assert.Equal(t, cs.Monotonic.SentPackets, flow.PacketsOut)
	assert.Equal(t, cs.Monotonic.RecvPackets, flow.PacketsIn)
	assert.Equal(t, cs.LastUpdateEpoch, driverTimeToUnixTime(flow.Timestamp))
	assert.Equal(t, cs.Pid, uint32(flow.ProcessId))
	assert.Equal(t, cs.SPort, flow.LocalPort)
	assert.Equal(t, cs.DPort, flow.RemotePort)
	assert.Equal(t, cs.Family, AFINET6)
	assert.Equal(t, cs.Type, TCP)
	assert.Equal(t, cs.Direction, INCOMING)
	assert.Equal(t, cs.SPortIsEphemeral, IsPortInEphemeralRange(cs.Family, cs.Type, cs.SPort))
	assert.Equal(t, cs.Cookie, flow.FlowCookie)
	assert.Equal(t, cs.TLSTags.ChosenVersion, flow.Tls_version_chosen)
	assert.Equal(t, cs.TLSTags.OfferedVersions, uint8(flow.Tls_versions_offered))
	assert.Equal(t, cs.TLSTags.CipherSuite, flow.Tls_cipher_suite)

	// TCP related stats.
	assert.Equal(t, cs.Monotonic.Retransmits, uint32(tcpData.RetransmitCount))
	assert.Equal(t, cs.RTT, uint32(tcpData.SRTT))
	assert.Equal(t, cs.RTTVar, uint32(tcpData.RttVariance))
	assert.Equal(t, cs.TCPFailures[111], uint32(1))
	assert.Equal(t, cs.Monotonic.TCPEstablished, uint16(0))
	assert.Equal(t, cs.Monotonic.TCPClosed, uint16(1))
	assert.Equal(t, cs.ProtocolStack.Encryption, protocols.TLS)
}

// TestFlowToConnStatTCPFailureAlwaysSetsClosed verifies that when the Windows
// driver reports a TCP failure via ConnectionStatus (RST, timeout, refused),
// TCPClosed is always set to 1 — even if the driver's FlowClosedMask flag is
// not set in flow.Flags.
func TestFlowToConnStatTCPFailureAlwaysSetsClosed(t *testing.T) {
	tests := []struct {
		name             string
		connectionStatus driver.ConnectionStatus
		expectedErrno    uint16
	}{
		{"RecvRst", driver.ConnectionStatusRecvRst, 104},
		{"SentRst", driver.ConnectionStatusSentRst, 104},
		{"ACKRST", driver.ConnectionStatusACKRST, 111},
		{"Timeout", driver.ConnectionStatusTimeout, 110},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Deliberately omit FlowClosedMask from Flags to simulate the
			// driver not marking the flow as closed for failure cases.
			flow := &driver.PerFlowData{
				AddressFamily: syscall.AF_INET,
				Protocol:      syscall.IPPROTO_TCP,
				Flags:         driver.FlowDirectionOutbound << driver.FlowDirectionBits, // NO FlowClosedMask
			}

			flowData := (*driver.TCPFlowData)(unsafe.Pointer(&flow.Protocol_u[0]))
			flowData.ConnectionStatus = uint32(tc.connectionStatus)

			cs := &ConnectionStats{}
			FlowToConnStat(cs, flow, false)

			assert.Equal(t, uint32(1), cs.TCPFailures[tc.expectedErrno],
				"expected failure errno %d to be set", tc.expectedErrno)
			assert.Equal(t, uint16(1), cs.Monotonic.TCPClosed,
				"expected TCPClosed to be set when a TCP failure is reported")
		})
	}
}

func TestFlowToConnStatWithIPv4UdpNoMonotonicc(t *testing.T) {
	flow := &driver.PerFlowData{
		FlowHandle:               1,
		FlowCookie:               2,
		ProcessId:                3,
		AddressFamily:            syscall.AF_INET,
		Protocol:                 syscall.IPPROTO_UDP,
		Flags:                    (driver.FlowDirectionOutbound << driver.FlowDirectionBits),
		PacketsOut:               10,
		MonotonicSentBytes:       11,
		TransportBytesOut:        12,
		PacketsIn:                13,
		MonotonicRecvBytes:       14,
		TransportBytesIn:         15,
		Timestamp:                16,
		LocalPort:                17,
		RemotePort:               18,
		ClassificationStatus:     driver.ClassificationClassified,
		ClassifyRequest:          driver.ClassificationRequestUnclassified,
		ClassifyResponse:         21,
		HttpUpgradeToH2Requested: 22,
		HttpUpgradeToH2Accepted:  23,
		Tls_versions_offered:     0x1 | 0x2 | 0x4,
		Tls_version_chosen:       0x0301,
		Tls_alpn_requested:       26,
		Tls_alpn_chosen:          27,
		Tls_cipher_suite:         0xCCA8,
	}

	for i := 0; i < 16; i++ {
		flow.LocalAddress[i] = byte(i + 1)
		flow.RemoteAddress[i] = byte(i + 101)
	}

	cs := &ConnectionStats{}
	FlowToConnStat(cs, flow, false)

	assert.Equal(t, cs.Source, convertV4Addr(flow.LocalAddress))
	assert.Equal(t, cs.Dest, convertV4Addr(flow.RemoteAddress))
	assert.Equal(t, cs.Monotonic.SentBytes, flow.TransportBytesOut)
	assert.Equal(t, cs.Monotonic.RecvBytes, flow.TransportBytesIn)
	assert.Equal(t, cs.Monotonic.SentPackets, flow.PacketsOut)
	assert.Equal(t, cs.Monotonic.RecvPackets, flow.PacketsIn)
	assert.Equal(t, cs.LastUpdateEpoch, driverTimeToUnixTime(flow.Timestamp))
	assert.Equal(t, cs.Pid, uint32(flow.ProcessId))
	assert.Equal(t, cs.SPort, flow.LocalPort)
	assert.Equal(t, cs.DPort, flow.RemotePort)
	assert.Equal(t, cs.Family, AFINET)
	assert.Equal(t, cs.Type, UDP)
	assert.Equal(t, cs.Direction, OUTGOING)
	assert.Equal(t, cs.SPortIsEphemeral, IsPortInEphemeralRange(cs.Family, cs.Type, cs.SPort))
	assert.Equal(t, cs.Cookie, flow.FlowCookie)
	assert.Equal(t, cs.TLSTags.ChosenVersion, flow.Tls_version_chosen)
	assert.Equal(t, cs.TLSTags.OfferedVersions, uint8(flow.Tls_versions_offered))
	assert.Equal(t, cs.TLSTags.CipherSuite, flow.Tls_cipher_suite)
}
