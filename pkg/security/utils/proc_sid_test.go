// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package utils

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/unix"
)

func TestPidSID(t *testing.T) {
	pid := uint32(os.Getpid())
	sid := PidSID(pid)

	// our own SID should match what getsid(0) returns
	expected, err := unix.Getsid(0)
	if err != nil {
		t.Fatalf("getsid(0) failed: %v", err)
	}
	assert.Equal(t, uint32(expected), sid)
}

func TestPidSID_InvalidPid(t *testing.T) {
	sid := PidSID(0xFFFFFFFF)
	assert.Equal(t, uint32(0), sid)
}
