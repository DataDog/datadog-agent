// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package tc

import (
	"errors"
	"fmt"
	"os"
	"runtime"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/vishvananda/netns"
)

// RunInNetNS runs fn from the network namespace ns, on a new goroutine locked to its thread. The thread stays locked
// when it can't get back to its own namespace, so that the runtime terminates it instead of reusing it.
func RunInNetNS(ns *os.File, fn func() error) error {
	errCh := make(chan error, 1)
	go func() {
		runtime.LockOSThread()

		origin, err := netns.Get()
		if err != nil {
			runtime.UnlockOSThread()
			errCh <- fmt.Errorf("couldn't get the current network namespace: %w", err)
			return
		}
		defer origin.Close()

		if err := netns.Set(netns.NsHandle(ns.Fd())); err != nil {
			runtime.UnlockOSThread()
			errCh <- fmt.Errorf("couldn't enter the network namespace: %w", err)
			return
		}

		fnErr := fn()
		if err := netns.Set(origin); err != nil {
			errCh <- errors.Join(fnErr, fmt.Errorf("couldn't restore the network namespace: %w", err))
			return
		}
		runtime.UnlockOSThread()
		errCh <- fnErr
	}()
	return <-errCh
}

// QueryTCXPrograms returns the programs of a TCX hook of the interface ifindex, in the order in which they run. The
// kernel resolves ifindex in the network namespace of the caller.
func QueryTCXPrograms(ifindex int, direction manager.TrafficType) ([]TCXProgram, error) {
	attach := ebpf.AttachTCXEgress
	if direction == manager.Ingress {
		attach = ebpf.AttachTCXIngress
	}

	result, err := link.QueryPrograms(link.QueryOptions{Target: ifindex, Attach: attach})
	if err != nil {
		return nil, err
	}

	programs := make([]TCXProgram, 0, len(result.Programs))
	for _, prog := range result.Programs {
		linkID, _ := prog.LinkID()
		programs = append(programs, TCXProgram{ID: uint32(prog.ID), LinkID: uint32(linkID)})
	}
	return programs, nil
}
