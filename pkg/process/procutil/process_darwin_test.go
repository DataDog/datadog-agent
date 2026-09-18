// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package procutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestProcessFromPIDUsesKernelStartTime(t *testing.T) {
	const startSeconds int64 = 1700000000
	for _, tc := range []struct {
		name         string
		usec         int32
		milliseconds int64
	}{
		{"whole second", 0, 0},
		{"less than a millisecond", 999, 0},
		{"one millisecond", 1000, 1},
		{"subsecond precision", 123456, 123},
		{"last millisecond", 999999, 999},
	} {
		t.Run(tc.name, func(t *testing.T) {
			kernel := &unix.KinfoProc{}
			kernel.Proc.P_starttime = unix.Timeval{Sec: startSeconds, Usec: tc.usec}
			fields := []string{"123", "1", "0:00.00", "0:00.00", "00:01", "Z", "0", "0", "0", "zombie"}
			first, err := processFromPID(123, fields, kernel)
			require.NoError(t, err)
			assert.Equal(t, startSeconds*1000+tc.milliseconds, first.Stats.CreateTime)

			// Changing the ps elapsed time cannot change the same process's identity.
			fields[4] = "1-03:30:00"
			next, err := processFromPID(123, fields, kernel)
			require.NoError(t, err)
			assert.Equal(t, first.Stats.CreateTime, next.Stats.CreateTime)
			assert.True(t, IsSameProcess(first, next))
		})
	}
}
