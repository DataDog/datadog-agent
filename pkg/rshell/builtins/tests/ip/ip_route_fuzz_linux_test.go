// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package ip_test

// Fuzz tests for ip route. These verify that the /proc/net/route parser and
// ip route get address parser never panic across arbitrary inputs.
//
// Seed corpus sources:
//
//	A. Implementation constants/boundaries: MaxLineBytes, maxRoutes, field layout
//	B. CVE/security-class inputs: null bytes, CRLF, overflow values, binary magic
//	C. Existing test coverage: all inputs from ip_linux_test.go and ip_pentest_linux_test.go

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/require"

	ipcmd "github.com/DataDog/datadog-agent/pkg/rshell/builtins/ip"
)

// writeFuzzRoute writes content to a temp proc directory (dir/net/route),
// acquires procNetRouteMu (defined in ip_linux_test.go), sets ProcNetRoutePath
// to dir, and returns a cleanup
// function that restores the original path and releases the lock.
// Used within fuzz functions where t.Cleanup is not available.
func writeFuzzRoute(t *testing.T, content []byte) (cleanup func()) {
	t.Helper()
	dir := t.TempDir()
	netDir := filepath.Join(dir, "net")
	require.NoError(t, os.MkdirAll(netDir, 0o755))
	if err := os.WriteFile(filepath.Join(netDir, "route"), content, 0o644); err != nil {
		// WriteFile failed; no lock was acquired yet, so no cleanup needed.
		// Return a no-op so the caller can safely defer it.
		return func() {}
	}
	procNetRouteMu.Lock()
	orig := ipcmd.ProcNetRoutePath
	ipcmd.ProcNetRoutePath = dir
	return func() {
		ipcmd.ProcNetRoutePath = orig
		procNetRouteMu.Unlock()
	}
}

// FuzzIPRouteParse fuzzes the /proc/net/route parser with arbitrary file content.
// The fuzzer verifies the parser never panics across arbitrary inputs.
//
// Seeds cover:
//
//	A. Implementation edge cases: empty, header only, valid rows, bad field counts
//	B. CVE-class inputs: null bytes, CRLF, binary magic, very long lines
//	C. Existing test content: all syntheticProcNetRoute variants
func FuzzIPRouteParse(f *testing.F) {
	// Source A: Implementation edge cases

	// Empty file
	f.Add([]byte{})

	// Header only
	f.Add([]byte("Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\n"))

	// Single valid default route
	f.Add([]byte("Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\n" +
		"eth0\t00000000\t0101A8C0\t0003\t0\t0\t100\t00000000\t0\t0\t0\n"))

	// Down route (RTF_UP not set)
	f.Add([]byte("Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\n" +
		"eth0\t00000000\t0101A8C0\t0000\t0\t0\t100\t00000000\t0\t0\t0\n"))

	// Too few fields
	f.Add([]byte("Iface\tDestination\n"))
	f.Add([]byte("eth0\t00000000\n"))

	// Invalid hex fields
	f.Add([]byte("Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\n" +
		"eth0\tZZZZZZZZ\t0101A8C0\t0003\t0\t0\t100\t00000000\t0\t0\t0\n"))

	// Metric at uint32 max boundary
	f.Add([]byte("Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\n" +
		"eth0\t00000000\t0101A8C0\t0003\t0\t0\t4294967295\t00000000\t0\t0\t0\n"))

	// Metric overflow (MaxUint32 + 1)
	f.Add([]byte("Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\n" +
		"eth0\t00000000\t0101A8C0\t0003\t0\t0\t4294967296\t00000000\t0\t0\t0\n"))

	// Multiple valid routes
	f.Add([]byte(syntheticProcNetRoute))

	// Source B: CVE-class inputs

	// Null bytes
	f.Add([]byte("Iface\x00Dest\n"))
	f.Add([]byte{0x00, 0x01, 0x02, 0x03, 0xFF, 0xFE})

	// CRLF line endings
	f.Add([]byte("Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\r\n" +
		"eth0\t00000000\t0101A8C0\t0003\t0\t0\t100\t00000000\t0\t0\t0\r\n"))

	// Binary magic bytes
	f.Add([]byte("\x7fELF\x02\x01\x01\x00")) // ELF
	f.Add([]byte("MZ\x90\x00\x03\x00"))      // PE
	f.Add([]byte("PK\x03\x04"))              // ZIP
	f.Add([]byte("\x1b[31mred\x1b[0m\n"))    // ANSI escape

	// Line near MaxLineBytes boundary
	almostMax := make([]byte, ipcmd.MaxLineBytes-1)
	for i := range almostMax {
		almostMax[i] = 'X'
	}
	almostMax[len(almostMax)-1] = '\n'
	f.Add(almostMax)

	// Source C: Malformed lines from ip_linux_test.go
	f.Add([]byte("Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\n" +
		"not-enough-fields\n" +
		"eth0\tZZZZZZZZ\t0101A8C0\t0003\t0\t0\t100\t00000000\t0\t0\t0\n" +
		"eth0\t00000000\t0101A8C0\t0003\t0\t0\t100\t00000000\t0\t0\t0\n"))

	f.Fuzz(func(t *testing.T, content []byte) {
		if len(content) > 1<<20 { // cap at 1 MiB
			return
		}

		cleanup := writeFuzzRoute(t, content)
		defer cleanup()

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()

		_, _, code := cmdRunCtxFuzz(ctx, t, "ip route show")
		timedOut := ctx.Err() == context.DeadlineExceeded
		cancel()
		if timedOut {
			t.Errorf("FuzzIPRouteParse: timed out on %d-byte input", len(content))
			return
		}
		if code == -1 {
			return // internal shell error before the builtin ran — not our bug
		}
		if code != 0 && code != 1 {
			t.Errorf("FuzzIPRouteParse: unexpected exit code %d", code)
		}
	})
}

// FuzzIPRouteGetAddr fuzzes the address argument to "ip route get ADDRESS".
// The fuzzer verifies parseIPv4 and routeGet never panic across arbitrary inputs.
//
// Seeds cover:
//
//	A. Valid addresses, degenerate addresses, boundary cases
//	B. CVE-class: long inputs, null bytes, injection attempts
//	C. Existing test inputs from ip_linux_test.go
func FuzzIPRouteGetAddr(f *testing.F) {
	// Source A: Valid and degenerate IPv4 addresses
	f.Add("0.0.0.0")
	f.Add("255.255.255.255")
	f.Add("127.0.0.1")
	f.Add("192.168.1.1")
	f.Add("10.0.0.1")
	f.Add("8.8.8.8")

	// Source A: Invalid — too few/many octets
	f.Add("192.168")
	f.Add("192.168.1")
	f.Add("1.2.3.4.5")

	// Source A: Octet boundary overflow
	f.Add("256.0.0.0")
	f.Add("0.0.0.256")
	f.Add("999.999.999.999")

	// Source B: CVE-class inputs
	f.Add("")
	f.Add(strings.Repeat("1", 1024) + ".0.0.0")
	f.Add("::1")         // IPv6
	f.Add("2001:db8::1") // IPv6
	f.Add("abc.def.ghi.jkl")
	f.Add(fmt.Sprintf("192.168.1.%d", 1<<31))
	f.Add("not-an-ip")
	f.Add("192.168.1.256")
	f.Add("192.168.1")

	f.Fuzz(func(t *testing.T, addr string) {
		if len(addr) > 256 {
			return
		}
		// Reject invalid UTF-8 — shell parser would reject it.
		if !utf8.ValidString(addr) {
			return
		}
		// Reject shell metacharacters that would change the script meaning.
		// ~ triggers tilde expansion which is blocked by the safe shell (exit 2).
		for _, ch := range []string{"\n", "\r", ";", "|", "&", "`", "$", "\"", "'", "(", ")", "{", "}", "<", ">", "~"} {
			if strings.Contains(addr, ch) {
				return
			}
		}

		cleanup := writeFuzzRoute(t, []byte(syntheticProcNetRoute))
		defer cleanup()

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()

		_, _, code := cmdRunCtxFuzz(ctx, t, "ip route get "+addr)
		timedOut := ctx.Err() == context.DeadlineExceeded
		cancel()
		if timedOut {
			t.Errorf("FuzzIPRouteGetAddr %q: timed out", addr)
			return
		}
		if code == -1 {
			return // internal shell error before the builtin ran — not our bug
		}
		if code != 0 && code != 1 {
			t.Errorf("FuzzIPRouteGetAddr %q: unexpected exit code %d", addr, code)
		}
	})
}
