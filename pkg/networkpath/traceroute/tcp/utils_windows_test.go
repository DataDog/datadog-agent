// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package tcp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/common"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/testutils"
	"github.com/google/gopacket/layers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type (
	mockRawConn struct {
		readTimeoutCount int
		readTimeout      time.Duration
		readFromErr      error

		payload []byte
	}
)

func Test_handlePackets(t *testing.T) {
	_, tcpBytes := testutils.CreateMockTCPPacket(testutils.CreateMockIPv4Header(dstIP, srcIP, 6), testutils.CreateMockTCPLayer(443, 12345, 28394, 28395, true, true, true), true)

	tt := []struct {
		description string
		// input
		ctxTimeout time.Duration
		conn       *mockRawConn
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
				readTimeout:      100 * time.Millisecond,
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
		{
			description: "failed parsing eventually returns cancel timeout",
			ctxTimeout:  500 * time.Millisecond,
			conn: &mockRawConn{
				payload: []byte{},
			},
			errMsg: "canceled",
		},
		{
			description: "successful ICMP parsing returns IP, port, and type code",
			ctxTimeout:  500 * time.Millisecond,
			conn: &mockRawConn{
				payload: testutils.CreateMockICMPPacket(testutils.CreateMockIPv4Layer(srcIP, dstIP, layers.IPProtocolICMPv4), testutils.CreateMockICMPLayer(layers.ICMPv4CodeTTLExceeded), testutils.CreateMockIPv4Layer(innerSrcIP, innerDstIP, layers.IPProtocolTCP), testutils.CreateMockTCPLayer(12345, 443, 28394, 12737, true, true, true), false),
			},
			localIP:    innerSrcIP,
			localPort:  12345,
			remoteIP:   innerDstIP,
			remotePort: 443,
			seqNum:     28394,
			expectedIP: srcIP,
		},
		{
			description: "successful TCP parsing returns IP, port, and type code",
			ctxTimeout:  500 * time.Millisecond,
			conn: &mockRawConn{
				payload: tcpBytes,
			},
			localIP:    srcIP,
			localPort:  12345,
			remoteIP:   dstIP,
			remotePort: 443,
			seqNum:     28394,
			expectedIP: dstIP,
		},
	}

	for _, test := range tt {
		t.Run(test.description, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), test.ctxTimeout)
			defer cancel()
			recvFrom = test.conn.RecvFrom
			w := &common.Winrawsocket{}
			actualIP, _, err := handlePackets(ctx, w, test.localIP, test.localPort, test.remoteIP, test.remotePort, test.seqNum)
			if test.errMsg != "" {
				require.Error(t, err)
				assert.True(t, strings.Contains(err.Error(), test.errMsg), fmt.Sprintf("expected %q, got %q", test.errMsg, err.Error()))
				return
			}
			require.NoError(t, err)
			assert.Truef(t, test.expectedIP.Equal(actualIP), "mismatch source IPs: expected %s, got %s", test.expectedIP.String(), actualIP.String())
		})
	}
}

func Test_listenPackets(t *testing.T) {
	anIP := net.ParseIP("8.8.4.4")
	aFinishedTimestamp := time.Now()

	tts := []struct {
		description       string
		timeout           time.Duration
		handlePacketsFunc func(context.Context, *common.Winrawsocket, net.IP, uint16, net.IP, uint16, uint32) (net.IP, time.Time, error)
		expectedIP        net.IP
		expectedFinished  time.Time
		expectedErrMsg    string
	}{
		{
			description: "canceled context returns zero values and no error",
			timeout:     500 * time.Millisecond,
			handlePacketsFunc: func(context.Context, *common.Winrawsocket, net.IP, uint16, net.IP, uint16, uint32) (net.IP, time.Time, error) {
				return net.IP{}, time.Time{}, common.CanceledError("test canceled error")
			},
			expectedIP:       net.IP{},
			expectedFinished: time.Time{},
			expectedErrMsg:   "",
		},
		{
			description: "handlePackets error returns wrapped error",
			timeout:     500 * time.Millisecond,
			handlePacketsFunc: func(context.Context, *common.Winrawsocket, net.IP, uint16, net.IP, uint16, uint32) (net.IP, time.Time, error) {
				return net.IP{}, time.Time{}, errors.New("test handlePackets error")
			},
			expectedIP:       net.IP{},
			expectedFinished: time.Time{},
			expectedErrMsg:   "error: test handlePackets error",
		},
		{
			description: "successful handlePackets call returns IP and timestamp",
			timeout:     500 * time.Millisecond,
			handlePacketsFunc: func(context.Context, *common.Winrawsocket, net.IP, uint16, net.IP, uint16, uint32) (net.IP, time.Time, error) {
				return anIP, aFinishedTimestamp, nil
			},
			expectedIP:       anIP,
			expectedFinished: aFinishedTimestamp,
			expectedErrMsg:   "",
		},
	}

	// these don't matter in the test, but are required parameters
	socket := &common.Winrawsocket{}
	inputIP := net.ParseIP("127.0.0.1")
	inputPort := uint16(161)
	seqNum := uint32(1)
	for _, test := range tts {
		t.Run(test.description, func(t *testing.T) {
			handlePacketsFunc = test.handlePacketsFunc // mock out function call
			actualIP, actualFinished, err := listenPackets(socket, test.timeout, inputIP, inputPort, inputIP, inputPort, seqNum)
			if test.expectedErrMsg != "" {
				require.Error(t, err)
				assert.True(t, strings.Contains(err.Error(), test.expectedErrMsg), fmt.Sprintf("expected %q, got %q", test.expectedErrMsg, err.Error()))
			} else {
				require.NoError(t, err)
			}
			assert.Truef(t, test.expectedIP.Equal(actualIP), "mismatch IPs: expected %s, got %s", test.expectedIP.String(), actualIP.String())
			assert.Equal(t, test.expectedFinished, actualFinished)
		})
	}
}

func (m *mockRawConn) RecvFrom(_ windows.Handle, buf []byte, _ int) (int, windows.Sockaddr, error) {
	if m.readTimeoutCount > 0 {
		m.readTimeoutCount--
		time.Sleep(m.readTimeout)
		return 0, nil, windows.WSAETIMEDOUT
	}
	if m.readFromErr != nil {
		return 0, nil, m.readFromErr
	}
	copy(buf, m.payload)

	return len(m.payload), nil, nil
}
