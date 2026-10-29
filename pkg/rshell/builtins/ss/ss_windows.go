// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package ss

import (
	"context"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/internal/winnet"
)

// run is the Windows implementation. It collects socket data via iphlpapi.dll.
func run(ctx context.Context, callCtx *builtins.CallContext, opts options) builtins.Result {
	if ctx.Err() != nil {
		return builtins.Result{Code: 1}
	}
	raw, err := winnet.Collect()
	if err != nil {
		callCtx.Errf("ss: %v\n", err)
		return builtins.Result{Code: 1}
	}

	// Convert winnet.SocketEntry → socketEntry.
	entries := make([]socketEntry, 0, len(raw))
	for _, r := range raw {
		if ctx.Err() != nil {
			return builtins.Result{Code: 1}
		}
		var kind socketType
		switch r.Proto {
		case "tcp4":
			kind = sockTCP4
		case "tcp6":
			kind = sockTCP6
		case "udp4":
			kind = sockUDP4
		case "udp6":
			kind = sockUDP6
		default:
			continue
		}

		// Apply per-type protocol and IP-version filters here, before building
		// the socketEntry slice. This differs from the Linux/macOS approach where
		// all entries are collected first and filterEntry() handles all filtering
		// in the print loop. The behaviour is identical; the early skip here
		// avoids allocating socketEntry values for entries that will never be
		// printed. filterEntry() is still called in the print loop below for
		// the state/listening filters that this path does not handle.
		if (kind == sockTCP4 || kind == sockTCP6) && !opts.showTCP {
			continue
		}
		if (kind == sockUDP4 || kind == sockUDP6) && !opts.showUDP {
			continue
		}
		if opts.ipv4Only && !opts.ipv6Only && (kind == sockTCP6 || kind == sockUDP6) {
			continue
		}
		if opts.ipv6Only && !opts.ipv4Only && (kind == sockTCP4 || kind == sockUDP4) {
			continue
		}

		entries = append(entries, socketEntry{
			kind:      kind,
			state:     r.State,
			localAddr: r.LocalIP,
			localPort: strconv.Itoa(int(r.LocalPort)),
			peerAddr:  r.RemoteIP,
			peerPort:  strconv.Itoa(int(r.RemotePort)),
		})
	}

	if opts.summary {
		printSummary(callCtx, entries)
		return builtins.Result{}
	}

	printHeader(callCtx, opts)
	for _, e := range entries {
		if filterEntry(opts, e) {
			printEntry(callCtx, opts, e)
		}
	}
	return builtins.Result{}
}
