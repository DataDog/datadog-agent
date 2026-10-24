// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package ss

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/internal/procnetsocket"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/internal/procpath"
)

// ProcPath is the proc filesystem root used to locate /proc/net/* files.
// It is a package-level variable so tests can point it at a synthetic directory
// instead of the real /proc.
//
// Concurrency contract: this variable is written only in tests and is never
// mutated by production code after package initialization. Test code that
// mutates ProcPath must hold a test-package-level mutex for the duration of
// the test to prevent data races between concurrent test goroutines.
var ProcPath = procpath.Default

// run is the Linux implementation. It reads socket state from /proc/net/.
func run(ctx context.Context, callCtx *builtins.CallContext, opts options) builtins.Result {
	var entries []socketEntry
	var firstErr error

	collect := func(fn func(context.Context, string) ([]procnetsocket.SocketEntry, error)) {
		if firstErr != nil {
			return
		}
		got, err := fn(ctx, ProcPath)
		if err != nil {
			firstErr = err
			return
		}
		for _, e := range got {
			entries = append(entries, toSocketEntry(e))
		}
	}

	if opts.showTCP {
		if !opts.ipv6Only {
			collect(procnetsocket.ReadTCP4)
		}
		if !opts.ipv4Only {
			collect(procnetsocket.ReadTCP6)
		}
	}
	if opts.showUDP {
		if !opts.ipv6Only {
			collect(procnetsocket.ReadUDP4)
		}
		if !opts.ipv4Only {
			collect(procnetsocket.ReadUDP6)
		}
	}
	if opts.showUnix {
		collect(procnetsocket.ReadUnix)
	}

	if firstErr != nil {
		callCtx.Errf("ss: %v\n", firstErr)
		return builtins.Result{Code: 1}
	}

	// Summary mode: count and print statistics, then return.
	if opts.summary {
		printSummary(callCtx, entries)
		return builtins.Result{}
	}

	// Filter entries and print.
	printHeader(callCtx, opts)
	for _, e := range entries {
		if filterEntry(opts, e) {
			printEntry(callCtx, opts, e)
		}
	}
	return builtins.Result{}
}

// toSocketEntry converts a procnetsocket.SocketEntry to the ss-internal
// socketEntry type.
func toSocketEntry(e procnetsocket.SocketEntry) socketEntry {
	kind := sockTCP4
	switch e.Kind {
	case procnetsocket.KindTCP6:
		kind = sockTCP6
	case procnetsocket.KindUDP4:
		kind = sockUDP4
	case procnetsocket.KindUDP6:
		kind = sockUDP6
	case procnetsocket.KindUnix:
		kind = sockUnix
	}
	return socketEntry{
		kind:        kind,
		state:       e.State,
		recvQ:       e.RecvQ,
		sendQ:       e.SendQ,
		localAddr:   e.LocalAddr,
		localPort:   e.LocalPort,
		peerAddr:    e.PeerAddr,
		peerPort:    e.PeerPort,
		uid:         e.UID,
		inode:       e.Inode,
		hasExtended: e.HasExtended,
	}
}
