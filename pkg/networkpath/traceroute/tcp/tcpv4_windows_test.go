// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package tcp

import (
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/common"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/winconn"
	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSendAndReceive(t *testing.T) {
	dstIP := net.ParseIP("5.6.7.8")
	tcpv4 := NewTCPv4(dstIP, 443, 1, 1, 30, 0, 0)
	tcpv4.srcIP = net.ParseIP("1.2.3.4")
	tcpv4.srcPort = 12345
	tts := []struct {
		description     string
		mockSendError   error
		mockHopIP       net.IP
		mockEnd         time.Time
		mockListenError error
		expected        *common.Hop
		errMsg          string
	}{
		{
			description:   "send error returns error",
			mockSendError: errors.New("test send error"),
			errMsg:        "test send error",
		},
		{
			description:     "listen error returns error",
			mockListenError: errors.New("test listen error"),
			errMsg:          "test listen error",
		},
		{
			description: "successful send and receive, hop not found",
			mockEnd:     time.Now().Add(60 * time.Minute), // end time greater than start time
			mockHopIP:   net.IP{},
			expected: &common.Hop{
				IP:     net.IP{},
				RTT:    0, // RTT should be zero
				IsDest: false,
			},
		},
		{
			description: "successful send and receive, hop found",
			mockEnd:     time.Now().Add(60 * time.Minute), // end time greater than start time
			mockHopIP:   net.ParseIP("7.8.9.0"),
			expected: &common.Hop{
				IP:     net.ParseIP("7.8.9.0"),
				IsDest: false,
			},
		},
		{
			description: "successful send and receive, destination hop found",
			mockEnd:     time.Now().Add(60 * time.Minute), // end time greater than start time
			mockHopIP:   dstIP,
			expected: &common.Hop{
				IP:     dstIP,
				IsDest: true,
			},
		},
	}

	for _, test := range tts {
		t.Run(test.description, func(t *testing.T) {
			controller := gomock.NewController(t)
			defer controller.Finish()
			mockRawConn := winconn.NewMockRawConnWrapper(controller)
			mockRawConn.EXPECT().SendRawPacket(gomock.Any(), gomock.Any(), gomock.Any()).Return(test.mockSendError)
			if test.mockSendError == nil { // only expect ListenPackets call if SendRawPacket is successful
				mockRawConn.EXPECT().ListenPackets(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(test.mockHopIP, test.mockEnd, test.mockListenError)
			}
			actual, err := tcpv4.sendAndReceive(mockRawConn, 1, 418, 1*time.Second)
			if test.errMsg != "" {
				require.Error(t, err)
				assert.True(t, strings.Contains(err.Error(), test.errMsg), "error mismatch: excpected %q, got %q", test.errMsg, err.Error())
				return
			}
			require.NoError(t, err)
			assert.Empty(t, cmp.Diff(test.expected, actual, cmpopts.IgnoreFields(common.Hop{}, "RTT")))
			if !test.mockHopIP.Equal(net.IP{}) { // only if we get a hop IP back should RTT be >0
				assert.Greater(t, actual.RTT, time.Duration(0))
			} else {
				assert.Equal(t, actual.RTT, time.Duration(0))
			}
		})
	}
}
