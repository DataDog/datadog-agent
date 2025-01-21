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
	"strings"
	"testing"
	"time"

	"github.com/google/gopacket/layers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/ipv4"

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
		listener   string
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
			description: "invalid listener returns unsupported listener",
			ctxTimeout:  1 * time.Second,
			conn: &mockRawConn{
				header:  &ipv4.Header{},
				payload: nil,
			},
			listener: "invalid",
			errMsg:   "unsupported",
		},
		{
			description: "failed ICMP parsing eventuallly returns cancel timeout",
			ctxTimeout:  500 * time.Millisecond,
			conn: &mockRawConn{
				header:  &ipv4.Header{},
				payload: nil,
			},
			listener: "icmp",
			errMsg:   "canceled",
		},
		{
			description: "failed TCP parsing eventuallly returns cancel timeout",
			ctxTimeout:  500 * time.Millisecond,
			conn: &mockRawConn{
				header:  &ipv4.Header{},
				payload: nil,
			},
			listener: "tcp",
			errMsg:   "canceled",
		},
		{
			description: "successful ICMP parsing returns IP, port, and type code",
			ctxTimeout:  500 * time.Millisecond,
			conn: &mockRawConn{
				header:  testutils.CreateMockIPv4Header(srcIP, dstIP, 1),
				payload: testutils.CreateMockICMPPacket(nil, testutils.CreateMockICMPLayer(layers.ICMPv4CodeTTLExceeded), testutils.CreateMockIPv4Layer(innerSrcIP, innerDstIP, layers.IPProtocolTCP), testutils.CreateMockTCPLayer(12345, 443, 28394, 12737, true, true, true), false),
			},
			localIP:          innerSrcIP,
			localPort:        12345,
			remoteIP:         innerDstIP,
			remotePort:       443,
			seqNum:           28394,
			listener:         "icmp",
			expectedIP:       srcIP,
			expectedPort:     0,
			expectedTypeCode: layers.ICMPv4CodeTTLExceeded,
		},
		{
			description: "successful TCP parsing returns IP, port, and type code",
			ctxTimeout:  500 * time.Millisecond,
			conn: &mockRawConn{
				header:  testutils.CreateMockIPv4Header(dstIP, srcIP, 6),
				payload: tcpBytes,
			},
			localIP:          srcIP,
			localPort:        12345,
			remoteIP:         dstIP,
			remotePort:       443,
			seqNum:           28394,
			listener:         "tcp",
			expectedIP:       dstIP,
			expectedPort:     443,
			expectedTypeCode: 0,
		},
	}

	for _, test := range tt {
		t.Run(test.description, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), test.ctxTimeout)
			defer cancel()
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

func (m *mockRawConn) SetReadDeadline(t time.Time) error {
	if m.setReadDeadlineErr != nil {
		return m.setReadDeadlineErr
	}
	m.readDeadline = t

	return nil
}

func (m *mockRawConn) ReadFrom(_ []byte) (*ipv4.Header, []byte, *ipv4.ControlMessage, error) {
	if m.readTimeoutCount > 0 {
		m.readTimeoutCount--
		time.Sleep(time.Until(m.readDeadline))
		return nil, nil, nil, &net.OpError{Err: mockTimeoutErr("test timeout error")}
	}
	if m.readFromErr != nil {
		return nil, nil, nil, m.readFromErr
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

func (me mockTimeoutErr) Timeout() bool {
	return true
}
