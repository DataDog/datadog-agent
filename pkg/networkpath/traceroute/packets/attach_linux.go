// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package packets

import (
	"errors"
	"fmt"
	"math"
	"syscall"
	"unsafe"

	"golang.org/x/net/bpf"
	"golang.org/x/sys/unix"
)

// SetBPF attaches a BPF filter to the underlying socket
func SetBPF(c syscall.RawConn, filter []bpf.RawInstruction) error {
	var p unix.SockFprog
	if len(filter) > math.MaxUint16 {
		return errors.New("filter too large")
	}
	p.Len = uint16(len(filter))
	p.Filter = (*unix.SockFilter)(unsafe.Pointer(&filter[0]))

	var sockoptErr error
	err := c.Control(func(fd uintptr) {
		sockoptErr = unix.SetsockoptSockFprog(int(fd), unix.SOL_SOCKET, unix.SO_ATTACH_FILTER, &p)
	})
	err = errors.Join(err, sockoptErr)
	if err != nil {
		return fmt.Errorf("SetBPF failed to attach filter: %w", err)
	}
	return nil
}

// this is a simple BPF program that drops all packets no matter what
var dropAllFilter = []bpf.RawInstruction{
	{Op: 0x6, Jt: 0, Jf: 0, K: 0x00000000},
}

// SetBPFAndDrain sets the filter for a raw socket and drains old data, so that
// new packets are guaranteed to match the filter
func SetBPFAndDrain(c syscall.RawConn, filter []bpf.RawInstruction) error {
	// unfortunately there is truly no way to atomically create a raw socket and attach a filter.
	// when there is a lot of traffic, unwanted packets will always come through during initialization.
	// this means we have to do some hijinks to guarantee the correctness of socket reading.
	// details in this fantastic blog post: https://natanyellin.com/posts/ebpf-filtering-done-right/

	// first, stop new traffic from coming in by dropping everything
	err := SetBPF(c, dropAllFilter)
	if err != nil {
		return err
	}

	// then, drain this socket so there is no data
	var recvErr error
	err = c.Control(func(fd uintptr) {
		var buf [1]byte

		var n int
		for n >= 0 {
			n, _, recvErr = syscall.Recvfrom(int(fd), buf[:], syscall.MSG_DONTWAIT)
		}
	})
	if err != nil {
		return fmt.Errorf("SetBPFAndDrain control failed: %w", err)
	}
	if recvErr != syscall.EAGAIN {
		return fmt.Errorf("SetBPFAndDrain failed to drain: %w", err)
	}

	// lastly, set the intended filter and it's ready to go
	err = SetBPF(c, filter)
	if err != nil {
		return err
	}

	return nil
}
