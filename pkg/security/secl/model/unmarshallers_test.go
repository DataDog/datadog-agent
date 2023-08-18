// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const syscallsEventByteCount = 64

func syscallListEqual(t *testing.T, a, b []Syscall) bool {
mainLoop:
	for _, elemA := range a {
		for _, elemB := range b {
			if elemA == elemB {
				continue mainLoop
			}
		}
		t.Logf("syscall %s is missing", elemA)
		return false
	}
	return true
}

func allSyscallsTest() syscallsEventTest {
	all := syscallsEventTest{
		name: "all_syscalls",
		args: make([]byte, syscallsEventByteCount),
	}

	for i := 0; i < syscallsEventByteCount*8; i++ {
		all.want = append(all.want, Syscall(i))

		// should be tested in eBPF...
		index := i / 8
		bit := byte(1 << (i % 8))
		all.args[index] |= bit
	}

	return all
}

func oneSyscallTest(s Syscall) syscallsEventTest {
	one := syscallsEventTest{
		name: s.String(),
		args: make([]byte, syscallsEventByteCount),
	}

	// should be tested in eBPF ...
	index := s / 8
	bit := byte(1 << (s % 8))
	one.args[index] |= bit

	one.want = []Syscall{s}
	return one
}

type syscallsEventTest struct {
	name    string
	args    []byte
	want    []Syscall
	wantErr error
}

func TestSyscallsEvent_UnmarshalBinary(t *testing.T) {
	tests := []syscallsEventTest{
		{
			name:    "nil_array",
			wantErr: ErrNotEnoughData,
		},
		{
			name: "no_syscall",
			args: make([]byte, 64),
		},
		allSyscallsTest(),
	}

	// add single syscall tests
	for i := 0; i < syscallsEventByteCount*8; i++ {
		tests = append(tests, oneSyscallTest(Syscall(i)))
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &SyscallsEvent{}
			_, err := e.UnmarshalBinary(tt.args)
			if err != nil {
				if err == tt.wantErr {
					return
				}
				assert.Equal(t, nil, err, "expected normal unmarshalling")
			}
			assert.Equal(t, true, syscallListEqual(t, tt.want, e.Syscalls), "invalid list of syscalls: %s, expected: %s", e.Syscalls, tt.want)
		})
	}
}
