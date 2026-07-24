// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package listeners

import (
	"encoding/binary"
	"net"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeUDSQueueRequest(t *testing.T) {
	request := encodeUDSQueueRequest(0x11223344, 0x55667788)

	require.Len(t, request, netlinkHeaderLen+unixDiagReqLen)
	assert.Equal(t, uint32(len(request)), binary.NativeEndian.Uint32(request[0:4]))
	assert.Equal(t, uint16(sockDiagByFamily), binary.NativeEndian.Uint16(request[4:6]))
	assert.Equal(t, uint32(0x55667788), binary.NativeEndian.Uint32(request[8:12]))
	assert.Equal(t, byte(1), request[16])
	assert.Equal(t, ^uint32(0), binary.NativeEndian.Uint32(request[20:24]))
	assert.Equal(t, uint32(0x11223344), binary.NativeEndian.Uint32(request[24:28]))
	assert.Equal(t, uint32(udiagShowRQLen), binary.NativeEndian.Uint32(request[28:32]))
	assert.Equal(t, netlinkNoCookie, binary.NativeEndian.Uint32(request[32:36]))
	assert.Equal(t, netlinkNoCookie, binary.NativeEndian.Uint32(request[36:40]))
}

func TestParseUDSQueueResponse(t *testing.T) {
	const (
		sequence = 42
		inode    = 1234
	)
	response := make([]byte, netlinkHeaderLen+unixDiagMsgLen+12)
	binary.NativeEndian.PutUint32(response[0:4], uint32(len(response)))
	binary.NativeEndian.PutUint16(response[4:6], sockDiagByFamily)
	binary.NativeEndian.PutUint32(response[8:12], sequence)
	response[16] = 1
	response[17] = 2
	binary.NativeEndian.PutUint32(response[20:24], inode)
	attribute := response[netlinkHeaderLen+unixDiagMsgLen:]
	binary.NativeEndian.PutUint16(attribute[0:2], 12)
	binary.NativeEndian.PutUint16(attribute[2:4], unixDiagRQLen)
	binary.NativeEndian.PutUint32(attribute[4:8], 777)
	binary.NativeEndian.PutUint32(attribute[8:12], 888)

	stats, err := parseUDSQueueResponse(response, sequence, inode)
	require.NoError(t, err)
	assert.Equal(t, uint32(777), stats.nextPacketBytes)
}

func TestParseUDSQueueResponseRejectsMalformedMessages(t *testing.T) {
	valid := func() []byte {
		response := make([]byte, netlinkHeaderLen+unixDiagMsgLen+12)
		binary.NativeEndian.PutUint32(response[0:4], uint32(len(response)))
		binary.NativeEndian.PutUint16(response[4:6], sockDiagByFamily)
		binary.NativeEndian.PutUint32(response[8:12], 1)
		response[16] = 1
		binary.NativeEndian.PutUint32(response[20:24], 2)
		attribute := response[netlinkHeaderLen+unixDiagMsgLen:]
		binary.NativeEndian.PutUint16(attribute[0:2], 12)
		binary.NativeEndian.PutUint16(attribute[2:4], unixDiagRQLen)
		return response
	}

	tests := map[string]func([]byte) []byte{
		"short header": func(response []byte) []byte { return response[:10] },
		"invalid message length": func(response []byte) []byte {
			binary.NativeEndian.PutUint32(response[0:4], uint32(len(response)+1))
			return response
		},
		"wrong sequence": func(response []byte) []byte {
			binary.NativeEndian.PutUint32(response[8:12], 99)
			return response
		},
		"wrong inode": func(response []byte) []byte {
			binary.NativeEndian.PutUint32(response[20:24], 99)
			return response
		},
		"short rqlen": func(response []byte) []byte {
			binary.NativeEndian.PutUint16(response[32:34], 8)
			return response
		},
		"malformed attribute": func(response []byte) []byte {
			binary.NativeEndian.PutUint16(response[32:34], 100)
			return response
		},
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := parseUDSQueueResponse(mutate(valid()), 1, 2)
			require.Error(t, err)
		})
	}
}

func TestUDSQueueDiagReportsNextDatagramSize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dogstatsd.sock")
	address := &net.UnixAddr{Name: path, Net: "unixgram"}
	receiver, err := net.ListenUnixgram("unixgram", address)
	require.NoError(t, err)
	defer receiver.Close()

	inode, err := socketInode(receiver)
	require.NoError(t, err)
	diag, err := newUDSQueueDiag()
	require.NoError(t, err)
	defer diag.close()

	stats, err := diag.get(inode)
	require.NoError(t, err)
	assert.Zero(t, stats.nextPacketBytes)

	sender, err := net.DialUnix("unixgram", nil, address)
	require.NoError(t, err)
	defer sender.Close()
	_, err = sender.Write(make([]byte, 17))
	require.NoError(t, err)
	_, err = sender.Write(make([]byte, 53))
	require.NoError(t, err)

	stats, err = diag.get(inode)
	require.NoError(t, err)
	assert.Equal(t, uint32(17), stats.nextPacketBytes, "RQLEN is the next datagram size, not total queue depth")

	buffer := make([]byte, 100)
	_, _, err = receiver.ReadFromUnix(buffer)
	require.NoError(t, err)
	stats, err = diag.get(inode)
	require.NoError(t, err)
	assert.Equal(t, uint32(53), stats.nextPacketBytes)

	_, _, err = receiver.ReadFromUnix(buffer)
	require.NoError(t, err)
	stats, err = diag.get(inode)
	require.NoError(t, err)
	assert.Zero(t, stats.nextPacketBytes)
}
