// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package nstat

import (
	"encoding/binary"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEncodeAddAllSources(t *testing.T) {
	request := encodeAddAllSources(17, ProviderTCPKernel)

	require.Len(t, request, addAllSourcesRequestSize)
	require.Equal(t, uint64(17), binary.LittleEndian.Uint64(request[0:8]))
	require.Equal(t, messageAddAllSources, binary.LittleEndian.Uint32(request[8:12]))
	require.Equal(t, uint16(addAllSourcesRequestSize), binary.LittleEndian.Uint16(request[12:14]))
	require.Equal(t, headerFlagSupportsAggregate, binary.LittleEndian.Uint16(request[14:16]))
	require.Equal(t, filterFlagsV1Usage, binary.LittleEndian.Uint64(request[16:24]))
	require.Equal(t, ProviderTCPKernel, binary.LittleEndian.Uint32(request[32:36]))
	require.Equal(t, ^uint32(0), binary.LittleEndian.Uint32(request[36:40]))
	require.Equal(t, make([]byte, 16), request[40:])
}

func TestEncodeSourceRequest(t *testing.T) {
	request := encodeSourceRequest(23, messageGetSourceDesc, 42)

	require.Len(t, request, sourceRefRequestSize)
	require.Equal(t, uint64(23), binary.LittleEndian.Uint64(request[0:8]))
	require.Equal(t, messageGetSourceDesc, binary.LittleEndian.Uint32(request[8:12]))
	require.Equal(t, uint16(sourceRefRequestSize), binary.LittleEndian.Uint16(request[12:14]))
	require.Equal(t, headerFlagSupportsAggregate, binary.LittleEndian.Uint16(request[14:16]))
	require.Equal(t, uint64(42), binary.LittleEndian.Uint64(request[16:24]))
}

func TestClosedControlRejectsOperations(t *testing.T) {
	control := &Control{fd: -1}

	_, err := control.Subscribe(ProviderTCPKernel)
	require.ErrorIs(t, err, io.ErrClosedPipe)
	require.ErrorIs(t, control.QueryAll(), io.ErrClosedPipe)
	require.ErrorIs(t, control.RequestDescription(42), io.ErrClosedPipe)
	_, err = control.Poll(0)
	require.ErrorIs(t, err, io.ErrClosedPipe)
	_, err = control.Receive(make([]byte, 1))
	require.ErrorIs(t, err, io.ErrClosedPipe)
	require.NoError(t, control.Close())
}

func TestSubscribeRejectsUnknownProvider(t *testing.T) {
	control := &Control{fd: -1}

	_, err := control.Subscribe(99)

	require.ErrorContains(t, err, "unsupported nstat provider")
}
