// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux_bpf

package http

import (
	"fmt"
	"os"
	"strings"
)

// txCmdline returns a best-effort process command line for the PID associated
// with the given transaction. Used by the temporary discovery/USM diagnostics
// in statkeeper.go. Returns "" if the PID is unknown, the proc entry is gone,
// or the transaction type doesn't expose a PID.
func txCmdline(tx Transaction) string {
	e, ok := tx.(*EbpfEvent)
	if !ok || e.Tuple.Pid == 0 {
		return ""
	}
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", e.Tuple.Pid))
	if err != nil {
		return ""
	}
	return strings.ReplaceAll(strings.TrimRight(string(b), "\x00"), "\x00", " ")
}
