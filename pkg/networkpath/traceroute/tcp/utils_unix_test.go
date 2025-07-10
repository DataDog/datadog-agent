// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test && unix

package tcp

import (
	"context"
	"errors"
	"net"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/gopacket/layers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/ipv4"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/testutils"
)

var (
	innerSrcIP = net.ParseIP("10.0.0.1")
	innerDstIP = net.ParseIP("192.168.1.1")
)

type (
	mockRawConn struct {
		setReadDeadlineErr error
		readDeadline       time.Time

		readTimeoutCount int
		readFromErr      error
		header           *ipv4.Header
		payload          []byte
		cm               *ipv4.ControlMessage

		numReaders atomic.Int32

		writeDelay time.Duration
		writeToErr error
	}

	mockTimeoutErr string
)

func Test_handlePackets(t *testing.T) {
	_, tcpBytes := testutils.CreateMockTCPPacket(testutils.CreateMockIPv4Header(dstIP, srcIP, 6), testutils.CreateMockTCPLayer(443, 12345, 28394, 28395, true, true, true), false)

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
		packetID   uint16
		// output
		expectedIP   net.IP
		expectedPort uint16
		expectedType uint8
		expectedCode uint8
		errMsg       string
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
			description: "set timeout error returns an error",
			ctxTimeout:  300 * time.Millisecond,
			conn: &mockRawConn{
				setReadDeadlineErr: errors.New("good test error"),
				readTimeoutCount:   100,
				readFromErr:        errors.New("bad error"),
			},
			errMsg: "good test error",
		},
		{
			description: "non-timeout read error returns an error",
			ctxTimeout:  1 * time.Second,
			conn: &mockRawConn{
				readFromErr: errors.New("test read error"),
			},
			errMsg: "test read error",
		},
		{
			description: "invalid protocol packet eventually returns cancel timeout",
			ctxTimeout:  500 * time.Millisecond,
			conn: &mockRawConn{
				header: &ipv4.Header{
					Protocol: unix.IPPROTO_UDP,
				},
				payload: nil,
			},
			errMsg: "canceled",
		},
		{
			description: "failed ICMP parsing eventuallly returns cancel timeout",
			ctxTimeout:  500 * time.Millisecond,
			conn: &mockRawConn{
				header: &ipv4.Header{
					Protocol: unix.IPPROTO_ICMP,
				},
				payload: nil,
			},
			errMsg: "canceled",
		},
		{
			description: "failed TCP parsing eventuallly returns cancel timeout",
			ctxTimeout:  500 * time.Millisecond,
			conn: &mockRawConn{
				header: &ipv4.Header{
					Protocol: unix.IPPROTO_TCP,
				},
				payload: nil,
			},
			errMsg: "canceled",
		},
		{
			description: "successful ICMP parsing returns IP, port, and type code",
			ctxTimeout:  500 * time.Millisecond,
			conn: &mockRawConn{
				header:  testutils.CreateMockIPv4Header(srcIP, dstIP, 1),
				payload: testutils.CreateMockICMPWithTCPPacket(nil, testutils.CreateMockICMPLayer(layers.ICMPv4TypeTimeExceeded, layers.ICMPv4CodeTTLExceeded), testutils.CreateMockIPv4Layer(4321, innerSrcIP, innerDstIP, layers.IPProtocolTCP), testutils.CreateMockTCPLayer(12345, 443, 28394, 12737, true, true, true), false),
			},
			localIP:      innerSrcIP,
			localPort:    12345,
			remoteIP:     innerDstIP,
			remotePort:   443,
			seqNum:       28394,
			packetID:     4321,
			expectedIP:   srcIP,
			expectedPort: 0,
			expectedType: layers.ICMPv4TypeTimeExceeded,
			expectedCode: layers.ICMPv4CodeTTLExceeded,
		},
		{
			description: "successful TCP parsing returns IP, port, and type code",
			ctxTimeout:  500 * time.Millisecond,
			conn: &mockRawConn{
				header:  testutils.CreateMockIPv4Header(dstIP, srcIP, 6),
				payload: tcpBytes,
			},
			localIP:      srcIP,
			localPort:    12345,
			remoteIP:     dstIP,
			remotePort:   443,
			seqNum:       28394,
			expectedIP:   dstIP,
			expectedPort: 443,
			expectedType: 0,
			expectedCode: 0,
		},
	}

	for _, test := range tt {
		t.Run(test.description, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), test.ctxTimeout)
			defer cancel()
			actual := handlePackets(ctx, test.conn, test.localIP, test.localPort, test.remoteIP, test.remotePort, test.seqNum, test.packetID)
			if test.errMsg != "" {
				require.Error(t, actual.Err)
				assert.True(t, strings.Contains(actual.Err.Error(), test.errMsg), "error mismatch: excpected %q, got %q", test.errMsg, actual.Err.Error())
				return
			}
			require.NoError(t, actual.Err)
			assert.Truef(t, test.expectedIP.Equal(actual.IP), "mismatch source IPs: expected %s, got %s", test.expectedIP.String(), actual.IP.String())
			assert.Equal(t, test.expectedPort, actual.Port)
			assert.Equal(t, test.expectedType, actual.Type)
			assert.Equal(t, test.expectedCode, actual.Code)
		})
	}
}

func TestListenPackets(t *testing.T) {
	_, tcpBytes := testutils.CreateMockTCPPacket(testutils.CreateMockIPv4Header(dstIP, srcIP, 6), testutils.CreateMockTCPLayer(443, 12345, 28394, 28395, true, true, true), false)

	tt := []struct {
		description string
		// input
		icmpConn   *mockRawConn
		tcpConn    *mockRawConn
		timeout    time.Duration
		localIP    net.IP
		localPort  uint16
		remoteIP   net.IP
		remotePort uint16
		seqNum     uint32
		packetID   uint16
		// output
		expectedResponse packetResponse
		errMsg           string
	}{
		{
			description: "both connections timeout",
			icmpConn: &mockRawConn{
				readTimeoutCount: 100,
				readDeadline:     time.Now().Add(500 * time.Millisecond),
				readFromErr:      errors.New("icmp timeout error"),
			},
			tcpConn: &mockRawConn{
				readTimeoutCount: 100,
				readDeadline:     time.Now().Add(500 * time.Millisecond),
				readFromErr:      errors.New("tcp timeout error"),
			},
			timeout: 300 * time.Millisecond,
		},
		{
			description: "both connections error before timeout",
			icmpConn: &mockRawConn{
				readFromErr: errors.New("read error"),
			},
			tcpConn: &mockRawConn{
				readFromErr: errors.New("read error"),
			},
			timeout: 300 * time.Millisecond,
			errMsg:  "read error; read error", // both errors should be returned in any order
		},
		{
			description: "icmp connection returns error",
			icmpConn: &mockRawConn{
				readFromErr: errors.New("icmp read error"),
			},
			tcpConn: &mockRawConn{
				readTimeoutCount: 100,
				readDeadline:     time.Now().Add(500 * time.Millisecond),
			},
			timeout: 300 * time.Millisecond,
			errMsg:  "icmp read error",
		},
		{
			description: "tcp connection returns error",
			icmpConn: &mockRawConn{
				readTimeoutCount: 100,
				readDeadline:     time.Now().Add(500 * time.Millisecond),
			},
			tcpConn: &mockRawConn{
				readFromErr: errors.New("tcp read error"),
			},
			timeout: 300 * time.Millisecond,
			errMsg:  "tcp read error",
		},
		{
			description: "successful ICMP parsing returns IP, port, and type code",
			icmpConn: &mockRawConn{
				header:  testutils.CreateMockIPv4Header(srcIP, dstIP, 1),
				payload: testutils.CreateMockICMPWithTCPPacket(nil, testutils.CreateMockICMPLayer(layers.ICMPv4TypeTimeExceeded, layers.ICMPv4CodeTTLExceeded), testutils.CreateMockIPv4Layer(4321, innerSrcIP, innerDstIP, layers.IPProtocolTCP), testutils.CreateMockTCPLayer(12345, 443, 28394, 12737, true, true, true), false),
			},
			tcpConn: &mockRawConn{
				readTimeoutCount: 100,
				readDeadline:     time.Now().Add(500 * time.Millisecond),
			},
			timeout:    500 * time.Millisecond,
			localIP:    innerSrcIP,
			localPort:  12345,
			remoteIP:   innerDstIP,
			remotePort: 443,
			seqNum:     28394,
			packetID:   4321,
			expectedResponse: packetResponse{
				IP:   srcIP,
				Type: layers.ICMPv4TypeTimeExceeded,
				Code: layers.ICMPv4CodeTTLExceeded,
			},
		},
		{
			description: "successful TCP parsing returns IP, port, and type code",
			icmpConn: &mockRawConn{
				readTimeoutCount: 100,
			},
			tcpConn: &mockRawConn{
				header:  testutils.CreateMockIPv4Header(dstIP, srcIP, 6),
				payload: tcpBytes,
			},
			timeout:    500 * time.Millisecond,
			localIP:    srcIP,
			localPort:  12345,
			remoteIP:   dstIP,
			remotePort: 443,
			seqNum:     28394,
			expectedResponse: packetResponse{
				IP:   dstIP,
				Port: 443,
			},
		},
	}

	for _, test := range tt {
		t.Run(test.description, func(t *testing.T) {
			actualResponse := listenPackets(test.icmpConn, test.tcpConn, test.timeout, test.localIP, test.localPort, test.remoteIP, test.remotePort, test.seqNum, test.packetID)
			require.Zero(t, test.icmpConn.numReaders.Load())
			require.Zero(t, test.tcpConn.numReaders.Load())
			if test.errMsg != "" {
				require.Error(t, actualResponse.Err)
				assert.True(t, strings.Contains(actualResponse.Err.Error(), test.errMsg), "error mismatch: expected %q, got %q", test.errMsg, actualResponse.Err.Error())
				return
			}
			require.NoError(t, actualResponse.Err)
			diff := cmp.Diff(test.expectedResponse, actualResponse,
				cmpopts.IgnoreFields(packetResponse{}, "Time"), // not important for this test
			)
			assert.Empty(t, diff)
		})
	}
}

func (m *mockRawConn) SetReadDeadline(t time.Time) error {
	if m.setReadDeadlineErr != nil {
		return m.setReadDeadlineErr
	}
	m.readDeadline = t

	return nil
}

func (m *mockRawConn) ReadFrom(_ []byte) (*ipv4.Header, []byte, *ipv4.ControlMessage, error) {
	m.numReaders.Add(1)
	defer m.numReaders.Add(-1)

	if m.readTimeoutCount > 0 {
		m.readTimeoutCount--
		time.Sleep(time.Until(m.readDeadline))
		return nil, nil, nil, &net.OpError{Err: mockTimeoutErr("test timeout error")}
	}
	if m.readFromErr != nil {
		return nil, nil, nil, m.readFromErr
	}
	if m.payload == nil {
		// it reads in a loop, so sleep for a bit to avoid log spam
		time.Sleep(time.Until(m.readDeadline))
	}

	return m.header, m.payload, m.cm, nil
}

func (m *mockRawConn) WriteTo(_ *ipv4.Header, _ []byte, _ *ipv4.ControlMessage) error {
	time.Sleep(m.writeDelay)
	return m.writeToErr
}

func (me mockTimeoutErr) Error() string {
	return string(me)
}

func (me mockTimeoutErr) Unwrap() error {
	return os.ErrDeadlineExceeded
}
