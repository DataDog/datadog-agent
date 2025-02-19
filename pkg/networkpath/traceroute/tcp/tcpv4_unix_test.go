// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test && unix

package tcp

import (
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/common"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/ipv4"
)

func TestSendAndReceive(t *testing.T) {

	tcpv4 := NewTCPv4(net.ParseIP("5.6.7.8"), 443, 1, 1, 1, 0, 0)
	tcpv4.srcIP = net.ParseIP("1.2.3.4") // set these after constructor
	tcpv4.srcPort = 12345
	tts := []struct {
		description string
		ttl         int
		sendFunc    func(rawConnWrapper, *ipv4.Header, []byte) error
		listenFunc  func(rawConnWrapper, rawConnWrapper, time.Duration, net.IP, uint16, net.IP, uint16, uint32) packetResponse
		expected    *common.Hop
		errMsg      string
	}{
		{
			description: "sendPacket error",
			ttl:         64,
			sendFunc: func(_ rawConnWrapper, _ *ipv4.Header, _ []byte) error {
				return fmt.Errorf("sendPacket error")
			},
			listenFunc: func(_ rawConnWrapper, _ rawConnWrapper, _ time.Duration, _ net.IP, _ uint16, _ net.IP, _ uint16, _ uint32) packetResponse {
				return packetResponse{}
			},
			expected: nil,
			errMsg:   "sendPacket error",
		},
		{
			description: "listenPackets error",
			ttl:         64,
			sendFunc: func(_ rawConnWrapper, _ *ipv4.Header, _ []byte) error {
				return nil
			},
			listenFunc: func(_ rawConnWrapper, _ rawConnWrapper, _ time.Duration, _ net.IP, _ uint16, _ net.IP, _ uint16, _ uint32) packetResponse {
				return packetResponse{Err: fmt.Errorf("listenPackets error")}
			},
			expected: nil,
			errMsg:   "listenPackets error",
		},
		{
			description: "successful send and receive",
			ttl:         64,
			sendFunc: func(_ rawConnWrapper, _ *ipv4.Header, _ []byte) error {
				return nil
			},
			listenFunc: func(_ rawConnWrapper, _ rawConnWrapper, _ time.Duration, _ net.IP, _ uint16, _ net.IP, _ uint16, _ uint32) packetResponse {
				return packetResponse{
					IP:   net.ParseIP("7.8.9.0"),
					Type: 2,
					Code: 3,
					Port: 443,
					Time: time.Now(),
					Err:  nil,
				}
			},
			expected: &common.Hop{
				IP:       net.ParseIP("7.8.9.0"),
				ICMPType: 2,
				ICMPCode: 3,
				Port:     443,
				IsDest:   false,
			},
			errMsg: "",
		},
	}

	for _, test := range tts {
		t.Run(test.description, func(t *testing.T) {
			sendPacketFunc = test.sendFunc
			listenPacketsFunc = test.listenFunc
			actual, err := tcpv4.sendAndReceive(nil, nil, test.ttl, 418, 1*time.Second)
			if test.errMsg != "" {
				require.Error(t, err)
				assert.True(t, strings.Contains(err.Error(), test.errMsg), "error mismatch: excpected %q, got %q", test.errMsg, err.Error())
				return
			}
			require.NoError(t, err)
			assert.Empty(t, cmp.Diff(test.expected, actual, cmpopts.IgnoreFields(common.Hop{}, "RTT")))
			assert.Greater(t, actual.RTT, time.Duration(0))
		})
	}
}
