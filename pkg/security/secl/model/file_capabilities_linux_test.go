// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package model

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileCapabilitiesBinary(t *testing.T) {
	for _, caps := range []FileCapabilities{
		{},
		{Available: true},
		{Permitted: 1 << 10, Inheritable: 1 << 39, Available: true},
		{Permitted: 1<<10 | 1<<40, Inheritable: 1 << 39, Effective: true, Available: true},
	} {
		data := bytes.Repeat([]byte{0xff}, FileCapabilitiesSize)
		n, err := caps.MarshalBinary(data)
		require.NoError(t, err)
		assert.Equal(t, FileCapabilitiesSize, n)
		assert.Equal(t, caps.Permitted, binary.NativeEndian.Uint64(data[0:8]))
		assert.Equal(t, caps.Inheritable, binary.NativeEndian.Uint64(data[8:16]))
		assert.Equal(t, caps.Effective, binary.NativeEndian.Uint32(data[16:20]) == 1)
		assert.Equal(t, caps.Available, binary.NativeEndian.Uint32(data[20:24]) == 1)

		var decoded FileCapabilities
		n, err = decoded.UnmarshalBinary(data)
		require.NoError(t, err)
		assert.Equal(t, FileCapabilitiesSize, n)
		assert.Equal(t, caps, decoded)
	}
}

func TestFileCapabilitiesBinaryShortBuffer(t *testing.T) {
	for size := 0; size < FileCapabilitiesSize; size++ {
		data := make([]byte, size)
		var caps FileCapabilities
		_, err := caps.MarshalBinary(data)
		require.ErrorIs(t, err, ErrNotEnoughSpace)
		_, err = caps.UnmarshalBinary(data)
		require.ErrorIs(t, err, ErrNotEnoughData)
	}
}

func TestProcessFileCapabilitiesBinary(t *testing.T) {
	process := Process{
		FileCapabilities: FileCapabilities{
			Permitted:   1<<10 | 1<<40,
			Inheritable: 1 << 39,
			Effective:   true,
			Available:   true,
		},
		Comm:        "file-caps",
		ExecTime:    time.Unix(10, 0),
		Credentials: Credentials{UID: 123, CapPermitted: 1 << 10},
	}
	procCache := make([]byte, ProcCacheEntrySize)
	n, err := process.MarshalProcCache(procCache, time.Time{})
	require.NoError(t, err)
	assert.Equal(t, ProcCacheEntrySize, n)

	// Match process_event_t, including the fields following process_entry_t.
	data := make([]byte, ProcessEntrySize+PidCacheEntrySize+PathKeySize+28)
	copy(data, procCache[PathKeySize:])
	n, err = process.MarshalPidCache(data[ProcessEntrySize:], time.Time{})
	require.NoError(t, err)
	assert.Equal(t, PidCacheEntrySize, n)
	offset := ProcessEntrySize + PidCacheEntrySize + PathKeySize
	binary.NativeEndian.PutUint64(data[offset:offset+8], 1234)
	binary.NativeEndian.PutUint64(data[offset+8:offset+16], 5678)
	binary.NativeEndian.PutUint32(data[offset+16:offset+20], 1)
	binary.NativeEndian.PutUint32(data[offset+24:offset+28], 1)

	var decoded Process
	n, err = decoded.UnmarshalBinary(data)
	require.NoError(t, err)
	assert.Equal(t, len(data), n)
	assert.Equal(t, process.FileCapabilities, decoded.FileCapabilities)
	assert.Equal(t, process.Comm, decoded.Comm)
	assert.Equal(t, process.Credentials.UID, decoded.Credentials.UID)
	assert.Equal(t, process.Credentials.CapPermitted, decoded.Credentials.CapPermitted)
	assert.EqualValues(t, 1234, decoded.ArgsID)
	assert.EqualValues(t, 5678, decoded.EnvsID)
	assert.True(t, decoded.ArgsTruncated)
	assert.False(t, decoded.EnvsTruncated)
	assert.True(t, decoded.IsThroughSymLink)
}

func TestFileCapabilitiesAccessors(t *testing.T) {
	event := NewFakeEvent()
	event.Type = uint32(ExecEventType)
	event.Exec.Process = &event.ProcessContext.Process
	event.ProcessContext.FileCapabilities = FileCapabilities{
		Permitted:   1 << 10,
		Inheritable: 1 << 39,
		Effective:   true,
		Available:   true,
	}
	for _, prefix := range []string{"exec", "process"} {
		for _, field := range []struct {
			name string
			want interface{}
		}{
			{"permitted", int(1 << 10)},
			{"inheritable", int(1 << 39)},
			{"effective", true},
			{"available", true},
		} {
			value, err := event.GetFieldValue(prefix + ".file.capabilities." + field.name)
			require.NoError(t, err)
			assert.Equal(t, field.want, value)
		}
	}
}
