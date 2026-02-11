// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package model holds model related files
package model

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type testIntegerType interface {
	uint64 | uint32 | uint16
}

type testInteger[T testIntegerType] struct {
	value   T
	binData []byte
}

func newTestInteger[T testIntegerType](value T) *testInteger[T] {
	var buffer []byte
	switch value := any(value).(type) {
	case uint64:
		buffer = make([]byte, 8)
		binary.NativeEndian.PutUint64(buffer, value)
	case uint32:
		buffer = make([]byte, 4)
		binary.NativeEndian.PutUint32(buffer, value)
	case uint16:
		buffer = make([]byte, 2)
		binary.NativeEndian.PutUint16(buffer, value)
	default:
		panic("unsupported type")
	}

	return &testInteger[T]{
		value:   value,
		binData: buffer,
	}
}

func TestMarshalProcCache(t *testing.T) {
	const procCacheSize = 176

	testCGroupFileInode := newTestInteger(uint64(123456789))
	testCGroupFileMountID := newTestInteger(uint32(24))
	testCGroupFilePathID := newTestInteger(uint32(808))
	testFileInode := newTestInteger(uint64(987654321))
	testFileMountID := newTestInteger(uint32(42))
	testFilePathID := newTestInteger(uint32(404))
	testFileFlags := newTestInteger(uint32(3))
	testFileUID := newTestInteger(uint32(1000))
	testFileGID := newTestInteger(uint32(1000))
	testFileNLink := newTestInteger(uint32(2))
	testFileMode := newTestInteger(uint16(0755))
	testFileCTime := newTestInteger(uint64(622547800))
	testFileMTime := newTestInteger(uint64(622547800))

	testBootTime := time.Unix(159, 0)
	testExecTime := time.Unix(951, 0)
	testExecEpochTime := newTestInteger(uint64(testExecTime.Sub(testBootTime)))

	testCases := []struct {
		name               string
		process            Process
		bootTime           time.Time
		expectedDataChunks [][]byte
	}{
		{
			name:     "empty process",
			process:  Process{},
			bootTime: time.Time{},
			expectedDataChunks: [][]byte{
				make([]byte, procCacheSize),
			},
		},
		{
			name: "process with comm",
			process: Process{
				Comm: "test-process",
			},
			bootTime: time.Time{},
			expectedDataChunks: [][]byte{
				make([]byte, procCacheSize-16),
				{
					't', 'e', 's', 't', '-', 'p', 'r', 'o',
					'c', 'e', 's', 's', 0x00, 0x00, 0x00, 0x00,
				},
			},
		},
		{
			name: "process-with-cgroup-key",
			process: Process{
				CGroup: CGroupContext{
					CGroupPathKey: PathKey{
						Inode:   testCGroupFileInode.value,
						MountID: testCGroupFileMountID.value,
						PathID:  testCGroupFilePathID.value,
					},
				},
				FileEvent: FileEvent{
					FileFields: FileFields{
						PathKey: PathKey{
							Inode:   testFileInode.value,
							MountID: testFileMountID.value,
							PathID:  testFilePathID.value,
						},
						Flags: int32(testFileFlags.value),
						UID:   testFileUID.value,
						GID:   testFileGID.value,
						NLink: testFileNLink.value,
						Mode:  testFileMode.value,
						CTime: testFileCTime.value,
						MTime: testFileMTime.value,
					},
				},
				ExecTime: testExecTime,
				TTYName:  "tty1",
				Comm:     "test-process",
			},
			bootTime: testBootTime,
			expectedDataChunks: [][]byte{
				testCGroupFileInode.binData,
				testCGroupFileMountID.binData,
				testCGroupFilePathID.binData,
				testFileInode.binData,
				testFileMountID.binData,
				testFilePathID.binData,
				testFileFlags.binData,
				make([]byte, 4), // padding
				testFileUID.binData,
				testFileGID.binData,
				testFileNLink.binData,
				testFileMode.binData,
				make([]byte, 2), // padding
				make([]byte, 8), // ctime sec
				testFileCTime.binData,
				make([]byte, 8), // mtime sec
				testFileMTime.binData,
				testExecEpochTime.binData,
				{
					't', 't', 'y', '1', 0x00, 0x00, 0x00, 0x00,
					0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
					0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
					0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
					0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
					0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
					0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
					0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				},
				{
					't', 'e', 's', 't', '-', 'p', 'r', 'o',
					'c', 'e', 's', 's', 0x00, 0x00, 0x00, 0x00,
				},
			},
		},
		{
			name: "process-without-cgroup-key",
			process: Process{
				FileEvent: FileEvent{
					FileFields: FileFields{
						PathKey: PathKey{
							Inode:   testFileInode.value,
							MountID: testFileMountID.value,
							PathID:  testFilePathID.value,
						},
						Flags: int32(testFileFlags.value),
						UID:   testFileUID.value,
						GID:   testFileGID.value,
						NLink: testFileNLink.value,
						Mode:  testFileMode.value,
						CTime: testFileCTime.value,
						MTime: testFileMTime.value,
					},
				},
				ExecTime: testExecTime,
				TTYName:  "tty1",
				Comm:     "test-process",
			},
			bootTime: testBootTime,
			expectedDataChunks: [][]byte{
				make([]byte, 8), // cgroup file inode
				make([]byte, 4), // cgroup file mount id
				make([]byte, 4), // cgroup file path id
				testFileInode.binData,
				testFileMountID.binData,
				testFilePathID.binData,
				testFileFlags.binData,
				make([]byte, 4), // padding
				testFileUID.binData,
				testFileGID.binData,
				testFileNLink.binData,
				testFileMode.binData,
				make([]byte, 2), // padding
				make([]byte, 8), // ctime sec
				testFileCTime.binData,
				make([]byte, 8), // mtime sec
				testFileMTime.binData,
				testExecEpochTime.binData,
				{
					't', 't', 'y', '1', 0x00, 0x00, 0x00, 0x00,
					0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
					0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
					0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
					0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
					0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
					0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
					0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				},
				{
					't', 'e', 's', 't', '-', 'p', 'r', 'o',
					'c', 'e', 's', 's', 0x00, 0x00, 0x00, 0x00,
				},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			var expectedData []byte
			for _, chunk := range testCase.expectedDataChunks {
				expectedData = append(expectedData, chunk...)
			}

			data := make([]byte, procCacheSize)
			n, err := testCase.process.MarshalProcCache(data, testCase.bootTime)
			if err != nil {
				t.Fatalf("failed to marshal process: %v", err)
			}

			assert.Equal(t, len(expectedData), n)
			assert.Equal(t, len(expectedData), len(data))
			assert.Equal(t, expectedData, data)
		})
	}
}
