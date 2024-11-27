// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test && windows

package tcp

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/ipv4"
	"golang.org/x/sys/windows"
)

type (
	mockRawConn struct {
		readTimeoutCount int
		readTimeout      time.Duration

		payload []byte
		cm      *ipv4.ControlMessage
	}
)

func Test_handlePackets(t *testing.T) {
	_, tcpBytes := createMockFullTCPPacket(createMockIPv4Header(dstIP, srcIP, 6), createMockTCPLayer(443, 12345, 28394, 28395, true, true, true))

	tt := []struct {
		description string
		// input
		ctxTimeout time.Duration
		conn       rawConnWrapper
		localIP    net.IP
		localPort  uint16
		remoteIP   net.IP
		remotePort uint16
		seqNum     uint32
		// output
		expectedIP       net.IP
		expectedPort     uint16
		expectedTypeCode layers.ICMPv4TypeCode
		errMsg           string
	}{
		{
			description: "canceled context returns canceledErr",
			ctxTimeout:  300 * time.Millisecond,
			conn: &mockRawConn{
				readTimeoutCount: 100,
				readFromErr:      errors.New("bad test error"),
			},
			errMsg: "canceled",
		},
		{
			description: "non-timeout read error returns an error",
			ctxTimeout:  1 * time.Second,
			conn: &mockRawConn{
				readFromErr: errors.New("test read error"),
			},
			errMsg: "test read error",
		},
		// {
		// 	description: "failed ICMP parsing eventuallly returns cancel timeout",
		// 	ctxTimeout:  500 * time.Millisecond,
		// 	conn: &mockRawConn{
		// 		payload: nil,
		// 	},
		// 	errMsg:   "canceled",
		// },
		// {
		// 	description: "failed TCP parsing eventuallly returns cancel timeout",
		// 	ctxTimeout:  500 * time.Millisecond,
		// 	conn: &mockRawConn{
		// 		header:  &ipv4.Header{},
		// 		payload: nil,
		// 	},
		// 	listener: "tcp",
		// 	errMsg:   "canceled",
		// },
		{
			description: "successful ICMP parsing returns IP, port, and type code",
			ctxTimeout:  500 * time.Millisecond,
			conn: &mockRawConn{
				payload: createMockICMPPacket(createMockIPv4Header(srcIP, dstIP, 1), createMockICMPLayer(layers.ICMPv4CodeTTLExceeded), createMockIPv4Layer(innerSrcIP, innerDstIP, layers.IPProtocolTCP), createMockTCPLayer(12345, 443, 28394, 12737, true, true, true), false),
			},
			localIP:          innerSrcIP,
			localPort:        12345,
			remoteIP:         innerDstIP,
			remotePort:       443,
			seqNum:           28394,
			expectedIP:       srcIP,
			expectedPort:     0,
			expectedTypeCode: layers.ICMPv4CodeTTLExceeded,
		},
		{
			description: "successful TCP parsing returns IP, port, and type code",
			ctxTimeout:  500 * time.Millisecond,
			conn: &mockRawConn{
				payload: tcpBytes,
			},
			localIP:          srcIP,
			localPort:        12345,
			remoteIP:         dstIP,
			remotePort:       443,
			seqNum:           28394,
			expectedIP:       dstIP,
			expectedPort:     443,
			expectedTypeCode: 0,
		},
	}

	for _, test := range tt {
		t.Run(test.description, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), test.ctxTimeout)
			defer cancel()
			recvFrom = test.conn.RecvFrom
			w := &winrawsocket{}
			actualIP, actualPort, actualTypeCode, _, err := handlePackets(ctx, test.conn, test.listener, test.localIP, test.localPort, test.remoteIP, test.remotePort, test.seqNum)
			if test.errMsg != "" {
				require.Error(t, err)
				assert.True(t, strings.Contains(err.Error(), test.errMsg))
				return
			}
			require.NoError(t, err)
			assert.Truef(t, test.expectedIP.Equal(actualIP), "mismatch source IPs: expected %s, got %s", test.expectedIP.String(), actualIP.String())
			assert.Equal(t, test.expectedPort, actualPort)
			assert.Equal(t, test.expectedTypeCode, actualTypeCode)
		})
	}
}

func (m *mockRawConn) RecvFrom(_ windows.Handle, buf []byte, _ int) (int, windows.Sockaddr, error) {
	if m.readTimeoutCount > 0 {
		m.readTimeoutCount--
		time.Sleep(time.Until(m.readDeadline))
		return 0, nil, windows.WSAETIMEDOUT
	}
	if m.readFromErr != nil {
		return 0, nil, m.readFromErr
	}
	copy(buf, m.payload)

	return len(m.payload), nil, nil
}

func createMockFullTCPPacket(ipHeader *ipv4.Header, tcpLayer *layers.TCP) (*layers.TCP, []byte) {
	ipLayer := &layers.IPv4{
		Version:  4,
		SrcIP:    ipHeader.Src,
		DstIP:    ipHeader.Dst,
		Protocol: layers.IPProtocol(ipHeader.Protocol),
		TTL:      64,
		Length:   8,
	}
	tcpLayer.SetNetworkLayerForChecksum(ipLayer)
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	gopacket.SerializeLayers(buf, opts,
		ipLayer,
		tcpLayer,
	)

	pkt := gopacket.NewPacket(buf.Bytes(), layers.LayerTypeTCP, gopacket.Default)

	// return encoded TCP layer here
	return pkt.Layer(layers.LayerTypeTCP).(*layers.TCP), buf.Bytes()
}
