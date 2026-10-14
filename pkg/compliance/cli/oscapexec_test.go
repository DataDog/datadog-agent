// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package cli

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// capSets holds the capability sets of one thread.
type capSets struct{ eff, prm, inh uint64 }

// threadCaps returns the capability sets of the calling thread.
func threadCaps() (capSets, error) {
	hdr := unix.CapUserHeader{Version: unix.LINUX_CAPABILITY_VERSION_3}
	var d [2]unix.CapUserData
	if err := unix.Capget(&hdr, &d[0]); err != nil {
		return capSets{}, err
	}
	return capSets{
		eff: uint64(d[1].Effective)<<32 | uint64(d[0].Effective),
		prm: uint64(d[1].Permitted)<<32 | uint64(d[0].Permitted),
		inh: uint64(d[1].Inheritable)<<32 | uint64(d[0].Inheritable),
	}, nil
}

// removeCaps clears caps from the calling thread, as a container runtime that
// withholds them would. Removing a capability needs no privilege.
func removeCaps(caps []int) error {
	hdr := unix.CapUserHeader{Version: unix.LINUX_CAPABILITY_VERSION_3}
	var d [2]unix.CapUserData
	if err := unix.Capget(&hdr, &d[0]); err != nil {
		return err
	}
	for _, c := range caps {
		m := ^uint32(1 << (c % 32))
		d[c/32].Effective &= m
		d[c/32].Permitted &= m
		d[c/32].Inheritable &= m
	}
	return unix.Capset(&hdr, &d[0])
}

// dropOnThread removes missing and runs dropCapabilities on a thread of its own,
// and returns the thread's sets before and after the drop. The goroutine exits
// locked, so the runtime discards the thread and the test keeps its capabilities.
func dropOnThread(missing []int) (before, after capSets, err error) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		runtime.LockOSThread()
		if err = removeCaps(missing); err != nil {
			return
		}
		if before, err = threadCaps(); err != nil {
			return
		}
		if err = dropCapabilities(); err != nil {
			return
		}
		after, err = threadCaps()
	}()
	<-done
	return before, after, err
}

// TestDropCapabilities checks that oscap-io keeps the capabilities it needs that
// the agent holds. As root, each case first removes what a container runtime
// withholds. An unprivileged run starts and ends with empty sets.
func TestDropCapabilities(t *testing.T) {
	const keep = 1<<unix.CAP_DAC_OVERRIDE | 1<<unix.CAP_NET_ADMIN |
		1<<unix.CAP_NET_RAW | 1<<unix.CAP_SYS_CHROOT
	for _, tt := range []struct {
		name    string
		missing []int
	}{
		{"host", nil},
		{"docker", []int{unix.CAP_NET_ADMIN}},
		{"crio", []int{unix.CAP_NET_ADMIN, unix.CAP_NET_RAW, unix.CAP_SYS_CHROOT}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			before, after, err := dropOnThread(tt.missing)
			require.NoError(t, err)
			want := keep & before.prm
			assert.Equal(t, capSets{eff: want, prm: want, inh: want}, after,
				"permitted before the drop: %#x", before.prm)
		})
	}
}
