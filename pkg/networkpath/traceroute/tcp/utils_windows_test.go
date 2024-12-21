// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package tcp

import (
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/ipv4"
	"golang.org/x/sys/windows"
)

func Test_MatchTCP(t *testing.T) {
	srcPort := uint16(12345)
	dstPort := uint16(443)
	seqNum := uint32(2549)
	mockHeader := testutils.CreateMockIPv4Header(dstIP, srcIP, 6)
	_, tcpBytes := testutils.CreateMockTCPPacket(mockHeader, testutils.CreateMockTCPLayer(dstPort, srcPort, seqNum, seqNum+1, true, true, true), false)

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
			description: "protocol not TCP returns an error",
			header: &ipv4.Header{
				Protocol: windows.IPPROTO_UDP,
			},
			expectedIP:     net.IP{},
			expectedErrMsg: "expected a TCP packet",
		},
		{
			description:    "non-matching source IP returns mismatch error",
			header:         testutils.CreateMockIPv4Header(dstIP, net.ParseIP("2.2.2.2"), 6),
			localIP:        srcIP,
			remoteIP:       dstIP,
			expectedIP:     net.IP{},
			expectedErrMsg: "TCP packet doesn't match",
		},
		{
			description:    "non-matching destination IP returns mismatch error",
			header:         testutils.CreateMockIPv4Header(net.ParseIP("2.2.2.2"), srcIP, 6),
			localIP:        srcIP,
			remoteIP:       dstIP,
			expectedIP:     net.IP{},
			expectedErrMsg: "TCP packet doesn't match",
		},
		{
			description:    "bad TCP payload returns an error",
			header:         mockHeader,
			localIP:        srcIP,
			remoteIP:       dstIP,
			expectedIP:     net.IP{},
			expectedErrMsg: "TCP parse error",
		},
		{
			description:    "non-matching TCP payload returns mismatch error",
			header:         mockHeader,
			payload:        tcpBytes,
			localIP:        srcIP,
			localPort:      srcPort,
			remoteIP:       dstIP,
			remotePort:     9001,
			seqNum:         seqNum,
			expectedIP:     net.IP{},
			expectedErrMsg: "TCP packet doesn't match",
		},
		{
			description:    "matching TCP payload returns destination IP",
			header:         mockHeader,
			payload:        tcpBytes,
			localIP:        srcIP,
			localPort:      srcPort,
			remoteIP:       dstIP,
			remotePort:     dstPort,
			seqNum:         seqNum,
			expectedIP:     dstIP,
			expectedErrMsg: "",
		},
	}

	for _, test := range tts {
		t.Run(test.description, func(t *testing.T) {
			tp := newTCPParser()
			actualIP, err := tp.MatchTCP(test.header, test.payload, test.localIP, test.localPort, test.remoteIP, test.remotePort, test.seqNum)
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
