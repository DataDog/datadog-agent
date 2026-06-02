// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package model holds model related files
package model

import (
	"testing"
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
		args: make([]byte, syscallsEventByteCount+8),
	}

	for i := 0; i < syscallsEventByteCount*8; i++ {
		all.want = append(all.want, Syscall(i))

		// should be tested in eBPF...
		index := i / 8
		bit := byte(1 << (i % 8))
		all.args[index+8] |= bit
	}

	return all
}

func oneSyscallTest(s Syscall) syscallsEventTest {
	one := syscallsEventTest{
		name: s.String(),
		args: make([]byte, syscallsEventByteCount+8),
	}

	// should be tested in eBPF ...
	index := s / 8
	bit := byte(1 << (s % 8))
	one.args[index+8] |= bit

	one.want = []Syscall{s}
	return one
}

type syscallsEventTest struct {
	name    string
	args    []byte
	want    []Syscall
	wantErr error
}
