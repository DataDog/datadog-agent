// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package common

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/testutils"
	"github.com/google/gopacket/layers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/ipv4"
	"golang.org/x/sys/windows"
)

func Test_listenPackets(t *testing.T) {
	srcIP := net.ParseIP("99.99.99.99")
	dstIP := net.ParseIP("127.0.0.1")
	innerSrcIP := net.ParseIP("88.88.88.88")
	innerDstIP := net.ParseIP("77.77.77.77")
	mockICMPPacket := testutils.CreateMockICMPPacket(
		testutils.CreateMockIPv4Layer(srcIP, dstIP, layers.IPProtocolICMPv4),
		testutils.CreateMockICMPLayer(layers.ICMPv4CodeTTLExceeded),
		testutils.CreateMockIPv4Layer(innerSrcIP, innerDstIP, layers.IPProtocolTCP),
		testutils.CreateMockTCPLayer(12345, 443, 28394, 12737, true, true, true),
		false,
	)
	start := time.Now()

	tts := []struct {
		description    string
		timeout        time.Duration
		matcherFuncs   map[int]MatcherFunc
		recvFrom       func(windows.Handle, []byte, int) (int, windows.Sockaddr, error)
		expectedIP     net.IP
		expectFinished bool // if true, we should test that a later finish timestamp is returned
		expectedErrMsg string
	}{
		{
			description: "canceled context returns zero values and no error",
			timeout:     500 * time.Millisecond,
			recvFrom: func(_ windows.Handle, _ []byte, _ int) (int, windows.Sockaddr, error) {
				time.Sleep(100 * time.Millisecond)
				return 0, nil, windows.WSAETIMEDOUT // consistently return timeout errors
			},
			expectedIP:     net.IP{},
			expectedErrMsg: "",
		},
		{
			description: "downstream error returns the error",
			timeout:     500 * time.Millisecond,
			recvFrom: func(_ windows.Handle, _ []byte, _ int) (int, windows.Sockaddr, error) {
				return 0, nil, errors.New("test handlePackets error")
			},
			expectedIP:     net.IP{},
			expectedErrMsg: "error: test handlePackets error",
		},
		{
			description: "successful call returns IP and timestamp",
			timeout:     500 * time.Millisecond,
			matcherFuncs: map[int]MatcherFunc{
				windows.IPPROTO_ICMP: func(_ *ipv4.Header, _ []byte, _ net.IP, _ uint16, _ net.IP, _ uint16, _ uint32) (net.IP, error) {
					return srcIP, nil
				},
			},
			recvFrom: func(_ windows.Handle, buf []byte, _ int) (int, windows.Sockaddr, error) {
				copy(buf, mockICMPPacket)

				return len(mockICMPPacket), nil, nil
			},
			expectedIP:     srcIP,
			expectFinished: true,
			expectedErrMsg: "",
		},
	}

	// these don't matter in the test, but are required parameters
	socket := &Winrawsocket{}
	inputIP := net.ParseIP("127.0.0.1")
	inputPort := uint16(161)
	seqNum := uint32(1)
	for _, test := range tts {
		t.Run(test.description, func(t *testing.T) {
			recvFrom = test.recvFrom
			actualIP, finished, err := socket.ListenPackets(test.timeout, inputIP, inputPort, inputIP, inputPort, seqNum, test.matcherFuncs)
			if test.expectedErrMsg != "" {
				require.Error(t, err)
				assert.True(t, strings.Contains(err.Error(), test.expectedErrMsg), fmt.Sprintf("expected %q, got %q", test.expectedErrMsg, err.Error()))
			} else {
				require.NoError(t, err)
			}
			assert.Truef(t, test.expectedIP.Equal(actualIP), "mismatch IPs: expected %s, got %s", test.expectedIP.String(), actualIP.String())

			if test.expectFinished {
				assert.Truef(t, finished.After(start), "finished timestamp should be later than start: finished %s, start %s", finished, start)
			} else {
				assert.Equal(t, finished, time.Time{})
			}
		})
	}
}

func Test_handlePackets(t *testing.T) {
	_, tcpBytes := testutils.CreateMockTCPPacket(testutils.CreateMockIPv4Header(dstIP, srcIP, 6), testutils.CreateMockTCPLayer(443, 12345, 28394, 28395, true, true, true), true)

	tt := []struct {
		description string
		// input
		ctxTimeout   time.Duration
		matcherFuncs map[int]MatcherFunc
		recvFrom     func(windows.Handle, []byte, int) (int, windows.Sockaddr, error)
		// output
		expectedIP net.IP
		errMsg     string
	}{
		{
			description: "canceled context returns canceledErr",
			ctxTimeout:  300 * time.Millisecond,
			recvFrom: func(_ windows.Handle, _ []byte, _ int) (int, windows.Sockaddr, error) {
				time.Sleep(100 * time.Millisecond)
				return 0, nil, windows.WSAETIMEDOUT
			},
			errMsg: "canceled",
		},
		{
			description: "oversized messages eventually returns canceledErr",
			ctxTimeout:  300 * time.Millisecond,
			recvFrom: func(_ windows.Handle, _ []byte, _ int) (int, windows.Sockaddr, error) {
				time.Sleep(100 * time.Millisecond)
				return 0, nil, windows.WSAEMSGSIZE
			},
			errMsg: "canceled",
		},
		{
			description: "non-timeout read error returns an error",
			ctxTimeout:  1 * time.Second,
			recvFrom: func(_ windows.Handle, _ []byte, _ int) (int, windows.Sockaddr, error) {
				return 0, nil, errors.New("test read error")
			},
			errMsg: "test read error",
		},
		{
			description: "failed parsing eventually returns cancel timeout",
			ctxTimeout:  500 * time.Millisecond,
			recvFrom: func(_ windows.Handle, _ []byte, _ int) (int, windows.Sockaddr, error) {
				copy(buf, tcpBytes)

				return len(tcpBytes), nil, nil
			},
			matcherFuncs: map[int]MatcherFunc{
				windows.IPPROTO_TCP: func(_ *ipv4.Header, _ []byte, _ net.IP, _ uint16, _ net.IP, _ uint16, _ uint32) (net.IP, error) {
					return net.IP{}, errors.New("failed parsing packet")
				},
			},
			errMsg: "canceled",
		},
		{
			description: "no matcher eventually returns cancel timeout",
			ctxTimeout:  500 * time.Millisecond,
			recvFrom: func(_ windows.Handle, _ []byte, _ int) (int, windows.Sockaddr, error) {
				copy(buf, tcpBytes)

				return len(tcpBytes), nil, nil
			},
			matcherFuncs: map[int]MatcherFunc{},
			errMsg:       "canceled",
		},
		{
			description: "successful matching returns IP, port, and type code",
			ctxTimeout:  500 * time.Millisecond,
			recvFrom: func(_ windows.Handle, _ []byte, _ int) (int, windows.Sockaddr, error) {
				copy(buf, tcpBytes)

				return len(tcpBytes), nil, nil
			},
			matcherFuncs: map[int]MatcherFunc{
				windows.IPPROTO_TCP: func(_ *ipv4.Header, _ []byte, _ net.IP, _ uint16, _ net.IP, _ uint16, _ uint32) (net.IP, error) {
					return srcIP, nil
				},
			},
			expectedIP: srcIP,
		},
	}

	for _, test := range tt {
		t.Run(test.description, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), test.ctxTimeout)
			defer cancel()
			recvFrom = test.recvFrom
			w := &Winrawsocket{}
			actualIP, _, err := w.handlePackets(ctx, net.IP{}, uint16(0), net.IP{}, uint16(0), uint32(0), test.matcherFuncs)
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

func Test_SendRawPacket(t *testing.T) {
	tts := []struct {
		description    string
		destIP         net.IP
		destPort       uint16
		payload        []byte
		sendTo         func(windows.Handle, []byte, int, windows.Sockaddr) error
		expectedErrMsg string
	}{
		{
			description:    "non-IPv4 address returns an error",
			destIP:         net.ParseIP("e2cc:0314:92fe:1307:94e3:0108:a67c:980c"),
			destPort:       161,
			payload:        []byte{},
			sendTo:         nil,
			expectedErrMsg: "unable to parse IP address",
		},
		{
			description: "sendTo error returns an error",
			destIP:      net.ParseIP("8.8.8.8"),
			destPort:    161,
			payload:     []byte{},
			sendTo: func(_ windows.Handle, _ []byte, _ int, _ windows.Sockaddr) error {
				return errors.New("test error")
			},
			expectedErrMsg: "test error",
		},
		{
			description: "successful send returns nil",
			destIP:      net.ParseIP("8.8.8.8"),
			destPort:    161,
			payload:     []byte{1, 2, 3},
			sendTo: func(_ windows.Handle, payload []byte, _ int, addr windows.Sockaddr) error {
				expectedPayload := []byte{1, 2, 3}
				expectedSockaddr := &windows.SockaddrInet4{
					Port: 161,
					Addr: [4]byte{8, 8, 8, 8},
				}
				assert.Equalf(t, payload, expectedPayload, "mismatched payloads in sendTo: expected %+v, got %+v", expectedPayload, payload)
				assert.Equalf(t, addr, expectedSockaddr, "mismatched adddresses: expected %+v, got %+v", expectedSockaddr, addr)
				return nil
			},
			expectedErrMsg: "",
		},
	}

	w := &Winrawsocket{}
	for _, test := range tts {
		t.Run(test.description, func(t *testing.T) {
			sendTo = test.sendTo
			err := w.SendRawPacket(test.destIP, test.destPort, test.payload)
			if test.expectedErrMsg != "" {
				require.Error(t, err)
				assert.True(t, strings.Contains(err.Error(), test.expectedErrMsg), fmt.Sprintf("expected %q, got %q", test.expectedErrMsg, err.Error()))
				return
			}
			require.NoError(t, err)
		})
	}
}

func Test_MatchICMP(t *testing.T) {
	srcPort := uint16(12345)
	dstPort := uint16(443)
	seqNum := uint32(2549)
	mockHeader := testutils.CreateMockIPv4Header(srcIP, dstIP, 1)
	icmpLayer := testutils.CreateMockICMPLayer(layers.ICMPv4CodeTTLExceeded)
	innerIPv4Layer := testutils.CreateMockIPv4Layer(innerSrcIP, innerDstIP, layers.IPProtocolTCP)
	innerTCPLayer := testutils.CreateMockTCPLayer(srcPort, dstPort, seqNum, 12737, true, true, true)
	icmpBytes := testutils.CreateMockICMPPacket(nil, icmpLayer, innerIPv4Layer, innerTCPLayer, true)

	tts := []struct {
		description string
		header      *ipv4.Header
		payload     []byte
		localIP     net.IP
		localPort   uint16
		remoteIP    net.IP
		remotePort  uint16
		seqNum      uint32
		// expected
		expectedIP     net.IP
		expectedErrMsg string
	}{
		{
			description: "protocol not ICMP returns an error",
			header: &ipv4.Header{
				Protocol: windows.IPPROTO_UDP,
			},
			expectedIP:     net.IP{},
			expectedErrMsg: "expected an ICMP packet",
		},
		{
			description:    "bad ICMP payload returns an error",
			header:         mockHeader,
			localIP:        srcIP,
			remoteIP:       dstIP,
			expectedIP:     net.IP{},
			expectedErrMsg: "ICMP parse error",
		},
		{
			description:    "non-matching ICMP payload returns mismatch error",
			header:         mockHeader,
			payload:        icmpBytes,
			localIP:        srcIP,
			localPort:      srcPort,
			remoteIP:       dstIP,
			remotePort:     9001,
			seqNum:         seqNum,
			expectedIP:     net.IP{},
			expectedErrMsg: "ICMP packet doesn't match",
		},
		{
			description:    "matching ICMP payload returns destination IP",
			header:         mockHeader,
			payload:        icmpBytes,
			localIP:        innerSrcIP,
			localPort:      srcPort,
			remoteIP:       innerDstIP,
			remotePort:     dstPort,
			seqNum:         seqNum,
			expectedIP:     srcIP,
			expectedErrMsg: "",
		},
	}

	for _, test := range tts {
		t.Run(test.description, func(t *testing.T) {
			icmpParser := NewICMPTCPParser()
			actualIP, err := icmpParser.MatchICMP(test.header, test.payload, test.localIP, test.localPort, test.remoteIP, test.remotePort, test.seqNum)
			if test.expectedErrMsg != "" {
				require.Error(t, err)
				assert.True(t, strings.Contains(err.Error(), test.expectedErrMsg), fmt.Sprintf("expected %q, got %q", test.expectedErrMsg, err.Error()))
				return
			}
			require.NoError(t, err)
			assert.Truef(t, test.expectedIP.Equal(actualIP), "mismatch IPs: expected %s, got %s", test.expectedIP.String(), actualIP.String())
		})
	}
}
